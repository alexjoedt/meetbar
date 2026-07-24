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
