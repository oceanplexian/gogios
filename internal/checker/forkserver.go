package checker

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

// ErrCheckTimeout is returned by Run when the in-flight check exceeded its
// timeout. The persistent shell worker stays alive — only the subshell
// running the check is killed — so the same worker can serve the next check.
var ErrCheckTimeout = errors.New("check timed out")

// shellScript is the bash loop that each persistent shell worker runs.
// `set -m` (monitor mode / job control) makes every backgrounded job a
// process-group leader (pgid == pid), so the Go side can SIGKILL just that
// pgrp on timeout without disturbing the long-lived worker. The subshell's
// PID is written to fd 3 (a dedicated control pipe) before wait(), so the
// timeout can target the in-flight check immediately. After wait() returns,
// the sentinel line carries the subshell's exit status and the worker loops
// to the next command.
//
// Each check runs inside a fresh PID namespace via util-linux `unshare`.
// When SIGKILL hits the subshell's pgroup on timeout, unshare (and the
// shell it forked inside the namespace) die, and the kernel atomically
// tears down the namespace — every plugin descendant (fping under
// check_fping, etc.) dies and is reaped inside the namespace. No
// reparenting to PID 1, no orphan zombies.
//
// We use the C `unshare` binary rather than rolling our own Go shim
// because forking from a Go process is expensive (large mm → page-table
// copy contends on mmap_lock under load); unshare is ~85 KB and forks
// near-instantly. If unshare isn't installed we fall back to plain eval.
//
// bash (not /bin/sh) is required because dash refuses to enable job control
// without a controlling terminal.
const shellScript = `set -m
s="$1"
if [ -x /usr/bin/unshare ]; then
  spawn() { ( exec /usr/bin/unshare --pid --fork /bin/sh -c "$1" ) </dev/null 2>&1 3>&- & }
else
  spawn() { ( eval "$1" ) </dev/null 2>&1 3>&- & }
fi
while IFS= read -r c; do
  spawn "$c"
  pid=$!
  printf '%d\n' "$pid" >&3
  wait "$pid" 2>/dev/null
  printf '%s %d\n' "$s" $?
done`

// shellWorker manages a single persistent /bin/sh process that executes
// commands via pipe. Each command runs in its own session/process group
// (setsid in the loop) and the subshell PID is reported via a dedicated
// control pipe (fd 3 in the child) so timeouts target only the in-flight
// check, not the worker itself.
type shellWorker struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Scanner
	ctl      *bufio.Reader
	ctlFile  *os.File
	sentinel string
	alive    bool
}

// newShellWorker starts a persistent /bin/sh process running the read-eval loop.
func newShellWorker(sentinel string) (*shellWorker, error) {
	ctlR, ctlW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("ctl pipe: %w", err)
	}

	cmd := exec.Command("/bin/bash", "-c", shellScript, "--", sentinel)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.ExtraFiles = []*os.File{ctlW} // becomes fd 3 in the child

	stdin, err := cmd.StdinPipe()
	if err != nil {
		ctlR.Close()
		ctlW.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		ctlR.Close()
		ctlW.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		ctlR.Close()
		ctlW.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	// Close our copy of the write end — the child has its own.
	ctlW.Close()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB lines

	return &shellWorker{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   scanner,
		ctl:      bufio.NewReader(ctlR),
		ctlFile:  ctlR,
		sentinel: sentinel,
		alive:    true,
	}, nil
}

// Run sends a command to the persistent shell and reads output until the
// sentinel line. Returns the captured output, the subshell's exit code, and
// any error. On timeout, only the subshell's process group is killed — the
// worker stays alive — and the returned error is ErrCheckTimeout.
func (sw *shellWorker) Run(command string, timeout time.Duration) (output string, exitCode int, err error) {
	if !sw.alive {
		return "", -1, fmt.Errorf("shell worker is dead")
	}

	// Send the command to the worker.
	_, err = fmt.Fprintf(sw.stdin, "%s\n", command)
	if err != nil {
		sw.alive = false
		return "", -1, fmt.Errorf("write command: %w", err)
	}

	// Read the subshell PID from the control pipe. The worker writes this
	// immediately after backgrounding the subshell, before wait().
	pidLine, err := sw.ctl.ReadString('\n')
	if err != nil {
		sw.alive = false
		return "", -1, fmt.Errorf("read subshell pid: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(pidLine))
	if err != nil {
		sw.alive = false
		return "", -1, fmt.Errorf("parse subshell pid %q: %w", pidLine, err)
	}

	// Arm timeout: kill only the subshell's process group (setsid'd, so
	// pgid == pid). The worker keeps running, reports the (signal-induced)
	// exit code via the sentinel, and is ready for the next command.
	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		syscall.Kill(-pid, syscall.SIGKILL)
	})

	var b strings.Builder
	sentinelPrefix := sw.sentinel + " "
	for sw.stdout.Scan() {
		line := sw.stdout.Text()
		if strings.HasPrefix(line, sentinelPrefix) {
			timer.Stop()
			codeStr := line[len(sentinelPrefix):]
			code, parseErr := strconv.Atoi(codeStr)
			if parseErr != nil {
				code = 2
			}
			out := b.String()
			if len(out) > 8192 {
				out = out[:8192]
			}
			if timedOut.Load() {
				return out, 2, ErrCheckTimeout
			}
			return out, code, nil
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}

	// Scanner stopped — worker died unexpectedly.
	timer.Stop()
	sw.alive = false
	if sw.cmd.ProcessState == nil {
		sw.cmd.Wait()
	}
	return "", -1, fmt.Errorf("shell exited unexpectedly")
}

// Close kills the worker's process group and waits for cleanup.
func (sw *shellWorker) Close() {
	if sw.cmd.Process != nil {
		syscall.Kill(-sw.cmd.Process.Pid, syscall.SIGKILL)
		sw.cmd.Wait()
	}
	if sw.ctlFile != nil {
		sw.ctlFile.Close()
	}
	sw.alive = false
}
