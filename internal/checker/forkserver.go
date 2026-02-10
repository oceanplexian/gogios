package checker

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// shellScript is the POSIX shell loop that each persistent shell worker runs.
// It reads one command per line from stdin, evaluates it with stdout/stderr
// merged, then prints a sentinel line with the exit code.
// </dev/null prevents child commands from consuming the shell's stdin.
const shellScript = `s="$1"; while IFS= read -r c; do (eval "$c") </dev/null 2>&1; printf '%s %d\n' "$s" $?; done`

// shellWorker manages a single persistent /bin/sh process that executes
// commands via pipe, avoiding fork() from the large Go parent process.
type shellWorker struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Scanner
	sentinel string
	alive    bool
}

// newShellWorker starts a persistent /bin/sh process running the read-eval loop.
func newShellWorker(sentinel string) (*shellWorker, error) {
	cmd := exec.Command("/bin/sh", "-c", shellScript, "--", sentinel)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return nil, fmt.Errorf("start shell: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB lines

	return &shellWorker{
		cmd:      cmd,
		stdin:    stdin,
		stdout:   scanner,
		sentinel: sentinel,
		alive:    true,
	}, nil
}

// Run sends a command to the persistent shell and reads output until the
// sentinel line. Returns the captured output, exit code, and any error.
// On timeout, the entire process group is killed and the shell is marked dead.
func (sw *shellWorker) Run(command string, timeout time.Duration) (output string, exitCode int, err error) {
	if !sw.alive {
		return "", -1, fmt.Errorf("shell worker is dead")
	}

	// Write command to shell's stdin
	_, err = fmt.Fprintf(sw.stdin, "%s\n", command)
	if err != nil {
		sw.alive = false
		return "", -1, fmt.Errorf("write command: %w", err)
	}

	// Read output lines until we see the sentinel
	var b strings.Builder
	timer := time.AfterFunc(timeout, func() {
		// Kill entire process group on timeout
		if sw.cmd.Process != nil {
			syscall.Kill(-sw.cmd.Process.Pid, syscall.SIGKILL)
		}
	})

	sentinelPrefix := sw.sentinel + " "
	for sw.stdout.Scan() {
		line := sw.stdout.Text()
		if strings.HasPrefix(line, sentinelPrefix) {
			timer.Stop()
			// Parse exit code from sentinel line
			codeStr := line[len(sentinelPrefix):]
			code, parseErr := strconv.Atoi(codeStr)
			if parseErr != nil {
				code = 2
			}
			out := b.String()
			if len(out) > 8192 {
				out = out[:8192]
			}
			return out, code, nil
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}

	// Scanner stopped — shell died or was killed
	timer.Stop()
	sw.alive = false

	// Check if it was our timeout
	if sw.cmd.ProcessState == nil {
		// Process may still be running after SIGKILL; wait for it
		sw.cmd.Wait()
	}

	return "", -1, fmt.Errorf("shell exited unexpectedly")
}

// Close kills the shell process group and waits for cleanup.
func (sw *shellWorker) Close() {
	if sw.cmd.Process != nil {
		syscall.Kill(-sw.cmd.Process.Pid, syscall.SIGKILL)
		sw.cmd.Wait()
	}
	sw.alive = false
}
