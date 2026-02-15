package status

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/downtime"
	"github.com/oceanplexian/gogios/internal/objects"
)

// RetentionWriter writes Nagios-compatible retention.dat files.
type RetentionWriter struct {
	Path      string
	Store     *objects.ObjectStore
	Global    *objects.GlobalState
	Comments  *downtime.CommentManager
	Downtimes *downtime.DowntimeManager
	Version   string
}

// Write atomically writes the retention.dat file.
func (rw *RetentionWriter) Write() error {
	// Always create the temp file alongside the target so os.Rename
	// never crosses filesystem boundaries.
	dir := filepath.Dir(rw.Path)
	tmp, err := os.CreateTemp(dir, "retention.dat.tmp.*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if tmp != nil {
			tmp.Close()
			os.Remove(tmpName)
		}
	}()

	var b strings.Builder
	now := time.Now()

	// info
	b.WriteString("info {\n")
	fmt.Fprintf(&b, "created=%d\n", now.Unix())
	fmt.Fprintf(&b, "version=%s\n", rw.Version)
	b.WriteString("}\n\n")

	// program
	rw.writeProgram(&b)

	// hosts
	for _, h := range rw.Store.Hosts {
		rw.writeHost(&b, h)
	}

	// services
	for _, s := range rw.Store.Services {
		rw.writeService(&b, s)
	}

	// contacts
	for _, c := range rw.Store.Contacts {
		rw.writeContact(&b, c)
	}

	// comments
	for _, c := range rw.Comments.All() {
		if !c.Persistent {
			continue
		}
		rw.writeComment(&b, c)
	}

	// downtimes
	for _, d := range rw.Downtimes.All() {
		rw.writeDowntime(&b, d)
	}

	if _, err := tmp.WriteString(b.String()); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	tmp = nil
	return os.Rename(tmpName, rw.Path)
}

func (rw *RetentionWriter) writeProgram(b *strings.Builder) {
	g := rw.Global
	b.WriteString("program {\n")
	fmt.Fprintf(b, "modified_host_attributes=%d\n", g.ModifiedHostAttributes)
	fmt.Fprintf(b, "modified_service_attributes=%d\n", g.ModifiedServiceAttributes)
	fmt.Fprintf(b, "enable_notifications=%s\n", boolStr(g.EnableNotifications))
	fmt.Fprintf(b, "active_service_checks_enabled=%s\n", boolStr(g.ExecuteServiceChecks))
	fmt.Fprintf(b, "passive_service_checks_enabled=%s\n", boolStr(g.AcceptPassiveServiceChecks))
	fmt.Fprintf(b, "active_host_checks_enabled=%s\n", boolStr(g.ExecuteHostChecks))
	fmt.Fprintf(b, "passive_host_checks_enabled=%s\n", boolStr(g.AcceptPassiveHostChecks))
	fmt.Fprintf(b, "enable_event_handlers=%s\n", boolStr(g.EnableEventHandlers))
	fmt.Fprintf(b, "obsess_over_services=%s\n", boolStr(g.ObsessOverServices))
	fmt.Fprintf(b, "obsess_over_hosts=%s\n", boolStr(g.ObsessOverHosts))
	fmt.Fprintf(b, "check_service_freshness=%s\n", boolStr(g.CheckServiceFreshness))
	fmt.Fprintf(b, "check_host_freshness=%s\n", boolStr(g.CheckHostFreshness))
	fmt.Fprintf(b, "enable_flap_detection=%s\n", boolStr(g.EnableFlapDetection))
	fmt.Fprintf(b, "process_performance_data=%s\n", boolStr(g.ProcessPerformanceData))
	fmt.Fprintf(b, "global_host_event_handler=%s\n", g.GlobalHostEventHandler)
	fmt.Fprintf(b, "global_service_event_handler=%s\n", g.GlobalServiceEventHandler)
	fmt.Fprintf(b, "next_comment_id=%d\n", g.NextCommentID)
	fmt.Fprintf(b, "next_downtime_id=%d\n", g.NextDowntimeID)
	fmt.Fprintf(b, "next_event_id=%d\n", g.NextEventID)
	fmt.Fprintf(b, "next_problem_id=%d\n", g.NextProblemID)
	fmt.Fprintf(b, "next_notification_id=%d\n", g.NextNotificationID)
	b.WriteString("}\n\n")
}

