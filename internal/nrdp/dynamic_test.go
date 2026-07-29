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
	_ = store.AddCommand(&objects.Command{Name: "check_dummy"})
	_ = store.AddTimeperiod(&objects.Timeperiod{Name: "24x7"})
	for _, name := range []string{
		"discovered-hosts",
		"k8s-local-nodes",
		"storage-servers",
		"ai-servers",
	} {
		_ = store.AddHostGroup(&objects.HostGroup{Name: name})
	}
	for _, name := range []string{
		"passive-services",
		"kubernetes-node-services",
		"kubernetes-cluster-services",
		"dns-services",
		"fn2-services",
	} {
		_ = store.AddServiceGroup(&objects.ServiceGroup{Name: name})
	}
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
	if !svc.CheckFreshness || svc.FreshnessThreshold != defaultServiceFreshnessSeconds {
		t.Errorf("freshness = enabled:%v threshold:%d, want enabled:%v threshold:%d",
			svc.CheckFreshness, svc.FreshnessThreshold, true, defaultServiceFreshnessSeconds)
	}
	if svc.ActiveChecksEnabled || !svc.PassiveChecksEnabled {
		t.Errorf("service check mode = active:%v passive:%v, want passive-only",
			svc.ActiveChecksEnabled, svc.PassiveChecksEnabled)
	}
	if svc.CheckCommand == nil || svc.CheckCommandArgs != "3!UNKNOWN - passive result stale" {
		t.Errorf("freshness command = %#v args=%q", svc.CheckCommand, svc.CheckCommandArgs)
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

func TestOpenRouterCreditsNotifiesOncePerIncident(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)

	store.Mu.Lock()
	_ = store.AddHost(&objects.Host{
		Name:    "central",
		Alias:   "central",
		Dynamic: false,
	})
	tracker.EnsureService("central", "OpenRouter Credits")
	tracker.EnsureService("central", "Anycast DNS")
	// Simulate the service restored from an older generated config. The next
	// passive result must heal the live object as well as future config files.
	openRouter := store.GetService("central", "OpenRouter Credits")
	openRouter.NotificationInterval = 60
	openRouter.CurrentNotificationNumber = 1
	openRouter.NoMoreNotifications = false
	tracker.EnsureService("central", "OpenRouter Credits")
	store.Mu.Unlock()

	if openRouter.NotificationInterval != 0 {
		t.Fatalf("OpenRouter notification interval = %g, want notify-once interval 0",
			openRouter.NotificationInterval)
	}
	if !openRouter.NoMoreNotifications {
		t.Fatal("already-notified OpenRouter incident must suppress its pending repeat")
	}
	cfg := readCfg(t, path)
	openRouterStart := strings.Index(cfg, "service_description     OpenRouter Credits")
	if openRouterStart < 0 {
		t.Fatalf("generated config missing OpenRouter Credits:\n%s", cfg)
	}
	openRouterEnd := strings.Index(cfg[openRouterStart:], "}\n")
	openRouterStanza := cfg[openRouterStart : openRouterStart+openRouterEnd]
	if !strings.Contains(openRouterStanza, "notification_interval   0") {
		t.Fatalf("OpenRouter config must notify once per incident:\n%s", openRouterStanza)
	}
	anycastStart := strings.Index(cfg, "service_description     Anycast DNS")
	if anycastStart < 0 {
		t.Fatalf("generated config missing Anycast DNS:\n%s", cfg)
	}
	anycastEnd := strings.Index(cfg[anycastStart:], "}\n")
	anycastStanza := cfg[anycastStart : anycastStart+anycastEnd]
	if !strings.Contains(anycastStanza, "notification_interval   60") {
		t.Fatalf("other dynamic services must retain recurring notifications:\n%s", anycastStanza)
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

func TestEnsureServiceWiresProducerAndK8sDependencies(t *testing.T) {
	tracker, store := newTracker(t)
	hostname := "k8s-local-a1b2c3.fieldio.com"

	store.Mu.Lock()
	tracker.EnsureService(hostname, "Systemd FD Exhaustion")
	tracker.EnsureService(hostname, "API /health")
	tracker.EnsureService(hostname, "nrdc")
	tracker.EnsureService(hostname, "K8s Node Ready")
	tracker.EnsureService(hostname, "Systemd FD Exhaustion")
	store.Mu.Unlock()

	store.Mu.RLock()
	defer store.Mu.RUnlock()

	fd := store.GetService(hostname, "Systemd FD Exhaustion")
	if fd == nil {
		t.Fatal("Systemd FD Exhaustion service not created")
	}
	if len(fd.NotifyDeps) != 2 {
		t.Fatalf("NotifyDeps len = %d, want 2", len(fd.NotifyDeps))
	}
	if len(fd.ExecDeps) != 0 {
		t.Fatalf("ExecDeps len = %d, want 0 so freshness still executes", len(fd.ExecDeps))
	}
	masters := map[string]bool{}
	for _, dep := range fd.NotifyDeps {
		masters[dep.Service.Description] = true
	}
	for _, want := range []string{"nrdc", "K8s Node Ready"} {
		if !masters[want] {
			t.Errorf("missing dependency master %q; got %v", want, masters)
		}
	}
	nodeReady := store.GetService(hostname, "K8s Node Ready")
	if len(nodeReady.NotifyDeps) != 1 || nodeReady.NotifyDeps[0].Service.Description != "nrdc" {
		t.Fatalf("Node Ready parents = %#v, want only nrdc", nodeReady.NotifyDeps)
	}
	slashed := store.GetService(hostname, "API /health")
	if len(slashed.NotifyDeps) != 2 {
		t.Fatalf("slash service parents = %d, want nrdc + Node Ready", len(slashed.NotifyDeps))
	}
	if got := len(store.ServiceDependencies); got != 5 {
		t.Fatalf("ServiceDependencies len = %d, want 5", got)
	}
}

func TestGeneratedCfgContainsProducerAndK8sDependencies(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)
	hostname := "k8s-local-a1b2c3.fieldio.com"

	store.Mu.Lock()
	tracker.EnsureService(hostname, "nrdc")
	tracker.EnsureService(hostname, "K8s Node Ready")
	tracker.EnsureService(hostname, "Systemd FD Exhaustion")
	tracker.EnsureService(hostname, "API /health")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	for _, want := range []string{
		"define servicedependency {",
		"host_name                       " + hostname,
		"service_description             nrdc",
		"service_description             K8s Node Ready",
		"dependent_host_name             " + hostname,
		"dependent_service_description   Systemd FD Exhaustion",
		"dependent_service_description   API /health",
		"execution_failure_criteria      n",
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
	if strings.Contains(cfg, "service_description     stale") {
		t.Errorf("cfg still references pruned service stale:\n%s", cfg)
	}
}

func TestGeneratedCfgPolicyAndGroups(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)
	hostname := "k8s-local-a1b2c3.fieldio.com"

	store.Mu.Lock()
	tracker.EnsureService(hostname, "nrdc")
	tracker.EnsureService(hostname, "K8s Node Ready")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	for _, want := range []string{
		"hostgroups              discovered-hosts,k8s-local-nodes",
		"active_checks_enabled   0",
		"check_command           check_dummy!3!UNKNOWN - passive result stale",
		"freshness_threshold     180",
		"notification_options    u,c,r",
		"servicegroups           passive-services,kubernetes-node-services",
		"retain_nonstatus_information   0",
	} {
		if !strings.Contains(cfg, want) {
			t.Fatalf("cfg missing policy %q:\n%s", want, cfg)
		}
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

// A dynamic object restored from the persisted cfg has no tracker record,
// because records is only written when a check result ARRIVES and is empty on
// every boot. Prune iterates records, so before the seed pass such an object was
// never visited and could never age out: it sat in livestatus forever as
// permanent false staleness, unremovable (RemoveHost is called only from Prune).
// Two k8s-local-stage-* hosts left behind by a node rename survived exactly this
// way. Without the seed pass in Prune, this test fails.
func TestPruneAdoptsRestoredObjectsWithNoRecord(t *testing.T) {
	tracker, store := newTracker(t)

	// Simulate a restart: the object is in the store and flagged Dynamic (as
	// registerHosts does when reloading the generated cfg), but no result has
	// arrived, so there is no tracker record.
	store.Mu.Lock()
	if err := store.AddHost(&objects.Host{Name: "restored", Dynamic: true}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	store.Mu.Unlock()

	tracker.mu.Lock()
	if _, ok := tracker.records["restored"]; ok {
		t.Fatal("precondition failed: restored host should have no record")
	}
	tracker.mu.Unlock()

	// First prune adopts it and grants a full TTL of grace, so a restart never
	// prunes objects that simply have not re-reported yet.
	tracker.Prune()

	store.Mu.RLock()
	if store.GetHost("restored") == nil {
		t.Fatal("restored host was pruned immediately; it must get a full TTL of grace")
	}
	store.Mu.RUnlock()

	tracker.mu.Lock()
	seeded, ok := tracker.records["restored"]
	tracker.mu.Unlock()
	if !ok {
		t.Fatal("restored host was not adopted into the tracker; it can never age out")
	}

	// Age it past the TTL: now it must actually prune, which is the property
	// that was missing entirely.
	tracker.mu.Lock()
	tracker.records["restored"] = seeded.Add(-10 * time.Minute)
	tracker.mu.Unlock()

	tracker.Prune()

	store.Mu.RLock()
	defer store.Mu.RUnlock()
	if store.GetHost("restored") != nil {
		t.Error("adopted host still not pruned after exceeding TTL")
	}
}

func TestHealLoadedObjectsAppliesPolicyBeforeFirstPassiveResult(t *testing.T) {
	tracker, store := newTracker(t)
	hostname := "k8s-local-a1b2c3.fieldio.com"
	host := &objects.Host{
		Name:                hostname,
		Dynamic:             true,
		ActiveChecksEnabled: true,
		ShouldBeScheduled:   true,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}
	addService := func(description string) *objects.Service {
		service := &objects.Service{
			Host:                host,
			Description:         description,
			Dynamic:             true,
			ActiveChecksEnabled: true,
		}
		if err := store.AddService(service); err != nil {
			t.Fatal(err)
		}
		host.Services = append(host.Services, service)
		return service
	}
	nrdc := addService("nrdc")
	nodeReady := addService("K8s Node Ready")
	kubelet := addService("K8s Kubelet Health")

	tracker.HealLoadedObjects()

	if host.ActiveChecksEnabled || host.ShouldBeScheduled || !host.PassiveChecksEnabled {
		t.Fatalf("host was not healed to passive root: %#v", host)
	}
	if !nrdc.CheckFreshness || nrdc.FreshnessThreshold != nrdcServiceFreshnessSeconds {
		t.Fatalf("nrdc freshness = %v/%d", nrdc.CheckFreshness, nrdc.FreshnessThreshold)
	}
	if !nodeReady.CheckFreshness || !kubelet.CheckFreshness {
		t.Fatal("restored services did not get freshness policy")
	}
	if len(kubelet.NotifyDeps) != 2 {
		t.Fatalf("kubelet parents = %d, want nrdc + Node Ready", len(kubelet.NotifyDeps))
	}
	if len(host.HostGroups) != 2 || len(kubelet.ServiceGroups) != 2 {
		t.Fatalf("group policy missing: host=%d service=%d",
			len(host.HostGroups), len(kubelet.ServiceGroups))
	}
}

func TestHealLoadedObjectsPreservesUnseenServicesInGeneratedConfig(t *testing.T) {
	tracker, store, path := trackerWithCfg(t)
	host := &objects.Host{
		Name:    "central",
		Dynamic: true,
	}
	if err := store.AddHost(host); err != nil {
		t.Fatal(err)
	}
	addService := func(description string) {
		service := &objects.Service{
			Host:        host,
			Description: description,
			Dynamic:     true,
		}
		if err := store.AddService(service); err != nil {
			t.Fatal(err)
		}
		host.Services = append(host.Services, service)
	}
	addService("nrdc")
	addService("Five Hour Check")

	tracker.HealLoadedObjects()

	// Simulate a newly introduced fast producer result arriving before the
	// five-hour check. Creating it rewrites the generated config from
	// tracker.records.
	store.Mu.Lock()
	tracker.EnsureService("central", "New Fast Check")
	store.Mu.Unlock()

	cfg := readCfg(t, path)
	if !strings.Contains(cfg, "service_description     Five Hour Check") {
		t.Fatalf("startup inventory lost unseen low-frequency service:\n%s", cfg)
	}
	if !strings.Contains(cfg,
		"dependent_service_description   Five Hour Check") {
		t.Fatalf("startup inventory lost dependency for unseen service:\n%s", cfg)
	}
}
