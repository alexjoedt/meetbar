package calendar

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/alex/meetbar/internal/ipc"
	gcal "google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

type ClientFactory func(ctx context.Context) (*http.Client, error)

// Filter selects which calendars to sync.
// Mode: "primary" (default), "owned", "all".
// IDs: when non-empty, only these calendar IDs (use "*" for all).
type Filter struct {
	Mode string
	IDs  []string
}

type Syncer struct {
	mu        sync.RWMutex
	newClient ClientFactory
	horizon   time.Duration
	filter    Filter
	events    []ipc.Event
	lastSync  time.Time
	lastErr   string
}

func NewSyncer(newClient ClientFactory, horizon time.Duration, filter Filter) *Syncer {
	if filter.Mode == "" {
		filter.Mode = "primary"
	}
	return &Syncer{newClient: newClient, horizon: horizon, filter: filter}
}

func (s *Syncer) SetHorizon(d time.Duration) {
	s.mu.Lock()
	s.horizon = d
	s.mu.Unlock()
}

func (s *Syncer) SetFilter(f Filter) {
	if f.Mode == "" {
		f.Mode = "primary"
	}
	s.mu.Lock()
	s.filter = f
	s.mu.Unlock()
}

func (s *Syncer) Snapshot() (events []ipc.Event, lastSync time.Time, lastErr string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events = append([]ipc.Event(nil), s.events...)
	return events, s.lastSync, s.lastErr
}

func (s *Syncer) Upcoming(hours int) []ipc.Event {
	if hours <= 0 {
		hours = 12
	}
	now := time.Now()
	until := now.Add(time.Duration(hours) * time.Hour)
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ipc.Event
	for _, ev := range s.events {
		if ev.AllDay {
			continue
		}
		start, err := time.Parse(time.RFC3339, ev.Start)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, ev.End)
		if err != nil {
			end = start
		}
		if end.Before(now) {
			continue
		}
		if start.After(until) {
			continue
		}
		cp := ev
		mins := int(start.Sub(now).Minutes())
		cp.MinutesUntil = &mins
		out = append(out, cp)
	}
	return out
}

// Today returns every timed event starting on the current local day, earliest
// first, including ones that have already finished. All-day events are skipped,
// matching Upcoming.
func (s *Syncer) Today() []ipc.Event {
	return s.todayAt(time.Now())
}

// todayAt is the testable core of Today; tests pin now instead of the wall clock.
func (s *Syncer) todayAt(now time.Time) []ipc.Event {
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	dayEnd := dayStart.AddDate(0, 0, 1) // AddDate, not +24h, so DST shifts stay correct

	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []ipc.Event
	for _, ev := range s.events {
		if ev.AllDay {
			continue
		}
		start, err := time.Parse(time.RFC3339, ev.Start)
		if err != nil {
			continue
		}
		end, err := time.Parse(time.RFC3339, ev.End)
		if err != nil {
			end = start
		}
		if !end.After(dayStart) || !start.Before(dayEnd) {
			continue
		}
		cp := ev
		mins := int(start.Sub(now).Minutes())
		cp.MinutesUntil = &mins
		out = append(out, cp)
	}
	return out
}

func (s *Syncer) Refresh(ctx context.Context) error {
	client, err := s.newClient(ctx)
	if err != nil {
		s.setErr(err)
		return err
	}
	svc, err := gcal.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		s.setErr(err)
		return err
	}

	s.mu.RLock()
	horizon := s.horizon
	filter := s.filter
	s.mu.RUnlock()

	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	// Fetch from the start of the local day so `today` can also show meetings that
	// already finished. Keep the 15-minute grace window for the small hours, when it
	// reaches further back than midnight does.
	from := dayStart
	if grace := now.Add(-15 * time.Minute); grace.Before(from) {
		from = grace
	}
	timeMin := from.Format(time.RFC3339)
	timeMax := now.Add(horizon).Format(time.RFC3339)

	calIDs, err := listCalendarIDs(ctx, svc, filter)
	if err != nil {
		s.setErr(err)
		return err
	}

	var events []ipc.Event
	for _, calID := range calIDs {
		call := svc.Events.List(calID).
			Context(ctx).
			SingleEvents(true).
			OrderBy("startTime").
			TimeMin(timeMin).
			TimeMax(timeMax).
			ShowDeleted(false).
			MaxResults(100)
		resp, err := call.Do()
		if err != nil {
			s.setErr(err)
			return fmt.Errorf("events %s: %w — if this is a scope error, run: meetbarctl logout && meetbarctl login", calID, err)
		}
		for _, item := range resp.Items {
			if item == nil || item.Status == "cancelled" {
				continue
			}
			if declinedBySelf(item) {
				continue
			}
			ev, ok := mapEvent(calID, item)
			if ok {
				events = append(events, ev)
			}
		}
	}

	sortEvents(events)

	s.mu.Lock()
	s.events = events
	s.lastSync = time.Now()
	s.lastErr = ""
	s.mu.Unlock()
	return nil
}

