package main

import (
	"strings"
	"testing"
	"time"

	"github.com/alex/meetbar/internal/ipc"
)

func TestFormatTodayLineUnparseableEnd(t *testing.T) {
	now := time.Date(2026, 8, 12, 13, 0, 0, 0, time.UTC)
	ev := ipc.Event{
		Title: "Weird event",
		Start: now.Add(-30 * time.Minute).Format(time.RFC3339),
		End:   "not-a-timestamp",
	}

	line := formatTodayLine(now, ev)
	if !strings.Contains(line, "(done)") {
		t.Fatalf("expected line to eventually read (done) once started, got %q", line)
	}
}
