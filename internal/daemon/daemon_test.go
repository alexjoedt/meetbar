package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alex/meetbar/internal/alert"
	"github.com/alex/meetbar/internal/config"
	"github.com/alex/meetbar/internal/ipc"
)

// fakeAuth and fakeSyncer are minimal, concurrency-safe stand-ins for
// *auth.Manager and *calendar.Syncer so daemon logic can be tested without
// touching OAuth or the Calendar API.
type fakeAuth struct {
	mu    sync.Mutex
	state string
	email string
}

func (f *fakeAuth) Status() (string, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.state, f.email
}

func (f *fakeAuth) HTTPClient(ctx context.Context) (*http.Client, error) {
	return &http.Client{}, nil
}

func (f *fakeAuth) Logout() error {
	f.mu.Lock()
	f.state = "disconnected"
	f.email = ""
	f.mu.Unlock()
	return nil
}

type fakeSyncer struct {
	mu           sync.Mutex
	events       []ipc.Event
	refreshErr   error
	refreshCalls int
	calendars    []ipc.CalendarInfo
}

func (f *fakeSyncer) Refresh(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.refreshCalls++
	return f.refreshErr
}

func (f *fakeSyncer) Snapshot() ([]ipc.Event, time.Time, string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ipc.Event(nil), f.events...), time.Now(), ""
}

func (f *fakeSyncer) Upcoming(hours int) []ipc.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ipc.Event(nil), f.events...)
}

func (f *fakeSyncer) Today() []ipc.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ipc.Event(nil), f.events...)
}

func (f *fakeSyncer) ListCalendars(ctx context.Context) ([]ipc.CalendarInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calendars == nil {
		return []ipc.CalendarInfo{{ID: "primary", Summary: "Primary", Primary: true}}, nil
	}
	return append([]ipc.CalendarInfo(nil), f.calendars...), nil
}

func newTestApp(t *testing.T, warnMinutes []int) (*App, *fakeAuth, *fakeSyncer) {
	t.Helper()
	dir := t.TempDir()
	store, err := alert.NewStore(filepath.Join(dir, "alerts.json"), warnMinutes)
	if err != nil {
		t.Fatal(err)
	}
	fa := &fakeAuth{state: "connected", email: "you@example.com"}
	fs := &fakeSyncer{}
	app := &App{
		cfg:    config.Defaults(),
		paths:  config.Paths{ConfigDir: dir, DataDir: dir, TokenFile: filepath.Join(dir, "token.json")},
		auth:   fa,
		syncer: fs,
		alerts: store,
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		notify: func(ctx context.Context, title, body, actionURL string) error { return nil },
	}
	return app, fa, fs
}

