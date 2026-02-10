package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/api/livestatus"
	"github.com/oceanplexian/gogios/internal/checker"
	"github.com/oceanplexian/gogios/internal/config"
	"github.com/oceanplexian/gogios/internal/downtime"
	"github.com/oceanplexian/gogios/internal/extcmd"
	"github.com/oceanplexian/gogios/internal/logging"
	"github.com/oceanplexian/gogios/internal/macros"
	"github.com/oceanplexian/gogios/internal/notify"
	"github.com/oceanplexian/gogios/internal/objects"
	"github.com/oceanplexian/gogios/internal/scheduler"
	"github.com/oceanplexian/gogios/internal/status"
)

const version = "1.0.0"

func main() {
	// Nagios-compatible flags
	var verifyCount int
	var daemonMode, testScheduling, enableTimingPoint bool

	// Manual arg parsing to support -v -v (double verbose) like Nagios
	var configFile string
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-v", "--verify-config":
			verifyCount++
		case "-s", "--test-scheduling":
			testScheduling = true
		case "-d", "--daemon":
			daemonMode = true
		case "-T", "--enable-timing-point":
			enableTimingPoint = true
		case "-h", "--help":
			printUsage()
			os.Exit(0)
		case "-V", "--version":
			fmt.Printf("Gogios %s\n", version)
			os.Exit(0)
		default:
			if arg[0] == '-' {
				// Check for combined flags like -vv or -vvs
				if arg[1] != '-' {
					for _, ch := range arg[1:] {
						switch ch {
						case 'v':
							verifyCount++
						case 's':
							testScheduling = true
						case 'd':
							daemonMode = true
						case 'T':
							enableTimingPoint = true
						default:
							fmt.Fprintf(os.Stderr, "Unknown option: -%c\n", ch)
							printUsage()
							os.Exit(1)
						}
					}
				} else {
					fmt.Fprintf(os.Stderr, "Unknown option: %s\n", arg)
					printUsage()
					os.Exit(1)
				}
			} else {
				configFile = arg
			}
		}
	}

	if configFile == "" {
		printUsage()
		os.Exit(1)
	}

	if verifyCount > 0 {
		runVerify(configFile, verifyCount)
		return
	}

	if testScheduling {
		runSchedulingTest(configFile)
		return
	}

	_ = enableTimingPoint // reserved for future use

	runDaemon(configFile, daemonMode)
}

func printUsage() {
	fmt.Printf("\nGogios %s\n", version)
	fmt.Println("Copyright (c) 2024-present Gogios Contributors")
	fmt.Println("License: MIT")
	fmt.Println()
	fmt.Printf("Usage: %s [options] <main_config_file>\n", os.Args[0])
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println()
	fmt.Println("  -v, --verify-config          Verify all configuration data (-v -v for more info)")
	fmt.Println("  -s, --test-scheduling        Shows projected/recommended check scheduling and other")
	fmt.Println("                               diagnostic info based on the current configuration files.")
	fmt.Println("  -T, --enable-timing-point     Enable timed commentary on initialization")
	fmt.Println("  -d, --daemon                  Starts Gogios in daemon mode, instead of as a foreground process")
	fmt.Println("  -V, --version                 Print version information")
	fmt.Println("  -h, --help                    Print this help message")
	fmt.Println()
}

func runVerify(configFile string, verbosity int) {
	fmt.Printf("\nGogios %s\n", version)
	fmt.Println("Copyright (c) 2024-present Gogios Contributors")
	fmt.Print("License: MIT\n\n")
	fmt.Printf("Reading configuration data from %s...\n\n", configFile)

	result, errs := config.VerifyConfig(configFile)
	if len(errs) > 0 {
		fmt.Println()
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		}
		fmt.Printf("\nTotal Errors: %d\n", len(errs))
		os.Exit(1)
	}

	store := result.Store
	fmt.Println("Running pre-flight check on configuration data...")
	fmt.Println()

	if verbosity >= 2 {
		// -vv: print detailed object listing
		fmt.Println("Checking commands...")
		for _, c := range store.Commands {
			fmt.Printf("\tChecked command '%s'\n", c.Name)
		}
		fmt.Println("Checking contacts...")
		for _, c := range store.Contacts {
			fmt.Printf("\tChecked contact '%s'\n", c.Name)
		}
		fmt.Println("Checking contact groups...")
		for _, cg := range store.ContactGroups {
			fmt.Printf("\tChecked contact group '%s'\n", cg.Name)
		}
		fmt.Println("Checking hosts...")
		for _, h := range store.Hosts {
			fmt.Printf("\tChecked host '%s'\n", h.Name)
		}
		fmt.Println("Checking host groups...")
		for _, hg := range store.HostGroups {
			fmt.Printf("\tChecked host group '%s'\n", hg.Name)
		}
		fmt.Println("Checking services...")
		for _, svc := range store.Services {
			hostName := ""
			if svc.Host != nil {
				hostName = svc.Host.Name
			}
			fmt.Printf("\tChecked service '%s' on host '%s'\n", svc.Description, hostName)
		}
		fmt.Println("Checking service groups...")
		for _, sg := range store.ServiceGroups {
			fmt.Printf("\tChecked service group '%s'\n", sg.Name)
		}
		fmt.Println("Checking timeperiods...")
		for _, tp := range store.Timeperiods {
			fmt.Printf("\tChecked time period '%s'\n", tp.Name)
		}
		fmt.Println()
	}

	fmt.Printf("Checked %d commands.\n", len(store.Commands))
	fmt.Printf("Checked %d contacts.\n", len(store.Contacts))
	fmt.Printf("Checked %d contact groups.\n", len(store.ContactGroups))
	fmt.Printf("Checked %d hosts.\n", len(store.Hosts))
	fmt.Printf("Checked %d host groups.\n", len(store.HostGroups))
	fmt.Printf("Checked %d services.\n", len(store.Services))
	fmt.Printf("Checked %d service groups.\n", len(store.ServiceGroups))
	fmt.Printf("Checked %d timeperiods.\n", len(store.Timeperiods))
	fmt.Printf("Checked %d host dependencies.\n", len(store.HostDependencies))
	fmt.Printf("Checked %d service dependencies.\n", len(store.ServiceDependencies))
	fmt.Printf("Checked %d host escalations.\n", len(store.HostEscalations))
	fmt.Printf("Checked %d service escalations.\n", len(store.ServiceEscalations))
	fmt.Println()
	fmt.Println("Total Warnings: 0")
	fmt.Println("Total Errors:   0")
	fmt.Println()
	fmt.Println("Things look okay - No serious problems were detected during the pre-flight check")
	os.Exit(0)
}

