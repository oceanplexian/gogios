package scheduler

import (
	"container/heap"
	"log"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Scheduler is the main event loop that dispatches checks and processes results.
type Scheduler struct {
	cfg      *objects.Config
	queue    EventQueue
	hosts    map[string]*objects.Host
	services map[string]map[string]*objects.Service // host -> svc desc -> *Service

	resultCh  chan *objects.CheckResult
	commandCh chan Command
	stopCh    chan struct{}

	// Callbacks set by the application
	OnRunServiceCheck func(svc *objects.Service, options int)
	OnRunHostCheck    func(host *objects.Host, options int)
	OnStatusSave      func()
	OnRetentionSave   func()
	OnLogRotation     func()
	OnProcessResult   func(cr *objects.CheckResult)

	// Counters
	currentlyRunningServiceChecks int
	lastTimeChange                time.Time
}

// Command represents an external command sent to the scheduler.
type Command struct {
	Name string
	Args []string
}

// New creates a new Scheduler.
func New(cfg *objects.Config, hosts []*objects.Host, services []*objects.Service, resultCh chan *objects.CheckResult) *Scheduler {
	s := &Scheduler{
		cfg:       cfg,
		hosts:     make(map[string]*objects.Host, len(hosts)),
		services:  make(map[string]map[string]*objects.Service),
		resultCh:  resultCh,
		commandCh: make(chan Command, 100),
		stopCh:    make(chan struct{}),
	}

	for _, h := range hosts {
		s.hosts[h.Name] = h
	}
	for _, svc := range services {
		if svc.Host == nil {
			continue
		}
		if s.services[svc.Host.Name] == nil {
			s.services[svc.Host.Name] = make(map[string]*objects.Service)
		}
		s.services[svc.Host.Name][svc.Description] = svc
	}

	return s
}

// Init schedules all initial checks and recurring events, then returns.
func (s *Scheduler) Init(hosts []*objects.Host, services []*objects.Service) {
	now := time.Now()
	heap.Init(&s.queue)

	// Schedule initial checks
	checkEvents, _ := InitTimingLoop(s.cfg, services, hosts, now)
	for _, e := range checkEvents {
		heap.Push(&s.queue, e)
	}

	// Schedule recurring system events
	recurring := RecurringEvents(now,
		s.cfg.CheckReaperInterval,
		s.cfg.OrphanCheckInterval,
		s.cfg.ServiceFreshnessCheckInterval,
		s.cfg.HostFreshnessCheckInterval,
		s.cfg.StatusUpdateInterval,
		s.cfg.RetentionUpdateInterval,
		s.cfg.AutoReschedulingInterval,
		s.cfg.CheckServiceFreshness,
		s.cfg.CheckHostFreshness,
		s.cfg.AutoReschedulingEnabled,
	)
	for _, e := range recurring {
		heap.Push(&s.queue, e)
	}
}

// SendCommand sends an external command to the scheduler.
func (s *Scheduler) SendCommand(cmd Command) {
	s.commandCh <- cmd
}

// Stop signals the scheduler to shut down. Safe to call multiple times.
func (s *Scheduler) Stop() {
	select {
	case <-s.stopCh:
		// Already closed
	default:
		close(s.stopCh)
	}
}

// Run is the main event loop. It blocks until Stop() is called.
func (s *Scheduler) Run() {
	s.lastTimeChange = time.Now()

	for {
		var timer *time.Timer
		var timerCh <-chan time.Time

		if s.queue.Len() > 0 {
			next := s.queue[0]
			wait := time.Until(next.RunTime)
			if wait < 0 {
				wait = 0
			}
			timer = time.NewTimer(wait)
			timerCh = timer.C
		} else {
			// No events; poll every second
			timer = time.NewTimer(time.Second)
			timerCh = timer.C
		}

		select {
		case <-s.stopCh:
			timer.Stop()
			return

		case cr := <-s.resultCh:
			timer.Stop()
			if s.OnProcessResult != nil {
				s.OnProcessResult(cr)
			}

		case cmd := <-s.commandCh:
			timer.Stop()
			s.handleCommand(cmd)

		case <-timerCh:
			s.fireReadyEvents()
		}
	}
}

// fireReadyEvents fires all events whose RunTime has arrived (within 100ms tolerance).
func (s *Scheduler) fireReadyEvents() {
	now := time.Now()
	tolerance := 100 * time.Millisecond

	// Detect time change
	elapsed := now.Sub(s.lastTimeChange)
	if elapsed < -30*time.Second || elapsed > 5*time.Minute {
		log.Printf("Time change detected (%.1fs drift), adjusting events", elapsed.Seconds())
		s.compensateTimeChange(now)
	}
	s.lastTimeChange = now

	for s.queue.Len() > 0 {
		next := s.queue[0]
		if next.RunTime.After(now.Add(tolerance)) {
			break
		}

		// Check if event should run
		if !s.shouldRunEvent(next) {
			// Nudge the event forward
			heap.Pop(&s.queue)
			next.RunTime = now.Add(NudgeDuration())
			heap.Push(&s.queue, next)
			continue
		}

		heap.Pop(&s.queue)
		s.handleEvent(next, now)

		// Reschedule recurring events
		if next.Recurring && next.Interval > 0 {
			next.RunTime = next.RunTime.Add(next.Interval)
			if next.RunTime.Before(now) {
				next.RunTime = now.Add(next.Interval)
			}
			heap.Push(&s.queue, next)
		}
	}
}

// shouldRunEvent gates check events based on parallel limits and enabled flags.
func (s *Scheduler) shouldRunEvent(e *Event) bool {
	forced := e.CheckOptions&objects.CheckOptionForceExecution != 0

	switch e.Type {
	case EventServiceCheck:
		if forced {
			return true
		}
		if !s.cfg.ExecuteServiceChecks {
			return false
		}
		// Per-service active check toggle
		if svcMap := s.services[e.HostName]; svcMap != nil {
			if svc := svcMap[e.ServiceDescription]; svc != nil && !svc.ActiveChecksEnabled {
				return false
			}
		}
		if s.cfg.MaxParallelServiceChecks > 0 &&
			s.currentlyRunningServiceChecks >= s.cfg.MaxParallelServiceChecks {
			return false
		}
		return true

	case EventHostCheck:
		if forced {
			return true
		}
		if !s.cfg.ExecuteHostChecks {
			return false
		}
		// Per-host active check toggle
		if host := s.hosts[e.HostName]; host != nil && !host.ActiveChecksEnabled {
			return false
		}
		return true

	default:
		return true
	}
}

func (s *Scheduler) handleEvent(e *Event, now time.Time) {
	switch e.Type {
	case EventServiceCheck:
		svcMap := s.services[e.HostName]
		if svcMap == nil {
			return
		}
		svc := svcMap[e.ServiceDescription]
		if svc == nil {
			return
		}
		svc.Latency = now.Sub(e.RunTime).Seconds()
		if svc.Latency < 0 {
			svc.Latency = 0
		}
		s.currentlyRunningServiceChecks++
		svc.IsExecuting = true
		if s.OnRunServiceCheck != nil {
			s.OnRunServiceCheck(svc, e.CheckOptions)
		}

	case EventHostCheck:
		host := s.hosts[e.HostName]
		if host == nil {
			return
		}
		host.Latency = now.Sub(e.RunTime).Seconds()
		if host.Latency < 0 {
			host.Latency = 0
		}
		host.IsExecuting = true
		if s.OnRunHostCheck != nil {
			s.OnRunHostCheck(host, e.CheckOptions)
		}

	case EventStatusSave:
		if s.OnStatusSave != nil {
			s.OnStatusSave()
		}

	case EventRetentionSave:
		if s.OnRetentionSave != nil {
			s.OnRetentionSave()
		}

	case EventLogRotation:
		if s.OnLogRotation != nil {
			s.OnLogRotation()
		}

	case EventSFreshnessCheck, EventHFreshnessCheck:
		// Handled via callback in OnProcessResult or separate freshness checker

	case EventOrphanCheck:
		s.checkOrphans(now)

	case EventCheckReaper:
		// In Go, results come via channel, so this is mostly a no-op.
		// Could be used to check for external check result files.
	}
}

func (s *Scheduler) handleCommand(cmd Command) {
	// External command dispatch - placeholder for task #8
	_ = cmd
}

// checkOrphans finds checks that have been executing too long and reschedules them.
func (s *Scheduler) checkOrphans(now time.Time) {
	svcTimeout := time.Duration(s.cfg.ServiceCheckTimeout) * time.Second
	hostTimeout := time.Duration(s.cfg.HostCheckTimeout) * time.Second
	reaperSlack := time.Duration(s.cfg.CheckReaperInterval)*time.Second + 10*time.Minute

	for _, svcMap := range s.services {
		for _, svc := range svcMap {
			if !svc.IsExecuting {
				continue
			}
			expected := svc.NextCheck.Add(time.Duration(svc.Latency*float64(time.Second)) + svcTimeout + reaperSlack)
			if expected.Before(now) {
				svc.IsExecuting = false
				s.currentlyRunningServiceChecks--
				svc.NextCheck = now
				heap.Push(&s.queue, &Event{
					Type:               EventServiceCheck,
					RunTime:            now,
					HostName:           svc.Host.Name,
					ServiceDescription: svc.Description,
					CheckOptions:       objects.CheckOptionOrphanCheck,
				})
			}
		}
	}

	for _, host := range s.hosts {
		if !host.IsExecuting {
			continue
		}
		expected := host.NextCheck.Add(time.Duration(host.Latency*float64(time.Second)) + hostTimeout + reaperSlack)
		if expected.Before(now) {
			host.IsExecuting = false
			host.NextCheck = now
			heap.Push(&s.queue, &Event{
				Type:         EventHostCheck,
				RunTime:      now,
				HostName:     host.Name,
				CheckOptions: objects.CheckOptionOrphanCheck,
			})
		}
	}
}

// compensateTimeChange adjusts all events when a system time change is detected.
func (s *Scheduler) compensateTimeChange(now time.Time) {
	for _, e := range s.queue {
		if e.RunTime.After(now.Add(5 * time.Minute)) {
			// Event is too far in the future, bring it back
			e.RunTime = now.Add(NudgeDuration())
		}
	}
	heap.Init(&s.queue)
}

// AddEvent adds an event to the queue.
func (s *Scheduler) AddEvent(e *Event) {
	heap.Push(&s.queue, e)
}

// QueueLen returns the number of events in the queue.
func (s *Scheduler) QueueLen() int {
	return s.queue.Len()
}

// DecrementRunningServiceChecks decrements the counter (called after result processing).
func (s *Scheduler) DecrementRunningServiceChecks() {
	if s.currentlyRunningServiceChecks > 0 {
		s.currentlyRunningServiceChecks--
	}
}
