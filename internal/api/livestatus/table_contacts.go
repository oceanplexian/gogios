package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func contactsTable() *Table {
	return &Table{
		Name: "contacts",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.Contacts))
			for i, c := range p.Store.Contacts {
				rows[i] = c
			}
			return rows
		},
		Columns: map[string]*Column{
			"name":  {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Contact).Name }},
			"alias": {Name: "alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Contact).Alias }},
			"email": {Name: "email", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Contact).Email }},
			"pager": {Name: "pager", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Contact).Pager }},
			"host_notifications_enabled": {Name: "host_notifications_enabled", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*objects.Contact).HostNotificationsEnabled)
			}},
			"service_notifications_enabled": {Name: "service_notifications_enabled", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*objects.Contact).ServiceNotificationsEnabled)
			}},
			"can_submit_commands": {Name: "can_submit_commands", Type: "int", Extract: func(r interface{}) interface{} {
				return boolToInt(r.(*objects.Contact).CanSubmitCommands)
			}},
			"host_notification_period": {Name: "host_notification_period", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Contact).HostNotificationPeriod != nil {
					return r.(*objects.Contact).HostNotificationPeriod.Name
				}
				return ""
			}},
			"service_notification_period": {Name: "service_notification_period", Type: "string", Extract: func(r interface{}) interface{} {
				if r.(*objects.Contact).ServiceNotificationPeriod != nil {
					return r.(*objects.Contact).ServiceNotificationPeriod.Name
				}
				return ""
			}},
			"custom_variable_names": {Name: "custom_variable_names", Type: "list", Extract: func(r interface{}) interface{} {
				var names []string
				for k := range r.(*objects.Contact).CustomVars {
					names = append(names, k)
				}
				return names
			}},
			"custom_variable_values": {Name: "custom_variable_values", Type: "list", Extract: func(r interface{}) interface{} {
				var vals []string
				for _, v := range r.(*objects.Contact).CustomVars {
					vals = append(vals, v)
				}
				return vals
			}},
		},
	}
}
