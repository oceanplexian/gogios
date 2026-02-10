// Package perfdata handles performance data file writing and command execution.
package perfdata

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// Processor handles performance data output.
type Processor struct {
	Global *objects.GlobalState

	// File handles (opened once, reused)
	hostFile    *os.File
	serviceFile *os.File
}

// NewProcessor creates a new perfdata processor.
func NewProcessor(gs *objects.GlobalState) *Processor {
	return &Processor{Global: gs}
}

// OpenFiles opens the perfdata files for writing.
func (p *Processor) OpenFiles() error {
	var err error
	if p.Global.HostPerfdataFile != "" {
		p.hostFile, err = openPerfdataFile(p.Global.HostPerfdataFile, p.Global.HostPerfdataFileMode)
		if err != nil {
			return err
		}
	}
	if p.Global.ServicePerfdataFile != "" {
		p.serviceFile, err = openPerfdataFile(p.Global.ServicePerfdataFile, p.Global.ServicePerfdataFileMode)
		if err != nil {
			return err
		}
	}
	return nil
}

// Close closes any open perfdata files.
func (p *Processor) Close() {
	if p.hostFile != nil {
		p.hostFile.Close()
		p.hostFile = nil
	}
	if p.serviceFile != nil {
		p.serviceFile.Close()
		p.serviceFile = nil
	}
}

// UpdateHostPerfdata processes host check performance data.
func (p *Processor) UpdateHostPerfdata(h *objects.Host) {
	if !p.Global.ProcessPerformanceData || !h.ProcessPerfData {
		return
	}
	if !p.Global.HostPerfdataProcessEmptyResults && h.PerfData == "" {
		return
	}

	macros := hostMacros(h)

	// Perfdata command
	if p.Global.HostPerfdataCommand != "" {
		cmdLine := expandMacros(p.Global.HostPerfdataCommand, macros)
		go runCommand(cmdLine, 30*time.Second)
	}

	// Perfdata file
	if p.hostFile != nil && p.Global.HostPerfdataFileTemplate != "" {
		line := expandMacros(p.Global.HostPerfdataFileTemplate, macros)
		p.hostFile.WriteString(line + "\n")
	}
}

// UpdateServicePerfdata processes service check performance data.
func (p *Processor) UpdateServicePerfdata(s *objects.Service) {
	if !p.Global.ProcessPerformanceData || !s.ProcessPerfData {
		return
	}
	if !p.Global.ServicePerfdataProcessEmptyResults && s.PerfData == "" {
		return
	}

	macros := serviceMacros(s)

	if p.Global.ServicePerfdataCommand != "" {
		cmdLine := expandMacros(p.Global.ServicePerfdataCommand, macros)
		go runCommand(cmdLine, 30*time.Second)
	}

	if p.serviceFile != nil && p.Global.ServicePerfdataFileTemplate != "" {
		line := expandMacros(p.Global.ServicePerfdataFileTemplate, macros)
		p.serviceFile.WriteString(line + "\n")
	}
}

// RunHostFileProcessingCommand runs the host perfdata file processing command.
func (p *Processor) RunHostFileProcessingCommand() {
	if p.Global.HostPerfdataFileProcessingCommand == "" {
		return
	}
	// Close and reopen if in write mode
	if p.hostFile != nil && p.Global.HostPerfdataFileMode == objects.PerfdataFileWrite {
		p.hostFile.Close()
		p.hostFile = nil
	}
	runCommand(p.Global.HostPerfdataFileProcessingCommand, 60*time.Second)
	if p.Global.HostPerfdataFile != "" && p.Global.HostPerfdataFileMode == objects.PerfdataFileWrite {
		var err error
		p.hostFile, err = openPerfdataFile(p.Global.HostPerfdataFile, p.Global.HostPerfdataFileMode)
		if err != nil {
			p.hostFile = nil
		}
	}
}

// RunServiceFileProcessingCommand runs the service perfdata file processing command.
func (p *Processor) RunServiceFileProcessingCommand() {
	if p.Global.ServicePerfdataFileProcessingCommand == "" {
		return
	}
	if p.serviceFile != nil && p.Global.ServicePerfdataFileMode == objects.PerfdataFileWrite {
		p.serviceFile.Close()
		p.serviceFile = nil
	}
	runCommand(p.Global.ServicePerfdataFileProcessingCommand, 60*time.Second)
	if p.Global.ServicePerfdataFile != "" && p.Global.ServicePerfdataFileMode == objects.PerfdataFileWrite {
		var err error
		p.serviceFile, err = openPerfdataFile(p.Global.ServicePerfdataFile, p.Global.ServicePerfdataFileMode)
		if err != nil {
			p.serviceFile = nil
		}
	}
}

func openPerfdataFile(path string, mode int) (*os.File, error) {
	switch mode {
	case objects.PerfdataFileWrite:
		return os.Create(path)
	case objects.PerfdataFilePipe:
		return os.OpenFile(path, os.O_WRONLY, 0)
	default: // append
		return os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
}

func runCommand(cmdLine string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, "/bin/sh", "-c", cmdLine).Run()
}

func expandMacros(template string, macros map[string]string) string {
	result := template
	for k, v := range macros {
		result = strings.ReplaceAll(result, "$"+k+"$", v)
	}
	return result
}

func hostMacros(h *objects.Host) map[string]string {
	return map[string]string{
		"HOSTNAME":         h.Name,
		"HOSTALIAS":        h.Alias,
		"HOSTADDRESS":      h.Address,
		"HOSTSTATE":        objects.HostStateName(h.CurrentState),
		"HOSTSTATETYPE":    objects.StateTypeName(h.StateType),
		"HOSTOUTPUT":       h.PluginOutput,
		"LONGHOSTOUTPUT":   h.LongPluginOutput,
		"HOSTPERFDATA":     h.PerfData,
		"HOSTCHECKCOMMAND": cmdStr(h.CheckCommand),
	}
}

func serviceMacros(s *objects.Service) map[string]string {
	hostName := ""
	hostAlias := ""
	hostAddr := ""
	if s.Host != nil {
		hostName = s.Host.Name
		hostAlias = s.Host.Alias
		hostAddr = s.Host.Address
	}
	return map[string]string{
		"HOSTNAME":             hostName,
		"HOSTALIAS":            hostAlias,
		"HOSTADDRESS":          hostAddr,
		"SERVICEDESC":          s.Description,
		"SERVICESTATE":         objects.ServiceStateName(s.CurrentState),
		"SERVICESTATETYPE":     objects.StateTypeName(s.StateType),
		"SERVICEOUTPUT":        s.PluginOutput,
		"LONGSERVICEOUTPUT":    s.LongPluginOutput,
		"SERVICEPERFDATA":      s.PerfData,
		"SERVICECHECKCOMMAND":  cmdStr(s.CheckCommand),
	}
}

func cmdStr(cmd *objects.Command) string {
	if cmd == nil {
		return ""
	}
	return cmd.Name
}
