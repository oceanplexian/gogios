package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

// TimeRange represents a single HH:MM-HH:MM range.
type TimeRange struct {
	StartHour, StartMin int
	EndHour, EndMin     int
}

// ParseTimeRanges parses "HH:MM-HH:MM,HH:MM-HH:MM,..." into a slice of TimeRange.
func ParseTimeRanges(s string) ([]TimeRange, error) {
	if s == "" {
		return nil, nil
	}
	var ranges []TimeRange
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tr, err := parseOneRange(part)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, tr)
	}
	return ranges, nil
}

func parseOneRange(s string) (TimeRange, error) {
	parts := strings.SplitN(s, "-", 2)
	if len(parts) != 2 {
		return TimeRange{}, fmt.Errorf("invalid time range: %s", s)
	}
	start, err := parseHHMM(parts[0])
	if err != nil {
		return TimeRange{}, err
	}
	end, err := parseHHMM(parts[1])
	if err != nil {
		return TimeRange{}, err
	}
	return TimeRange{StartHour: start[0], StartMin: start[1], EndHour: end[0], EndMin: end[1]}, nil
}

func parseHHMM(s string) ([2]int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return [2]int{}, fmt.Errorf("invalid time: %s", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return [2]int{}, fmt.Errorf("invalid hour: %s", parts[0])
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return [2]int{}, fmt.Errorf("invalid minute: %s", parts[1])
	}
	return [2]int{h, m}, nil
}

// CheckTime returns true if the given time falls within the timeperiod.
func CheckTime(tp *objects.Timeperiod, t time.Time) bool {
	if tp == nil {
		return true
	}
	// Check exclusions first
	for _, exc := range tp.Exclusions {
		if CheckTime(exc, t) {
			return false
		}
	}
	// Check date exceptions (higher priority than weekday ranges)
	for _, exc := range tp.Exceptions {
		if matchException(exc, t) {
			return true
		}
	}
	// Check weekday ranges
	dow := int(t.Weekday()) // Sunday = 0
	rangeStr := tp.Ranges[dow]
	if rangeStr == "" {
		return false
	}
	ranges, err := ParseTimeRanges(rangeStr)
	if err != nil {
		return false
	}
	return timeInRanges(t, ranges)
}

// GetNextValidTime returns the next time >= t that is valid in the timeperiod.
func GetNextValidTime(tp *objects.Timeperiod, t time.Time) time.Time {
	if tp == nil {
		return t
	}
	if CheckTime(tp, t) {
		return t
	}
	// Search forward in 1-minute increments, up to 366 days
	maxSearch := t.Add(366 * 24 * time.Hour)
	candidate := t.Truncate(time.Minute).Add(time.Minute)
	for candidate.Before(maxSearch) {
		if CheckTime(tp, candidate) {
			return candidate
		}
		candidate = candidate.Add(time.Minute)
	}
	return t // fallback
}

func timeInRanges(t time.Time, ranges []TimeRange) bool {
	minutes := t.Hour()*60 + t.Minute()
	for _, r := range ranges {
		start := r.StartHour*60 + r.StartMin
		end := r.EndHour*60 + r.EndMin
		if minutes >= start && minutes < end {
			return true
		}
	}
	return false
}

// matchException checks if time t matches a date exception.
func matchException(exc objects.TimeDateException, t time.Time) bool {
	// Parse the raw Timerange string which contains the full directive
	// Format examples:
	//   "january 1 00:00-24:00"
	//   "july 4 00:00-24:00"
	//   "december 25 00:00-24:00"
	//   "monday -1 may 00:00-24:00"
	//   "monday 1 september 00:00-24:00"
	//   "thursday 4 november 00:00-24:00"
	//   "2008-12-25 00:00-24:00"
	//   "day 21 00:00-24:00"
	raw := exc.Timerange
	if raw == "" {
		return false
	}

	// Try to parse different formats
	parts := strings.Fields(raw)
	if len(parts) < 2 {
		return false
	}

	// Calendar date: "2008-12-25 HH:MM-HH:MM"
	if len(parts) >= 2 && strings.Contains(parts[0], "-") && len(parts[0]) >= 8 {
		dateStr := parts[0]
		dateParts := strings.SplitN(dateStr, "-", 3)
		if len(dateParts) == 3 {
			yr, _ := strconv.Atoi(dateParts[0])
			mo, _ := strconv.Atoi(dateParts[1])
			dy, _ := strconv.Atoi(dateParts[2])
			if t.Year() == yr && int(t.Month()) == mo && t.Day() == dy {
				if len(parts) > 1 {
					ranges, _ := ParseTimeRanges(parts[len(parts)-1])
					return timeInRanges(t, ranges)
				}
				return true
			}
		}
		return false
	}

	// Month date: "month day HH:MM-HH:MM" e.g. "january 1 00:00-24:00"
	if mo := parseMonth(parts[0]); mo > 0 && len(parts) >= 3 {
		day, err := strconv.Atoi(parts[1])
		if err == nil && int(t.Month()) == mo && t.Day() == day {
			ranges, _ := ParseTimeRanges(parts[2])
			return timeInRanges(t, ranges)
		}
	}

	// Weekday of month: "weekday N month HH:MM-HH:MM" e.g. "monday 1 september 00:00-24:00"
	if wd := parseWeekday(parts[0]); wd >= 0 && len(parts) >= 4 {
		n, err := strconv.Atoi(parts[1])
		if err == nil {
			if mo := parseMonth(parts[2]); mo > 0 {
				if matchWeekdayOfMonth(t, wd, n, mo) {
					ranges, _ := ParseTimeRanges(parts[3])
					return timeInRanges(t, ranges)
				}
			}
		}
	}

	// Day of month: "day N HH:MM-HH:MM"
	if parts[0] == "day" && len(parts) >= 3 {
		day, err := strconv.Atoi(parts[1])
		if err == nil && t.Day() == day {
			ranges, _ := ParseTimeRanges(parts[2])
			return timeInRanges(t, ranges)
		}
	}

	return false
}

func matchWeekdayOfMonth(t time.Time, weekday, n, month int) bool {
	if int(t.Month()) != month {
		return false
	}
	if int(t.Weekday()) != weekday {
		return false
	}
	if n > 0 {
		// Nth weekday of month (1-based)
		return (t.Day()-1)/7+1 == n
	}
	// Negative: -1 = last, -2 = second to last
	// Find the last occurrence
	daysInMonth := daysIn(t.Month(), t.Year())
	weekNum := (daysInMonth - t.Day()) / 7
	return weekNum == (-n - 1)
}

func daysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

var monthNames = map[string]int{
	"january": 1, "february": 2, "march": 3, "april": 4,
	"may": 5, "june": 6, "july": 7, "august": 8,
	"september": 9, "october": 10, "november": 11, "december": 12,
}

func parseMonth(s string) int {
	return monthNames[strings.ToLower(s)]
}

var weekdayNames = map[string]int{
	"sunday": 0, "monday": 1, "tuesday": 2, "wednesday": 3,
	"thursday": 4, "friday": 5, "saturday": 6,
}

func parseWeekday(s string) int {
	v, ok := weekdayNames[strings.ToLower(s)]
	if !ok {
		return -1
	}
	return v
}
