package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type MainConfig struct {
	// File paths
	LogFile              string
	CfgFiles             []string
	CfgDirs              []string
	ResourceFiles        []string
	StatusFile           string
	StateRetentionFile   string
	ObjectCacheFile      string
	PrecachedObjectFile  string
	TempFile             string
	TempPath             string
	CheckResultPath      string
	LockFile             string
	LogArchivePath       string
	CommandFile          string
	DebugFile            string

	// Permissions
	NagiosUser  string
	NagiosGroup string

	// Logging
	UseSyslog           bool
	LogNotifications    bool
	LogServiceRetries   bool
	LogHostRetries      bool
	LogEventHandlers    bool
	LogExternalCommands bool
	LogPassiveChecks    bool
	LogInitialStates    bool
	LogCurrentStates    bool
	LogRotationMethod   byte   // n/h/d/w/m
	MaxLogFileSize      uint64 // bytes; 0=unlimited (default 100MB)
	DebugLevel          int
	DebugVerbosity      int
	MaxDebugFileSize    uint64

	// Check execution
	ServiceCheckTimeout      int
	ServiceCheckTimeoutState byte // o/w/c/u
	HostCheckTimeout         int
	EventHandlerTimeout      int
	NotificationTimeout      int
	OCSPTimeout              int
	OCHPTimeout              int
	PerfdataTimeout          int
	MaxConcurrentChecks      int
	MaxCheckResultFileAge    uint64
	CheckWorkers             int

	// Scheduling
	IntervalLength                int
	ServiceInterCheckDelayMethod  string
	HostInterCheckDelayMethod     string
	ServiceInterleaveFactor       string
	MaxServiceCheckSpread         int
	MaxHostCheckSpread            int
	CheckResultReaperFrequency    int
	MaxCheckResultReaperTime      int
	AutoRescheduleChecks          bool
	AutoReschedulingInterval      int
	AutoReschedulingWindow        int

	// State management
	RetainStateInformation                bool
	RetentionUpdateInterval               int
	UseRetainedProgramState               bool
	UseRetainedSchedulingInfo             bool
	RetentionSchedulingHorizon            int
	StatusUpdateInterval                  int
	AdditionalFreshnessLatency            int
	RetainedHostAttributeMask             uint64
	RetainedServiceAttributeMask          uint64
	RetainedProcessHostAttributeMask      uint64
	RetainedProcessServiceAttributeMask   uint64
	RetainedContactHostAttributeMask      uint64
	RetainedContactServiceAttributeMask   uint64

	// Feature toggles
	ExecuteServiceChecks                      bool
	AcceptPassiveServiceChecks                bool
	ExecuteHostChecks                         bool
	AcceptPassiveHostChecks                   bool
	EnableEventHandlers                       bool
	EnableNotifications                       bool
	EnableFlapDetection                       bool
	ProcessPerformanceData                    bool
	ObsessOverServices                        bool
	ObsessOverHosts                           bool
	CheckForOrphanedServices                  bool
	CheckForOrphanedHosts                     bool
	CheckServiceFreshness                     bool
	CheckHostFreshness                        bool
	CheckExternalCommands                     bool
	CheckForUpdates                           bool
	BareUpdateCheck                           bool

	// Freshness
	ServiceFreshnessCheckInterval int
	HostFreshnessCheckInterval    int

	// Flap detection
	LowServiceFlapThreshold  float64
	HighServiceFlapThreshold float64
	LowHostFlapThreshold     float64
	HighHostFlapThreshold    float64

	// Host checking
	UseAggressiveHostChecking              bool
	CachedHostCheckHorizon                 uint64
	CachedServiceCheckHorizon              uint64
	EnablePredictiveHostDependencyChecks   bool
	EnablePredictiveServiceDependencyChecks bool
	SoftStateDependencies                  bool
	TranslatePassiveHostChecks             bool
	PassiveHostChecksAreSoft               bool

	// Commands
	GlobalHostEventHandler    string
	GlobalServiceEventHandler string
	OCSPCommand               string
	OCHPCommand               string

	// Performance data
	HostPerfdataCommand                  string
	ServicePerfdataCommand               string
	HostPerfdataFile                     string
	ServicePerfdataFile                  string
	HostPerfdataFileTemplate             string
	ServicePerfdataFileTemplate          string
	HostPerfdataFileMode                 byte
	ServicePerfdataFileMode              byte
	HostPerfdataFileProcessingInterval   uint64
	ServicePerfdataFileProcessingInterval uint64
	HostPerfdataFileProcessingCommand    string
	ServicePerfdataFileProcessingCommand string
	HostPerfdataProcessEmptyResults      bool
	ServicePerfdataProcessEmptyResults   bool

	// Misc
	DateFormat                    string
	UseTimezone                   string
	IllegalObjectNameChars        string
	IllegalMacroOutputChars       string
	UseRegexpMatching             bool
	UseTrueRegexpMatching         bool
	AdminEmail                    string
	AdminPager                    string
	EventBrokerOptions            int
	BrokerModules                 []string
	DaemonDumpsCore               bool
	UseLargeInstallationTweaks    bool
	EnableEnvironmentMacros       bool
	FreeChildProcessMemory        int
	ChildProcessesForkTwice       int
	AllowEmptyHostgroupAssignment bool
	HostDownDisableServiceChecks  uint64
	TimeChangeThreshold           int
	LoadctlOptions                string
	QuerySocket                   string
	LivestatusTCP                 string

	// NRDP Relay (Gogios extension)
	NRDPListen         string // listen address, e.g. ":5668"
	NRDPPath           string // URL path, default "/nrdp/"
	NRDPTokenHash      string // bcrypt hash of accepted token
	NRDPDynamicEnabled bool   // auto-register hosts/services from NRDP submissions
	NRDPDynamicTTL     int    // seconds before stale dynamic objects are pruned (default 86400)
	NRDPDynamicPrune   int    // seconds between prune runs (default 600)
	NRDPSSLCert        string // TLS certificate file
	NRDPSSLKey         string // TLS key file

	// For resolving relative paths
	basedir string
}

