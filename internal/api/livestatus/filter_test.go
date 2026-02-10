package livestatus

import (
	"testing"
	"time"
)

func TestCompareString(t *testing.T) {
	tests := []struct {
		name string
		a    string
		op   string
		b    string
		want bool
	}{
		// Equality
		{"eq match", "hello", "=", "hello", true},
		{"eq no match", "hello", "=", "world", false},
		// Not equal
		{"neq match", "hello", "!=", "world", true},
		{"neq no match", "hello", "!=", "hello", false},
		// Less than
		{"lt true", "abc", "<", "def", true},
		{"lt false", "def", "<", "abc", false},
		{"lt equal", "abc", "<", "abc", false},
		// Greater than
		{"gt true", "def", ">", "abc", true},
		{"gt false", "abc", ">", "def", false},
		{"gt equal", "abc", ">", "abc", false},
		// Less or equal
		{"le less", "abc", "<=", "def", true},
		{"le equal", "abc", "<=", "abc", true},
		{"le greater", "def", "<=", "abc", false},
		// Greater or equal
		{"ge greater", "def", ">=", "abc", true},
		{"ge equal", "abc", ">=", "abc", true},
		{"ge less", "abc", ">=", "def", false},
		// Regex match
		{"regex match", "web-01", "~", "^web", true},
		{"regex no match", "app-01", "~", "^web", false},
		// Regex not match
		{"regex not match true", "app-01", "!~", "^web", true},
		{"regex not match false", "web-01", "!~", "^web", false},
		// Case-insensitive equal
		{"ci eq match", "Hello", "=~", "hello", true},
		{"ci eq no match", "Hello", "=~", "world", false},
		// Case-insensitive not equal
		{"ci neq match", "Hello", "!=~", "world", true},
		{"ci neq no match", "Hello", "!=~", "hello", false},
		// Case-insensitive contains
		{"ci contains match", "Hello World", "~~", "world", true},
		{"ci contains no match", "Hello World", "~~", "foo", false},
		// Case-insensitive not contains
		{"ci not contains true", "Hello World", "!~~", "foo", true},
		{"ci not contains false", "Hello World", "!~~", "world", false},
		// Unknown op
		{"unknown op", "a", "??", "b", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareString(tt.a, tt.op, tt.b)
			if got != tt.want {
				t.Errorf("compareString(%q, %q, %q) = %v, want %v", tt.a, tt.op, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareInt(t *testing.T) {
	tests := []struct {
		name string
		a    int
		op   string
		b    int
		want bool
	}{
		{"eq true", 42, "=", 42, true},
		{"eq false", 42, "=", 43, false},
		{"neq true", 42, "!=", 43, true},
		{"neq false", 42, "!=", 42, false},
		{"lt true", 1, "<", 2, true},
		{"lt false equal", 2, "<", 2, false},
		{"lt false greater", 3, "<", 2, false},
		{"gt true", 3, ">", 2, true},
		{"gt false equal", 2, ">", 2, false},
		{"gt false less", 1, ">", 2, false},
		{"le less", 1, "<=", 2, true},
		{"le equal", 2, "<=", 2, true},
		{"le greater", 3, "<=", 2, false},
		{"ge greater", 3, ">=", 2, true},
		{"ge equal", 2, ">=", 2, true},
		{"ge less", 1, ">=", 2, false},
		{"unknown op", 1, "??", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareInt(tt.a, tt.op, tt.b)
			if got != tt.want {
				t.Errorf("compareInt(%d, %q, %d) = %v, want %v", tt.a, tt.op, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareInt64(t *testing.T) {
	tests := []struct {
		name string
		a    int64
		op   string
		b    int64
		want bool
	}{
		{"eq true", 42, "=", 42, true},
		{"eq false", 42, "=", 43, false},
		{"neq true", 42, "!=", 43, true},
		{"neq false", 42, "!=", 42, false},
		{"lt true", 1, "<", 2, true},
		{"lt false equal", 2, "<", 2, false},
		{"lt false greater", 3, "<", 2, false},
		{"gt true", 3, ">", 2, true},
		{"gt false equal", 2, ">", 2, false},
		{"gt false less", 1, ">", 2, false},
		{"le less", 1, "<=", 2, true},
		{"le equal", 2, "<=", 2, true},
		{"le greater", 3, "<=", 2, false},
		{"ge greater", 3, ">=", 2, true},
		{"ge equal", 2, ">=", 2, true},
		{"ge less", 1, ">=", 2, false},
		{"unknown op", 1, "??", 2, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareInt64(tt.a, tt.op, tt.b)
			if got != tt.want {
				t.Errorf("compareInt64(%d, %q, %d) = %v, want %v", tt.a, tt.op, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareFloat(t *testing.T) {
	tests := []struct {
		name string
		a    float64
		op   string
		b    float64
		want bool
	}{
		{"eq true", 1.5, "=", 1.5, true},
		{"eq false", 1.5, "=", 2.5, false},
		{"neq true", 1.5, "!=", 2.5, true},
		{"neq false", 1.5, "!=", 1.5, false},
		{"lt true", 1.5, "<", 2.5, true},
		{"lt false equal", 2.5, "<", 2.5, false},
		{"lt false greater", 3.5, "<", 2.5, false},
		{"gt true", 3.5, ">", 2.5, true},
		{"gt false equal", 2.5, ">", 2.5, false},
		{"gt false less", 1.5, ">", 2.5, false},
		{"le less", 1.5, "<=", 2.5, true},
		{"le equal", 2.5, "<=", 2.5, true},
		{"le greater", 3.5, "<=", 2.5, false},
		{"ge greater", 3.5, ">=", 2.5, true},
		{"ge equal", 2.5, ">=", 2.5, true},
		{"ge less", 1.5, ">=", 2.5, false},
		{"unknown op", 1.5, "??", 2.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareFloat(tt.a, tt.op, tt.b)
			if got != tt.want {
				t.Errorf("compareFloat(%g, %q, %g) = %v, want %v", tt.a, tt.op, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareValue_String(t *testing.T) {
	got := compareValue("hello", "=", "hello")
	if !got {
		t.Error("compareValue(\"hello\", \"=\", \"hello\") should be true")
	}
}

func TestCompareValue_Int(t *testing.T) {
	if !compareValue(42, "=", "42") {
		t.Error("compareValue(42, \"=\", \"42\") should be true")
	}
	if !compareValue(42, ">", "10") {
		t.Error("compareValue(42, \">\", \"10\") should be true")
	}
}

func TestCompareValue_Int64(t *testing.T) {
	if !compareValue(int64(1000000), ">", "999999") {
		t.Error("compareValue(int64(1000000), \">\", \"999999\") should be true")
	}
}

func TestCompareValue_Float64(t *testing.T) {
	if !compareValue(3.14, "<", "4.0") {
		t.Error("compareValue(3.14, \"<\", \"4.0\") should be true")
	}
}

func TestCompareValue_Bool(t *testing.T) {
	if !compareValue(true, "=", "1") {
		t.Error("compareValue(true, \"=\", \"1\") should be true")
	}
	if !compareValue(false, "=", "0") {
		t.Error("compareValue(false, \"=\", \"0\") should be true")
	}
}

func TestCompareValue_Time(t *testing.T) {
	ts := time.Unix(1700000000, 0)
	if !compareValue(ts, ">", "1699999999") {
		t.Error("compareValue(time.Unix(1700000000, 0), \">\", \"1699999999\") should be true")
	}
	// Zero time should map to unix 0
	zero := time.Time{}
	if !compareValue(zero, "=", "0") {
		t.Error("compareValue(time.Time{}, \"=\", \"0\") should be true")
	}
}

func TestCompareValue_UnknownType(t *testing.T) {
	// struct{}{} formats as "{}" via fmt.Sprintf("%v", ...)
	got := compareValue(struct{}{}, "=", "{}")
	if !got {
		t.Error("compareValue(struct{}{}, \"=\", \"{}\") should be true via string fallback")
	}
}

func TestCompareList_Contains(t *testing.T) {
	if !compareList([]string{"a", "b", "c"}, ">=", "b") {
		t.Error("list [a,b,c] should contain b")
	}
}

func TestCompareList_NotContains(t *testing.T) {
	if compareList([]string{"a", "b"}, ">=", "x") {
		t.Error("list [a,b] should not contain x")
	}
}

func TestCompareList_NotContainsOp(t *testing.T) {
	if !compareList([]string{"a", "b"}, "!>=", "x") {
		t.Error("list [a,b] !>= x should be true")
	}
}

func TestCompareList_EmptyEq(t *testing.T) {
	if !compareList([]string{}, "=", "") {
		t.Error("empty list = '' should be true")
	}
	if compareList([]string{"a"}, "=", "") {
		t.Error("non-empty list = '' should be false")
	}
}

func TestCompareList_NotEmptyNeq(t *testing.T) {
	if !compareList([]string{"a"}, "!=", "") {
		t.Error("non-empty list != '' should be true")
	}
	if compareList([]string{}, "!=", "") {
		t.Error("empty list != '' should be false")
	}
}

func TestCompareList_UnknownOp(t *testing.T) {
	if compareList([]string{"a"}, "??", "a") {
		t.Error("unknown op should return false")
	}
}