func TestHandleDispatch(t *testing.T) {
	tests := []struct {
		name   string
		method string
		params any
		setup  func(fs *fakeSyncer)
		check  func(t *testing.T, result any, err error)
	}{
		{
			name:   "status reports auth and email",
			method: "status",
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				res := result.(ipc.StatusResult)
				if res.Daemon != "ok" || res.Auth != "connected" || res.Email != "you@example.com" {
					t.Fatalf("unexpected status: %#v", res)
				}
			},
		},
		{
			name:   "upcoming returns synced events",
			method: "upcoming",
			params: ipc.UpcomingParams{Hours: 6},
			setup: func(fs *fakeSyncer) {
				fs.events = []ipc.Event{{ID: "e1", Title: "Standup", Start: time.Now().Format(time.RFC3339)}}
			},
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				res := result.(ipc.UpcomingResult)
				if len(res.Events) != 1 || res.Events[0].ID != "e1" {
					t.Fatalf("unexpected events: %#v", res.Events)
				}
			},
		},
		{
			name:   "today returns synced events",
			method: "today",
			setup: func(fs *fakeSyncer) {
				fs.events = []ipc.Event{{ID: "e1", Title: "Standup", Start: time.Now().Format(time.RFC3339)}}
			},
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				res := result.(ipc.TodayResult)
				if len(res.Events) != 1 || res.Events[0].ID != "e1" {
					t.Fatalf("unexpected events: %#v", res.Events)
				}
			},
		},
		{
			name:   "alerts.poll with no due alerts",
			method: "alerts.poll",
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				res := result.(ipc.AlertsPollResult)
				if len(res.Alerts) != 0 {
					t.Fatalf("expected no alerts, got %#v", res.Alerts)
				}
			},
		},
		{
			name:   "calendars.list",
			method: "calendars.list",
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				res := result.(ipc.CalendarsResult)
				if len(res.Calendars) != 1 || !res.Calendars[0].Primary {
					t.Fatalf("unexpected calendars: %#v", res.Calendars)
				}
			},
		},
		{
			name:   "sync forces refresh",
			method: "sync",
			setup: func(fs *fakeSyncer) {
				fs.events = []ipc.Event{{ID: "e1", Title: "Standup"}}
			},
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
				res := result.(ipc.SyncResult)
				if res.Events != 1 {
					t.Fatalf("events = %d, want 1", res.Events)
				}
				if res.LastSync == "" {
					t.Fatal("expected last_sync to be set")
				}
			},
		},
		{
			name:   "auth.logout disconnects",
			method: "auth.logout",
			check: func(t *testing.T, result any, err error) {
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "unknown method errors",
			method: "bogus",
			check: func(t *testing.T, result any, err error) {
				if err == nil {
					t.Fatal("expected error for unknown method, got nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, fs := newTestApp(t, []int{15, 5, 0})
			if tt.setup != nil {
				tt.setup(fs)
			}

			var raw json.RawMessage
			if tt.params != nil {
				b, err := json.Marshal(tt.params)
				if err != nil {
					t.Fatal(err)
				}
				raw = b
			}

			result, err := app.handle(context.Background(), tt.method, raw)
			tt.check(t, result, err)
		})
	}
}

func TestSyncRequiresConnected(t *testing.T) {
	app, fa, fs := newTestApp(t, []int{0})
	fa.state = "disconnected"
	fa.email = ""

	_, err := app.handle(context.Background(), "sync", nil)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if fs.refreshCalls != 0 {
		t.Fatalf("refreshCalls = %d, want 0 when disconnected", fs.refreshCalls)
	}
}

func TestSyncPropagatesRefreshError(t *testing.T) {
	app, _, fs := newTestApp(t, []int{0})
	fs.refreshErr = errors.New("calendar down")

	_, err := app.handle(context.Background(), "sync", nil)
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if fs.refreshCalls != 1 {
		t.Fatalf("refreshCalls = %d, want 1", fs.refreshCalls)
	}
}

func TestHandleAuthLoginWithoutCredentials(t *testing.T) {
	// Isolate from the environment/embedded defaults so this deterministically
	// hits the "no credentials configured" error path without opening a browser.
	t.Setenv("MEETBAR_CLIENT_ID", "")
	t.Setenv("MEETBAR_CLIENT_SECRET", "")
	t.Setenv("MEETBAR_CREDENTIALS", "")

	app, _, _ := newTestApp(t, []int{0})

	_, err := app.handle(context.Background(), "auth.login", nil)
	if err == nil {
		t.Fatal("expected error when no OAuth credentials are configured")
	}
}

func TestHandleAlertsPollAndAck(t *testing.T) {
	app, _, fs := newTestApp(t, []int{0})

	start := time.Now().Add(-time.Minute)
	fs.events = []ipc.Event{{
		ID:    "evt1",
		Title: "Standup",
		Start: start.Format(time.RFC3339),
		End:   start.Add(30 * time.Minute).Format(time.RFC3339),
	}}

	result, err := app.handle(context.Background(), "alerts.poll", nil)
	if err != nil {
		t.Fatal(err)
	}
	poll := result.(ipc.AlertsPollResult)
	if len(poll.Alerts) != 1 {
		t.Fatalf("expected 1 due alert, got %#v", poll.Alerts)
	}

	ackParams, err := json.Marshal(ipc.AlertsAckParams{Keys: []string{poll.Alerts[0].Key}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.handle(context.Background(), "alerts.ack", ackParams); err != nil {
		t.Fatal(err)
	}

	result, err = app.handle(context.Background(), "alerts.poll", nil)
	if err != nil {
		t.Fatal(err)
	}
	poll = result.(ipc.AlertsPollResult)
	if len(poll.Alerts) != 0 {
		t.Fatalf("expected alerts cleared after ack, got %#v", poll.Alerts)
	}
}

func TestEvaluateAlertsNotifyPaths(t *testing.T) {
	tests := []struct {
		name                 string
		notifyPath           string
		notifyErr            error
		wantNotifyCalls      int
		wantRemainingPending int
	}{
		{
			name:                 "noctalia path never calls daemon notify",
			notifyPath:           "noctalia",
			wantNotifyCalls:      0,
			wantRemainingPending: 1,
		},
		{
			name:                 "daemon path notifies and acks",
			notifyPath:           "daemon",
			wantNotifyCalls:      1,
			wantRemainingPending: 0,
		},
		{
			name:                 "both path notifies but leaves pending for the plugin to ack",
			notifyPath:           "both",
			wantNotifyCalls:      1,
			wantRemainingPending: 1,
		},
		{
			name:                 "daemon path notify failure leaves alert pending",
			notifyPath:           "daemon",
			notifyErr:            errors.New("notify-send failed"),
			wantNotifyCalls:      1,
			wantRemainingPending: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _, fs := newTestApp(t, []int{0})
			app.cfg.NotifyPath = tt.notifyPath

			start := time.Now()
			fs.events = []ipc.Event{{
				ID:    "evt1",
				Title: "Standup",
				Start: start.Format(time.RFC3339),
				End:   start.Add(30 * time.Minute).Format(time.RFC3339),
			}}

			var calls int
			app.notify = func(ctx context.Context, title, body, actionURL string) error {
				calls++
				return tt.notifyErr
			}

			app.evaluateAlerts(context.Background())

			if calls != tt.wantNotifyCalls {
				t.Fatalf("notify calls = %d, want %d", calls, tt.wantNotifyCalls)
			}
			pending := app.alerts.Poll()
			if len(pending) != tt.wantRemainingPending {
				t.Fatalf("remaining pending = %d, want %d (%#v)", len(pending), tt.wantRemainingPending, pending)
			}
		})
	}
}

// TestAppAuthSyncerSwapIsRaceFree exercises the exact scenario that used to
// be a data race: concurrent reads of auth/syncer (as happen from other
// in-flight IPC requests and the background sync loop) interleaved with a
// wholesale swap of both (as login() performs on success). Run with -race.
func TestAppAuthSyncerSwapIsRaceFree(t *testing.T) {
	app, _, _ := newTestApp(t, []int{0})

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			app.setAuthAndSyncer(&fakeAuth{state: "connected"}, &fakeSyncer{})
		}
	}()

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = app.getAuth().Status()
				_, _, _ = app.getSyncer().Snapshot()
			}
		}()
	}

	wg.Wait()
}
