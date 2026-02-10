package perfdata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestExpandMacros_Basic(t *testing.T) {
	got := expandMacros("Hello $NAME$", map[string]string{"NAME": "World"})
	if got != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", got)
	}
}

func TestExpandMacros_Multiple(t *testing.T) {
	macros := map[string]string{"FIRST": "Hello", "SECOND": "World"}
	got := expandMacros("$FIRST$ $SECOND$", macros)
	if got != "Hello World" {
		t.Errorf("expected 'Hello World', got %q", got)
	}
}

func TestExpandMacros_NoMatch(t *testing.T) {
	got := expandMacros("$UNKNOWN$", map[string]string{"NAME": "x"})
	if got != "$UNKNOWN$" {
		t.Errorf("expected '$UNKNOWN$' unchanged, got %q", got)
	}
}

func TestExpandMacros_Empty(t *testing.T) {
	got := expandMacros("", map[string]string{"NAME": "x"})
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestHostMacros(t *testing.T) {
	h := &objects.Host{
		Name:         "myhost",
		Alias:        "My Host",
		Address:      "10.0.0.1",
		PluginOutput: "OK - up",
		PerfData:     "rta=1.00ms",
		CheckCommand: &objects.Command{Name: "check_ping"},
	}
	macros := hostMacros(h)

	expectedKeys := []string{
		"HOSTNAME", "HOSTALIAS", "HOSTADDRESS", "HOSTSTATE",
		"HOSTSTATETYPE", "HOSTOUTPUT", "LONGHOSTOUTPUT",
		"HOSTPERFDATA", "HOSTCHECKCOMMAND",
	}
	for _, k := range expectedKeys {
		if _, ok := macros[k]; !ok {
			t.Errorf("missing expected key %q", k)
		}
	}
	if macros["HOSTNAME"] != "myhost" {
		t.Errorf("HOSTNAME: expected 'myhost', got %q", macros["HOSTNAME"])
	}
	if macros["HOSTPERFDATA"] != "rta=1.00ms" {
		t.Errorf("HOSTPERFDATA: expected 'rta=1.00ms', got %q", macros["HOSTPERFDATA"])
	}
}

func TestServiceMacros(t *testing.T) {
	h := &objects.Host{
		Name:    "myhost",
		Alias:   "My Host",
		Address: "10.0.0.1",
	}
	s := &objects.Service{
		Host:         h,
		Description:  "HTTP",
		PluginOutput: "OK - 200",
		PerfData:     "time=0.5s",
		CheckCommand: &objects.Command{Name: "check_http"},
	}
	macros := serviceMacros(s)

	expectedKeys := []string{
		"HOSTNAME", "HOSTALIAS", "HOSTADDRESS", "SERVICEDESC",
		"SERVICESTATE", "SERVICESTATETYPE", "SERVICEOUTPUT",
		"LONGSERVICEOUTPUT", "SERVICEPERFDATA", "SERVICECHECKCOMMAND",
	}
	for _, k := range expectedKeys {
		if _, ok := macros[k]; !ok {
			t.Errorf("missing expected key %q", k)
		}
	}
	if macros["HOSTNAME"] != "myhost" {
		t.Errorf("HOSTNAME: expected 'myhost', got %q", macros["HOSTNAME"])
	}
	if macros["SERVICEDESC"] != "HTTP" {
		t.Errorf("SERVICEDESC: expected 'HTTP', got %q", macros["SERVICEDESC"])
	}
}

func TestServiceMacros_NilHost(t *testing.T) {
	s := &objects.Service{
		Description: "HTTP",
	}
	macros := serviceMacros(s)
	if macros["HOSTNAME"] != "" {
		t.Errorf("expected empty HOSTNAME with nil host, got %q", macros["HOSTNAME"])
	}
}

func TestCmdStr_Nil(t *testing.T) {
	if got := cmdStr(nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestCmdStr_Valid(t *testing.T) {
	cmd := &objects.Command{Name: "check_ping"}
	if got := cmdStr(cmd); got != "check_ping" {
		t.Errorf("expected 'check_ping', got %q", got)
	}
}

func TestUpdateHostPerfdata_Disabled(t *testing.T) {
	gs := &objects.GlobalState{ProcessPerformanceData: false}
	p := NewProcessor(gs)
	h := &objects.Host{Name: "test", ProcessPerfData: true, PerfData: "rta=1ms"}
	// Should not panic or write anything
	p.UpdateHostPerfdata(h)
}

func TestUpdateServicePerfdata_Disabled(t *testing.T) {
	gs := &objects.GlobalState{ProcessPerformanceData: false}
	p := NewProcessor(gs)
	s := &objects.Service{Description: "HTTP", ProcessPerfData: true, PerfData: "time=1s"}
	// Should not panic or write anything
	p.UpdateServicePerfdata(s)
}

func TestOpenPerfdataFile_Append(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "host-perfdata.dat")
	f, err := openPerfdataFile(path, objects.PerfdataFileAppend)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.WriteString("test line\n")
	f.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist after write")
	}
}

func TestOpenPerfdataFile_Write(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "host-perfdata.dat")
	f, err := openPerfdataFile(path, objects.PerfdataFileWrite)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	f.Close()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to exist")
	}
}