func NewMainConfig() *MainConfig {
	return &MainConfig{
		// Defaults matching Nagios
		UseSyslog:           true,
		LogNotifications:    true,
		LogServiceRetries:   true,
		LogHostRetries:      true,
		LogEventHandlers:    true,
		LogExternalCommands: true,
		LogPassiveChecks:    true,
		LogInitialStates:    false,
		LogCurrentStates:    true,
		LogRotationMethod:   'd',
		MaxLogFileSize:      100 * 1024 * 1024, // 100MB
		ServiceCheckTimeout: 60,
		HostCheckTimeout:    30,
		EventHandlerTimeout: 30,
		NotificationTimeout: 30,
		OCSPTimeout:         15,
		OCHPTimeout:         15,
		IntervalLength:      60,
		ServiceInterCheckDelayMethod: "s",
		HostInterCheckDelayMethod:    "s",
		ServiceInterleaveFactor:      "s",
		MaxServiceCheckSpread:        30,
		MaxHostCheckSpread:           30,
		CheckResultReaperFrequency:   10,
		MaxCheckResultReaperTime:     30,
		RetainStateInformation:       true,
		RetentionUpdateInterval:      60,
		UseRetainedProgramState:      true,
		StatusUpdateInterval:         10,
		RetentionSchedulingHorizon:   900,
		AdditionalFreshnessLatency:   15,
		ExecuteServiceChecks:         true,
		AcceptPassiveServiceChecks:   true,
		ExecuteHostChecks:            true,
		AcceptPassiveHostChecks:      true,
		EnableEventHandlers:          true,
		EnableNotifications:          true,
		CheckForOrphanedServices:     true,
		CheckForOrphanedHosts:        true,
		CheckExternalCommands:        true,
		CheckForUpdates:              true,
		ServiceFreshnessCheckInterval: 60,
		HostFreshnessCheckInterval:    60,
		LowServiceFlapThreshold:      25.0,
		HighServiceFlapThreshold:     50.0,
		LowHostFlapThreshold:         25.0,
		HighHostFlapThreshold:        50.0,
		CachedHostCheckHorizon:       15,
		CachedServiceCheckHorizon:    15,
		EnablePredictiveHostDependencyChecks:   true,
		EnablePredictiveServiceDependencyChecks: true,
		DateFormat:            "us",
		EnableEnvironmentMacros: true,
		FreeChildProcessMemory:  -1,
		ChildProcessesForkTwice: -1,
		TimeChangeThreshold:     900,
		HostPerfdataFileMode:    'a',
		ServicePerfdataFileMode: 'a',
		NRDPPath:                "/nrdp/",
		NRDPDynamicTTL:          86400,
		NRDPDynamicPrune:        600,
	}
}

func ReadMainConfig(path string) (*MainConfig, error) {
	cfg := NewMainConfig()
	cfg.basedir = filepath.Dir(path)

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open main config: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || line[0] == '#' || line[0] == 0 {
			continue
		}
		// Strip inline comments starting with ;
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
			if line == "" {
				continue
			}
		}
		eqIdx := strings.IndexByte(line, '=')
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])

		if err := cfg.setDirective(key, val); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNum, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return cfg, nil
}

