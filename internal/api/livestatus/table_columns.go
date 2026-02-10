package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
)

// columnRow represents a single entry in the columns meta-table.
type columnRow struct {
	table       string
	name        string
	description string
	colType     string
}

func columnsTable() *Table {
	return &Table{
		Name: "columns",
		GetRows: func(p *api.StateProvider) []interface{} {
			var rows []interface{}
			for tableName, table := range Registry {
				for _, col := range table.Columns {
					rows = append(rows, &columnRow{
						table:       tableName,
						name:        col.Name,
						description: col.Description,
						colType:     col.Type,
					})
				}
			}
			return rows
		},
		Columns: map[string]*Column{
			"table":       {Name: "table", Type: "string", Extract: func(r interface{}) interface{} { return r.(*columnRow).table }},
			"name":        {Name: "name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*columnRow).name }},
			"description": {Name: "description", Type: "string", Extract: func(r interface{}) interface{} { return r.(*columnRow).description }},
			"type":        {Name: "type", Type: "string", Extract: func(r interface{}) interface{} { return r.(*columnRow).colType }},
		},
	}
}
