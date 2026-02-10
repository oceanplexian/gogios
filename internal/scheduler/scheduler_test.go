package scheduler

import (
	"container/heap"
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestEventQueue_Ordering(t *testing.T) {
	eq := &EventQueue{}
	heap.Init(eq)

	now := time.Now()
	heap.Push(eq, &Event{Type: EventServiceCheck, RunTime: now.Add(3 * time.Second)})
	heap.Push(eq, &Event{Type: EventHostCheck, RunTime: now.Add(1 * time.Second)})
	heap.Push(eq, &Event{Type: EventStatusSave, RunTime: now.Add(2 * time.Second)})

	e1 := heap.Pop(eq).(*Event)
	if e1.Type != EventHostCheck {
		t.Errorf("expected host check first, got type %d", e1.Type)
	}
	e2 := heap.Pop(eq).(*Event)
	if e2.Type != EventStatusSave {
		t.Errorf("expected status save second, got type %d", e2.Type)
	}
	e3 := heap.Pop(eq).(*Event)
	if e3.Type != EventServiceCheck {
		t.Errorf("expected service check third, got type %d", e3.Type)
	}
}

func TestEventQueue_PriorityAtSameTime(t *testing.T) {
	eq := &EventQueue{}
	heap.Init(eq)

	now := time.Now()
	heap.Push(eq, &Event{Type: EventServiceCheck, RunTime: now, Priority: 0})
	heap.Push(eq, &Event{Type: EventHostCheck, RunTime: now, Priority: 1})

	e := heap.Pop(eq).(*Event)
	if e.Type != EventHostCheck {
		t.Error("higher priority should fire first at same time")
	}
}

func TestInitTimingLoop_SpreadChecks(t *testing.T) {
	cfg := objects.DefaultConfig()
	cfg.ServiceInterCheckDelayMethod = ICDSmart
	cfg.HostInterCheckDelayMethod = ICDSmart

	host := &objects.Host{
		Name:                "h1",
		CheckInterval:       5,
		ActiveChecksEnabled: true,
		MaxCheckAttempts:    3,
	}
	svcs := make([]*objects.Service, 10)
	for i := range svcs {
		svcs[i] = &objects.Service{
			Host:                host,
			Description:         "svc" + string(rune('0'+i)),
			CheckInterval:       5,
			RetryInterval:       1,
			ActiveChecksEnabled: true,
			MaxCheckAttempts:    3,
		}
	}

	now := time.Now()
	events, params := InitTimingLoop(cfg, svcs, []*objects.Host{host}, now)

	if params.TotalScheduledSvcs != 10 {
		t.Errorf("expected 10 scheduled svcs, got %d", params.TotalScheduledSvcs)
	}
	if params.TotalScheduledHosts != 1 {
		t.Errorf("expected 1 scheduled host, got %d", params.TotalScheduledHosts)
	}
	if len(events) != 11 { // 10 svcs + 1 host
		t.Errorf("expected 11 events, got %d", len(events))
	}

	// All events should be scheduled after now
	for i, e := range events {
		if e.RunTime.Before(now) {
			t.Errorf("event %d scheduled before now", i)
		}
	}

	// Check that not all service checks fire at the same time
	firstSvc := events[0].RunTime
	allSame := true
	for _, e := range events[:10] {
		if !e.RunTime.Equal(firstSvc) {
			allSame = false
			break
		}
	}
	if allSame && params.ServiceICD > 0 {
		t.Error("service checks should be spread over time")
	}
}

func TestScheduleServiceCheck_Deconfliction(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-time.Second)
	later := now.Add(time.Second)

	// New forced replaces existing non-forced
	existing := &Event{RunTime: earlier, CheckOptions: 0}
	e, replace := ScheduleServiceCheck(existing, later, objects.CheckOptionForceExecution)
	if !replace {
		t.Error("forced should replace non-forced")
	}
	if e == nil {
		t.Fatal("expected new event")
	}

	// Existing forced keeps over new non-forced
	existing = &Event{RunTime: later, CheckOptions: objects.CheckOptionForceExecution}
	_, replace = ScheduleServiceCheck(existing, earlier, 0)
	if replace {
		t.Error("non-forced should not replace forced")
	}

	// Both non-forced: earlier wins
	existing = &Event{RunTime: later, CheckOptions: 0}
	e, replace = ScheduleServiceCheck(existing, earlier, 0)
	if !replace {
		t.Error("earlier non-forced should replace later")
	}
	if e == nil {
		t.Fatal("expected new event")
	}
}

func TestRecurringEvents(t *testing.T) {
	now := time.Now()
	events := RecurringEvents(now, 10, 60, 60, 60, 60, 60, 30, true, true, false)
	// Should have: reaper, orphan, sfreshness, hfreshness, status, retention
	// NOT auto_reschedule (disabled)
	if len(events) != 6 {
		t.Errorf("expected 6 recurring events, got %d", len(events))
	}
	for _, e := range events {
		if !e.Recurring {
			t.Errorf("event type %d should be recurring", e.Type)
		}
	}
}
