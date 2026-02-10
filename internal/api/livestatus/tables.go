package livestatus

import "github.com/oceanplexian/gogios/internal/api"

// Column describes a single column in a livestatus table.
type Column struct {
	Name        string
	Description string
	Type        string // "int", "float", "string", "time", "list"
	Extract     func(row interface{}) interface{}
	// ProviderExtract is used for columns that need access to the StateProvider
	// (e.g. comments/downtimes lists that require lookups against managers).
	ProviderExtract func(row interface{}, p *api.StateProvider) interface{}
}

// ExtractValue returns the column value, using ProviderExtract if available.
func (c *Column) ExtractValue(row interface{}, p *api.StateProvider) interface{} {
	if c.ProviderExtract != nil {
		return c.ProviderExtract(row, p)
	}
	return c.Extract(row)
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
