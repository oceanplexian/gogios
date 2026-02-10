package nrdp

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func newTracker(t *testing.T) (*DynamicTracker, *objects.ObjectStore) {
	t.Helper()
	store := objects.NewObjectStore()
	tracker := NewDynamicTracker(store, 5*time.Minute, 1*time.Minute)
	// Suppress log output in tests
	tracker.SetLogger(func(string, ...interface{}) {})
	return tracker, store
}

func TestEnsureHostCreatesNew(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureHost("newhost")
	store.Mu.Unlock()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	host := store.GetHost("newhost")
	if host == nil {
		t.Fatal("host not created")
	}
	if !host.Dynamic {
		t.Error("host.Dynamic = false, want true")
	}
	if host.Name != "newhost" {
		t.Errorf("host.Name = %q, want newhost", host.Name)
	}
}

func TestEnsureHostIdempotent(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureHost("myhost")
	tracker.EnsureHost("myhost")
	store.Mu.Unlock()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	count := 0
	for _, h := range store.Hosts {
		if h.Name == "myhost" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("host count = %d, want 1", count)
	}
}

func TestEnsureServiceCreatesHostAndService(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("svchost", "HTTP")
	store.Mu.Unlock()

	store.Mu.RLock()
	defer store.Mu.RUnlock()

	host := store.GetHost("svchost")
	if host == nil {
		t.Fatal("host not created")
	}
	svc := store.GetService("svchost", "HTTP")
	if svc == nil {
		t.Fatal("service not created")
	}
	if !svc.Dynamic {
		t.Error("svc.Dynamic = false, want true")
	}
}

func TestEnsureServiceIdempotent(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("h", "s")
	tracker.EnsureService("h", "s")
	store.Mu.Unlock()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	count := 0
	for _, svc := range store.Services {
		if svc.Host.Name == "h" && svc.Description == "s" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("service count = %d, want 1", count)
	}
}

func TestTouchUpdatesLastSeen(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("touchhost", "svc1")
	store.Mu.Unlock()

	// Record time before touch
	time.Sleep(10 * time.Millisecond)
	before := time.Now()

	tracker.Touch("touchhost", "svc1")

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	host := store.GetHost("touchhost")
	if host == nil {
		t.Fatal("host not found")
	}
	if host.LastSeen.Before(before) {
		t.Errorf("host.LastSeen = %v, want >= %v", host.LastSeen, before)
	}
	svc := store.GetService("touchhost", "svc1")
	if svc == nil {
		t.Fatal("service not found")
	}
	if svc.LastSeen.Before(before) {
		t.Errorf("svc.LastSeen = %v, want >= %v", svc.LastSeen, before)
	}
}

func TestPruneRemovesStale(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("stalehost", "stalesvc")
	store.Mu.Unlock()

	// Manually set records to the past (beyond TTL)
	tracker.mu.Lock()
	past := time.Now().Add(-10 * time.Minute)
	tracker.records["stalehost"] = past
	tracker.records["stalehost\tstalesvc"] = past
	tracker.mu.Unlock()

	tracker.Prune()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	if store.GetHost("stalehost") != nil {
		t.Error("stale host was not pruned")
	}
	if store.GetService("stalehost", "stalesvc") != nil {
		t.Error("stale service was not pruned")
	}
}

func TestPruneSparesStatic(t *testing.T) {
	tracker, store := newTracker(t)

	// Add a static (non-dynamic) host directly to the store
	store.Mu.Lock()
	store.AddHost(&objects.Host{
		Name:    "statichost",
		Dynamic: false,
	})
	store.Mu.Unlock()

	// Add a record for it in the tracker with old timestamp
	tracker.mu.Lock()
	tracker.records["statichost"] = time.Now().Add(-10 * time.Minute)
	tracker.mu.Unlock()

	tracker.Prune()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	if store.GetHost("statichost") == nil {
		t.Error("static host was incorrectly pruned")
	}
}

func TestPruneRemovesServicesWithHost(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("prunehost", "svc1")
	tracker.EnsureService("prunehost", "svc2")
	store.Mu.Unlock()

	// Set all records to past
	tracker.mu.Lock()
	past := time.Now().Add(-10 * time.Minute)
	for k := range tracker.records {
		tracker.records[k] = past
	}
	tracker.mu.Unlock()

	tracker.Prune()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	if store.GetHost("prunehost") != nil {
		t.Error("host was not pruned")
	}
	if store.GetService("prunehost", "svc1") != nil {
		t.Error("svc1 was not pruned")
	}
	if store.GetService("prunehost", "svc2") != nil {
		t.Error("svc2 was not pruned")
	}
}
