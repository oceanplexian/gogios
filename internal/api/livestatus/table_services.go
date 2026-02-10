package livestatus

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func servicesTable() *Table {
	return &Table{
		Name: "services",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.Services))
			for i, svc := range p.Store.Services {
				rows[i] = svc
			}
			return rows
		},
		Columns: map[string]*Column{
			"host_name":        {Name: "host_name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.Name }},
			"host_display_name": {Name: "host_display_name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.DisplayName }},
			"host_alias":       {Name: "host_alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.Alias }},
			"host_address":     {Name: "host_address", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.Address }},
			"host_state":       {Name: "host_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.CurrentState }},
			"host_has_been_checked": {Name: "host_has_been_checked", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.HasBeenChecked) }},
			"host_acknowledged": {Name: "host_acknowledged", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.ProblemAcknowledged) }},
			"host_scheduled_downtime_depth": {Name: "host_scheduled_downtime_depth", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.ScheduledDowntimeDepth }},
			"host_notifications_enabled": {Name: "host_notifications_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.NotificationsEnabled) }},
			"host_active_checks_enabled": {Name: "host_active_checks_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.ActiveChecksEnabled) }},
			"host_accept_passive_checks": {Name: "host_accept_passive_checks", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.PassiveChecksEnabled) }},
			"host_icon_image": {Name: "host_icon_image", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.IconImage }},
			"host_notes_url":  {Name: "host_notes_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.NotesURL }},
			"host_action_url": {Name: "host_action_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.ActionURL }},
			"host_groups": {Name: "host_groups", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, hg := range r.(*objects.Service).Host.HostGroups {
					names = append(names, hg.Name)
				}
				return names
			}},
			"description":     {Name: "description", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Description }},
			"display_name":    {Name: "display_name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).DisplayName }},
			"state":           {Name: "state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).CurrentState }},
			"state_type":      {Name: "state_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).StateType }},
			"plugin_output":   {Name: "plugin_output", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).PluginOutput }},
			"long_plugin_output": {Name: "long_plugin_output", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LongPluginOutput }},
			"perf_data":       {Name: "perf_data", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).PerfData }},
			"has_been_checked": {Name: "has_been_checked", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).HasBeenChecked) }},
			"current_attempt": {Name: "current_attempt", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).CurrentAttempt }},
			"max_check_attempts": {Name: "max_check_attempts", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).MaxCheckAttempts }},
			"last_check":      {Name: "last_check", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastCheck }},
			"next_check":      {Name: "next_check", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).NextCheck }},
			"last_state_change": {Name: "last_state_change", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastStateChange }},
			"last_hard_state_change": {Name: "last_hard_state_change", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastHardStateChange }},
			"last_hard_state": {Name: "last_hard_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastHardState }},
			"last_time_ok":      {Name: "last_time_ok", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastTimeOK }},
			"last_time_warning": {Name: "last_time_warning", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastTimeWarning }},
			"last_time_critical": {Name: "last_time_critical", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastTimeCritical }},
			"last_time_unknown": {Name: "last_time_unknown", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastTimeUnknown }},
			"check_command": {Name: "check_command", Type: "string", Extract: func(r interface{}) interface{} {
				svc := r.(*objects.Service)
				return commandStr(svc.CheckCommand, svc.CheckCommandArgs)
			}},
			"check_interval":    {Name: "check_interval", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).CheckInterval }},
			"retry_interval":    {Name: "retry_interval", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).RetryInterval }},
			"check_period": {Name: "check_period", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Service).CheckPeriod != nil {
					return r.(*objects.Service).CheckPeriod.Name
				}
				return ""
			}},
			"notification_period": {Name: "notification_period", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Service).NotificationPeriod != nil {
					return r.(*objects.Service).NotificationPeriod.Name
				}
				return ""
			}},
			"notifications_enabled": {Name: "notifications_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).NotificationsEnabled) }},
			"notification_interval": {Name: "notification_interval", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).NotificationInterval }},
			"active_checks_enabled": {Name: "active_checks_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).ActiveChecksEnabled) }},
			"accept_passive_checks": {Name: "accept_passive_checks", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).PassiveChecksEnabled) }},
			"obsess_over_service":   {Name: "obsess_over_service", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).ObsessOver) }},
			"is_volatile":           {Name: "is_volatile", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).IsVolatile) }},
			"event_handler_enabled": {Name: "event_handler_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).EventHandlerEnabled) }},
			"event_handler": {Name: "event_handler", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Service).EventHandler != nil {
					return r.(*objects.Service).EventHandler.Name
				}
				return ""
			}},
			"check_freshness":       {Name: "check_freshness", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).CheckFreshness) }},
			"freshness_threshold":   {Name: "freshness_threshold", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).FreshnessThreshold }},
			"flap_detection_enabled": {Name: "flap_detection_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).FlapDetectionEnabled) }},
			"is_flapping":           {Name: "is_flapping", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).IsFlapping) }},
			"percent_state_change":  {Name: "percent_state_change", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).PercentStateChange }},
			"latency":               {Name: "latency", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Latency }},
			"execution_time":        {Name: "execution_time", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).ExecutionTime }},
			"process_performance_data": {Name: "process_performance_data", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).ProcessPerfData) }},
			"scheduled_downtime_depth": {Name: "scheduled_downtime_depth", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).ScheduledDowntimeDepth }},
			"acknowledged":          {Name: "acknowledged", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).ProblemAcknowledged) }},
			"acknowledgement_type":  {Name: "acknowledgement_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).AckType }},
			"notes":                 {Name: "notes", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Notes }},
			"notes_url":             {Name: "notes_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).NotesURL }},
			"notes_url_expanded":    {Name: "notes_url_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).NotesURL }},
			"action_url":            {Name: "action_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).ActionURL }},
			"action_url_expanded":   {Name: "action_url_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).ActionURL }},
			"icon_image":            {Name: "icon_image", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).IconImage }},
			"icon_image_alt":        {Name: "icon_image_alt", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).IconImageAlt }},
			"icon_image_expanded":   {Name: "icon_image_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).IconImage }},
			"contact_groups": {Name: "contact_groups", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, cg := range r.(*objects.Service).ContactGroups {
					names = append(names, cg.Name)
				}
				return names
			}},
			"contacts": {Name: "contacts", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, c := range r.(*objects.Service).Contacts {
					names = append(names, c.Name)
				}
				return names
			}},
			"groups": {Name: "groups", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, sg := range r.(*objects.Service).ServiceGroups {
					names = append(names, sg.Name)
				}
				return names
			}},
			"custom_variable_names": {Name: "custom_variable_names", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for k := range r.(*objects.Service).CustomVars {
					names = append(names, k)
				}
				return names
			}},
			"custom_variable_values": {Name: "custom_variable_values", Type: "list", Extract: func(r interface{}) interface{} {
				var vals []string
				for _, v := range r.(*objects.Service).CustomVars {
					vals = append(vals, v)
				}
				return vals
			}},
			"custom_variables": {Name: "custom_variables", Type: "string", Extract: func(r interface{}) interface{} {
				svc := r.(*objects.Service)
				if len(svc.CustomVars) == 0 {
					return ""
				}
				var parts []string
				for k, v := range svc.CustomVars {
					parts = append(parts, k+" "+v)
				}
				return strings.Join(parts, "\n")
			}},
			"last_notification": {Name: "last_notification", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastNotification }},
			"next_notification": {Name: "next_notification", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).NextNotification }},
			"current_notification_number": {Name: "current_notification_number", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).CurrentNotificationNumber }},
			"check_type": {Name: "check_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).CheckType }},
			"last_state": {Name: "last_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastState }},
			"should_be_scheduled": {Name: "should_be_scheduled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).ShouldBeScheduled) }},
			"low_flap_threshold": {Name: "low_flap_threshold", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LowFlapThreshold }},
			"high_flap_threshold": {Name: "high_flap_threshold", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).HighFlapThreshold }},
			"modified_attributes": {Name: "modified_attributes", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*objects.Service).ModifiedAttributes) }},
			"is_executing": {Name: "is_executing", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).IsExecuting) }},
			"hourly_value": {Name: "hourly_value", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*objects.Service).HourlyValue) }},
			"staleness": {Name: "staleness", Type: "float", Extract: func(r interface{}) interface{} {
				svc := r.(*objects.Service)
				if svc.CheckInterval <= 0 || svc.LastCheck.IsZero() {
					return 0.0
				}
				age := time.Since(svc.LastCheck).Seconds()
				interval := svc.CheckInterval * 60
				return age / interval
			}},
			// Aliases required by Thruk
			"checks_enabled":        {Name: "checks_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).ActiveChecksEnabled) }},
			"host_checks_enabled":   {Name: "host_checks_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.ActiveChecksEnabled) }},
			"host_check_type":       {Name: "host_check_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.CheckType }},
			"in_check_period":       {Name: "in_check_period", Type: "int", Extract: func(r interface{}) interface{} { return 1 }},
			"in_notification_period": {Name: "in_notification_period", Type: "int", Extract: func(r interface{}) interface{} { return 1 }},
			"comments": {Name: "comments", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				svc := r.(*objects.Service)
				ids := make([]string, 0)
				for _, c := range p.Comments.ForService(svc.Host.Name, svc.Description) {
					ids = append(ids, strconv.FormatUint(c.CommentID, 10))
				}
				return ids
			}},
			"downtimes": {Name: "downtimes", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				svc := r.(*objects.Service)
				ids := make([]string, 0)
				for _, d := range p.Downtimes.All() {
					if d.Type == objects.ServiceDowntimeType && d.HostName == svc.Host.Name && d.ServiceDescription == svc.Description {
						ids = append(ids, strconv.FormatUint(d.DowntimeID, 10))
					}
				}
				return ids
			}},
			"comments_with_info": {Name: "comments_with_info", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				svc := r.(*objects.Service)
				infos := make([]string, 0)
				for _, c := range p.Comments.ForService(svc.Host.Name, svc.Description) {
					infos = append(infos, fmt.Sprintf("%d|%d|%s|%s", c.CommentID, c.EntryType, c.Author, c.Data))
				}
				return infos
			}},
			"downtimes_with_info": {Name: "downtimes_with_info", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				svc := r.(*objects.Service)
				infos := make([]string, 0)
				for _, d := range p.Downtimes.All() {
					if d.Type == objects.ServiceDowntimeType && d.HostName == svc.Host.Name && d.ServiceDescription == svc.Description {
						infos = append(infos, fmt.Sprintf("%d|%s|%s", d.DowntimeID, d.Author, d.Comment))
					}
				}
				return infos
			}},
			"hard_state": {Name: "hard_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).LastHardState }},
			"last_update": {Name: "last_update", Type: "time", Extract: func(r interface{}) interface{} { return time.Now() }},
			"modified_attributes_list": {Name: "modified_attributes_list", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			"check_options": {Name: "check_options", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).CheckOptions }},
			"first_notification_delay": {Name: "first_notification_delay", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).FirstNotificationDelay }},
			"notes_expanded": {Name: "notes_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Notes }},
			"depends_exec": {Name: "depends_exec", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			"depends_notify": {Name: "depends_notify", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			"parents": {Name: "parents", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			// Additional host_ prefix columns Thruk expects
			"host_current_attempt": {Name: "host_current_attempt", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.CurrentAttempt }},
			"host_check_command": {Name: "host_check_command", Type: "string", Extract: func(r interface{}) interface{} {
				return commandStr(r.(*objects.Service).Host.CheckCommand, r.(*objects.Service).Host.CheckCommandArgs)
			}},
			"host_custom_variable_names": {Name: "host_custom_variable_names", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for k := range r.(*objects.Service).Host.CustomVars {
					names = append(names, k)
				}
				return names
			}},
			"host_custom_variable_values": {Name: "host_custom_variable_values", Type: "list", Extract: func(r interface{}) interface{} {
				vals := make([]string, 0)
				for _, v := range r.(*objects.Service).Host.CustomVars {
					vals = append(vals, v)
				}
				return vals
			}},
			"host_comments": {Name: "host_comments", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				svc := r.(*objects.Service)
				ids := make([]string, 0)
				for _, c := range p.Comments.ForHost(svc.Host.Name) {
					ids = append(ids, strconv.FormatUint(c.CommentID, 10))
				}
				return ids
			}},
			"host_contacts": {Name: "host_contacts", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, c := range r.(*objects.Service).Host.Contacts {
					names = append(names, c.Name)
				}
				return names
			}},
			"host_contact_groups": {Name: "host_contact_groups", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, cg := range r.(*objects.Service).Host.ContactGroups {
					names = append(names, cg.Name)
				}
				return names
			}},
			"host_is_executing": {Name: "host_is_executing", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.IsExecuting) }},
			"host_is_flapping": {Name: "host_is_flapping", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Service).Host.IsFlapping) }},
			"host_last_state_change": {Name: "host_last_state_change", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.LastStateChange }},
			"host_latency": {Name: "host_latency", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.Latency }},
			"host_notes": {Name: "host_notes", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.Notes }},
			"host_notes_url_expanded": {Name: "host_notes_url_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.NotesURL }},
			"host_icon_image_alt": {Name: "host_icon_image_alt", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.IconImageAlt }},
			"host_icon_image_expanded": {Name: "host_icon_image_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.IconImage }},
			"host_action_url_expanded": {Name: "host_action_url_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.ActionURL }},
			"host_perf_data": {Name: "host_perf_data", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.PerfData }},
			"host_plugin_output": {Name: "host_plugin_output", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Service).Host.PluginOutput }},
			"host_parents": {Name: "host_parents", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, p := range r.(*objects.Service).Host.Parents {
					names = append(names, p.Name)
				}
				return names
			}},
		},
	}
}
