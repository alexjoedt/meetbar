package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/alex/meetbar/internal/alert"
	"github.com/alex/meetbar/internal/auth"
	"github.com/alex/meetbar/internal/calendar"
	"github.com/alex/meetbar/internal/config"
	"github.com/alex/meetbar/internal/ipc"
	"github.com/alex/meetbar/internal/notify"
)

// authManager and eventSyncer are the subsets of *auth.Manager and
// *calendar.Syncer that App depends on. Keeping them as interfaces lets
// tests substitute fakes without touching OAuth or the Calendar API.
type authManager interface {
	Status() (string, string)
	HTTPClient(ctx context.Context) (*http.Client, error)
	Logout() error
}

type eventSyncer interface {
	Refresh(ctx context.Context) error
	Snapshot() (events []ipc.Event, lastSync time.Time, lastErr string)
	Upcoming(hours int) []ipc.Event
	ListCalendars(ctx context.Context) ([]ipc.CalendarInfo, error)
}

type App struct {
	cfg    config.Config
	paths  config.Paths
	alerts *alert.Store
	log    *slog.Logger
	notify func(ctx context.Context, title, body, actionURL string) error

	// auth and syncer are replaced wholesale on a successful login, which
	// happens on its own request goroutine while other goroutines (the
	// background sync/alert loop, other in-flight IPC requests) may be
	// reading them concurrently. mu guards both fields.
	mu     sync.RWMutex
	auth   authManager
	syncer eventSyncer
}

func (a *App) getAuth() authManager {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.auth
}

func (a *App) getSyncer() eventSyncer {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.syncer
}

func (a *App) setAuthAndSyncer(am authManager, sy eventSyncer) {
	a.mu.Lock()
	a.auth = am
	a.syncer = sy
	a.mu.Unlock()
}

func Run(ctx context.Context, log *slog.Logger) error {
	paths, err := config.ResolvePaths()
	if err != nil {
		return err
	}
	if err := config.EnsureDir(paths.DataDir, 0o700); err != nil {
		return err
	}
	if err := config.EnsureDir(paths.ConfigDir, 0o755); err != nil {
		return err
	}

	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return err
	}

	credPaths := []string{
		filepath.Join(paths.ConfigDir, "credentials.json"),
		filepath.Join(paths.DataDir, "credentials.json"),
	}
	creds, err := auth.ResolveCredentials(credPaths...)
	if err != nil {
		log.Warn("oauth credentials not configured yet", "err", err)
		// Allow daemon to start; login will fail with a clear error until configured.
		creds = auth.Credentials{}
	}

	authMgr, err := auth.NewManager(creds, paths.TokenFile)
	if err != nil {
		return err
	}

	syncer := calendar.NewSyncer(func(ctx context.Context) (*http.Client, error) {
		return authMgr.HTTPClient(ctx)
	}, cfg.Horizon, calendar.Filter{
		Mode: cfg.CalendarFilter,
		IDs:  cfg.Calendars,
	})

	alerts, err := alert.NewStore(paths.AlertsFile, cfg.WarnMinutes)
	if err != nil {
		return err
	}

	app := &App{
		cfg:    cfg,
		paths:  paths,
		auth:   authMgr,
		syncer: syncer,
		alerts: alerts,
		log:    log,
		notify: notify.Desktop,
	}

	srv := ipc.NewServer(paths.SocketPath, app.handle)
	if err := srv.Start(ctx); err != nil {
		return err
	}
	log.Info("listening", "socket", paths.SocketPath)

	// Initial sync if authenticated
	if st, _ := authMgr.Status(); st == "connected" {
		if err := syncer.Refresh(ctx); err != nil {
			log.Warn("initial sync failed", "err", err)
		}
	}

	go app.loop(ctx)

	srv.Wait()
	return nil
}

func (a *App) loop(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.SyncInterval)
	defer ticker.Stop()
	alertTicker := time.NewTicker(30 * time.Second)
	defer alertTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if st, _ := a.getAuth().Status(); st != "connected" {
				continue
			}
			if err := a.getSyncer().Refresh(ctx); err != nil {
				a.log.Warn("sync failed", "err", err)
				continue
			}
			a.evaluateAlerts(ctx)
		case <-alertTicker.C:
			if st, _ := a.getAuth().Status(); st != "connected" {
				continue
			}
			a.evaluateAlerts(ctx)
		}
	}
}

