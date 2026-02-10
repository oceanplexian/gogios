package status

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/downtime"
	"github.com/oceanplexian/gogios/internal/objects"
)

func TestStatusWriter_Write(t *testing.T) {
	tmpDir := t.TempDir()
	statusPath := tmpDir + "/status.dat"

	store := objects.NewObjectStore()
	h := &objects.Host{
		Name:                 "host1",
		CurrentState:         objects.HostUp,
		StateType:            objects.StateTypeHard,
		HasBeenChecked:       true,
		NotificationsEnabled: true,
		ActiveChecksEnabled:  true,
		PassiveChecksEnabled: true,
		PluginOutput:         "OK - Host alive",
		LastCheck:            time.Now(),
	}
	store.AddHost(h)

	svc := &objects.Service{
		Host:                 h,
		Description:          "HTTP",
		CurrentState:         objects.ServiceOK,
		StateType:            objects.StateTypeHard,
		HasBeenChecked:       true,
		NotificationsEnabled: true,
		PluginOutput:         "HTTP OK",
	}
	store.AddService(svc)

	cm := downtime.NewCommentManager(1)
	dm := downtime.NewDowntimeManager(1, cm, store)

	gs := &objects.GlobalState{
		EnableNotifications:        true,
		ExecuteServiceChecks:       true,
		ExecuteHostChecks:          true,
		AcceptPassiveServiceChecks: true,
		AcceptPassiveHostChecks:    true,
		ProgramStart:               time.Now(),
		PID:                        1234,
		NextCommentID:              1,
		NextDowntimeID:             1,
	}

	sw := &StatusWriter{
		Path:      statusPath,
		Store:     store,
		Global:    gs,
		Comments:  cm,
		Downtimes: dm,
		Version:   "4.1.1-go",
	}

	if err := sw.Write(); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	data, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	content := string(data)

	// Check for required blocks
	for _, expected := range []string{
		"info {",
		"programstatus {",
		"hoststatus {",
		"servicestatus {",
		"host_name=host1",
		"service_description=HTTP",
		"plugin_output=OK - Host alive",
		"enable_notifications=1",
	} {
		if !strings.Contains(content, expected) {
			t.Errorf("status.dat missing expected string: %s", expected)
		}
	}
}

func TestRetentionWriter_WriteAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	retPath := tmpDir + "/retention.dat"

	store := objects.NewObjectStore()
	h := &objects.Host{
		Name:                 "host1",
		CurrentState:         objects.HostDown,
		LastState:            objects.HostDown,
		LastHardState:        objects.HostDown,
		StateType:            objects.StateTypeHard,
		CurrentAttempt:       3,
		HasBeenChecked:       true,
		NotificationsEnabled: true,
		PluginOutput:         "CRITICAL - Host unreachable",
		NotifiedOn:           objects.OptDown,
		ProblemAcknowledged:  true,
		AckType:              objects.AckSticky,
	}
	store.AddHost(h)

	contact := &objects.Contact{
		Name:                        "admin",
		HostNotificationsEnabled:    true,
		ServiceNotificationsEnabled: true,
	}
	store.AddContact(contact)

	cm := downtime.NewCommentManager(100)
	dm := downtime.NewDowntimeManager(200, cm, store)

	gs := &objects.GlobalState{
		EnableNotifications:        true,
		NextCommentID:              100,
		NextDowntimeID:             200,
		NextNotificationID:         50,
	}

	rw := &RetentionWriter{
		Path:      retPath,
		Store:     store,
		Global:    gs,
		Comments:  cm,
		Downtimes: dm,
		Version:   "4.1.1-go",
	}

	if err := rw.Write(); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Reset state
	h.CurrentState = objects.HostUp
	h.NotifiedOn = 0
	h.ProblemAcknowledged = false
	h.AckType = objects.AckNone

	// Read it back
	store2 := objects.NewObjectStore()
	h2 := &objects.Host{Name: "host1"}
	store2.AddHost(h2)
	contact2 := &objects.Contact{Name: "admin"}
	store2.AddContact(contact2)

	cm2 := downtime.NewCommentManager(1)
	dm2 := downtime.NewDowntimeManager(1, cm2, store2)
	gs2 := &objects.GlobalState{}

	rr := &RetentionReader{
		Store:     store2,
		Global:    gs2,
		Comments:  cm2,
		Downtimes: dm2,
	}

	if err := rr.Read(retPath); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if h2.CurrentState != objects.HostDown {
		t.Errorf("expected host state DOWN, got %d", h2.CurrentState)
	}
	if h2.NotifiedOn&objects.OptDown == 0 {
		t.Error("expected NotifiedOn to include DOWN")
	}
	if !h2.ProblemAcknowledged {
		t.Error("expected problem to be acknowledged")
	}
	if h2.AckType != objects.AckSticky {
		t.Errorf("expected sticky ack, got %d", h2.AckType)
	}
	if gs2.NextNotificationID != 50 {
		t.Errorf("expected next_notification_id=50, got %d", gs2.NextNotificationID)
	}
}
