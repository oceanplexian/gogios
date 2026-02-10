package checker

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestExecutorSubmitDoesNotBlock(t *testing.T) {
	// Regression test for the scheduler deadlock:
	// When more checks are submitted than the concurrency limit, Submit
	// must not block the caller. If it did, the scheduler's event loop
	// couldn't drain resultCh, preventing completed checks from freeing
	// worker slots — a classic deadlock.
	const (
		concurrency = 4
		numSubmits  = 20 // much more than concurrency
	)

	resultCh := make(chan *objects.CheckResult, concurrency)
	executor := NewExecutor(concurrency, resultCh)

	// Submit many checks — this must return promptly even though
	// concurrency << numSubmits. The old blocking Submit would deadlock
	// here because resultCh would fill up and no one would drain it.
	done := make(chan struct{})
	go func() {
		for i := 0; i < numSubmits; i++ {
			executor.Submit("host", "svc", "/usr/bin/true", 5*time.Second, 0, 0, 0)
		}
		close(done)
	}()

	// Submit should return within a second even with 20 submits and 4 slots,
	// because it doesn't block on the semaphore.
	select {
	case <-done:
		// good
	case <-time.After(3 * time.Second):
		t.Fatal("Submit blocked — possible deadlock regression")
	}

	// Now drain all results to let goroutines finish
	for i := 0; i < numSubmits; i++ {
		select {
		case <-resultCh:
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out waiting for result %d/%d", i+1, numSubmits)
		}
	}
}

func TestExecutorConcurrencyLimit(t *testing.T) {
	// Verify that no more than maxConcurrent checks run simultaneously.
	const concurrency = 4
	resultCh := make(chan *objects.CheckResult, 100)
	executor := NewExecutor(concurrency, resultCh)

	// Submit checks that sleep briefly so we can observe concurrency
	for i := 0; i < 12; i++ {
		executor.Submit("host", "svc", "sleep 0.1", 5*time.Second, 0, 0, 0)
	}

	// Give checks time to start
	time.Sleep(50 * time.Millisecond)

	peak := executor.JobsRunning()
	if peak > int64(concurrency) {
		t.Errorf("JobsRunning()=%d exceeds concurrency limit %d", peak, concurrency)
	}

	// Drain results
	for i := 0; i < 12; i++ {
		select {
		case <-resultCh:
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for result %d", i+1)
		}
	}
}

func TestExecutorDefaultConcurrency(t *testing.T) {
	resultCh := make(chan *objects.CheckResult, 1)
	executor := NewExecutor(0, resultCh)
	// 0 should default to 256 workers
	if executor.Workers() != 256 {
		t.Errorf("expected default concurrency 256, got %d", executor.Workers())
	}
}

func TestExecutorWorkerPoolProcessesAllJobs(t *testing.T) {
	// Verify the worker pool actually completes all submitted jobs
	const numJobs = 50
	resultCh := make(chan *objects.CheckResult, numJobs)
	executor := NewExecutor(8, resultCh)

	for i := 0; i < numJobs; i++ {
		executor.Submit("host", "svc", "/usr/bin/true", 5*time.Second, 0, 0, 0)
	}

	received := 0
	for received < numJobs {
		select {
		case <-resultCh:
			received++
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out: got %d/%d results", received, numJobs)
		}
	}
}

func TestExecutorOverflowDoesNotBlock(t *testing.T) {
	// When job channel is full, Submit should not block (uses overflow goroutine)
	const concurrency = 2
	resultCh := make(chan *objects.CheckResult, 1)
	executor := NewExecutor(concurrency, resultCh)

	// Fill up the job channel buffer (concurrency*4 = 8) plus workers (2)
	// by submitting jobs that sleep
	numJobs := concurrency*4 + concurrency + 10 // more than buffer + workers
	done := make(chan struct{})
	go func() {
		for i := 0; i < numJobs; i++ {
			executor.Submit("host", "svc", "sleep 0.5", 5*time.Second, 0, 0, 0)
		}
		close(done)
	}()

	select {
	case <-done:
		// good - Submit returned without blocking
	case <-time.After(3 * time.Second):
		t.Fatal("Submit blocked when job channel was full")
	}

	// Drain results
	for i := 0; i < numJobs; i++ {
		select {
		case <-resultCh:
		case <-time.After(30 * time.Second):
			t.Fatalf("timed out draining result %d/%d", i+1, numJobs)
		}
	}
}
