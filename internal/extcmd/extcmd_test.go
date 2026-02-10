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

func TestDispatch_RegisteredHandler(t *testing.T) {
	p := NewProcessor("/dev/null", 10)
	called := false
	var gotArgs []string
	p.RegisterHandler("TEST_CMD", func(cmd *Command) {
		called = true
		gotArgs = cmd.Args
	})
	p.Dispatch("TEST_CMD", []string{"arg1", "arg2"})
	if !called {
		t.Fatal("expected handler to be called")
	}
	if len(gotArgs) != 2 || gotArgs[0] != "arg1" || gotArgs[1] != "arg2" {
		t.Errorf("expected [arg1 arg2], got %v", gotArgs)
	}
}

func TestDispatch_UnregisteredHandler(t *testing.T) {
	p := NewProcessor("/dev/null", 10)
	// Should not panic
	p.Dispatch("NONEXISTENT", nil)
}

func TestRegisterHandlers_Bulk(t *testing.T) {
	p := NewProcessor("/dev/null", 10)
	results := map[string]bool{}
	handlers := map[string]Handler{
		"CMD_A": func(cmd *Command) { results["CMD_A"] = true },
		"CMD_B": func(cmd *Command) { results["CMD_B"] = true },
		"CMD_C": func(cmd *Command) { results["CMD_C"] = true },
	}
	p.RegisterHandlers(handlers)
	p.Dispatch("CMD_A", nil)
	p.Dispatch("CMD_B", nil)
	p.Dispatch("CMD_C", nil)
	for _, name := range []string{"CMD_A", "CMD_B", "CMD_C"} {
		if !results[name] {
			t.Errorf("handler %s was not called", name)
		}
	}
}

func TestParse_NoArgs(t *testing.T) {
	cmd, err := Parse("[1234567890] ENABLE_NOTIFICATIONS")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "ENABLE_NOTIFICATIONS" {
		t.Errorf("expected ENABLE_NOTIFICATIONS, got %s", cmd.Name)
	}
	if len(cmd.Args) != 0 {
		t.Errorf("expected no args, got %v", cmd.Args)
	}
}

func TestParse_EmptyLine(t *testing.T) {
	_, err := Parse("")
	if err == nil {
		t.Error("expected error for empty line")
	}
}

func TestParse_MissingOpenBracket(t *testing.T) {
	_, err := Parse("1234] ENABLE_NOTIFICATIONS")
	if err == nil {
		t.Error("expected error for missing open bracket")
	}
}

func TestParse_MissingCloseBracket(t *testing.T) {
	_, err := Parse("[1234 ENABLE_NOTIFICATIONS")
	if err == nil {
		t.Error("expected error for missing close bracket")
	}
}

func TestParse_InvalidTimestamp(t *testing.T) {
	_, err := Parse("[abc] ENABLE_NOTIFICATIONS")
	if err == nil {
		t.Error("expected error for invalid timestamp")
	}
}

func TestSplitArgs_KnownCommand(t *testing.T) {
	args := splitArgs("PROCESS_SERVICE_CHECK_RESULT", "host;svc;0;OK output with;semicolons")
	if len(args) != 4 {
		t.Fatalf("expected 4 args, got %d: %v", len(args), args)
	}
	if args[0] != "host" {
		t.Errorf("expected host, got %s", args[0])
	}
	if args[1] != "svc" {
		t.Errorf("expected svc, got %s", args[1])
	}
	if args[2] != "0" {
		t.Errorf("expected 0, got %s", args[2])
	}
	if args[3] != "OK output with;semicolons" {
		t.Errorf("expected 'OK output with;semicolons', got '%s'", args[3])
	}
}

func TestSplitArgs_UnknownCommand(t *testing.T) {
	args := splitArgs("UNKNOWN_CMD", "some;stuff")
	if len(args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %v", len(args), args)
	}
	if args[0] != "some;stuff" {
		t.Errorf("expected 'some;stuff', got '%s'", args[0])
	}
}

func TestSplitArgs_EmptyArgs(t *testing.T) {
	args := splitArgs("UNKNOWN_CMD", "")
	if args != nil {
		t.Errorf("expected nil, got %v", args)
	}
}

func TestExpectedArgCount_KnownCommands(t *testing.T) {
	tests := []struct {
		cmd   string
		count int
	}{
		{"ACKNOWLEDGE_HOST_PROBLEM", 6},
		{"ACKNOWLEDGE_SVC_PROBLEM", 7},
		{"SCHEDULE_HOST_DOWNTIME", 8},
		{"SCHEDULE_SVC_DOWNTIME", 9},
		{"PROCESS_SERVICE_CHECK_RESULT", 4},
		{"PROCESS_HOST_CHECK_RESULT", 3},
		{"ENABLE_HOST_NOTIFICATIONS", 1},
		{"ENABLE_SVC_CHECK", 2},
	}
	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			got := expectedArgCount(tt.cmd)
			if got != tt.count {
				t.Errorf("expectedArgCount(%s) = %d, want %d", tt.cmd, got, tt.count)
			}
		})
	}
}

func TestExpectedArgCount_UnknownCommand(t *testing.T) {
	got := expectedArgCount("TOTALLY_FAKE")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}
