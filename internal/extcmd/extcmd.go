// Package extcmd implements the Nagios external command pipe interface.
package extcmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Command represents a parsed external command.
type Command struct {
	Timestamp int64
	Name      string
	Args      []string
	Raw       string
}

// Handler is a function that processes an external command.
type Handler func(cmd *Command)

// Processor reads external commands from a named pipe and dispatches them.
type Processor struct {
	pipePath string
	handlers map[string]Handler
	cmdChan  chan *Command
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
	logger   func(string, ...interface{})
}

// NewProcessor creates a new command processor.
func NewProcessor(pipePath string, bufSize int) *Processor {
	if bufSize <= 0 {
		bufSize = 256
	}
	return &Processor{
		pipePath: pipePath,
		handlers: make(map[string]Handler),
		cmdChan:  make(chan *Command, bufSize),
		stopChan: make(chan struct{}),
	}
}

// SetLogger sets the logging function.
func (p *Processor) SetLogger(l func(string, ...interface{})) {
	p.logger = l
}

func (p *Processor) log(format string, args ...interface{}) {
	if p.logger != nil {
		p.logger(format, args...)
	}
}

// RegisterHandler registers a handler for a command name.
func (p *Processor) RegisterHandler(name string, h Handler) {
	p.mu.Lock()
	p.handlers[name] = h
	p.mu.Unlock()
}

// Dispatch directly invokes a registered command handler by name.
// This allows external APIs (like Livestatus) to route commands
// through the same handler infrastructure as the pipe interface.
func (p *Processor) Dispatch(name string, args []string) {
	p.mu.RLock()
	handler, ok := p.handlers[name]
	p.mu.RUnlock()
	if ok {
		handler(&Command{
			Timestamp: time.Now().Unix(),
			Name:      name,
			Args:      args,
		})
	}
}

// RegisterHandlers registers multiple handlers at once.
func (p *Processor) RegisterHandlers(handlers map[string]Handler) {
	p.mu.Lock()
	for name, h := range handlers {
		p.handlers[name] = h
	}
	p.mu.Unlock()
}

// CommandChan returns the channel for receiving parsed commands.
// Use this to receive commands in the main event loop.
func (p *Processor) CommandChan() <-chan *Command {
	return p.cmdChan
}

// Start begins reading from the command pipe in a goroutine.
func (p *Processor) Start() error {
	// Create FIFO if it doesn't exist
	if _, err := os.Stat(p.pipePath); os.IsNotExist(err) {
		if err := mkfifo(p.pipePath); err != nil {
			return fmt.Errorf("failed to create command pipe %s: %w", p.pipePath, err)
		}
	}

	p.wg.Add(1)
	go p.readLoop()
	return nil
}

// Stop stops the command processor.
func (p *Processor) Stop() {
	close(p.stopChan)
	// Unblock the readLoop if it's stuck in os.Open() on the FIFO.
	// Use O_WRONLY|O_NONBLOCK so this open doesn't block itself,
	// and the write-side open wakes up the blocking read-side open.
	fd, err := syscall.Open(p.pipePath, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err == nil {
		syscall.Close(fd)
	}
	p.wg.Wait()
}

func (p *Processor) readLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.stopChan:
			return
		default:
		}

		f, err := os.Open(p.pipePath)
		if err != nil {
			select {
			case <-p.stopChan:
				return
			default:
				continue
			}
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			select {
			case <-p.stopChan:
				f.Close()
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}

			cmd, err := Parse(line)
			if err != nil {
				p.log("Error parsing external command: %s", err)
				continue
			}

			// Try direct dispatch first
			p.mu.RLock()
			handler, ok := p.handlers[cmd.Name]
			p.mu.RUnlock()

			if ok {
				handler(cmd)
			}

			// Also send to channel for main loop processing
			select {
			case p.cmdChan <- cmd:
			default:
				p.log("External command channel full, dropping: %s", cmd.Name)
			}
		}
		f.Close()
	}
}

