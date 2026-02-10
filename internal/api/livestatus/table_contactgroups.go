package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func contactgroupsTable() *Table {
	return &Table{
		Name: "contactgroups",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.ContactGroups))
			for i, cg := range p.Store.ContactGroups {
				rows[i] = cg
			}
			return rows
		},
		Columns: map[string]*Column{
			"name":  {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ContactGroup).Name }},
			"alias": {Name: "alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.ContactGroup).Alias }},
			"members": {Name: "members", Type: "list", Extract: func(r interface{}) interface{} {
				names := make([]string, 0)
				for _, c := range r.(*objects.ContactGroup).Members {
					names = append(names, c.Name)
				}
				return names
			}},
		},
	}
}
