package macros

import (
	"testing"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestExpander_BasicMacros(t *testing.T) {
	cfg := objects.DefaultConfig()
	cfg.UserMacros[0] = "/usr/local/nagios/libexec"

	host := &objects.Host{
		Name:    "webserver1",
		Alias:   "Web Server 1",
		Address: "192.168.1.100",
	}
	svc := &objects.Service{
		Description:  "HTTP",
		CurrentState: objects.ServiceCritical,
	}

	e := &Expander{Cfg: cfg}
	result := e.Expand("$USER1$/check_http -H $HOSTADDRESS$ -p 80", host, svc, nil)
	expected := "/usr/local/nagios/libexec/check_http -H 192.168.1.100 -p 80"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExpander_ARGMacros(t *testing.T) {
	cfg := objects.DefaultConfig()
	e := &Expander{Cfg: cfg}

	host := &objects.Host{Name: "h1"}
	result := e.Expand("check_disk -w $ARG1$ -c $ARG2$ -p $ARG3$", host, nil, []string{"20%", "10%", "/"})
	expected := "check_disk -w 20% -c 10% -p /"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExpander_DollarEscape(t *testing.T) {
	cfg := objects.DefaultConfig()
	e := &Expander{Cfg: cfg}

	result := e.Expand("echo $$ money $$", nil, nil, nil)
	if result != "echo $ money $" {
		t.Errorf("got %q", result)
	}
}

func TestExpander_UnknownMacroLeftAsIs(t *testing.T) {
	cfg := objects.DefaultConfig()
	e := &Expander{Cfg: cfg}

	result := e.Expand("$NONEXISTENT$", nil, nil, nil)
	if result != "$NONEXISTENT$" {
		t.Errorf("unknown macro should be left as-is, got %q", result)
	}
}

func TestExpander_CustomVars(t *testing.T) {
	cfg := objects.DefaultConfig()
	e := &Expander{Cfg: cfg}

	host := &objects.Host{
		Name:       "h1",
		CustomVars: map[string]string{"SNMP_COMMUNITY": "public"},
	}

	result := e.Expand("check_snmp -C $_HOSTSNMP_COMMUNITY$", host, nil, nil)
	expected := "check_snmp -C public"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExpander_OnDemandHost(t *testing.T) {
	cfg := objects.DefaultConfig()
	router := &objects.Host{Name: "router1", CurrentState: objects.HostDown}

	e := &Expander{
		Cfg:        cfg,
		HostLookup: func(name string) *objects.Host {
			if name == "router1" {
				return router
			}
			return nil
		},
	}

	result := e.Expand("Router state is $HOSTSTATE:router1$", nil, nil, nil)
	expected := "Router state is DOWN"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestExpander_ServiceMacros(t *testing.T) {
	cfg := objects.DefaultConfig()
	e := &Expander{Cfg: cfg}

	svc := &objects.Service{
		Description:  "HTTP",
		CurrentState: objects.ServiceWarning,
		StateType:    objects.StateTypeHard,
	}

	result := e.Expand("$SERVICEDESC$ is $SERVICESTATE$ ($SERVICESTATETYPE$)", nil, svc, nil)
	expected := "HTTP is WARNING (HARD)"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestSplitCommandArgs(t *testing.T) {
	name, args := SplitCommandArgs("check_nrpe!check_disk!20%!10%")
	if name != "check_nrpe" {
		t.Errorf("got name %q", name)
	}
	if len(args) != 3 || args[0] != "check_disk" || args[1] != "20%" || args[2] != "10%" {
		t.Errorf("got args %v", args)
	}

	name, args = SplitCommandArgs("check_ping")
	if name != "check_ping" || args != nil {
		t.Errorf("got name=%q args=%v", name, args)
	}
}

func TestExpander_HostStateMacros(t *testing.T) {
	cfg := objects.DefaultConfig()
	e := &Expander{Cfg: cfg}

	host := &objects.Host{
		Name:         "h1",
		CurrentState: objects.HostUp,
		StateType:    objects.StateTypeHard,
	}

	tests := []struct {
		macro string
		want  string
	}{
		{"$HOSTNAME$", "h1"},
		{"$HOSTSTATE$", "UP"},
		{"$HOSTSTATEID$", "0"},
		{"$HOSTSTATETYPE$", "HARD"},
	}

	for _, tt := range tests {
		got := e.Expand(tt.macro, host, nil, nil)
		if got != tt.want {
			t.Errorf("Expand(%q) = %q, want %q", tt.macro, got, tt.want)
		}
	}
}
