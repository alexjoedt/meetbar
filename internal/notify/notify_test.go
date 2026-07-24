package notify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFormatAlert(t *testing.T) {
	tests := []struct {
		name         string
		thresholdMin int
		title        string
		joinURL      string
		wantTitle    string
		wantBody     string
	}{
		{
			name:         "zero threshold means starting now",
			thresholdMin: 0,
			title:        "Standup",
			wantTitle:    "Standup",
			wantBody:     "Starting now",
		},
		{
			name:         "negative threshold treated as starting",
			thresholdMin: -1,
			title:        "Standup",
			wantTitle:    "Standup",
			wantBody:     "Starting now",
		},
		{
			name:         "one minute uses singular wording",
			thresholdMin: 1,
			title:        "Standup",
			wantTitle:    "Standup",
			wantBody:     "Starts in 1 minute",
		},
		{
			name:         "plural minutes",
			thresholdMin: 15,
			title:        "Standup",
			wantTitle:    "Standup",
			wantBody:     "Starts in 15 minutes",
		},
		{
			name:         "join url is not embedded in body",
			thresholdMin: 5,
			title:        "Standup",
			joinURL:      "https://meet.google.com/abc-defg-hij",
			wantTitle:    "Standup",
			wantBody:     "Starts in 5 minutes",
		},
		{
			name:         "empty title falls back to Meeting",
			thresholdMin: 5,
			title:        "",
			wantTitle:    "Meeting",
			wantBody:     "Starts in 5 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTitle, gotBody := FormatAlert(tt.thresholdMin, tt.title, tt.joinURL)
			if gotTitle != tt.wantTitle {
				t.Errorf("title = %q, want %q", gotTitle, tt.wantTitle)
			}
			if gotBody != tt.wantBody {
				t.Errorf("body = %q, want %q", gotBody, tt.wantBody)
			}
			if strings.Contains(gotBody, "http") {
				t.Errorf("body %q must not embed a URL", gotBody)
			}
		})
	}
}

func TestDesktopMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := Desktop(t.Context(), "title", "body", ""); err == nil {
		t.Fatal("Desktop() error = nil, want error when notify-send is missing")
	}
}

func TestDesktopRunsConfiguredBinary(t *testing.T) {
	prependFakeNotifySendToPATH(t, "#!/bin/sh\nexit 0\n")

	if err := Desktop(t.Context(), "title", "body", ""); err != nil {
		t.Fatalf("Desktop() unexpected error: %v", err)
	}
}

func TestDesktopPropagatesBinaryFailure(t *testing.T) {
	prependFakeNotifySendToPATH(t, "#!/bin/sh\nexit 1\n")

	if err := Desktop(t.Context(), "title", "body", ""); err == nil {
		t.Fatal("Desktop() error = nil, want error when notify-send exits non-zero")
	}
}

func TestDesktopRespectsTimeout(t *testing.T) {
	prependFakeNotifySendToPATH(t, "#!/bin/sh\nsleep 5\nexit 0\n")

	orig := sendTimeout
	sendTimeout = 50 * time.Millisecond
	t.Cleanup(func() { sendTimeout = orig })

	start := time.Now()
	err := Desktop(t.Context(), "title", "body", "")
	if err == nil {
		t.Fatal("Desktop() error = nil, want timeout error")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("Desktop() took %v, want it to be cut short by sendTimeout", elapsed)
	}
}

func TestDesktopWithActionOpensURLOnJoin(t *testing.T) {
	prependFakeNotifySendToPATH(t, "#!/bin/sh\necho join\nexit 0\n")

	var (
		mu      sync.Mutex
		opened  string
		openedN int
	)
	origOpen := openURL
	openURL = func(url string) error {
		mu.Lock()
		defer mu.Unlock()
		opened = url
		openedN++
		return nil
	}
	t.Cleanup(func() { openURL = origOpen })

	const wantURL = "https://meet.google.com/abc-defg-hij"
	if err := Desktop(t.Context(), "Standup", "Starts in 5 minutes", wantURL); err != nil {
		t.Fatalf("Desktop() unexpected error: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := openedN
		got := opened
		mu.Unlock()
		if n > 0 {
			if got != wantURL {
				t.Fatalf("opened URL = %q, want %q", got, wantURL)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for action handler to open URL")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDesktopWithActionIgnoresDismiss(t *testing.T) {
	prependFakeNotifySendToPATH(t, "#!/bin/sh\necho\nexit 0\n")

	var openedN int
	origOpen := openURL
	openURL = func(url string) error {
		openedN++
		return nil
	}
	t.Cleanup(func() { openURL = origOpen })

	if err := Desktop(t.Context(), "Standup", "Starts in 5 minutes", "https://example.com"); err != nil {
		t.Fatalf("Desktop() unexpected error: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	if openedN != 0 {
		t.Fatalf("openURL called %d times, want 0 on dismiss", openedN)
	}
}

// prependFakeNotifySendToPATH installs a fake notify-send script ahead of
// the real PATH (rather than replacing PATH outright) so scripts that shell
// out to real tools like `sleep` keep working.
func prependFakeNotifySendToPATH(t *testing.T, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake notify-send script requires a POSIX shell")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "notify-send")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
