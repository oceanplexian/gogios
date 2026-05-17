//go:build linux

package main

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func run(command string) int {
	// First attempt: isolate in a new PID namespace. Requires CAP_SYS_ADMIN
	// (granted to the nagios pod via securityContext.capabilities.add).
	code, err := exec1(command, syscall.CLONE_NEWPID)
	if err == nil {
		return code
	}
	// EPERM from clone means caps are missing. Fall back to a plain exec so
	// dev environments and non-privileged containers don't break; gogios still
	// gets timeouts via pgroup-kill, it just loses the atomic-teardown
	// guarantee.
	if errors.Is(err, syscall.EPERM) {
		code, err = exec1(command, 0)
		if err == nil {
			return code
		}
	}
	return 127
}

func exec1(command string, cloneflags uintptr) (int, error) {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneflags,
		// If the shim dies, kernel SIGKILLs PID 1 of the new namespace,
		// which triggers atomic teardown of every descendant.
		Pdeathsig: syscall.SIGKILL,
	}
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	if ee, ok := err.(*exec.ExitError); ok {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok {
			if ws.Signaled() {
				return 128 + int(ws.Signal()), nil
			}
			return ws.ExitStatus(), nil
		}
		return ee.ExitCode(), nil
	}
	return 0, err
}