// Parse parses a single external command line.
// Format: [<timestamp>] <COMMAND_NAME>;<arg1>;<arg2>;...
func Parse(line string) (*Command, error) {
	line = strings.TrimSpace(line)
	if len(line) == 0 {
		return nil, fmt.Errorf("empty command")
	}

	// Parse timestamp
	if line[0] != '[' {
		return nil, fmt.Errorf("missing timestamp bracket")
	}
	closeBracket := strings.IndexByte(line, ']')
	if closeBracket < 0 {
		return nil, fmt.Errorf("missing closing bracket")
	}

	tsStr := line[1:closeBracket]
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid timestamp: %w", err)
	}

	// Rest is after "] "
	rest := strings.TrimSpace(line[closeBracket+1:])

	// Split command name from args
	cmd := &Command{
		Timestamp: ts,
		Raw:       line,
	}

	semiIdx := strings.IndexByte(rest, ';')
	if semiIdx < 0 {
		cmd.Name = rest
		return cmd, nil
	}

	cmd.Name = rest[:semiIdx]
	argStr := rest[semiIdx+1:]

	// Parse args - most commands use semicolons as separators
	// The last argument may contain semicolons
	cmd.Args = splitArgs(cmd.Name, argStr)

	return cmd, nil
}

// splitArgs splits command arguments. The number of expected semicolons varies
// by command. We split on semicolons but respect that the last arg may contain them.
func splitArgs(cmdName, argStr string) []string {
	// Determine expected argument count for known commands
	n := expectedArgCount(cmdName)
	if n <= 0 {
		// Unknown command or single-arg: return whole string
		if argStr == "" {
			return nil
		}
		return []string{argStr}
	}

	args := make([]string, 0, n)
	remaining := argStr
	for i := 0; i < n-1; i++ {
		idx := strings.IndexByte(remaining, ';')
		if idx < 0 {
			args = append(args, remaining)
			return args
		}
		args = append(args, remaining[:idx])
		remaining = remaining[idx+1:]
	}
	// Last arg gets the rest
	args = append(args, remaining)
	return args
}

