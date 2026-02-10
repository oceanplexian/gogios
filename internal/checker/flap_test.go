package checker

import (
	"math"
	"testing"
)

func TestCalculateFlapPercent_NoChanges(t *testing.T) {
	var history [21]int
	// All same state -> 0% change
	pct := CalculateFlapPercent(&history, 0)
	if pct != 0 {
		t.Errorf("expected 0%%, got %.2f%%", pct)
	}
}

func TestCalculateFlapPercent_AllChanges(t *testing.T) {
	var history [21]int
	// Alternating states: 0,1,0,1,...
	for i := 0; i < 21; i++ {
		history[i] = i % 2
	}
	pct := CalculateFlapPercent(&history, 0)
	// Every pair is a change, so all 20 transitions are changes
	// Sum of weights for all 20 changes:
	// weight(x) = (x-1)*(1.25-0.75)/(20-1) + 0.75 for x=1..20
	var expectedCurved float64
	for x := 1; x < 21; x++ {
		weight := float64(x-1)*(1.25-0.75)/float64(21-2) + 0.75
		expectedCurved += weight
	}
	expected := (expectedCurved * 100.0) / 20.0
	if math.Abs(pct-expected) > 0.01 {
		t.Errorf("expected %.2f%%, got %.2f%%", expected, pct)
	}
}

func TestCheckFlapping_Hysteresis(t *testing.T) {
	// Not flapping, below high threshold
	isFlap, changed := CheckFlapping(false, 25.0, 20.0, 30.0)
	if isFlap || changed {
		t.Error("should not start flapping in hysteresis zone")
	}

	// Not flapping, above high threshold
	isFlap, changed = CheckFlapping(false, 35.0, 20.0, 30.0)
	if !isFlap || !changed {
		t.Error("should start flapping above high threshold")
	}

	// Flapping, above low threshold (in hysteresis)
	isFlap, changed = CheckFlapping(true, 25.0, 20.0, 30.0)
	if !isFlap || changed {
		t.Error("should stay flapping in hysteresis zone")
	}

	// Flapping, below low threshold
	isFlap, changed = CheckFlapping(true, 15.0, 20.0, 30.0)
	if isFlap || !changed {
		t.Error("should stop flapping below low threshold")
	}
}
