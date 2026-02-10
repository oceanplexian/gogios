package livestatus

import (
	"os"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
)

// statusRow wraps the provider so we have a single-row "table".
type statusRow struct {
	p *api.StateProvider
}

func statusTable() *Table {
	return &Table{
		Name: "status",
		GetRows: func(p *api.StateProvider) []interface{} {
			return []interface{}{&statusRow{p: p}}
		},
		Columns: map[string]*Column{
			"program_start": {Name: "program_start", Type: "time", Extract: func(r interface{}) interface{} {
				return r.(*statusRow).p.Global.ProgramStart
			}},
			"program_version": {Name: "program_version", Type: "string", Extract: func(r interface{}) interface{} {
				return "Gogios 1.0.0"
			}},
			"livestatus_version": {Name: "livestatus_version", Type: "string", Extract: func(r interface{}) interface{} {
				return "1.0.0"
			}},
			"nagios_pid": {Name: "nagios_pid", Type: "int", Extract: func(r interface{}) interface{} {
				return os.Getpid()
			}},
			"interval_length": {Name: "interval_length", Type: "int", Extract: func(r interface{}) interface{} {
				return r.(*statusRow).p.Global.IntervalLength
			}},
			"enable_notifications": {Name: "enable_notifications", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.EnableNotifications)
			}},
			"execute_service_checks": {Name: "execute_service_checks", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.ExecuteServiceChecks)
			}},
			"execute_host_checks": {Name: "execute_host_checks", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.ExecuteHostChecks)
			}},
			"accept_passive_host_checks": {Name: "accept_passive_host_checks", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.AcceptPassiveHostChecks)
			}},
			"accept_passive_service_checks": {Name: "accept_passive_service_checks", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.AcceptPassiveServiceChecks)
			}},
			"enable_event_handlers": {Name: "enable_event_handlers", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.EnableEventHandlers)
			}},
			"enable_flap_detection": {Name: "enable_flap_detection", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.EnableFlapDetection)
			}},
			"obsess_over_hosts": {Name: "obsess_over_hosts", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.ObsessOverHosts)
			}},
			"obsess_over_services": {Name: "obsess_over_services", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.ObsessOverServices)
			}},
			"check_host_freshness": {Name: "check_host_freshness", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.CheckHostFreshness)
			}},
			"check_service_freshness": {Name: "check_service_freshness", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.CheckServiceFreshness)
			}},
			"process_performance_data": {Name: "process_performance_data", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*statusRow).p.Global.ProcessPerformanceData)
			}},
			"check_external_commands": {Name: "check_external_commands", Type: "int", Extract: func(r interface{}) interface{} {
				return 1 // always enabled
			}},
			"last_command_check": {Name: "last_command_check", Type: "time", Extract: func(r interface{}) interface{} {
				return time.Now()
			}},
			"last_log_rotation": {Name: "last_log_rotation", Type: "time", Extract: func(r interface{}) interface{} {
				return time.Time{} // not tracked
			}},
			// Performance stats stubs — Thruk queries these
			"connections":         {Name: "connections", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"connections_rate":    {Name: "connections_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
			"requests":            {Name: "requests", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"requests_rate":       {Name: "requests_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
			"host_checks":         {Name: "host_checks", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"host_checks_rate":    {Name: "host_checks_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
			"service_checks":      {Name: "service_checks", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"service_checks_rate": {Name: "service_checks_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
			"cached_log_messages": {Name: "cached_log_messages", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"neb_callbacks":       {Name: "neb_callbacks", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"neb_callbacks_rate":  {Name: "neb_callbacks_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
			"log_messages":        {Name: "log_messages", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"log_messages_rate":   {Name: "log_messages_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
			"forks":               {Name: "forks", Type: "int", Extract: func(r interface{}) interface{} { return 0 }},
			"forks_rate":          {Name: "forks_rate", Type: "float", Extract: func(r interface{}) interface{} { return 0.0 }},
		},
	}
}
