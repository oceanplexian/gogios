package notify

import (
	"sync/atomic"
	"time"

	"github.com/oceanplexian/gogios/internal/dependency"
	"github.com/oceanplexian/gogios/internal/objects"
)

// Logger interface for notification logging.
type Logger interface {
	Log(format string, args ...interface{})
}

// NotificationEngine handles all notification logic.
type NotificationEngine struct {
	GlobalState    *objects.GlobalState
	Store          *objects.ObjectStore
	Logger         Logger
	CmdExecutor    *CommandExecutor
	nextNotifID    atomic.Uint64
}

// NewNotificationEngine creates a new notification engine.
func NewNotificationEngine(gs *objects.GlobalState, store *objects.ObjectStore, logger Logger) *NotificationEngine {
	return &NotificationEngine{
		GlobalState: gs,
		Store:       store,
		Logger:      logger,
		CmdExecutor: NewCommandExecutor(30 * time.Second),
	}
}

// SetNextNotificationID sets the next notification ID (from retention).
func (ne *NotificationEngine) SetNextNotificationID(id uint64) {
	ne.nextNotifID.Store(id)
}

// NextNotificationID returns the current next notification ID.
func (ne *NotificationEngine) NextNotificationID() uint64 {
	return ne.nextNotifID.Load()
}

func (ne *NotificationEngine) log(format string, args ...interface{}) {
	if ne.Logger != nil {
		ne.Logger.Log(format, args...)
	}
}

// ServiceNotification is the main entry point for sending service notifications.
func (ne *NotificationEngine) ServiceNotification(svc *objects.Service, ntype int, author, data string, options int) int {
	// Check viability
	if ne.checkServiceNotificationViability(svc, ntype, options) != 0 {
		return 1
	}

	// Increment notification number for NORMAL or INCREMENT
	if ntype == objects.NotificationNormal || options&objects.NotificationOptionIncrement != 0 {
		svc.CurrentNotificationNumber++
	}

	// Assign notification ID
	svc.CurrentNotificationID = ne.nextNotifID.Add(1) - 1

	// Build contact list
	contacts := ne.createServiceNotificationList(svc, options)

	contactsNotified := 0
	now := time.Now()
	typeName := objects.NotificationTypeName(ntype, svc.CurrentState, false)

	for _, contact := range contacts {
		if ne.checkContactServiceViability(contact, svc, ntype, options) != 0 {
			continue
		}
		ne.notifyContactOfService(contact, svc, ntype, typeName, author, data)
		contactsNotified++
	}

	if ntype == objects.NotificationNormal && contactsNotified > 0 {
		svc.NextNotification = GetNextServiceNotificationTime(svc, now, ne.intervalLength())
		svc.LastNotification = now
		// Update notified_on flags
		switch svc.CurrentState {
		case objects.ServiceWarning:
			svc.NotifiedOn |= objects.OptWarning
		case objects.ServiceCritical:
			svc.NotifiedOn |= objects.OptCritical
		case objects.ServiceUnknown:
			svc.NotifiedOn |= objects.OptUnknown
		case objects.ServiceOK:
			svc.NotifiedOn = 0
			svc.CurrentNotificationNumber = 0
			svc.NoMoreNotifications = false
		}
	}

	// Decrement if no contacts notified
	if contactsNotified == 0 && (ntype == objects.NotificationNormal || options&objects.NotificationOptionIncrement != 0) {
		svc.CurrentNotificationNumber--
		if svc.CurrentNotificationNumber < 0 {
			svc.CurrentNotificationNumber = 0
		}
	}

	return 0
}

// HostNotification is the main entry point for sending host notifications.
func (ne *NotificationEngine) HostNotification(hst *objects.Host, ntype int, author, data string, options int) int {
	if ne.checkHostNotificationViability(hst, ntype, options) != 0 {
		return 1
	}

	if ntype == objects.NotificationNormal || options&objects.NotificationOptionIncrement != 0 {
		hst.CurrentNotificationNumber++
	}

	hst.CurrentNotificationID = ne.nextNotifID.Add(1) - 1

	contacts := ne.createHostNotificationList(hst, options)

	contactsNotified := 0
	now := time.Now()
	typeName := objects.NotificationTypeName(ntype, hst.CurrentState, true)

	for _, contact := range contacts {
		if ne.checkContactHostViability(contact, hst, ntype, options) != 0 {
			continue
		}
		ne.notifyContactOfHost(contact, hst, ntype, typeName, author, data)
		contactsNotified++
	}

	if ntype == objects.NotificationNormal && contactsNotified > 0 {
		hst.NextNotification = GetNextHostNotificationTime(hst, now, ne.intervalLength())
		hst.LastNotification = now
		switch hst.CurrentState {
		case objects.HostDown:
			hst.NotifiedOn |= objects.OptDown
		case objects.HostUnreachable:
			hst.NotifiedOn |= objects.OptUnreachable
		case objects.HostUp:
			hst.NotifiedOn = 0
			hst.CurrentNotificationNumber = 0
			hst.NoMoreNotifications = false
		}
	}

	if contactsNotified == 0 && (ntype == objects.NotificationNormal || options&objects.NotificationOptionIncrement != 0) {
		hst.CurrentNotificationNumber--
		if hst.CurrentNotificationNumber < 0 {
			hst.CurrentNotificationNumber = 0
		}
	}

	return 0
}

