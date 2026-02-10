package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func servicegroupsTable() *Table {
	return &Table{
		Name: "servicegroups",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.ServiceGroups))
			for i, sg := range p.Store.ServiceGroups {
				rows[i] = sg
			}
			return rows
		},
		Columns: map[string]*Column{
			"name":  {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ServiceGroup).Name }},
			"alias": {Name: "alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ServiceGroup).Alias }},
			"members": {Name: "members", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, svc := range r.(*objects.ServiceGroup).Members {
					names = append(names, svc.Host.Name)
					names = append(names, svc.Description)
				}
				return names
			}},
			"notes":     {Name: "notes", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ServiceGroup).Notes }},
			"notes_url": {Name: "notes_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ServiceGroup).NotesURL }},
			"action_url": {Name: "action_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ServiceGroup).ActionURL }},
			"num_services": {Name: "num_services", Type: "int", Extract: func(r interface{}) interface{} { return len(r.(*objects.ServiceGroup).Members) }},
			"num_services_ok": {Name: "num_services_ok", Type: "int", Extract: func(r interface{}) interface{} {
				return countSGServicesByState(r.(*objects.ServiceGroup), objects.ServiceOK)
			}},
			"num_services_warn": {Name: "num_services_warn", Type: "int", Extract: func(r interface{}) interface{} {
				return countSGServicesByState(r.(*objects.ServiceGroup), objects.ServiceWarning)
			}},
			"num_services_crit": {Name: "num_services_crit", Type: "int", Extract: func(r interface{}) interface{} {
				return countSGServicesByState(r.(*objects.ServiceGroup), objects.ServiceCritical)
			}},
			"num_services_unknown": {Name: "num_services_unknown", Type: "int", Extract: func(r interface{}) interface{} {
				return countSGServicesByState(r.(*objects.ServiceGroup), objects.ServiceUnknown)
			}},
			"num_services_pending": {Name: "num_services_pending", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, svc := range r.(*objects.ServiceGroup).Members {
					if !svc.HasBeenChecked {
						count++
					}
				}
				return count
			}},
			"worst_service_state": {Name: "worst_service_state", Type: "int", Extract: func(r interface{}) interface{} {
				worst := 0
				for _, svc := range r.(*objects.ServiceGroup).Members {
					if svc.CurrentState > worst {
						worst = svc.CurrentState
					}
				}
				return worst
			}},
		},
	}
}

func countSGServicesByState(sg *objects.ServiceGroup, state int) int {
	count := 0
	for _, svc := range sg.Members {
		if svc.HasBeenChecked && svc.CurrentState == state {
			count++
		}
	}
	return count
}
