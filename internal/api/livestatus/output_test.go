package livestatus

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFormatValue_String(t *testing.T) {
	if got := formatValue("hello"); got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestFormatValue_Int(t *testing.T) {
	if got := formatValue(42); got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
}

func TestFormatValue_Int64(t *testing.T) {
	if got := formatValue(int64(123456789)); got != "123456789" {
		t.Errorf("got %q, want %q", got, "123456789")
	}
}

func TestFormatValue_Float_Whole(t *testing.T) {
	if got := formatValue(3.0); got != "3" {
		t.Errorf("got %q, want %q", got, "3")
	}
}

func TestFormatValue_Float_Fraction(t *testing.T) {
	got := formatValue(3.14)
	if !strings.Contains(got, "3.14") {
		t.Errorf("got %q, want it to contain %q", got, "3.14")
	}
}

func TestFormatValue_Bool(t *testing.T) {
	if got := formatValue(true); got != "1" {
		t.Errorf("formatValue(true) = %q, want %q", got, "1")
	}
	if got := formatValue(false); got != "0" {
		t.Errorf("formatValue(false) = %q, want %q", got, "0")
	}
}

func TestFormatValue_Time(t *testing.T) {
	if got := formatValue(time.Unix(1700000000, 0)); got != "1700000000" {
		t.Errorf("got %q, want %q", got, "1700000000")
	}
	if got := formatValue(time.Time{}); got != "0" {
		t.Errorf("got %q, want %q", got, "0")
	}
}

func TestFormatValue_StringList(t *testing.T) {
	if got := formatValue([]string{"a", "b", "c"}); got != "a,b,c" {
		t.Errorf("got %q, want %q", got, "a,b,c")
	}
}

func TestJsonSafe_Time(t *testing.T) {
	got := jsonSafe(time.Unix(100, 0))
	if v, ok := got.(int64); !ok || v != 100 {
		t.Errorf("jsonSafe(time.Unix(100,0)) = %v (%T), want int64(100)", got, got)
	}
	got = jsonSafe(time.Time{})
	if v, ok := got.(int); !ok || v != 0 {
		t.Errorf("jsonSafe(time.Time{}) = %v (%T), want 0", got, got)
	}
}

func TestJsonSafe_NilList(t *testing.T) {
	got := jsonSafe([]string(nil))
	if sl, ok := got.([]string); !ok || sl == nil {
		t.Errorf("jsonSafe(nil []string) should return non-nil empty slice, got %v", got)
	}
}

func TestErrorResponse_Plain(t *testing.T) {
	q := &Query{}
	got := errorResponse(q, 404, "msg")
	if got != "msg\n" {
		t.Errorf("got %q, want %q", got, "msg\n")
	}
}

func TestErrorResponse_Fixed16(t *testing.T) {
	q := &Query{ResponseHeader: "fixed16"}
	got := errorResponse(q, 404, "msg")
	if !strings.HasPrefix(got, "404") {
		t.Errorf("expected response to start with 404, got %q", got)
	}
	if !strings.Contains(got, "msg\n") {
		t.Errorf("expected response to contain msg, got %q", got)
	}
}

func TestFormatCSV_Basic(t *testing.T) {
	q := &Query{OutputFormat: "csv"}
	cols := []string{"name", "state"}
	rows := [][]interface{}{
		{"web-01", 0},
		{"web-02", 1},
	}
	got := formatCSV(q, cols, rows)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), got)
	}
	if lines[0] != "web-01;0" {
		t.Errorf("line 0 = %q, want %q", lines[0], "web-01;0")
	}
}

func TestFormatCSV_WithHeaders(t *testing.T) {
	q := &Query{OutputFormat: "csv", ColumnHeaders: true}
	cols := []string{"name", "state"}
	rows := [][]interface{}{{"web-01", 0}}
	got := formatCSV(q, cols, rows)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (header + data), got %d: %q", len(lines), got)
	}
	if lines[0] != "name;state" {
		t.Errorf("header = %q, want %q", lines[0], "name;state")
	}
}

func TestFormatJSON_Basic(t *testing.T) {
	q := &Query{OutputFormat: "json"}
	cols := []string{"name"}
	rows := [][]interface{}{{"web-01"}, {"web-02"}}
	got := formatJSON(q, cols, rows)
	var parsed [][]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("expected 2 rows, got %d", len(parsed))
	}
}

func TestFormatWrappedJSON(t *testing.T) {
	q := &Query{OutputFormat: "wrapped_json"}
	cols := []string{"name"}
	rows := [][]interface{}{{"web-01"}}
	got := formatWrappedJSON(q, cols, rows)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := parsed["data"]; !ok {
		t.Error("missing 'data' key")
	}
	if _, ok := parsed["total_count"]; !ok {
		t.Error("missing 'total_count' key")
	}
}
