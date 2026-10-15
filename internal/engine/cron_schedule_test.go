package engine

import (
	"testing"
	"time"
)

func TestCronSchedule(t *testing.T) {
	sched, err := ParseCronSchedule("0 0 * * *")
	if err != nil {
		t.Fatalf("unexpected error parsing cron: %v", err)
	}

	// 2026-10-15 00:00:00 should match
	matchTime := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if !sched.Matches(matchTime) {
		t.Errorf("expected %v to match '0 0 * * *'", matchTime)
	}

	// 2026-10-15 00:01:00 should not match
	nonMatchTime := time.Date(2026, 10, 15, 0, 1, 0, 0, time.UTC)
	if sched.Matches(nonMatchTime) {
		t.Errorf("expected %v to NOT match '0 0 * * *'", nonMatchTime)
	}

	// Next after 2026-10-15 05:30:00 should be 2026-10-16 00:00:00
	from := time.Date(2026, 10, 15, 5, 30, 0, 0, time.UTC)
	next := sched.Next(from)
	expectedNext := time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)
	if !next.Equal(expectedNext) {
		t.Errorf("expected next to be %v, got %v", expectedNext, next)
	}
}

func TestCronScheduleEveryFiveMinutes(t *testing.T) {
	sched, err := ParseCronSchedule("*/5 * * * *")
	if err != nil {
		t.Fatalf("unexpected error parsing cron: %v", err)
	}

	from := time.Date(2026, 10, 15, 5, 32, 10, 0, time.UTC)
	next := sched.Next(from)
	expectedNext := time.Date(2026, 10, 15, 5, 35, 0, 0, time.UTC)
	if !next.Equal(expectedNext) {
		t.Errorf("expected next to be %v, got %v", expectedNext, next)
	}
}