func runSchedulingTest(configFile string) {
	fmt.Printf("\nGogios %s\n", version)
	fmt.Print("Copyright (c) 2024-present Gogios Contributors\n\n")

	result, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	store := result.Store
	mainCfg := result.MainCfg

	cfg := objects.DefaultConfig()
	cfg.IntervalLength = mainCfg.IntervalLength
	if cfg.IntervalLength <= 0 {
		cfg.IntervalLength = 60
	}
	cfg.MaxParallelServiceChecks = mainCfg.MaxConcurrentChecks
	cfg.MaxServiceCheckSpread = mainCfg.MaxServiceCheckSpread
	cfg.MaxHostCheckSpread = mainCfg.MaxHostCheckSpread

	totalServices := len(store.Services)
	totalHosts := len(store.Hosts)

	// Calculate ICD
	var serviceICD, hostICD float64
	if totalServices > 0 {
		avgInterval := 0.0
		for _, svc := range store.Services {
			avgInterval += svc.CheckInterval
		}
		avgInterval = avgInterval / float64(totalServices) * float64(cfg.IntervalLength)
		serviceICD = avgInterval / float64(totalServices)
	}
	if totalHosts > 0 {
		avgInterval := 0.0
		for _, h := range store.Hosts {
			avgInterval += h.CheckInterval
		}
		avgInterval = avgInterval / float64(totalHosts) * float64(cfg.IntervalLength)
		hostICD = avgInterval / float64(totalHosts)
	}

	// Interleave factor
	interleaveFactor := totalServices / totalHosts
	if interleaveFactor < 1 {
		interleaveFactor = 1
	}

	fmt.Println("Projected scheduling information for host and service checks")
	fmt.Println("is listed below.  This information assumes that you are going")
	fmt.Print("to start running Gogios with your current config files.\n\n")

	fmt.Printf("HOST SCHEDULING INFORMATION\n")
	fmt.Printf("--------------------------\n")
	fmt.Printf("Total hosts:                        %d\n", totalHosts)
	fmt.Printf("Host inter-check delay:             %.2f sec\n", hostICD)
	fmt.Printf("Max host check spread:              %d min\n", cfg.MaxHostCheckSpread)
	fmt.Println()

	fmt.Printf("SERVICE SCHEDULING INFORMATION\n")
	fmt.Printf("------------------------------\n")
	fmt.Printf("Total services:                     %d\n", totalServices)
	fmt.Printf("Service inter-check delay:          %.2f sec\n", serviceICD)
	fmt.Printf("Inter-check delay method:           SMART\n")
	fmt.Printf("Service interleave factor:          %d\n", interleaveFactor)
	fmt.Printf("Max service check spread:           %d min\n", cfg.MaxServiceCheckSpread)
	fmt.Println()

	fmt.Printf("CHECK PROCESSING INFORMATION\n")
	fmt.Printf("----------------------------\n")
	fmt.Printf("Max concurrent service checks:      ")
	if cfg.MaxParallelServiceChecks <= 0 {
		fmt.Printf("Unlimited\n")
	} else {
		fmt.Printf("%d\n", cfg.MaxParallelServiceChecks)
	}
	fmt.Println()
}

