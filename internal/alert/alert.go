package alert

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alex/meetbar/internal/ipc"
)

type Store struct {
	mu       sync.Mutex
	path     string
	acked    map[string]time.Time
	pending  []ipc.Alert
	warnMins []int
}

func NewStore(path string, warnMinutes []int) (*Store, error) {
	s := &Store{
		path:     path,
		acked:    map[string]time.Time{},
		warnMins: append([]int(nil), warnMinutes...),
	}
	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

func (s *Store) SetWarnMinutes(mins []int) {
	s.mu.Lock()
	s.warnMins = append([]int(nil), mins...)
	s.mu.Unlock()
}

type eventWinner struct {
	alert        ipc.Alert
	thresholdMin int
}

func (s *Store) Evaluate(now time.Time, events []ipc.Event) []ipc.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()

	activeIDs := map[string]bool{}
	silentlyAcked := false

	// Pass 1: per-event collapse — pick the most-urgent overdue threshold for
	// each event, silently acking the rest (handles suspend/missed ticks).
	var winners []eventWinner
	for _, ev := range events {
		if ev.AllDay {
			continue
		}
		start, err := time.Parse(time.RFC3339, ev.Start)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, ev.End)
		if err != nil {
			end = start.Add(time.Hour)
		}
		if end.Before(now) {
			continue
		}
		activeIDs[ev.ID] = true

		// overdue collects every not-yet-acked threshold whose time has
		// already passed. Normally only one threshold becomes due per
		// evaluation, but if the event was created/synced less than the
		// largest warn window before it starts (or the daemon missed a
		// stretch of ticks, e.g. suspend/sleep), several thresholds can be
		// overdue simultaneously. Firing one notification per stacked
		// threshold would surface a burst of near-duplicate alerts for the
		// same meeting, so only the most urgent (smallest) one is surfaced;
		// the rest are acked silently without notifying.
		var overdue []int
		for _, mins := range s.warnMins {
			key := fmt.Sprintf("%s:%d", ev.ID, mins)
			if _, ok := s.acked[key]; ok {
				continue
			}
			thresholdAt := start.Add(-time.Duration(mins) * time.Minute)
			if now.Before(thresholdAt) {
				continue
			}
			// Don't fire very late after start for non-zero thresholds (missed window > 2h).
			if mins > 0 && now.After(start.Add(2*time.Hour)) {
				continue
			}
			overdue = append(overdue, mins)
		}
		if len(overdue) == 0 {
			continue
		}
		notifyMin := overdue[0]
		for _, mins := range overdue[1:] {
			if mins < notifyMin {
				notifyMin = mins
			}
		}
		for _, mins := range overdue {
			key := fmt.Sprintf("%s:%d", ev.ID, mins)
			if mins != notifyMin {
				s.acked[key] = now
				silentlyAcked = true
				continue
			}
			winners = append(winners, eventWinner{
				alert: ipc.Alert{
					Key:          key,
					EventID:      ev.ID,
					ThresholdMin: mins,
					Title:        ev.Title,
					Start:        ev.Start,
					JoinURL:      ev.JoinURL,
				},
				thresholdMin: mins,
			})
		}
	}

	// Pass 2: cross-event suppression — if any meeting is starting now
	// (threshold == 0), suppress reminder notifications for upcoming meetings
	// so the "Starting now" alert isn't buried under future-meeting noise.
	hasStarting := false
	for _, w := range winners {
		if w.thresholdMin == 0 {
			hasStarting = true
			break
		}
	}

	var due []ipc.Alert
	for _, w := range winners {
		if hasStarting && w.thresholdMin > 0 {
			s.acked[w.alert.Key] = now
			silentlyAcked = true
			continue
		}
		due = append(due, w.alert)
	}

	// Drop acks for events no longer relevant.
	for key := range s.acked {
		eventID, _, _ := strings.Cut(key, ":")
		if !activeIDs[eventID] {
			delete(s.acked, key)
		}
	}

	s.pending = due
	if silentlyAcked {
		// Best effort: if this fails, the worst case is a stacked threshold
		// re-firing (and being collapsed again) after a daemon restart.
		_ = s.save()
	}
	return append([]ipc.Alert(nil), due...)
}

func (s *Store) Poll() []ipc.Alert {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ipc.Alert(nil), s.pending...)
}

func (s *Store) Ack(keys []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for _, k := range keys {
		s.acked[k] = now
	}
	// Remove acked from pending
	var rest []ipc.Alert
	acked := map[string]bool{}
	for _, k := range keys {
		acked[k] = true
	}
	for _, a := range s.pending {
		if !acked[a.Key] {
			rest = append(rest, a)
		}
	}
	s.pending = rest
	return s.save()
}

type fileShape struct {
	Acked map[string]time.Time `json:"acked"`
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var f fileShape
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	if f.Acked != nil {
		s.acked = f.Acked
	}
	return nil
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	f := fileShape{Acked: s.acked}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
