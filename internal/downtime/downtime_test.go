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