func runDaemon(configFile string, daemonMode bool) {
	if !daemonMode {
		fmt.Printf("\nGogios %s\n", version)
		fmt.Println("Copyright (c) 2024-present Gogios Contributors")
		fmt.Print("License: MIT\n\n")
	}

	// --- Load configuration ---
	result, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	mainCfg := result.MainCfg
	store := result.Store

	// --- Build runtime Config from MainConfig ---
	cfg := objects.DefaultConfig()
	cfg.IntervalLength = mainCfg.IntervalLength
	if cfg.IntervalLength <= 0 {
		cfg.IntervalLength = 60
	}
	cfg.ServiceCheckTimeout = mainCfg.ServiceCheckTimeout
	cfg.HostCheckTimeout = mainCfg.HostCheckTimeout
	cfg.MaxParallelServiceChecks = mainCfg.MaxConcurrentChecks
	cfg.ExecuteServiceChecks = mainCfg.ExecuteServiceChecks
	cfg.ExecuteHostChecks = mainCfg.ExecuteHostChecks
	cfg.CheckServiceFreshness = mainCfg.CheckServiceFreshness
	cfg.CheckHostFreshness = mainCfg.CheckHostFreshness
	cfg.ServiceFreshnessCheckInterval = mainCfg.ServiceFreshnessCheckInterval
	cfg.HostFreshnessCheckInterval = mainCfg.HostFreshnessCheckInterval
	cfg.StatusUpdateInterval = mainCfg.StatusUpdateInterval
	cfg.RetentionUpdateInterval = mainCfg.RetentionUpdateInterval
	cfg.AdditionalFreshnessLatency = mainCfg.AdditionalFreshnessLatency
	cfg.UseAggressiveHostChecking = mainCfg.UseAggressiveHostChecking
	cfg.TranslatePassiveHostChecks = mainCfg.TranslatePassiveHostChecks
	cfg.MaxServiceCheckSpread = mainCfg.MaxServiceCheckSpread
	cfg.MaxHostCheckSpread = mainCfg.MaxHostCheckSpread
	cfg.CheckReaperInterval = mainCfg.CheckResultReaperFrequency
	cfg.UserMacros = result.UserMacros

	// Map timeout state
	switch mainCfg.ServiceCheckTimeoutState {
	case 'o':
		cfg.ServiceCheckTimeoutState = objects.ServiceOK
	case 'w':
		cfg.ServiceCheckTimeoutState = objects.ServiceWarning
	case 'u':
		cfg.ServiceCheckTimeoutState = objects.ServiceUnknown
	default:
		cfg.ServiceCheckTimeoutState = objects.ServiceCritical
	}

	// Map log rotation method
	logRotation := objects.LogRotationNone
	switch mainCfg.LogRotationMethod {
	case 'h':
		logRotation = objects.LogRotationHourly
	case 'd':
		logRotation = objects.LogRotationDaily
	case 'w':
		logRotation = objects.LogRotationWeekly
	case 'm':
		logRotation = objects.LogRotationMonthly
	}

	// --- Initialize global state ---
	globalState := &objects.GlobalState{
		EnableNotifications:        mainCfg.EnableNotifications,
		ExecuteServiceChecks:       mainCfg.ExecuteServiceChecks,
		ExecuteHostChecks:          mainCfg.ExecuteHostChecks,
		AcceptPassiveServiceChecks: mainCfg.AcceptPassiveServiceChecks,
		AcceptPassiveHostChecks:    mainCfg.AcceptPassiveHostChecks,
		EnableEventHandlers:        mainCfg.EnableEventHandlers,
		ObsessOverServices:         mainCfg.ObsessOverServices,
		ObsessOverHosts:            mainCfg.ObsessOverHosts,
		CheckServiceFreshness:      mainCfg.CheckServiceFreshness,
		CheckHostFreshness:         mainCfg.CheckHostFreshness,
		EnableFlapDetection:        mainCfg.EnableFlapDetection,
		ProcessPerformanceData:     mainCfg.ProcessPerformanceData,
		GlobalHostEventHandler:     mainCfg.GlobalHostEventHandler,
		GlobalServiceEventHandler:  mainCfg.GlobalServiceEventHandler,
		ProgramStart:               time.Now(),
		PID:                        os.Getpid(),
		DaemonMode:                 true,
		IntervalLength:             mainCfg.IntervalLength,
		SoftStateDependencies:      mainCfg.SoftStateDependencies,
		LogNotifications:           mainCfg.LogNotifications,
		LogServiceRetries:          mainCfg.LogServiceRetries,
		LogEventHandlers:           mainCfg.LogEventHandlers,
		LogExternalCommands:        mainCfg.LogExternalCommands,
		NextCommentID:              1,
		NextDowntimeID:             1,
		NextEventID:                1,
		NextProblemID:              1,
		NextNotificationID:         1,
	}

	// --- Ensure var directories exist ---
	for _, dir := range []string{
		filepath.Dir(mainCfg.LogFile),
		filepath.Dir(mainCfg.StatusFile),
		filepath.Dir(mainCfg.StateRetentionFile),
		mainCfg.LogArchivePath,
		mainCfg.CheckResultPath,
		filepath.Dir(mainCfg.CommandFile),
	} {
		if dir != "" {
			os.MkdirAll(dir, 0755)
		}
	}

	// --- Initialize logger ---
	nagLogger, err := logging.NewLogger(mainCfg.LogFile, mainCfg.LogArchivePath, logRotation, mainCfg.UseSyslog, globalState)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer nagLogger.Close()

	// In foreground mode, echo all log output to stdout
	if !daemonMode {
		nagLogger.SetStdout(true)
	}

	nagLogger.Log("Gogios %s starting... (PID=%d)", version, os.Getpid())
	nagLogger.Log("Local time is %s", time.Now().Format("Mon Jan 02 15:04:05 MST 2006"))
	nagLogger.Log("LOG VERSION: 2.0")
	nagLogger.Log("Finished loading configuration with %d hosts, %d services",
		len(store.Hosts), len(store.Services))

	// --- Initialize subsystems ---

	// Comment and downtime managers
	commentMgr := downtime.NewCommentManager(1)
	downtimeMgr := downtime.NewDowntimeManager(1, commentMgr, store)
	downtimeMgr.SetLogger(nagLogger)

	// Macro expander
	macroExpander := &macros.Expander{
		Cfg:        cfg,
		HostLookup: store.GetHost,
		SvcLookup:  store.GetService,
	}

	// Notification engine
	notifEngine := notify.NewNotificationEngine(globalState, store, nagLogger)

	// Status writer
	statusWriter := &status.StatusWriter{
		Path:      mainCfg.StatusFile,
		TempDir:   mainCfg.TempPath,
		Store:     store,
		Global:    globalState,
		Comments:  commentMgr,
		Downtimes: downtimeMgr,
		Version:   "1.0.0",
	}

	// Retention writer/reader
	retentionWriter := &status.RetentionWriter{
		Path:      mainCfg.StateRetentionFile,
		TempDir:   mainCfg.TempPath,
		Store:     store,
		Global:    globalState,
		Comments:  commentMgr,
		Downtimes: downtimeMgr,
		Version:   "1.0.0",
	}

	// Load retention data if it exists
	if mainCfg.RetainStateInformation {
		if _, err := os.Stat(mainCfg.StateRetentionFile); err == nil {
			retReader := &status.RetentionReader{
				Store:     store,
				Global:    globalState,
				Comments:  commentMgr,
				Downtimes: downtimeMgr,
			}
			if err := retReader.Read(mainCfg.StateRetentionFile); err != nil {
				nagLogger.Log("Warning: Failed to read retention data: %v", err)
			} else {
				nagLogger.Log("Successfully read retention data from %s", mainCfg.StateRetentionFile)
			}
		}
	}

	// --- Check executor ---
	resultCh := make(chan *objects.CheckResult, 1024)
	executor := checker.NewExecutor(mainCfg.MaxConcurrentChecks, resultCh)

	// --- Service result handler ---
	svcHandler := &checker.ServiceResultHandler{
		Cfg: cfg,
		HostLookup: store.GetHost,
		OnNotification: func(svc *objects.Service, notifType int) {
			notifEngine.ServiceNotification(svc, notifType, "", "", 0)
		},
		OnStateChange: func(svc *objects.Service, oldState, newState int, hardChange bool) {
			stateStr := objects.ServiceStateName(newState)
			typeStr := objects.StateTypeName(svc.StateType)
			nagLogger.Log("SERVICE ALERT: %s;%s;%s;%s;%d;%s",
				svc.Host.Name, svc.Description, stateStr, typeStr,
				svc.CurrentAttempt, svc.PluginOutput)
		},
	}

	// --- Host result handler ---
	hostHandler := &checker.HostResultHandler{
		Cfg: cfg,
		OnNotification: func(h *objects.Host, notifType int) {
			notifEngine.HostNotification(h, notifType, "", "", 0)
		},
		OnStateChange: func(h *objects.Host, oldState, newState int, hardChange bool) {
			stateStr := objects.HostStateName(newState)
			typeStr := objects.StateTypeName(h.StateType)
			nagLogger.Log("HOST ALERT: %s;%s;%s;%d;%s",
				h.Name, stateStr, typeStr, h.CurrentAttempt, h.PluginOutput)
		},
	}

	// --- Scheduler ---
	sched := scheduler.New(cfg, store.Hosts, store.Services, resultCh)

	// Wire up scheduler callbacks
	sched.OnRunServiceCheck = func(svc *objects.Service, options int) {
		if svc.CheckCommand == nil {
			return
		}
		var args []string
		if svc.CheckCommandArgs != "" {
			args = strings.Split(svc.CheckCommandArgs, "!")
		}
		rawCmd := svc.CheckCommand.CommandLine
		expanded := macroExpander.Expand(rawCmd, svc.Host, svc, args)
		timeout := time.Duration(cfg.ServiceCheckTimeout) * time.Second
		executor.Submit(svc.Host.Name, svc.Description, expanded, timeout, options, objects.CheckTypeActive, svc.Latency)
	}

	sched.OnRunHostCheck = func(host *objects.Host, options int) {
		if host.CheckCommand == nil {
			// Hosts without check commands are assumed UP
			resultCh <- &objects.CheckResult{
				HostName:      host.Name,
				CheckType:     objects.CheckTypeActive,
				CheckOptions:  options,
				ReturnCode:    0,
				Output:        "(No check command defined - host assumed UP)",
				StartTime:     time.Now(),
				FinishTime:    time.Now(),
				ExitedOK:      true,
				Latency:       host.Latency,
			}
			return
		}
		checker.AdjustHostCheckAttempt(host)
		var args []string
		if host.CheckCommandArgs != "" {
			args = strings.Split(host.CheckCommandArgs, "!")
		}
		rawCmd := host.CheckCommand.CommandLine
		expanded := macroExpander.Expand(rawCmd, host, nil, args)
		timeout := time.Duration(cfg.HostCheckTimeout) * time.Second
		executor.Submit(host.Name, "", expanded, timeout, options, objects.CheckTypeActive, host.Latency)
	}

	sched.OnProcessResult = func(cr *objects.CheckResult) {
		if cr.ServiceDescription != "" {
			// Service check result
			svc := store.GetService(cr.HostName, cr.ServiceDescription)
			if svc == nil {
				return
			}
			svcHandler.HandleResult(svc, cr)
			sched.DecrementRunningServiceChecks()

			// Check if a flexible downtime should start
			downtimeMgr.CheckPendingFlexServiceDowntime(cr.HostName, cr.ServiceDescription, svc.CurrentState)

			// Reschedule service check
			sched.AddEvent(&scheduler.Event{
				Type:               scheduler.EventServiceCheck,
				RunTime:            svc.NextCheck,
				HostName:           cr.HostName,
				ServiceDescription: cr.ServiceDescription,
			})
		} else {
			// Host check result
			host := store.GetHost(cr.HostName)
			if host == nil {
				return
			}
			hostHandler.HandleResult(host, cr)

			// Check if a flexible downtime should start
			downtimeMgr.CheckPendingFlexHostDowntime(cr.HostName, host.CurrentState)

			// Reschedule host check
			sched.AddEvent(&scheduler.Event{
				Type:     scheduler.EventHostCheck,
				RunTime:  host.NextCheck,
				HostName: cr.HostName,
			})
		}
	}

	sched.OnStatusSave = func() {
		if err := statusWriter.Write(); err != nil {
			nagLogger.Log("Error writing status data: %v", err)
		}
	}

	sched.OnRetentionSave = func() {
		if mainCfg.RetainStateInformation {
			if err := retentionWriter.Write(); err != nil {
				nagLogger.Log("Error saving retention data: %v", err)
			} else {
				nagLogger.Log("Auto-save of retention data completed successfully.")
			}
		}
	}

	sched.OnLogRotation = func() {
		if err := nagLogger.Rotate(); err != nil {
			log.Printf("Error rotating log: %v", err)
		}
	}

	// --- External command processor ---
	var cmdProcessor *extcmd.Processor
	if mainCfg.CheckExternalCommands && mainCfg.CommandFile != "" {
		cmdProcessor = extcmd.NewProcessor(mainCfg.CommandFile, 256)
		cmdProcessor.SetLogger(func(format string, args ...interface{}) {
			nagLogger.Log(format, args...)
		})

		// Register common command handlers
		registerCommandHandlers(cmdProcessor, store, globalState, sched, notifEngine, commentMgr, downtimeMgr, nagLogger, resultCh)

		if err := cmdProcessor.Start(); err != nil {
			nagLogger.Log("Warning: Failed to start command processor: %v", err)
		} else {
			nagLogger.Log("External command processor started on %s", mainCfg.CommandFile)
			// Drain commands into scheduler
			go func() {
				for cmd := range cmdProcessor.CommandChan() {
					sched.SendCommand(scheduler.Command{
						Name: cmd.Name,
						Args: cmd.Args,
					})
				}
			}()
		}
	}

	// --- Livestatus API server ---
	var livestatusServer *livestatus.Server
	if mainCfg.QuerySocket != "" || mainCfg.LivestatusTCP != "" {
		livestatusServer = livestatus.New(mainCfg.QuerySocket, mainCfg.LivestatusTCP)
		apiState := &api.StateProvider{
			Store:     store,
			Global:    globalState,
			Comments:  commentMgr,
			Downtimes: downtimeMgr,
			Logger:    nagLogger,
			LogFile:   mainCfg.LogFile,
		}
		cmdSink := api.CommandSink(func(name string, args []string) {
			if cmdProcessor != nil {
				cmdProcessor.Dispatch(name, args)
			}
		})
		if err := livestatusServer.Start(apiState, cmdSink); err != nil {
			nagLogger.Log("Warning: Failed to start Livestatus server: %v", err)
		} else {
			if mainCfg.QuerySocket != "" {
				nagLogger.Log("Livestatus API listening on unix:%s", mainCfg.QuerySocket)
			}
			if mainCfg.LivestatusTCP != "" {
				nagLogger.Log("Livestatus API listening on tcp:%s", mainCfg.LivestatusTCP)
			}
		}
	}

	// --- Initialize scheduling ---
	nagLogger.Log("Scheduling initial checks...")
	sched.Init(store.Hosts, store.Services)
	nagLogger.Log("Scheduled %d events in queue", sched.QueueLen())

	// Write initial status
	if err := statusWriter.Write(); err != nil {
		nagLogger.Log("Warning: Failed to write initial status: %v", err)
	}

	// Log initial states if configured
	if mainCfg.LogInitialStates {
		for _, h := range store.Hosts {
			nagLogger.Log("INITIAL HOST STATE: %s;%s;%s;%d;%s",
				h.Name, objects.HostStateName(h.CurrentState),
				objects.StateTypeName(h.StateType), h.CurrentAttempt, h.PluginOutput)
		}
		for _, svc := range store.Services {
			nagLogger.Log("INITIAL SERVICE STATE: %s;%s;%s;%s;%d;%s",
				svc.Host.Name, svc.Description,
				objects.ServiceStateName(svc.CurrentState),
				objects.StateTypeName(svc.StateType), svc.CurrentAttempt, svc.PluginOutput)
		}
	}

	nagLogger.Log("Gogios ready. Entering main event loop.")

	// --- Signal handling ---
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		for sig := range sigCh {
			switch sig {
			case syscall.SIGTERM, syscall.SIGINT:
				nagLogger.Log("Caught %s, shutting down...", sig)
				sched.Stop()
				return
			case syscall.SIGHUP:
				nagLogger.Log("Caught SIGHUP, reloading not yet implemented")
			}
		}
	}()

	// --- Run main event loop (blocks until Stop) ---
	sched.Run()

	// --- Shutdown ---
	nagLogger.Log("Shutting down...")

	if livestatusServer != nil {
		livestatusServer.Stop()
	}

	if cmdProcessor != nil {
		cmdProcessor.Stop()
	}

	// Save final retention data
	if mainCfg.RetainStateInformation {
		if err := retentionWriter.Write(); err != nil {
			nagLogger.Log("Error saving final retention data: %v", err)
		} else {
			nagLogger.Log("Retention data saved.")
		}
	}

	// Write final status
	statusWriter.Write()

	nagLogger.Log("Successfully shutdown... (PID=%d)", os.Getpid())
}

