package checker

import (
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// ServiceResultHandler processes service check results and manages the
// SOFT/HARD state machine. This is the Go equivalent of Nagios's
// ~700-line handle_async_service_check_result().
type ServiceResultHandler struct {
	Cfg *objects.Config
	// HostLookup finds a host by name. Set by the scheduler.
	HostLookup func(name string) *objects.Host
	// ScheduleHostCheck requests an immediate host check.
	ScheduleHostCheck func(h *objects.Host, t time.Time, options int)
	// OnStateChange is called when a service state change should trigger
	// notifications/event handlers (provided by task #8).
	OnStateChange func(svc *objects.Service, oldState, newState int, hardChange bool)
	// OnNotification is called when a notification should be sent.
	OnNotification func(svc *objects.Service, notifType int)
}

// HandleResult processes a check result for a service.
// Returns true if the state changed (HARD state change).
func (h *ServiceResultHandler) HandleResult(svc *objects.Service, cr *objects.CheckResult) bool {
	now := cr.FinishTime
	if now.IsZero() {
		now = time.Now()
	}

	// Bookkeeping
	svc.IsExecuting = false
	svc.Latency = cr.Latency
	svc.ExecutionTime = cr.ExecutionTime
	svc.LastCheck = cr.StartTime
	svc.HasBeenChecked = true

	// Clear freshness flag - race condition protection:
	// if freshness triggered this check but a result arrived meanwhile, skip
	if cr.CheckOptions&objects.CheckOptionFreshnessCheck != 0 && svc.IsBeingFreshened {
		svc.IsBeingFreshened = false
	}

	// Parse output
	parsed := ParseCheckOutput(cr.Output)
	cr.Output = AugmentReturnCodeOutput(cr)
	svc.PluginOutput = parsed.ShortOutput
	svc.LongPluginOutput = parsed.LongOutput
	svc.PerfData = parsed.PerfData

	// Determine new state
	newState := GetServiceCheckReturnCode(cr, h.Cfg.ServiceCheckTimeoutState)

	// Record last time in each state
	switch newState {
	case objects.ServiceOK:
		svc.LastTimeOK = now
	case objects.ServiceWarning:
		svc.LastTimeWarning = now
	case objects.ServiceCritical:
		svc.LastTimeCritical = now
	case objects.ServiceUnknown:
		svc.LastTimeUnknown = now
	}

	// Save previous state info
	lastState := svc.CurrentState
	lastStateType := svc.StateType
	lastHardState := svc.LastHardState

	// State change detection
	stateChange := newState != lastState
	hardChange := false

	// Update current state early so notification callbacks see the correct state.
	// All state machine branches below use local lastState, not svc.CurrentState.
	svc.CurrentState = newState
	svc.LastState = lastState

	// Check host state when service has a problem
	hostProblem := false
	if newState != objects.ServiceOK && svc.Host != nil {
		host := svc.Host
		if host.CurrentState != objects.HostUp {
			hostProblem = true
		}
	}

	// --- SOFT/HARD state machine ---

	if newState == objects.ServiceOK {
		// Recovery or continued OK
		if lastState != objects.ServiceOK {
			// Recovery from a problem - clear acknowledgement but preserve
			// NotifiedOn until after the recovery notification is sent,
			// because the viability check uses it to verify a prior PROBLEM.
			svc.ProblemAcknowledged = false
			svc.AckType = objects.AckNone
			svc.LastNotification = time.Time{}
			svc.NextNotification = time.Time{}
			svc.NoMoreNotifications = false
			svc.FirstProblemTime = time.Time{}
			if lastStateType == objects.StateTypeHard {
				// HARD recovery - send recovery notification
				hardChange = true
				svc.StateType = objects.StateTypeHard
				svc.CurrentAttempt = 1
				if h.OnNotification != nil {
					h.OnNotification(svc, objects.NotificationNormal)
				}
			} else {
				// SOFT recovery - no notification
				svc.StateType = objects.StateTypeHard
				svc.CurrentAttempt = 1
			}
			// Now safe to clear notification tracking state
			svc.CurrentNotificationNumber = 0
			svc.NotifiedOn = 0
		} else {
			// Continued OK
			svc.StateType = objects.StateTypeHard
			svc.CurrentAttempt = 1
		}
		svc.HostProblemAtLastCheck = false
	} else if hostProblem {
		// Host is DOWN - force service to HARD state immediately
		svc.StateType = objects.StateTypeHard
		svc.CurrentAttempt = svc.MaxCheckAttempts
		svc.HostProblemAtLastCheck = true
		// No notifications for service when host is down
	} else if svc.MaxCheckAttempts <= 1 {
		// max_check_attempts=1 means immediate HARD
		svc.StateType = objects.StateTypeHard
		svc.CurrentAttempt = 1
		if stateChange || lastStateType == objects.StateTypeSoft {
			hardChange = true
			if h.OnNotification != nil {
				h.OnNotification(svc, objects.NotificationNormal)
			}
		}
		svc.HostProblemAtLastCheck = false
	} else if lastState == objects.ServiceOK {
		// First failure - transition to SOFT
		svc.StateType = objects.StateTypeSoft
		svc.CurrentAttempt = 1
		svc.HostProblemAtLastCheck = false
	} else if svc.StateType == objects.StateTypeSoft {
		// Already in SOFT state
		if svc.CurrentAttempt < svc.MaxCheckAttempts {
			svc.CurrentAttempt++
		}
		if svc.CurrentAttempt >= svc.MaxCheckAttempts {
			// Transition to HARD
			svc.StateType = objects.StateTypeHard
			hardChange = true
			if h.OnNotification != nil {
				h.OnNotification(svc, objects.NotificationNormal)
			}
		}
		svc.HostProblemAtLastCheck = false
	} else {
		// HARD non-OK state, continued problem
		svc.CurrentAttempt = svc.MaxCheckAttempts
		svc.HostProblemAtLastCheck = false
	}

	// Non-sticky ack: clear on any state change (including non-OK to different non-OK)
	if stateChange && svc.ProblemAcknowledged && svc.AckType == objects.AckNormal {
		svc.ProblemAcknowledged = false
		svc.AckType = objects.AckNone
	}

	if hardChange || (svc.StateType == objects.StateTypeHard && lastStateType == objects.StateTypeHard && stateChange) {
		svc.LastHardState = newState
		svc.LastHardStateChange = now
	}

	if stateChange {
		svc.LastStateChange = now
	}

	// Flap detection
	if svc.FlapDetectionEnabled {
		if ShouldRecordServiceFlapState(newState, svc.StateType, lastState, lastHardState) {
			UpdateFlapHistory(&svc.StateHistory, &svc.StateHistoryIndex, &svc.PercentStateChange, newState)
			newFlapping, flapChanged := CheckFlapping(svc.IsFlapping, svc.PercentStateChange,
				svc.LowFlapThreshold, svc.HighFlapThreshold)
			if flapChanged {
				svc.IsFlapping = newFlapping
			}
		}
	}

	// Determine next check interval
	if newState == objects.ServiceOK || svc.StateType == objects.StateTypeHard || hostProblem {
		svc.NextCheck = now.Add(h.normalCheckWindow(svc))
	} else {
		svc.NextCheck = now.Add(h.retryCheckWindow(svc))
	}

	_ = lastHardState // suppress unused if no callbacks

	// Notify on state change
	if h.OnStateChange != nil && (stateChange || hardChange) {
		h.OnStateChange(svc, lastState, newState, hardChange)
	}

	return hardChange
}

func (h *ServiceResultHandler) normalCheckWindow(svc *objects.Service) time.Duration {
	il := h.Cfg.IntervalLength
	if il <= 0 {
		il = 60
	}
	return time.Duration(svc.CheckInterval*float64(il)) * time.Second
}

func (h *ServiceResultHandler) retryCheckWindow(svc *objects.Service) time.Duration {
	il := h.Cfg.IntervalLength
	if il <= 0 {
		il = 60
	}
	return time.Duration(svc.RetryInterval*float64(il)) * time.Second
}