func (rw *RetentionWriter) writeHost(b *strings.Builder, h *objects.Host) {
	b.WriteString("host {\n")
	fmt.Fprintf(b, "host_name=%s\n", h.Name)
	fmt.Fprintf(b, "modified_attributes=%d\n", h.ModifiedAttributes)
	fmt.Fprintf(b, "check_command=%s\n", cmdName(h.CheckCommand, h.CheckCommandArgs))
	fmt.Fprintf(b, "check_interval=%f\n", h.CheckInterval)
	fmt.Fprintf(b, "retry_interval=%f\n", h.RetryInterval)
	fmt.Fprintf(b, "has_been_checked=%s\n", boolStr(h.HasBeenChecked))
	fmt.Fprintf(b, "check_execution_time=%f\n", h.ExecutionTime)
	fmt.Fprintf(b, "check_latency=%f\n", h.Latency)
	fmt.Fprintf(b, "check_type=%d\n", h.CheckType)
	fmt.Fprintf(b, "current_state=%d\n", h.CurrentState)
	fmt.Fprintf(b, "last_state=%d\n", h.LastState)
	fmt.Fprintf(b, "last_hard_state=%d\n", h.LastHardState)
	fmt.Fprintf(b, "state_type=%d\n", h.StateType)
	fmt.Fprintf(b, "current_attempt=%d\n", h.CurrentAttempt)
	fmt.Fprintf(b, "plugin_output=%s\n", h.PluginOutput)
	fmt.Fprintf(b, "long_plugin_output=%s\n", h.LongPluginOutput)
	fmt.Fprintf(b, "performance_data=%s\n", h.PerfData)
	fmt.Fprintf(b, "last_check=%d\n", timeToUnix(h.LastCheck))
	fmt.Fprintf(b, "next_check=%d\n", timeToUnix(h.NextCheck))
	fmt.Fprintf(b, "last_state_change=%d\n", timeToUnix(h.LastStateChange))
	fmt.Fprintf(b, "last_hard_state_change=%d\n", timeToUnix(h.LastHardStateChange))
	fmt.Fprintf(b, "last_time_up=%d\n", timeToUnix(h.LastTimeUp))
	fmt.Fprintf(b, "last_time_down=%d\n", timeToUnix(h.LastTimeDown))
	fmt.Fprintf(b, "last_time_unreachable=%d\n", timeToUnix(h.LastTimeUnreachable))
	fmt.Fprintf(b, "last_notification=%d\n", timeToUnix(h.LastNotification))
	fmt.Fprintf(b, "next_notification=%d\n", timeToUnix(h.NextNotification))
	fmt.Fprintf(b, "no_more_notifications=%s\n", boolStr(h.NoMoreNotifications))
	fmt.Fprintf(b, "current_notification_number=%d\n", h.CurrentNotificationNumber)
	fmt.Fprintf(b, "current_notification_id=%d\n", h.CurrentNotificationID)
	fmt.Fprintf(b, "notifications_enabled=%s\n", boolStr(h.NotificationsEnabled))
	fmt.Fprintf(b, "problem_has_been_acknowledged=%s\n", boolStr(h.ProblemAcknowledged))
	fmt.Fprintf(b, "acknowledgement_type=%d\n", h.AckType)
	fmt.Fprintf(b, "active_checks_enabled=%s\n", boolStr(h.ActiveChecksEnabled))
	fmt.Fprintf(b, "passive_checks_enabled=%s\n", boolStr(h.PassiveChecksEnabled))
	fmt.Fprintf(b, "event_handler_enabled=%s\n", boolStr(h.EventHandlerEnabled))
	fmt.Fprintf(b, "flap_detection_enabled=%s\n", boolStr(h.FlapDetectionEnabled))
	fmt.Fprintf(b, "process_performance_data=%s\n", boolStr(h.ProcessPerfData))
	fmt.Fprintf(b, "obsess=%s\n", boolStr(h.ObsessOver))
	fmt.Fprintf(b, "is_flapping=%s\n", boolStr(h.IsFlapping))
	fmt.Fprintf(b, "percent_state_change=%f\n", h.PercentStateChange)
	fmt.Fprintf(b, "scheduled_downtime_depth=%d\n", h.ScheduledDowntimeDepth)
	fmt.Fprintf(b, "notified_on_down=%s\n", boolStr(h.NotifiedOn&objects.OptDown != 0))
	fmt.Fprintf(b, "notified_on_unreachable=%s\n", boolStr(h.NotifiedOn&objects.OptUnreachable != 0))
	fmt.Fprintf(b, "check_flapping_recovery_notification=%s\n", boolStr(h.CheckFlapRecoveryNotif))
	// state_history
	histParts := make([]string, len(h.StateHistory))
	for i, v := range h.StateHistory {
		histParts[i] = strconv.Itoa(v)
	}
	fmt.Fprintf(b, "state_history=%s\n", strings.Join(histParts, ","))
	for k, v := range h.CustomVars {
		fmt.Fprintf(b, "_%s=%d;%s\n", k, 0, v)
	}
	b.WriteString("}\n\n")
}

