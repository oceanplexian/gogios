package checker

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Executor runs check plugins with a bounded goroutine pool.
type Executor struct {
	sem          chan struct{}
	jobsRunning  atomic.Int64
	resultCh     chan *objects.CheckResult
	maxTimeout   time.Duration
}

// NewExecutor creates an executor with the given concurrency limit.
// resultCh is where completed check results are sent.
func NewExecutor(maxConcurrent int, resultCh chan *objects.CheckResult) *Executor {
	if maxConcurrent <= 0 {
		maxConcurrent = 256
	}
	return &Executor{
		sem:      make(chan struct{}, maxConcurrent),
		resultCh: resultCh,
	}
}

// JobsRunning returns the current number of executing checks.
func (e *Executor) JobsRunning() int64 {
	return e.jobsRunning.Load()
}

// Submit sends a check for async execution. It blocks if the pool is full.
func (e *Executor) Submit(hostName, svcDesc, command string, timeout time.Duration, checkOptions int, checkType int, latency float64) {
	e.sem <- struct{}{} // acquire slot
	e.jobsRunning.Add(1)
	go func() {
		defer func() {
			<-e.sem // release slot
			e.jobsRunning.Add(-1)
		}()
		cr := e.runPlugin(hostName, svcDesc, command, timeout, checkOptions, checkType, latency)
		e.resultCh <- cr
	}()
}

// runPlugin executes the command and captures output/return code.
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
