package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

const appName = "meetbar"

type Config struct {
	WarnMinutes  []int         `toml:"warn_minutes"`
	SyncInterval time.Duration `toml:"sync_interval"`
	Horizon      time.Duration `toml:"horizon"`
	NotifyPath   string        `toml:"notify_path"` // noctalia | daemon | both
	// CalendarFilter: primary (default) | owned | all
	// Ignored when Calendars is non-empty.
	CalendarFilter string   `toml:"calendar_filter"`
	Calendars      []string `toml:"calendars"` // explicit calendar IDs, or ["*"] for all
}

func Defaults() Config {
	return Config{
		WarnMinutes:    []int{15, 5, 0},
		SyncInterval:   2 * time.Minute,
		Horizon:        24 * time.Hour,
		NotifyPath:     "noctalia",
		CalendarFilter: "primary",
		Calendars:      nil,
	}
}

type Paths struct {
	ConfigDir  string
	DataDir    string
	RuntimeDir string
	ConfigFile string
	TokenFile  string
	AlertsFile string
	SocketPath string
}

func ResolvePaths() (Paths, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		configHome = filepath.Join(home, ".config")
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = filepath.Join(os.TempDir(), fmt.Sprintf("meetbar-%d", os.Getuid()))
	}

	configDir := filepath.Join(configHome, appName)
	dataDir := filepath.Join(dataHome, appName)
	rt := filepath.Join(runtimeDir, appName)

	return Paths{
		ConfigDir:  configDir,
		DataDir:    dataDir,
		RuntimeDir: rt,
		ConfigFile: filepath.Join(configDir, "config.toml"),
		TokenFile:  filepath.Join(dataDir, "token.json"),
		AlertsFile: filepath.Join(dataDir, "alerts.json"),
		SocketPath: filepath.Join(rt, "meetbar.sock"),
	}, nil
}

func Load(path string) (Config, error) {
	cfg := Defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	type fileCfg struct {
		WarnMinutes    []int    `toml:"warn_minutes"`
		SyncInterval   string   `toml:"sync_interval"`
		Horizon        string   `toml:"horizon"`
		NotifyPath     string   `toml:"notify_path"`
		CalendarFilter string   `toml:"calendar_filter"`
		Calendars      []string `toml:"calendars"`
	}
	var raw fileCfg
	if _, err := toml.Decode(string(data), &raw); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}
	if len(raw.WarnMinutes) > 0 {
		cfg.WarnMinutes = raw.WarnMinutes
	}
	if raw.SyncInterval != "" {
		d, err := time.ParseDuration(raw.SyncInterval)
		if err != nil {
			return cfg, fmt.Errorf("sync_interval: %w", err)
		}
		cfg.SyncInterval = d
	}
	if raw.Horizon != "" {
		d, err := time.ParseDuration(raw.Horizon)
		if err != nil {
			return cfg, fmt.Errorf("horizon: %w", err)
		}
		cfg.Horizon = d
	}
	if raw.NotifyPath != "" {
		cfg.NotifyPath = raw.NotifyPath
	}
	if raw.CalendarFilter != "" {
		cfg.CalendarFilter = raw.CalendarFilter
	}
	if raw.Calendars != nil {
		cfg.Calendars = raw.Calendars
	}
	return cfg, nil
}

func (c Config) DaemonNotify() bool {
	return c.NotifyPath == "daemon" || c.NotifyPath == "both"
}

func EnsureDir(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
