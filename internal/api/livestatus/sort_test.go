package livestatus

import (
	"testing"
	"time"
)

func TestCompareValues_Ints(t *testing.T) {
	if got := compareValues(1, 2); got != -1 {
		t.Errorf("compareValues(1, 2) = %d, want -1", got)
	}
	if got := compareValues(2, 1); got != 1 {
		t.Errorf("compareValues(2, 1) = %d, want 1", got)
	}
	if got := compareValues(1, 1); got != 0 {
		t.Errorf("compareValues(1, 1) = %d, want 0", got)
	}
}

func TestCompareValues_Int64(t *testing.T) {
	if got := compareValues(int64(1), int64(2)); got != -1 {
		t.Errorf("compareValues(int64(1), int64(2)) = %d, want -1", got)
	}
	if got := compareValues(int64(2), int64(1)); got != 1 {
		t.Errorf("compareValues(int64(2), int64(1)) = %d, want 1", got)
	}
	if got := compareValues(int64(1), int64(1)); got != 0 {
		t.Errorf("compareValues(int64(1), int64(1)) = %d, want 0", got)
	}
}

func TestCompareValues_Floats(t *testing.T) {
	if got := compareValues(1.5, 2.5); got != -1 {
		t.Errorf("compareValues(1.5, 2.5) = %d, want -1", got)
	}
}

func TestCompareValues_Strings(t *testing.T) {
	if got := compareValues("apple", "banana"); got != -1 {
		t.Errorf("compareValues(apple, banana) = %d, want -1", got)
	}
}

func TestCompareValues_Times(t *testing.T) {
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(2000, 0)
	if got := compareValues(t1, t2); got != -1 {
		t.Errorf("compareValues(t1, t2) = %d, want -1", got)
	}
}

func TestCompareValues_MismatchedTypes(t *testing.T) {
	if got := compareValues(int(1), string("1")); got != 0 {
		t.Errorf("compareValues(int, string) = %d, want 0", got)
	}
}

func TestCompareValues_UnknownType(t *testing.T) {
	// Structs fall through to string comparison via fmt.Sprintf
	type dummy struct{ X int }
	a := dummy{X: 1}
	b := dummy{X: 2}
	got := compareValues(a, b)
	// Just verify it returns an int without panicking
	if got != -1 && got != 0 && got != 1 {
		t.Errorf("compareValues(unknown) = %d, want -1, 0, or 1", got)
	}
}
