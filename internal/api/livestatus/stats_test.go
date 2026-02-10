package livestatus

import (
	"testing"
	"time"
)

func TestToFloat64_Int(t *testing.T) {
	if got := toFloat64(42); got != 42.0 {
		t.Errorf("toFloat64(42) = %f, want 42.0", got)
	}
}

func TestToFloat64_Int64(t *testing.T) {
	if got := toFloat64(int64(100)); got != 100.0 {
		t.Errorf("toFloat64(int64(100)) = %f, want 100.0", got)
	}
}

func TestToFloat64_Float64(t *testing.T) {
	if got := toFloat64(3.14); got != 3.14 {
		t.Errorf("toFloat64(3.14) = %f, want 3.14", got)
	}
}

func TestToFloat64_Bool(t *testing.T) {
	if got := toFloat64(true); got != 1.0 {
		t.Errorf("toFloat64(true) = %f, want 1.0", got)
	}
	if got := toFloat64(false); got != 0.0 {
		t.Errorf("toFloat64(false) = %f, want 0.0", got)
	}
}

func TestToFloat64_Time(t *testing.T) {
	if got := toFloat64(time.Unix(1000, 0)); got != 1000.0 {
		t.Errorf("toFloat64(time.Unix(1000,0)) = %f, want 1000.0", got)
	}
	if got := toFloat64(time.Time{}); got != 0.0 {
		t.Errorf("toFloat64(time.Time{}) = %f, want 0.0", got)
	}
}

func TestToFloat64_Unknown(t *testing.T) {
	if got := toFloat64("hello"); got != 0.0 {
		t.Errorf("toFloat64(\"hello\") = %f, want 0.0", got)
	}
}

func TestJoinKey_Single(t *testing.T) {
	if got := joinKey([]string{"a"}); got != "a" {
		t.Errorf("joinKey([a]) = %q, want %q", got, "a")
	}
}

func TestJoinKey_Multiple(t *testing.T) {
	if got := joinKey([]string{"a", "b", "c"}); got != "a\x00b\x00c" {
		t.Errorf("joinKey([a,b,c]) = %q, want %q", got, "a\x00b\x00c")
	}
}

func TestJoinKey_Empty(t *testing.T) {
	if got := joinKey([]string{}); got != "" {
		t.Errorf("joinKey([]) = %q, want %q", got, "")
	}
}