// registerCommandHandlers wires up the most common external commands.
func registerCommandHandlers(
	p *extcmd.Processor,
	store *objects.ObjectStore,
	gs *objects.GlobalState,
	sched *scheduler.Scheduler,
	notifEngine *notify.NotificationEngine,
	commentMgr *downtime.CommentManager,
	downtimeMgr *downtime.DowntimeManager,
	logger *logging.Logger,
	resultCh chan *objects.CheckResult,
) {
	// System commands
	p.RegisterHandler("ENABLE_NOTIFICATIONS", func(cmd *extcmd.Command) {
		gs.EnableNotifications = true
		logger.Log("EXTERNAL COMMAND: ENABLE_NOTIFICATIONS")
	})
	p.RegisterHandler("DISABLE_NOTIFICATIONS", func(cmd *extcmd.Command) {
		gs.EnableNotifications = false
		logger.Log("EXTERNAL COMMAND: DISABLE_NOTIFICATIONS")
	})
	p.RegisterHandler("START_EXECUTING_SVC_CHECKS", func(cmd *extcmd.Command) {
		gs.ExecuteServiceChecks = true
		logger.Log("EXTERNAL COMMAND: START_EXECUTING_SVC_CHECKS")
	})
	p.RegisterHandler("STOP_EXECUTING_SVC_CHECKS", func(cmd *extcmd.Command) {
		gs.ExecuteServiceChecks = false
		logger.Log("EXTERNAL COMMAND: STOP_EXECUTING_SVC_CHECKS")
	})
	p.RegisterHandler("START_EXECUTING_HOST_CHECKS", func(cmd *extcmd.Command) {
		gs.ExecuteHostChecks = true
		logger.Log("EXTERNAL COMMAND: START_EXECUTING_HOST_CHECKS")
	})
	p.RegisterHandler("STOP_EXECUTING_HOST_CHECKS", func(cmd *extcmd.Command) {
		gs.ExecuteHostChecks = false
		logger.Log("EXTERNAL COMMAND: STOP_EXECUTING_HOST_CHECKS")
	})
	p.RegisterHandler("ENABLE_EVENT_HANDLERS", func(cmd *extcmd.Command) {
		gs.EnableEventHandlers = true
		logger.Log("EXTERNAL COMMAND: ENABLE_EVENT_HANDLERS")
	})
	p.RegisterHandler("DISABLE_EVENT_HANDLERS", func(cmd *extcmd.Command) {
		gs.EnableEventHandlers = false
		logger.Log("EXTERNAL COMMAND: DISABLE_EVENT_HANDLERS")
	})
	p.RegisterHandler("ENABLE_FLAP_DETECTION", func(cmd *extcmd.Command) {
		gs.EnableFlapDetection = true
		logger.Log("EXTERNAL COMMAND: ENABLE_FLAP_DETECTION")
	})
	p.RegisterHandler("DISABLE_FLAP_DETECTION", func(cmd *extcmd.Command) {
		gs.EnableFlapDetection = false
		logger.Log("EXTERNAL COMMAND: DISABLE_FLAP_DETECTION")
	})

	// Process passive check results
	p.RegisterHandler("PROCESS_SERVICE_CHECK_RESULT", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 4 {
			return
		}
		hostName := cmd.Args[0]
		svcDesc := cmd.Args[1]
		rc := 0
		fmt.Sscanf(cmd.Args[2], "%d", &rc)
		output := cmd.Args[3]

		svc := store.GetService(hostName, svcDesc)
		if svc == nil {
			return
		}
		now := time.Now()
		sched.SendCommand(scheduler.Command{Name: "_INTERNAL_RESULT"})
		// Send directly as check result
		cr := &objects.CheckResult{
			HostName:           hostName,
			ServiceDescription: svcDesc,
			CheckType:          objects.CheckTypePassive,
			ReturnCode:         rc,
			Output:             output,
			StartTime:          now,
			FinishTime:         now,
			ExitedOK:           true,
		}
		// Process inline since we're on the command handler goroutine
		// The scheduler's OnProcessResult will be called via resultCh
		go func() { resultCh <- cr }()
	})

	p.RegisterHandler("PROCESS_HOST_CHECK_RESULT", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 3 {
			return
		}
		hostName := cmd.Args[0]
		rc := 0
		fmt.Sscanf(cmd.Args[1], "%d", &rc)
		output := cmd.Args[2]

		host := store.GetHost(hostName)
		if host == nil {
			return
		}
		now := time.Now()
		cr := &objects.CheckResult{
			HostName:   hostName,
			CheckType:  objects.CheckTypePassive,
			ReturnCode: rc,
			Output:     output,
			StartTime:  now,
			FinishTime: now,
			ExitedOK:   true,
		}
		go func() { resultCh <- cr }()
	})

	// Schedule forced checks
	p.RegisterHandler("SCHEDULE_FORCED_SVC_CHECK", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 3 {
			return
		}
		hostName := cmd.Args[0]
		svcDesc := cmd.Args[1]
		var checkTime int64
		fmt.Sscanf(cmd.Args[2], "%d", &checkTime)
		sched.AddEvent(&scheduler.Event{
			Type:               scheduler.EventServiceCheck,
			RunTime:            time.Unix(checkTime, 0),
			HostName:           hostName,
			ServiceDescription: svcDesc,
			CheckOptions:       objects.CheckOptionForceExecution,
		})
		logger.Log("EXTERNAL COMMAND: SCHEDULE_FORCED_SVC_CHECK;%s;%s;%d", hostName, svcDesc, checkTime)
	})

	p.RegisterHandler("SCHEDULE_FORCED_HOST_CHECK", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 2 {
			return
		}
		hostName := cmd.Args[0]
		var checkTime int64
		fmt.Sscanf(cmd.Args[1], "%d", &checkTime)
		sched.AddEvent(&scheduler.Event{
			Type:         scheduler.EventHostCheck,
			RunTime:      time.Unix(checkTime, 0),
			HostName:     hostName,
			CheckOptions: objects.CheckOptionForceExecution,
		})
		logger.Log("EXTERNAL COMMAND: SCHEDULE_FORCED_HOST_CHECK;%s;%d", hostName, checkTime)
	})

	// Acknowledge problems
	p.RegisterHandler("ACKNOWLEDGE_SVC_PROBLEM", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 7 {
			return
		}
		hostName := cmd.Args[0]
		svcDesc := cmd.Args[1]
		svc := store.GetService(hostName, svcDesc)
		if svc == nil {
			return
		}
		sticky := cmd.Args[2] == "2"
		sendNotif := cmd.Args[3] == "1"
		// persistent := cmd.Args[4] == "1"
		author := cmd.Args[5]
		comment := cmd.Args[6]

		if sticky {
			svc.AckType = objects.AckSticky
		} else {
			svc.AckType = objects.AckNormal
		}
		svc.ProblemAcknowledged = true

		if sendNotif {
			notifEngine.ServiceNotification(svc, objects.NotificationAcknowledgement, author, comment, 0)
		}
		logger.Log("EXTERNAL COMMAND: ACKNOWLEDGE_SVC_PROBLEM;%s;%s", hostName, svcDesc)
	})

	p.RegisterHandler("ACKNOWLEDGE_HOST_PROBLEM", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 6 {
			return
		}
		hostName := cmd.Args[0]
		host := store.GetHost(hostName)
		if host == nil {
			return
		}
		sticky := cmd.Args[1] == "2"
		sendNotif := cmd.Args[2] == "1"
		author := cmd.Args[4]
		comment := cmd.Args[5]

		if sticky {
			host.AckType = objects.AckSticky
		} else {
			host.AckType = objects.AckNormal
		}
		host.ProblemAcknowledged = true

		if sendNotif {
			notifEngine.HostNotification(host, objects.NotificationAcknowledgement, author, comment, 0)
		}
		logger.Log("EXTERNAL COMMAND: ACKNOWLEDGE_HOST_PROBLEM;%s", hostName)
	})

	// Schedule downtimes
	p.RegisterHandler("SCHEDULE_HOST_DOWNTIME", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 8 {
			return
		}
		hostName := cmd.Args[0]
		host := store.GetHost(hostName)
		if host == nil {
			return
		}
		var startTS, endTS, triggerID, duration int64
		fixed := cmd.Args[3] == "1"
		fmt.Sscanf(cmd.Args[1], "%d", &startTS)
		fmt.Sscanf(cmd.Args[2], "%d", &endTS)
		fmt.Sscanf(cmd.Args[4], "%d", &triggerID)
		fmt.Sscanf(cmd.Args[5], "%d", &duration)
		author := cmd.Args[6]
		comment := cmd.Args[7]

		d := &downtime.Downtime{
			Type:        objects.HostDowntimeType,
			HostName:    hostName,
			StartTime:   time.Unix(startTS, 0),
			EndTime:     time.Unix(endTS, 0),
			Fixed:       fixed,
			TriggeredBy: uint64(triggerID),
			Duration:    time.Duration(duration) * time.Second,
			Author:      author,
			Comment:     comment,
		}
		id := downtimeMgr.Schedule(d)
		logger.Log("EXTERNAL COMMAND: SCHEDULE_HOST_DOWNTIME;%s", hostName)

		// For fixed downtimes that start now or in the past, start immediately
		if fixed && !time.Unix(startTS, 0).After(time.Now()) {
			downtimeMgr.HandleStart(id)
		}

		// Schedule end via goroutine timer
		endTime := time.Unix(endTS, 0)
		go func(dtID uint64) {
			wait := time.Until(endTime)
			if wait > 0 {
				time.Sleep(wait)
			}
			downtimeMgr.HandleEnd(dtID)
		}(id)
	})

	p.RegisterHandler("SCHEDULE_SVC_DOWNTIME", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 9 {
			return
		}
		hostName := cmd.Args[0]
		svcDesc := cmd.Args[1]
		svc := store.GetService(hostName, svcDesc)
		if svc == nil {
			return
		}
		var startTS, endTS, triggerID, duration int64
		fixed := cmd.Args[4] == "1"
		fmt.Sscanf(cmd.Args[2], "%d", &startTS)
		fmt.Sscanf(cmd.Args[3], "%d", &endTS)
		fmt.Sscanf(cmd.Args[5], "%d", &triggerID)
		fmt.Sscanf(cmd.Args[6], "%d", &duration)
		author := cmd.Args[7]
		comment := cmd.Args[8]

		d := &downtime.Downtime{
			Type:               objects.ServiceDowntimeType,
			HostName:           hostName,
			ServiceDescription: svcDesc,
			StartTime:          time.Unix(startTS, 0),
			EndTime:            time.Unix(endTS, 0),
			Fixed:              fixed,
			TriggeredBy:        uint64(triggerID),
			Duration:           time.Duration(duration) * time.Second,
			Author:             author,
			Comment:            comment,
		}
		id := downtimeMgr.Schedule(d)
		logger.Log("EXTERNAL COMMAND: SCHEDULE_SVC_DOWNTIME;%s;%s", hostName, svcDesc)

		// For fixed downtimes that start now or in the past, start immediately
		if fixed && !time.Unix(startTS, 0).After(time.Now()) {
			downtimeMgr.HandleStart(id)
		}

		// Schedule end via goroutine timer
		endTime := time.Unix(endTS, 0)
		go func(dtID uint64) {
			wait := time.Until(endTime)
			if wait > 0 {
				time.Sleep(wait)
			}
			downtimeMgr.HandleEnd(dtID)
		}(id)
	})

	p.RegisterHandler("DEL_HOST_DOWNTIME", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		var id uint64
		fmt.Sscanf(cmd.Args[0], "%d", &id)
		downtimeMgr.Unschedule(id)
		logger.Log("EXTERNAL COMMAND: DEL_HOST_DOWNTIME;%d", id)
	})

	p.RegisterHandler("DEL_SVC_DOWNTIME", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		var id uint64
		fmt.Sscanf(cmd.Args[0], "%d", &id)
		downtimeMgr.Unschedule(id)
		logger.Log("EXTERNAL COMMAND: DEL_SVC_DOWNTIME;%d", id)
	})

	// Remove acknowledgement
	p.RegisterHandler("REMOVE_SVC_ACKNOWLEDGEMENT", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 2 {
			return
		}
		hostName := cmd.Args[0]
		svcDesc := cmd.Args[1]
		svc := store.GetService(hostName, svcDesc)
		if svc == nil {
			return
		}
		svc.ProblemAcknowledged = false
		svc.AckType = objects.AckNone
		logger.Log("EXTERNAL COMMAND: REMOVE_SVC_ACKNOWLEDGEMENT;%s;%s", hostName, svcDesc)
	})

	p.RegisterHandler("REMOVE_HOST_ACKNOWLEDGEMENT", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		hostName := cmd.Args[0]
		host := store.GetHost(hostName)
		if host == nil {
			return
		}
		host.ProblemAcknowledged = false
		host.AckType = objects.AckNone
		logger.Log("EXTERNAL COMMAND: REMOVE_HOST_ACKNOWLEDGEMENT;%s", hostName)
	})

	// Per-host/service notification and check toggles
	p.RegisterHandler("DISABLE_HOST_NOTIFICATIONS", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		host := store.GetHost(cmd.Args[0])
		if host != nil {
			host.NotificationsEnabled = false
		}
		logger.Log("EXTERNAL COMMAND: DISABLE_HOST_NOTIFICATIONS;%s", cmd.Args[0])
	})

	p.RegisterHandler("ENABLE_HOST_NOTIFICATIONS", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		host := store.GetHost(cmd.Args[0])
		if host != nil {
			host.NotificationsEnabled = true
		}
		logger.Log("EXTERNAL COMMAND: ENABLE_HOST_NOTIFICATIONS;%s", cmd.Args[0])
	})

	p.RegisterHandler("DISABLE_SVC_NOTIFICATIONS", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 2 {
			return
		}
		svc := store.GetService(cmd.Args[0], cmd.Args[1])
		if svc != nil {
			svc.NotificationsEnabled = false
		}
		logger.Log("EXTERNAL COMMAND: DISABLE_SVC_NOTIFICATIONS;%s;%s", cmd.Args[0], cmd.Args[1])
	})

	p.RegisterHandler("ENABLE_SVC_NOTIFICATIONS", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 2 {
			return
		}
		svc := store.GetService(cmd.Args[0], cmd.Args[1])
		if svc != nil {
			svc.NotificationsEnabled = true
		}
		logger.Log("EXTERNAL COMMAND: ENABLE_SVC_NOTIFICATIONS;%s;%s", cmd.Args[0], cmd.Args[1])
	})

	p.RegisterHandler("DISABLE_HOST_CHECK", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		hst := store.GetHost(cmd.Args[0])
		if hst != nil {
			hst.ActiveChecksEnabled = false
		}
		logger.Log("EXTERNAL COMMAND: DISABLE_HOST_CHECK;%s", cmd.Args[0])
	})

	p.RegisterHandler("ENABLE_HOST_CHECK", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 1 {
			return
		}
		hst := store.GetHost(cmd.Args[0])
		if hst != nil {
			hst.ActiveChecksEnabled = true
		}
		logger.Log("EXTERNAL COMMAND: ENABLE_HOST_CHECK;%s", cmd.Args[0])
	})

	p.RegisterHandler("DISABLE_SVC_CHECK", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 2 {
			return
		}
		svc := store.GetService(cmd.Args[0], cmd.Args[1])
		if svc != nil {
			svc.ActiveChecksEnabled = false
		}
		logger.Log("EXTERNAL COMMAND: DISABLE_SVC_CHECK;%s;%s", cmd.Args[0], cmd.Args[1])
	})

	p.RegisterHandler("ENABLE_SVC_CHECK", func(cmd *extcmd.Command) {
		if len(cmd.Args) < 2 {
			return
		}
		svc := store.GetService(cmd.Args[0], cmd.Args[1])
		if svc != nil {
			svc.ActiveChecksEnabled = true
		}
		logger.Log("EXTERNAL COMMAND: ENABLE_SVC_CHECK;%s;%s", cmd.Args[0], cmd.Args[1])
	})

	// Shutdown
	p.RegisterHandler("SHUTDOWN_PROCESS", func(cmd *extcmd.Command) {
		logger.Log("EXTERNAL COMMAND: SHUTDOWN_PROCESS")
		sched.Stop()
	})
	p.RegisterHandler("SHUTDOWN_PROGRAM", func(cmd *extcmd.Command) {
		logger.Log("EXTERNAL COMMAND: SHUTDOWN_PROGRAM")
		sched.Stop()
	})
}
