// Package logging implements Nagios-compatible log file management.
package logging

import (
	"fmt"
	"log/syslog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Verbosity bitmask flags for selective verbose logging.
const (
	VerboseChecks     = 1 << 0 // Log every check result
	VerboseLivestatus = 1 << 1 // Log every livestatus query
)

// Logger handles Nagios log output with rotation support.
type Logger struct {
	mu             sync.Mutex
	logFile        *os.File
	logPath        string
	archivePath    string
	rotationMethod int
	useSyslog      bool
	useStdout      bool
	syslogWriter   *syslog.Writer
	global         *objects.GlobalState
	Verbosity      int
}

// NewLogger creates a new Nagios logger.
func NewLogger(logPath, archivePath string, rotationMethod int, useSyslog bool, global *objects.GlobalState) (*Logger, error) {
	l := &Logger{
		logPath:        logPath,
		archivePath:    archivePath,
		rotationMethod: rotationMethod,
		useSyslog:      useSyslog,
		global:         global,
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("open log file %s: %w", logPath, err)
	}
	l.logFile = f

	if useSyslog {
		sw, err := syslog.New(syslog.LOG_USER|syslog.LOG_INFO, "nagios")
		if err != nil {
			// Syslog failure is non-fatal
			l.useSyslog = false
		} else {
			l.syslogWriter = sw
		}
	}

	return l, nil
}

// Close closes the log file and syslog connection.
func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.logFile != nil {
		l.logFile.Close()
	}
	if l.syslogWriter != nil {
		l.syslogWriter.Close()
	}
}

// SetStdout enables or disables echoing log messages to stdout.
func (l *Logger) SetStdout(enabled bool) {
	l.mu.Lock()
	l.useStdout = enabled
	l.mu.Unlock()
}

// Log writes a timestamped message to the log file.
func (l *Logger) Log(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%d] %s\n", time.Now().Unix(), msg)

	l.mu.Lock()
	if l.logFile != nil {
		l.logFile.WriteString(line)
	}
	if l.useStdout {
		os.Stdout.WriteString(line)
	}
	l.mu.Unlock()

	if l.useSyslog && l.syslogWriter != nil {
		l.syslogWriter.Info(msg)
	}
}

// LogVerbose writes a log message only if the given verbosity flag is enabled.
func (l *Logger) LogVerbose(flag int, format string, args ...interface{}) {
	if l.Verbosity&flag == 0 {
		return
	}
	l.Log(format, args...)
}

// LogServiceAlert logs a service state change alert.
func (l *Logger) LogServiceAlert(hostName, svcDesc string, state, stateType, attempt int, output string) {
	l.Log("SERVICE ALERT: %s;%s;%s;%s;%d;%s",
		hostName, svcDesc,
		objects.ServiceStateName(state),
		objects.StateTypeName(stateType),
		attempt, output)
}

// LogHostAlert logs a host state change alert.
func (l *Logger) LogHostAlert(hostName string, state, stateType, attempt int, output string) {
	l.Log("HOST ALERT: %s;%s;%s;%d;%s",
		hostName,
		objects.HostStateName(state),
		objects.StateTypeName(stateType),
		attempt, output)
}

// LogServiceNotification logs a service notification event.
func (l *Logger) LogServiceNotification(contactName, hostName, svcDesc, notifType, cmdName, output, author, comment string) {
	if l.global != nil && !l.global.LogNotifications {
		return
	}
	msg := fmt.Sprintf("SERVICE NOTIFICATION: %s;%s;%s;%s;%s;%s",
		contactName, hostName, svcDesc, notifType, cmdName, output)
	if author != "" || comment != "" {
		msg += ";" + author + ";" + comment
	}
	l.Log("%s", msg)
}

// LogHostNotification logs a host notification event.
func (l *Logger) LogHostNotification(contactName, hostName, notifType, cmdName, output, author, comment string) {
	if l.global != nil && !l.global.LogNotifications {
		return
	}
	msg := fmt.Sprintf("HOST NOTIFICATION: %s;%s;%s;%s;%s",
		contactName, hostName, notifType, cmdName, output)
	if author != "" || comment != "" {
		msg += ";" + author + ";" + comment
	}
	l.Log("%s", msg)
}

// LogHostDowntime logs a host downtime alert.
func (l *Logger) LogHostDowntime(hostName, action, message string) {
	l.Log("HOST DOWNTIME ALERT: %s;%s; %s", hostName, action, message)
}

// LogServiceDowntime logs a service downtime alert.
func (l *Logger) LogServiceDowntime(hostName, svcDesc, action, message string) {
	l.Log("SERVICE DOWNTIME ALERT: %s;%s;%s; %s", hostName, svcDesc, action, message)
}