func expectedArgCount(cmdName string) int {
	switch cmdName {
	case "ACKNOWLEDGE_HOST_PROBLEM":
		return 6 // host;sticky;notify;persistent;author;comment
	case "ACKNOWLEDGE_SVC_PROBLEM":
		return 7 // host;svc;sticky;notify;persistent;author;comment
	case "ADD_HOST_COMMENT":
		return 4 // host;persistent;author;comment
	case "ADD_SVC_COMMENT":
		return 5 // host;svc;persistent;author;comment
	case "DEL_HOST_COMMENT", "DEL_SVC_COMMENT":
		return 1
	case "DEL_ALL_HOST_COMMENTS":
		return 1
	case "DEL_ALL_SVC_COMMENTS":
		return 2
	case "SCHEDULE_HOST_DOWNTIME":
		return 8 // host;start;end;fixed;trigger_id;duration;author;comment
	case "SCHEDULE_SVC_DOWNTIME":
		return 9 // host;svc;start;end;fixed;trigger_id;duration;author;comment
	case "SCHEDULE_HOST_SVC_DOWNTIME":
		return 8
	case "DEL_HOST_DOWNTIME", "DEL_SVC_DOWNTIME":
		return 1
	case "REMOVE_HOST_ACKNOWLEDGEMENT":
		return 1
	case "REMOVE_SVC_ACKNOWLEDGEMENT":
		return 2
	case "ENABLE_HOST_NOTIFICATIONS", "DISABLE_HOST_NOTIFICATIONS":
		return 1
	case "ENABLE_SVC_NOTIFICATIONS", "DISABLE_SVC_NOTIFICATIONS":
		return 2
	case "ENABLE_HOST_SVC_NOTIFICATIONS", "DISABLE_HOST_SVC_NOTIFICATIONS":
		return 1
	case "SCHEDULE_HOST_CHECK", "SCHEDULE_FORCED_HOST_CHECK":
		return 2
	case "SCHEDULE_SVC_CHECK", "SCHEDULE_FORCED_SVC_CHECK":
		return 3
	case "PROCESS_SERVICE_CHECK_RESULT":
		return 4 // host;svc;return_code;output
	case "PROCESS_HOST_CHECK_RESULT":
		return 3 // host;return_code;output
	case "SEND_CUSTOM_HOST_NOTIFICATION":
		return 4 // host;options;author;comment
	case "SEND_CUSTOM_SVC_NOTIFICATION":
		return 5 // host;svc;options;author;comment
	case "DELAY_HOST_NOTIFICATION":
		return 2
	case "DELAY_SVC_NOTIFICATION":
		return 3
	case "SCHEDULE_HOST_SVC_CHECKS", "SCHEDULE_FORCED_HOST_SVC_CHECKS":
		return 2
	case "ENABLE_HOST_CHECK", "DISABLE_HOST_CHECK":
		return 1
	case "ENABLE_SVC_CHECK", "DISABLE_SVC_CHECK":
		return 2
	case "ENABLE_PASSIVE_HOST_CHECKS", "DISABLE_PASSIVE_HOST_CHECKS":
		return 1
	case "ENABLE_PASSIVE_SVC_CHECKS", "DISABLE_PASSIVE_SVC_CHECKS":
		return 2
	case "ENABLE_HOST_EVENT_HANDLER", "DISABLE_HOST_EVENT_HANDLER":
		return 1
	case "ENABLE_SVC_EVENT_HANDLER", "DISABLE_SVC_EVENT_HANDLER":
		return 2
	case "ENABLE_HOST_FLAP_DETECTION", "DISABLE_HOST_FLAP_DETECTION":
		return 1
	case "ENABLE_SVC_FLAP_DETECTION", "DISABLE_SVC_FLAP_DETECTION":
		return 2
	case "SET_HOST_NOTIFICATION_NUMBER":
		return 2
	case "SET_SVC_NOTIFICATION_NUMBER":
		return 3
	case "CHANGE_NORMAL_HOST_CHECK_INTERVAL", "CHANGE_RETRY_HOST_CHECK_INTERVAL":
		return 2
	case "CHANGE_NORMAL_SVC_CHECK_INTERVAL", "CHANGE_RETRY_SVC_CHECK_INTERVAL":
		return 3
	case "CHANGE_MAX_HOST_CHECK_ATTEMPTS":
		return 2
	case "CHANGE_MAX_SVC_CHECK_ATTEMPTS":
		return 3
	case "CHANGE_HOST_EVENT_HANDLER", "CHANGE_HOST_CHECK_COMMAND":
		return 2
	case "CHANGE_SVC_EVENT_HANDLER", "CHANGE_SVC_CHECK_COMMAND":
		return 3
	case "CHANGE_HOST_CHECK_TIMEPERIOD", "CHANGE_HOST_NOTIFICATION_TIMEPERIOD":
		return 2
	case "CHANGE_SVC_CHECK_TIMEPERIOD", "CHANGE_SVC_NOTIFICATION_TIMEPERIOD":
		return 3
	case "CHANGE_CUSTOM_HOST_VAR":
		return 3
	case "CHANGE_CUSTOM_SVC_VAR":
		return 4
	case "CHANGE_CUSTOM_CONTACT_VAR":
		return 3
	case "CHANGE_GLOBAL_HOST_EVENT_HANDLER", "CHANGE_GLOBAL_SVC_EVENT_HANDLER":
		return 1
	case "ENABLE_HOSTGROUP_HOST_NOTIFICATIONS", "DISABLE_HOSTGROUP_HOST_NOTIFICATIONS",
		"ENABLE_HOSTGROUP_SVC_NOTIFICATIONS", "DISABLE_HOSTGROUP_SVC_NOTIFICATIONS",
		"ENABLE_HOSTGROUP_HOST_CHECKS", "DISABLE_HOSTGROUP_HOST_CHECKS",
		"ENABLE_HOSTGROUP_PASSIVE_HOST_CHECKS", "DISABLE_HOSTGROUP_PASSIVE_HOST_CHECKS",
		"ENABLE_HOSTGROUP_SVC_CHECKS", "DISABLE_HOSTGROUP_SVC_CHECKS",
		"ENABLE_HOSTGROUP_PASSIVE_SVC_CHECKS", "DISABLE_HOSTGROUP_PASSIVE_SVC_CHECKS":
		return 1
	case "SCHEDULE_HOSTGROUP_HOST_DOWNTIME":
		return 8
	case "SCHEDULE_HOSTGROUP_SVC_DOWNTIME":
		return 8
	case "ENABLE_SERVICEGROUP_HOST_NOTIFICATIONS", "DISABLE_SERVICEGROUP_HOST_NOTIFICATIONS",
		"ENABLE_SERVICEGROUP_SVC_NOTIFICATIONS", "DISABLE_SERVICEGROUP_SVC_NOTIFICATIONS",
		"ENABLE_SERVICEGROUP_HOST_CHECKS", "DISABLE_SERVICEGROUP_HOST_CHECKS",
		"ENABLE_SERVICEGROUP_PASSIVE_HOST_CHECKS", "DISABLE_SERVICEGROUP_PASSIVE_HOST_CHECKS",
		"ENABLE_SERVICEGROUP_SVC_CHECKS", "DISABLE_SERVICEGROUP_SVC_CHECKS",
		"ENABLE_SERVICEGROUP_PASSIVE_SVC_CHECKS", "DISABLE_SERVICEGROUP_PASSIVE_SVC_CHECKS":
		return 1
	case "SCHEDULE_SERVICEGROUP_HOST_DOWNTIME", "SCHEDULE_SERVICEGROUP_SVC_DOWNTIME":
		return 8
	case "ENABLE_CONTACT_HOST_NOTIFICATIONS", "DISABLE_CONTACT_HOST_NOTIFICATIONS":
		return 1
	case "ENABLE_CONTACT_SVC_NOTIFICATIONS", "DISABLE_CONTACT_SVC_NOTIFICATIONS":
		return 1
	case "CHANGE_CONTACT_HOST_NOTIFICATION_TIMEPERIOD", "CHANGE_CONTACT_SVC_NOTIFICATION_TIMEPERIOD":
		return 2
	case "CHANGE_CONTACT_MODATTR", "CHANGE_CONTACT_MODHATTR", "CHANGE_CONTACT_MODSATTR":
		return 2
	case "ENABLE_CONTACTGROUP_HOST_NOTIFICATIONS", "DISABLE_CONTACTGROUP_HOST_NOTIFICATIONS":
		return 1
	case "ENABLE_CONTACTGROUP_SVC_NOTIFICATIONS", "DISABLE_CONTACTGROUP_SVC_NOTIFICATIONS":
		return 1
	case "DEL_DOWNTIME_BY_HOST_NAME":
		return 4 // host;svc;start;comment (all optional except host)
	case "DEL_DOWNTIME_BY_HOSTGROUP_NAME":
		return 4
	case "DEL_DOWNTIME_BY_START_TIME_COMMENT":
		return 2
	case "SCHEDULE_AND_PROPAGATE_TRIGGERED_HOST_DOWNTIME":
		return 8
	case "SCHEDULE_AND_PROPAGATE_HOST_DOWNTIME":
		return 8
	case "ENABLE_HOST_AND_CHILD_NOTIFICATIONS", "DISABLE_HOST_AND_CHILD_NOTIFICATIONS":
		return 1
	case "ENABLE_ALL_NOTIFICATIONS_BEYOND_HOST", "DISABLE_ALL_NOTIFICATIONS_BEYOND_HOST":
		return 1
	case "START_OBSESSING_OVER_HOST", "STOP_OBSESSING_OVER_HOST":
		return 1
	case "START_OBSESSING_OVER_SVC", "STOP_OBSESSING_OVER_SVC":
		return 2
	case "CHANGE_HOST_MODATTR":
		return 2
	case "CHANGE_SVC_MODATTR":
		return 3
	case "PROCESS_FILE":
		return 2
	default:
		return 0
	}
}

// mkfifo creates a named pipe. On Unix systems this uses syscall.
func mkfifo(path string) error {
	return mkfifoImpl(path)
}
