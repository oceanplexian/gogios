package config

import (
	"testing"
)

func TestLoadConfig(t *testing.T) {
	result, err := LoadConfig(testConfigPath("nagios.cfg"))
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	store := result.Store

	// Verify object counts
	if len(store.Commands) != 25 {
		t.Errorf("expected 25 commands, got %d", len(store.Commands))
	}
	if len(store.Timeperiods) != 9 {
		t.Errorf("expected 9 timeperiods, got %d", len(store.Timeperiods))
	}
	if len(store.Contacts) != 6 {
		t.Errorf("expected 6 contacts, got %d", len(store.Contacts))
	}
	if len(store.ContactGroups) != 5 {
		t.Errorf("expected 5 contact groups, got %d", len(store.ContactGroups))
	}
	if len(store.Hosts) != 13 {
		t.Errorf("expected 13 hosts, got %d", len(store.Hosts))
	}
	if len(store.HostGroups) != 9 {
		t.Errorf("expected 9 host groups, got %d", len(store.HostGroups))
	}
	if len(store.Services) != 38 {
		t.Errorf("expected 38 services, got %d", len(store.Services))
	}
	if len(store.ServiceGroups) != 7 {
		t.Errorf("expected 7 service groups, got %d", len(store.ServiceGroups))
	}

	// Verify specific host
	web01 := store.GetHost("web-01")
	if web01 == nil {
		t.Fatal("host web-01 not found")
	}
	if web01.Address != "10.0.1.10" {
		t.Errorf("web-01 address: expected 10.0.1.10, got %s", web01.Address)
	}
	if web01.MaxCheckAttempts != 5 {
		t.Errorf("web-01 max_check_attempts: expected 5, got %d", web01.MaxCheckAttempts)
	}
	if web01.CheckInterval != 5.0 {
		t.Errorf("web-01 check_interval: expected 5.0, got %f", web01.CheckInterval)
	}
	if web01.CheckCommand == nil {
		t.Error("web-01 missing check_command")
	} else if web01.CheckCommand.Name != "check-host-alive" {
		t.Errorf("web-01 check_command: expected check-host-alive, got %s", web01.CheckCommand.Name)
	}
	if web01.Notes != "Primary web server - Apache/Nginx" {
		t.Errorf("web-01 notes: expected 'Primary web server - Apache/Nginx', got %q", web01.Notes)
	}

	// Verify service duplication (SSH on web-01,web-02)
	sshWeb01 := store.GetService("web-01", "SSH")
	sshWeb02 := store.GetService("web-02", "SSH")
	if sshWeb01 == nil || sshWeb02 == nil {
		t.Error("SSH services on web-01/web-02 not found")
	}

	// Verify contact group resolution
	admins := store.GetContactGroup("admins")
	if admins == nil {
		t.Fatal("admins contactgroup not found")
	}
	if len(admins.Members) != 2 {
		t.Errorf("expected 2 members in admins, got %d", len(admins.Members))
	}

	// Verify contactgroup_members resolution
	everyone := store.GetContactGroup("everyone")
	if everyone == nil {
		t.Fatal("everyone contactgroup not found")
	}
	if len(everyone.Members) < 4 {
		t.Errorf("expected at least 4 members in everyone (from contactgroup_members), got %d", len(everyone.Members))
	}

	// Verify timeperiod exclusion
	sans := store.GetTimeperiod("24x7-sans-holidays")
	if sans == nil {
		t.Fatal("24x7-sans-holidays timeperiod not found")
	}
	if len(sans.Exclusions) != 1 {
		t.Errorf("expected 1 exclusion, got %d", len(sans.Exclusions))
	}
	if sans.Exclusions[0].Name != "us-holidays" {
		t.Errorf("expected exclusion us-holidays, got %s", sans.Exclusions[0].Name)
	}

	// Verify host group
	webServers := store.GetHostGroup("web-servers")
	if webServers == nil {
		t.Fatal("web-servers hostgroup not found")
	}
	if len(webServers.Members) != 2 {
		t.Errorf("expected 2 members in web-servers, got %d", len(webServers.Members))
	}

	// Verify service groups
	httpSvcs := store.GetServiceGroup("http-services")
	if httpSvcs == nil {
		t.Fatal("http-services servicegroup not found")
	}
	if len(httpSvcs.Members) != 3 {
		t.Errorf("expected 3 members in http-services, got %d", len(httpSvcs.Members))
	}

	// Verify host dependencies
	if len(store.HostDependencies) != 12 {
		t.Errorf("expected 12 host dependencies, got %d", len(store.HostDependencies))
	}

	// Verify service dependencies
	if len(store.ServiceDependencies) != 6 {
		t.Errorf("expected 6 service dependencies, got %d", len(store.ServiceDependencies))
	}

	// Verify user macros
	if result.UserMacros[0] != "/bin" {
		t.Errorf("expected USER1=/bin, got %q", result.UserMacros[0])
	}
}

func TestVerifyConfig(t *testing.T) {
	result, errs := VerifyConfig(testConfigPath("nagios.cfg"))
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("validation error: %v", e)
		}
		t.Fatalf("expected 0 errors, got %d", len(errs))
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestServiceInheritContactsFromHost(t *testing.T) {
	result, err := LoadConfig(testConfigPath("nagios.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	// mail-01 services should inherit contacts from host (which gets from template)
	smtp := result.Store.GetService("mail-01", "SMTP")
	if smtp == nil {
		t.Fatal("mail-01 SMTP not found")
	}
	if len(smtp.ContactGroups) == 0 && len(smtp.Contacts) == 0 {
		t.Error("mail-01 SMTP should have inherited contacts from host")
	}
}

func TestHostGroupBidirectionalRefs(t *testing.T) {
	result, err := LoadConfig(testConfigPath("nagios.cfg"))
	if err != nil {
		t.Fatal(err)
	}
	web01 := result.Store.GetHost("web-01")
	if web01 == nil {
		t.Fatal("web-01 not found")
	}
	if len(web01.HostGroups) == 0 {
		t.Error("web-01 should belong to at least one hostgroup")
	}
}
