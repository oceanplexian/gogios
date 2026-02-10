package livestatus

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/api"
)

// compareCtx carries pre-compiled state for filter evaluation to avoid
// per-row allocations (e.g. regex compilation).
type compareCtx struct {
	compiledRe *regexp.Regexp
}

// evaluateFilter checks if a row matches a filter expression.
func evaluateFilter(f *FilterExpr, row interface{}, table *Table, provider *api.StateProvider) bool {
	var result bool

	if len(f.SubFilters) > 0 {
		// Compound filter
		if f.IsAnd {
			result = true
			for _, sub := range f.SubFilters {
				if !evaluateFilter(sub, row, table, provider) {
					result = false
					break
				}
			}
		} else {
			result = false
			for _, sub := range f.SubFilters {
				if evaluateFilter(sub, row, table, provider) {
					result = true
					break
				}
			}
		}
	} else {
		// Leaf filter
		col := table.Columns[f.Column]
		if col == nil {
			return false
		}
		result = compareValue(col.ExtractValue(row, provider), f.Operator, f.Value, &compareCtx{compiledRe: f.CompiledRe})
	}

	if f.IsNegate {
		return !result
	}
	return result
}

// evaluateFilters checks if a row matches all filters (implicit AND).
func evaluateFilters(filters []*FilterExpr, row interface{}, table *Table, provider *api.StateProvider) bool {
	for _, f := range filters {
		if !evaluateFilter(f, row, table, provider) {
			return false
		}
	}
	return true
}

// compareValue compares an extracted column value against the filter value.
// The optional ctx carries pre-compiled state (e.g. regex) to avoid per-row work.
func compareValue(colVal interface{}, op, filterVal string, ctx ...*compareCtx) bool {
	var cc *compareCtx
	if len(ctx) > 0 {
		cc = ctx[0]
	}
	switch v := colVal.(type) {
	case string:
		return compareString(v, op, filterVal, cc)
	case int:
		fv, err := strconv.Atoi(filterVal)
		if err != nil {
			return compareString(fmt.Sprintf("%d", v), op, filterVal, cc)
		}
		return compareInt(v, op, fv)
	case int64:
		fv, err := strconv.ParseInt(filterVal, 10, 64)
		if err != nil {
			return compareString(fmt.Sprintf("%d", v), op, filterVal, cc)
		}
		return compareInt64(v, op, fv)
	case float64:
		fv, err := strconv.ParseFloat(filterVal, 64)
		if err != nil {
			return compareString(fmt.Sprintf("%g", v), op, filterVal, cc)
		}
		return compareFloat(v, op, fv)
	case bool:
		iv := 0
		if v {
			iv = 1
		}
		fv, err := strconv.Atoi(filterVal)
		if err != nil {
			return false
		}
		return compareInt(iv, op, fv)
	case []string:
		return compareList(v, op, filterVal)
	case time.Time:
		// Convert to Unix epoch for numeric comparison (Thruk filters on timestamps)
		unix := int64(0)
		if !v.IsZero() {
			unix = v.Unix()
		}
		fv, err := strconv.ParseInt(filterVal, 10, 64)
		if err != nil {
			return compareString(fmt.Sprintf("%d", unix), op, filterVal, cc)
		}
		return compareInt64(unix, op, fv)
	default:
		return compareString(fmt.Sprintf("%v", colVal), op, filterVal, cc)
	}
}

func compareInt(a int, op string, b int) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	default:
		return false
	}
}

func compareInt64(a int64, op string, b int64) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	default:
		return false
	}
}

func compareFloat(a float64, op string, b float64) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	default:
		return false
	}
}

func compareString(a, op, b string, ctx ...*compareCtx) bool {
	switch op {
	case "=":
		return a == b
	case "!=":
		return a != b
	case "<":
		return a < b
	case ">":
		return a > b
	case "<=":
		return a <= b
	case ">=":
		return a >= b
	case "~":
		// Use pre-compiled regex when available (avoids per-row compilation)
		if len(ctx) > 0 && ctx[0] != nil && ctx[0].compiledRe != nil {
			return ctx[0].compiledRe.MatchString(a)
		}
		re, err := regexp.Compile(b)
		if err != nil {
			return false
		}
		return re.MatchString(a)
	case "!~":
		if len(ctx) > 0 && ctx[0] != nil && ctx[0].compiledRe != nil {
			return !ctx[0].compiledRe.MatchString(a)
		}
		re, err := regexp.Compile(b)
		if err != nil {
			return true
		}
		return !re.MatchString(a)
	case "=~":
		return strings.EqualFold(a, b)
	case "!=~":
		return !strings.EqualFold(a, b)
	case "~~":
		return strings.Contains(strings.ToLower(a), strings.ToLower(b))
	case "!~~":
		return !strings.Contains(strings.ToLower(a), strings.ToLower(b))
	default:
		return false
	}
}

func compareList(list []string, op, val string) bool {
	switch op {
	case ">=":
		// list contains val
		for _, s := range list {
			if s == val {
				return true
			}
		}
		return false
	case "!>=":
		for _, s := range list {
			if s == val {
				return false
			}
		}
		return true
	case "=":
		// empty check
		if val == "" {
			return len(list) == 0
		}
		return false
	case "!=":
		if val == "" {
			return len(list) > 0
		}
		return true
	default:
		return false
	}
}