func (ne *NotificationEngine) intervalLength() int {
	if ne.GlobalState != nil && ne.GlobalState.IntervalLength > 0 {
		return ne.GlobalState.IntervalLength
	}
	return 60
}

// checkServiceNotificationViability implements the exact filter order from Nagios.
func (ne *NotificationEngine) checkServiceNotificationViability(svc *objects.Service, ntype int, options int) int {
	// 1. Forced notifications bypass ALL filters
	if options&objects.NotificationOptionForced != 0 {
		return 0
	}

	// 2. Global notifications disabled
	if ne.GlobalState != nil && !ne.GlobalState.EnableNotifications {
		return 1
	}

	// 3. Host not found
	if svc.Host == nil {
		return 1
	}

	// 4. Service parents all bad
	if len(svc.ServiceParents) > 0 {
		allBad := true
		for _, parent := range svc.ServiceParents {
			if parent.CurrentState == objects.ServiceOK {
				allBad = false
				break
			}
		}
		if allBad {
			return 1
		}
	}

	// 5. Notification period
	if svc.NotificationPeriod != nil && !objects.InTimeperiod(svc.NotificationPeriod, time.Now()) {
		return 1
	}

	// 6. Service notifications disabled
	if !svc.NotificationsEnabled {
		return 1
	}

	// 7. CUSTOM - pass unless in scheduled downtime
	if ntype == objects.NotificationCustom {
		if svc.ScheduledDowntimeDepth > 0 || svc.Host.ScheduledDowntimeDepth > 0 {
			return 1
		}
		return 0
	}

	// 8. ACKNOWLEDGEMENT - pass unless already OK
	if ntype == objects.NotificationAcknowledgement {
		if svc.CurrentState == objects.ServiceOK {
			return 1
		}
		return 0
	}

	// 9. FLAPPING
	if ntype == objects.NotificationFlappingStart || ntype == objects.NotificationFlappingStop || ntype == objects.NotificationFlappingDisabled {
		if svc.NotificationOptions&objects.OptFlapping == 0 {
			return 1
		}
		if svc.ScheduledDowntimeDepth > 0 {
			return 1
		}
		return 0
	}

	// 10. DOWNTIME
	if ntype == objects.NotificationDowntimeStart || ntype == objects.NotificationDowntimeEnd || ntype == objects.NotificationDowntimeCancelled {
		if svc.NotificationOptions&objects.OptDowntime == 0 {
			return 1
		}
		if svc.ScheduledDowntimeDepth > 0 {
			return 1
		}
		return 0
	}

	// 11. NORMAL notifications
	// Must be HARD state
	if svc.StateType != objects.StateTypeHard {
		return 1
	}

	// Already acknowledged
	if svc.ProblemAcknowledged {
		return 1
	}

	// Service notification dependencies
	if dependency.CheckServiceDependencies(svc, objects.NotificationDependency, ne.softStateDeps()) != dependency.DependenciesOK {
		return 1
	}

	// Notification options don't match current state
	if !objects.StateMatchesSvcOptions(svc.CurrentState, svc.NotificationOptions) {
		return 1
	}

	// Recovery with no previous notification
	if svc.CurrentState == objects.ServiceOK && svc.NotifiedOn == 0 {
		return 1
	}

	// first_notification_delay
	if svc.CurrentNotificationNumber == 0 && svc.CurrentState != objects.ServiceOK {
		if svc.FirstNotificationDelay > 0 && !svc.FirstProblemTime.IsZero() {
			delaySeconds := svc.FirstNotificationDelay * float64(ne.intervalLength())
			if time.Since(svc.FirstProblemTime).Seconds() < delaySeconds {
				return 1
			}
		}
	}

	// Currently flapping
	if svc.IsFlapping {
		return 1
	}

	// Recovery passes at this point
	if svc.CurrentState == objects.ServiceOK {
		return 0
	}

	// notification_interval==0 and no_more_notifications
	if svc.NotificationInterval == 0 && svc.NoMoreNotifications {
		return 1
	}

	// Host is DOWN or UNREACHABLE
	if svc.Host.CurrentState != objects.HostUp {
		return 1
	}

	// Not enough time elapsed (unless volatile)
	now := time.Now()
	if !svc.IsVolatile && !svc.NextNotification.IsZero() && now.Before(svc.NextNotification) {
		return 1
	}

	// Service in scheduled downtime
	if svc.ScheduledDowntimeDepth > 0 {
		return 1
	}

	// Host in scheduled downtime
	if svc.Host.ScheduledDowntimeDepth > 0 {
		return 1
	}

	return 0
}

