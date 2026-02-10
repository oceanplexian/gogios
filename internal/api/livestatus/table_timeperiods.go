package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/objects"
)

func timeperiodsTable() *Table {
	return &Table{
		Name: "timeperiods",
		GetRows: func(p *api.StateProvider) []interface{} {
			rows := make([]interface{}, len(p.Store.Timeperiods))
			for i, tp := range p.Store.Timeperiods {
				rows[i] = tp
			}
			return rows
		},
		Columns: map[string]*Column{
			"name":  {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Timeperiod).Name }},
			"alias": {Name: "alias", Type: "string", Extract: func(r interface{}) interface{} { return r.(*objects.Timeperiod).Alias }},
		},
	}
}
