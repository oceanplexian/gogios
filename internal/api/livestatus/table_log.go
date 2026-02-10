package livestatus

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
)

// logEntry represents a parsed Nagios log line.
type logEntry struct {
	Time               time.Time
	Class              int
	Type               string
	State              int
	HostName           string
	ServiceDescription string
	PluginOutput       string
	Message            string
	Options            string
	StateType          string
	ContactName        string
}

func logTable() *Table {
	return &Table{
		Name: "log",
		GetRows: func(p *api.StateProvider) []interface{} {
			if p.LogFile == "" {
				return nil
			}
			entries := parseLogFile(p.LogFile)
			rows := make([]interface{}, len(entries))
			for i, e := range entries {
				rows[i] = e
			}
			return rows
		},
		Columns: map[string]*Column{
			"time": {Name: "time", Type: "time", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).Time
			}},
			"class": {Name: "class", Type: "int", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).Class
			}},
			"type": {Name: "type", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).Type
			}},
			"state": {Name: "state", Type: "int", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).State
			}},
			"host_name": {Name: "host_name", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).HostName
			}},
			"service_description": {Name: "service_description", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).ServiceDescription
			}},
			"plugin_output": {Name: "plugin_output", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).PluginOutput
			}},
			"message": {Name: "message", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).Message
			}},
			"options": {Name: "options", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).Options
			}},
			"state_type": {Name: "state_type", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).StateType
			}},
			"contact_name": {Name: "contact_name", Type: "string", Extract: func(r interface{}) interface{} {
				return r.(*logEntry).ContactName
			}},
		},
	}
}

func parseLogFile(path string) []*logEntry {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []*logEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		e := parseLogLine(line)
		if e != nil {
			entries = append(entries, e)
		}
	}
	return entries
}

func parseLogLine(line string) *logEntry {
	// Format: [timestamp] TYPE: details
	if len(line) < 14 || line[0] != '[' {
		return nil
	}
	closeBracket := strings.Index(line, "]")
	if closeBracket < 2 {
		return nil
	}
	tsStr := line[1:closeBracket]
	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return nil
	}

	rest := strings.TrimSpace(line[closeBracket+1:])
	e := &logEntry{
		Time:    time.Unix(ts, 0),
		Message: line,
	}

	// Extract type label (everything before the colon)
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		e.Type = rest
		e.Class = classifyLogType(rest)
		return e
	}

	e.Type = strings.TrimSpace(rest[:colonIdx])
	detail := strings.TrimSpace(rest[colonIdx+1:])
	e.Class = classifyLogType(e.Type)

	switch e.Type {
	case "HOST ALERT":
		parseHostAlert(e, detail)
	case "SERVICE ALERT":
		parseServiceAlert(e, detail)
	case "HOST NOTIFICATION":
		parseHostNotification(e, detail)
	case "SERVICE NOTIFICATION":
		parseServiceNotification(e, detail)
	case "INITIAL HOST STATE", "CURRENT HOST STATE":
		parseHostAlert(e, detail)
	case "INITIAL SERVICE STATE", "CURRENT SERVICE STATE":
		parseServiceAlert(e, detail)
	case "HOST DOWNTIME ALERT":
		parseDowntimeAlert(e, detail, true)
	case "SERVICE DOWNTIME ALERT":
		parseDowntimeAlert(e, detail, false)
	case "EXTERNAL COMMAND":
		e.Options = detail
	}

	return e
}

// parseHostAlert: hostname;state;state_type;attempt;output
func parseHostAlert(e *logEntry, detail string) {
	parts := strings.SplitN(detail, ";", 5)
	if len(parts) < 4 {
		return
	}
	e.HostName = parts[0]
	e.State = hostStateFromName(parts[1])
	e.StateType = parts[2]
	if len(parts) >= 5 {
		e.PluginOutput = parts[4]
	}
}

// parseServiceAlert: hostname;svc;state;state_type;attempt;output
func parseServiceAlert(e *logEntry, detail string) {
	parts := strings.SplitN(detail, ";", 6)
	if len(parts) < 5 {
		return
	}
	e.HostName = parts[0]
	e.ServiceDescription = parts[1]
	e.State = serviceStateFromName(parts[2])
	e.StateType = parts[3]
	if len(parts) >= 6 {
		e.PluginOutput = parts[5]
	}
}

// parseHostNotification: contact;hostname;state;command;output
func parseHostNotification(e *logEntry, detail string) {
	parts := strings.SplitN(detail, ";", 5)
	if len(parts) < 3 {
		return
	}
	e.ContactName = parts[0]
	e.HostName = parts[1]
	e.State = hostStateFromName(parts[2])
	if len(parts) >= 5 {
		e.PluginOutput = parts[4]
	}
}

// parseServiceNotification: contact;hostname;svc;state;command;output
func parseServiceNotification(e *logEntry, detail string) {
	parts := strings.SplitN(detail, ";", 6)
	if len(parts) < 4 {
		return
	}
	e.ContactName = parts[0]
	e.HostName = parts[1]
	e.ServiceDescription = parts[2]
	e.State = serviceStateFromName(parts[3])
	if len(parts) >= 6 {
		e.PluginOutput = parts[5]
	}
}

func parseDowntimeAlert(e *logEntry, detail string, isHost bool) {
	parts := strings.SplitN(detail, ";", 4)
	if len(parts) < 2 {
		return
	}
	e.HostName = parts[0]
	if !isHost && len(parts) >= 3 {
		e.ServiceDescription = parts[1]
		e.Options = parts[2]
	} else if len(parts) >= 2 {
		e.Options = parts[1]
	}
}

func classifyLogType(typ string) int {
	switch typ {
	case "HOST ALERT", "SERVICE ALERT":
		return 1
	case "HOST NOTIFICATION", "SERVICE NOTIFICATION":
		return 3
	case "PASSIVE HOST CHECK", "PASSIVE SERVICE CHECK":
		return 4
	case "EXTERNAL COMMAND":
		return 5
	case "INITIAL HOST STATE", "INITIAL SERVICE STATE",
		"CURRENT HOST STATE", "CURRENT SERVICE STATE":
		return 6
	case "LOG ROTATION", "LOG VERSION":
		return 7
	case "HOST DOWNTIME ALERT", "SERVICE DOWNTIME ALERT",
		"HOST FLAPPING ALERT", "SERVICE FLAPPING ALERT",
		"TIMEPERIOD TRANSITION":
		return 2
	default:
		return 0
	}
}

func hostStateFromName(s string) int {
	switch strings.TrimSpace(strings.ToUpper(s)) {
	case "UP", "RECOVERY":
		return 0
	case "DOWN":
		return 1
	case "UNREACHABLE":
		return 2
	default:
		return 0
	}
}

func serviceStateFromName(s string) int {
	switch strings.TrimSpace(strings.ToUpper(s)) {
	case "OK", "RECOVERY":
		return 0
	case "WARNING":
		return 1
	case "CRITICAL":
		return 2
	case "UNKNOWN":
		return 3
	default:
		return 0
	}
}