// LogEventHandler logs an event handler execution.
func (l *Logger) LogEventHandler(global bool, isHost bool, hostName, svcDesc string, state, stateType, attempt int, handler string) {
	if l.global != nil && !l.global.LogEventHandlers {
		return
	}
	prefix := ""
	if global {
		prefix = "GLOBAL "
	}
	if isHost {
		l.Log("%sHOST EVENT HANDLER: %s;%s;%s;%d;%s",
			prefix, hostName,
			objects.HostStateName(state),
			objects.StateTypeName(stateType),
			attempt, handler)
	} else {
		l.Log("%sSERVICE EVENT HANDLER: %s;%s;%s;%s;%d;%s",
			prefix, hostName, svcDesc,
			objects.ServiceStateName(state),
			objects.StateTypeName(stateType),
			attempt, handler)
	}
}

// LogExternalCommand logs an external command.
func (l *Logger) LogExternalCommand(cmdName string, args []string) {
	if l.global != nil && !l.global.LogExternalCommands {
		return
	}
	argStr := ""
	if len(args) > 0 {
		argStr = ";" + strings.Join(args, ";")
	}
	l.Log("EXTERNAL COMMAND: %s%s", cmdName, argStr)
}

// LogPassiveCheck logs a passive check result.
func (l *Logger) LogPassiveCheck(isHost bool, hostName, svcDesc string, returnCode int, output string) {
	if l.global != nil && !l.global.LogPassiveChecks {
		return
	}
	if isHost {
		l.Log("PASSIVE HOST CHECK: %s;%d;%s", hostName, returnCode, output)
	} else {
		l.Log("PASSIVE SERVICE CHECK: %s;%s;%d;%s", hostName, svcDesc, returnCode, output)
	}
}

// LogInitialHostState logs initial host state at startup.
func (l *Logger) LogInitialHostState(h *objects.Host) {
	if l.global != nil && !l.global.LogInitialStates {
		return
	}
	l.Log("INITIAL HOST STATE: %s;%s;%s;%d;%s",
		h.Name,
		objects.HostStateName(h.CurrentState),
		objects.StateTypeName(h.StateType),
		h.CurrentAttempt,
		h.PluginOutput)
}

// LogInitialServiceState logs initial service state at startup.
func (l *Logger) LogInitialServiceState(s *objects.Service) {
	if l.global != nil && !l.global.LogInitialStates {
		return
	}
	hostName := ""
	if s.Host != nil {
		hostName = s.Host.Name
	}
	l.Log("INITIAL SERVICE STATE: %s;%s;%s;%s;%d;%s",
		hostName, s.Description,
		objects.ServiceStateName(s.CurrentState),
		objects.StateTypeName(s.StateType),
		s.CurrentAttempt,
		s.PluginOutput)
}

// LogServiceRetry logs a service retry (soft state).
func (l *Logger) LogServiceRetry(hostName, svcDesc string, state, stateType, attempt int, output string) {
	if l.global != nil && !l.global.LogServiceRetries {
		return
	}
	l.LogServiceAlert(hostName, svcDesc, state, stateType, attempt, output)
}

// Rotate rotates the log file.
func (l *Logger) Rotate() error {
	now := time.Now()
	archiveName := fmt.Sprintf("nagios-%02d-%02d-%04d-%02d.log",
		now.Month(), now.Day(), now.Year(), now.Hour())
	archivePath := filepath.Join(l.archivePath, archiveName)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Don't overwrite existing archive
	if _, err := os.Stat(archivePath); err == nil {
		return nil
	}

	if l.logFile != nil {
		l.logFile.Close()
	}

	if err := os.Rename(l.logPath, archivePath); err != nil {
		// If rename fails, reopen the log
		l.logFile, _ = os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		return fmt.Errorf("rotate log: %w", err)
	}

	var err error
	l.logFile, err = os.OpenFile(l.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open new log: %w", err)
	}

	// Log the rotation event
	fmt.Fprintf(l.logFile, "[%d] LOG ROTATION: %s\n", time.Now().Unix(), archivePath)
	fmt.Fprintf(l.logFile, "[%d] LOG VERSION: 2.0\n", time.Now().Unix())

	return nil
}

// NextRotationTime returns the next time the log should be rotated.
func (l *Logger) NextRotationTime(from time.Time) time.Time {
	switch l.rotationMethod {
	case objects.LogRotationHourly:
		return from.Truncate(time.Hour).Add(time.Hour)
	case objects.LogRotationDaily:
		y, m, d := from.Date()
		return time.Date(y, m, d+1, 0, 0, 0, 0, from.Location())
	case objects.LogRotationWeekly:
		y, m, d := from.Date()
		daysUntilSunday := (7 - int(from.Weekday())) % 7
		if daysUntilSunday == 0 {
			daysUntilSunday = 7
		}
		return time.Date(y, m, d+daysUntilSunday, 0, 0, 0, 0, from.Location())
	case objects.LogRotationMonthly:
		y, m, _ := from.Date()
		return time.Date(y, m+1, 1, 0, 0, 0, 0, from.Location())
	default:
		return time.Time{} // No rotation
	}
}

// RotationMethod returns the current rotation method.
func (l *Logger) RotationMethod() int {
	return l.rotationMethod
}
