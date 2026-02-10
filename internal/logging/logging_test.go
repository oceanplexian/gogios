package logging

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestLogger_Log(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/test.log"

	gs := &objects.GlobalState{
		LogNotifications:    true,
		LogServiceRetries:   true,
		LogEventHandlers:    true,
		LogExternalCommands: true,
		LogPassiveChecks:    true,
		LogInitialStates:    true,
	}

	l, err := NewLogger(logPath, tmpDir, objects.LogRotationNone, false, gs)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	defer l.Close()

	l.Log("Test message %d", 42)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Test message 42") {
		t.Errorf("expected 'Test message 42' in log, got: %s", content)
	}
	if !strings.HasPrefix(content, "[") {
		t.Error("expected timestamp prefix")
	}
}

func TestLogger_ServiceAlert(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/test.log"

	l, err := NewLogger(logPath, tmpDir, objects.LogRotationNone, false, &objects.GlobalState{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.LogServiceAlert("host1", "HTTP", objects.ServiceCritical, objects.StateTypeHard, 3, "Connection refused")

	data, _ := os.ReadFile(logPath)
	content := string(data)
	if !strings.Contains(content, "SERVICE ALERT: host1;HTTP;CRITICAL;HARD;3;Connection refused") {
		t.Errorf("unexpected log content: %s", content)
	}
}

func TestLogger_HostAlert(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/test.log"

	l, err := NewLogger(logPath, tmpDir, objects.LogRotationNone, false, &objects.GlobalState{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.LogHostAlert("host1", objects.HostDown, objects.StateTypeHard, 3, "PING CRITICAL")

	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "HOST ALERT: host1;DOWN;HARD;3;PING CRITICAL") {
		t.Errorf("unexpected log: %s", string(data))
	}
}

func TestLogger_ConditionalLogging(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/test.log"

	gs := &objects.GlobalState{LogNotifications: false}
	l, err := NewLogger(logPath, tmpDir, objects.LogRotationNone, false, gs)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.LogServiceNotification("admin", "host1", "HTTP", "PROBLEM", "notify", "CRITICAL", "", "")

	data, _ := os.ReadFile(logPath)
	if strings.Contains(string(data), "NOTIFICATION") {
		t.Error("expected notification log to be suppressed")
	}
}

func TestLogger_NextRotationTime(t *testing.T) {
	logPath := "/dev/null"
	gs := &objects.GlobalState{}

	tests := []struct {
		method   int
		from     time.Time
		expected time.Time
	}{
		{
			objects.LogRotationHourly,
			time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
			time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC),
		},
		{
			objects.LogRotationDaily,
			time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
			time.Date(2024, 1, 16, 0, 0, 0, 0, time.UTC),
		},
		{
			objects.LogRotationMonthly,
			time.Date(2024, 1, 15, 14, 30, 0, 0, time.UTC),
			time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		l, err := NewLogger(logPath, "/tmp", tt.method, false, gs)
		if err != nil {
			t.Fatal(err)
		}
		got := l.NextRotationTime(tt.from)
		if !got.Equal(tt.expected) {
			t.Errorf("method %d: expected %v, got %v", tt.method, tt.expected, got)
		}
		l.Close()
	}
}

func TestLogger_Rotate(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := tmpDir + "/nagios.log"

	l, err := NewLogger(logPath, tmpDir, objects.LogRotationDaily, false, &objects.GlobalState{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	l.Log("Before rotation")

	if err := l.Rotate(); err != nil {
		t.Fatalf("Rotate: %v", err)
	}

	l.Log("After rotation")

	// Check new log has content
	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "After rotation") {
		t.Error("expected new log to contain 'After rotation'")
	}
	if strings.Contains(string(data), "Before rotation") {
		t.Error("expected 'Before rotation' to be in archive, not current log")
	}

	// Check archive exists
	entries, _ := os.ReadDir(tmpDir)
	foundArchive := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "nagios-") && strings.HasSuffix(e.Name(), ".log") {
			foundArchive = true
		}
	}
	if !foundArchive {
		t.Error("expected archive file to exist")
	}
}
