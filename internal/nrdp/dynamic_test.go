package nrdp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

func TestTouchRecordUpdatesTimestamp(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("touchhost", "svc1")
	store.Mu.Unlock()

	// Record time before touch
	time.Sleep(10 * time.Millisecond)
	before := time.Now()

	tracker.TouchRecord("touchhost", "svc1")

	// TouchRecord only updates the tracker records, not the store objects.
	// Verify the record timestamp was updated.
	tracker.mu.Lock()
	ts, ok := tracker.records["touchhost\tsvc1"]
	tracker.mu.Unlock()
	if !ok {
		t.Fatal("record not found")
	}
	if ts.Before(before) {
		t.Errorf("record timestamp = %v, want >= %v", ts, before)
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

// trackerWithCfg returns a tracker wired to a temp cfg file so the writer is
// exercised end-to-end.
func trackerWithCfg(t *testing.T) (*DynamicTracker, *objects.ObjectStore, string) {
	t.Helper()
	tr, store := newTracker(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "nrdp_generated.cfg")
	tr.SetConfigPath(path)
	return tr, store, path
}

func readCfg(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestGeneratedCfgContainsEnsuredHost(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	tracker.EnsureHost("foo")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	if !strings.Contains(cfg, "define host {") {
		t.Fatalf("cfg missing `define host {`:\n%s", cfg)
	}
	if !strings.Contains(cfg, "host_name               foo") {
		t.Fatalf("cfg missing host_name=foo:\n%s", cfg)
	}
}

func TestGeneratedCfgContainsEnsuredService(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	tracker.EnsureService("foo", "bar")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	if !strings.Contains(cfg, "define service {") {
		t.Fatalf("cfg missing `define service {`:\n%s", cfg)
	}
	if !strings.Contains(cfg, "host_name               foo") {
		t.Fatalf("cfg missing host_name=foo:\n%s", cfg)
	}
	if !strings.Contains(cfg, "service_description     bar") {
		t.Fatalf("cfg missing service_description=bar:\n%s", cfg)
	}
}

func TestEnsureServiceWiresSystemdFDDependency(t *testing.T) {
	tracker, store := newTracker(t)

	store.Mu.Lock()
	tracker.EnsureService("node-01", "K8s Node Ready")
	tracker.EnsureService("node-01", "Systemd FD Exhaustion")
	tracker.EnsureService("node-01", "Systemd FD Exhaustion")
	store.Mu.Unlock()

	store.Mu.RLock()
	defer store.Mu.RUnlock()

	fd := store.GetService("node-01", "Systemd FD Exhaustion")
	if fd == nil {
		t.Fatal("Systemd FD Exhaustion service not created")
	}
	if len(fd.NotifyDeps) != 1 {
		t.Fatalf("NotifyDeps len = %d, want 1", len(fd.NotifyDeps))
	}
	if len(fd.ExecDeps) != 1 {
		t.Fatalf("ExecDeps len = %d, want 1", len(fd.ExecDeps))
	}
	dep := fd.NotifyDeps[0]
	if dep.Service == nil || dep.Service.Description != "K8s Node Ready" {
		t.Fatalf("dependency master = %#v, want K8s Node Ready", dep.Service)
	}
	if got := len(store.ServiceDependencies); got != 1 {
		t.Fatalf("ServiceDependencies len = %d, want 1", got)
	}
}

func TestGeneratedCfgContainsSystemdFDDependency(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	tracker.EnsureService("node-01", "K8s Node Ready")
	tracker.EnsureService("node-01", "Systemd FD Exhaustion")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	for _, want := range []string{
		"define servicedependency {",
		"host_name                       node-01",
		"service_description             K8s Node Ready",
		"dependent_host_name             node-01",
		"dependent_service_description   Systemd FD Exhaustion",
		"execution_failure_criteria      w,u,c,p",
		"notification_failure_criteria   w,u,c,p",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("cfg missing %q:\n%s", want, cfg)
		}
	}
}

func TestGeneratedCfgPruneRemovesExpiredEntries(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	tracker.EnsureService("keepme", "ok")
	tracker.EnsureService("goneby", "stale")
	store.Mu.Unlock()

	// Force the stale records past TTL.
	tracker.mu.Lock()
	past := time.Now().Add(-1 * time.Hour)
	tracker.records["goneby"] = past
	tracker.records["goneby\tstale"] = past
	tracker.mu.Unlock()

	tracker.Prune()

	cfg := readCfg(t, path)
	if !strings.Contains(cfg, "host_name               keepme") {
		t.Errorf("cfg should still contain keepme:\n%s", cfg)
	}
	if strings.Contains(cfg, "goneby") {
		t.Errorf("cfg still references pruned host goneby:\n%s", cfg)
	}
	if strings.Contains(cfg, "stale") {
		t.Errorf("cfg still references pruned service stale:\n%s", cfg)
	}
}

func TestGeneratedCfgAtomicWriteNoStaleTmp(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	tracker.EnsureHost("foo")
	store.Mu.Unlock()

	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf(".tmp file leaked: stat err=%v", err)
	}
}

func TestGeneratedCfgConcurrentEnsureHost(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("racy-%03d", idx)
			store.Mu.Lock()
			tracker.EnsureHost(name)
			store.Mu.Unlock()
		}(i)
	}
	wg.Wait()

	cfg := readCfg(t, path)
	for i := 0; i < N; i++ {
		want := fmt.Sprintf("host_name               racy-%03d", i)
		if !strings.Contains(cfg, want) {
			t.Errorf("cfg missing host racy-%03d after concurrent EnsureHost", i)
		}
	}
}

