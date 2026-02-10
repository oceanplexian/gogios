package livestatus

import (
	"fmt"
	"sort"
)

// sortRows sorts rows in place according to the query's Sort directives.
func sortRows(rows []interface{}, q *Query, table *Table) {
	if len(q.Sort) == 0 {
		return
	}

	sort.SliceStable(rows, func(i, j int) bool {
		for _, s := range q.Sort {
			col := table.Columns[s.Column]
			if col == nil {
				continue
			}
			vi := col.Extract(rows[i])
			vj := col.Extract(rows[j])
			cmp := compareValues(vi, vj)
			if cmp == 0 {
				continue
			}
			if s.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
}

// compareValues returns -1, 0, or 1 comparing two values.
func compareValues(a, b interface{}) int {
	switch va := a.(type) {
	case int:
		vb, ok := b.(int)
		if !ok {
			return 0
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
		return 0
	case int64:
		vb, ok := b.(int64)
		if !ok {
			return 0
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
		return 0
	case float64:
		vb, ok := b.(float64)
		if !ok {
			return 0
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
		return 0
	case string:
		vb, ok := b.(string)
		if !ok {
			return 0
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
		return 0
	default:
		sa := fmt.Sprintf("%v", a)
		sb := fmt.Sprintf("%v", b)
		if sa < sb {
			return -1
		}
		if sa > sb {
			return 1
		}
		return 0
	}
}
