package livestatus

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Query represents a parsed LQL (Livestatus Query Language) request.
type Query struct {
	Table          string
	Columns        []string
	Filters        []*FilterExpr
	Stats          []*StatsExpr
	Sort           []SortSpec
	Limit          int
	Offset         int
	OutputFormat   string // "json", "wrapped_json", "csv", "python"
	ResponseHeader string // "", "fixed16"
	KeepAlive      bool
	ColumnHeaders  bool
	AuthUser       string
}

// SortSpec describes a single sort directive.
type SortSpec struct {
	Column string
	Desc   bool
}

// FilterExpr represents a single filter or a compound filter (And/Or/Negate).
type FilterExpr struct {
	Column     string
	Operator   string
	Value      string
	CompiledRe *regexp.Regexp // pre-compiled for ~ and !~ operators
	IsAnd      bool           // true=And, false=Or (for compound filters)
	IsNegate   bool
	SubFilters []*FilterExpr
}

// StatsExpr represents a single stats directive or a compound stats (StatsAnd/StatsOr).
type StatsExpr struct {
	Column     string
	Operator   string
	Value      string
	CompiledRe *regexp.Regexp // pre-compiled for ~ and !~ operators
	// If Op is an aggregation function:
	Function string // "sum", "avg", "min", "max", "std", or "" for filter-count
	// For compound stats (StatsAnd/StatsOr)
	SubStats []*StatsExpr
	IsAnd    bool // true=And, false=Or
}

// ParseQuery parses an LQL query from a multi-line request string.
func ParseQuery(request string) (*Query, error) {
	lines := strings.Split(strings.TrimRight(request, "\n"), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	// First line: GET <table>
	first := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(first, "GET ") {
		return nil, fmt.Errorf("query must start with GET, got: %s", first)
	}
	q := &Query{
		Table:        strings.TrimSpace(first[4:]),
		OutputFormat: "csv", // default
		Limit:        -1,    // no limit
	}

	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		idx := strings.Index(line, ":")
		if idx < 0 {
			return nil, fmt.Errorf("invalid header line: %s", line)
		}

		header := line[:idx]
		value := strings.TrimSpace(line[idx+1:])

		switch header {
		case "Columns":
			q.Columns = strings.Fields(value)

		case "ColumnHeaders":
			q.ColumnHeaders = value == "on"

		case "Filter":
			f, err := parseFilterExpr(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Filter: %w", err)
			}
			q.Filters = append(q.Filters, f)

		case "And":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid And count: %w", err)
			}
			if len(q.Filters) < n {
				return nil, fmt.Errorf("And: %d requires %d filters, only %d available", n, n, len(q.Filters))
			}
			start := len(q.Filters) - n
			sub := make([]*FilterExpr, n)
			copy(sub, q.Filters[start:])
			q.Filters = q.Filters[:start]
			q.Filters = append(q.Filters, &FilterExpr{SubFilters: sub, IsAnd: true})

		case "Or":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Or count: %w", err)
			}
			if len(q.Filters) < n {
				return nil, fmt.Errorf("Or: %d requires %d filters, only %d available", n, n, len(q.Filters))
			}
			start := len(q.Filters) - n
			sub := make([]*FilterExpr, n)
			copy(sub, q.Filters[start:])
			q.Filters = q.Filters[:start]
			q.Filters = append(q.Filters, &FilterExpr{SubFilters: sub, IsAnd: false})

		case "Negate":
			if len(q.Filters) == 0 {
				return nil, fmt.Errorf("Negate: no filter to negate")
			}
			last := q.Filters[len(q.Filters)-1]
			q.Filters[len(q.Filters)-1] = &FilterExpr{SubFilters: []*FilterExpr{last}, IsNegate: true}

		case "Stats":
			s, err := parseStatsExpr(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Stats: %w", err)
			}
			q.Stats = append(q.Stats, s)

		case "StatsAnd":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid StatsAnd count: %w", err)
			}
			if len(q.Stats) < n {
				return nil, fmt.Errorf("StatsAnd: %d requires %d stats, only %d available", n, n, len(q.Stats))
			}
			start := len(q.Stats) - n
			sub := make([]*StatsExpr, n)
			copy(sub, q.Stats[start:])
			q.Stats = q.Stats[:start]
			q.Stats = append(q.Stats, &StatsExpr{SubStats: sub, IsAnd: true})

		case "StatsOr":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid StatsOr count: %w", err)
			}
			if len(q.Stats) < n {
				return nil, fmt.Errorf("StatsOr: %d requires %d stats, only %d available", n, n, len(q.Stats))
			}
			start := len(q.Stats) - n
			sub := make([]*StatsExpr, n)
			copy(sub, q.Stats[start:])
			q.Stats = q.Stats[:start]
			q.Stats = append(q.Stats, &StatsExpr{SubStats: sub, IsAnd: false})

		case "StatsNegate":
			// Not commonly used but handle gracefully
			if len(q.Stats) > 0 {
				// Just ignore for now
			}

		case "Limit":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Limit: %w", err)
			}
			q.Limit = n

		case "Offset":
			n, err := strconv.Atoi(value)
			if err != nil {
				return nil, fmt.Errorf("invalid Offset: %w", err)
			}
			q.Offset = n

		case "OutputFormat":
			q.OutputFormat = value

		case "ResponseHeader":
			q.ResponseHeader = value

		case "KeepAlive":
			q.KeepAlive = value == "on"

		case "Sort":
			parts := strings.Fields(value)
			if len(parts) < 1 {
				return nil, fmt.Errorf("invalid Sort: %s", value)
			}
			ss := SortSpec{Column: parts[0]}
			if len(parts) >= 2 && strings.ToLower(parts[1]) == "desc" {
				ss.Desc = true
			}
			q.Sort = append(q.Sort, ss)

		case "AuthUser":
			q.AuthUser = value

		default:
			// Ignore unknown headers for forward compatibility
		}
	}

	return q, nil
}

