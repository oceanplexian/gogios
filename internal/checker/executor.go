package checker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// checkJob holds all parameters for a single check execution.
type checkJob struct {
	hostName     string
	svcDesc      string
	command      string
	timeout      time.Duration
	checkOptions int
	checkType    int
	latency      float64
}

// Executor runs check plugins with a fixed-size worker pool.
// Workers are started once in NewExecutor and read jobs from a buffered
// channel, eliminating the goroutine-per-check overhead that caused
// memory explosion at scale (e.g. 500k goroutines for 500k services).
//
// Each worker owns a persistent /bin/sh process (fork server) to avoid
// expensive fork() calls from the large Go parent process.
type Executor struct {
	jobCh       chan checkJob
	jobsRunning atomic.Int64
	resultCh    chan *objects.CheckResult
	workers     int
	sentinel    string
}

// NewExecutor creates an executor with the given concurrency limit.
// resultCh is where completed check results are sent.
func NewExecutor(maxConcurrent int, resultCh chan *objects.CheckResult) *Executor {
	if maxConcurrent <= 0 {
		maxConcurrent = 256
	}

	// Generate a random sentinel for fork server protocol
	sentinelBytes := make([]byte, 16)
	if _, err := rand.Read(sentinelBytes); err != nil {
		log.Printf("Warning: could not generate random sentinel: %v", err)
	}
	sentinel := hex.EncodeToString(sentinelBytes)

	e := &Executor{
		jobCh:    make(chan checkJob, maxConcurrent*4),
		resultCh: resultCh,
		workers:  maxConcurrent,
		sentinel: sentinel,
	}
	for i := 0; i < maxConcurrent; i++ {
		go e.forkServerWorker()
	}
	return e
}

// Workers returns the configured worker pool size.
func (e *Executor) Workers() int {
	return e.workers
}

// JobsRunning returns the current number of executing checks.
func (e *Executor) JobsRunning() int64 {
	return e.jobsRunning.Load()
}

// Submit sends a check for async execution. If the job channel buffer
// is full, a temporary goroutine is spawned to avoid blocking the
// scheduler's event loop.
func (e *Executor) Submit(hostName, svcDesc, command string, timeout time.Duration, checkOptions int, checkType int, latency float64) {
	job := checkJob{
		hostName:     hostName,
		svcDesc:      svcDesc,
		command:      command,
		timeout:      timeout,
		checkOptions: checkOptions,
		checkType:    checkType,
		latency:      latency,
	}
	select {
	case e.jobCh <- job:
		// sent without blocking
	default:
		// buffer full — spawn a short-lived goroutine to avoid blocking scheduler
		go func() { e.jobCh <- job }()
	}
}

// Stop shuts down all workers. Blocks until all in-flight checks complete.
func (e *Executor) Stop() {
	close(e.jobCh)
}

// forkServerWorker owns a persistent shell process and processes jobs through it.
// If the shell can't be started or crashes irrecoverably, falls back to runPlugin.
func (e *Executor) forkServerWorker() {
	var sw *shellWorker

	// Try to start the shell worker
	var err error
	sw, err = newShellWorker(e.sentinel)
	if err != nil {
		log.Printf("Fork server: could not start shell worker, falling back to direct exec: %v", err)
		sw = nil
	}

	defer func() {
		if sw != nil {
			sw.Close()
		}
	}()

	for job := range e.jobCh {
		e.jobsRunning.Add(1)
		cr := e.runViaShell(sw, job)
		if cr == nil {
			// Shell failed, try respawn
			if sw != nil {
				sw.Close()
			}
			sw, err = newShellWorker(e.sentinel)
			if err != nil {
				sw = nil
			}
			// Retry via shell or fall back
			cr = e.runViaShell(sw, job)
			if cr == nil {
				// Final fallback to direct exec
				cr = e.runPlugin(job.hostName, job.svcDesc, job.command, job.timeout, job.checkOptions, job.checkType, job.latency)
			}
		}
		e.jobsRunning.Add(-1)
		e.resultCh <- cr
	}
}

// runViaShell executes a check through the persistent shell worker.
// Returns nil if the shell is unavailable or the command failed at the protocol level.
func (e *Executor) runViaShell(sw *shellWorker, job checkJob) *objects.CheckResult {
	if sw == nil || !sw.alive {
		return nil
	}

	cr := &objects.CheckResult{
		HostName:           job.hostName,
		ServiceDescription: job.svcDesc,
		CheckType:          job.checkType,
		CheckOptions:       job.checkOptions,
		Latency:            job.latency,
		ExitedOK:           true,
	}

	cr.StartTime = time.Now()
	output, exitCode, err := sw.Run(job.command, job.timeout)
	cr.FinishTime = time.Now()
	cr.ExecutionTime = cr.FinishTime.Sub(cr.StartTime).Seconds()

	if err != nil {
		// Shell-level failure (timeout killed the shell, crash, etc.)
		// Check if it was a timeout
		if !sw.alive {
			// Shell was killed — likely timeout
			cr.EarlyTimeout = true
			cr.ReturnCode = 2
			cr.Output = fmt.Sprintf("(Check timed out after %.0f seconds)", job.timeout.Seconds())
			return cr
		}
		return nil // signal caller to retry/fallback
	}

	cr.ReturnCode = exitCode
	if output != "" {
		cr.Output = output
	}

	return cr
}

// runPlugin executes the command via direct fork+exec and captures output/return code.
// Used as fallback when the fork server is unavailable.
func (e *Executor) runPlugin(hostName, svcDesc, command string, timeout time.Duration, checkOptions int, checkType int, latency float64) *objects.CheckResult {
	cr := &objects.CheckResult{
		HostName:           hostName,
		ServiceDescription: svcDesc,
		CheckType:          checkType,
		CheckOptions:       checkOptions,
		Latency:            latency,
		ExitedOK:           true,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cr.StartTime = time.Now()
	err := cmd.Run()
	cr.FinishTime = time.Now()
	cr.ExecutionTime = cr.FinishTime.Sub(cr.StartTime).Seconds()

	// Extract return code
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			cr.EarlyTimeout = true
			cr.ReturnCode = 2
			cr.Output = fmt.Sprintf("(Check timed out after %.0f seconds)", timeout.Seconds())
			return cr
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			if ws, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				cr.ReturnCode = ws.ExitStatus()
			} else {
				cr.ReturnCode = 2
				cr.ExitedOK = false
			}
		} else {
			// Could not execute at all (e.g., command not found)
			cr.ReturnCode = 127
			cr.ExitedOK = false
			cr.Output = fmt.Sprintf("(Could not execute plugin: %v)", err)
			return cr
		}
	} else {
		cr.ReturnCode = 0
	}

	// Capture output
	if stdout.Len() > 0 {
		out := stdout.String()
		if len(out) > 8192 {
			out = out[:8192]
		}
		cr.Output = out
	} else if stderr.Len() > 0 {
		out := stderr.String()
		if len(out) > 8192 {
			out = out[:8192]
		}
		cr.Output = "(No output on stdout) stderr: " + out
	}

	return cr
}
