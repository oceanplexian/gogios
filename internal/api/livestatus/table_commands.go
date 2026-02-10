package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func commandsTable() *Table {
	return &Table{
		Name: "commands",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.Commands))
			for i, c := range p.Store.Commands {
				rows[i] = c
			}
			return rows
		},
		Columns: map[string]*Column{
			"name": {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Command).Name }},
			"line": {Name: "line", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Command).CommandLine }},
		},
	}
}
