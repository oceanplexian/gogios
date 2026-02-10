package livestatus

import (
	"testing"

	"github.com/oceanplexian/gogios/internal/api"
)

func TestHandleCommand_Basic(t *testing.T) {
	var gotName string
	var gotArgs []string
	sink := api.CommandSink(func(name string, args []string) {
		gotName = name
		gotArgs = args
	})
	handleCommand("COMMAND ENABLE_NOTIFICATIONS", sink)
	if gotName != "ENABLE_NOTIFICATIONS" {
		t.Errorf("name = %q, want %q", gotName, "ENABLE_NOTIFICATIONS")
	}
	if len(gotArgs) != 0 {
		t.Errorf("args = %v, want empty", gotArgs)
	}
}

func TestHandleCommand_WithTimestamp(t *testing.T) {
	var gotName string
	var gotArgs []string
	sink := api.CommandSink(func(name string, args []string) {
		gotName = name
		gotArgs = args
	})
	handleCommand("COMMAND [1234567890] SCHEDULE_FORCED_SVC_CHECK;web-01;HTTP;1234567890", sink)
	if gotName != "SCHEDULE_FORCED_SVC_CHECK" {
		t.Errorf("name = %q, want %q", gotName, "SCHEDULE_FORCED_SVC_CHECK")
	}
	if len(gotArgs) != 3 {
		t.Fatalf("args len = %d, want 3", len(gotArgs))
	}
	if gotArgs[0] != "web-01" || gotArgs[1] != "HTTP" || gotArgs[2] != "1234567890" {
		t.Errorf("args = %v, want [web-01 HTTP 1234567890]", gotArgs)
	}
}

func TestHandleCommand_WithArgs(t *testing.T) {
	var gotArgs []string
	sink := api.CommandSink(func(name string, args []string) {
		gotArgs = args
	})
	handleCommand("COMMAND DO_THING;a;b;c", sink)
	if len(gotArgs) != 3 || gotArgs[0] != "a" || gotArgs[1] != "b" || gotArgs[2] != "c" {
		t.Errorf("args = %v, want [a b c]", gotArgs)
	}
}

func TestHandleCommand_NilSink(t *testing.T) {
	// Should not panic
	handleCommand("COMMAND ENABLE_NOTIFICATIONS", nil)
}

func TestHandleCommand_NoArgs(t *testing.T) {
	var gotArgs []string
	sink := api.CommandSink(func(name string, args []string) {
		gotArgs = args
	})
	handleCommand("COMMAND SOME_CMD", sink)
	if len(gotArgs) != 0 {
		t.Errorf("args = %v, want empty", gotArgs)
	}
}