func (rw *RetentionWriter) writeService(b *strings.Builder, s *objects.Service) {
	hostName := ""
	if s.Host != nil {
		hostName = s.Host.Name
	}
	b.WriteString("service {\n")
	fmt.Fprintf(b, "host_name=%s\n", hostName)
	fmt.Fprintf(b, "service_description=%s\n", s.Description)
	fmt.Fprintf(b, "modified_attributes=%d\n", s.ModifiedAttributes)
	fmt.Fprintf(b, "check_command=%s\n", cmdName(s.CheckCommand, s.CheckCommandArgs))
	fmt.Fprintf(b, "check_interval=%f\n", s.CheckInterval)
	fmt.Fprintf(b, "retry_interval=%f\n", s.RetryInterval)
	fmt.Fprintf(b, "has_been_checked=%s\n", boolStr(s.HasBeenChecked))
	fmt.Fprintf(b, "check_execution_time=%f\n", s.ExecutionTime)
	fmt.Fprintf(b, "check_latency=%f\n", s.Latency)
	fmt.Fprintf(b, "check_type=%d\n", s.CheckType)
	fmt.Fprintf(b, "current_state=%d\n", s.CurrentState)
	fmt.Fprintf(b, "last_state=%d\n", s.LastState)
	fmt.Fprintf(b, "last_hard_state=%d\n", s.LastHardState)
	fmt.Fprintf(b, "state_type=%d\n", s.StateType)
	fmt.Fprintf(b, "current_attempt=%d\n", s.CurrentAttempt)
	fmt.Fprintf(b, "plugin_output=%s\n", s.PluginOutput)
	fmt.Fprintf(b, "long_plugin_output=%s\n", s.LongPluginOutput)
	fmt.Fprintf(b, "performance_data=%s\n", s.PerfData)
	fmt.Fprintf(b, "last_check=%d\n", timeToUnix(s.LastCheck))
	fmt.Fprintf(b, "next_check=%d\n", timeToUnix(s.NextCheck))
	fmt.Fprintf(b, "last_state_change=%d\n", timeToUnix(s.LastStateChange))
	fmt.Fprintf(b, "last_hard_state_change=%d\n", timeToUnix(s.LastHardStateChange))
	fmt.Fprintf(b, "last_time_ok=%d\n", timeToUnix(s.LastTimeOK))
	fmt.Fprintf(b, "last_time_warning=%d\n", timeToUnix(s.LastTimeWarning))
	fmt.Fprintf(b, "last_time_critical=%d\n", timeToUnix(s.LastTimeCritical))
	fmt.Fprintf(b, "last_time_unknown=%d\n", timeToUnix(s.LastTimeUnknown))
	fmt.Fprintf(b, "last_notification=%d\n", timeToUnix(s.LastNotification))
	fmt.Fprintf(b, "next_notification=%d\n", timeToUnix(s.NextNotification))
	fmt.Fprintf(b, "no_more_notifications=%s\n", boolStr(s.NoMoreNotifications))
	fmt.Fprintf(b, "current_notification_number=%d\n", s.CurrentNotificationNumber)
	fmt.Fprintf(b, "current_notification_id=%d\n", s.CurrentNotificationID)
	fmt.Fprintf(b, "notifications_enabled=%s\n", boolStr(s.NotificationsEnabled))
	fmt.Fprintf(b, "problem_has_been_acknowledged=%s\n", boolStr(s.ProblemAcknowledged))
	fmt.Fprintf(b, "acknowledgement_type=%d\n", s.AckType)
	fmt.Fprintf(b, "active_checks_enabled=%s\n", boolStr(s.ActiveChecksEnabled))
	fmt.Fprintf(b, "passive_checks_enabled=%s\n", boolStr(s.PassiveChecksEnabled))
	fmt.Fprintf(b, "event_handler_enabled=%s\n", boolStr(s.EventHandlerEnabled))
	fmt.Fprintf(b, "flap_detection_enabled=%s\n", boolStr(s.FlapDetectionEnabled))
	fmt.Fprintf(b, "process_performance_data=%s\n", boolStr(s.ProcessPerfData))
	fmt.Fprintf(b, "obsess=%s\n", boolStr(s.ObsessOver))
	fmt.Fprintf(b, "is_flapping=%s\n", boolStr(s.IsFlapping))
	fmt.Fprintf(b, "percent_state_change=%f\n", s.PercentStateChange)
	fmt.Fprintf(b, "scheduled_downtime_depth=%d\n", s.ScheduledDowntimeDepth)
	fmt.Fprintf(b, "notified_on_unknown=%s\n", boolStr(s.NotifiedOn&objects.OptUnknown != 0))
	fmt.Fprintf(b, "notified_on_warning=%s\n", boolStr(s.NotifiedOn&objects.OptWarning != 0))
	fmt.Fprintf(b, "notified_on_critical=%s\n", boolStr(s.NotifiedOn&objects.OptCritical != 0))
	fmt.Fprintf(b, "check_flapping_recovery_notification=%s\n", boolStr(s.CheckFlapRecoveryNotif))
	histParts := make([]string, len(s.StateHistory))
	for i, v := range s.StateHistory {
		histParts[i] = strconv.Itoa(v)
	}
	fmt.Fprintf(b, "state_history=%s\n", strings.Join(histParts, ","))
	for k, v := range s.CustomVars {
		fmt.Fprintf(b, "_%s=%d;%s\n", k, 0, v)
	}
	b.WriteString("}\n\n")
}

