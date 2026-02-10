package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func hostgroupsTable() *Table {
	return &Table{
		Name: "hostgroups",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.HostGroups))
			for i, hg := range p.Store.HostGroups {
				rows[i] = hg
			}
			return rows
		},
		Columns: map[string]*Column{
			"name":  {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.HostGroup).Name }},
			"alias": {Name: "alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.HostGroup).Alias }},
			"members": {Name: "members", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, h := range r.(*objects.HostGroup).Members {
					names = append(names, h.Name)
				}
				return names
			}},
			"notes":     {Name: "notes", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.HostGroup).Notes }},
			"notes_url": {Name: "notes_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.HostGroup).NotesURL }},
			"action_url": {Name: "action_url", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.HostGroup).ActionURL }},
			"num_hosts": {Name: "num_hosts", Type: "int", Extract: func(r interface{}) interface{} { return len(r.(*objects.HostGroup).Members) }},
			"num_hosts_up": {Name: "num_hosts_up", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, h := range r.(*objects.HostGroup).Members {
					if h.HasBeenChecked && h.CurrentState == objects.HostUp {
						count++
					}
				}
				return count
			}},
			"num_hosts_down": {Name: "num_hosts_down", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, h := range r.(*objects.HostGroup).Members {
					if h.HasBeenChecked && h.CurrentState == objects.HostDown {
						count++
					}
				}
				return count
			}},
			"num_hosts_unreach": {Name: "num_hosts_unreach", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, h := range r.(*objects.HostGroup).Members {
					if h.HasBeenChecked && h.CurrentState == objects.HostUnreachable {
						count++
					}
				}
				return count
			}},
			"num_hosts_pending": {Name: "num_hosts_pending", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, h := range r.(*objects.HostGroup).Members {
					if !h.HasBeenChecked {
						count++
					}
				}
				return count
			}},
			"num_services": {Name: "num_services", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, h := range r.(*objects.HostGroup).Members {
					count += len(h.Services)
				}
				return count
			}},
			"num_services_ok": {Name: "num_services_ok", Type: "int", Extract: func(r interface{}) interface{} {
				return countHGServicesByState(r.(*objects.HostGroup), objects.ServiceOK)
			}},
			"num_services_warn": {Name: "num_services_warn", Type: "int", Extract: func(r interface{}) interface{} {
				return countHGServicesByState(r.(*objects.HostGroup), objects.ServiceWarning)
			}},
			"num_services_crit": {Name: "num_services_crit", Type: "int", Extract: func(r interface{}) interface{} {
				return countHGServicesByState(r.(*objects.HostGroup), objects.ServiceCritical)
			}},
			"num_services_unknown": {Name: "num_services_unknown", Type: "int", Extract: func(r interface{}) interface{} {
				return countHGServicesByState(r.(*objects.HostGroup), objects.ServiceUnknown)
			}},
			"num_services_pending": {Name: "num_services_pending", Type: "int", Extract: func(r interface{}) interface{} {
				count := 0
				for _, h := range r.(*objects.HostGroup).Members {
					for _, svc := range h.Services {
						if !svc.HasBeenChecked {
							count++
						}
					}
				}
				return count
			}},
			"worst_host_state": {Name: "worst_host_state", Type: "int", Extract: func(r interface{}) interface{} {
				worst := 0
				for _, h := range r.(*objects.HostGroup).Members {
					if h.CurrentState > worst {
						worst = h.CurrentState
					}
				}
				return worst
			}},
			"worst_service_state": {Name: "worst_service_state", Type: "int", Extract: func(r interface{}) interface{} {
				worst := 0
				for _, h := range r.(*objects.HostGroup).Members {
					for _, svc := range h.Services {
						if svc.CurrentState > worst {
							worst = svc.CurrentState
						}
					}
				}
				return worst
			}},
		},
	}
}

func countHGServicesByState(hg *objects.HostGroup, state int) int {
	count := 0
	for _, h := range hg.Members {
		for _, svc := range h.Services {
			if svc.HasBeenChecked && svc.CurrentState == state {
				count++
			}
		}
	}
	return count
}
