package notify

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

type testLogger struct{ msgs []string }

func (l *testLogger) Log(format string, args ...interface{}) {}

func newTestEngine() *NotificationEngine {
	gs := &objects.GlobalState{
		EnableNotifications: true,
		IntervalLength:      60,
	}
	store := objects.NewObjectStore()
	return NewNotificationEngine(gs, store, &testLogger{})
}

func TestServiceNotification_ForcedBypassesAll(t *testing.T) {
	ne := newTestEngine()
	host := &objects.Host{Name: "h1", CurrentState: objects.HostUp}
	contact := &objects.Contact{
		Name:                        "admin",
		ServiceNotificationsEnabled: true,
		ServiceNotificationOptions:  objects.OptCritical | objects.OptRecovery,
		ServiceNotificationCommands: []*objects.Command{{Name: "notify", CommandLine: "true"}},
	}
	svc := &objects.Service{
		Host:                 host,
		Description:          "HTTP",
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeHard,
		NotificationsEnabled: false, // disabled - should be bypassed by force
		NotificationOptions:  objects.OptCritical,
		Contacts:             []*objects.Contact{contact},
	}

	// Without forced, should fail (notifications disabled)
	result := ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0)
	if result == 0 {
		t.Error("expected notification to be blocked when disabled")
	}

	// With forced, should pass
	result = ne.checkServiceNotificationViability(svc, objects.NotificationNormal, objects.NotificationOptionForced)
	if result != 0 {
		t.Error("expected forced notification to pass")
	}
}

func TestServiceNotification_GlobalDisabled(t *testing.T) {
	ne := newTestEngine()
	ne.GlobalState.EnableNotifications = false
	svc := &objects.Service{
		Host:                 &objects.Host{Name: "h1"},
		NotificationsEnabled: true,
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeHard,
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked when global notifications disabled")
	}
}

func TestServiceNotification_SoftStateBlocked(t *testing.T) {
	ne := newTestEngine()
	svc := &objects.Service{
		Host:                 &objects.Host{Name: "h1", CurrentState: objects.HostUp},
		NotificationsEnabled: true,
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeSoft, // soft state
		NotificationOptions:  objects.OptCritical,
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked for soft state")
	}
}

func TestServiceNotification_AcknowledgedBlocked(t *testing.T) {
	ne := newTestEngine()
	svc := &objects.Service{
		Host:                 &objects.Host{Name: "h1", CurrentState: objects.HostUp},
		NotificationsEnabled: true,
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeHard,
		NotificationOptions:  objects.OptCritical,
		ProblemAcknowledged:  true,
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked when problem acknowledged")
	}
}

func TestServiceNotification_RecoveryNoNotif(t *testing.T) {
	ne := newTestEngine()
	svc := &objects.Service{
		Host:                 &objects.Host{Name: "h1", CurrentState: objects.HostUp},
		NotificationsEnabled: true,
		CurrentState:         objects.ServiceOK,
		StateType:            objects.StateTypeHard,
		NotificationOptions:  objects.OptRecovery,
		NotifiedOn:           0, // no previous notification
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked for recovery with no prior notification")
	}
}

func TestServiceNotification_HostDown(t *testing.T) {
	ne := newTestEngine()
	svc := &objects.Service{
		Host:                 &objects.Host{Name: "h1", CurrentState: objects.HostDown},
		NotificationsEnabled: true,
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeHard,
		NotificationOptions:  objects.OptCritical,
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked when host is down")
	}
}

func TestServiceNotification_Downtime(t *testing.T) {
	ne := newTestEngine()
	svc := &objects.Service{
		Host:                   &objects.Host{Name: "h1", CurrentState: objects.HostUp},
		NotificationsEnabled:   true,
		CurrentState:           objects.ServiceCritical,
		StateType:              objects.StateTypeHard,
		NotificationOptions:    objects.OptCritical,
		ScheduledDowntimeDepth: 1,
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked during downtime")
	}
}

