package freshness

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestChecker_ServiceFreshness_NotStale(t *testing.T) {
	cfg := objects.DefaultConfig()
	now := time.Now()
	c := &Checker{
		Cfg:        cfg,
		EventStart: now.Add(-10 * time.Minute),
	}

	svc := &objects.Service{
		CheckFreshness:      true,
		ActiveChecksEnabled: true,
		CheckInterval:       5,
		LastCheck:           now.Add(-2 * time.Minute), // checked 2 min ago
		FreshnessThreshold:  300,                       // 5 min threshold
	}

	count := c.CheckServiceFreshness([]*objects.Service{svc}, now)
	if count != 0 {
		t.Error("service should not be stale (checked 2 min ago, threshold 5 min)")
	}
}

func TestChecker_ServiceFreshness_Stale(t *testing.T) {
	cfg := objects.DefaultConfig()
	now := time.Now()
	c := &Checker{
		Cfg:        cfg,
		EventStart: now.Add(-10 * time.Minute),
	}

	scheduled := false
	c.ScheduleServiceCheck = func(svc *objects.Service, _ time.Time, options int) {
		scheduled = true
		if options&objects.CheckOptionForceExecution == 0 {
			panic("expected force execution flag")
		}
		if options&objects.CheckOptionFreshnessCheck == 0 {
			panic("expected freshness check flag")
		}
	}

	svc := &objects.Service{
		CheckFreshness:      true,
		ActiveChecksEnabled: true,
		CheckInterval:       5,
		LastCheck:           now.Add(-10 * time.Minute), // checked 10 min ago
		FreshnessThreshold:  300,                        // 5 min threshold
	}

	count := c.CheckServiceFreshness([]*objects.Service{svc}, now)
	if count != 1 {
		t.Errorf("expected 1 stale, got %d", count)
	}
	if !scheduled {
		t.Error("expected check to be scheduled")
	}
	if !svc.IsBeingFreshened {
		t.Error("expected IsBeingFreshened to be set")
	}
}

func TestChecker_ServiceFreshness_SkipDisabled(t *testing.T) {
	cfg := objects.DefaultConfig()
	now := time.Now()
	c := &Checker{Cfg: cfg, EventStart: now.Add(-10 * time.Minute)}

	svc := &objects.Service{
		CheckFreshness:      false, // disabled
		ActiveChecksEnabled: true,
		CheckInterval:       5,
		LastCheck:           now.Add(-10 * time.Minute),
		FreshnessThreshold:  300,
	}

	count := c.CheckServiceFreshness([]*objects.Service{svc}, now)
	if count != 0 {
		t.Error("should skip service with freshness checking disabled")
	}
}

func TestChecker_AutoThreshold(t *testing.T) {
	cfg := objects.DefaultConfig()
	now := time.Now()
	c := &Checker{Cfg: cfg, EventStart: now.Add(-60 * time.Minute)}

	svc := &objects.Service{
		CheckFreshness:      true,
		ActiveChecksEnabled: true,
		CheckInterval:       5,
		FreshnessThreshold:  0, // auto-calculate
		LastCheck:           now.Add(-20 * time.Minute),
		Latency:             0.5,
	}

	// Auto threshold = check_interval * interval_length + latency + additional
	// = 5 * 60 + 0.5 + 15 = 315.5 seconds
	// Last check was 20 min ago = 1200 seconds ago
	// 1200 > 315.5, so should be stale
	scheduled := false
	c.ScheduleServiceCheck = func(s *objects.Service, t time.Time, options int) { scheduled = true }

	c.CheckServiceFreshness([]*objects.Service{svc}, now)
	if !scheduled {
		t.Error("expected stale service with auto-threshold")
	}
}

func TestChecker_GoldenRatioHeuristic(t *testing.T) {
	cfg := objects.DefaultConfig()
	now := time.Now()
	eventStart := now.Add(-5 * time.Minute)
	c := &Checker{Cfg: cfg, EventStart: eventStart}

	// Last check was way before event start (engine was down for hours)
	svc := &objects.Service{
		CheckFreshness:       true,
		PassiveChecksEnabled: true,
		CheckInterval:        5,
		FreshnessThreshold:   600, // 10 min
		LastCheck:            eventStart.Add(-2 * time.Hour),
	}

	// Golden ratio: downtime (2h) > 0.618 * 600s (371s)
	// So expiration = eventStart + threshold = eventStart + 10min = eventStart+10min
	// now = eventStart + 5min, so NOT stale yet
	count := c.CheckServiceFreshness([]*objects.Service{svc}, now)
	if count != 0 {
		t.Error("golden ratio should delay staleness after long downtime")
	}

	// Now check again 6 minutes later (11 min after event_start)
	later := eventStart.Add(11 * time.Minute)
	scheduled := false
	c.ScheduleServiceCheck = func(s *objects.Service, t time.Time, options int) { scheduled = true }
	c.CheckServiceFreshness([]*objects.Service{svc}, later)
	if !scheduled {
		t.Error("should be stale 11 min after event_start with 10 min threshold")
	}
}
