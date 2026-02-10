package checker

import "github.com/oceanplexian/gogios/internal/objects"

const maxStateHistoryEntries = objects.MaxStateHistoryEntries // 21

// UpdateFlapHistory records a new state in the circular buffer and recalculates
// the weighted percent state change.
// For services: skip SOFT non-OK non-recovery states.
func UpdateFlapHistory(history *[maxStateHistoryEntries]int, histIdx *int, percentChange *float64, newState int) {
	history[*histIdx] = newState
	*histIdx = (*histIdx + 1) % maxStateHistoryEntries
	*percentChange = CalculateFlapPercent(history, *histIdx)
}

// CalculateFlapPercent computes the weighted percent state change across the
// 21-entry circular buffer. Recent changes carry more weight (up to 1.25x),
// older changes carry less (down to 0.75x).
func CalculateFlapPercent(history *[maxStateHistoryEntries]int, currentIdx int) float64 {
	var curvedChanges float64

	for x := 1; x < maxStateHistoryEntries; x++ {
		thisIdx := (currentIdx + x) % maxStateHistoryEntries
		prevIdx := (currentIdx + x - 1) % maxStateHistoryEntries

		if history[thisIdx] != history[prevIdx] {
			weight := float64(x-1)*(1.25-0.75)/float64(maxStateHistoryEntries-2) + 0.75
			curvedChanges += weight
		}
	}

	return (curvedChanges * 100.0) / float64(maxStateHistoryEntries-1)
}

// CheckFlapping evaluates whether an object has started or stopped flapping
// based on the current percent state change and the high/low thresholds.
// Returns (isFlapping, stateChanged).
func CheckFlapping(currentlyFlapping bool, percentChange float64, lowThreshold, highThreshold float64) (bool, bool) {
	if lowThreshold <= 0 {
		lowThreshold = 20.0
	}
	if highThreshold <= 0 {
		highThreshold = 30.0
	}

	if !currentlyFlapping && percentChange >= highThreshold {
		return true, true // started flapping
	}
	if currentlyFlapping && percentChange < lowThreshold {
		return false, true // stopped flapping
	}
	return currentlyFlapping, false // no change
}

// ShouldRecordServiceFlapState returns true if this state should be recorded
// in the flap history for a service. SOFT non-OK non-recovery states are excluded.
func ShouldRecordServiceFlapState(newState int, stateType int, lastState int, lastHardState int) bool {
	// Skip SOFT non-OK states that aren't recoveries
	if stateType == objects.StateTypeSoft && newState != objects.ServiceOK {
		return false
	}
	return true
}