func (s *Syncer) ListCalendars(ctx context.Context) ([]ipc.CalendarInfo, error) {
	client, err := s.newClient(ctx)
	if err != nil {
		return nil, err
	}
	svc, err := gcal.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	calList, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		// Fall back to primary when list is denied.
		return []ipc.CalendarInfo{{ID: "primary", Summary: "Primary", Primary: true}}, nil
	}
	var out []ipc.CalendarInfo
	for _, cal := range calList.Items {
		if cal == nil {
			continue
		}
		out = append(out, ipc.CalendarInfo{
			ID:      cal.Id,
			Summary: cal.Summary,
			Primary: cal.Primary,
		})
	}
	return out, nil
}

func listCalendarIDs(ctx context.Context, svc *gcal.Service, filter Filter) ([]string, error) {
	// Explicit ID list wins (unless it is just ["*"]).
	if len(filter.IDs) > 0 && !(len(filter.IDs) == 1 && filter.IDs[0] == "*") {
		return append([]string(nil), filter.IDs...), nil
	}
	wantAll := filter.Mode == "all" || (len(filter.IDs) == 1 && filter.IDs[0] == "*")
	if filter.Mode == "primary" && !wantAll {
		return []string{"primary"}, nil
	}

	calList, err := svc.CalendarList.List().Context(ctx).Do()
	if err != nil {
		return []string{"primary"}, nil
	}

	var ids []string
	for _, cal := range calList.Items {
		if cal == nil || cal.Id == "" {
			continue
		}
		if wantAll {
			ids = append(ids, cal.Id)
			continue
		}
		// owned: owner or writer (your calendars, not holidays/subscribed read-only)
		role := cal.AccessRole
		if role == "owner" || role == "writer" {
			ids = append(ids, cal.Id)
		}
	}
	if len(ids) == 0 {
		return []string{"primary"}, nil
	}
	return ids, nil
}

func declinedBySelf(item *gcal.Event) bool {
	if item.Attendees == nil {
		return false
	}
	for _, a := range item.Attendees {
		if a == nil {
			continue
		}
		if a.Self && a.ResponseStatus == "declined" {
			return true
		}
	}
	return false
}

func (s *Syncer) setErr(err error) {
	s.mu.Lock()
	s.lastErr = err.Error()
	s.mu.Unlock()
}

func mapEvent(calendarID string, item *gcal.Event) (ipc.Event, bool) {
	start, end, allDay, ok := parseTimes(item)
	if !ok {
		return ipc.Event{}, false
	}
	title := item.Summary
	if title == "" {
		title = "(no title)"
	}
	id := item.Id
	if item.RecurringEventId != "" {
		id = item.Id // instance id already unique with SingleEvents
	}
	return ipc.Event{
		ID:         id,
		Title:      title,
		Start:      start.Format(time.RFC3339),
		End:        end.Format(time.RFC3339),
		JoinURL:    ExtractJoinURL(item),
		Location:   item.Location,
		CalendarID: calendarID,
		AllDay:     allDay,
	}, true
}

func parseTimes(item *gcal.Event) (start, end time.Time, allDay bool, ok bool) {
	if item.Start == nil {
		return time.Time{}, time.Time{}, false, false
	}
	if item.Start.DateTime != "" {
		st, err := time.Parse(time.RFC3339, item.Start.DateTime)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		en := st.Add(time.Hour)
		if item.End != nil && item.End.DateTime != "" {
			if t, err := time.Parse(time.RFC3339, item.End.DateTime); err == nil {
				en = t
			}
		}
		return st, en, false, true
	}
	if item.Start.Date != "" {
		st, err := time.ParseInLocation("2006-01-02", item.Start.Date, time.Local)
		if err != nil {
			return time.Time{}, time.Time{}, false, false
		}
		en := st.Add(24 * time.Hour)
		if item.End != nil && item.End.Date != "" {
			if t, err := time.ParseInLocation("2006-01-02", item.End.Date, time.Local); err == nil {
				en = t
			}
		}
		return st, en, true, true
	}
	return time.Time{}, time.Time{}, false, false
}

func sortEvents(events []ipc.Event) {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start < events[j].Start
	})
}