func TestServiceNotification_AckPassesUnlessOK(t *testing.T) {
	ne := newTestEngine()
	svc := &objects.Service{
		Host:                 &objects.Host{Name: "h1", CurrentState: objects.HostUp},
		NotificationsEnabled: true,
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeHard,
		NotificationOptions:  objects.OptCritical,
	}
	if ne.checkServiceNotificationViability(svc, objects.NotificationAcknowledgement, 0) != 0 {
		t.Error("expected ack notification to pass when service is critical")
	}

	svc.CurrentState = objects.ServiceOK
	if ne.checkServiceNotificationViability(svc, objects.NotificationAcknowledgement, 0) == 0 {
		t.Error("expected ack notification to fail when service is OK")
	}
}

func TestServiceNotification_NumberTracking(t *testing.T) {
	ne := newTestEngine()
	host := &objects.Host{Name: "h1", CurrentState: objects.HostUp}
	contact := &objects.Contact{
		Name:                        "admin",
		ServiceNotificationsEnabled: true,
		ServiceNotificationOptions:  objects.OptCritical | objects.OptRecovery,
		ServiceNotificationCommands: []*objects.Command{{Name: "notify", CommandLine: "true"}},
	}
	svc := &objects.Service{
		Host:                 host,
		Description:          "HTTP",
		CurrentState:         objects.ServiceCritical,
		StateType:            objects.StateTypeHard,
		NotificationsEnabled: true,
		NotificationOptions:  objects.OptCritical | objects.OptRecovery,
		NotificationInterval: 5,
		Contacts:             []*objects.Contact{contact},
	}

	ne.ServiceNotification(svc, objects.NotificationNormal, "", "", 0)
	if svc.CurrentNotificationNumber != 1 {
		t.Errorf("expected notification number 1, got %d", svc.CurrentNotificationNumber)
	}
	if svc.NotifiedOn&objects.OptCritical == 0 {
		t.Error("expected NotifiedOn to include critical")
	}
}

func TestHostNotification_BasicFlow(t *testing.T) {
	ne := newTestEngine()
	contact := &objects.Contact{
		Name:                       "admin",
		HostNotificationsEnabled:   true,
		HostNotificationOptions:    objects.OptDown | objects.OptRecovery,
		HostNotificationCommands:   []*objects.Command{{Name: "notify", CommandLine: "true"}},
	}
	hst := &objects.Host{
		Name:                 "h1",
		CurrentState:         objects.HostDown,
		StateType:            objects.StateTypeHard,
		NotificationsEnabled: true,
		NotificationOptions:  objects.OptDown | objects.OptRecovery,
		NotificationInterval: 5,
		Contacts:             []*objects.Contact{contact},
	}

	ne.HostNotification(hst, objects.NotificationNormal, "", "", 0)
	if hst.CurrentNotificationNumber != 1 {
		t.Errorf("expected notification number 1, got %d", hst.CurrentNotificationNumber)
	}
}

func TestEscalation_ValidRange(t *testing.T) {
	svc := &objects.Service{
		CurrentState:              objects.ServiceCritical,
		CurrentNotificationNumber: 3,
	}
	esc := &objects.ServiceEscalation{
		FirstNotification: 2,
		LastNotification:  5,
		EscalationOptions: objects.OptCritical,
	}
	if !IsValidServiceEscalation(svc, esc, 3, 0) {
		t.Error("expected escalation to be valid for notification 3 in range 2-5")
	}
}

func TestEscalation_OutOfRange(t *testing.T) {
	svc := &objects.Service{
		CurrentState:              objects.ServiceCritical,
		CurrentNotificationNumber: 1,
	}
	esc := &objects.ServiceEscalation{
		FirstNotification: 3,
		LastNotification:  5,
	}
	if IsValidServiceEscalation(svc, esc, 1, 0) {
		t.Error("expected escalation to be invalid for notification 1 in range 3-5")
	}
}

