package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadResourceFile(t *testing.T) {
	var macros [MaxUserMacros]string
	err := ReadResourceFile(testConfigPath("resource.cfg"), &macros)
	if err != nil {
		t.Fatalf("ReadResourceFile failed: %v", err)
	}
	if macros[0] != "/bin" {
		t.Errorf("expected USER1=/bin, got %q", macros[0])
	}
	if macros[1] != "public" {
		t.Errorf("expected USER2=public, got %q", macros[1])
	}
	if macros[2] != "dbuser" {
		t.Errorf("expected USER3=dbuser, got %q", macros[2])
	}
	if macros[8] != "test-api-token-12345" {
		t.Errorf("expected USER9=test-api-token-12345, got %q", macros[8])
	}
	if macros[9] != "spare_value_10" {
		t.Errorf("expected USER10=spare_value_10, got %q", macros[9])
	}
	if macros[31] != "max_user_macro" {
		t.Errorf("expected USER32=max_user_macro, got %q", macros[31])
	}
}

func TestReadResourceFileInvalid(t *testing.T) {
	dir := t.TempDir()
	content := "$USER0$=invalid\n"
	path := filepath.Join(dir, "resource.cfg")
	os.WriteFile(path, []byte(content), 0644)
	var macros [MaxUserMacros]string
	err := ReadResourceFile(path, &macros)
	if err == nil {
		t.Error("expected error for USER0, got nil")
	}
}
