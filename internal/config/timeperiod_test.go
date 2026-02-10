package config

import (
	"testing"
	"time"

	"github.com/oceanplexian/gogios/internal/objects"
)

func TestParseTimeRanges(t *testing.T) {
	ranges, err := ParseTimeRanges("09:00-17:00,19:00-23:00")
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 2 {
		t.Fatalf("expected 2 ranges, got %d", len(ranges))
	}
	if ranges[0].StartHour != 9 || ranges[0].EndHour != 17 {
		t.Errorf("first range: expected 09:00-17:00, got %02d:%02d-%02d:%02d",
			ranges[0].StartHour, ranges[0].StartMin, ranges[0].EndHour, ranges[0].EndMin)
	}
}

func TestCheckTime24x7(t *testing.T) {
	tp := &objects.Timeperiod{
		Name: "24x7",
	}
	for i := range tp.Ranges {
		tp.Ranges[i] = "00:00-24:00"
	}

	// Any time should be valid
	now := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC) // Saturday
	if !CheckTime(tp, now) {
		t.Error("expected 24x7 to be valid at any time")
	}
	sunday := time.Date(2024, 6, 16, 3, 0, 0, 0, time.UTC)
	if !CheckTime(tp, sunday) {
		t.Error("expected 24x7 to be valid on Sunday 3am")
	}
}

func TestCheckTimeWorkHours(t *testing.T) {
	tp := &objects.Timeperiod{Name: "workhours"}
	tp.Ranges[1] = "09:00-17:00" // monday
	tp.Ranges[2] = "09:00-17:00" // tuesday
	tp.Ranges[3] = "09:00-17:00" // wednesday
	tp.Ranges[4] = "09:00-17:00" // thursday
	tp.Ranges[5] = "09:00-17:00" // friday

	// Monday 10am - should be valid
	mon := time.Date(2024, 6, 17, 10, 0, 0, 0, time.UTC) // Monday
	if !CheckTime(tp, mon) {
		t.Error("expected work hours to be valid Monday 10am")
	}
	// Monday 8am - before work hours
	early := time.Date(2024, 6, 17, 8, 0, 0, 0, time.UTC)
	if CheckTime(tp, early) {
		t.Error("expected work hours to be invalid Monday 8am")
	}
	// Saturday - should be invalid
	sat := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if CheckTime(tp, sat) {
		t.Error("expected work hours to be invalid on Saturday")
	}
}

func TestCheckTimeWithExclusion(t *testing.T) {
	excluded := &objects.Timeperiod{Name: "excluded"}
	for i := range excluded.Ranges {
		excluded.Ranges[i] = "00:00-24:00"
	}

	tp := &objects.Timeperiod{Name: "main"}
	for i := range tp.Ranges {
		tp.Ranges[i] = "00:00-24:00"
	}
	tp.Exclusions = []*objects.Timeperiod{excluded}

	// Everything is excluded
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if CheckTime(tp, now) {
		t.Error("expected time to be excluded")
	}
}

func TestCheckTimeNever(t *testing.T) {
	tp := &objects.Timeperiod{Name: "never"}
	// No ranges set
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if CheckTime(tp, now) {
		t.Error("expected 'never' timeperiod to always be invalid")
	}
}

func TestCheckTimeNilTimeperiod(t *testing.T) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	if !CheckTime(nil, now) {
		t.Error("nil timeperiod should always return true")
	}
}

func TestGetNextValidTime(t *testing.T) {
	tp := &objects.Timeperiod{Name: "workhours"}
	tp.Ranges[1] = "09:00-17:00" // monday

	// Saturday at noon - next valid should be Monday 9am
	sat := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	next := GetNextValidTime(tp, sat)
	if next.Weekday() != time.Monday {
		t.Errorf("expected next valid on Monday, got %s", next.Weekday())
	}
	if next.Hour() != 9 || next.Minute() != 0 {
		t.Errorf("expected 09:00, got %02d:%02d", next.Hour(), next.Minute())
	}
}

func TestCheckTimeMultipleRanges(t *testing.T) {
	tp := &objects.Timeperiod{Name: "nonwork"}
	tp.Ranges[1] = "00:00-09:00,17:00-24:00" // monday

	mon8am := time.Date(2024, 6, 17, 8, 0, 0, 0, time.UTC) // Monday
	if !CheckTime(tp, mon8am) {
		t.Error("expected 8am Monday to be valid in nonwork (00:00-09:00)")
	}
	mon12 := time.Date(2024, 6, 17, 12, 0, 0, 0, time.UTC)
	if CheckTime(tp, mon12) {
		t.Error("expected 12pm Monday to be invalid in nonwork")
	}
	mon18 := time.Date(2024, 6, 17, 18, 0, 0, 0, time.UTC)
	if !CheckTime(tp, mon18) {
		t.Error("expected 6pm Monday to be valid in nonwork (17:00-24:00)")
	}
}
