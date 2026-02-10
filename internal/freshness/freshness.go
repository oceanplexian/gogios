package freshness

import (
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

const goldenRatio = 0.618

// Checker checks for stale service/host check results and triggers fresh checks.
type Checker struct {
	Cfg        *objects.Config
	EventStart time.Time // when the monitoring engine started

	// ScheduleServiceCheck triggers a forced freshness check for a stale service.
	ScheduleServiceCheck func(svc *objects.Service, t time.Time, options int)
	// ScheduleHostCheck triggers a forced freshness check for a stale host.
	ScheduleHostCheck func(h *objects.Host, t time.Time, options int)
}

// CheckServiceFreshness iterates all services and checks for stale results.
func (c *Checker) CheckServiceFreshness(services []*objects.Service, now time.Time) int {
	staleCount := 0
	for _, svc := range services {
		if c.isServiceStale(svc, now) {
			svc.IsBeingFreshened = true
			if c.ScheduleServiceCheck != nil {
				c.ScheduleServiceCheck(svc, now,
					objects.CheckOptionForceExecution|objects.CheckOptionFreshnessCheck)
			}
			staleCount++
		}
	}
	return staleCount
}

// CheckHostFreshness iterates all hosts and checks for stale results.
func (c *Checker) CheckHostFreshness(hosts []*objects.Host, now time.Time) int {
	staleCount := 0
	for _, host := range hosts {
		if c.isHostStale(host, now) {
			host.IsBeingFreshened = true
			if c.ScheduleHostCheck != nil {
				c.ScheduleHostCheck(host, now,
					objects.CheckOptionForceExecution|objects.CheckOptionFreshnessCheck)
			}
			staleCount++
		}
	}
	return staleCount
}

func (c *Checker) isServiceStale(svc *objects.Service, now time.Time) bool {
	if !svc.CheckFreshness {
		return false
	}
	if svc.IsExecuting {
		return false
	}
	if !svc.ActiveChecksEnabled && !svc.PassiveChecksEnabled {
		return false
	}
	if svc.IsBeingFreshened {
		return false
	}
	if svc.CheckInterval == 0 && svc.FreshnessThreshold == 0 {
		return false
	}

	threshold := c.serviceFreshnessThreshold(svc)
	if threshold <= 0 {
		return false
	}

	expiration := c.serviceExpirationTime(svc, threshold)
	return now.After(expiration)
}

func (c *Checker) isHostStale(host *objects.Host, now time.Time) bool {
	if !host.CheckFreshness {
		return false
	}
	if host.IsExecuting {
		return false
	}
	if !host.ActiveChecksEnabled && !host.PassiveChecksEnabled {
		return false
	}
	if host.IsBeingFreshened {
		return false
	}
	if host.CheckInterval == 0 && host.FreshnessThreshold == 0 {
		return false
	}

	threshold := c.hostFreshnessThreshold(host)
	if threshold <= 0 {
		return false
	}

	expiration := c.hostExpirationTime(host, threshold)
	return now.After(expiration)
}

// serviceFreshnessThreshold returns the threshold in seconds.
func (c *Checker) serviceFreshnessThreshold(svc *objects.Service) float64 {
	if svc.FreshnessThreshold > 0 {
		return float64(svc.FreshnessThreshold)
	}
	il := c.Cfg.IntervalLength
	if il <= 0 {
		il = 60
	}
	latency := svc.Latency
	additional := float64(c.Cfg.AdditionalFreshnessLatency)

	if svc.CurrentState != objects.ServiceOK && svc.StateType == objects.StateTypeSoft {
		return svc.RetryInterval*float64(il) + latency + additional
	}
	return svc.CheckInterval*float64(il) + latency + additional
}

func (c *Checker) hostFreshnessThreshold(host *objects.Host) float64 {
	if host.FreshnessThreshold > 0 {
		return float64(host.FreshnessThreshold)
	}
	il := c.Cfg.IntervalLength
	if il <= 0 {
		il = 60
	}
	latency := host.Latency
	additional := float64(c.Cfg.AdditionalFreshnessLatency)

	if host.CurrentState != objects.HostUp && host.StateType == objects.StateTypeSoft {
		return host.RetryInterval*float64(il) + latency + additional
	}
	return host.CheckInterval*float64(il) + latency + additional
}

// serviceExpirationTime calculates when a service result expires.
func (c *Checker) serviceExpirationTime(svc *objects.Service, threshold float64) time.Time {
	threshDur := time.Duration(threshold * float64(time.Second))

	// Never checked
	if svc.LastCheck.IsZero() {
		return c.EventStart.Add(threshDur)
	}

	// Passive check special case: golden ratio heuristic
	// If last_check < event_start and downtime > 61.8% of threshold,
	// use event_start to prevent notification storms after long outage
	if svc.LastCheck.Before(c.EventStart) {
		downtime := c.EventStart.Sub(svc.LastCheck)
		if downtime.Seconds() > goldenRatio*threshold {
			return c.EventStart.Add(threshDur)
		}
	}

	// Active checks enabled, event_start > last_check, no user threshold
	if svc.ActiveChecksEnabled && c.EventStart.After(svc.LastCheck) && svc.FreshnessThreshold == 0 {
		il := c.Cfg.IntervalLength
		if il <= 0 {
			il = 60
		}
		spreadExtra := time.Duration(c.Cfg.MaxServiceCheckSpread*il) * time.Second
		return c.EventStart.Add(threshDur + spreadExtra)
	}

	return svc.LastCheck.Add(threshDur)
}

func (c *Checker) hostExpirationTime(host *objects.Host, threshold float64) time.Time {
	threshDur := time.Duration(threshold * float64(time.Second))

	if host.LastCheck.IsZero() {
		return c.EventStart.Add(threshDur)
	}

	if host.LastCheck.Before(c.EventStart) {
		downtime := c.EventStart.Sub(host.LastCheck)
		if downtime.Seconds() > goldenRatio*threshold {
			return c.EventStart.Add(threshDur)
		}
	}

	if host.ActiveChecksEnabled && c.EventStart.After(host.LastCheck) && host.FreshnessThreshold == 0 {
		il := c.Cfg.IntervalLength
		if il <= 0 {
			il = 60
		}
		spreadExtra := time.Duration(c.Cfg.MaxHostCheckSpread*il) * time.Second
		return c.EventStart.Add(threshDur + spreadExtra)
	}

	return host.LastCheck.Add(threshDur)
}
