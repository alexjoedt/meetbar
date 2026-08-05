package alert

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alex/meetbar/internal/ipc"
)

func TestEvaluateAndAck(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "alerts.json"), []int{15, 5, 0})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now().Add(10 * time.Minute).Truncate(time.Second)
	ev := ipc.Event{
		ID:    "evt1",
		Title: "Standup",
		Start: start.Format(time.RFC3339),
		End:   start.Add(30 * time.Minute).Format(time.RFC3339),
	}

	// 10 minutes before start: 15m threshold is due, 5m and 0 not yet
	now := start.Add(-10 * time.Minute)
	due := store.Evaluate(now, []ipc.Event{ev})
	if len(due) != 1 || due[0].Key != "evt1:15" {
		t.Fatalf("expected evt1:15, got %#v", due)
	}

	if err := store.Ack([]string{"evt1:15"}); err != nil {
		t.Fatal(err)
	}
	due = store.Evaluate(now, []ipc.Event{ev})
	if len(due) != 0 {
		t.Fatalf("expected no due after ack, got %#v", due)
	}

	// At start, both the 5m and 0m thresholds have technically elapsed, but
	// only the most urgent (0) should surface as a notification; the 5m
	// threshold is acked silently so it doesn't also fire.
	due = store.Evaluate(start, []ipc.Event{ev})
	if len(due) != 1 || due[0].Key != "evt1:0" {
		t.Fatalf("expected only evt1:0 at start, got %#v", due)
	}
}

func TestEvaluateCollapsesStackedThresholds(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "alerts.json"), []int{15, 5, 0})
	if err != nil {
		t.Fatal(err)
	}

	// A meeting synced only 2 minutes before it starts: the 15m and 5m
	// thresholds are both already overdue the first time it's evaluated.
	start := time.Now().Add(2 * time.Minute).Truncate(time.Second)
	ev := ipc.Event{
		ID:    "evt1",
		Title: "Standup",
		Start: start.Format(time.RFC3339),
		End:   start.Add(30 * time.Minute).Format(time.RFC3339),
	}

	due := store.Evaluate(time.Now(), []ipc.Event{ev})
	if len(due) != 1 || due[0].Key != "evt1:5" {
		t.Fatalf("expected only the most urgent overdue threshold (evt1:5), got %#v", due)
	}

	// The skipped 15m threshold must be acked (not just dropped) so it
	// never fires later, and it should be persisted without an explicit
	// Ack call so a daemon restart doesn't re-surface it.
	store2, err := NewStore(filepath.Join(dir, "alerts.json"), []int{15, 5, 0})
	if err != nil {
		t.Fatal(err)
	}
	due = store2.Evaluate(time.Now(), []ipc.Event{ev})
	if len(due) != 1 || due[0].Key != "evt1:5" {
		t.Fatalf("expected evt1:15 to already be acked from disk (only evt1:5 due), got %#v", due)
	}
}

func TestEvaluateSkipsAllDay(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "alerts.json"), []int{15, 5, 0})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now().Truncate(time.Second)
	allDay := ipc.Event{
		ID:     "allday1",
		Title:  "Zuhause",
		Start:  start.Format(time.RFC3339),
		End:    start.Add(24 * time.Hour).Format(time.RFC3339),
		AllDay: true,
	}
	timed := ipc.Event{
		ID:    "evt1",
		Title: "Standup",
		Start: start.Add(10 * time.Minute).Format(time.RFC3339),
		End:   start.Add(40 * time.Minute).Format(time.RFC3339),
	}

	due := store.Evaluate(start, []ipc.Event{allDay, timed})
	if len(due) != 1 || due[0].Key != "evt1:15" {
		t.Fatalf("expected only timed evt1:15, got %#v", due)
	}
}

func TestEvaluateCrossEventSuppression(t *testing.T) {
	base := time.Date(2024, 1, 15, 9, 0, 0, 0, time.UTC)

	mkEvent := func(id, title string, start time.Time) ipc.Event {
		return ipc.Event{
			ID:    id,
			Title: title,
			Start: start.Format(time.RFC3339),
			End:   start.Add(30 * time.Minute).Format(time.RFC3339),
		}
	}

	tests := []struct {
		name     string
		events   []ipc.Event
		now      time.Time
		wantKeys []string
	}{
		{
			name: "starting now suppresses upcoming reminder",
			events: []ipc.Event{
				mkEvent("a", "Standup", base),         // starts at 9:00
				mkEvent("b", "Retro", base.Add(15*time.Minute)), // starts at 9:15
			},
			now:      base,
			wantKeys: []string{"a:0"},
		},
		{
			name: "no conflict: only upcoming reminder fires",
			events: []ipc.Event{
				mkEvent("b", "Retro", base.Add(15*time.Minute)),
			},
			now:      base,
			wantKeys: []string{"b:15"},
		},
		{
			name: "two simultaneous starts both fire",
			events: []ipc.Event{
				mkEvent("a", "Standup", base),
				mkEvent("b", "Retro", base),
			},
			now:      base,
			wantKeys: []string{"a:0", "b:0"},
		},
		{
			name: "a ended, upcoming reminder fires normally",
			events: []ipc.Event{
				// a started at 8:50 and ends at 9:20, but its :0 is already acked
				mkEvent("b", "Retro", base.Add(5*time.Minute)),
			},
			now:      base,
			wantKeys: []string{"b:5"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			store, err := NewStore(filepath.Join(dir, "alerts.json"), []int{15, 5, 1, 0})
			if err != nil {
				t.Fatal(err)
			}
			due := store.Evaluate(tc.now, tc.events)
			if len(due) != len(tc.wantKeys) {
				t.Fatalf("want %d alerts %v, got %d: %#v", len(tc.wantKeys), tc.wantKeys, len(due), due)
			}
			gotKeys := map[string]bool{}
			for _, a := range due {
				gotKeys[a.Key] = true
			}
			for _, k := range tc.wantKeys {
				if !gotKeys[k] {
					t.Errorf("want key %q in due, got %v", k, due)
				}
			}
		})
	}
}

func TestAckPruningForInactiveEvents(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "alerts.json"), []int{0})
	if err != nil {
		t.Fatal(err)
	}

	start := time.Now().Truncate(time.Second)
	ev := ipc.Event{
		ID:    "evt1",
		Title: "Standup",
		Start: start.Format(time.RFC3339),
		End:   start.Add(30 * time.Minute).Format(time.RFC3339),
	}

	due := store.Evaluate(start, []ipc.Event{ev})
	if len(due) != 1 || due[0].Key != "evt1:0" {
		t.Fatalf("expected evt1:0 due, got %#v", due)
	}
	if err := store.Ack([]string{due[0].Key}); err != nil {
		t.Fatal(err)
	}

	// Event no longer present in the synced set (ended and dropped from
	// cache) - its ack should be pruned so a same-ID event reappearing later
	// (e.g. a new instance of a recurring meeting) fires again.
	store.Evaluate(start, nil)

	due = store.Evaluate(start, []ipc.Event{ev})
	if len(due) != 1 || due[0].Key != "evt1:0" {
		t.Fatalf("expected ack to be pruned and alert to re-fire, got %#v", due)
	}
}
