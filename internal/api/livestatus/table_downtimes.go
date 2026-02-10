package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/downtime"
)

func downtimesTable() *Table {
	return &Table{
		Name: "downtimes",
		GetRows: func(p *api.StateProvider) []interface{} {
			all := p.Downtimes.All()
			rows := make([]interface{}, len(all))
			for i, d := range all {
				rows[i] = d
			}
			return rows
		},
		Columns: map[string]*Column{
			"id": {Name: "id", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*downtime.Downtime).DowntimeID) }},
			"host_name": {Name: "host_name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).HostName }},
			"service_description": {Name: "service_description", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).ServiceDescription }},
			"author": {Name: "author", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).Author }},
			"comment": {Name: "comment", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).Comment }},
			"type": {Name: "type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).Type }},
			"start_time": {Name: "start_time", Type: "time", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).StartTime }},
			"end_time": {Name: "end_time", Type: "time", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).EndTime }},
			"entry_time": {Name: "entry_time", Type: "time", Extract: func(r interface{}) interface{} { return r.(*downtime.Downtime).EntryTime }},
			"fixed": {Name: "fixed", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*downtime.Downtime).Fixed) }},
			"duration": {Name: "duration", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*downtime.Downtime).Duration.Seconds()) }},
			"triggered_by": {Name: "triggered_by", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*downtime.Downtime).TriggeredBy) }},
		},
	}
}
