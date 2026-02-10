package scheduler

import "time"

// Event types matching Nagios 4.1.1 constants.
const (
	EventServiceCheck       = 0
	EventCommandCheck       = 1
	EventLogRotation        = 2
	EventProgramShutdown    = 3
	EventProgramRestart     = 4
	EventCheckReaper        = 5
	EventOrphanCheck        = 6
	EventRetentionSave      = 7
	EventStatusSave         = 8
	EventScheduledDowntime  = 9
	EventSFreshnessCheck    = 10
	EventExpireDowntime     = 11
	EventHostCheck          = 12
	EventHFreshnessCheck    = 13
	EventRescheduleChecks   = 14
	EventExpireComment      = 15
	EventCheckProgramUpdate = 16
	EventSleep              = 98
	EventUserFunction       = 99
)

// Event represents a scheduled event in the priority queue.
type Event struct {
	Type      int
	RunTime   time.Time
	Recurring bool
	Interval  time.Duration
	Priority  int // higher priority fires first at same time

	// For check events: pointer to host/service name
	HostName           string
	ServiceDescription string

	// For check events: forced or not
	CheckOptions int

	// Index in heap (managed by container/heap)
	index int
}

// EventQueue implements container/heap.Interface as a min-heap sorted by RunTime.
type EventQueue []*Event

func (eq EventQueue) Len() int { return len(eq) }

func (eq EventQueue) Less(i, j int) bool {
	if eq[i].RunTime.Equal(eq[j].RunTime) {
		return eq[i].Priority > eq[j].Priority
	}
	return eq[i].RunTime.Before(eq[j].RunTime)
}

func (eq EventQueue) Swap(i, j int) {
	eq[i], eq[j] = eq[j], eq[i]
	eq[i].index = i
	eq[j].index = j
}

func (eq *EventQueue) Push(x interface{}) {
	e := x.(*Event)
	e.index = len(*eq)
	*eq = append(*eq, e)
}

func (eq *EventQueue) Pop() interface{} {
	old := *eq
	n := len(old)
	e := old[n-1]
	old[n-1] = nil
	e.index = -1
	*eq = old[:n-1]
	return e
}

// RecurringEvents returns the standard set of recurring system events.
func RecurringEvents(now time.Time, reaperInterval, orphanInterval, sfreshnessInterval, hfreshnessInterval, statusInterval, retentionMinutes, autoRescheduleInterval int, checkServiceFreshness, checkHostFreshness, autoRescheduleEnabled bool) []*Event {
	var events []*Event

	if reaperInterval > 0 {
		events = append(events, &Event{
			Type:      EventCheckReaper,
			RunTime:   now.Add(time.Duration(reaperInterval) * time.Second),
			Recurring: true,
			Interval:  time.Duration(reaperInterval) * time.Second,
		})
	}

	if orphanInterval > 0 {
		events = append(events, &Event{
			Type:      EventOrphanCheck,
			RunTime:   now.Add(time.Duration(orphanInterval) * time.Second),
			Recurring: true,
			Interval:  time.Duration(orphanInterval) * time.Second,
		})
	}

	if checkServiceFreshness && sfreshnessInterval > 0 {
		events = append(events, &Event{
			Type:      EventSFreshnessCheck,
			RunTime:   now.Add(time.Duration(sfreshnessInterval) * time.Second),
			Recurring: true,
			Interval:  time.Duration(sfreshnessInterval) * time.Second,
		})
	}

	if checkHostFreshness && hfreshnessInterval > 0 {
		events = append(events, &Event{
			Type:      EventHFreshnessCheck,
			RunTime:   now.Add(time.Duration(hfreshnessInterval) * time.Second),
			Recurring: true,
			Interval:  time.Duration(hfreshnessInterval) * time.Second,
		})
	}

	if statusInterval > 0 {
		events = append(events, &Event{
			Type:      EventStatusSave,
			RunTime:   now.Add(time.Duration(statusInterval) * time.Second),
			Recurring: true,
			Interval:  time.Duration(statusInterval) * time.Second,
		})
	}

	if retentionMinutes > 0 {
		events = append(events, &Event{
			Type:      EventRetentionSave,
			RunTime:   now.Add(time.Duration(retentionMinutes) * time.Minute),
			Recurring: true,
			Interval:  time.Duration(retentionMinutes) * time.Minute,
		})
	}

	if autoRescheduleEnabled && autoRescheduleInterval > 0 {
		events = append(events, &Event{
			Type:      EventRescheduleChecks,
			RunTime:   now.Add(time.Duration(autoRescheduleInterval) * time.Second),
			Recurring: true,
			Interval:  time.Duration(autoRescheduleInterval) * time.Second,
		})
	}

	return events
}
