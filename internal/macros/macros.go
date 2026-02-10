package macros

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Expander resolves $MACRO$ references in command lines.
type Expander struct {
	Cfg        *objects.Config
	HostLookup func(name string) *objects.Host
	SvcLookup  func(hostName, svcDesc string) *objects.Service
}

// Expand replaces all $MACRO$ references in the input string.
// host and svc provide context for host/service-specific macros (svc may be nil for host checks).
// args are the !-separated arguments from the check command definition.
func (e *Expander) Expand(input string, host *objects.Host, svc *objects.Service, args []string) string {
	var result strings.Builder
	result.Grow(len(input))

	i := 0
	for i < len(input) {
		if input[i] != '$' {
			result.WriteByte(input[i])
			i++
			continue
		}

		// Check for $$ (literal dollar)
		if i+1 < len(input) && input[i+1] == '$' {
			result.WriteByte('$')
			i += 2
			continue
		}

		// Find closing $
		end := strings.IndexByte(input[i+1:], '$')
		if end < 0 {
			result.WriteByte('$')
			i++
			continue
		}
		end += i + 1

		macroName := input[i+1 : end]
		resolved, ok := e.resolveMacro(macroName, host, svc, args)
		if ok {
			result.WriteString(resolved)
		} else {
			// Unknown macros left as-is
			result.WriteString(input[i : end+1])
		}
		i = end + 1
	}

	return result.String()
}

