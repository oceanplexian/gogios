package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTemplates(t *testing.T) {
	parser := NewObjectParser()
	if err := parser.ParseFile(testConfigPath("templates.cfg")); err != nil {
		t.Fatal(err)
	}
	if err := ResolveTemplates(parser); err != nil {
		t.Fatalf("ResolveTemplates failed: %v", err)
	}

	// linux-server should have inherited from generic-host
	linux := parser.GetTemplate("host", "linux-server")
	if linux == nil {
		t.Fatal("linux-server not found")
	}
	// check_command should be inherited from generic-host
	cmd, _ := linux.Get("check_command")
	if cmd != "check-host-alive" {
		t.Errorf("expected check_command=check-host-alive, got %q", cmd)
	}
	// check_interval should be linux-server's own value (5), not overridden
	ci, _ := linux.Get("check_interval")
	if ci != "5" {
		t.Errorf("expected check_interval=5, got %q", ci)
	}

	// critical-server inherits from linux-server which inherits from generic-host
	critical := parser.GetTemplate("host", "critical-server")
	if critical == nil {
		t.Fatal("critical-server not found")
	}
	// Should have check_command from generic-host (through linux-server)
	cmd, _ = critical.Get("check_command")
	if cmd != "check-host-alive" {
		t.Errorf("critical-server: expected check_command=check-host-alive, got %q", cmd)
	}
	// check_period from linux-server
	cp, _ := critical.Get("check_period")
	if cp != "24x7" {
		t.Errorf("critical-server: expected check_period=24x7, got %q", cp)
	}
}

func TestAdditiveInheritance(t *testing.T) {
	dir := t.TempDir()
	content := `define host {
    name            base-host
    register        0
    contacts        admin
}
define host {
    name            child-host
    use             base-host
    register        0
    contacts        +extra-admin
}
`
	path := filepath.Join(dir, "test.cfg")
	os.WriteFile(path, []byte(content), 0644)

	parser := NewObjectParser()
	if err := parser.ParseFile(path); err != nil {
		t.Fatal(err)
	}
	if err := ResolveTemplates(parser); err != nil {
		t.Fatal(err)
	}

	child := parser.GetTemplate("host", "child-host")
	contacts, _ := child.Get("contacts")
	if contacts != "admin,extra-admin" {
		t.Errorf("expected contacts='admin,extra-admin', got %q", contacts)
	}
}

func TestCircularTemplateDetection(t *testing.T) {
	dir := t.TempDir()
	content := `define host {
    name    a
    use     b
    register 0
}
define host {
    name    b
    use     a
    register 0
}
`
	path := filepath.Join(dir, "test.cfg")
	os.WriteFile(path, []byte(content), 0644)

	parser := NewObjectParser()
	if err := parser.ParseFile(path); err != nil {
		t.Fatal(err)
	}
	err := ResolveTemplates(parser)
	if err == nil {
		t.Error("expected circular template error")
	}
}

func TestNullValueClearing(t *testing.T) {
	dir := t.TempDir()
	content := `define host {
    name            base
    register        0
    notes           Some notes
    notes_url       http://example.com
}
define host {
    host_name       test-host
    alias           Test
    address         10.0.0.1
    use             base
    notes           null
}
`
	path := filepath.Join(dir, "test.cfg")
	os.WriteFile(path, []byte(content), 0644)

	parser := NewObjectParser()
	if err := parser.ParseFile(path); err != nil {
		t.Fatal(err)
	}
	if err := ResolveTemplates(parser); err != nil {
		t.Fatal(err)
	}

	var host *TemplateObject
	for _, obj := range parser.Objects {
		if name, _ := obj.Get("host_name"); name == "test-host" {
			host = obj
			break
		}
	}
	if host == nil {
		t.Fatal("test-host not found")
	}
	// notes should be "null" (cleared)
	notes, _ := host.Get("notes")
	if notes != "null" {
		t.Errorf("expected notes='null', got %q", notes)
	}
	// notes_url should be inherited from base
	url, _ := host.Get("notes_url")
	if url != "http://example.com" {
		t.Errorf("expected notes_url='http://example.com', got %q", url)
	}
}
