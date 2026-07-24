package notify

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// sendTimeout bounds how long we wait for a plain notify-send before giving up.
// Declared as a var (not const) so tests can shrink it.
var sendTimeout = 5 * time.Second

// actionWaitTimeout bounds how long we keep a clickable notification alive
// while waiting for the user to hit Join (or dismiss).
var actionWaitTimeout = 2 * time.Minute

// openURL launches url with xdg-open. Overridable in tests.
var openURL = func(url string) error {
	return exec.Command("xdg-open", url).Start()
}

// Desktop sends a desktop notification via notify-send, bounded by ctx and
// sendTimeout so a hung or missing binary can never block the caller.
// When actionURL is non-empty, a "Join meeting" action is offered; selecting
// it opens the URL. Action wait runs in the background so the caller returns
// as soon as the notification is posted.
func Desktop(ctx context.Context, title, body, actionURL string) error {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return fmt.Errorf("notify-send not found")
	}
	if actionURL == "" {
		return sendPlain(ctx, title, body)
	}
	return sendWithAction(title, body, actionURL)
}

func sendPlain(ctx context.Context, title, body string) error {
	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "notify-send",
		"--app-name=Meetbar",
		"--urgency=normal",
		"--expire-time=12000",
		title, body,
	)
	return cmd.Run()
}

func sendWithAction(title, body, actionURL string) error {
	// --wait keeps notify-send alive until the user acts or the notification
	// expires; run it off-thread so alert evaluation is never blocked.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), actionWaitTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "notify-send",
			"--app-name=Meetbar",
			"--urgency=critical",
			"--expire-time=15000",
			"--wait",
			"--action=join=Join meeting",
			title, body,
		)
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return
		}
		if strings.TrimSpace(stdout.String()) != "join" {
			return
		}
		_ = openURL(actionURL)
	}()
	return nil
}

// FormatAlert builds notification title/body for a meeting alert.
// Title is the meeting name; body is the urgency line. The join URL is not
// embedded in the body — callers surface it as a clickable action instead.
func FormatAlert(thresholdMin int, title, _ string) (string, string) {
	head := title
	if head == "" {
		head = "Meeting"
	}
	var body string
	switch {
	case thresholdMin <= 0:
		body = "Starting now"
	case thresholdMin == 1:
		body = "Starts in 1 minute"
	default:
		body = fmt.Sprintf("Starts in %d minutes", thresholdMin)
	}
	return head, body
}
