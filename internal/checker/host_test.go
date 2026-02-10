package checker

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func newTestHost() *objects.Host {
	return &objects.Host{
		Name:             "testhost",
		CheckInterval:    5,
		RetryInterval:    1,
		MaxCheckAttempts: 3,
		ActiveChecksEnabled: true,
		CurrentState:     objects.HostUp,
		StateType:        objects.StateTypeHard,
		CurrentAttempt:   1,
	}
}

func TestAdjustHostCheckAttempt_HardUp(t *testing.T) {
	h := newTestHost()
	h.CurrentAttempt = 5
	AdjustHostCheckAttempt(h)
	if h.CurrentAttempt != 1 {
		t.Errorf("expected reset to 1, got %d", h.CurrentAttempt)
	}
}

func TestAdjustHostCheckAttempt_SoftDown(t *testing.T) {
	h := newTestHost()
	h.CurrentState = objects.HostDown
	h.StateType = objects.StateTypeSoft
	h.CurrentAttempt = 1
	AdjustHostCheckAttempt(h)
	if h.CurrentAttempt != 2 {
		t.Errorf("expected increment to 2, got %d", h.CurrentAttempt)
	}
}

func TestDetermineHostReachability_NoParents(t *testing.T) {
	h := newTestHost()
	got := DetermineHostReachability(h, objects.HostDown)
	if got != objects.HostDown {
		t.Errorf("no parents: expected DOWN, got %d", got)
	}
}

func TestDetermineHostReachability_ParentUp(t *testing.T) {
	parent := newTestHost()
	parent.CurrentState = objects.HostUp

	h := newTestHost()
	h.Parents = []*objects.Host{parent}

	got := DetermineHostReachability(h, objects.HostDown)
	if got != objects.HostDown {
		t.Errorf("parent UP: expected DOWN, got %d", got)
	}
}

func TestDetermineHostReachability_AllParentsDown(t *testing.T) {
	parent1 := newTestHost()
	parent1.CurrentState = objects.HostDown
	parent2 := newTestHost()
	parent2.CurrentState = objects.HostDown

	h := newTestHost()
	h.Parents = []*objects.Host{parent1, parent2}

	got := DetermineHostReachability(h, objects.HostDown)
	if got != objects.HostUnreachable {
		t.Errorf("all parents DOWN: expected UNREACHABLE, got %d", got)
	}
}

func TestHostResultHandler_SoftToHard(t *testing.T) {
	cfg := objects.DefaultConfig()
	host := newTestHost()
	handler := &HostResultHandler{Cfg: cfg}
	now := time.Now()

	// Pre-adjust attempt (host does this BEFORE check)
	AdjustHostCheckAttempt(host)

	// First failure
	cr := &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "DOWN", StartTime: now, FinishTime: now}
	handler.HandleResult(host, cr)
	if host.StateType != objects.StateTypeSoft {
		t.Errorf("expected SOFT, got stateType=%d", host.StateType)
	}

	// Second failure (pre-adjust)
	AdjustHostCheckAttempt(host)
	cr = &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "DOWN", StartTime: now, FinishTime: now}
	handler.HandleResult(host, cr)

	// Third failure (pre-adjust)
	AdjustHostCheckAttempt(host)
	cr = &objects.CheckResult{ReturnCode: 2, ExitedOK: true, Output: "DOWN", StartTime: now, FinishTime: now}
	changed := handler.HandleResult(host, cr)
	if !changed {
		t.Error("expected HARD change on max attempts")
	}
	if host.StateType != objects.StateTypeHard {
		t.Error("expected HARD state")
	}
}

func TestHostResultHandler_PassiveImmedateHard(t *testing.T) {
	cfg := objects.DefaultConfig()
	host := newTestHost()
	handler := &HostResultHandler{Cfg: cfg}
	now := time.Now()

	cr := &objects.CheckResult{
		ReturnCode: 1, ExitedOK: true, Output: "DOWN",
		StartTime: now, FinishTime: now,
		CheckType: objects.CheckTypePassive,
	}
	changed := handler.HandleResult(host, cr)
	if !changed {
		t.Error("passive host check should be immediate HARD")
	}
	if host.StateType != objects.StateTypeHard {
		t.Error("expected HARD")
	}
}
