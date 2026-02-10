package checker

import (
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// HostResultHandler processes host check results and manages state.
type HostResultHandler struct {
	Cfg *objects.Config
	// OnStateChange is called on host state changes.
	OnStateChange func(h *objects.Host, oldState, newState int, hardChange bool)
	// OnNotification is called when a notification should be sent.
	OnNotification func(h *objects.Host, notifType int)
	// ScheduleHostCheck requests a host check (for parent/child propagation).
	ScheduleHostCheck func(h *objects.Host, t time.Time, options int)
}

// AdjustHostCheckAttempt is called BEFORE a host check runs (unlike services).
func AdjustHostCheckAttempt(host *objects.Host) {
	if host.StateType == objects.StateTypeHard && host.CurrentState == objects.HostUp {
		host.CurrentAttempt = 1
	} else if host.StateType == objects.StateTypeSoft && host.CurrentState == objects.HostUp {
		// Active check: reset to 1
		host.CurrentAttempt = 1
	} else {
		if host.CurrentAttempt < host.MaxCheckAttempts {
			host.CurrentAttempt++
		}
	}
}

// HandleResult processes a host check result.
// Returns true if a HARD state change occurred.
func (h *HostResultHandler) HandleResult(host *objects.Host, cr *objects.CheckResult) bool {
	now := cr.FinishTime
	if now.IsZero() {
		now = time.Now()
	}

	// Bookkeeping
	host.IsExecuting = false
	host.Latency = cr.Latency
	host.ExecutionTime = cr.ExecutionTime
	host.LastCheck = cr.StartTime
	host.HasBeenChecked = true

	if cr.CheckOptions&objects.CheckOptionFreshnessCheck != 0 && host.IsBeingFreshened {
		host.IsBeingFreshened = false
	}

	// Parse output
	parsed := ParseCheckOutput(cr.Output)
	cr.Output = AugmentReturnCodeOutput(cr)
	host.PluginOutput = parsed.ShortOutput
	host.LongPluginOutput = parsed.LongOutput
	host.PerfData = parsed.PerfData

	// Determine new state
	var newState int
	if cr.CheckType == objects.CheckTypePassive && !h.Cfg.TranslatePassiveHostChecks {
		newState = GetPassiveHostCheckReturnCode(cr.ReturnCode)
	} else {
		newState = GetHostCheckReturnCode(cr, h.Cfg.UseAggressiveHostChecking)
	}

	// If host appears DOWN, check reachability via parents
	if newState != objects.HostUp && cr.CheckType == objects.CheckTypeActive {
		newState = DetermineHostReachability(host, newState)
	}
	if newState != objects.HostUp && cr.CheckType == objects.CheckTypePassive && h.Cfg.TranslatePassiveHostChecks {
		newState = DetermineHostReachability(host, newState)
	}

	// Record last time in each state
	switch newState {
	case objects.HostUp:
		host.LastTimeUp = now
	case objects.HostDown:
		host.LastTimeDown = now
	case objects.HostUnreachable:
		host.LastTimeUnreachable = now
	}

	lastState := host.CurrentState
	lastStateType := host.StateType
	stateChange := newState != lastState
	hardChange := false

	// Update current state early so notification callbacks see the correct state.
	// All state machine branches below use local lastState, not host.CurrentState.
	host.CurrentState = newState
	host.LastState = lastState

	// --- SOFT/HARD state machine ---

	if newState == objects.HostUp {
		// Recovery or continued UP
		if lastState != objects.HostUp {
			// Clear acknowledgement but preserve NotifiedOn until after
			// the recovery notification is sent (viability check needs it).
			host.ProblemAcknowledged = false
			host.AckType = objects.AckNone
			host.LastNotification = time.Time{}
			host.NextNotification = time.Time{}
			host.NoMoreNotifications = false
			host.FirstProblemTime = time.Time{}
			if lastStateType == objects.StateTypeHard {
				hardChange = true
				host.StateType = objects.StateTypeHard
				host.CurrentAttempt = 1
				if h.OnNotification != nil {
					h.OnNotification(host, objects.NotificationNormal)
				}
			} else {
				host.StateType = objects.StateTypeHard
				host.CurrentAttempt = 1
			}
			// Now safe to clear notification tracking state
			host.CurrentNotificationNumber = 0
			host.NotifiedOn = 0
		} else {
			host.StateType = objects.StateTypeHard
			host.CurrentAttempt = 1
		}
	} else if host.MaxCheckAttempts <= 1 {
		// Immediate HARD
		host.StateType = objects.StateTypeHard
		host.CurrentAttempt = 1
		if stateChange || lastStateType == objects.StateTypeSoft {
			hardChange = true
			if h.OnNotification != nil {
				h.OnNotification(host, objects.NotificationNormal)
			}
		}
	} else if lastState == objects.HostUp && cr.CheckType == objects.CheckTypeActive {
		// Active check: first failure -> SOFT
		host.StateType = objects.StateTypeSoft
	} else if lastState == objects.HostUp && cr.CheckType == objects.CheckTypePassive {
		// Passive checks default to immediate HARD
		host.StateType = objects.StateTypeHard
		host.CurrentAttempt = host.MaxCheckAttempts
		hardChange = true
		if h.OnNotification != nil {
			h.OnNotification(host, objects.NotificationNormal)
		}
	} else if host.StateType == objects.StateTypeSoft {
		if host.CurrentAttempt >= host.MaxCheckAttempts {
			host.StateType = objects.StateTypeHard
			hardChange = true
			if h.OnNotification != nil {
				h.OnNotification(host, objects.NotificationNormal)
			}
		}
	} else {
		// Continued HARD non-UP
		host.CurrentAttempt = host.MaxCheckAttempts
	}

	// Non-sticky ack: clear on any state change
	if stateChange && host.ProblemAcknowledged && host.AckType == objects.AckNormal {
		host.ProblemAcknowledged = false
		host.AckType = objects.AckNone
	}

	if hardChange || (host.StateType == objects.StateTypeHard && lastStateType == objects.StateTypeHard && stateChange) {
		host.LastHardState = newState
		host.LastHardStateChange = now
	}

	if stateChange {
		host.LastStateChange = now
	}

	// Flap detection
	if host.FlapDetectionEnabled {
		UpdateFlapHistory(&host.StateHistory, &host.StateHistoryIndex, &host.PercentStateChange, newState)
		newFlapping, flapChanged := CheckFlapping(host.IsFlapping, host.PercentStateChange,
			host.LowFlapThreshold, host.HighFlapThreshold)
		if flapChanged {
			host.IsFlapping = newFlapping
		}
	}

	// Calculate next check time
	if newState == objects.HostUp || host.StateType == objects.StateTypeHard {
		host.NextCheck = now.Add(h.normalCheckWindow(host))
	} else {
		host.NextCheck = now.Add(h.retryCheckWindow(host))
	}

	// Propagate checks to parent/child hosts on state changes
	if stateChange && h.ScheduleHostCheck != nil {
		h.propagateChecks(host, lastState, newState, now)
	}

	if h.OnStateChange != nil && (stateChange || hardChange) {
		h.OnStateChange(host, lastState, newState, hardChange)
	}

	return hardChange
}

// DetermineHostReachability checks whether a non-UP host is DOWN or UNREACHABLE
// based on parent host states.
func DetermineHostReachability(host *objects.Host, currentState int) int {
	if currentState == objects.HostUp {
		return objects.HostUp
	}
	if len(host.Parents) == 0 {
		return objects.HostDown // top-level hosts are always DOWN
	}
	for _, parent := range host.Parents {
		if parent.CurrentState == objects.HostUp {
			return objects.HostDown // at least one parent is UP, so we are DOWN
		}
	}
	return objects.HostUnreachable // all parents non-UP
}

func (h *HostResultHandler) propagateChecks(host *objects.Host, oldState, newState int, now time.Time) {
	if newState != objects.HostUp && oldState == objects.HostUp {
		// Host went DOWN - check parents and children
		for _, parent := range host.Parents {
			if parent.CurrentState == objects.HostUp {
				h.ScheduleHostCheck(parent, now, objects.CheckOptionDependencyCheck)
			}
		}
		for _, child := range host.Children {
			if child.CurrentState != objects.HostUnreachable {
				h.ScheduleHostCheck(child, now, objects.CheckOptionDependencyCheck)
			}
		}
	} else if newState == objects.HostUp && oldState != objects.HostUp {
		// Host recovered - check parents and children that are non-UP
		for _, parent := range host.Parents {
			if parent.CurrentState != objects.HostUp {
				h.ScheduleHostCheck(parent, now, objects.CheckOptionDependencyCheck)
			}
		}
		for _, child := range host.Children {
			if child.CurrentState != objects.HostUp {
				h.ScheduleHostCheck(child, now, objects.CheckOptionDependencyCheck)
			}
		}
	}
}

func (h *HostResultHandler) normalCheckWindow(host *objects.Host) time.Duration {
	il := h.Cfg.IntervalLength
	if il <= 0 {
		il = 60
	}
	return time.Duration(host.CheckInterval*float64(il)) * time.Second
}

func (h *HostResultHandler) retryCheckWindow(host *objects.Host) time.Duration {
	il := h.Cfg.IntervalLength
	if il <= 0 {
		il = 60
	}
	return time.Duration(host.RetryInterval*float64(il)) * time.Second
}