func (rw *RetentionWriter) writeContact(b *strings.Builder, c *objects.Contact) {
	b.WriteString("contact {\n")
	fmt.Fprintf(b, "contact_name=%s\n", c.Name)
	fmt.Fprintf(b, "modified_attributes=%d\n", c.ModifiedAttributes)
	fmt.Fprintf(b, "modified_host_attributes=%d\n", c.ModifiedHostAttributes)
	fmt.Fprintf(b, "modified_service_attributes=%d\n", c.ModifiedServiceAttributes)
	tpName := ""
	if c.HostNotificationPeriod != nil {
		tpName = c.HostNotificationPeriod.Name
	}
	fmt.Fprintf(b, "host_notification_period=%s\n", tpName)
	tpName = ""
	if c.ServiceNotificationPeriod != nil {
		tpName = c.ServiceNotificationPeriod.Name
	}
	fmt.Fprintf(b, "service_notification_period=%s\n", tpName)
	fmt.Fprintf(b, "host_notifications_enabled=%s\n", boolStr(c.HostNotificationsEnabled))
	fmt.Fprintf(b, "service_notifications_enabled=%s\n", boolStr(c.ServiceNotificationsEnabled))
	fmt.Fprintf(b, "last_host_notification=%d\n", timeToUnix(c.LastHostNotification))
	fmt.Fprintf(b, "last_service_notification=%d\n", timeToUnix(c.LastServiceNotification))
	for k, v := range c.CustomVars {
		fmt.Fprintf(b, "_%s=%d;%s\n", k, 0, v)
	}
	b.WriteString("}\n\n")
}

