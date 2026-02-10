package livestatus

import (
	"testing"
)

func TestParseQuery_BasicGET(t *testing.T) {
	q, err := ParseQuery("GET hosts\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Table != "hosts" {
		t.Errorf("Table = %q, want %q", q.Table, "hosts")
	}
	if q.OutputFormat != "csv" {
		t.Errorf("OutputFormat = %q, want %q", q.OutputFormat, "csv")
	}
	if q.Limit != -1 {
		t.Errorf("Limit = %d, want %d", q.Limit, -1)
	}
}

func TestParseQuery_WithColumns(t *testing.T) {
	q, err := ParseQuery("GET services\nColumns: host_name description state\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Columns) != 3 {
		t.Fatalf("len(Columns) = %d, want 3", len(q.Columns))
	}
	want := []string{"host_name", "description", "state"}
	for i, col := range want {
		if q.Columns[i] != col {
			t.Errorf("Columns[%d] = %q, want %q", i, q.Columns[i], col)
		}
	}
}

func TestParseQuery_WithFilters(t *testing.T) {
	q, err := ParseQuery("GET hosts\nFilter: state = 0\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1", len(q.Filters))
	}
	f := q.Filters[0]
	if f.Column != "state" {
		t.Errorf("Column = %q, want %q", f.Column, "state")
	}
	if f.Operator != "=" {
		t.Errorf("Operator = %q, want %q", f.Operator, "=")
	}
	if f.Value != "0" {
		t.Errorf("Value = %q, want %q", f.Value, "0")
	}
}

func TestParseQuery_MultipleFilters(t *testing.T) {
	q, err := ParseQuery("GET hosts\nFilter: state = 0\nFilter: name ~ web\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Filters) != 2 {
		t.Fatalf("len(Filters) = %d, want 2", len(q.Filters))
	}
}

func TestParseQuery_FilterAnd(t *testing.T) {
	q, err := ParseQuery("GET hosts\nFilter: state = 0\nFilter: name ~ web\nAnd: 2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1", len(q.Filters))
	}
	f := q.Filters[0]
	if !f.IsAnd {
		t.Error("IsAnd = false, want true")
	}
	if len(f.SubFilters) != 2 {
		t.Errorf("len(SubFilters) = %d, want 2", len(f.SubFilters))
	}
}

func TestParseQuery_FilterOr(t *testing.T) {
	q, err := ParseQuery("GET hosts\nFilter: state = 0\nFilter: state = 1\nOr: 2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1", len(q.Filters))
	}
	f := q.Filters[0]
	if f.IsAnd {
		t.Error("IsAnd = true, want false")
	}
	if len(f.SubFilters) != 2 {
		t.Errorf("len(SubFilters) = %d, want 2", len(f.SubFilters))
	}
}

func TestParseQuery_FilterNegate(t *testing.T) {
	q, err := ParseQuery("GET hosts\nFilter: state = 0\nNegate:\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1", len(q.Filters))
	}
	f := q.Filters[0]
	if !f.IsNegate {
		t.Error("IsNegate = false, want true")
	}
	if len(f.SubFilters) != 1 {
		t.Errorf("len(SubFilters) = %d, want 1", len(f.SubFilters))
	}
}

func TestParseQuery_Stats(t *testing.T) {
	q, err := ParseQuery("GET hosts\nStats: state = 0\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Stats) != 1 {
		t.Fatalf("len(Stats) = %d, want 1", len(q.Stats))
	}
	s := q.Stats[0]
	if s.Column != "state" {
		t.Errorf("Column = %q, want %q", s.Column, "state")
	}
	if s.Operator != "=" {
		t.Errorf("Operator = %q, want %q", s.Operator, "=")
	}
	if s.Value != "0" {
		t.Errorf("Value = %q, want %q", s.Value, "0")
	}
	if s.Function != "" {
		t.Errorf("Function = %q, want %q", s.Function, "")
	}
}

func TestParseQuery_StatsAggregation(t *testing.T) {
	q, err := ParseQuery("GET services\nStats: sum execution_time\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Stats) != 1 {
		t.Fatalf("len(Stats) = %d, want 1", len(q.Stats))
	}
	s := q.Stats[0]
	if s.Function != "sum" {
		t.Errorf("Function = %q, want %q", s.Function, "sum")
	}
	if s.Column != "execution_time" {
		t.Errorf("Column = %q, want %q", s.Column, "execution_time")
	}
}

func TestParseQuery_StatsAnd(t *testing.T) {
	q, err := ParseQuery("GET hosts\nStats: state = 0\nStats: state = 1\nStatsAnd: 2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Stats) != 1 {
		t.Fatalf("len(Stats) = %d, want 1", len(q.Stats))
	}
	s := q.Stats[0]
	if !s.IsAnd {
		t.Error("IsAnd = false, want true")
	}
	if len(s.SubStats) != 2 {
		t.Errorf("len(SubStats) = %d, want 2", len(s.SubStats))
	}
}

