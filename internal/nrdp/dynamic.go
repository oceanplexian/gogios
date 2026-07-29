package nrdp

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

const defaultDynamicServiceNotificationInterval = 60.0

const (
	nrdcServiceName                = "nrdc"
	k8sNodeReadyServiceName        = "K8s Node Ready"
	containerdRollupServiceName    = "Containerd Health (All Nodes)"
	defaultServiceFreshnessSeconds = 1800
	nrdcServiceFreshnessSeconds    = 180
	centralServiceFreshnessSeconds = 21600
)

func dynamicServiceNotificationInterval(hostname, servicename string) float64 {
	// A low prepaid balance can remain actionable for hours while somebody
	// tops up the account. Repeating the identical warning every five minutes
	// creates a page storm without adding information. Interval zero still
	// sends the initial problem and recovery notifications; a later incident
	// starts a new notification cycle.
	if hostname == "central" && servicename == "OpenRouter Credits" {
		return 0
	}
	return defaultDynamicServiceNotificationInterval
}

func dynamicServiceFreshnessThreshold(hostname, servicename string) int {
	switch {
	case servicename == nrdcServiceName:
		return nrdcServiceFreshnessSeconds
	case hostname == "central":
		// Central rollups include intentionally low-frequency checks (up to
		// five hours). Six hours detects a dead producer without turning the
		// legitimate cadence into a false freshness incident.
		return centralServiceFreshnessSeconds
	default:
		return defaultServiceFreshnessSeconds
	}
}

func dynamicServiceNotificationsEnabled(hostname, servicename string) bool {
	// Per-node K8s Containerd checks are the actionable leaves. The all-node
	// service remains useful as a rollup/dashboard signal, but must not create
	// a duplicate page for the same node failure.
	return !(hostname == "central" && servicename == containerdRollupServiceName)
}

func dynamicServiceNotificationOptions(servicename string) uint32 {
	if servicename == k8sNodeReadyServiceName {
		// A clean cordon is normalized to WARNING and acts as the maintenance
		// parent for the node. Keep it visible without paging; actual
		// NotReady/pressure conditions remain CRITICAL and notify.
		return objects.OptUnknown | objects.OptCritical | objects.OptRecovery
	}
	return objects.OptWarning | objects.OptCritical | objects.OptUnknown | objects.OptRecovery
}

func dynamicServiceNotificationCriteria(servicename string) string {
	if servicename == k8sNodeReadyServiceName {
		return "u,c,r"
	}
	return "w,u,c,r"
}

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

	// cfgPath is the persistent .cfg file we regenerate atomically on every
	// EnsureHost / EnsureService / Prune. Empty disables the writer entirely
	// (matches pre-KANB-110 behavior for tests / minimal embeddings).
	cfgPath string

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

// SetConfigPath enables persistent .cfg writing for dynamic hosts/services.
// On every EnsureHost / EnsureService / Prune call the tracker rewrites this
// path with the full current set of dynamic objects, atomically (write tmp +
// rename). On gogios restart nagios will load these definitions via cfg_dir
// and retention.dat will attach state to them — closing KANB-110, the
// 15-minute "monitoring hole" after every restart. Pass empty to disable.
func (d *DynamicTracker) SetConfigPath(path string) {
	d.cfgPath = path
}

