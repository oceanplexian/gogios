package checker

import (
	"testing"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestParseCheckOutput_ShortOnly(t *testing.T) {
	p := ParseCheckOutput("OK - everything is fine")
	if p.ShortOutput != "OK - everything is fine" {
		t.Errorf("got short=%q", p.ShortOutput)
	}
	if p.LongOutput != "" {
		t.Errorf("got long=%q", p.LongOutput)
	}
	if p.PerfData != "" {
		t.Errorf("got perf=%q", p.PerfData)
	}
}

func TestParseCheckOutput_ShortWithPerfdata(t *testing.T) {
	p := ParseCheckOutput("OK - load | load1=0.5;5;10;0")
	if p.ShortOutput != "OK - load" {
		t.Errorf("got short=%q", p.ShortOutput)
	}
	if p.PerfData != "load1=0.5;5;10;0" {
		t.Errorf("got perf=%q", p.PerfData)
	}
}

func TestParseCheckOutput_FullFormat(t *testing.T) {
	raw := "CRITICAL - disk full | disk=95%;80;90;0;100\nPartition /var is 95% full\nConsider cleanup\n| inode=5000;10000;20000"
	p := ParseCheckOutput(raw)
	if p.ShortOutput != "CRITICAL - disk full" {
		t.Errorf("got short=%q", p.ShortOutput)
	}
	if p.LongOutput != "Partition /var is 95% full\\nConsider cleanup" {
		t.Errorf("got long=%q", p.LongOutput)
	}
	if p.PerfData != "disk=95%;80;90;0;100 inode=5000;10000;20000" {
		t.Errorf("got perf=%q", p.PerfData)
	}
}

func TestParseCheckOutput_SemicolonReplacement(t *testing.T) {
	p := ParseCheckOutput("WARN; check output; more | perf=1;2;3")
	if p.ShortOutput != "WARN: check output: more" {
		t.Errorf("semicolons not replaced in short output: %q", p.ShortOutput)
	}
	// Perfdata should keep semicolons
	if p.PerfData != "perf=1;2;3" {
		t.Errorf("perfdata should keep semicolons: %q", p.PerfData)
	}
}

func TestGetServiceCheckReturnCode(t *testing.T) {
	tests := []struct {
		rc       int
		timeout  bool
		exitedOK bool
		want     int
	}{
		{0, false, true, objects.ServiceOK},
		{1, false, true, objects.ServiceWarning},
		{2, false, true, objects.ServiceCritical},
		{3, false, true, objects.ServiceUnknown},
		{126, false, true, objects.ServiceCritical},
		{127, false, true, objects.ServiceCritical},
		{255, false, true, objects.ServiceCritical},
		{0, true, true, objects.ServiceCritical}, // timeout -> critical (default)
		{0, false, false, objects.ServiceCritical},
	}

	for _, tt := range tests {
		cr := &objects.CheckResult{
			ReturnCode:   tt.rc,
			EarlyTimeout: tt.timeout,
			ExitedOK:     tt.exitedOK,
		}
		got := GetServiceCheckReturnCode(cr, objects.ServiceCritical)
		if got != tt.want {
			t.Errorf("rc=%d timeout=%v exited=%v: got %d want %d",
				tt.rc, tt.timeout, tt.exitedOK, got, tt.want)
		}
	}
}

func TestGetHostCheckReturnCode(t *testing.T) {
	tests := []struct {
		rc         int
		aggressive bool
		want       int
	}{
		{0, false, objects.HostUp},
		{1, false, objects.HostUp},
		{1, true, objects.HostDown},
		{2, false, objects.HostDown},
		{3, false, objects.HostDown},
	}

	for _, tt := range tests {
		cr := &objects.CheckResult{ReturnCode: tt.rc, ExitedOK: true}
		got := GetHostCheckReturnCode(cr, tt.aggressive)
		if got != tt.want {
			t.Errorf("rc=%d aggressive=%v: got %d want %d", tt.rc, tt.aggressive, got, tt.want)
		}
	}
}
