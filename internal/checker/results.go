package checker

import (
	"strings"

	"github.com/oceanplexian/gogios/internal/objects"
)

// ParsedOutput contains the parsed components of plugin output.
type ParsedOutput struct {
	ShortOutput string
	LongOutput  string
	PerfData    string
}

// ParseCheckOutput parses plugin output into short output, long output, and perfdata.
//
// Format:
//
//	SHORT OUTPUT | perfdata
//	LONG OUTPUT LINE 1
//	LONG OUTPUT LINE 2
//	| more perfdata
//	more perfdata lines
//
// Semicolons in plugin output (NOT perfdata) are replaced with colons.
func ParseCheckOutput(raw string) ParsedOutput {
	if raw == "" {
		return ParsedOutput{}
	}

	lines := strings.Split(raw, "\n")
	var p ParsedOutput
	var longLines []string
	var perfLines []string
	inPerfData := false

	for i, line := range lines {
		if i == 0 {
			// First line: split on first |
			if idx := strings.Index(line, "|"); idx >= 0 {
				p.ShortOutput = strings.TrimSpace(line[:idx])
				perfLines = append(perfLines, strings.TrimSpace(line[idx+1:]))
			} else {
				p.ShortOutput = strings.TrimSpace(line)
			}
			continue
		}

		if inPerfData {
			perfLines = append(perfLines, strings.TrimSpace(line))
			continue
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "|") {
			inPerfData = true
			rest := strings.TrimSpace(trimmed[1:])
			if rest != "" {
				perfLines = append(perfLines, rest)
			}
			continue
		}

		if idx := strings.Index(line, "|"); idx >= 0 {
			longLines = append(longLines, strings.TrimSpace(line[:idx]))
			inPerfData = true
			rest := strings.TrimSpace(line[idx+1:])
			if rest != "" {
				perfLines = append(perfLines, rest)
			}
			continue
		}

		longLines = append(longLines, line)
	}

	// Replace semicolons with colons in plugin output (NOT perfdata)
	p.ShortOutput = strings.ReplaceAll(p.ShortOutput, ";", ":")
	for i, l := range longLines {
		longLines[i] = strings.ReplaceAll(l, ";", ":")
	}

	p.LongOutput = strings.Join(longLines, "\\n")
	p.PerfData = strings.Join(perfLines, " ")

	return p
}

// GetServiceCheckReturnCode maps a raw return code to a service state.
func GetServiceCheckReturnCode(cr *objects.CheckResult, timeoutState int) int {
	if cr.EarlyTimeout {
		return timeoutState
	}
	if !cr.ExitedOK {
		return objects.ServiceCritical
	}
	switch cr.ReturnCode {
	case 0:
		return objects.ServiceOK
	case 1:
		return objects.ServiceWarning
	case 2:
		return objects.ServiceCritical
	case 3:
		return objects.ServiceUnknown
	case 126, 127:
		return objects.ServiceCritical
	default:
		return objects.ServiceCritical
	}
}

// GetHostCheckReturnCode maps a raw return code to a host state.
func GetHostCheckReturnCode(cr *objects.CheckResult, aggressiveHostChecking bool) int {
	if cr.EarlyTimeout || !cr.ExitedOK {
		return objects.HostDown
	}
	switch cr.ReturnCode {
	case 0:
		return objects.HostUp
	case 1:
		if aggressiveHostChecking {
			return objects.HostDown
		}
		return objects.HostUp
	case 2, 3:
		return objects.HostDown
	default:
		return objects.HostDown
	}
}

// GetPassiveHostCheckReturnCode maps passive host check return codes directly.
func GetPassiveHostCheckReturnCode(returnCode int) int {
	switch returnCode {
	case 0:
		return objects.HostUp
	case 1:
		return objects.HostDown
	case 2:
		return objects.HostUnreachable
	default:
		return objects.HostDown
	}
}

// AugmentReturnCodeOutput adds special messages for return codes 126/127.
func AugmentReturnCodeOutput(cr *objects.CheckResult) string {
	output := cr.Output
	switch cr.ReturnCode {
	case 126:
		if output == "" {
			output = "(Return code of 126 is out of bounds - plugin may not be executable)"
		}
	case 127:
		if output == "" {
			output = "(Return code of 127 is out of bounds - plugin may be missing)"
		}
	}
	return output
}
