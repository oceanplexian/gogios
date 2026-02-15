package livestatus

import (
	"net"
	"sync"
	"testing"
	"time"

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

func TestHandleConnection_BulkCommands(t *testing.T) {
	// Simulate Thruk sending multiple commands separated by \n\n on one connection.
	var mu sync.Mutex
	var commands []string
	sink := api.CommandSink(func(name string, args []string) {
		mu.Lock()
		commands = append(commands, name)
		mu.Unlock()
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &Server{
		quit:     make(chan struct{}),
		provider: &api.StateProvider{},
		cmdSink:  sink,
	}

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		srv.handleConnection(conn)
		close(done)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	// Send 3 commands separated by blank lines, like Thruk does
	bulk := "COMMAND [1234567890] ENABLE_SVC_NOTIFICATIONS;host1;svc1\n\n" +
		"COMMAND [1234567890] ENABLE_SVC_NOTIFICATIONS;host2;svc2\n\n" +
		"COMMAND [1234567890] ENABLE_SVC_NOTIFICATIONS;host3;svc3\n\n"
	conn.Write([]byte(bulk))
	conn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConnection did not finish in time")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(commands) != 3 {
		t.Errorf("got %d commands, want 3: %v", len(commands), commands)
	}
}

func TestHandleConnection_BatchDispatch(t *testing.T) {
	// Verify that bulk commands are dispatched through the batch sink
	// (single lock acquisition) rather than per-command dispatch.
	var batchCount int
	var totalCmds int
	batchSink := api.BatchCommandSink(func(cmds []api.CommandEntry) {
		batchCount++
		totalCmds += len(cmds)
	})
	// The per-command sink should NOT be called when batch sink is set.
	perCmdCalled := false
	sink := api.CommandSink(func(name string, args []string) {
		perCmdCalled = true
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &Server{
		quit:         make(chan struct{}),
		provider:     &api.StateProvider{},
		cmdSink:      sink,
		batchCmdSink: batchSink,
	}

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		srv.handleConnection(conn)
		close(done)
	}()

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	// Send 100 commands on one connection
	for i := 0; i < 100; i++ {
		conn.Write([]byte("COMMAND [1234567890] ENABLE_SVC_NOTIFICATIONS;host1;svc1\n\n"))
	}
	conn.Close()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleConnection did not finish in time")
	}

	if perCmdCalled {
		t.Error("per-command sink was called despite batch sink being set")
	}
	if totalCmds != 100 {
		t.Errorf("batch sink received %d commands, want 100", totalCmds)
	}
	if batchCount != 1 {
		t.Errorf("batch sink called %d times, want 1 (single batch)", batchCount)
	}
}

func TestParseCommandEntry(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantArgs []string
	}{
		{"COMMAND ENABLE_NOTIFICATIONS", "ENABLE_NOTIFICATIONS", nil},
		{"COMMAND [123] ENABLE_SVC_NOTIFICATIONS;host1;svc1", "ENABLE_SVC_NOTIFICATIONS", []string{"host1", "svc1"}},
		{"COMMAND DO_THING;a;b;c", "DO_THING", []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		entry := parseCommandEntry(tt.input)
		if entry == nil {
			t.Errorf("parseCommandEntry(%q) returned nil", tt.input)
			continue
		}
		if entry.Name != tt.wantName {
			t.Errorf("parseCommandEntry(%q).Name = %q, want %q", tt.input, entry.Name, tt.wantName)
		}
		if len(entry.Args) != len(tt.wantArgs) {
			t.Errorf("parseCommandEntry(%q).Args = %v, want %v", tt.input, entry.Args, tt.wantArgs)
		}
	}
}