// checkHostNotificationViability implements host notification filters.
func (ne *NotificationEngine) checkHostNotificationViability(hst *objects.Host, ntype int, options int) int {
	if options&objects.NotificationOptionForced != 0 {
		return 0
	}

	if ne.GlobalState != nil && !ne.GlobalState.EnableNotifications {
		return 1
	}

	if hst.NotificationPeriod != nil && !objects.InTimeperiod(hst.NotificationPeriod, time.Now()) {
		return 1
	}

	if !hst.NotificationsEnabled {
		return 1
	}

	if ntype == objects.NotificationCustom {
		if hst.ScheduledDowntimeDepth > 0 {
			return 1
		}
		return 0
	}

	if ntype == objects.NotificationAcknowledgement {
		if hst.CurrentState == objects.HostUp {
			return 1
		}
		return 0
	}

	if ntype == objects.NotificationFlappingStart || ntype == objects.NotificationFlappingStop || ntype == objects.NotificationFlappingDisabled {
		if hst.NotificationOptions&objects.OptFlapping == 0 {
			return 1
		}
		if hst.ScheduledDowntimeDepth > 0 {
			return 1
		}
		return 0
	}

	if ntype == objects.NotificationDowntimeStart || ntype == objects.NotificationDowntimeEnd || ntype == objects.NotificationDowntimeCancelled {
		if hst.NotificationOptions&objects.OptDowntime == 0 {
			return 1
		}
		if hst.ScheduledDowntimeDepth > 0 {
			return 1
		}
		return 0
	}

	// NORMAL
	if hst.StateType != objects.StateTypeHard {
		return 1
	}

	if hst.ProblemAcknowledged {
		return 1
	}

	if dependency.CheckHostDependencies(hst, objects.NotificationDependency, ne.softStateDeps()) != dependency.DependenciesOK {
		return 1
	}

	if !objects.StateMatchesHostOptions(hst.CurrentState, hst.NotificationOptions) {
		return 1
	}

	if hst.CurrentState == objects.HostUp && hst.NotifiedOn == 0 {
		return 1
	}

	if hst.CurrentNotificationNumber == 0 && hst.CurrentState != objects.HostUp {
		if hst.FirstNotificationDelay > 0 && !hst.FirstProblemTime.IsZero() {
			delaySeconds := hst.FirstNotificationDelay * float64(ne.intervalLength())
			if time.Since(hst.FirstProblemTime).Seconds() < delaySeconds {
				return 1
			}
		}
	}

	if hst.IsFlapping {
		return 1
	}

	// Recovery passes
	if hst.CurrentState == objects.HostUp {
		return 0
	}

	if hst.ScheduledDowntimeDepth > 0 {
		return 1
	}

	if hst.NotificationInterval == 0 && hst.NoMoreNotifications {
		return 1
	}

	now := time.Now()
	if !hst.NextNotification.IsZero() && now.Before(hst.NextNotification) {
		return 1
	}

	return 0
}