func parseFilterExpr(s string) (*FilterExpr, error) {
	// Format: column operator value
	// Operators: = != < > <= >= ~ !~ ~~ ~~~ =~ !=~ >= <=
	parts := strings.SplitN(s, " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("filter needs at least column and operator: %s", s)
	}
	f := &FilterExpr{
		Column:   parts[0],
		Operator: parts[1],
	}
	if len(parts) >= 3 {
		f.Value = parts[2]
	}
	// Pre-compile regex for ~ and !~ operators (avoids per-row compilation)
	if f.Operator == "~" || f.Operator == "!~" {
		re, err := regexp.Compile(f.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in filter %q: %w", f.Value, err)
		}
		f.CompiledRe = re
	}
	return f, nil
}

func parseStatsExpr(s string) (*StatsExpr, error) {
	// Format: function column  (e.g. "sum execution_time")
	// or: column operator value  (filter-count style, e.g. "state = 0")
	parts := strings.SplitN(s, " ", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("stats needs at least 2 parts: %s", s)
	}

	// Check if first part is an aggregation function
	switch strings.ToLower(parts[0]) {
	case "sum", "avg", "min", "max", "std":
		return &StatsExpr{
			Function: strings.ToLower(parts[0]),
			Column:   parts[1],
		}, nil
	}

	// Otherwise it's a filter-style stat
	se := &StatsExpr{
		Column:   parts[0],
		Operator: parts[1],
	}
	if len(parts) >= 3 {
		se.Value = parts[2]
	}
	// Pre-compile regex for ~ and !~ operators
	if se.Operator == "~" || se.Operator == "!~" {
		re, err := regexp.Compile(se.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid regex in stats %q: %w", se.Value, err)
		}
		se.CompiledRe = re
	}
	return se, nil
}
