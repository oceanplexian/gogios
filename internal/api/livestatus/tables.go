package livestatus

import "github.com/oceanplexian/gogios/internal/api"

// Column describes a single column in a livestatus table.
type Column struct {
	Name        string
	Description string
	Type        string // "int", "float", "string", "time", "list"
	Extract     func(row interface{}) interface{}
}

// Table describes a livestatus table with its columns and row retrieval.
type Table struct {
	Name    string
	Columns map[string]*Column
	GetRows func(p *api.StateProvider) []interface{}
}

// Registry maps table names to Table definitions.
var Registry = map[string]*Table{}

func registerTable(t *Table) {
	Registry[t.Name] = t
}

func init() {
	registerTable(hostsTable())
	registerTable(servicesTable())
	registerTable(contactsTable())
	registerTable(contactgroupsTable())
	registerTable(commandsTable())
	registerTable(timeperiodsTable())
	registerTable(hostgroupsTable())
	registerTable(servicegroupsTable())
	registerTable(statusTable())
	registerTable(columnsTable())
	registerTable(commentsTable())
	registerTable(downtimesTable())
	registerTable(logTable())
}