func (rw *RetentionWriter) writeComment(b *strings.Builder, c *downtime.Comment) {
	blockName := "hostcomment"
	if c.CommentType == objects.ServiceCommentType {
		blockName = "servicecomment"
	}
	fmt.Fprintf(b, "%s {\n", blockName)
	fmt.Fprintf(b, "host_name=%s\n", c.HostName)
	if c.CommentType == objects.ServiceCommentType {
		fmt.Fprintf(b, "service_description=%s\n", c.ServiceDescription)
	}
	fmt.Fprintf(b, "entry_type=%d\n", c.EntryType)
	fmt.Fprintf(b, "comment_id=%d\n", c.CommentID)
	fmt.Fprintf(b, "source=%d\n", c.Source)
	fmt.Fprintf(b, "persistent=%s\n", boolStr(c.Persistent))
	fmt.Fprintf(b, "entry_time=%d\n", c.EntryTime.Unix())
	fmt.Fprintf(b, "expires=%s\n", boolStr(c.Expires))
	fmt.Fprintf(b, "expire_time=%d\n", timeToUnix(c.ExpireTime))
	fmt.Fprintf(b, "author=%s\n", c.Author)
	fmt.Fprintf(b, "comment_data=%s\n", c.Data)
	b.WriteString("}\n\n")
}

func (rw *RetentionWriter) writeDowntime(b *strings.Builder, d *downtime.Downtime) {
	blockName := "hostdowntime"
	if d.Type == objects.ServiceDowntimeType {
		blockName = "servicedowntime"
	}
	fmt.Fprintf(b, "%s {\n", blockName)
	fmt.Fprintf(b, "host_name=%s\n", d.HostName)
	if d.Type == objects.ServiceDowntimeType {
		fmt.Fprintf(b, "service_description=%s\n", d.ServiceDescription)
	}
	fmt.Fprintf(b, "downtime_id=%d\n", d.DowntimeID)
	fmt.Fprintf(b, "entry_time=%d\n", d.EntryTime.Unix())
	fmt.Fprintf(b, "start_time=%d\n", d.StartTime.Unix())
	fmt.Fprintf(b, "end_time=%d\n", d.EndTime.Unix())
	fmt.Fprintf(b, "triggered_by=%d\n", d.TriggeredBy)
	fmt.Fprintf(b, "fixed=%s\n", boolStr(d.Fixed))
	fmt.Fprintf(b, "duration=%d\n", int64(d.Duration.Seconds()))
	fmt.Fprintf(b, "is_in_effect=%s\n", boolStr(d.IsInEffect))
	fmt.Fprintf(b, "author=%s\n", d.Author)
	fmt.Fprintf(b, "comment=%s\n", d.Comment)
	b.WriteString("}\n\n")
}

func cmdName(cmd *objects.Command, args string) string {
	if cmd == nil {
		return ""
	}
	if args != "" {
		return cmd.Name + "!" + args
	}
	return cmd.Name
}

// RetentionReader reads a retention.dat file and applies state to objects.
type RetentionReader struct {
	Store     *objects.ObjectStore
	Global    *objects.GlobalState
	Comments  *downtime.CommentManager
	Downtimes *downtime.DowntimeManager
}

// Read reads and applies the retention.dat file.
func (rr *RetentionReader) Read(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No retention data is fine
		}
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var blockType string
	var fields map[string]string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasSuffix(line, "{") {
			blockType = strings.TrimSpace(strings.TrimSuffix(line, "{"))
			fields = make(map[string]string)
			continue
		}

		if line == "}" {
			if fields != nil {
				rr.applyBlock(blockType, fields)
			}
			blockType = ""
			fields = nil
			continue
		}

		if fields != nil {
			idx := strings.IndexByte(line, '=')
			if idx > 0 {
				fields[line[:idx]] = line[idx+1:]
			}
		}
	}
	return scanner.Err()
}

