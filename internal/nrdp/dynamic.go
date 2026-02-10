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

// EnsureHost creates a minimal dynamic host if it does not already exist.
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
		MaxCheckAttempts:     1,
		PassiveChecksEnabled: true,
		ActiveChecksEnabled:  false,
		Dynamic:              true,
		LastSeen:             time.Now(),
		ShouldBeScheduled:    false,
		CurrentState:         4, // pending
		StateType:            objects.StateTypeHard,
	}
	d.store.AddHost(host)
	d.mu.Lock()
	d.records[hostname] = time.Now()
	d.mu.Unlock()
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

// Touch updates the last-seen timestamp for a host or service.
// It acquires store.Mu write lock internally.
func (d *DynamicTracker) Touch(hostname, servicename string) {
	now := time.Now()

	d.store.Mu.Lock()
	if host := d.store.GetHost(hostname); host != nil {
		host.LastSeen = now
	}
	if servicename != "" {
		if svc := d.store.GetService(hostname, servicename); svc != nil {
			svc.LastSeen = now
		}
	}
	d.store.Mu.Unlock()

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
