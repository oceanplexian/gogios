package nrdp

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

const defaultDynamicServiceNotificationInterval = 60.0

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
		// Heal pre-existing dynamic hosts that loaded from the generated cfg
		// before the active-checks-by-default change. Make them active +
		// scheduled with check_dummy if they have no check command, and clear
		// any stale PENDING state.
		if existing.Dynamic {
			now := time.Now()
			if !existing.ActiveChecksEnabled {
				existing.ActiveChecksEnabled = true
				existing.ShouldBeScheduled = true
			}
			if existing.CheckCommand == nil {
				if cmd := d.store.GetCommand("check_dummy"); cmd != nil {
					existing.CheckCommand = cmd
					existing.CheckCommandArgs = "0!OK"
				}
			}
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
			// Queue a check event with the scheduler. Hosts loaded from the
			// generated cfg AFTER startup never went through InitTimingLoop,
			// so without this nudge they sit with next_check=0 forever. The
			// orphan-check backstop (KANB-108) catches them eventually, but
			// this kicks the first check immediately.
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
		MaxCheckAttempts:     3,
		CheckInterval:        5,
		RetryInterval:        1,
		PassiveChecksEnabled: true,
		ActiveChecksEnabled:  true,
		NotificationsEnabled: true,
		NotificationOptions:  objects.OptDown | objects.OptUnreachable | objects.OptRecovery,
		NotificationInterval: 120,
		ContactGroups:        d.defaultContactGroups(),
		Dynamic:              true,
		LastSeen:             now,
		ShouldBeScheduled:    true,
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

	// Prefer an explicitly configured host check command (e.g., fping) if
	// the user wired one up via nrdp_dynamic_host_check_command. Otherwise
	// fall back to check_dummy!0!OK — a no-op that always returns OK and
	// keeps last_check current without flapping on no-DNS hosts.
	if d.hostCheckCmd != "" {
		if cmd := d.store.GetCommand(d.hostCheckCmd); cmd != nil {
			host.CheckCommand = cmd
		}
	}
	if host.CheckCommand == nil {
		if cmd := d.store.GetCommand("check_dummy"); cmd != nil {
			host.CheckCommand = cmd
			host.CheckCommandArgs = "0!OK"
		}
	}

	d.store.AddHost(host)

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
		Host:                 host,
		Description:          servicename,
		DisplayName:          servicename,
		MaxCheckAttempts:     1,
		PassiveChecksEnabled: true,
		ActiveChecksEnabled:  false,
		NotificationsEnabled: true,
		NotificationOptions:  objects.OptWarning | objects.OptCritical | objects.OptUnknown | objects.OptRecovery,
		NotificationInterval: dynamicServiceNotificationInterval(hostname, servicename),
		ContactGroups:        d.defaultContactGroups(),
		Dynamic:              true,
		LastSeen:             time.Now(),
		ShouldBeScheduled:    false,
		CurrentState:         4, // pending
		StateType:            objects.StateTypeHard,
	}
	d.store.AddService(svc)
	host.Services = append(host.Services, svc)
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

// Stop signals the pruner goroutine to exit.
func (d *DynamicTracker) Stop() {
	close(d.stopCh)
}