func TestParseQuery_StatsOr(t *testing.T) {
	q, err := ParseQuery("GET hosts\nStats: state = 0\nStats: state = 1\nStatsOr: 2\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Stats) != 1 {
		t.Fatalf("len(Stats) = %d, want 1", len(q.Stats))
	}
	s := q.Stats[0]
	if s.IsAnd {
		t.Error("IsAnd = true, want false")
	}
	if len(s.SubStats) != 2 {
		t.Errorf("len(SubStats) = %d, want 2", len(s.SubStats))
	}
}

func TestParseQuery_Sort(t *testing.T) {
	q, err := ParseQuery("GET hosts\nSort: name asc\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.Sort) != 1 {
		t.Fatalf("len(Sort) = %d, want 1", len(q.Sort))
	}
	if q.Sort[0].Column != "name" {
		t.Errorf("Column = %q, want %q", q.Sort[0].Column, "name")
	}
	if q.Sort[0].Desc {
		t.Error("Desc = true, want false")
	}

	q2, err := ParseQuery("GET hosts\nSort: state desc\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !q2.Sort[0].Desc {
		t.Error("Desc = false, want true")
	}
}

func TestParseQuery_LimitOffset(t *testing.T) {
	q, err := ParseQuery("GET hosts\nLimit: 50\nOffset: 10\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Limit != 50 {
		t.Errorf("Limit = %d, want 50", q.Limit)
	}
	if q.Offset != 10 {
		t.Errorf("Offset = %d, want 10", q.Offset)
	}
}

func TestParseQuery_OutputFormat(t *testing.T) {
	q, err := ParseQuery("GET hosts\nOutputFormat: json\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", q.OutputFormat, "json")
	}
}

func TestParseQuery_ResponseHeader(t *testing.T) {
	q, err := ParseQuery("GET hosts\nResponseHeader: fixed16\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.ResponseHeader != "fixed16" {
		t.Errorf("ResponseHeader = %q, want %q", q.ResponseHeader, "fixed16")
	}
}

func TestParseQuery_KeepAlive(t *testing.T) {
	q, err := ParseQuery("GET hosts\nKeepAlive: on\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !q.KeepAlive {
		t.Error("KeepAlive = false, want true")
	}
}

func TestParseQuery_ColumnHeaders(t *testing.T) {
	q, err := ParseQuery("GET hosts\nColumnHeaders: on\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !q.ColumnHeaders {
		t.Error("ColumnHeaders = false, want true")
	}
}

func TestParseQuery_InvalidNoGET(t *testing.T) {
	_, err := ParseQuery("FETCH hosts\n")
	if err == nil {
		t.Fatal("expected error for non-GET query, got nil")
	}
}

func TestParseQuery_InvalidAndCount(t *testing.T) {
	_, err := ParseQuery("GET hosts\nFilter: state = 0\nAnd: 3\n")
	if err == nil {
		t.Fatal("expected error for And count exceeding filters, got nil")
	}
}

func TestParseQuery_EmptyQuery(t *testing.T) {
	_, err := ParseQuery("")
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestParseQuery_FullThrukStyle(t *testing.T) {
	query := "GET services\n" +
		"Columns: host_name description state plugin_output\n" +
		"Filter: host_groups >= linux-servers\n" +
		"Filter: state != 0\n" +
		"And: 2\n" +
		"Sort: state desc\n" +
		"Limit: 100\n" +
		"OutputFormat: json\n" +
		"ResponseHeader: fixed16\n"

	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.Table != "services" {
		t.Errorf("Table = %q, want %q", q.Table, "services")
	}
	if len(q.Columns) != 4 {
		t.Errorf("len(Columns) = %d, want 4", len(q.Columns))
	}
	if len(q.Filters) != 1 {
		t.Fatalf("len(Filters) = %d, want 1 (compound And)", len(q.Filters))
	}
	if !q.Filters[0].IsAnd {
		t.Error("compound filter IsAnd = false, want true")
	}
	if len(q.Filters[0].SubFilters) != 2 {
		t.Errorf("len(SubFilters) = %d, want 2", len(q.Filters[0].SubFilters))
	}
	if len(q.Sort) != 1 || q.Sort[0].Column != "state" || !q.Sort[0].Desc {
		t.Errorf("Sort unexpected: %+v", q.Sort)
	}
	if q.Limit != 100 {
		t.Errorf("Limit = %d, want 100", q.Limit)
	}
	if q.OutputFormat != "json" {
		t.Errorf("OutputFormat = %q, want %q", q.OutputFormat, "json")
	}
	if q.ResponseHeader != "fixed16" {
		t.Errorf("ResponseHeader = %q, want %q", q.ResponseHeader, "fixed16")
	}
}
