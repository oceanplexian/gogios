package downtime

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

type mockLogger struct{}

func (m *mockLogger) Log(format string, args ...interface{}) {}

type mockNotifier struct {
	hostNotifs    int
	serviceNotifs int
}

func (m *mockNotifier) SendHostNotification(hostName string, ntype int, author, data string, options int) {
	m.hostNotifs++
}
func (m *mockNotifier) SendServiceNotification(hostName, svcDesc string, ntype int, author, data string, options int) {
	m.serviceNotifs++
}

func newTestSetup() (*DowntimeManager, *CommentManager, *objects.ObjectStore, *mockNotifier) {
	store := objects.NewObjectStore()
	store.AddHost(&objects.Host{Name: "host1"})
	cm := NewCommentManager(1)
	dm := NewDowntimeManager(1, cm, store)
	dm.SetLogger(&mockLogger{})
	notifier := &mockNotifier{}
	dm.SetNotifier(notifier)
	return dm, cm, store, notifier
}

func TestScheduleDowntime_FixedHost(t *testing.T) {
	dm, cm, store, notifier := newTestSetup()

	now := time.Now()
	d := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		Fixed:     true,
		Author:    "admin",
		Comment:   "Maintenance",
	}
	id := dm.Schedule(d)

	if id == 0 {
		t.Error("expected non-zero downtime ID")
	}
	if d.CommentID == 0 {
		t.Error("expected comment to be created")
	}
	if len(cm.All()) != 1 {
		t.Errorf("expected 1 comment, got %d", len(cm.All()))
	}

	// Start downtime
	dm.HandleStart(id)
	h := store.GetHost("host1")
	if h.ScheduledDowntimeDepth != 1 {
		t.Errorf("expected downtime depth 1, got %d", h.ScheduledDowntimeDepth)
	}
	if notifier.hostNotifs != 1 {
		t.Errorf("expected 1 host notification, got %d", notifier.hostNotifs)
	}

	// End downtime
	dm.HandleEnd(id)
	if h.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected downtime depth 0 after end, got %d", h.ScheduledDowntimeDepth)
	}
	if notifier.hostNotifs != 2 {
		t.Errorf("expected 2 host notifications, got %d", notifier.hostNotifs)
	}
}

func TestScheduleDowntime_Overlapping(t *testing.T) {
	dm, _, store, _ := newTestSetup()

	now := time.Now()
	d1 := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now,
		EndTime:   now.Add(2 * time.Hour),
		Fixed:     true,
	}
	d2 := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now.Add(time.Hour),
		EndTime:   now.Add(3 * time.Hour),
		Fixed:     true,
	}

	id1 := dm.Schedule(d1)
	id2 := dm.Schedule(d2)

	dm.HandleStart(id1)
	dm.HandleStart(id2)

	h := store.GetHost("host1")
	if h.ScheduledDowntimeDepth != 2 {
		t.Errorf("expected depth 2 with overlapping downtimes, got %d", h.ScheduledDowntimeDepth)
	}

	dm.HandleEnd(id1)
	if h.ScheduledDowntimeDepth != 1 {
		t.Errorf("expected depth 1 after ending first, got %d", h.ScheduledDowntimeDepth)
	}
}

func TestScheduleDowntime_Cancel(t *testing.T) {
	dm, _, store, notifier := newTestSetup()

	now := time.Now()
	d := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		Fixed:     true,
	}
	id := dm.Schedule(d)
	dm.HandleStart(id)

	dm.Unschedule(id)

	h := store.GetHost("host1")
	if h.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected depth 0 after cancel, got %d", h.ScheduledDowntimeDepth)
	}
	// Should have received CANCELLED notification
	if notifier.hostNotifs < 2 {
		t.Errorf("expected at least 2 notifications (start + cancel), got %d", notifier.hostNotifs)
	}
}

func TestScheduleDowntime_Triggered(t *testing.T) {
	dm, _, store, _ := newTestSetup()

	now := time.Now()
	parent := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		Fixed:     true,
	}
	parentID := dm.Schedule(parent)

	child := &Downtime{
		Type:        objects.HostDowntimeType,
		HostName:    "host1",
		StartTime:   now,
		EndTime:     now.Add(time.Hour),
		Fixed:       true,
		TriggeredBy: parentID,
	}
	dm.Schedule(child)

	// Starting parent should also start child
	dm.HandleStart(parentID)

	h := store.GetHost("host1")
	if h.ScheduledDowntimeDepth != 2 {
		t.Errorf("expected depth 2 (parent + triggered), got %d", h.ScheduledDowntimeDepth)
	}
}