func (c *MainConfig) resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.basedir, p)
}

func (c *MainConfig) setDirective(key, val string) error {
	switch key {
	// Paths (multi-value)
	case "cfg_file":
		c.CfgFiles = append(c.CfgFiles, c.resolvePath(val))
	case "cfg_dir":
		c.CfgDirs = append(c.CfgDirs, c.resolvePath(val))
	case "resource_file":
		c.ResourceFiles = append(c.ResourceFiles, c.resolvePath(val))
	case "broker_module":
		c.BrokerModules = append(c.BrokerModules, val)

	// Paths (single)
	case "log_file":
		c.LogFile = c.resolvePath(val)
	case "status_file":
		c.StatusFile = c.resolvePath(val)
	case "state_retention_file":
		c.StateRetentionFile = c.resolvePath(val)
	case "object_cache_file":
		c.ObjectCacheFile = c.resolvePath(val)
	case "precached_object_file":
		c.PrecachedObjectFile = c.resolvePath(val)
	case "temp_file":
		c.TempFile = c.resolvePath(val)
	case "temp_path":
		c.TempPath = c.resolvePath(val)
	case "check_result_path":
		c.CheckResultPath = c.resolvePath(val)
	case "lock_file":
		c.LockFile = c.resolvePath(val)
	case "log_archive_path":
		c.LogArchivePath = c.resolvePath(val)
	case "command_file":
		c.CommandFile = c.resolvePath(val)
	case "debug_file":
		c.DebugFile = c.resolvePath(val)
	case "host_perfdata_file":
		c.HostPerfdataFile = c.resolvePath(val)
	case "service_perfdata_file":
		c.ServicePerfdataFile = c.resolvePath(val)
	case "query_socket":
		c.QuerySocket = c.resolvePath(val)
	case "livestatus_tcp":
		c.LivestatusTCP = val

	// NRDP
	case "nrdp_listen":
		c.NRDPListen = val
	case "nrdp_path":
		c.NRDPPath = val
	case "nrdp_token_hash":
		c.NRDPTokenHash = val
	case "nrdp_dynamic_enabled":
		c.NRDPDynamicEnabled = val == "1"
	case "nrdp_dynamic_ttl":
		return setInt(&c.NRDPDynamicTTL, val)
	case "nrdp_dynamic_prune_interval":
		return setInt(&c.NRDPDynamicPrune, val)
	case "nrdp_ssl_cert":
		c.NRDPSSLCert = c.resolvePath(val)
	case "nrdp_ssl_key":
		c.NRDPSSLKey = c.resolvePath(val)

	// Permissions
	case "nagios_user":
		c.NagiosUser = val
	case "nagios_group":
		c.NagiosGroup = val

	// Strings
	case "global_host_event_handler":
		c.GlobalHostEventHandler = val
	case "global_service_event_handler":
		c.GlobalServiceEventHandler = val
	case "ocsp_command":
		c.OCSPCommand = val
	case "ochp_command":
		c.OCHPCommand = val
	case "host_perfdata_command":
		c.HostPerfdataCommand = val
	case "service_perfdata_command":
		c.ServicePerfdataCommand = val
	case "host_perfdata_file_template":
		c.HostPerfdataFileTemplate = val
	case "service_perfdata_file_template":
		c.ServicePerfdataFileTemplate = val
	case "host_perfdata_file_processing_command":
		c.HostPerfdataFileProcessingCommand = val
	case "service_perfdata_file_processing_command":
		c.ServicePerfdataFileProcessingCommand = val
	case "date_format":
		c.DateFormat = val
	case "use_timezone":
		c.UseTimezone = val
	case "illegal_object_name_chars":
		c.IllegalObjectNameChars = val
	case "illegal_macro_output_chars":
		c.IllegalMacroOutputChars = val
	case "admin_email":
		c.AdminEmail = val
	case "admin_pager":
		c.AdminPager = val
	case "service_inter_check_delay_method":
		c.ServiceInterCheckDelayMethod = val
	case "host_inter_check_delay_method":
		c.HostInterCheckDelayMethod = val
	case "service_interleave_factor":
		c.ServiceInterleaveFactor = val
	case "loadctl_options":
		c.LoadctlOptions = val

	// Booleans
	case "use_syslog":
		c.UseSyslog = val == "1"
	case "log_notifications":
		c.LogNotifications = val == "1"
	case "log_service_retries":
		c.LogServiceRetries = val == "1"
	case "log_host_retries":
		c.LogHostRetries = val == "1"
	case "log_event_handlers":
		c.LogEventHandlers = val == "1"
	case "log_external_commands":
		c.LogExternalCommands = val == "1"
	case "log_passive_checks":
		c.LogPassiveChecks = val == "1"
	case "log_initial_states":
		c.LogInitialStates = val == "1"
	case "log_current_states":
		c.LogCurrentStates = val == "1"
	case "retain_state_information":
		c.RetainStateInformation = val == "1"
	case "use_retained_program_state":
		c.UseRetainedProgramState = val == "1"
	case "use_retained_scheduling_info":
		c.UseRetainedSchedulingInfo = val == "1"
	case "execute_service_checks":
		c.ExecuteServiceChecks = val == "1"
	case "accept_passive_service_checks":
		c.AcceptPassiveServiceChecks = val == "1"
	case "execute_host_checks":
		c.ExecuteHostChecks = val == "1"
	case "accept_passive_host_checks":
		c.AcceptPassiveHostChecks = val == "1"
	case "enable_event_handlers":
		c.EnableEventHandlers = val == "1"
	case "enable_notifications":
		c.EnableNotifications = val == "1"
	case "enable_flap_detection":
		c.EnableFlapDetection = val == "1"
	case "process_performance_data":
		c.ProcessPerformanceData = val == "1"
	case "obsess_over_services":
		c.ObsessOverServices = val == "1"
	case "obsess_over_hosts":
		c.ObsessOverHosts = val == "1"
	case "check_for_orphaned_services":
		c.CheckForOrphanedServices = val == "1"
	case "check_for_orphaned_hosts":
		c.CheckForOrphanedHosts = val == "1"
	case "check_service_freshness":
		c.CheckServiceFreshness = val == "1"
	case "check_host_freshness":
		c.CheckHostFreshness = val == "1"
	case "check_external_commands":
		c.CheckExternalCommands = val == "1"
	case "check_for_updates":
		c.CheckForUpdates = val == "1"
	case "bare_update_check":
		c.BareUpdateCheck = val == "1"
	case "auto_reschedule_checks":
		c.AutoRescheduleChecks = val == "1"
	case "use_aggressive_host_checking":
		c.UseAggressiveHostChecking = val == "1"
	case "soft_state_dependencies":
		c.SoftStateDependencies = val == "1"
	case "translate_passive_host_checks":
		c.TranslatePassiveHostChecks = val == "1"
	case "passive_host_checks_are_soft":
		c.PassiveHostChecksAreSoft = val == "1"
	case "use_regexp_matching":
		c.UseRegexpMatching = val == "1"
	case "use_true_regexp_matching":
		c.UseTrueRegexpMatching = val == "1"
	case "daemon_dumps_core":
		c.DaemonDumpsCore = val == "1"
	case "use_large_installation_tweaks":
		c.UseLargeInstallationTweaks = val == "1"
	case "enable_environment_macros":
		c.EnableEnvironmentMacros = val == "1"
	case "enable_predictive_host_dependency_checks":
		c.EnablePredictiveHostDependencyChecks = val == "1"
	case "enable_predictive_service_dependency_checks":
		c.EnablePredictiveServiceDependencyChecks = val == "1"
	case "allow_empty_hostgroup_assignment":
		c.AllowEmptyHostgroupAssignment = val == "1"
	case "host_perfdata_process_empty_results":
		c.HostPerfdataProcessEmptyResults = val == "1"
	case "service_perfdata_process_empty_results":
		c.ServicePerfdataProcessEmptyResults = val == "1"

	// Ints
	case "service_check_timeout":
		return setInt(&c.ServiceCheckTimeout, val)
	case "host_check_timeout":
		return setInt(&c.HostCheckTimeout, val)
	case "event_handler_timeout":
		return setInt(&c.EventHandlerTimeout, val)
	case "notification_timeout":
		return setInt(&c.NotificationTimeout, val)
	case "ocsp_timeout":
		return setInt(&c.OCSPTimeout, val)
	case "ochp_timeout":
		return setInt(&c.OCHPTimeout, val)
	case "perfdata_timeout":
		return setInt(&c.PerfdataTimeout, val)
	case "max_concurrent_checks":
		return setInt(&c.MaxConcurrentChecks, val)
	case "check_workers":
		return setInt(&c.CheckWorkers, val)
	case "interval_length":
		return setInt(&c.IntervalLength, val)
	case "max_service_check_spread":
		return setInt(&c.MaxServiceCheckSpread, val)
	case "max_host_check_spread":
		return setInt(&c.MaxHostCheckSpread, val)
	case "check_result_reaper_frequency":
		return setInt(&c.CheckResultReaperFrequency, val)
	case "max_check_result_reaper_time":
		return setInt(&c.MaxCheckResultReaperTime, val)
	case "auto_rescheduling_interval":
		return setInt(&c.AutoReschedulingInterval, val)
	case "auto_rescheduling_window":
		return setInt(&c.AutoReschedulingWindow, val)
	case "retention_update_interval":
		return setInt(&c.RetentionUpdateInterval, val)
	case "retention_scheduling_horizon":
		return setInt(&c.RetentionSchedulingHorizon, val)
	case "status_update_interval":
		return setInt(&c.StatusUpdateInterval, val)
	case "additional_freshness_latency":
		return setInt(&c.AdditionalFreshnessLatency, val)
	case "service_freshness_check_interval":
		return setInt(&c.ServiceFreshnessCheckInterval, val)
	case "host_freshness_check_interval":
		return setInt(&c.HostFreshnessCheckInterval, val)
	case "debug_level":
		return setInt(&c.DebugLevel, val)
	case "debug_verbosity":
		return setInt(&c.DebugVerbosity, val)
	case "event_broker_options":
		return setInt(&c.EventBrokerOptions, val)
	case "free_child_process_memory":
		return setInt(&c.FreeChildProcessMemory, val)
	case "child_processes_fork_twice":
		return setInt(&c.ChildProcessesForkTwice, val)
	case "time_change_threshold":
		return setInt(&c.TimeChangeThreshold, val)

	// Unsigned ints
	case "max_debug_file_size":
		return setUint64(&c.MaxDebugFileSize, val)
	case "max_log_file_size":
		return setUint64(&c.MaxLogFileSize, val)
	case "max_check_result_file_age":
		return setUint64(&c.MaxCheckResultFileAge, val)
	case "cached_host_check_horizon":
		return setUint64(&c.CachedHostCheckHorizon, val)
	case "cached_service_check_horizon":
		return setUint64(&c.CachedServiceCheckHorizon, val)
	case "retained_host_attribute_mask":
		return setUint64(&c.RetainedHostAttributeMask, val)
	case "retained_service_attribute_mask":
		return setUint64(&c.RetainedServiceAttributeMask, val)
	case "retained_process_host_attribute_mask":
		return setUint64(&c.RetainedProcessHostAttributeMask, val)
	case "retained_process_service_attribute_mask":
		return setUint64(&c.RetainedProcessServiceAttributeMask, val)
	case "retained_contact_host_attribute_mask":
		return setUint64(&c.RetainedContactHostAttributeMask, val)
	case "retained_contact_service_attribute_mask":
		return setUint64(&c.RetainedContactServiceAttributeMask, val)
	case "host_perfdata_file_processing_interval":
		return setUint64(&c.HostPerfdataFileProcessingInterval, val)
	case "service_perfdata_file_processing_interval":
		return setUint64(&c.ServicePerfdataFileProcessingInterval, val)
	case "host_down_disable_service_checks":
		return setUint64(&c.HostDownDisableServiceChecks, val)

	// Floats
	case "low_service_flap_threshold":
		return setFloat64(&c.LowServiceFlapThreshold, val)
	case "high_service_flap_threshold":
		return setFloat64(&c.HighServiceFlapThreshold, val)
	case "low_host_flap_threshold":
		return setFloat64(&c.LowHostFlapThreshold, val)
	case "high_host_flap_threshold":
		return setFloat64(&c.HighHostFlapThreshold, val)

	// Char
	case "log_rotation_method":
		if len(val) > 0 {
			c.LogRotationMethod = val[0]
		}
	case "service_check_timeout_state":
		if len(val) > 0 {
			c.ServiceCheckTimeoutState = val[0]
		}
	case "host_perfdata_file_mode":
		if len(val) > 0 {
			c.HostPerfdataFileMode = val[0]
		}
	case "service_perfdata_file_mode":
		if len(val) > 0 {
			c.ServicePerfdataFileMode = val[0]
		}
	}
	return nil
}

func setInt(dst *int, val string) error {
	v, err := strconv.Atoi(val)
	if err != nil {
		return fmt.Errorf("invalid integer %q: %w", val, err)
	}
	*dst = v
	return nil
}

func setUint64(dst *uint64, val string) error {
	v, err := strconv.ParseUint(val, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid unsigned integer %q: %w", val, err)
	}
	*dst = v
	return nil
}

func setFloat64(dst *float64, val string) error {
	v, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fmt.Errorf("invalid float %q: %w", val, err)
	}
	*dst = v
	return nil
}