// checkContactServiceViability checks per-contact filters.
func (ne *NotificationEngine) checkContactServiceViability(contact *objects.Contact, svc *objects.Service, ntype int, options int) int {
	if options&objects.NotificationOptionForced != 0 {
		return 0
	}

	// minimum_value check
	if contact.MinimumImportance > 0 && svc.HourlyValue < contact.MinimumImportance {
		return 1
	}

	if !contact.ServiceNotificationsEnabled {
		return 1
	}

	if contact.ServiceNotificationPeriod != nil && !objects.InTimeperiod(contact.ServiceNotificationPeriod, time.Now()) {
		return 1
	}

	if ntype == objects.NotificationCustom {
		return 0
	}

	if ntype == objects.NotificationFlappingStart || ntype == objects.NotificationFlappingStop || ntype == objects.NotificationFlappingDisabled {
		if contact.ServiceNotificationOptions&objects.OptFlapping == 0 {
			return 1
		}
		return 0
	}

	if ntype == objects.NotificationDowntimeStart || ntype == objects.NotificationDowntimeEnd || ntype == objects.NotificationDowntimeCancelled {
		if contact.ServiceNotificationOptions&objects.OptDowntime == 0 {
			return 1
		}
		return 0
	}

	// State match
	if !objects.StateMatchesSvcOptions(svc.CurrentState, contact.ServiceNotificationOptions) {
		return 1
	}

	// Recovery: contact must have OPT_RECOVERY
	if svc.CurrentState == objects.ServiceOK {
		if contact.ServiceNotificationOptions&objects.OptRecovery == 0 {
			return 1
		}
	}

	return 0
}

// checkContactHostViability checks per-contact host notification filters.
func (ne *NotificationEngine) checkContactHostViability(contact *objects.Contact, hst *objects.Host, ntype int, options int) int {
	if options&objects.NotificationOptionForced != 0 {
		return 0
	}

	if contact.MinimumImportance > 0 && hst.HourlyValue < contact.MinimumImportance {
		return 1
	}

	if !contact.HostNotificationsEnabled {
		return 1
	}

	if contact.HostNotificationPeriod != nil && !objects.InTimeperiod(contact.HostNotificationPeriod, time.Now()) {
		return 1
	}

	if ntype == objects.NotificationCustom {
		return 0
	}

	if ntype == objects.NotificationFlappingStart || ntype == objects.NotificationFlappingStop || ntype == objects.NotificationFlappingDisabled {
		if contact.HostNotificationOptions&objects.OptFlapping == 0 {
			return 1
		}
		return 0
	}

	if ntype == objects.NotificationDowntimeStart || ntype == objects.NotificationDowntimeEnd || ntype == objects.NotificationDowntimeCancelled {
		if contact.HostNotificationOptions&objects.OptDowntime == 0 {
			return 1
		}
		return 0
	}

	if !objects.StateMatchesHostOptions(hst.CurrentState, contact.HostNotificationOptions) {
		return 1
	}

	if hst.CurrentState == objects.HostUp {
		if contact.HostNotificationOptions&objects.OptRecovery == 0 {
			return 1
		}
	}

	return 0
}

// createServiceNotificationList builds the deduplicated contact list.
func (ne *NotificationEngine) createServiceNotificationList(svc *objects.Service, options int) []*objects.Contact {
	seen := make(map[string]bool)
	var contacts []*objects.Contact
	addContact := func(c *objects.Contact) {
		if !seen[c.Name] {
			seen[c.Name] = true
			contacts = append(contacts, c)
		}
	}

	escalated := ShouldServiceNotificationBeEscalated(svc, options)
	broadcast := options&objects.NotificationOptionBroadcast != 0

	if escalated || broadcast {
		for _, esc := range svc.Escalations {
			if !IsValidServiceEscalation(svc, esc, svc.CurrentNotificationNumber, options) {
				continue
			}
			for _, c := range esc.Contacts {
				addContact(c)
			}
			for _, cg := range esc.ContactGroups {
				for _, c := range cg.Members {
					addContact(c)
				}
			}
		}
	}

	if !escalated || broadcast {
		for _, c := range svc.Contacts {
			addContact(c)
		}
		for _, cg := range svc.ContactGroups {
			for _, c := range cg.Members {
				addContact(c)
			}
		}
	}

	return contacts
}

// createHostNotificationList builds the deduplicated host contact list.
func (ne *NotificationEngine) createHostNotificationList(hst *objects.Host, options int) []*objects.Contact {
	seen := make(map[string]bool)
	var contacts []*objects.Contact
	addContact := func(c *objects.Contact) {
		if !seen[c.Name] {
			seen[c.Name] = true
			contacts = append(contacts, c)
		}
	}

	escalated := ShouldHostNotificationBeEscalated(hst, options)
	broadcast := options&objects.NotificationOptionBroadcast != 0

	if escalated || broadcast {
		for _, esc := range hst.Escalations {
			if !IsValidHostEscalation(hst, esc, hst.CurrentNotificationNumber, options) {
				continue
			}
			for _, c := range esc.Contacts {
				addContact(c)
			}
			for _, cg := range esc.ContactGroups {
				for _, c := range cg.Members {
					addContact(c)
				}
			}
		}
	}

	if !escalated || broadcast {
		for _, c := range hst.Contacts {
			addContact(c)
		}
		for _, cg := range hst.ContactGroups {
			for _, c := range cg.Members {
				addContact(c)
			}
		}
	}

	return contacts
}

