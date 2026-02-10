package objects

import "testing"

func TestObjectStoreDuplicateHost(t *testing.T) {
	store := NewObjectStore()
	h := &Host{Name: "test-host"}
	if err := store.AddHost(h); err != nil {
		t.Fatal(err)
	}
	if err := store.AddHost(h); err == nil {
		t.Error("expected duplicate host error")
	}
}

func TestObjectStoreGetHost(t *testing.T) {
	store := NewObjectStore()
	h := &Host{Name: "test-host", Address: "10.0.0.1"}
	store.AddHost(h)
	got := store.GetHost("test-host")
	if got == nil {
		t.Fatal("expected to find host")
	}
	if got.Address != "10.0.0.1" {
		t.Errorf("expected address 10.0.0.1, got %s", got.Address)
	}
	if store.GetHost("nonexistent") != nil {
		t.Error("expected nil for nonexistent host")
	}
}

func TestObjectStoreService(t *testing.T) {
	store := NewObjectStore()
	h := &Host{Name: "web-01"}
	store.AddHost(h)
	svc := &Service{Host: h, Description: "HTTP"}
	if err := store.AddService(svc); err != nil {
		t.Fatal(err)
	}
	got := store.GetService("web-01", "HTTP")
	if got == nil {
		t.Fatal("expected to find service")
	}
	if got.Description != "HTTP" {
		t.Errorf("expected HTTP, got %s", got.Description)
	}
	// Duplicate
	if err := store.AddService(svc); err == nil {
		t.Error("expected duplicate service error")
	}
}

func TestObjectStoreGetServicesForHost(t *testing.T) {
	store := NewObjectStore()
	h := &Host{Name: "web-01"}
	store.AddHost(h)
	store.AddService(&Service{Host: h, Description: "HTTP"})
	store.AddService(&Service{Host: h, Description: "SSH"})

	svcs := store.GetServicesForHost("web-01")
	if len(svcs) != 2 {
		t.Errorf("expected 2 services, got %d", len(svcs))
	}
}

func TestObjectStoreAllTypes(t *testing.T) {
	store := NewObjectStore()

	// Commands
	store.AddCommand(&Command{Name: "check_ping"})
	if store.GetCommand("check_ping") == nil {
		t.Error("command not found")
	}

	// Timeperiods
	store.AddTimeperiod(&Timeperiod{Name: "24x7"})
	if store.GetTimeperiod("24x7") == nil {
		t.Error("timeperiod not found")
	}

	// Contacts
	store.AddContact(&Contact{Name: "admin"})
	if store.GetContact("admin") == nil {
		t.Error("contact not found")
	}

	// Contact groups
	store.AddContactGroup(&ContactGroup{Name: "admins"})
	if store.GetContactGroup("admins") == nil {
		t.Error("contactgroup not found")
	}

	// Host groups
	store.AddHostGroup(&HostGroup{Name: "web-servers"})
	if store.GetHostGroup("web-servers") == nil {
		t.Error("hostgroup not found")
	}

	// Service groups
	store.AddServiceGroup(&ServiceGroup{Name: "http-services"})
	if store.GetServiceGroup("http-services") == nil {
		t.Error("servicegroup not found")
	}
}