func (e *Expander) resolveMacro(name string, host *objects.Host, svc *objects.Service, args []string) (string, bool) {
	// $ARGn$ macros (1-32)
	if strings.HasPrefix(name, "ARG") {
		n, err := strconv.Atoi(name[3:])
		if err == nil && n >= 1 && n <= 32 {
			if n-1 < len(args) {
				return args[n-1], true
			}
			return "", true
		}
	}

	// $USERn$ macros (1-256)
	if strings.HasPrefix(name, "USER") {
		n, err := strconv.Atoi(name[4:])
		if err == nil && n >= 1 && n <= 256 {
			return e.Cfg.UserMacros[n-1], true
		}
	}

	// Custom variable macros
	if strings.HasPrefix(name, "_HOST") {
		varName := name[5:]
		if host != nil && host.CustomVars != nil {
			if v, ok := host.CustomVars[varName]; ok {
				return v, true
			}
			// Try case-insensitive
			for k, v := range host.CustomVars {
				if strings.EqualFold(k, varName) {
					return v, true
				}
			}
		}
		return "", true
	}
	if strings.HasPrefix(name, "_SERVICE") {
		varName := name[8:]
		if svc != nil && svc.CustomVars != nil {
			if v, ok := svc.CustomVars[varName]; ok {
				return v, true
			}
			for k, v := range svc.CustomVars {
				if strings.EqualFold(k, varName) {
					return v, true
				}
			}
		}
		return "", true
	}
	if strings.HasPrefix(name, "_CONTACT") {
		// Contact macros would need contact context; return empty for now
		return "", true
	}

	// On-demand host macros: $HOSTSTATE:hostname$
	if strings.Contains(name, ":") {
		return e.resolveOnDemand(name)
	}

	// Standard macros
	now := time.Now()
	switch name {
	// Host macros
	case "HOSTNAME":
		if host != nil {
			return host.Name, true
		}
	case "HOSTALIAS":
		if host != nil {
			return host.Alias, true
		}
	case "HOSTADDRESS":
		if host != nil {
			return host.Address, true
		}
	case "HOSTSTATE":
		if host != nil {
			return hostStateString(host.CurrentState), true
		}
	case "HOSTSTATEID":
		if host != nil {
			return strconv.Itoa(host.CurrentState), true
		}
	case "HOSTSTATETYPE":
		if host != nil {
			return stateTypeString(host.StateType), true
		}
	case "HOSTATTEMPT":
		if host != nil {
			return strconv.Itoa(host.CurrentAttempt), true
		}
	case "MAXHOSTATTEMPTS":
		if host != nil {
			return strconv.Itoa(host.MaxCheckAttempts), true
		}
	case "HOSTOUTPUT":
		if host != nil {
			return host.PluginOutput, true
		}
	case "LONGHOSTOUTPUT":
		if host != nil {
			return host.LongPluginOutput, true
		}
	case "HOSTPERFDATA":
		if host != nil {
			return host.PerfData, true
		}
	case "HOSTCHECKCOMMAND":
		if host != nil && host.CheckCommand != nil {
			return host.CheckCommand.Name, true
		}
	case "HOSTLATENCY":
		if host != nil {
			return fmt.Sprintf("%.3f", host.Latency), true
		}
	case "HOSTEXECUTIONTIME":
		if host != nil {
			return fmt.Sprintf("%.3f", host.ExecutionTime), true
		}
	case "HOSTDURATION":
		if host != nil {
			return formatDuration(now.Sub(host.LastStateChange)), true
		}
	case "HOSTDURATIONSEC":
		if host != nil {
			return strconv.FormatInt(int64(now.Sub(host.LastStateChange).Seconds()), 10), true
		}
	case "HOSTDOWNTIME":
		if host != nil {
			return strconv.Itoa(host.ScheduledDowntimeDepth), true
		}
	case "HOSTPERCENTCHANGE":
		if host != nil {
			return fmt.Sprintf("%.2f", host.PercentStateChange), true
		}
	case "LASTHOSTCHECK":
		if host != nil {
			return strconv.FormatInt(host.LastCheck.Unix(), 10), true
		}
	case "LASTHOSTSTATECHANGE":
		if host != nil {
			return strconv.FormatInt(host.LastStateChange.Unix(), 10), true
		}
	case "LASTHOSTUP":
		if host != nil {
			return strconv.FormatInt(host.LastTimeUp.Unix(), 10), true
		}
	case "LASTHOSTDOWN":
		if host != nil {
			return strconv.FormatInt(host.LastTimeDown.Unix(), 10), true
		}
	case "LASTHOSTUNREACHABLE":
		if host != nil {
			return strconv.FormatInt(host.LastTimeUnreachable.Unix(), 10), true
		}
	case "HOSTNOTIFICATIONNUMBER":
		if host != nil {
			return strconv.Itoa(host.CurrentNotificationNumber), true
		}
	case "HOSTNOTIFICATIONID":
		if host != nil {
			return strconv.FormatUint(host.CurrentEventID, 10), true
		}
	case "HOSTNOTES":
		if host != nil {
			return host.Notes, true
		}
	case "HOSTNOTESURL":
		if host != nil {
			return host.NotesURL, true
		}
	case "HOSTACTIONURL":
		if host != nil {
			return host.ActionURL, true
		}
	case "HOSTDISPLAYNAME":
		if host != nil {
			if host.DisplayName != "" {
				return host.DisplayName, true
			}
			return host.Name, true
		}
	case "TOTALHOSTSERVICES":
		if host != nil {
			return strconv.Itoa(len(host.Services)), true
		}
	case "TOTALHOSTSERVICESOK":
		if host != nil {
			return strconv.Itoa(countServicesByState(host.Services, objects.ServiceOK)), true
		}
	case "TOTALHOSTSERVICESWARNING":
		if host != nil {
			return strconv.Itoa(countServicesByState(host.Services, objects.ServiceWarning)), true
		}
	case "TOTALHOSTSERVICESCRITICAL":
		if host != nil {
			return strconv.Itoa(countServicesByState(host.Services, objects.ServiceCritical)), true
		}
	case "TOTALHOSTSERVICESUNKNOWN":
		if host != nil {
			return strconv.Itoa(countServicesByState(host.Services, objects.ServiceUnknown)), true
		}

	// Service macros
	case "SERVICEDESC":
		if svc != nil {
			return svc.Description, true
		}
	case "SERVICEDISPLAYNAME":
		if svc != nil {
			if svc.DisplayName != "" {
				return svc.DisplayName, true
			}
			return svc.Description, true
		}
	case "SERVICESTATE":
		if svc != nil {
			return serviceStateString(svc.CurrentState), true
		}
	case "SERVICESTATEID":
		if svc != nil {
			return strconv.Itoa(svc.CurrentState), true
		}
	case "SERVICESTATETYPE":
		if svc != nil {
			return stateTypeString(svc.StateType), true
		}
	case "SERVICEATTEMPT":
		if svc != nil {
			return strconv.Itoa(svc.CurrentAttempt), true
		}
	case "MAXSERVICEATTEMPTS":
		if svc != nil {
			return strconv.Itoa(svc.MaxCheckAttempts), true
		}
	case "SERVICEOUTPUT":
		if svc != nil {
			return svc.PluginOutput, true
		}
	case "LONGSERVICEOUTPUT":
		if svc != nil {
			return svc.LongPluginOutput, true
		}
	case "SERVICEPERFDATA":
		if svc != nil {
			return svc.PerfData, true
		}
	case "SERVICECHECKCOMMAND":
		if svc != nil && svc.CheckCommand != nil {
			return svc.CheckCommand.Name, true
		}
	case "SERVICELATENCY":
		if svc != nil {
			return fmt.Sprintf("%.3f", svc.Latency), true
		}
	case "SERVICEEXECUTIONTIME":
		if svc != nil {
			return fmt.Sprintf("%.3f", svc.ExecutionTime), true
		}
	case "SERVICEDURATION":
		if svc != nil {
			return formatDuration(now.Sub(svc.LastStateChange)), true
		}
	case "SERVICEDURATIONSEC":
		if svc != nil {
			return strconv.FormatInt(int64(now.Sub(svc.LastStateChange).Seconds()), 10), true
		}
	case "SERVICEDOWNTIME":
		if svc != nil {
			return strconv.Itoa(svc.ScheduledDowntimeDepth), true
		}
	case "SERVICEPERCENTCHANGE":
		if svc != nil {
			return fmt.Sprintf("%.2f", svc.PercentStateChange), true
		}
	case "LASTSERVICECHECK":
		if svc != nil {
			return strconv.FormatInt(svc.LastCheck.Unix(), 10), true
		}
	case "LASTSERVICESTATECHANGE":
		if svc != nil {
			return strconv.FormatInt(svc.LastStateChange.Unix(), 10), true
		}
	case "LASTSERVICEOK":
		if svc != nil {
			return strconv.FormatInt(svc.LastTimeOK.Unix(), 10), true
		}
	case "LASTSERVICEWARNING":
		if svc != nil {
			return strconv.FormatInt(svc.LastTimeWarning.Unix(), 10), true
		}
	case "LASTSERVICECRITICAL":
		if svc != nil {
			return strconv.FormatInt(svc.LastTimeCritical.Unix(), 10), true
		}
	case "LASTSERVICEUNKNOWN":
		if svc != nil {
			return strconv.FormatInt(svc.LastTimeUnknown.Unix(), 10), true
		}
	case "SERVICENOTIFICATIONNUMBER":
		if svc != nil {
			return strconv.Itoa(svc.CurrentNotificationNumber), true
		}
	case "SERVICENOTIFICATIONID":
		if svc != nil {
			return strconv.FormatUint(svc.CurrentEventID, 10), true
		}
	case "SERVICEISVOLATILE":
		if svc != nil {
			if svc.IsVolatile {
				return "1", true
			}
			return "0", true
		}
	case "SERVICENOTES":
		if svc != nil {
			return svc.Notes, true
		}
	case "SERVICENOTESURL":
		if svc != nil {
			return svc.NotesURL, true
		}
	case "SERVICEACTIONURL":
		if svc != nil {
			return svc.ActionURL, true
		}

	// Date/time macros
	case "LONGDATETIME":
		return now.Format("Mon Jan 02 15:04:05 MST 2006"), true
	case "SHORTDATETIME":
		return now.Format("01-02-2006 15:04:05"), true
	case "DATE":
		return now.Format("01-02-2006"), true
	case "TIME":
		return now.Format("15:04:05"), true
	case "TIMET":
		return strconv.FormatInt(now.Unix(), 10), true
	case "PROCESSSTARTTIME":
		// Would need engine start time; return current for now
		return strconv.FormatInt(now.Unix(), 10), true
	}

	return "", false
}

