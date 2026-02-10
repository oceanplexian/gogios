package checker

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func newTestConfig() *objects.Config {
	cfg := objects.DefaultConfig()
	return cfg
}

func newTestService() *objects.Service {
	host := &objects.Host{
		Name:             "testhost",
		CurrentState:     objects.HostUp,
		ActiveChecksEnabled: true,
	}
	return &objects.Service{
		Host:                host,
		Description:         "testsvc",
		CheckInterval:      5,
		RetryInterval:      1,
		MaxCheckAttempts:    3,
		ActiveChecksEnabled: true,
		CurrentState:        objects.ServiceOK,
		StateType:           objects.StateTypeHard,
		CurrentAttempt:      1,
	}
}

func TestServiceResultHandler_OKStaysOK(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService()
	h := &ServiceResultHandler{Cfg: cfg}

	cr := &objects.CheckResult{
		ReturnCode: 0,
		ExitedOK:   true,
		Output:     "OK - all good",
		StartTime:  time.Now(),
		FinishTime: time.Now(),
	}
	changed := h.HandleResult(svc, cr)
	if changed {
		t.Error("no HARD change expected for OK->OK")
	}
	if svc.CurrentState != objects.ServiceOK {
		t.Errorf("expected OK, got %d", svc.CurrentState)
	}
	if svc.StateType != objects.StateTypeHard {
		t.Error("expected HARD state")
	}
	if svc.CurrentAttempt != 1 {
		t.Errorf("expected attempt 1, got %d", svc.CurrentAttempt)
	}
}

func TestServiceResultHandler_SoftToHard(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService()
	h := &ServiceResultHandler{Cfg: cfg}
	now := time.Now()

	// First failure: OK -> SOFT CRITICAL (attempt 1)
	cr := &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "CRITICAL", StartTime: now, FinishTime: now}
	changed := h.HandleResult(svc, cr)
	if changed {
		t.Error("first failure should not be HARD change")
	}
	if svc.StateType != objects.StateTypeSoft {
		t.Error("expected SOFT after first failure")
	}
	if svc.CurrentAttempt != 1 {
		t.Errorf("expected attempt 1, got %d", svc.CurrentAttempt)
	}

	// Second failure: SOFT CRITICAL attempt 2
	cr = &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "CRITICAL", StartTime: now, FinishTime: now}
	changed = h.HandleResult(svc, cr)
	if changed {
		t.Error("second failure should not be HARD change (attempt 2 of 3)")
	}
	if svc.CurrentAttempt != 2 {
		t.Errorf("expected attempt 2, got %d", svc.CurrentAttempt)
	}

	// Third failure: SOFT -> HARD CRITICAL
	cr = &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "CRITICAL", StartTime: now, FinishTime: now}
	changed = h.HandleResult(svc, cr)
	if !changed {
		t.Error("third failure should be HARD change")
	}
	if svc.StateType != objects.StateTypeHard {
		t.Error("expected HARD after max attempts")
	}
	if svc.CurrentAttempt != 3 {
		t.Errorf("expected attempt 3, got %d", svc.CurrentAttempt)
	}
}

func TestServiceResultHandler_MaxAttempts1(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService()
	svc.MaxCheckAttempts = 1
	h := &ServiceResultHandler{Cfg: cfg}
	now := time.Now()

	cr := &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "CRITICAL", StartTime: now, FinishTime: now}
	changed := h.HandleResult(svc, cr)
	if !changed {
		t.Error("max_attempts=1 should be immediate HARD")
	}
	if svc.StateType != objects.StateTypeHard {
		t.Error("expected HARD")
	}
}

func TestServiceResultHandler_HardRecovery(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService()
	svc.CurrentState = objects.ServiceCritical
	svc.StateType = objects.StateTypeHard
	svc.CurrentAttempt = 3
	svc.MaxCheckAttempts = 3
	svc.LastHardState = objects.ServiceCritical
	h := &ServiceResultHandler{Cfg: cfg}
	now := time.Now()

	notified := false
	h.OnNotification = func(s *objects.Service, nt int) { notified = true }

	cr := &objects.CheckResult{ReturnCode: 0, ExitedOK: true, Output: "OK", StartTime: now, FinishTime: now}
	changed := h.HandleResult(svc, cr)
	if !changed {
		t.Error("HARD recovery should report change")
	}
	if svc.CurrentState != objects.ServiceOK {
		t.Error("expected OK")
	}
	if svc.StateType != objects.StateTypeHard {
		t.Error("recovery should be HARD")
	}
	if svc.CurrentAttempt != 1 {
		t.Errorf("expected attempt reset to 1, got %d", svc.CurrentAttempt)
	}
	if !notified {
		t.Error("expected notification on HARD recovery")
	}
}

func TestServiceResultHandler_HostDownMasksRetries(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService()
	svc.Host.CurrentState = objects.HostDown // host is down
	h := &ServiceResultHandler{Cfg: cfg}
	now := time.Now()

	cr := &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "CRITICAL", StartTime: now, FinishTime: now}
	h.HandleResult(svc, cr)

	if svc.StateType != objects.StateTypeHard {
		t.Error("service should go HARD immediately when host is down")
	}
	if !svc.HostProblemAtLastCheck {
		t.Error("HostProblemAtLastCheck should be set")
	}
}

func TestServiceResultHandler_SoftRecoveryNoNotification(t *testing.T) {
	cfg := newTestConfig()
	svc := newTestService()
	svc.CurrentState = objects.ServiceCritical
	svc.StateType = objects.StateTypeSoft
	svc.CurrentAttempt = 2
	svc.MaxCheckAttempts = 3
	h := &ServiceResultHandler{Cfg: cfg}
	now := time.Now()

	notified := false
	h.OnNotification = func(s *objects.Service, nt int) { notified = true }

	cr := &objects.CheckResult{ReturnCode: 0, ExitedOK: true, Output: "OK", StartTime: now, FinishTime: now}
	h.HandleResult(svc, cr)

	if svc.CurrentState != objects.ServiceOK {
		t.Error("expected OK")
	}
	if notified {
		t.Error("SOFT recovery should NOT send notification")
	}
}
