package objects

import "testing"

func TestNotificationTypeName(t *testing.T) {
	tests := []struct {
		name   string
		ntype  int
		state  int
		isHost bool
		want   string
	}{
		{"Acknowledgement", NotificationAcknowledgement, 0, true, "ACKNOWLEDGEMENT"},
		{"FlappingStart", NotificationFlappingStart, 0, true, "FLAPPINGSTART"},
		{"FlappingStop", NotificationFlappingStop, 0, true, "FLAPPINGSTOP"},
		{"FlappingDisabled", NotificationFlappingDisabled, 0, true, "FLAPPINGDISABLED"},
		{"DowntimeStart", NotificationDowntimeStart, 0, true, "DOWNTIMESTART"},
		{"DowntimeEnd", NotificationDowntimeEnd, 0, true, "DOWNTIMEEND"},
		{"DowntimeCancelled", NotificationDowntimeCancelled, 0, true, "DOWNTIMECANCELLED"},
		{"Custom", NotificationCustom, 0, true, "CUSTOM"},
		{"HostRecovery", NotificationNormal, HostUp, true, "RECOVERY"},
		{"HostProblem", NotificationNormal, HostDown, true, "PROBLEM"},
		{"ServiceRecovery", NotificationNormal, ServiceOK, false, "RECOVERY"},
		{"ServiceProblem", NotificationNormal, ServiceCritical, false, "PROBLEM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NotificationTypeName(tt.ntype, tt.state, tt.isHost)
			if got != tt.want {
				t.Errorf("NotificationTypeName(%d, %d, %v) = %q, want %q",
					tt.ntype, tt.state, tt.isHost, got, tt.want)
			}
		})
	}
}

func TestHostStateName(t *testing.T) {
	tests := []struct {
		state int
		want  string
	}{
		{HostUp, "UP"},
		{HostDown, "DOWN"},
		{HostUnreachable, "UNREACHABLE"},
		{99, "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := HostStateName(tt.state); got != tt.want {
				t.Errorf("HostStateName(%d) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestServiceStateName(t *testing.T) {
	tests := []struct {
		state int
		want  string
	}{
		{ServiceOK, "OK"},
		{ServiceWarning, "WARNING"},
		{ServiceCritical, "CRITICAL"},
		{ServiceUnknown, "UNKNOWN"},
		{99, "UNKNOWN"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := ServiceStateName(tt.state); got != tt.want {
				t.Errorf("ServiceStateName(%d) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}

func TestStateTypeName(t *testing.T) {
	if got := StateTypeName(StateTypeHard); got != "HARD" {
		t.Errorf("expected HARD, got %q", got)
	}
	if got := StateTypeName(StateTypeSoft); got != "SOFT" {
		t.Errorf("expected SOFT, got %q", got)
	}
}

func TestStateMatchesHostOptions(t *testing.T) {
	tests := []struct {
		name  string
		state int
		opts  uint32
		want  bool
	}{
		{"Down+OptDown", HostDown, OptDown, true},
		{"Down+OptUnreachable", HostDown, OptUnreachable, false},
		{"Unreachable+OptUnreachable", HostUnreachable, OptUnreachable, true},
		{"Up+OptRecovery", HostUp, OptRecovery, true},
		{"Up+OptDown", HostUp, OptDown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StateMatchesHostOptions(tt.state, tt.opts); got != tt.want {
				t.Errorf("StateMatchesHostOptions(%d, %d) = %v, want %v",
					tt.state, tt.opts, got, tt.want)
			}
		})
	}
}

func TestStateMatchesSvcOptions(t *testing.T) {
	tests := []struct {
		name  string
		state int
		opts  uint32
		want  bool
	}{
		{"Warning+OptWarning", ServiceWarning, OptWarning, true},
		{"Warning+OptCritical", ServiceWarning, OptCritical, false},
		{"Critical+OptCritical", ServiceCritical, OptCritical, true},
		{"Unknown+OptUnknown", ServiceUnknown, OptUnknown, true},
		{"OK+OptRecovery", ServiceOK, OptRecovery, true},
		{"OK+OptWarning", ServiceOK, OptWarning, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StateMatchesSvcOptions(tt.state, tt.opts); got != tt.want {
				t.Errorf("StateMatchesSvcOptions(%d, %d) = %v, want %v",
					tt.state, tt.opts, got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.IntervalLength != 60 {
		t.Errorf("IntervalLength: got %d, want 60", cfg.IntervalLength)
	}
	if cfg.ServiceCheckTimeout != 60 {
		t.Errorf("ServiceCheckTimeout: got %d, want 60", cfg.ServiceCheckTimeout)
	}
	if cfg.HostCheckTimeout != 30 {
		t.Errorf("HostCheckTimeout: got %d, want 30", cfg.HostCheckTimeout)
	}
	if cfg.MaxServiceCheckSpread != 30 {
		t.Errorf("MaxServiceCheckSpread: got %d, want 30", cfg.MaxServiceCheckSpread)
	}
	if !cfg.ExecuteServiceChecks {
		t.Error("ExecuteServiceChecks: expected true")
	}
	if !cfg.ExecuteHostChecks {
		t.Error("ExecuteHostChecks: expected true")
	}
	if cfg.ServiceCheckTimeoutState != ServiceCritical {
		t.Errorf("ServiceCheckTimeoutState: got %d, want %d", cfg.ServiceCheckTimeoutState, ServiceCritical)
	}
}