func TestEscalation_BroadcastOverride(t *testing.T) {
	svc := &objects.Service{
		CurrentState:              objects.ServiceCritical,
		CurrentNotificationNumber: 1,
	}
	esc := &objects.ServiceEscalation{
		FirstNotification: 3, // would normally exclude notif #1
	}
	if !IsValidServiceEscalation(svc, esc, 1, objects.NotificationOptionBroadcast) {
		t.Error("expected broadcast to override notification range")
	}
}

func TestGetNextServiceNotificationTime(t *testing.T) {
	svc := &objects.Service{
		NotificationInterval: 30, // 30 intervals = 30 * 60s = 1800s
	}
	now := time.Now()
	next := GetNextServiceNotificationTime(svc, now, 60)
	expected := now.Add(30 * 60 * time.Second)
	if !next.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, next)
	}
}

func TestGetNextServiceNotificationTime_EscalationShortest(t *testing.T) {
	svc := &objects.Service{
		NotificationInterval:      30,
		CurrentState:              objects.ServiceCritical,
		CurrentNotificationNumber: 3,
		Escalations: []*objects.ServiceEscalation{
			{
				FirstNotification:    2,
				LastNotification:     5,
				NotificationInterval: 10, // shorter
				EscalationOptions:    objects.OptCritical,
			},
		},
	}
	now := time.Now()
	next := GetNextServiceNotificationTime(svc, now, 60)
	expected := now.Add(10 * 60 * time.Second)
	if !next.Equal(expected) {
		t.Errorf("expected escalation interval 10*60, got %v vs %v", next, expected)
	}
}

func TestContactViability_DisabledContact(t *testing.T) {
	ne := newTestEngine()
	contact := &objects.Contact{
		Name:                        "admin",
		ServiceNotificationsEnabled: false,
	}
	svc := &objects.Service{
		Host:         &objects.Host{Name: "h1"},
		CurrentState: objects.ServiceCritical,
	}
	if ne.checkContactServiceViability(contact, svc, objects.NotificationNormal, 0) == 0 {
		t.Error("expected blocked when contact notifications disabled")
	}
}

func TestContactViability_ForcedBypass(t *testing.T) {
	ne := newTestEngine()
	contact := &objects.Contact{
		Name:                        "admin",
		ServiceNotificationsEnabled: false,
	}
	svc := &objects.Service{
		Host:         &objects.Host{Name: "h1"},
		CurrentState: objects.ServiceCritical,
	}
	if ne.checkContactServiceViability(contact, svc, objects.NotificationNormal, objects.NotificationOptionForced) != 0 {
		t.Error("expected forced to bypass contact filters")
	}
}

func TestCreateNotificationList_Dedup(t *testing.T) {
	ne := newTestEngine()
	contact := &objects.Contact{Name: "admin"}
	svc := &objects.Service{
		Contacts: []*objects.Contact{contact, contact}, // duplicate
	}
	list := ne.createServiceNotificationList(svc, 0)
	if len(list) != 1 {
		t.Errorf("expected 1 contact after dedup, got %d", len(list))
	}
}

func TestCreateNotificationList_Broadcast(t *testing.T) {
	ne := newTestEngine()
	normalContact := &objects.Contact{Name: "normal"}
	escContact := &objects.Contact{Name: "escalated"}
	svc := &objects.Service{
		CurrentState:              objects.ServiceCritical,
		CurrentNotificationNumber: 3,
		Contacts:                  []*objects.Contact{normalContact},
		Escalations: []*objects.ServiceEscalation{
			{
				FirstNotification: 2,
				LastNotification:  5,
				Contacts:          []*objects.Contact{escContact},
				EscalationOptions: objects.OptCritical,
			},
		},
	}
	list := ne.createServiceNotificationList(svc, objects.NotificationOptionBroadcast)
	if len(list) != 2 {
		t.Errorf("expected 2 contacts with broadcast, got %d", len(list))
	}
}
