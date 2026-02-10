package livestatus

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func hostsTable() *Table {
	return &Table{
		Name: "hosts",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.Hosts))
			for i, h := range p.Store.Hosts {
				rows[i] = h
			}
			return rows
		},
		Columns: map[string]*Column{
			"name":            {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).Name }},
			"display_name":    {Name: "display_name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).DisplayName }},
			"alias":           {Name: "alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).Alias }},
			"address":         {Name: "address", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).Address }},
			"state":           {Name: "state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).CurrentState }},
			"state_type":      {Name: "state_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).StateType }},
			"plugin_output":   {Name: "plugin_output", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).PluginOutput }},
			"long_plugin_output": {Name: "long_plugin_output", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LongPluginOutput }},
			"perf_data":       {Name: "perf_data", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).PerfData }},
			"has_been_checked": {Name: "has_been_checked", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).HasBeenChecked) }},
			"current_attempt": {Name: "current_attempt", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).CurrentAttempt }},
			"max_check_attempts": {Name: "max_check_attempts", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).MaxCheckAttempts }},
			"last_check":      {Name: "last_check", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastCheck }},
			"next_check":      {Name: "next_check", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).NextCheck }},
			"last_state_change": {Name: "last_state_change", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastStateChange }},
			"last_hard_state_change": {Name: "last_hard_state_change", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastHardStateChange }},
			"last_hard_state": {Name: "last_hard_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastHardState }},
			"last_time_up":    {Name: "last_time_up", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastTimeUp }},
			"last_time_down":  {Name: "last_time_down", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastTimeDown }},
			"last_time_unreachable": {Name: "last_time_unreachable", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastTimeUnreachable }},
			"check_command": {Name: "check_command", Type: "string", Extract: func(r interface{}) interface{} {
				h := r.(*objects.Host)
				if h.CheckCommand == nil {
					return ""
				}
				cmd := h.CheckCommand.Name
				if h.CheckCommandArgs != "" {
					cmd += "!" + h.CheckCommandArgs
				}
				return cmd
			}},
			"check_interval":    {Name: "check_interval", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).CheckInterval }},
			"retry_interval":    {Name: "retry_interval", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).RetryInterval }},
			"check_period": {Name: "check_period", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Host).CheckPeriod != nil {
					return r.(*objects.Host).CheckPeriod.Name
				}
				return ""
			}},
			"notification_period": {Name: "notification_period", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Host).NotificationPeriod != nil {
					return r.(*objects.Host).NotificationPeriod.Name
				}
				return ""
			}},
			"notifications_enabled": {Name: "notifications_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).NotificationsEnabled) }},
			"notification_interval": {Name: "notification_interval", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).NotificationInterval }},
			"active_checks_enabled": {Name: "active_checks_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).ActiveChecksEnabled) }},
			"accept_passive_checks": {Name: "accept_passive_checks", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).PassiveChecksEnabled) }},
			"obsess_over_host":      {Name: "obsess_over_host", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).ObsessOver) }},
			"event_handler_enabled": {Name: "event_handler_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).EventHandlerEnabled) }},
			"event_handler": {Name: "event_handler", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Host).EventHandler != nil {
					return r.(*objects.Host).EventHandler.Name
				}
				return ""
			}},
			"check_freshness":       {Name: "check_freshness", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).CheckFreshness) }},
			"freshness_threshold":   {Name: "freshness_threshold", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).FreshnessThreshold }},
			"flap_detection_enabled": {Name: "flap_detection_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).FlapDetectionEnabled) }},
			"is_flapping":           {Name: "is_flapping", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).IsFlapping) }},
			"percent_state_change":  {Name: "percent_state_change", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).PercentStateChange }},
			"latency":               {Name: "latency", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).Latency }},
			"execution_time":        {Name: "execution_time", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).ExecutionTime }},
			"process_performance_data": {Name: "process_performance_data", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).ProcessPerfData) }},
			"scheduled_downtime_depth": {Name: "scheduled_downtime_depth", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).ScheduledDowntimeDepth }},
			"acknowledged":          {Name: "acknowledged", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).ProblemAcknowledged) }},
			"acknowledgement_type":  {Name: "acknowledgement_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).AckType }},
			"notes":                 {Name: "notes", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).Notes }},
			"notes_url":             {Name: "notes_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).NotesURL }},
			"notes_url_expanded":    {Name: "notes_url_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).NotesURL }},
			"action_url":            {Name: "action_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).ActionURL }},
			"action_url_expanded":   {Name: "action_url_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).ActionURL }},
			"icon_image":            {Name: "icon_image", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).IconImage }},
			"icon_image_alt":        {Name: "icon_image_alt", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).IconImageAlt }},
			"icon_image_expanded":   {Name: "icon_image_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).IconImage }},
			"num_services":          {Name: "num_services", Type: "int", Extract: func(r interface{}) interface{} { return len(r.(*objects.Host).Services) }},
			"num_services_ok": {Name: "num_services_ok", Type: "int", Extract: func(r interface{}) interface{} {
				return countServicesByState(r.(*objects.Host).Services, objects.ServiceOK)
			}},
			"num_services_warn": {Name: "num_services_warn", Type: "int", Extract: func(r interface{}) interface{} {
				return countServicesByState(r.(*objects.Host).Services, objects.ServiceWarning)
			}},
			"num_services_crit": {Name: "num_services_crit", Type: "int", Extract: func(r interface{}) interface{} {
				return countServicesByState(r.(*objects.Host).Services, objects.ServiceCritical)
			}},
			"num_services_unknown": {Name: "num_services_unknown", Type: "int", Extract: func(r interface{}) interface{} {
				return countServicesByState(r.(*objects.Host).Services, objects.ServiceUnknown)
			}},
			"num_services_pending": {Name: "num_services_pending", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, svc := range r.(*objects.Host).Services {
					if !svc.HasBeenChecked {
						count++
					}
				}
				return count
			}},
			"worst_service_state": {Name: "worst_service_state", Type: "int", Extract: func(r interface{}) interface{} {
				worst := 0
				for _, svc := range r.(*objects.Host).Services {
					if svc.CurrentState > worst {
						worst = svc.CurrentState
					}
				}
				return worst
			}},
			"parents": {Name: "parents", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, p := range r.(*objects.Host).Parents {
					names = append(names, p.Name)
				}
				return names
			}},
			"childs": {Name: "childs", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, c := range r.(*objects.Host).Children {
					names = append(names, c.Name)
				}
				return names
			}},
			"contact_groups": {Name: "contact_groups", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, cg := range r.(*objects.Host).ContactGroups {
					names = append(names, cg.Name)
				}
				return names
			}},
			"contacts": {Name: "contacts", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, c := range r.(*objects.Host).Contacts {
					names = append(names, c.Name)
				}
				return names
			}},
			"groups": {Name: "groups", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for _, hg := range r.(*objects.Host).HostGroups {
					names = append(names, hg.Name)
				}
				return names
			}},
			"custom_variable_names": {Name: "custom_variable_names", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for k := range r.(*objects.Host).CustomVars {
					names = append(names, k)
				}
				return names
			}},
			"custom_variable_values": {Name: "custom_variable_values", Type: "list", Extract: func(r interface{}) interface{} {
				var vals []string
				for _, v := range r.(*objects.Host).CustomVars {
					vals = append(vals, v)
				}
				return vals
			}},
			"custom_variables": {Name: "custom_variables", Type: "string", Extract: func(r interface{}) interface{} {
				h := r.(*objects.Host)
				if len(h.CustomVars) == 0 {
					return ""
				}
				var parts []string
				for k, v := range h.CustomVars {
					parts = append(parts, k+" "+v)
				}
				return strings.Join(parts, "\n")
			}},
			"last_notification": {Name: "last_notification", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastNotification }},
			"next_notification": {Name: "next_notification", Type: "time", Extract: func(r interface{}) interface{} { return r.(*objects.Host).NextNotification }},
			"current_notification_number": {Name: "current_notification_number", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).CurrentNotificationNumber }},
			"check_type": {Name: "check_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).CheckType }},
			"last_state": {Name: "last_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastState }},
			"should_be_scheduled": {Name: "should_be_scheduled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).ShouldBeScheduled) }},
			"low_flap_threshold": {Name: "low_flap_threshold", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LowFlapThreshold }},
			"high_flap_threshold": {Name: "high_flap_threshold", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).HighFlapThreshold }},
			"modified_attributes": {Name: "modified_attributes", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*objects.Host).ModifiedAttributes) }},
			"is_executing": {Name: "is_executing", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).IsExecuting) }},
			"hourly_value": {Name: "hourly_value", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*objects.Host).HourlyValue) }},
			"staleness": {Name: "staleness", Type: "float", Extract: func(r interface{}) interface{} {
				h := r.(*objects.Host)
				if h.CheckInterval <= 0 || h.LastCheck.IsZero() {
					return 0.0
				}
				age := time.Since(h.LastCheck).Seconds()
				interval := h.CheckInterval * 60 // intervals are in minutes
				return age / interval
			}},
			// Aliases required by Thruk
			"checks_enabled":        {Name: "checks_enabled", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*objects.Host).ActiveChecksEnabled) }},
			"in_check_period":       {Name: "in_check_period", Type: "int", Extract: func(r interface{}) interface{} { return 1 }},
			"in_notification_period": {Name: "in_notification_period", Type: "int", Extract: func(r interface{}) interface{} { return 1 }},
			"comments": {Name: "comments", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				h := r.(*objects.Host)
				ids := make([]string, 0)
				for _, c := range p.Comments.ForHost(h.Name) {
					ids = append(ids, strconv.FormatUint(c.CommentID, 10))
				}
				return ids
			}},
			"downtimes": {Name: "downtimes", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				h := r.(*objects.Host)
				ids := make([]string, 0)
				for _, d := range p.Downtimes.All() {
					if d.Type == objects.HostDowntimeType && d.HostName == h.Name {
						ids = append(ids, strconv.FormatUint(d.DowntimeID, 10))
					}
				}
				return ids
			}},
			"comments_with_info": {Name: "comments_with_info", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				h := r.(*objects.Host)
				infos := make([]string, 0)
				for _, c := range p.Comments.ForHost(h.Name) {
					infos = append(infos, fmt.Sprintf("%d|%d|%s|%s", c.CommentID, c.EntryType, c.Author, c.Data))
				}
				return infos
			}},
			"downtimes_with_info": {Name: "downtimes_with_info", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}, ProviderExtract: func(r interface{}, p *api.StateProvider) interface{} {
				h := r.(*objects.Host)
				infos := make([]string, 0)
				for _, d := range p.Downtimes.All() {
					if d.Type == objects.HostDowntimeType && d.HostName == h.Name {
						infos = append(infos, fmt.Sprintf("%d|%s|%s", d.DowntimeID, d.Author, d.Comment))
					}
				}
				return infos
			}},
			"services": {Name: "services", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, svc := range r.(*objects.Host).Services {
					names = append(names, svc.Description)
				}
				return names
			}},
			"services_with_state": {Name: "services_with_state", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			"services_with_info": {Name: "services_with_info", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			"hard_state": {Name: "hard_state", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).LastHardState }},
			"last_update": {Name: "last_update", Type: "time", Extract: func(r interface{}) interface{} { return time.Now() }},
			"modified_attributes_list": {Name: "modified_attributes_list", Type: "list", Extract: func(r interface{}) interface{} {
				return make([]string, 0)
			}},
			"check_options": {Name: "check_options", Type: "int", Extract: func(r interface{}) interface{} { return r.(*objects.Host).CheckOptions }},
			"first_notification_delay": {Name: "first_notification_delay", Type: "float", Extract: func(r interface{}) interface{} { return r.(*objects.Host).FirstNotificationDelay }},
			"notes_expanded": {Name: "notes_expanded", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Host).Notes }},
		},
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func countServicesByState(services []*objects.Service, state int) int {
	count := 0
	for _, svc := range services {
		if svc.HasBeenChecked && svc.CurrentState == state {
			count++
		}
	}
	return count
}

func timeToUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func commandStr(cmd *objects.Command, args string) string {
	if cmd == nil {
		return ""
	}
	if args != "" {
		return fmt.Sprintf("%s!%s", cmd.Name, args)
	}
	return cmd.Name
}
