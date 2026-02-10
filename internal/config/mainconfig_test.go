package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadMainConfig(t *testing.T) {
	cfg, err := ReadMainConfig(testConfigPath("nagios.cfg"))
	if err != nil {
		t.Fatalf("ReadMainConfig failed: %v", err)
	}

	if cfg.LogRotationMethod != 'd' {
		t.Errorf("expected log_rotation_method='d', got '%c'", cfg.LogRotationMethod)
	}
	if cfg.IntervalLength != 60 {
		t.Errorf("expected interval_length=60, got %d", cfg.IntervalLength)
	}
	if cfg.EnableFlapDetection != true {
		t.Errorf("expected enable_flap_detection=true")
	}
	if cfg.LowServiceFlapThreshold != 5.0 {
		t.Errorf("expected low_service_flap_threshold=5.0, got %f", cfg.LowServiceFlapThreshold)
	}
	if cfg.AdminEmail != "admin@example.com" {
		t.Errorf("expected admin_email='admin@example.com', got '%s'", cfg.AdminEmail)
	}
	if cfg.DateFormat != "iso8601" {
		t.Errorf("expected date_format='iso8601', got '%s'", cfg.DateFormat)
	}
	if len(cfg.CfgFiles) != 10 {
		t.Errorf("expected 10 cfg_files, got %d", len(cfg.CfgFiles))
	}
	if len(cfg.CfgDirs) != 1 {
		t.Errorf("expected 1 cfg_dir, got %d", len(cfg.CfgDirs))
	}
	if cfg.ServiceCheckTimeoutState != 'c' {
		t.Errorf("expected service_check_timeout_state='c', got '%c'", cfg.ServiceCheckTimeoutState)
	}
	if !cfg.ProcessPerformanceData {
		t.Error("expected process_performance_data=true")
	}
	if cfg.GlobalHostEventHandler != "global-host-event-handler" {
		t.Errorf("expected global_host_event_handler='global-host-event-handler', got '%s'", cfg.GlobalHostEventHandler)
	}
}

func TestReadMainConfigRelativePaths(t *testing.T) {
	// Create a temp config with relative paths
	dir := t.TempDir()
	content := "log_file=var/nagios.log\ncfg_file=objects/hosts.cfg\n"
	cfgPath := filepath.Join(dir, "nagios.cfg")
	if err := os.WriteFile(cfgPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadMainConfig(cfgPath)
	if err != nil {
		t.Fatalf("ReadMainConfig failed: %v", err)
	}
	expected := filepath.Join(dir, "var/nagios.log")
	if cfg.LogFile != expected {
		t.Errorf("expected log_file=%q, got %q", expected, cfg.LogFile)
	}
}

func testConfigPath(name string) string {
	return filepath.Join(testConfigDir(), name)
}

func testConfigDir() string {
	return filepath.Join(projectRoot(), "test-configs")
}

func projectRoot() string {
	// Walk up from the test file location to find go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
}