func (a *App) evaluateAlerts(ctx context.Context) {
	events, _, _ := a.getSyncer().Snapshot()
	due := a.alerts.Evaluate(time.Now(), events)
	if len(due) == 0 || !a.cfg.DaemonNotify() {
		return
	}
	// Only auto-ack when daemon is the sole notifier. For "both", leave
	// alerts pending so the Noctalia plugin can notify and ack.
	ackAfter := a.cfg.NotifyPath == "daemon"
	var keys []string
	for _, al := range due {
		title, body := notify.FormatAlert(al.ThresholdMin, al.Title, al.JoinURL)
		if err := a.notify(ctx, title, body, al.JoinURL); err != nil {
			a.log.Warn("notify-send failed", "err", err)
			continue
		}
		if ackAfter {
			keys = append(keys, al.Key)
		}
	}
	if len(keys) > 0 {
		if err := a.alerts.Ack(keys); err != nil {
			a.log.Warn("ack failed", "err", err)
		}
	}
}

func (a *App) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "status":
		return a.status(), nil
	case "auth.login":
		return a.login(ctx)
	case "auth.logout":
		if err := a.getAuth().Logout(); err != nil {
			return nil, err
		}
		return map[string]string{"auth": "disconnected"}, nil
	case "upcoming":
		var p ipc.UpcomingParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		if p.Hours <= 0 {
			p.Hours = 12
		}
		// Grab stable references for the duration of this request so a
		// concurrent login (which swaps both) can't interleave weirdly.
		syncer := a.getSyncer()
		authMgr := a.getAuth()
		// refresh if empty or stale
		_, last, _ := syncer.Snapshot()
		if last.IsZero() || time.Since(last) > a.cfg.SyncInterval {
			if st, _ := authMgr.Status(); st == "connected" {
				_ = syncer.Refresh(ctx)
			}
		}
		return ipc.UpcomingResult{Events: syncer.Upcoming(p.Hours)}, nil
	case "alerts.poll":
		events, _, _ := a.getSyncer().Snapshot()
		a.alerts.Evaluate(time.Now(), events)
		return ipc.AlertsPollResult{Alerts: a.alerts.Poll()}, nil
	case "alerts.ack":
		var p ipc.AlertsAckParams
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if err := a.alerts.Ack(p.Keys); err != nil {
			return nil, err
		}
		return map[string]any{"acked": len(p.Keys)}, nil
	case "calendars.list":
		cals, err := a.getSyncer().ListCalendars(ctx)
		if err != nil {
			return nil, err
		}
		return ipc.CalendarsResult{Calendars: cals}, nil
	case "sync":
		return a.syncNow(ctx)
	default:
		return nil, fmt.Errorf("unknown method %q", method)
	}
}

// syncNow forces a Google Calendar pull, then re-evaluates alert thresholds.
func (a *App) syncNow(ctx context.Context) (any, error) {
	authMgr := a.getAuth()
	st, _ := authMgr.Status()
	if st != "connected" {
		return nil, fmt.Errorf("not connected")
	}
	syncer := a.getSyncer()
	if err := syncer.Refresh(ctx); err != nil {
		return nil, fmt.Errorf("sync calendars: %w", err)
	}
	a.evaluateAlerts(ctx)
	events, last, _ := syncer.Snapshot()
	res := ipc.SyncResult{Events: len(events)}
	if !last.IsZero() {
		res.LastSync = last.Format(time.RFC3339)
	}
	return res, nil
}

func (a *App) status() ipc.StatusResult {
	authState, email := a.getAuth().Status()
	_, last, syncErr := a.getSyncer().Snapshot()
	res := ipc.StatusResult{
		Daemon: "ok",
		Auth:   authState,
		Email:  email,
	}
	if !last.IsZero() {
		res.LastSync = last.Format(time.RFC3339)
	}
	if syncErr != "" {
		res.SyncErr = syncErr
	}
	return res
}

func (a *App) login(ctx context.Context) (any, error) {
	// Re-resolve credentials in case user dropped credentials.json after start
	credPaths := []string{
		filepath.Join(a.paths.ConfigDir, "credentials.json"),
		filepath.Join(a.paths.DataDir, "credentials.json"),
	}
	creds, err := auth.ResolveCredentials(credPaths...)
	if err != nil {
		return nil, err
	}
	mgr, err := auth.NewManager(creds, a.paths.TokenFile)
	if err != nil {
		return nil, err
	}
	// preserve existing token state if any — replace manager
	email, err := mgr.Login(ctx)
	if err != nil {
		return nil, err
	}
	syncer := calendar.NewSyncer(func(ctx context.Context) (*http.Client, error) {
		return a.getAuth().HTTPClient(ctx)
	}, a.cfg.Horizon, calendar.Filter{
		Mode: a.cfg.CalendarFilter,
		IDs:  a.cfg.Calendars,
	})
	a.setAuthAndSyncer(mgr, syncer)
	if err := syncer.Refresh(ctx); err != nil {
		a.log.Warn("post-login sync failed", "err", err)
	}
	return ipc.LoginResult{Email: email}, nil
}
