package nrdp

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// DynamicTracker manages auto-created NRDP hosts and services with TTL-based pruning.
type DynamicTracker struct {
	mu       sync.Mutex
	records  map[string]time.Time // key = "hostname" or "hostname\tservicename"
	store    *objects.ObjectStore
	ttl      time.Duration
	interval time.Duration
	stopCh   chan struct{}
	logFunc  func(format string, args ...interface{})

	// Host check configuration for dynamic hosts.
	hostCheckCmd string // command name, e.g. "check-host-alive"; empty = passive only

	// OnScheduleHost is called after a new dynamic host is created with
	// active checks enabled, so the scheduler can enqueue a host check event.
	OnScheduleHost func(host *objects.Host)
}

// NewDynamicTracker creates a tracker that auto-creates hosts/services and prunes
// them after ttl of inactivity, checking every pruneInterval.
func NewDynamicTracker(store *objects.ObjectStore, ttl, pruneInterval time.Duration) *DynamicTracker {
	return &DynamicTracker{
		records:  make(map[string]time.Time),
		store:    store,
		ttl:      ttl,
		interval: pruneInterval,
		stopCh:   make(chan struct{}),
		logFunc:  log.Printf,
	}
}

// SetLogger overrides the default log function.
func (d *DynamicTracker) SetLogger(fn func(string, ...interface{})) {
	d.logFunc = fn
}

// SetHostCheckCommand configures the check command name used for dynamic
// hosts. If non-empty, dynamic hosts get active checks enabled with this
// command. Pass empty string to keep hosts passive-only.
func (d *DynamicTracker) SetHostCheckCommand(name string) {
	d.hostCheckCmd = name
}

// EnsureHost creates a minimal dynamic host if it does not already exist.
// If a host check command is configured, the host gets active checks
// enabled and is scheduled for checking.
// IMPORTANT: The caller must hold store.Mu write lock.
func (d *DynamicTracker) EnsureHost(hostname string) {
	if d.store.GetHost(hostname) != nil {
		d.mu.Lock()
		d.records[hostname] = time.Now()
		d.mu.Unlock()
		return
	}

	host := &objects.Host{
		Name:                 hostname,
		DisplayName:          hostname,
		Alias:                hostname,
		Address:              hostname,
		MaxCheckAttempts:     3,
		CheckInterval:        5,
		RetryInterval:        1,
		PassiveChecksEnabled: true,
		ActiveChecksEnabled:  false,
		Dynamic:              true,
		LastSeen:             time.Now(),
		ShouldBeScheduled:    false,
		CurrentState:         4, // pending
		StateType:            objects.StateTypeHard,
	}

	// If a host check command is configured and exists in the store,
	// enable active checks so the host gets pinged on schedule.
	if d.hostCheckCmd != "" {
		if cmd := d.store.GetCommand(d.hostCheckCmd); cmd != nil {
			host.CheckCommand = cmd
			host.ActiveChecksEnabled = true
			host.ShouldBeScheduled = true
		}
	}

	d.store.AddHost(host)

	d.mu.Lock()
	d.records[hostname] = time.Now()
	d.mu.Unlock()

	// Notify the scheduler to enqueue a check event for this host.
	if host.ShouldBeScheduled && d.OnScheduleHost != nil {
		d.OnScheduleHost(host)
	}
}

// EnsureService creates a minimal dynamic service (and its host) if they do not exist.
// IMPORTANT: The caller must hold store.Mu write lock.
func (d *DynamicTracker) EnsureService(hostname, servicename string) {
	d.EnsureHost(hostname)

	if d.store.GetService(hostname, servicename) != nil {
		d.mu.Lock()
		d.records[hostname+"\t"+servicename] = time.Now()
		d.mu.Unlock()
		return
	}

	host := d.store.GetHost(hostname)
	svc := &objects.Service{
		Host:                 host,
		Description:          servicename,
		DisplayName:          servicename,
		MaxCheckAttempts:     1,
		PassiveChecksEnabled: true,
		ActiveChecksEnabled:  false,
		Dynamic:              true,
		LastSeen:             time.Now(),
		ShouldBeScheduled:    false,
		CurrentState:         4, // pending
		StateType:            objects.StateTypeHard,
	}
	d.store.AddService(svc)
	host.Services = append(host.Services, svc)

	d.mu.Lock()
	d.records[hostname+"\t"+servicename] = time.Now()
	d.mu.Unlock()
}

// TouchRecord updates the last-seen timestamp in the tracker records map.
// It does NOT acquire store.Mu; the caller is responsible for updating
// Host.LastSeen / Service.LastSeen under the store lock if needed.
func (d *DynamicTracker) TouchRecord(hostname, servicename string) {
	now := time.Now()
	d.mu.Lock()
	if servicename != "" {
		d.records[hostname+"\t"+servicename] = now
	} else {
		d.records[hostname] = now
	}
	d.mu.Unlock()
}

// Prune removes dynamic hosts and services that have not been seen within the TTL.
// It acquires store.Mu write lock internally.
func (d *DynamicTracker) Prune() {
	cutoff := time.Now().Add(-d.ttl)
	var prunedHosts, prunedServices int

	d.mu.Lock()
	defer d.mu.Unlock()

	d.store.Mu.Lock()
	defer d.store.Mu.Unlock()

	// First pass: prune stale services
	for key, lastSeen := range d.records {
		if !strings.Contains(key, "\t") {
			continue
		}
		if lastSeen.After(cutoff) {
			continue
		}
		parts := strings.SplitN(key, "\t", 2)
		hostname, desc := parts[0], parts[1]
		svc := d.store.GetService(hostname, desc)
		if svc != nil && !svc.Dynamic {
			continue
		}
		d.store.RemoveService(hostname, desc)
		delete(d.records, key)
		prunedServices++
	}

	// Second pass: prune stale hosts
	for key, lastSeen := range d.records {
		if strings.Contains(key, "\t") {
			continue
		}
		if lastSeen.After(cutoff) {
			continue
		}
		hostname := key
		host := d.store.GetHost(hostname)
		if host == nil || !host.Dynamic {
			continue
		}
		// RemoveHost also removes all its services from the store
		d.store.RemoveHost(hostname)
		// Clean up any remaining service records for this host
		for svcKey := range d.records {
			if strings.HasPrefix(svcKey, hostname+"\t") {
				delete(d.records, svcKey)
			}
		}
		delete(d.records, key)
		prunedHosts++
	}

	if prunedHosts > 0 || prunedServices > 0 {
		d.logFunc("dynamic pruner: removed %d hosts, %d services", prunedHosts, prunedServices)
	}
}

// StartPruner launches a background goroutine that calls Prune at the configured interval.
func (d *DynamicTracker) StartPruner() {
	go func() {
		ticker := time.NewTicker(d.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				d.Prune()
			case <-d.stopCh:
				return
			}
		}
	}()
}

// Stop signals the pruner goroutine to exit.
func (d *DynamicTracker) Stop() {
	close(d.stopCh)
}