func TestScheduleDowntime_FlexibleHost(t *testing.T) {
	dm, _, store, _ := newTestSetup()

	now := time.Now()
	d := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now.Add(-time.Minute),
		EndTime:   now.Add(time.Hour),
		Fixed:     false,
		Duration:  30 * time.Minute,
	}
	dm.Schedule(d)

	// Flex downtime should start when host goes down
	dm.CheckPendingFlexHostDowntime("host1", objects.HostDown)

	h := store.GetHost("host1")
	if h.ScheduledDowntimeDepth != 1 {
		t.Errorf("expected depth 1 after flex trigger, got %d", h.ScheduledDowntimeDepth)
	}
}

// TestCheckExpired_ActiveDowntimePastEndTime exercises the KANB-109 root
// cause: a fixed downtime that is in-effect and whose EndTime has already
// passed (e.g. because the process restarted and the goroutine HandleEnd
// timer was lost). CheckExpired must drop it AND decrement
// ScheduledDowntimeDepth back to 0. Before the fix, CheckExpired only
// touched non-in-effect downtimes, so the depth lingered forever.
func TestCheckExpired_ActiveDowntimePastEndTime(t *testing.T) {
	dm, _, store, _ := newTestSetup()

	now := time.Now()
	d := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now.Add(-2 * time.Hour),
		EndTime:   now.Add(-time.Hour), // ended an hour ago
		Fixed:     true,
	}
	id := dm.Schedule(d)
	// Pretend the start fired before the (now-past) EndTime.
	dm.HandleStart(id)

	h := store.GetHost("host1")
	if h.ScheduledDowntimeDepth != 1 {
		t.Fatalf("setup: expected depth 1 after HandleStart, got %d", h.ScheduledDowntimeDepth)
	}

	dm.CheckExpired()

	if h.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected depth 0 after CheckExpired of expired in-effect downtime, got %d", h.ScheduledDowntimeDepth)
	}
	if dm.Get(id) != nil {
		t.Errorf("expected expired downtime to be removed from manager")
	}
}

// TestCheckExpired_ServiceDowntimePastEndTime is the service-level twin of
// the host test. This is the exact case from the ticket: localhost / Total
// Processes carried scheduled_downtime_depth=1 with no matching downtime.
func TestCheckExpired_ServiceDowntimePastEndTime(t *testing.T) {
	dm, _, store, _ := newTestSetup()
	host := store.GetHost("host1")
	if err := store.AddService(&objects.Service{Host: host, Description: "Total Processes"}); err != nil {
		t.Fatalf("AddService: %v", err)
	}

	now := time.Now()
	d := &Downtime{
		Type:               objects.ServiceDowntimeType,
		HostName:           "host1",
		ServiceDescription: "Total Processes",
		StartTime:          now.Add(-time.Hour),
		EndTime:            now.Add(-30 * time.Minute),
		Fixed:              true,
	}
	id := dm.Schedule(d)
	dm.HandleStart(id)

	svc := store.GetService("host1", "Total Processes")
	if svc.ScheduledDowntimeDepth != 1 {
		t.Fatalf("setup: expected svc depth 1, got %d", svc.ScheduledDowntimeDepth)
	}

	dm.CheckExpired()

	if svc.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected svc depth 0 after expiry sweep, got %d", svc.ScheduledDowntimeDepth)
	}
	if dm.Get(id) != nil {
		t.Errorf("expected expired svc downtime to be removed")
	}
}

// TestCheckExpired_PendingFlexUntouchedDuringWindow ensures the sweep does
// NOT prematurely expire pending flex downtimes that haven't yet reached
// their EndTime — only those past it.
func TestCheckExpired_PendingFlexUntouchedDuringWindow(t *testing.T) {
	dm, _, _, _ := newTestSetup()

	now := time.Now()
	d := &Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now.Add(-time.Minute),
		EndTime:   now.Add(time.Hour),
		Fixed:     false,
		Duration:  30 * time.Minute,
	}
	id := dm.Schedule(d)

	dm.CheckExpired()

	if dm.Get(id) == nil {
		t.Error("CheckExpired should not have removed a pending downtime still in its window")
	}
}