func (e *Expander) resolveOnDemand(name string) (string, bool) {
	parts := strings.SplitN(name, ":", 2)
	if len(parts) != 2 {
		return "", false
	}
	macroBase := parts[0]
	target := parts[1]

	// $HOSTSTATE:hostname$ style
	if strings.HasPrefix(macroBase, "HOST") && e.HostLookup != nil {
		host := e.HostLookup(target)
		if host == nil {
			return "", false
		}
		return e.resolveMacro(macroBase, host, nil, nil)
	}

	// $SERVICESTATE:host:svc$ style
	if strings.HasPrefix(macroBase, "SERVICE") {
		colonIdx := strings.Index(target, ":")
		if colonIdx < 0 {
			return "", false
		}
		hostName := target[:colonIdx]
		svcDesc := target[colonIdx+1:]
		if e.SvcLookup != nil {
			svc := e.SvcLookup(hostName, svcDesc)
			if svc == nil {
				return "", false
			}
			var host *objects.Host
			if e.HostLookup != nil {
				host = e.HostLookup(hostName)
			}
			return e.resolveMacro(macroBase, host, svc, nil)
		}
	}

	return "", false
}

// SplitCommandArgs splits "command_name!arg1!arg2!arg3" into command name and args.
func SplitCommandArgs(checkCommand string) (string, []string) {
	parts := strings.Split(checkCommand, "!")
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) == 1 {
		return parts[0], nil
	}
	return parts[0], parts[1:]
}

func hostStateString(state int) string {
	switch state {
	case objects.HostUp:
		return "UP"
	case objects.HostDown:
		return "DOWN"
	case objects.HostUnreachable:
		return "UNREACHABLE"
	default:
		return "UNKNOWN"
	}
}

func serviceStateString(state int) string {
	switch state {
	case objects.ServiceOK:
		return "OK"
	case objects.ServiceWarning:
		return "WARNING"
	case objects.ServiceCritical:
		return "CRITICAL"
	case objects.ServiceUnknown:
		return "UNKNOWN"
	default:
		return "UNKNOWN"
	}
}

func stateTypeString(st int) string {
	if st == objects.StateTypeHard {
		return "HARD"
	}
	return "SOFT"
}

func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	secs := int(d.Seconds()) % 60
	return fmt.Sprintf("%dd %dh %dm %ds", days, hours, mins, secs)
}

func countServicesByState(svcs []*objects.Service, state int) int {
	n := 0
	for _, s := range svcs {
		if s.CurrentState == state && s.HasBeenChecked {
			n++
		}
	}
	return n
}
