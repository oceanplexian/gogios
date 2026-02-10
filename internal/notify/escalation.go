// Package notify implements the Nagios notification system.
package notify

import (
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// IsValidServiceEscalation checks if an escalation entry is valid for the current notification.
func IsValidServiceEscalation(svc *objects.Service, esc *objects.ServiceEscalation, notifNum int, options int) bool {
	// BROADCAST overrides all checks
	if options&objects.NotificationOptionBroadcast != 0 {
		return true
	}

	// Check notification number range
	num := notifNum
	if svc.CurrentState == objects.ServiceOK {
		// For recovery, use previous notification number
		num = notifNum - 1
	}
	if esc.FirstNotification > 0 && num < esc.FirstNotification {
		return false
	}
	if esc.LastNotification > 0 && num > esc.LastNotification {
		return false
	}

	// Check escalation options match current state
	if esc.EscalationOptions != 0 && !objects.StateMatchesSvcOptions(svc.CurrentState, esc.EscalationOptions) {
		return false
	}

	// Check escalation period
	if esc.EscalationPeriod != nil && !objects.InTimeperiod(esc.EscalationPeriod, time.Now()) {
		return false
	}

	return true
}

// IsValidHostEscalation checks if a host escalation entry is valid.
func IsValidHostEscalation(hst *objects.Host, esc *objects.HostEscalation, notifNum int, options int) bool {
	if options&objects.NotificationOptionBroadcast != 0 {
		return true
	}

	num := notifNum
	if hst.CurrentState == objects.HostUp {
		num = notifNum - 1
	}
	if esc.FirstNotification > 0 && num < esc.FirstNotification {
		return false
	}
	if esc.LastNotification > 0 && num > esc.LastNotification {
		return false
	}

	if esc.EscalationOptions != 0 && !objects.StateMatchesHostOptions(hst.CurrentState, esc.EscalationOptions) {
		return false
	}

	if esc.EscalationPeriod != nil && !objects.InTimeperiod(esc.EscalationPeriod, time.Now()) {
		return false
	}

	return true
}

// ShouldServiceNotificationBeEscalated checks if any escalation is valid.
func ShouldServiceNotificationBeEscalated(svc *objects.Service, options int) bool {
	for _, esc := range svc.Escalations {
		if IsValidServiceEscalation(svc, esc, svc.CurrentNotificationNumber, options) {
			return true
		}
	}
	return false
}

// ShouldHostNotificationBeEscalated checks if any host escalation is valid.
func ShouldHostNotificationBeEscalated(hst *objects.Host, options int) bool {
	for _, esc := range hst.Escalations {
		if IsValidHostEscalation(hst, esc, hst.CurrentNotificationNumber, options) {
			return true
		}
	}
	return false
}

// GetNextServiceNotificationTime calculates when the next notification should be sent.
func GetNextServiceNotificationTime(svc *objects.Service, offset time.Time, intervalLength int) time.Time {
	interval := svc.NotificationInterval

	// Check escalations for shortest interval
	hasEscInterval := false
	for _, esc := range svc.Escalations {
		if !IsValidServiceEscalation(svc, esc, svc.CurrentNotificationNumber, 0) {
			continue
		}
		if esc.NotificationInterval >= 0 {
			if !hasEscInterval || esc.NotificationInterval < interval {
				interval = esc.NotificationInterval
				hasEscInterval = true
			}
		}
	}

	if interval == 0 {
		svc.NoMoreNotifications = true
	}

	return offset.Add(time.Duration(interval*float64(intervalLength)) * time.Second)
}

// GetNextHostNotificationTime calculates when the next host notification should be sent.
func GetNextHostNotificationTime(hst *objects.Host, offset time.Time, intervalLength int) time.Time {
	interval := hst.NotificationInterval

	hasEscInterval := false
	for _, esc := range hst.Escalations {
		if !IsValidHostEscalation(hst, esc, hst.CurrentNotificationNumber, 0) {
			continue
		}
		if esc.NotificationInterval >= 0 {
			if !hasEscInterval || esc.NotificationInterval < interval {
				interval = esc.NotificationInterval
				hasEscInterval = true
			}
		}
	}

	if interval == 0 {
		hst.NoMoreNotifications = true
	}

	return offset.Add(time.Duration(interval*float64(intervalLength)) * time.Second)
}
