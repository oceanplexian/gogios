package checker

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func testSentinel() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func TestShellWorkerBasic(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	output, code, err := sw.Run("/usr/bin/true", 5*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if output != "" {
		t.Errorf("expected empty output, got %q", output)
	}
}

func TestShellWorkerExitCode(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	_, code, err := sw.Run("exit 2", 5*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 2 {
		t.Errorf("expected exit 2, got %d", code)
	}
}

func TestShellWorkerOutput(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	output, code, err := sw.Run("echo hello", 5*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if output != "hello" {
		t.Errorf("expected %q, got %q", "hello", output)
	}
}

func TestShellWorkerMultilineOutput(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	output, code, err := sw.Run("echo line1; echo line2; echo line3", 5*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	expected := "line1\nline2\nline3"
	if output != expected {
		t.Errorf("expected %q, got %q", expected, output)
	}
}

func TestShellWorkerTimeout(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	_, _, err = sw.Run("sleep 60", 500*time.Millisecond)
	if !errors.Is(err, ErrCheckTimeout) {
		t.Fatalf("expected ErrCheckTimeout, got %v", err)
	}
	if !sw.alive {
		t.Fatal("worker must stay alive after a check-level timeout")
	}

	// Worker should be reusable for the next command.
	output, code, err := sw.Run("echo recovered", 5*time.Second)
	if err != nil {
		t.Fatalf("Run after timeout: %v", err)
	}
	if code != 0 || output != "recovered" {
		t.Fatalf("post-timeout result: code=%d output=%q", code, output)
	}
}

func TestShellWorkerTimeoutKillsSubshellProcessGroup(t *testing.T) {
	// A check that backgrounds a long-lived grandchild must have the
	// grandchild killed too — kill -pgid is what makes that happen.
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	marker := fmt.Sprintf("/tmp/gogios-pgrp-test-%d", time.Now().UnixNano())
	// Spawn a sleeper that touches marker on exit. If the pgrp kill works,
	// the sleeper dies before marker is written.
	cmd := fmt.Sprintf("(sleep 30; touch %s) & echo started; wait", marker)
	_, _, err = sw.Run(cmd, 500*time.Millisecond)
	if !errors.Is(err, ErrCheckTimeout) {
		t.Fatalf("expected ErrCheckTimeout, got %v", err)
	}
	// Give the kernel a moment to deliver SIGKILL.
	time.Sleep(200 * time.Millisecond)
	if _, statErr := os.Stat(marker); statErr == nil {
		os.Remove(marker)
		t.Fatal("grandchild survived timeout — process group kill failed")
	}
}

func TestRequiresSharedPIDNamespace(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "check_fping", command: "/usr/lib/nagios/plugins/check_fping -H 10.0.0.1", want: true},
		{name: "wrapped ping", command: "/usr/bin/timeout 5 /usr/bin/ping -c 1 10.0.0.1", want: true},
		{name: "check_icmp", command: "sudo -n /opt/plugins/check_icmp -H 10.0.0.1", want: true},
		{name: "http", command: "/usr/lib/nagios/plugins/check_http -H example.com", want: false},
		{name: "shell", command: "/bin/sh -c 'echo ok'", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requiresSharedPIDNamespace(tt.command); got != tt.want {
				t.Fatalf("requiresSharedPIDNamespace(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestShellWorkersRawICMPCommandsUseDistinctPIDs(t *testing.T) {
	workers := make([]*shellWorker, 2)
	for i := range workers {
		var err error
		workers[i], err = newShellWorker(testSentinel())
		if err != nil {
			t.Fatalf("newShellWorker %d: %v", i, err)
		}
		defer workers[i].Close()
	}

	type result struct {
		output string
		code   int
		err    error
	}
	results := make(chan result, len(workers))
	start := make(chan struct{})
	for _, sw := range workers {
		go func(sw *shellWorker) {
			<-start
			// /usr/bin/fping is argv[0] for the inner shell and makes this a
			// raw-ICMP-classified command without requiring fping to be installed.
			output, code, err := sw.Run("/bin/sh -c 'sleep 0.1; echo $$' /usr/bin/fping", 5*time.Second)
			results <- result{output: output, code: code, err: err}
		}(sw)
	}
	close(start)

	pids := make(map[int]struct{}, len(workers))
	for range workers {
		res := <-results
		if res.err != nil || res.code != 0 {
			t.Fatalf("raw ICMP command failed: code=%d output=%q err=%v", res.code, res.output, res.err)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(res.output))
		if err != nil {
			t.Fatalf("parse PID %q: %v", res.output, err)
		}
		pids[pid] = struct{}{}
	}
	if len(pids) != len(workers) {
		t.Fatalf("raw ICMP checks reused PIDs across workers: %v", pids)
	}
}

func TestShellWorkerSharedPIDTimeoutKillsProcessGroup(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	marker := fmt.Sprintf("/tmp/gogios-shared-pgrp-test-%d", time.Now().UnixNano())
	cmd := fmt.Sprintf("/bin/sh -c '(sleep 30; touch %s) & echo started; wait' /usr/bin/fping", marker)
	_, _, err = sw.Run(cmd, 500*time.Millisecond)
	if !errors.Is(err, ErrCheckTimeout) {
		t.Fatalf("expected ErrCheckTimeout, got %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if _, statErr := os.Stat(marker); statErr == nil {
		os.Remove(marker)
		t.Fatal("shared-PID grandchild survived timeout — process group kill failed")
	}
}

func TestShellWorkerCrashRecovery(t *testing.T) {
	sentinel := testSentinel()
	sw, err := newShellWorker(sentinel)
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}

	// Run a command successfully first
	output, code, err := sw.Run("echo before", 5*time.Second)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if output != "before" || code != 0 {
		t.Fatalf("unexpected first result: %q %d", output, code)
	}

	// Kill the shell externally
	syscall.Kill(-sw.cmd.Process.Pid, syscall.SIGKILL)
	time.Sleep(100 * time.Millisecond)

	// Next command should fail
	_, _, err = sw.Run("echo after", 5*time.Second)
	if err == nil {
		t.Fatal("expected error after kill, got nil")
	}
	if sw.alive {
		t.Fatal("shell should be marked dead after crash")
	}
	sw.Close()
}

func TestShellWorkerMultipleCommands(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	// Run many commands sequentially through the same shell
	for i := 0; i < 100; i++ {
		output, code, err := sw.Run("echo ok", 5*time.Second)
		if err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
		if code != 0 || output != "ok" {
			t.Fatalf("Run %d: code=%d output=%q", i, code, output)
		}
	}
}

func TestShellWorkerSentinelInOutput(t *testing.T) {
	sentinel := testSentinel()
	sw, err := newShellWorker(sentinel)
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	// Command that outputs partial sentinel — should not confuse parser
	// because actual sentinel line is "sentinel <int>" with exact prefix match
	cmd := "echo " + sentinel
	output, code, err := sw.Run(cmd, 5*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	// The sentinel without a trailing space+digit should be captured as output
	if !strings.Contains(output, sentinel) {
		t.Errorf("expected output to contain sentinel, got %q", output)
	}
}

func TestShellWorkerStderr(t *testing.T) {
	sw, err := newShellWorker(testSentinel())
	if err != nil {
		t.Fatalf("newShellWorker: %v", err)
	}
	defer sw.Close()

	// stderr should be merged into stdout via 2>&1 in the shell script
	output, code, err := sw.Run("echo err >&2", 5*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if output != "err" {
		t.Errorf("expected stderr captured as %q, got %q", "err", output)
	}
}
