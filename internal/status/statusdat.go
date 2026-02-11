// Package status implements status.dat and retention.dat file I/O.
package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/downtime"
	"github.com/oceanplexian/gogios/internal/objects"
)

// StatusWriter writes Nagios-compatible status.dat files.
type StatusWriter struct {
	Path      string
	Store     *objects.ObjectStore
	Global    *objects.GlobalState
	Comments  *downtime.CommentManager
	Downtimes *downtime.DowntimeManager
	Version   string
}

// Write atomically writes the status.dat file.
func (sw *StatusWriter) Write() error {
	// Always create the temp file alongside the target so os.Rename
	// never crosses filesystem boundaries.
	dir := filepath.Dir(sw.Path)
	tmp, err := os.CreateTemp(dir, "status.dat.tmp.*")
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

	// info block
	b.WriteString("info {\n")
	fmt.Fprintf(&b, "\tcreated=%d\n", now.Unix())
	fmt.Fprintf(&b, "\tversion=%s\n", sw.Version)
	b.WriteString("\t}\n\n")

	// programstatus block
	sw.writeProgramStatus(&b)

	// hosts
	for _, h := range sw.Store.Hosts {
		sw.writeHostStatus(&b, h)
	}

	// services
	for _, s := range sw.Store.Services {
		sw.writeServiceStatus(&b, s)
	}

	// comments
	for _, c := range sw.Comments.All() {
		sw.writeComment(&b, c)
	}

	// downtimes
	for _, d := range sw.Downtimes.All() {
		sw.writeDowntime(&b, d)
	}

	if _, err := tmp.WriteString(b.String()); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	tmp = nil

	if err := os.Rename(tmpName, sw.Path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

func (sw *StatusWriter) writeProgramStatus(b *strings.Builder) {
	g := sw.Global
	b.WriteString("programstatus {\n")
	fmt.Fprintf(b, "\tnagios_pid=%d\n", g.PID)
	fmt.Fprintf(b, "\tdaemon_mode=%s\n", boolStr(g.DaemonMode))
	fmt.Fprintf(b, "\tprogram_start=%d\n", g.ProgramStart.Unix())
	fmt.Fprintf(b, "\tenable_notifications=%s\n", boolStr(g.EnableNotifications))
	fmt.Fprintf(b, "\tactive_service_checks_enabled=%s\n", boolStr(g.ExecuteServiceChecks))
	fmt.Fprintf(b, "\tpassive_service_checks_enabled=%s\n", boolStr(g.AcceptPassiveServiceChecks))
	fmt.Fprintf(b, "\tactive_host_checks_enabled=%s\n", boolStr(g.ExecuteHostChecks))
	fmt.Fprintf(b, "\tpassive_host_checks_enabled=%s\n", boolStr(g.AcceptPassiveHostChecks))
	fmt.Fprintf(b, "\tenable_event_handlers=%s\n", boolStr(g.EnableEventHandlers))
	fmt.Fprintf(b, "\tobsess_over_services=%s\n", boolStr(g.ObsessOverServices))
	fmt.Fprintf(b, "\tobsess_over_hosts=%s\n", boolStr(g.ObsessOverHosts))
	fmt.Fprintf(b, "\tcheck_service_freshness=%s\n", boolStr(g.CheckServiceFreshness))
	fmt.Fprintf(b, "\tcheck_host_freshness=%s\n", boolStr(g.CheckHostFreshness))
	fmt.Fprintf(b, "\tenable_flap_detection=%s\n", boolStr(g.EnableFlapDetection))
	fmt.Fprintf(b, "\tprocess_performance_data=%s\n", boolStr(g.ProcessPerformanceData))
	fmt.Fprintf(b, "\tglobal_host_event_handler=%s\n", g.GlobalHostEventHandler)
	fmt.Fprintf(b, "\tglobal_service_event_handler=%s\n", g.GlobalServiceEventHandler)
	fmt.Fprintf(b, "\tnext_comment_id=%d\n", g.NextCommentID)
	fmt.Fprintf(b, "\tnext_downtime_id=%d\n", g.NextDowntimeID)
	fmt.Fprintf(b, "\tnext_event_id=%d\n", g.NextEventID)
	fmt.Fprintf(b, "\tnext_problem_id=%d\n", g.NextProblemID)
	fmt.Fprintf(b, "\tnext_notification_id=%d\n", g.NextNotificationID)
	b.WriteString("\t}\n\n")
}

func (sw *StatusWriter) writeHostStatus(b *strings.Builder, h *objects.Host) {
	b.WriteString("hoststatus {\n")
	fmt.Fprintf(b, "\thost_name=%s\n", h.Name)
	fmt.Fprintf(b, "\tmodified_attributes=%d\n", h.ModifiedAttributes)
	writeCheckCommand(b, h.CheckCommand, h.CheckCommandArgs)
	writeTimeperiodName(b, "check_period", h.CheckPeriod)
	writeTimeperiodName(b, "notification_period", h.NotificationPeriod)
	fmt.Fprintf(b, "\tcheck_interval=%f\n", h.CheckInterval)
	fmt.Fprintf(b, "\tretry_interval=%f\n", h.RetryInterval)
	writeCommandName(b, "event_handler", h.EventHandler)
	fmt.Fprintf(b, "\thas_been_checked=%s\n", boolStr(h.HasBeenChecked))
	fmt.Fprintf(b, "\tshould_be_scheduled=%s\n", boolStr(h.ShouldBeScheduled))
	fmt.Fprintf(b, "\tcheck_execution_time=%f\n", h.ExecutionTime)
	fmt.Fprintf(b, "\tcheck_latency=%f\n", h.Latency)
	fmt.Fprintf(b, "\tcheck_type=%d\n", h.CheckType)
	fmt.Fprintf(b, "\tcurrent_state=%d\n", h.CurrentState)
	fmt.Fprintf(b, "\tlast_hard_state=%d\n", h.LastHardState)
	fmt.Fprintf(b, "\tplugin_output=%s\n", h.PluginOutput)
	fmt.Fprintf(b, "\tlong_plugin_output=%s\n", h.LongPluginOutput)
	fmt.Fprintf(b, "\tperformance_data=%s\n", h.PerfData)
	fmt.Fprintf(b, "\tlast_check=%d\n", timeToUnix(h.LastCheck))
	fmt.Fprintf(b, "\tnext_check=%d\n", timeToUnix(h.NextCheck))
	fmt.Fprintf(b, "\tcurrent_attempt=%d\n", h.CurrentAttempt)
	fmt.Fprintf(b, "\tmax_attempts=%d\n", h.MaxCheckAttempts)
	fmt.Fprintf(b, "\tstate_type=%d\n", h.StateType)
	fmt.Fprintf(b, "\tlast_state_change=%d\n", timeToUnix(h.LastStateChange))
	fmt.Fprintf(b, "\tlast_hard_state_change=%d\n", timeToUnix(h.LastHardStateChange))
	fmt.Fprintf(b, "\tlast_time_up=%d\n", timeToUnix(h.LastTimeUp))
	fmt.Fprintf(b, "\tlast_time_down=%d\n", timeToUnix(h.LastTimeDown))
	fmt.Fprintf(b, "\tlast_time_unreachable=%d\n", timeToUnix(h.LastTimeUnreachable))
	fmt.Fprintf(b, "\tlast_notification=%d\n", timeToUnix(h.LastNotification))
	fmt.Fprintf(b, "\tnext_notification=%d\n", timeToUnix(h.NextNotification))
	fmt.Fprintf(b, "\tno_more_notifications=%s\n", boolStr(h.NoMoreNotifications))
	fmt.Fprintf(b, "\tcurrent_notification_number=%d\n", h.CurrentNotificationNumber)
	fmt.Fprintf(b, "\tcurrent_notification_id=%d\n", h.CurrentNotificationID)
	fmt.Fprintf(b, "\tnotifications_enabled=%s\n", boolStr(h.NotificationsEnabled))
	fmt.Fprintf(b, "\tproblem_has_been_acknowledged=%s\n", boolStr(h.ProblemAcknowledged))
	fmt.Fprintf(b, "\tacknowledgement_type=%d\n", h.AckType)
	fmt.Fprintf(b, "\tactive_checks_enabled=%s\n", boolStr(h.ActiveChecksEnabled))
	fmt.Fprintf(b, "\tpassive_checks_enabled=%s\n", boolStr(h.PassiveChecksEnabled))
	fmt.Fprintf(b, "\tevent_handler_enabled=%s\n", boolStr(h.EventHandlerEnabled))
	fmt.Fprintf(b, "\tflap_detection_enabled=%s\n", boolStr(h.FlapDetectionEnabled))
	fmt.Fprintf(b, "\tprocess_performance_data=%s\n", boolStr(h.ProcessPerfData))
	fmt.Fprintf(b, "\tobsess=%s\n", boolStr(h.ObsessOver))
	fmt.Fprintf(b, "\tis_flapping=%s\n", boolStr(h.IsFlapping))
	fmt.Fprintf(b, "\tpercent_state_change=%f\n", h.PercentStateChange)
	fmt.Fprintf(b, "\tscheduled_downtime_depth=%d\n", h.ScheduledDowntimeDepth)
	for k, v := range h.CustomVars {
		fmt.Fprintf(b, "\t_%s=%d;%s\n", k, 0, v)
	}
	b.WriteString("\t}\n\n")
}

func (sw *StatusWriter) writeServiceStatus(b *strings.Builder, s *objects.Service) {
	hostName := ""
	if s.Host != nil {
		hostName = s.Host.Name
	}
	b.WriteString("servicestatus {\n")
	fmt.Fprintf(b, "\thost_name=%s\n", hostName)
	fmt.Fprintf(b, "\tservice_description=%s\n", s.Description)
	fmt.Fprintf(b, "\tmodified_attributes=%d\n", s.ModifiedAttributes)
	writeCheckCommand(b, s.CheckCommand, s.CheckCommandArgs)
	writeTimeperiodName(b, "check_period", s.CheckPeriod)
	writeTimeperiodName(b, "notification_period", s.NotificationPeriod)
	fmt.Fprintf(b, "\tcheck_interval=%f\n", s.CheckInterval)
	fmt.Fprintf(b, "\tretry_interval=%f\n", s.RetryInterval)
	writeCommandName(b, "event_handler", s.EventHandler)
	fmt.Fprintf(b, "\thas_been_checked=%s\n", boolStr(s.HasBeenChecked))
	fmt.Fprintf(b, "\tshould_be_scheduled=%s\n", boolStr(s.ShouldBeScheduled))
	fmt.Fprintf(b, "\tcheck_execution_time=%f\n", s.ExecutionTime)
	fmt.Fprintf(b, "\tcheck_latency=%f\n", s.Latency)
	fmt.Fprintf(b, "\tcheck_type=%d\n", s.CheckType)
	fmt.Fprintf(b, "\tcurrent_state=%d\n", s.CurrentState)
	fmt.Fprintf(b, "\tlast_hard_state=%d\n", s.LastHardState)
	fmt.Fprintf(b, "\tplugin_output=%s\n", s.PluginOutput)
	fmt.Fprintf(b, "\tlong_plugin_output=%s\n", s.LongPluginOutput)
	fmt.Fprintf(b, "\tperformance_data=%s\n", s.PerfData)
	fmt.Fprintf(b, "\tlast_check=%d\n", timeToUnix(s.LastCheck))
	fmt.Fprintf(b, "\tnext_check=%d\n", timeToUnix(s.NextCheck))
	fmt.Fprintf(b, "\tcurrent_attempt=%d\n", s.CurrentAttempt)
	fmt.Fprintf(b, "\tmax_attempts=%d\n", s.MaxCheckAttempts)
	fmt.Fprintf(b, "\tstate_type=%d\n", s.StateType)
	fmt.Fprintf(b, "\tlast_state_change=%d\n", timeToUnix(s.LastStateChange))
	fmt.Fprintf(b, "\tlast_hard_state_change=%d\n", timeToUnix(s.LastHardStateChange))
	fmt.Fprintf(b, "\tlast_time_ok=%d\n", timeToUnix(s.LastTimeOK))
	fmt.Fprintf(b, "\tlast_time_warning=%d\n", timeToUnix(s.LastTimeWarning))
	fmt.Fprintf(b, "\tlast_time_critical=%d\n", timeToUnix(s.LastTimeCritical))
	fmt.Fprintf(b, "\tlast_time_unknown=%d\n", timeToUnix(s.LastTimeUnknown))
	fmt.Fprintf(b, "\tlast_notification=%d\n", timeToUnix(s.LastNotification))
	fmt.Fprintf(b, "\tnext_notification=%d\n", timeToUnix(s.NextNotification))
	fmt.Fprintf(b, "\tno_more_notifications=%s\n", boolStr(s.NoMoreNotifications))
	fmt.Fprintf(b, "\tcurrent_notification_number=%d\n", s.CurrentNotificationNumber)
	fmt.Fprintf(b, "\tcurrent_notification_id=%d\n", s.CurrentNotificationID)
	fmt.Fprintf(b, "\tnotifications_enabled=%s\n", boolStr(s.NotificationsEnabled))
	fmt.Fprintf(b, "\tproblem_has_been_acknowledged=%s\n", boolStr(s.ProblemAcknowledged))
	fmt.Fprintf(b, "\tacknowledgement_type=%d\n", s.AckType)
	fmt.Fprintf(b, "\tactive_checks_enabled=%s\n", boolStr(s.ActiveChecksEnabled))
	fmt.Fprintf(b, "\tpassive_checks_enabled=%s\n", boolStr(s.PassiveChecksEnabled))
	fmt.Fprintf(b, "\tevent_handler_enabled=%s\n", boolStr(s.EventHandlerEnabled))
	fmt.Fprintf(b, "\tflap_detection_enabled=%s\n", boolStr(s.FlapDetectionEnabled))
	fmt.Fprintf(b, "\tprocess_performance_data=%s\n", boolStr(s.ProcessPerfData))
	fmt.Fprintf(b, "\tobsess=%s\n", boolStr(s.ObsessOver))
	fmt.Fprintf(b, "\tis_flapping=%s\n", boolStr(s.IsFlapping))
	fmt.Fprintf(b, "\tpercent_state_change=%f\n", s.PercentStateChange)
	fmt.Fprintf(b, "\tscheduled_downtime_depth=%d\n", s.ScheduledDowntimeDepth)
	for k, v := range s.CustomVars {
		fmt.Fprintf(b, "\t_%s=%d;%s\n", k, 0, v)
	}
	b.WriteString("\t}\n\n")
}

func (sw *StatusWriter) writeComment(b *strings.Builder, c *downtime.Comment) {
	blockName := "hostcomment"
	if c.CommentType == objects.ServiceCommentType {
		blockName = "servicecomment"
	}
	fmt.Fprintf(b, "%s {\n", blockName)
	fmt.Fprintf(b, "\thost_name=%s\n", c.HostName)
	if c.CommentType == objects.ServiceCommentType {
		fmt.Fprintf(b, "\tservice_description=%s\n", c.ServiceDescription)
	}
	fmt.Fprintf(b, "\tentry_type=%d\n", c.EntryType)
	fmt.Fprintf(b, "\tcomment_id=%d\n", c.CommentID)
	fmt.Fprintf(b, "\tsource=%d\n", c.Source)
	fmt.Fprintf(b, "\tpersistent=%s\n", boolStr(c.Persistent))
	fmt.Fprintf(b, "\tentry_time=%d\n", c.EntryTime.Unix())
	fmt.Fprintf(b, "\texpires=%s\n", boolStr(c.Expires))
	fmt.Fprintf(b, "\texpire_time=%d\n", timeToUnix(c.ExpireTime))
	fmt.Fprintf(b, "\tauthor=%s\n", c.Author)
	fmt.Fprintf(b, "\tcomment_data=%s\n", c.Data)
	b.WriteString("\t}\n\n")
}

func (sw *StatusWriter) writeDowntime(b *strings.Builder, d *downtime.Downtime) {
	blockName := "hostdowntime"
	if d.Type == objects.ServiceDowntimeType {
		blockName = "servicedowntime"
	}
	fmt.Fprintf(b, "%s {\n", blockName)
	fmt.Fprintf(b, "\thost_name=%s\n", d.HostName)
	if d.Type == objects.ServiceDowntimeType {
		fmt.Fprintf(b, "\tservice_description=%s\n", d.ServiceDescription)
	}
	fmt.Fprintf(b, "\tdowntime_id=%d\n", d.DowntimeID)
	fmt.Fprintf(b, "\tentry_time=%d\n", d.EntryTime.Unix())
	fmt.Fprintf(b, "\tstart_time=%d\n", d.StartTime.Unix())
	fmt.Fprintf(b, "\tend_time=%d\n", d.EndTime.Unix())
	fmt.Fprintf(b, "\ttriggered_by=%d\n", d.TriggeredBy)
	fmt.Fprintf(b, "\tfixed=%s\n", boolStr(d.Fixed))
	fmt.Fprintf(b, "\tduration=%d\n", int64(d.Duration.Seconds()))
	fmt.Fprintf(b, "\tis_in_effect=%s\n", boolStr(d.IsInEffect))
	fmt.Fprintf(b, "\tauthor=%s\n", d.Author)
	fmt.Fprintf(b, "\tcomment=%s\n", d.Comment)
	b.WriteString("\t}\n\n")
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func timeToUnix(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

func writeCheckCommand(b *strings.Builder, cmd *objects.Command, args string) {
	name := ""
	if cmd != nil {
		name = cmd.Name
	}
	if args != "" {
		name += "!" + args
	}
	fmt.Fprintf(b, "\tcheck_command=%s\n", name)
}

func writeTimeperiodName(b *strings.Builder, field string, tp *objects.Timeperiod) {
	name := ""
	if tp != nil {
		name = tp.Name
	}
	fmt.Fprintf(b, "\t%s=%s\n", field, name)
}

func writeCommandName(b *strings.Builder, field string, cmd *objects.Command) {
	name := ""
	if cmd != nil {
		name = cmd.Name
	}
	fmt.Fprintf(b, "\t%s=%s\n", field, name)
}
