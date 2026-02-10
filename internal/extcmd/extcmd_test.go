package extcmd

import (
	"testing"
)

func TestParse_Basic(t *testing.T) {
	cmd, err := Parse("[1609459200] ENABLE_NOTIFICATIONS")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Timestamp != 1609459200 {
		t.Errorf("expected timestamp 1609459200, got %d", cmd.Timestamp)
	}
	if cmd.Name != "ENABLE_NOTIFICATIONS" {
		t.Errorf("expected ENABLE_NOTIFICATIONS, got %s", cmd.Name)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("expected 0 args, got %d", len(cmd.Args))
	}
}

func TestParse_HostComment(t *testing.T) {
	cmd, err := Parse("[1609459200] ADD_HOST_COMMENT;myhost;1;admin;This is a test comment")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "ADD_HOST_COMMENT" {
		t.Errorf("expected ADD_HOST_COMMENT, got %s", cmd.Name)
	}
	if len(cmd.Args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(cmd.Args), cmd.Args)
	}
	if cmd.Args[0] != "myhost" {
		t.Errorf("expected myhost, got %s", cmd.Args[0])
	}
	if cmd.Args[3] != "This is a test comment" {
		t.Errorf("expected comment text, got %s", cmd.Args[3])
	}
}

func TestParse_AckWithSemicolonInComment(t *testing.T) {
	cmd, err := Parse("[1609459200] ACKNOWLEDGE_HOST_PROBLEM;myhost;2;1;1;admin;Problem noted; will fix later")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) != 6 {
		t.Fatalf("expected 6 args, got %d: %v", len(cmd.Args), cmd.Args)
	}
	// Last arg should contain the semicolons
	if cmd.Args[5] != "Problem noted; will fix later" {
		t.Errorf("expected 'Problem noted; will fix later', got '%s'", cmd.Args[5])
	}
}

func TestParse_ProcessServiceCheckResult(t *testing.T) {
	cmd, err := Parse("[1609459200] PROCESS_SERVICE_CHECK_RESULT;myhost;HTTP;2;CRITICAL - Connection refused")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "PROCESS_SERVICE_CHECK_RESULT" {
		t.Errorf("expected PROCESS_SERVICE_CHECK_RESULT, got %s", cmd.Name)
	}
	if len(cmd.Args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(cmd.Args))
	}
	if cmd.Args[2] != "2" {
		t.Errorf("expected return code 2, got %s", cmd.Args[2])
	}
}

func TestParse_InvalidFormat(t *testing.T) {
	_, err := Parse("no brackets here")
	if err == nil {
		t.Error("expected error for missing brackets")
	}

	_, err = Parse("")
	if err == nil {
		t.Error("expected error for empty string")
	}

	_, err = Parse("[abc] COMMAND")
	if err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

func TestParse_ScheduleDowntime(t *testing.T) {
	cmd, err := Parse("[1609459200] SCHEDULE_SVC_DOWNTIME;myhost;HTTP;1609459200;1609462800;1;0;3600;admin;Maintenance window")
	if err != nil {
		t.Fatal(err)
	}
	if len(cmd.Args) != 9 {
		t.Fatalf("expected 9 args, got %d: %v", len(cmd.Args), cmd.Args)
	}
	if cmd.Args[0] != "myhost" {
		t.Errorf("expected myhost, got %s", cmd.Args[0])
	}
	if cmd.Args[1] != "HTTP" {
		t.Errorf("expected HTTP, got %s", cmd.Args[1])
	}
}
