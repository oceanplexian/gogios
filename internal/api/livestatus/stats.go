package livestatus

import (
	"math"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
)

// evaluateStats computes stats results for a set of rows that already passed filters.
func evaluateStats(stats []*StatsExpr, rows []interface{}, table *Table, provider *api.StateProvider) []float64 {
	results := make([]float64, len(stats))

	for i, s := range stats {
		if len(s.SubStats) > 0 {
			results[i] = evaluateCompoundStat(s, rows, table, provider)
		} else if s.Function != "" {
			results[i] = evaluateAggStat(s, rows, table, provider)
		} else {
			results[i] = evaluateFilterStat(s, rows, table, provider)
		}
	}

	return results
}

// evaluateFilterStat counts rows matching a filter-style stat.
func evaluateFilterStat(s *StatsExpr, rows []interface{}, table *Table, provider *api.StateProvider) float64 {
	count := 0.0
	col := table.Columns[s.Column]
	if col == nil {
		return 0
	}
	for _, row := range rows {
		if compareValue(col.ExtractValue(row, provider), s.Operator, s.Value) {
			count++
		}
	}
	return count
}

// evaluateAggStat computes an aggregate function over a column.
func evaluateAggStat(s *StatsExpr, rows []interface{}, table *Table, provider *api.StateProvider) float64 {
	col := table.Columns[s.Column]
	if col == nil {
		return 0
	}

	var vals []float64
	for _, row := range rows {
		v := col.ExtractValue(row, provider)
		vals = append(vals, toFloat64(v))
	}
	if len(vals) == 0 {
		return 0
	}

	switch s.Function {
	case "sum":
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		return sum
	case "avg":
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		return sum / float64(len(vals))
	case "min":
		m := vals[0]
		for _, v := range vals[1:] {
			if v < m {
				m = v
			}
		}
		return m
	case "max":
		m := vals[0]
		for _, v := range vals[1:] {
			if v > m {
				m = v
			}
		}
		return m
	case "std":
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		mean := sum / float64(len(vals))
		variance := 0.0
		for _, v := range vals {
			d := v - mean
			variance += d * d
		}
		variance /= float64(len(vals))
		return math.Sqrt(variance)
	default:
		return 0
	}
}

// evaluateCompoundStat evaluates StatsAnd/StatsOr combinations.
func evaluateCompoundStat(s *StatsExpr, rows []interface{}, table *Table, provider *api.StateProvider) float64 {
	count := 0.0
	for _, row := range rows {
		match := evaluateCompoundStatRow(s, row, table, provider)
		if match {
			count++
		}
	}
	return count
}

// evaluateCompoundStatRow checks if a single row matches a compound stat.
func evaluateCompoundStatRow(s *StatsExpr, row interface{}, table *Table, provider *api.StateProvider) bool {
	if s.IsAnd {
		for _, sub := range s.SubStats {
			if !statRowMatch(sub, row, table, provider) {
				return false
			}
		}
		return true
	}
	// Or
	for _, sub := range s.SubStats {
		if statRowMatch(sub, row, table, provider) {
			return true
		}
	}
	return false
}

// statRowMatch checks if a single row matches a single stat expression.
func statRowMatch(s *StatsExpr, row interface{}, table *Table, provider *api.StateProvider) bool {
	if len(s.SubStats) > 0 {
		return evaluateCompoundStatRow(s, row, table, provider)
	}
	col := table.Columns[s.Column]
	if col == nil {
		return false
	}
	return compareValue(col.ExtractValue(row, provider), s.Operator, s.Value)
}

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case bool:
		if val {
			return 1
		}
		return 0
	case time.Time:
		if val.IsZero() {
			return 0
		}
		return float64(val.Unix())
	default:
		return 0
	}
}

// evaluateStatsGroupBy computes stats grouped by the requested columns.
func evaluateStatsGroupBy(q *Query, stats []*StatsExpr, rows []interface{}, table *Table, provider *api.StateProvider) [][]interface{} {
	type groupKey string

	groups := map[groupKey][]interface{}{}
	var keyOrder []groupKey

	for _, row := range rows {
		var keyParts []string
		for _, col := range q.Columns {
			c := table.Columns[col]
			if c == nil {
				keyParts = append(keyParts, "")
				continue
			}
			v := c.ExtractValue(row, provider)
			keyParts = append(keyParts, formatValue(v))
		}
		key := groupKey(joinKey(keyParts))
		if _, ok := groups[key]; !ok {
			keyOrder = append(keyOrder, key)
		}
		groups[key] = append(groups[key], row)
	}

	var result [][]interface{}
	for _, key := range keyOrder {
		groupRows := groups[key]
		statResults := evaluateStats(stats, groupRows, table, provider)

		// Build result row: column values + stat values
		var resultRow []interface{}
		// Get column values from first row in group
		if len(groupRows) > 0 {
			for _, col := range q.Columns {
				c := table.Columns[col]
				if c == nil {
					resultRow = append(resultRow, "")
					continue
				}
				resultRow = append(resultRow, c.ExtractValue(groupRows[0], provider))
			}
		}
		for _, sv := range statResults {
			resultRow = append(resultRow, sv)
		}
		result = append(result, resultRow)
	}

	return result
}

func joinKey(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\x00"
		}
		result += p
	}
	return result
}
