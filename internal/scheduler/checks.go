package scheduler

import (
	"math"
	"math/rand"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// ICD methods
const (
	ICDNone  = 0
	ICDDumb  = 1
	ICDSmart = 2
	ICDUser  = 3
)

// ILF methods
const (
	ILFUser  = 0
	ILFSmart = 2
)

// NUDGE constants for overloaded check rescheduling.
const (
	NudgeMin = 5
	NudgeMax = 17
)

// SchedulingParams holds computed scheduling parameters.
type SchedulingParams struct {
	ServiceICD          float64
	HostICD             float64
	InterleaveFactor    int
	TotalScheduledSvcs  int
	TotalScheduledHosts int
}

// CalculateSchedulingParams computes inter-check delay and interleave factor.
func CalculateSchedulingParams(cfg *objects.Config, services []*objects.Service, hosts []*objects.Host) SchedulingParams {
	var p SchedulingParams

	// Count scheduled services and total interval
	var totalSvcInterval float64
	for _, svc := range services {
		if svc.CheckInterval <= 0 || !svc.ActiveChecksEnabled {
			svc.ShouldBeScheduled = false
			continue
		}
		svc.ShouldBeScheduled = true
		p.TotalScheduledSvcs++
		totalSvcInterval += svc.CheckInterval
	}

	// Count scheduled hosts
	var totalHostInterval float64
	for _, h := range hosts {
		if h.CheckInterval <= 0 || !h.ActiveChecksEnabled {
			h.ShouldBeScheduled = false
			continue
		}
		h.ShouldBeScheduled = true
		p.TotalScheduledHosts++
		totalHostInterval += h.CheckInterval
	}

	// Service ICD
	switch cfg.ServiceInterCheckDelayMethod {
	case ICDNone:
		p.ServiceICD = 0
	case ICDDumb:
		p.ServiceICD = 1.0
	case ICDSmart:
		if p.TotalScheduledSvcs > 0 {
			avgInterval := totalSvcInterval / float64(p.TotalScheduledSvcs)
			p.ServiceICD = avgInterval / float64(p.TotalScheduledSvcs)
			maxDelay := float64(cfg.MaxServiceCheckSpread*60) / float64(p.TotalScheduledSvcs)
			if p.ServiceICD > maxDelay {
				p.ServiceICD = maxDelay
			}
		}
	case ICDUser:
		p.ServiceICD = cfg.ServiceInterCheckDelay
	}

	// Host ICD
	switch cfg.HostInterCheckDelayMethod {
	case ICDNone:
		p.HostICD = 0
	case ICDDumb:
		p.HostICD = 1.0
	case ICDSmart:
		if p.TotalScheduledHosts > 0 {
			avgInterval := totalHostInterval / float64(p.TotalScheduledHosts)
			p.HostICD = avgInterval / float64(p.TotalScheduledHosts)
			maxDelay := float64(cfg.MaxHostCheckSpread*60) / float64(p.TotalScheduledHosts)
			if p.HostICD > maxDelay {
				p.HostICD = maxDelay
			}
		}
	case ICDUser:
		p.HostICD = cfg.HostInterCheckDelay
	}

	// Interleave factor
	switch cfg.ServiceInterleaveMethod {
	case ILFSmart:
		if p.TotalScheduledHosts > 0 {
			avg := float64(p.TotalScheduledSvcs) / float64(p.TotalScheduledHosts)
			p.InterleaveFactor = int(math.Ceil(avg))
		}
		if p.InterleaveFactor < 1 {
			p.InterleaveFactor = 1
		}
	default:
		if cfg.ServiceInterleaveFactor > 0 {
			p.InterleaveFactor = cfg.ServiceInterleaveFactor
		} else {
			p.InterleaveFactor = 1
		}
	}

	return p
}

// InitTimingLoop schedules all initial service and host checks, spreading them
// across time to prevent thundering herd.
func InitTimingLoop(cfg *objects.Config, services []*objects.Service, hosts []*objects.Host, now time.Time) ([]*Event, SchedulingParams) {
	params := CalculateSchedulingParams(cfg, services, hosts)
	il := cfg.IntervalLength
	if il <= 0 {
		il = 60
	}

	var events []*Event

	// Schedule service checks with interleaving
	if params.TotalScheduledSvcs > 0 && params.InterleaveFactor > 0 {
		totalInterleaveBlocks := int(math.Ceil(float64(params.TotalScheduledSvcs) / float64(params.InterleaveFactor)))
		currentInterleaveBlock := 0
		interleaveBlockIndex := 0

		for _, svc := range services {
			if !svc.ShouldBeScheduled {
				continue
			}
			interleaveBlockIndex++
			multFactor := currentInterleaveBlock + (interleaveBlockIndex * totalInterleaveBlocks)
			checkDelay := float64(multFactor) * params.ServiceICD

			window := checkWindow(svc.CurrentState, svc.StateType, svc.CheckInterval, svc.RetryInterval, il)
			if checkDelay > window {
				checkDelay = rand.Float64() * window
			}

			svc.NextCheck = now.Add(time.Duration(checkDelay * float64(time.Second)))

			events = append(events, &Event{
				Type:               EventServiceCheck,
				RunTime:            svc.NextCheck,
				HostName:           svc.Host.Name,
				ServiceDescription: svc.Description,
			})

			if interleaveBlockIndex >= params.InterleaveFactor {
				currentInterleaveBlock++
				interleaveBlockIndex = 0
			}
		}
	}

	// Schedule host checks (no interleaving)
	multFactor := 0
	for _, h := range hosts {
		if !h.ShouldBeScheduled {
			continue
		}
		checkDelay := float64(multFactor) * params.HostICD
		window := checkWindow(h.CurrentState, h.StateType, h.CheckInterval, h.RetryInterval, il)
		if checkDelay > window {
			checkDelay = rand.Float64() * window
		}

		h.NextCheck = now.Add(time.Duration(checkDelay * float64(time.Second)))

		events = append(events, &Event{
			Type:     EventHostCheck,
			RunTime:  h.NextCheck,
			HostName: h.Name,
		})
		multFactor++
	}

	return events, params
}

// checkWindow returns the appropriate check window in seconds based on state.
func checkWindow(currentState, stateType int, checkInterval, retryInterval float64, intervalLength int) float64 {
	if currentState != 0 && stateType == objects.StateTypeSoft {
		return retryInterval * float64(intervalLength)
	}
	return checkInterval * float64(intervalLength)
}

// ScheduleServiceCheck creates or replaces a service check event with deconfliction.
// Returns the event to add (caller adds to heap).
func ScheduleServiceCheck(existing *Event, newTime time.Time, newOptions int) (*Event, bool) {
	newForced := newOptions&objects.CheckOptionForceExecution != 0

	if existing != nil {
		existForced := existing.CheckOptions&objects.CheckOptionForceExecution != 0

		if existForced && newForced {
			if newTime.Before(existing.RunTime) {
				return &Event{
					Type:         EventServiceCheck,
					RunTime:      newTime,
					CheckOptions: newOptions,
				}, true
			}
			return nil, false // keep existing
		}
		if existForced && !newForced {
			return nil, false // keep existing forced
		}
		if !existForced && newForced {
			return &Event{
				Type:         EventServiceCheck,
				RunTime:      newTime,
				CheckOptions: newOptions,
			}, true
		}
		// both non-forced: use earlier
		if newTime.Before(existing.RunTime) {
			return &Event{
				Type:         EventServiceCheck,
				RunTime:      newTime,
				CheckOptions: newOptions,
			}, true
		}
		return nil, false
	}

	return &Event{
		Type:         EventServiceCheck,
		RunTime:      newTime,
		CheckOptions: newOptions,
	}, true
}

// NudgeDuration returns a random nudge between NudgeMin and NudgeMax seconds.
func NudgeDuration() time.Duration {
	n := NudgeMin + rand.Intn(NudgeMax-NudgeMin+1)
	return time.Duration(n) * time.Second
}