func (rr *RetentionReader) applyBlock(blockType string, fields map[string]string) {
	switch blockType {
	case "program":
		rr.applyProgram(fields)
	case "host":
		rr.applyHost(fields)
	case "service":
		rr.applyService(fields)
	case "contact":
		rr.applyContact(fields)
	case "hostcomment", "servicecomment":
		rr.applyComment(fields, blockType)
	case "hostdowntime", "servicedowntime":
		rr.applyDowntimeBlock(fields, blockType)
	}
}

func (rr *RetentionReader) applyProgram(f map[string]string) {
	g := rr.Global
	if v, ok := f["enable_notifications"]; ok {
		g.EnableNotifications = v == "1"
	}
	if v, ok := f["active_service_checks_enabled"]; ok {
		g.ExecuteServiceChecks = v == "1"
	}
	if v, ok := f["passive_service_checks_enabled"]; ok {
		g.AcceptPassiveServiceChecks = v == "1"
	}
	if v, ok := f["active_host_checks_enabled"]; ok {
		g.ExecuteHostChecks = v == "1"
	}
	if v, ok := f["passive_host_checks_enabled"]; ok {
		g.AcceptPassiveHostChecks = v == "1"
	}
	if v, ok := f["enable_event_handlers"]; ok {
		g.EnableEventHandlers = v == "1"
	}
	if v, ok := f["enable_flap_detection"]; ok {
		g.EnableFlapDetection = v == "1"
	}
	if v, ok := f["process_performance_data"]; ok {
		g.ProcessPerformanceData = v == "1"
	}
	if v, ok := f["next_comment_id"]; ok {
		g.NextCommentID = parseUint64(v)
	}
	if v, ok := f["next_downtime_id"]; ok {
		g.NextDowntimeID = parseUint64(v)
	}
	if v, ok := f["next_event_id"]; ok {
		g.NextEventID = parseUint64(v)
	}
	if v, ok := f["next_problem_id"]; ok {
		g.NextProblemID = parseUint64(v)
	}
	if v, ok := f["next_notification_id"]; ok {
		g.NextNotificationID = parseUint64(v)
	}
}

func (rr *RetentionReader) applyHost(f map[string]string) {
	name := f["host_name"]
	h := rr.Store.GetHost(name)
	if h == nil {
		return
	}
	// Only override config-level toggles (notifications, active/passive checks)
	// if an admin explicitly changed them (modified_attributes != 0).
	modAttrs := parseUint64(f["modified_attributes"])
	if v, ok := f["current_state"]; ok {
		h.CurrentState = parseInt(v)
	}
	if v, ok := f["last_state"]; ok {
		h.LastState = parseInt(v)
	}
	if v, ok := f["last_hard_state"]; ok {
		h.LastHardState = parseInt(v)
	}
	if v, ok := f["state_type"]; ok {
		h.StateType = parseInt(v)
	}
	if v, ok := f["current_attempt"]; ok {
		h.CurrentAttempt = parseInt(v)
	}
	if v, ok := f["has_been_checked"]; ok {
		h.HasBeenChecked = v == "1"
	}
	if v, ok := f["plugin_output"]; ok {
		h.PluginOutput = v
	}
	if v, ok := f["long_plugin_output"]; ok {
		h.LongPluginOutput = v
	}
	if v, ok := f["performance_data"]; ok {
		h.PerfData = v
	}
	if v, ok := f["last_check"]; ok {
		h.LastCheck = unixToTime(v)
	}
	if v, ok := f["next_check"]; ok {
		h.NextCheck = unixToTime(v)
	}
	if v, ok := f["last_state_change"]; ok {
		h.LastStateChange = unixToTime(v)
	}
	if v, ok := f["last_hard_state_change"]; ok {
		h.LastHardStateChange = unixToTime(v)
	}
	if v, ok := f["last_notification"]; ok {
		h.LastNotification = unixToTime(v)
	}
	if v, ok := f["next_notification"]; ok {
		h.NextNotification = unixToTime(v)
	}
	if v, ok := f["current_notification_number"]; ok {
		h.CurrentNotificationNumber = parseInt(v)
	}
	if v, ok := f["current_notification_id"]; ok {
		h.CurrentNotificationID = parseUint64(v)
	}
	if modAttrs != 0 {
		if v, ok := f["notifications_enabled"]; ok {
			h.NotificationsEnabled = v == "1"
		}
		if v, ok := f["active_checks_enabled"]; ok {
			h.ActiveChecksEnabled = v == "1"
		}
		if v, ok := f["passive_checks_enabled"]; ok {
			h.PassiveChecksEnabled = v == "1"
		}
	}
	if v, ok := f["problem_has_been_acknowledged"]; ok {
		h.ProblemAcknowledged = v == "1"
	}
	if v, ok := f["acknowledgement_type"]; ok {
		h.AckType = parseInt(v)
	}
	if v, ok := f["is_flapping"]; ok {
		h.IsFlapping = v == "1"
	}
	if v, ok := f["percent_state_change"]; ok {
		h.PercentStateChange = parseFloat(v)
	}
	if v, ok := f["scheduled_downtime_depth"]; ok {
		h.ScheduledDowntimeDepth = parseInt(v)
	}
	// notified_on reconstruction
	var notified uint32
	if f["notified_on_down"] == "1" {
		notified |= objects.OptDown
	}
	if f["notified_on_unreachable"] == "1" {
		notified |= objects.OptUnreachable
	}
	h.NotifiedOn = notified
	if v, ok := f["check_flapping_recovery_notification"]; ok {
		h.CheckFlapRecoveryNotif = v == "1"
	}
	if v, ok := f["state_history"]; ok {
		rr.parseStateHistory(v, h.StateHistory[:])
	}
}

