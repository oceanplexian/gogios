package livestatus

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
)

// ExecuteQuery runs a parsed query against the provider and returns the response string.
func ExecuteQuery(q *Query, provider *api.StateProvider) string {
	table := Registry[q.Table]
	if table == nil {
		return errorResponse(q, 404, "Unknown table: "+q.Table)
	}

	// Snapshot the row pointers under a brief read lock, then release.
	// Filtering, sorting, stats, and formatting run lock-free on the
	// snapshot.  A concurrent check-result write may update individual
	// struct fields mid-query, but for monitoring data this is acceptable
	// (at worst one object shows a mix of old/new fields for one cycle).
	provider.Store.Mu.RLock()
	rows := table.GetRows(provider)
	provider.Store.Mu.RUnlock()

	// Fast path: ungrouped, filter-count-only stats can be evaluated in a
	// single pass without materializing the filtered slice.
	if len(q.Stats) > 0 && len(q.Columns) == 0 && canSinglePassStats(q.Stats) {
		results := evaluateStatsSinglePass(q, rows, table, provider)
		var row []interface{}
		for _, v := range results {
			row = append(row, v)
		}
		return formatResponse(q, nil, [][]interface{}{row})
	}

	// Apply filters
	filtered := make([]interface{}, 0, len(rows))
	for _, row := range rows {
		if evaluateFilters(q.Filters, row, table, provider) {
			filtered = append(filtered, row)
		}
	}

	// Stats mode (grouped or aggregate stats that need the filtered set)
	if len(q.Stats) > 0 {
		return formatStatsResponse(q, filtered, table, provider)
	}

	// Sort
	sortRows(filtered, q, table, provider)

	// Offset
	if q.Offset > 0 {
		if q.Offset >= len(filtered) {
			filtered = nil
		} else {
			filtered = filtered[q.Offset:]
		}
	}

	// Limit
	if q.Limit >= 0 && q.Limit < len(filtered) {
		filtered = filtered[:q.Limit]
	}

	// Determine columns to output
	cols := q.Columns
	if len(cols) == 0 {
		// Default: all columns
		for name := range table.Columns {
			cols = append(cols, name)
		}
	}

	// Build result rows
	var resultRows [][]interface{}
	for _, row := range filtered {
		var resultRow []interface{}
		for _, colName := range cols {
			col := table.Columns[colName]
			if col == nil {
				resultRow = append(resultRow, "")
				continue
			}
			resultRow = append(resultRow, col.ExtractValue(row, provider))
		}
		resultRows = append(resultRows, resultRow)
	}

	return formatResponse(q, cols, resultRows)
}

func formatStatsResponse(q *Query, filtered []interface{}, table *Table, provider *api.StateProvider) string {
	if len(q.Columns) > 0 {
		// Grouped stats
		groupedResults := evaluateStatsGroupBy(q, q.Stats, filtered, table, provider)
		var resultRows [][]interface{}
		for _, row := range groupedResults {
			resultRows = append(resultRows, row)
		}
		return formatResponse(q, nil, resultRows)
	}

	// Ungrouped stats
	results := evaluateStats(q.Stats, filtered, table, provider)
	var row []interface{}
	for _, v := range results {
		row = append(row, v)
	}
	return formatResponse(q, nil, [][]interface{}{row})
}

func formatResponse(q *Query, cols []string, rows [][]interface{}) string {
	var body string

	switch q.OutputFormat {
	case "json":
		body = formatJSON(q, cols, rows)
	case "wrapped_json":
		body = formatWrappedJSON(q, cols, rows)
	case "python":
		body = formatJSON(q, cols, rows) // close enough
	default: // csv
		body = formatCSV(q, cols, rows)
	}

	if q.ResponseHeader == "fixed16" {
		header := fmt.Sprintf("%3d %11d\n", 200, len(body))
		return header + body
	}
	return body
}

func formatCSV(q *Query, cols []string, rows [][]interface{}) string {
	var sb strings.Builder
	if q.ColumnHeaders {
		if cols != nil {
			sb.WriteString(strings.Join(cols, ";"))
			sb.WriteString("\n")
		}
	}
	for _, row := range rows {
		for i, val := range row {
			if i > 0 {
				sb.WriteString(";")
			}
			sb.WriteString(formatValue(val))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func formatJSON(q *Query, cols []string, rows [][]interface{}) string {
	// JSON output: array of arrays
	output := make([][]interface{}, 0, len(rows))
	if q.ColumnHeaders && cols != nil {
		var headerRow []interface{}
		for _, c := range cols {
			headerRow = append(headerRow, c)
		}
		output = append(output, headerRow)
	}
	for _, row := range rows {
		jsonRow := make([]interface{}, len(row))
		for i, v := range row {
			jsonRow[i] = jsonSafe(v)
		}
		output = append(output, jsonRow)
	}
	data, err := json.Marshal(output)
	if err != nil {
		return "[]"
	}
	return string(data) + "\n"
}

func formatWrappedJSON(q *Query, cols []string, rows [][]interface{}) string {
	// wrapped_json: {"columns": [...], "data": [...], "total_count": N}
	output := make([][]interface{}, 0, len(rows))
	for _, row := range rows {
		jsonRow := make([]interface{}, len(row))
		for i, v := range row {
			jsonRow[i] = jsonSafe(v)
		}
		output = append(output, jsonRow)
	}

	wrapper := map[string]interface{}{
		"data":        output,
		"total_count": len(rows),
	}
	if cols != nil {
		wrapper["columns"] = cols
	}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return "{}"
	}
	return string(data) + "\n"
}

func formatValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return fmt.Sprintf("%d", val)
	case int64:
		return fmt.Sprintf("%d", val)
	case float64:
		// Use integer format if it's a whole number
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val))
		}
		return fmt.Sprintf("%.6f", val)
	case bool:
		if val {
			return "1"
		}
		return "0"
	case time.Time:
		if val.IsZero() {
			return "0"
		}
		return fmt.Sprintf("%d", val.Unix())
	case []string:
		return strings.Join(val, ",")
	default:
		return fmt.Sprintf("%v", v)
	}
}

func jsonSafe(v interface{}) interface{} {
	switch val := v.(type) {
	case time.Time:
		if val.IsZero() {
			return 0
		}
		return val.Unix()
	case []string:
		if val == nil {
			return []string{}
		}
		return val
	default:
		return v
	}
}

func errorResponse(q *Query, code int, msg string) string {
	body := msg + "\n"
	if q.ResponseHeader == "fixed16" {
		header := fmt.Sprintf("%3d %11d\n", code, len(body))
		return header + body
	}
	return body
}