// TestReconcileDepths_PhantomDepthCleared simulates exactly the KANB-109
// post-restart state: retention.dat set scheduled_downtime_depth=1 on a
// service, but there is no in-effect downtime backing it (the original
// downtime ended while gogios was stopped, then was dropped). Reconcile
// must reset the depth to 0.
func TestReconcileDepths_PhantomDepthCleared(t *testing.T) {
	dm, _, store, _ := newTestSetup()
	host := store.GetHost("host1")
	if err := store.AddService(&objects.Service{Host: host, Description: "Total Processes"}); err != nil {
		t.Fatalf("AddService: %v", err)
	}
	svc := store.GetService("host1", "Total Processes")
	// Simulate retention restoring a phantom depth with no matching downtime.
	svc.ScheduledDowntimeDepth = 1
	host.ScheduledDowntimeDepth = 1

	dm.ReconcileDepths()

	if host.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected host phantom depth cleared, got %d", host.ScheduledDowntimeDepth)
	}
	if svc.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected svc phantom depth cleared, got %d", svc.ScheduledDowntimeDepth)
	}
}

// TestReconcileDepths_PreservesLegitimateDepth ensures the reconciler does
// not zap a depth that IS backed by an in-effect downtime — and corrects
// under-counts too.
func TestReconcileDepths_PreservesLegitimateDepth(t *testing.T) {
	dm, _, store, _ := newTestSetup()
	host := store.GetHost("host1")
	if err := store.AddService(&objects.Service{Host: host, Description: "Disk"}); err != nil {
		t.Fatalf("AddService: %v", err)
	}
	now := time.Now()
	d := &Downtime{
		Type:               objects.ServiceDowntimeType,
		HostName:           "host1",
		ServiceDescription: "Disk",
		StartTime:          now,
		EndTime:            now.Add(time.Hour),
		Fixed:              true,
	}
	id := dm.Schedule(d)
	dm.HandleStart(id)

	svc := store.GetService("host1", "Disk")
	if svc.ScheduledDowntimeDepth != 1 {
		t.Fatalf("setup: expected svc depth 1, got %d", svc.ScheduledDowntimeDepth)
	}

	// Corrupt the depth (simulating retention drift) and let reconcile fix it.
	svc.ScheduledDowntimeDepth = 5
	dm.ReconcileDepths()

	if svc.ScheduledDowntimeDepth != 1 {
		t.Errorf("expected svc depth corrected to 1, got %d", svc.ScheduledDowntimeDepth)
	}
}

// TestRestartSurvival_RetentionExpiryReconcile is an end-to-end test of the
// KANB-109 fix: schedule a downtime, advance time past its EndTime, then
// run the sweep+reconcile sequence that fires after retention load on
// startup. Depth must end at 0.
func TestRestartSurvival_RetentionExpiryReconcile(t *testing.T) {
	dm, _, store, _ := newTestSetup()
	host := store.GetHost("host1")
	if err := store.AddService(&objects.Service{Host: host, Description: "Total Processes"}); err != nil {
		t.Fatalf("AddService: %v", err)
	}

	// Simulate retention restoring an in-effect downtime whose EndTime
	// already passed (the goroutine HandleEnd timer didn't survive restart).
	past := time.Now().Add(-24 * time.Hour)
	d := &Downtime{
		Type:               objects.ServiceDowntimeType,
		HostName:           "host1",
		ServiceDescription: "Total Processes",
		StartTime:          past.Add(-time.Hour),
		EndTime:            past, // 24h ago
		Fixed:              true,
		IsInEffect:         true,
		DowntimeID:         42,
	}
	dm.ScheduleWithID(d)
	// Retention also restored a phantom depth.
	svc := store.GetService("host1", "Total Processes")
	svc.ScheduledDowntimeDepth = 1

	// Boot sequence: sweep expired, then reconcile.
	dm.CheckExpired()
	dm.ReconcileDepths()

	if svc.ScheduledDowntimeDepth != 0 {
		t.Errorf("expected svc depth 0 after boot sweep, got %d", svc.ScheduledDowntimeDepth)
	}
	if dm.Get(42) != nil {
		t.Errorf("expected expired downtime removed after boot sweep")
	}
}

func TestScheduleDowntime_SortOrder(t *testing.T) {
	dm, _, _, _ := newTestSetup()

	now := time.Now()
	dm.Schedule(&Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now.Add(2 * time.Hour),
		EndTime:   now.Add(3 * time.Hour),
		Fixed:     true,
	})
	dm.Schedule(&Downtime{
		Type:      objects.HostDowntimeType,
		HostName:  "host1",
		StartTime: now,
		EndTime:   now.Add(time.Hour),
		Fixed:     true,
	})

	all := dm.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 downtimes, got %d", len(all))
	}
	if !all[0].StartTime.Before(all[1].StartTime) {
		t.Error("expected downtimes sorted by start time")
	}
}
