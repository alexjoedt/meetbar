package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(filepath.Join(dir, "does-not-exist.toml"))
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Fatalf("Load() = %#v, want defaults %#v", cfg, Defaults())
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		toml    string
		wantErr bool
		check   func(t *testing.T, cfg Config)
	}{
		{
			name: "full override",
			toml: `
warn_minutes    = [30, 10]
sync_interval   = "5m"
horizon         = "48h"
notify_path     = "both"
calendar_filter = "all"
calendars       = ["primary", "team@example.com"]
`,
			check: func(t *testing.T, cfg Config) {
				if !reflect.DeepEqual(cfg.WarnMinutes, []int{30, 10}) {
					t.Errorf("WarnMinutes = %v, want [30 10]", cfg.WarnMinutes)
				}
				if cfg.SyncInterval != 5*time.Minute {
					t.Errorf("SyncInterval = %v, want 5m", cfg.SyncInterval)
				}
				if cfg.Horizon != 48*time.Hour {
					t.Errorf("Horizon = %v, want 48h", cfg.Horizon)
				}
				if cfg.NotifyPath != "both" {
					t.Errorf("NotifyPath = %q, want both", cfg.NotifyPath)
				}
				if cfg.CalendarFilter != "all" {
					t.Errorf("CalendarFilter = %q, want all", cfg.CalendarFilter)
				}
				want := []string{"primary", "team@example.com"}
				if !reflect.DeepEqual(cfg.Calendars, want) {
					t.Errorf("Calendars = %v, want %v", cfg.Calendars, want)
				}
			},
		},
		{
			name: "partial override keeps other defaults",
			toml: `notify_path = "daemon"`,
			check: func(t *testing.T, cfg Config) {
				if cfg.NotifyPath != "daemon" {
					t.Errorf("NotifyPath = %q, want daemon", cfg.NotifyPath)
				}
				def := Defaults()
				if cfg.SyncInterval != def.SyncInterval {
					t.Errorf("SyncInterval = %v, want default %v", cfg.SyncInterval, def.SyncInterval)
				}
				if cfg.Horizon != def.Horizon {
					t.Errorf("Horizon = %v, want default %v", cfg.Horizon, def.Horizon)
				}
				if !reflect.DeepEqual(cfg.WarnMinutes, def.WarnMinutes) {
					t.Errorf("WarnMinutes = %v, want default %v", cfg.WarnMinutes, def.WarnMinutes)
				}
			},
		},
		{
			name: "empty file keeps defaults",
			toml: "",
			check: func(t *testing.T, cfg Config) {
				if !reflect.DeepEqual(cfg, Defaults()) {
					t.Errorf("cfg = %#v, want defaults %#v", cfg, Defaults())
				}
			},
		},
		{
			name:    "invalid sync_interval duration",
			toml:    `sync_interval = "banana"`,
			wantErr: true,
		},
		{
			name:    "invalid horizon duration",
			toml:    `horizon = "banana"`,
			wantErr: true,
		},
		{
			name:    "invalid toml syntax",
			toml:    `not = [valid toml`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(path, []byte(tt.toml), 0o600); err != nil {
				t.Fatal(err)
			}

			cfg, err := Load(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() unexpected error: %v", err)
			}
			tt.check(t, cfg)
		})
	}
}

func TestDaemonNotify(t *testing.T) {
	tests := []struct {
		notifyPath string
		want       bool
	}{
		{notifyPath: "noctalia", want: false},
		{notifyPath: "daemon", want: true},
		{notifyPath: "both", want: true},
		{notifyPath: "", want: false},
		{notifyPath: "unknown", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.notifyPath, func(t *testing.T) {
			cfg := Config{NotifyPath: tt.notifyPath}
			if got := cfg.DaemonNotify(); got != tt.want {
				t.Errorf("DaemonNotify() with NotifyPath=%q = %v, want %v", tt.notifyPath, got, tt.want)
			}
		})
	}
}

func TestResolvePathsUsesXDGEnv(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(root, "run"))

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}

	tests := []struct {
		name string
		got  string
		want string
	}{
		{"ConfigDir", paths.ConfigDir, filepath.Join(root, "config", appName)},
		{"DataDir", paths.DataDir, filepath.Join(root, "data", appName)},
		{"RuntimeDir", paths.RuntimeDir, filepath.Join(root, "run", appName)},
		{"ConfigFile", paths.ConfigFile, filepath.Join(root, "config", appName, "config.toml")},
		{"TokenFile", paths.TokenFile, filepath.Join(root, "data", appName, "token.json")},
		{"AlertsFile", paths.AlertsFile, filepath.Join(root, "data", appName, "alerts.json")},
		{"SocketPath", paths.SocketPath, filepath.Join(root, "run", appName, "meetbar.sock")},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %q, want %q", tt.name, tt.got, tt.want)
		}
	}
}

func TestResolvePathsFallsBackToHomeDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir available: %v", err)
	}

	paths, err := ResolvePaths()
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	wantConfig := filepath.Join(home, ".config", appName)
	if paths.ConfigDir != wantConfig {
		t.Errorf("ConfigDir = %q, want %q", paths.ConfigDir, wantConfig)
	}
	wantData := filepath.Join(home, ".local", "share", appName)
	if paths.DataDir != wantData {
		t.Errorf("DataDir = %q, want %q", paths.DataDir, wantData)
	}
}
