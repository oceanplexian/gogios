package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseObjectFile(t *testing.T) {
	parser := NewObjectParser()
	err := parser.ParseFile(testConfigPath("commands.cfg"))
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	// Count command objects
	count := 0
	for _, obj := range parser.Objects {
		if obj.Type == "command" {
			count++
		}
	}
	if count != 25 {
		t.Errorf("expected 25 commands, got %d", count)
	}
	// Check a specific command
	found := false
	for _, obj := range parser.Objects {
		if obj.Type == "command" {
			name, _ := obj.Get("command_name")
			if name == "check_ping" {
				found = true
				line, _ := obj.Get("command_line")
				if line == "" {
					t.Error("check_ping has empty command_line")
				}
			}
		}
	}
	if !found {
		t.Error("check_ping command not found")
	}
}

func TestParseObjectFileTemplates(t *testing.T) {
	parser := NewObjectParser()
	err := parser.ParseFile(testConfigPath("templates.cfg"))
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	// Check that generic-host template exists
	tmpl := parser.GetTemplate("host", "generic-host")
	if tmpl == nil {
		t.Fatal("generic-host template not found")
	}
	if tmpl.Register() {
		t.Error("generic-host should have register=0")
	}
	// Check linux-server inherits from generic-host
	linux := parser.GetTemplate("host", "linux-server")
	if linux == nil {
		t.Fatal("linux-server template not found")
	}
	use, _ := linux.Get("use")
	if use != "generic-host" {
		t.Errorf("expected linux-server use=generic-host, got %q", use)
	}
}

func TestParseDir(t *testing.T) {
	parser := NewObjectParser()
	err := parser.ParseDir(testConfigPath("extra"))
	if err != nil {
		t.Fatalf("ParseDir failed: %v", err)
	}
	if len(parser.Objects) != 2 {
		t.Errorf("expected 2 objects from extra dir, got %d", len(parser.Objects))
	}
}

func TestParseSemicolonComments(t *testing.T) {
	dir := t.TempDir()
	content := `define host {
    host_name       test-host   ; this is a comment
    alias           Test Host
    address         10.0.0.1
}
`
	path := filepath.Join(dir, "test.cfg")
	os.WriteFile(path, []byte(content), 0644)

	parser := NewObjectParser()
	if err := parser.ParseFile(path); err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(parser.Objects) != 1 {
		t.Fatalf("expected 1 object, got %d", len(parser.Objects))
	}
	name, _ := parser.Objects[0].Get("host_name")
	if name != "test-host" {
		t.Errorf("expected host_name=test-host, got %q", name)
	}
}

func TestParseCustomVars(t *testing.T) {
	dir := t.TempDir()
	content := `define host {
    host_name       test-host
    alias           Test
    address         10.0.0.1
    _CUSTOM_VAR     some_value
    _LOCATION       rack_42
}
`
	path := filepath.Join(dir, "test.cfg")
	os.WriteFile(path, []byte(content), 0644)

	parser := NewObjectParser()
	if err := parser.ParseFile(path); err != nil {
		t.Fatal(err)
	}
	obj := parser.Objects[0]
	if v := obj.CustomVars["CUSTOM_VAR"]; v != "some_value" {
		t.Errorf("expected CUSTOM_VAR=some_value, got %q", v)
	}
	if v := obj.CustomVars["LOCATION"]; v != "rack_42" {
		t.Errorf("expected LOCATION=rack_42, got %q", v)
	}
}

func TestAliasNormalization(t *testing.T) {
	tests := []struct {
		objType, key, expected string
	}{
		{"host", "obsess", "obsess_over_host"},
		{"service", "obsess", "obsess_over_service"},
		{"service", "importance", "hourly_value"},
		{"hostdependency", "master_host", "host_name"},
		{"servicedependency", "master_description", "service_description"},
	}
	for _, tt := range tests {
		got := normalizeAlias(tt.objType, tt.key)
		if got != tt.expected {
			t.Errorf("normalizeAlias(%s, %s) = %q, want %q", tt.objType, tt.key, got, tt.expected)
		}
	}
}

func TestNestedDefineError(t *testing.T) {
	dir := t.TempDir()
	content := `define host {
    host_name test
    define service {
    }
}
`
	path := filepath.Join(dir, "test.cfg")
	os.WriteFile(path, []byte(content), 0644)

	parser := NewObjectParser()
	err := parser.ParseFile(path)
	if err == nil {
		t.Error("expected error for nested define")
	}
}