func (rr *RetentionReader) applyService(f map[string]string) {
	hostName := f["host_name"]
	desc := f["service_description"]
	s := rr.Store.GetService(hostName, desc)
	if s == nil {
		return
	}
	modAttrs := parseUint64(f["modified_attributes"])
	if v, ok := f["current_state"]; ok {
		s.CurrentState = parseInt(v)
	}
	if v, ok := f["last_state"]; ok {
		s.LastState = parseInt(v)
	}
	if v, ok := f["last_hard_state"]; ok {
		s.LastHardState = parseInt(v)
	}
	if v, ok := f["state_type"]; ok {
		s.StateType = parseInt(v)
	}
	if v, ok := f["current_attempt"]; ok {
		s.CurrentAttempt = parseInt(v)
	}
	if v, ok := f["has_been_checked"]; ok {
		s.HasBeenChecked = v == "1"
	}
	if v, ok := f["plugin_output"]; ok {
		s.PluginOutput = v
	}
	if v, ok := f["long_plugin_output"]; ok {
		s.LongPluginOutput = v
	}
	if v, ok := f["performance_data"]; ok {
		s.PerfData = v
	}
	if v, ok := f["last_check"]; ok {
		s.LastCheck = unixToTime(v)
	}
	if v, ok := f["next_check"]; ok {
		s.NextCheck = unixToTime(v)
	}
	if v, ok := f["last_state_change"]; ok {
		s.LastStateChange = unixToTime(v)
	}
	if v, ok := f["last_hard_state_change"]; ok {
		s.LastHardStateChange = unixToTime(v)
	}
	if v, ok := f["last_notification"]; ok {
		s.LastNotification = unixToTime(v)
	}
	if v, ok := f["next_notification"]; ok {
		s.NextNotification = unixToTime(v)
	}
	if v, ok := f["current_notification_number"]; ok {
		s.CurrentNotificationNumber = parseInt(v)
	}
	if v, ok := f["current_notification_id"]; ok {
		s.CurrentNotificationID = parseUint64(v)
	}
	if modAttrs != 0 {
		if v, ok := f["notifications_enabled"]; ok {
			s.NotificationsEnabled = v == "1"
		}
		if v, ok := f["active_checks_enabled"]; ok {
			s.ActiveChecksEnabled = v == "1"
		}
		if v, ok := f["passive_checks_enabled"]; ok {
			s.PassiveChecksEnabled = v == "1"
		}
	}
	if v, ok := f["problem_has_been_acknowledged"]; ok {
		s.ProblemAcknowledged = v == "1"
	}
	if v, ok := f["acknowledgement_type"]; ok {
		s.AckType = parseInt(v)
	}
	if v, ok := f["is_flapping"]; ok {
		s.IsFlapping = v == "1"
	}
	if v, ok := f["percent_state_change"]; ok {
		s.PercentStateChange = parseFloat(v)
	}
	if v, ok := f["scheduled_downtime_depth"]; ok {
		s.ScheduledDowntimeDepth = parseInt(v)
	}
	var notified uint32
	if f["notified_on_unknown"] == "1" {
		notified |= objects.OptUnknown
	}
	if f["notified_on_warning"] == "1" {
		notified |= objects.OptWarning
	}
	if f["notified_on_critical"] == "1" {
		notified |= objects.OptCritical
	}
	s.NotifiedOn = notified
	if v, ok := f["check_flapping_recovery_notification"]; ok {
		s.CheckFlapRecoveryNotif = v == "1"
	}
	if v, ok := f["state_history"]; ok {
		rr.parseStateHistory(v, s.StateHistory[:])
	}
}