func (ne *NotificationEngine) notifyContactOfService(contact *objects.Contact, svc *objects.Service, ntype int, typeName, author, data string) {
	for _, cmd := range contact.ServiceNotificationCommands {
		macros := map[string]string{
			"NOTIFICATIONTYPE":    typeName,
			"CONTACTNAME":        contact.Name,
			"CONTACTEMAIL":       contact.Email,
			"CONTACTPAGER":       contact.Pager,
			"HOSTNAME":           svc.Host.Name,
			"HOSTALIAS":          svc.Host.Alias,
			"HOSTADDRESS":        svc.Host.Address,
			"SERVICEDESC":        svc.Description,
			"SERVICESTATE":       objects.ServiceStateName(svc.CurrentState),
			"SERVICESTATETYPE":   objects.StateTypeName(svc.StateType),
			"SERVICEATTEMPT":     itoa(svc.CurrentAttempt),
			"MAXSERVICEATTEMPTS": itoa(svc.MaxCheckAttempts),
			"SERVICEOUTPUT":      svc.PluginOutput,
			"LONGSERVICEOUTPUT":  svc.LongPluginOutput,
			"NOTIFICATIONAUTHOR":  author,
			"NOTIFICATIONCOMMENT": data,
		}
		cmdLine := ExpandMacros(cmd.CommandLine, macros)
		// Log notification
		logMsg := "SERVICE NOTIFICATION: " + contact.Name + ";" + svc.Host.Name + ";" + svc.Description + ";" + typeName + ";" + cmd.Name + ";" + svc.PluginOutput
		if ntype == objects.NotificationCustom || ntype == objects.NotificationAcknowledgement {
			logMsg += ";" + author + ";" + data
		}
		ne.log(logMsg)

		ne.CmdExecutor.Execute(cmdLine)
	}
	contact.LastServiceNotification = time.Now()
}

func (ne *NotificationEngine) notifyContactOfHost(contact *objects.Contact, hst *objects.Host, ntype int, typeName, author, data string) {
	for _, cmd := range contact.HostNotificationCommands {
		macros := map[string]string{
			"NOTIFICATIONTYPE":    typeName,
			"CONTACTNAME":        contact.Name,
			"CONTACTEMAIL":       contact.Email,
			"CONTACTPAGER":       contact.Pager,
			"HOSTNAME":           hst.Name,
			"HOSTALIAS":          hst.Alias,
			"HOSTADDRESS":        hst.Address,
			"HOSTSTATE":          objects.HostStateName(hst.CurrentState),
			"HOSTSTATETYPE":      objects.StateTypeName(hst.StateType),
			"HOSTATTEMPT":        itoa(hst.CurrentAttempt),
			"MAXHOSTATTEMPTS":    itoa(hst.MaxCheckAttempts),
			"HOSTOUTPUT":         hst.PluginOutput,
			"LONGHOSTOUTPUT":     hst.LongPluginOutput,
			"NOTIFICATIONAUTHOR":  author,
			"NOTIFICATIONCOMMENT": data,
		}
		cmdLine := ExpandMacros(cmd.CommandLine, macros)
		logMsg := "HOST NOTIFICATION: " + contact.Name + ";" + hst.Name + ";" + typeName + ";" + cmd.Name + ";" + hst.PluginOutput
		if ntype == objects.NotificationCustom || ntype == objects.NotificationAcknowledgement {
			logMsg += ";" + author + ";" + data
		}
		ne.log(logMsg)

		ne.CmdExecutor.Execute(cmdLine)
	}
	contact.LastHostNotification = time.Now()
}

func (ne *NotificationEngine) softStateDeps() bool {
	if ne.GlobalState != nil {
		return ne.GlobalState.SoftStateDependencies
	}
	return false
}

func itoa(i int) string {
	// Simple int to string without importing strconv
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	buf := make([]byte, 0, 10)
	for i > 0 {
		buf = append(buf, byte('0'+i%10))
		i /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	// reverse
	for l, r := 0, len(buf)-1; l < r; l, r = l+1, r-1 {
		buf[l], buf[r] = buf[r], buf[l]
	}
	return string(buf)
}