func TestGeneratedCfgDisabledWhenNoPath(t *testing.T) {
	tracker, store := newTracker(t)
	// SetConfigPath NOT called — the writer must be a no-op.

	store.Mu.Lock()
	tracker.EnsureHost("foo")
	store.Mu.Unlock()
	// Nothing to assert beyond "didn't panic / didn't write somewhere weird".
	// If a future regression makes us attempt a write with empty path, os.Rename
	// of an empty source would error and we'd see it in logFunc — which is a
	// silent no-op in tests. So just exercise the path.
}

func TestGeneratedCfgSkipsStaticHosts(t *testing.T) {
	// Regression test for the duplicate-host startup crash seen during
	// initial KANB-110 deploy. Static hosts get tracked in d.records when
	// NRDP starts pushing results for them (e.g. host "central" is defined
	// in hosts.cfg AND receives NRDP submissions). The generated cfg must
	// omit them; emitting `define host { host_name=central }` while the
	// static def also exists makes nagios refuse to load the whole config
	// ("duplicate host: central").
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	// Pre-seed a static host the way config loading would.
	_ = store.AddHost(&objects.Host{
		Name:    "static-box",
		Alias:   "static-box",
		Dynamic: false,
	})
	// EnsureHost on a pre-existing static host should record the touch
	// (so TTL tracking works) but the writer must skip emitting the host
	// stanza.
	tracker.EnsureHost("static-box")
	tracker.EnsureHost("real-dyn")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	if strings.Contains(cfg, "host_name               static-box") {
		t.Errorf("cfg must not redefine static host:\n%s", cfg)
	}
	if !strings.Contains(cfg, "host_name               real-dyn") {
		t.Errorf("cfg missing dynamic host real-dyn:\n%s", cfg)
	}
}

func TestGeneratedCfgEmitsDynamicSvcOnStaticHost(t *testing.T) {
	// A dynamic service on a static host is a real configuration: e.g.
	// central is statically defined, but NRDP discovers "Anycast DNS"
	// passively on it. The service must be emitted, but the host stanza
	// must NOT be (it'd duplicate the static def).
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	_ = store.AddHost(&objects.Host{
		Name:    "central",
		Alias:   "central",
		Dynamic: false,
	})
	tracker.EnsureService("central", "Anycast DNS")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	// Host stanza must be omitted.
	if strings.Contains(cfg, "host_name               central\n    alias                   central") {
		t.Errorf("cfg should not redefine static host 'central':\n%s", cfg)
	}
	// Service stanza must be present.
	if !strings.Contains(cfg, "service_description     Anycast DNS") {
		t.Errorf("cfg missing dynamic service on static host:\n%s", cfg)
	}
}

func TestGeneratedCfgIncludesContactGroupsWhenPresent(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	// Pre-seed the contact groups that defaultContactGroups looks for so the
	// generated cfg uses the same names. Otherwise contactGroupsCSV falls back
	// to "bridge-admins" alone.
	store.Mu.Lock()
	_ = store.AddContactGroup(&objects.ContactGroup{Name: "admins"})
	_ = store.AddContactGroup(&objects.ContactGroup{Name: "bridge-admins"})
	tracker.EnsureService("h1", "s1")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	if !strings.Contains(cfg, "contact_groups          admins,bridge-admins") {
		t.Fatalf("cfg missing expected contact_groups line:\n%s", cfg)
	}
}
