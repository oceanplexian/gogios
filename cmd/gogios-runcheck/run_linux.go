//go:build linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// PR_SET_PDEATHSIG; not exposed by stdlib syscall on Linux but stable since 2.1.57.
const prSetPdeathsig = 1

func run(command string) int {
	// If the bash worker (our parent) dies, we must die too so the
	// PID-namespace teardown fires and reaps every descendant. Without this,
	// a gogios crash could leak in-flight check subtrees.
	if _, _, errno := syscall.RawSyscall6(syscall.SYS_PRCTL, prSetPdeathsig, uintptr(syscall.SIGKILL), 0, 0, 0, 0); errno != 0 {
		fmt.Fprintf(os.Stderr, "gogios-runcheck: prctl(PR_SET_PDEATHSIG): %v\n", errno)
		// Non-fatal: continue, just lose the parent-death cleanup guarantee.
	}

	// Preferred path: isolate in a new PID namespace so SIGKILL on the shim
	// triggers atomic kernel teardown of all descendants. Needs CAP_SYS_ADMIN.
	code, err := exec1(command, syscall.CLONE_NEWPID)
	if err == nil {
		return code
	}
	// EPERM means the calling user/container lacks CAP_SYS_ADMIN. Run without
	// the namespace so dev environments still work — gogios still gets check
	// timeouts via pgroup-kill; it just loses the orphan-zombie guarantee.
	if errors.Is(err, syscall.EPERM) {
		if code, err = exec1(command, 0); err == nil {
			return code
		}
	}
	fmt.Fprintf(os.Stderr, "gogios-runcheck: exec failed: %v\n", err)
	return 127
}

func exec1(command string, cloneflags uintptr) (int, error) {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: cloneflags,
		Pdeathsig:  syscall.SIGKILL,
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
