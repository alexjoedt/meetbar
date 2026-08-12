package calendar

import (
	"testing"
	"time"

	"github.com/alex/meetbar/internal/ipc"
)

func TestUpcomingExcludesAllDay(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	s := NewSyncer(nil, 24*time.Hour, Filter{})
	s.events = []ipc.Event{
		{
			ID:         "allday-ongoing",
			Title:      "Zuhause",
			Start:      dayStart.Format(time.RFC3339),
			End:        dayStart.Add(24 * time.Hour).Format(time.RFC3339),
			CalendarID: "primary",
			AllDay:     true,
		},
		{
			ID:         "allday-future",
			Title:      "Holiday",
			Start:      dayStart.Add(24 * time.Hour).Format(time.RFC3339),
			End:        dayStart.Add(48 * time.Hour).Format(time.RFC3339),
			CalendarID: "primary",
			AllDay:     true,
		},
		{
			ID:         "timed",
			Title:      "Standup",
			Start:      now.Add(30 * time.Minute).Format(time.RFC3339),
			End:        now.Add(60 * time.Minute).Format(time.RFC3339),
			CalendarID: "primary",
		},
	}

	got := s.Upcoming(48)
	if len(got) != 1 {
		t.Fatalf("expected 1 timed event, got %#v", got)
	}
	if got[0].ID != "timed" {
		t.Fatalf("expected timed meeting, got %#v", got[0])
	}
	if got[0].MinutesUntil == nil {
		t.Fatal("expected minutes_until on timed event")
	}
}

func TestTodayAt(t *testing.T) {
	now := time.Date(2026, 8, 12, 13, 0, 0, 0, time.Local)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	events := []ipc.Event{
		{
			ID:         "finished",
			Title:      "Standup",
			Start:      dayStart.Add(9 * time.Hour).Format(time.RFC3339),
			End:        dayStart.Add(9*time.Hour + 15*time.Minute).Format(time.RFC3339),
			CalendarID: "primary",
		},
		{
			ID:         "running",
			Title:      "Lunch",
			Start:      now.Add(-30 * time.Minute).Format(time.RFC3339),
			End:        now.Add(30 * time.Minute).Format(time.RFC3339),
			CalendarID: "primary",
		},
		{
			ID:         "later",
			Title:      "Refinement",
			Start:      dayStart.Add(14 * time.Hour).Format(time.RFC3339),
			End:        dayStart.Add(15 * time.Hour).Format(time.RFC3339),
			CalendarID: "primary",
		},
		{
			ID:         "allday-today",
			Title:      "WFH",
			Start:      dayStart.Format(time.RFC3339),
			End:        dayStart.Add(24 * time.Hour).Format(time.RFC3339),
			CalendarID: "primary",
			AllDay:     true,
		},
		{
			ID:         "yesterday",
			Title:      "Retro",
			Start:      dayStart.Add(-2 * time.Hour).Format(time.RFC3339),
			End:        dayStart.Add(-1 * time.Hour).Format(time.RFC3339),
			CalendarID: "primary",
		},
		{
			ID:         "tomorrow",
			Title:      "Planning",
			Start:      dayStart.Add(25 * time.Hour).Format(time.RFC3339),
			End:        dayStart.Add(26 * time.Hour).Format(time.RFC3339),
			CalendarID: "primary",
		},
	}

	s := NewSyncer(nil, 24*time.Hour, Filter{})
	s.events = events

	got := s.todayAt(now)

	wantIDs := []string{"finished", "running", "later"}
	if len(got) != len(wantIDs) {
		t.Fatalf("expected %d events, got %#v", len(wantIDs), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Fatalf("event %d = %q, want %q (order: %#v)", i, got[i].ID, id, got)
		}
	}

	for _, ev := range got {
		if ev.MinutesUntil == nil {
			t.Fatalf("expected minutes_until on %q", ev.ID)
		}
	}
	if *got[0].MinutesUntil >= 0 {
		t.Fatalf("expected negative minutes_until for finished event, got %d", *got[0].MinutesUntil)
	}
}
