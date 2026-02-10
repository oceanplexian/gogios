package livestatus

import (
	"github.com/oceanplexian/gogios/internal/api"
	"github.com/oceanplexian/gogios/internal/downtime"
)

func commentsTable() *Table {
	return &Table{
		Name: "comments",
		GetRows: func(p *api.StateProvider) []interface{} {
			all := p.Comments.All()
			rows := make([]interface{}, len(all))
			for i, c := range all {
				rows[i] = c
			}
			return rows
		},
		Columns: map[string]*Column{
			"id": {Name: "id", Type: "int", Extract: func(r interface{}) interface{} { return int(r.(*downtime.Comment).CommentID) }},
			"host_name": {Name: "host_name", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).HostName }},
			"service_description": {Name: "service_description", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).ServiceDescription }},
			"author": {Name: "author", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).Author }},
			"comment": {Name: "comment", Type: "string", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).Data }},
			"type": {Name: "type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).CommentType }},
			"entry_type": {Name: "entry_type", Type: "int", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).EntryType }},
			"entry_time": {Name: "entry_time", Type: "time", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).EntryTime }},
			"source": {Name: "source", Type: "int", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).Source }},
			"persistent": {Name: "persistent", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*downtime.Comment).Persistent) }},
			"expires": {Name: "expires", Type: "int", Extract: func(r interface{}) interface{} { return boolToInt(r.(*downtime.Comment).Expires) }},
			"expire_time": {Name: "expire_time", Type: "time", Extract: func(r interface{}) interface{} { return r.(*downtime.Comment).ExpireTime }},
		},
	}
}