// EnsureHost creates a minimal dynamic host if it does not already exist.
// If a host check command is configured, the host gets active checks
// enabled and is scheduled for checking.
// IMPORTANT: The caller must hold store.Mu write lock.
func (d *DynamicTracker) EnsureHost(hostname string) {
	if existing := d.store.GetHost(hostname); existing != nil {
		// Heal pre-existing dynamic hosts the same way we do for services.
		if cg := d.store.GetContactGroup("bridge-admins"); cg != nil {
			has := false
			for _, g := range existing.ContactGroups {
				if g != nil && g.Name == "bridge-admins" {
					has = true
					break
				}
			}
			if !has {
				existing.ContactGroups = append(existing.ContactGroups, cg)
			}
		}
		// Dynamic host state is derived from the nrdc producer service. This is
		// more authoritative than ICMP for passive-only systems (and works for
		// names with no DNS record), while still giving Nagios a real host root
		// that suppresses all child service noise when the producer disappears.
		if existing.Dynamic {
			now := time.Now()
			existing.ActiveChecksEnabled = false
			existing.PassiveChecksEnabled = true
			existing.ShouldBeScheduled = false
			existing.CheckCommand = nil
			existing.CheckCommandArgs = ""
			if d.hostCheckCmd != "" {
				if cmd := d.store.GetCommand(d.hostCheckCmd); cmd != nil {
					existing.CheckCommand = cmd
					existing.ActiveChecksEnabled = true
					existing.ShouldBeScheduled = true
				}
			}
			existing.RetainStatusInformation = true
			existing.RetainNonstatusInformation = false
			d.attachDynamicHostGroups(existing)
			if !existing.HasBeenChecked {
				existing.CurrentState = objects.HostUp
				existing.StateType = objects.StateTypeHard
				existing.HasBeenChecked = true
				existing.LastCheck = now
				existing.LastStateChange = now
				if existing.PluginOutput == "" {
					existing.PluginOutput = "Host UP - registered via NRDP"
				}
			}
		}
		d.mu.Lock()
		_, existed := d.records[hostname]
		d.records[hostname] = time.Now()
		if !existed {
			// First time we've seen this pre-existing static/dynamic host
			// via NRDP — make sure it lands in the generated cfg so a future
			// restart (after the static def is removed, say) doesn't lose it.
			d.writeGeneratedConfigLocked()
			if existing.ShouldBeScheduled && d.OnScheduleHost != nil {
				d.OnScheduleHost(existing)
			}
		}
		d.mu.Unlock()
		return
	}

	now := time.Now()
	host := &objects.Host{
		Name:                 hostname,
		DisplayName:          hostname,
		Alias:                hostname,
		Address:              hostname,
		MaxCheckAttempts:     1,
		CheckInterval:        5,
		RetryInterval:        1,
		PassiveChecksEnabled: true,
		ActiveChecksEnabled:  false,
		NotificationsEnabled: true,
		NotificationOptions:  objects.OptDown | objects.OptUnreachable | objects.OptRecovery,
		NotificationInterval: 120,
		ContactGroups:        d.defaultContactGroups(),
		Dynamic:              true,
		LastSeen:             now,
		ShouldBeScheduled:    false,
		// A submitter actively reaching us implies the host is alive. Mark UP
		// at registration so the host doesn't sit at PENDING for the first
		// check interval. Active checks default to check_dummy below (cheap
		// no-op that always returns OK) so last_check stays fresh.
		CurrentState:    objects.HostUp,
		StateType:       objects.StateTypeHard,
		HasBeenChecked:  true,
		LastCheck:       now,
		LastStateChange: now,
		PluginOutput:    "Host UP - registered via NRDP",
	}

	// An explicitly configured command remains supported for installations
	// that want active dynamic-host checks. The production configuration
	// intentionally leaves this empty so the nrdc service is the host's
	// authoritative passive state source.
	if d.hostCheckCmd != "" {
		if cmd := d.store.GetCommand(d.hostCheckCmd); cmd != nil {
			host.CheckCommand = cmd
			host.ActiveChecksEnabled = true
			host.ShouldBeScheduled = true
		}
	}

	d.store.AddHost(host)
	d.attachDynamicHostGroups(host)

	d.mu.Lock()
	d.records[hostname] = time.Now()
	d.writeGeneratedConfigLocked()
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

	if existing := d.store.GetService(hostname, servicename); existing != nil {
		if existing.Dynamic {
			interval := dynamicServiceNotificationInterval(hostname, servicename)
			existing.NotificationInterval = interval
			existing.NoMoreNotifications = interval == 0 && existing.CurrentNotificationNumber > 0
			existing.MaxCheckAttempts = 1
			existing.PassiveChecksEnabled = true
			existing.ActiveChecksEnabled = false
			existing.ShouldBeScheduled = false
			existing.CheckFreshness = true
			existing.FreshnessThreshold = dynamicServiceFreshnessThreshold(hostname, servicename)
			existing.NotificationsEnabled = dynamicServiceNotificationsEnabled(hostname, servicename)
			existing.NotificationOptions = dynamicServiceNotificationOptions(servicename)
			existing.RetainStatusInformation = true
			existing.RetainNonstatusInformation = false
			if cmd := d.store.GetCommand("check_dummy"); cmd != nil {
				existing.CheckCommand = cmd
				existing.CheckCommandArgs = "3!UNKNOWN - passive result stale"
			}
			d.attachDynamicServiceGroups(existing)
		}
		// Ensure bridge-admins is attached to pre-existing dynamic services so
		// the nagios-bridge gets every state-change notification. Services
		// created before bridge-admins existed have a stale contact_groups
		// list in retention.dat; this opportunistically heals it.
		if cg := d.store.GetContactGroup("bridge-admins"); cg != nil {
			has := false
			for _, g := range existing.ContactGroups {
				if g != nil && g.Name == "bridge-admins" {
					has = true
					break
				}
			}
			if !has {
				existing.ContactGroups = append(existing.ContactGroups, cg)
			}
		}
		d.ensureDynamicServiceDependenciesForHost(hostname)
		d.mu.Lock()
		key := hostname + "\t" + servicename
		_, existed := d.records[key]
		d.records[key] = time.Now()
		if !existed {
			d.writeGeneratedConfigLocked()
		}
		d.mu.Unlock()
		return
	}

	host := d.store.GetHost(hostname)
	svc := &objects.Service{
		Host:                       host,
		Description:                servicename,
		DisplayName:                servicename,
		MaxCheckAttempts:           1,
		PassiveChecksEnabled:       true,
		ActiveChecksEnabled:        false,
		NotificationsEnabled:       true,
		NotificationOptions:        dynamicServiceNotificationOptions(servicename),
		NotificationInterval:       dynamicServiceNotificationInterval(hostname, servicename),
		ContactGroups:              d.defaultContactGroups(),
		CheckFreshness:             true,
		FreshnessThreshold:         dynamicServiceFreshnessThreshold(hostname, servicename),
		RetainStatusInformation:    true,
		RetainNonstatusInformation: false,
		Dynamic:                    true,
		LastSeen:                   time.Now(),
		ShouldBeScheduled:          false,
		CurrentState:               4, // pending
		StateType:                  objects.StateTypeHard,
	}
	svc.NotificationsEnabled = dynamicServiceNotificationsEnabled(hostname, servicename)
	if cmd := d.store.GetCommand("check_dummy"); cmd != nil {
		svc.CheckCommand = cmd
		svc.CheckCommandArgs = "3!UNKNOWN - passive result stale"
	}
	d.store.AddService(svc)
	host.Services = append(host.Services, svc)
	d.attachDynamicServiceGroups(svc)
	d.ensureDynamicServiceDependenciesForHost(hostname)

	d.mu.Lock()
	d.records[hostname+"\t"+servicename] = time.Now()
	d.writeGeneratedConfigLocked()
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

	// Seed pass: adopt dynamic objects that exist in the store but have no
	// tracker record.
	//
	// records is only ever written when a check result ARRIVES
	// (EnsureHost/EnsureService/TouchRecord), and it starts empty on every
	// boot. Dynamic objects, however, are also restored from the persisted
	// generated cfg by registerHosts/registerServices, which set Dynamic=true
	// directly on the store. A restored object whose host has stopped
	// submitting therefore had no record, and since the passes below iterate
	// records exclusively, the pruner never visited it: it became immortal and
	// sat in livestatus forever, decaying into permanent false staleness that
	// no TTL could clear and no operator command could remove (RemoveHost is
	// called from here and nowhere else).
	//
	// Observed live: two k8s-local-stage-* hosts left behind by a node rename
	// survived indefinitely because of exactly this.
	//
	// Seeding here rather than at startup keeps the fix self-contained -- there
	// is no initialisation order to get wrong, and an object that appears by
	// any future path is adopted too. Seeding with `now` deliberately grants a
	// full TTL of grace, so a restart never prunes objects that simply have not
	// re-reported yet.
	now := time.Now()
	for _, h := range d.store.Hosts {
		if h == nil || !h.Dynamic {
			continue
		}
		if _, ok := d.records[h.Name]; !ok {
			d.records[h.Name] = now
		}
	}
	for _, svc := range d.store.Services {
		if svc == nil || !svc.Dynamic {
			continue
		}
		if svc.Host == nil {
			continue
		}
		key := svc.Host.Name + "\t" + svc.Description
		if _, ok := d.records[key]; !ok {
			d.records[key] = now
		}
	}

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
		// Persist the new (smaller) set so a restart doesn't resurrect
		// the just-pruned objects from the previous cfg snapshot.
		d.writeGeneratedConfigLocked()
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

// defaultContactGroups returns the admins and discord-admins contact groups
// from the object store, for use as defaults on dynamically created objects.
func (d *DynamicTracker) defaultContactGroups() []*objects.ContactGroup {
	var cgs []*objects.ContactGroup
	for _, name := range []string{"admins", "discord-admins", "bridge-admins"} {
		if cg := d.store.GetContactGroup(name); cg != nil {
			cgs = append(cgs, cg)
		}
	}
	return cgs
}

func (d *DynamicTracker) attachDynamicHostGroups(host *objects.Host) {
	for _, name := range dynamicHostGroupNames(host.Name) {
		group := d.store.GetHostGroup(name)
		if group == nil {
			continue
		}
		if !containsHostGroup(host.HostGroups, group) {
			host.HostGroups = append(host.HostGroups, group)
		}
		if !containsHost(group.Members, host) {
			group.Members = append(group.Members, host)
		}
	}
}

func (d *DynamicTracker) attachDynamicServiceGroups(svc *objects.Service) {
	for _, name := range dynamicServiceGroupNames(svc.Host.Name, svc.Description) {
		group := d.store.GetServiceGroup(name)
		if group == nil {
			continue
		}
		if !containsServiceGroup(svc.ServiceGroups, group) {
			svc.ServiceGroups = append(svc.ServiceGroups, group)
		}
		if !containsService(group.Members, svc) {
			group.Members = append(group.Members, svc)
		}
	}
}

func containsHostGroup(groups []*objects.HostGroup, want *objects.HostGroup) bool {
	for _, group := range groups {
		if group == want {
			return true
		}
	}
	return false
}

func containsServiceGroup(groups []*objects.ServiceGroup, want *objects.ServiceGroup) bool {
	for _, group := range groups {
		if group == want {
			return true
		}
	}
	return false
}

func containsHost(hosts []*objects.Host, want *objects.Host) bool {
	for _, host := range hosts {
		if host == want {
			return true
		}
	}
	return false
}

func containsService(services []*objects.Service, want *objects.Service) bool {
	for _, service := range services {
		if service == want {
			return true
		}
	}
	return false
}

// Stop signals the pruner goroutine to exit.
func (d *DynamicTracker) Stop() {
	close(d.stopCh)
}