func (rr *RetentionReader) applyContact(f map[string]string) {
	name := f["contact_name"]
	c := rr.Store.GetContact(name)
	if c == nil {
		return
	}
	if v, ok := f["host_notifications_enabled"]; ok {
		c.HostNotificationsEnabled = v == "1"
	}
	if v, ok := f["service_notifications_enabled"]; ok {
		c.ServiceNotificationsEnabled = v == "1"
	}
	if v, ok := f["last_host_notification"]; ok {
		c.LastHostNotification = unixToTime(v)
	}
	if v, ok := f["last_service_notification"]; ok {
		c.LastServiceNotification = unixToTime(v)
	}
	if v, ok := f["modified_attributes"]; ok {
		c.ModifiedAttributes = parseUint64(v)
	}
}

func (rr *RetentionReader) applyComment(f map[string]string, blockType string) {
	c := &downtime.Comment{
		HostName:           f["host_name"],
		ServiceDescription: f["service_description"],
		CommentType:        objects.HostCommentType,
		EntryType:          parseInt(f["entry_type"]),
		CommentID:          parseUint64(f["comment_id"]),
		Source:             parseInt(f["source"]),
		Persistent:         f["persistent"] == "1",
		EntryTime:          unixToTime(f["entry_time"]),
		Expires:            f["expires"] == "1",
		ExpireTime:         unixToTime(f["expire_time"]),
		Author:             f["author"],
		Data:               f["comment_data"],
	}
	if blockType == "servicecomment" {
		c.CommentType = objects.ServiceCommentType
	}
	rr.Comments.AddWithID(c)
}

func (rr *RetentionReader) applyDowntimeBlock(f map[string]string, blockType string) {
	dtype := objects.HostDowntimeType
	if blockType == "servicedowntime" {
		dtype = objects.ServiceDowntimeType
	}
	d := &downtime.Downtime{
		Type:               dtype,
		HostName:           f["host_name"],
		ServiceDescription: f["service_description"],
		DowntimeID:         parseUint64(f["downtime_id"]),
		EntryTime:          unixToTime(f["entry_time"]),
		StartTime:          unixToTime(f["start_time"]),
		EndTime:            unixToTime(f["end_time"]),
		TriggeredBy:        parseUint64(f["triggered_by"]),
		Fixed:              f["fixed"] == "1",
		Duration:           time.Duration(parseInt(f["duration"])) * time.Second,
		IsInEffect:         f["is_in_effect"] == "1",
		Author:             f["author"],
		Comment:            f["comment"],
	}
	rr.Downtimes.ScheduleWithID(d)
}

func (rr *RetentionReader) parseStateHistory(s string, hist []int) {
	parts := strings.Split(s, ",")
	for i := 0; i < len(parts) && i < len(hist); i++ {
		hist[i] = parseInt(strings.TrimSpace(parts[i]))
	}
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(s)
	return v
}

func parseUint64(s string) uint64 {
	v, _ := strconv.ParseUint(s, 10, 64)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func unixToTime(s string) time.Time {
	v := parseInt(s)
	if v == 0 {
		return time.Time{}
	}
	return time.Unix(int64(v), 0)
}
