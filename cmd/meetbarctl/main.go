package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alex/meetbar/internal/config"
	"github.com/alex/meetbar/internal/daemon"
	"github.com/alex/meetbar/internal/ipc"
	"log/slog"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	jsonOut := false
	args := os.Args[1:]
	filtered := args[:0]
	for _, a := range args {
		if a == "--json" {
			jsonOut = true
			continue
		}
		filtered = append(filtered, a)
	}
	args = filtered
	if len(args) == 0 {
		usage()
		os.Exit(2)
	}

	cmd := args[0]
	if cmd == "serve" {
		runServe()
		return
	}

	paths, err := config.ResolvePaths()
	if err != nil {
		fatal(err)
	}
	client := ipc.NewClient(paths.SocketPath)

	switch cmd {
	case "status":
		res, err := ipc.CallDecode[ipc.StatusResult](client, "status", nil)
		if err != nil {
			fatal(err)
		}
		printResult(jsonOut, res, func() {
			fmt.Printf("daemon: %s\n", res.Daemon)
			fmt.Printf("auth:   %s\n", res.Auth)
			if res.Email != "" {
				fmt.Printf("email:  %s\n", res.Email)
			}
			if res.LastSync != "" {
				fmt.Printf("sync:   %s\n", res.LastSync)
			}
			if res.SyncErr != "" {
				fmt.Printf("error:  %s\n", res.SyncErr)
			}
		})
	case "login":
		res, err := ipc.CallDecode[ipc.LoginResult](client, "auth.login", nil)
		if err != nil {
			fatal(err)
		}
		printResult(jsonOut, res, func() {
			fmt.Printf("connected as %s\n", res.Email)
		})
	case "logout":
		res, err := client.Call("auth.logout", nil)
		if err != nil {
			fatal(err)
		}
		printResult(jsonOut, jsonRaw(res), func() {
			fmt.Println("logged out")
		})
	case "upcoming":
		hours := 12
		for i := 1; i < len(args); i++ {
			if args[i] == "--hours" && i+1 < len(args) {
				h, err := strconv.Atoi(args[i+1])
				if err != nil {
					fatal(fmt.Errorf("invalid --hours"))
				}
				hours = h
				i++
			}
		}
		res, err := ipc.CallDecode[ipc.UpcomingResult](client, "upcoming", ipc.UpcomingParams{Hours: hours})
		if err != nil {
			fatal(err)
		}
		printResult(jsonOut, res, func() {
			if len(res.Events) == 0 {
				fmt.Println("no upcoming events")
				return
			}
			for _, ev := range res.Events {
				start := ev.Start
				if t, err := time.Parse(time.RFC3339, ev.Start); err == nil {
					start = t.Local().Format("Mon 15:04")
				}
				line := fmt.Sprintf("%s  %s", start, ev.Title)
				if ev.JoinURL != "" {
					line += "  " + ev.JoinURL
				}
				fmt.Println(line)
			}
		})
	case "alerts":
		if len(args) < 2 {
			fatal(fmt.Errorf("usage: meetbarctl alerts poll|ack"))
		}
		switch args[1] {
		case "poll":
			res, err := ipc.CallDecode[ipc.AlertsPollResult](client, "alerts.poll", nil)
			if err != nil {
				fatal(err)
			}
			printResult(jsonOut, res, func() {
				if len(res.Alerts) == 0 {
					fmt.Println("no due alerts")
					return
				}
				for _, a := range res.Alerts {
					fmt.Printf("%s  %s (%dm)\n", a.Key, a.Title, a.ThresholdMin)
				}
			})
		case "ack":
			keys := args[2:]
			if len(keys) == 0 {
				fatal(fmt.Errorf("usage: meetbarctl alerts ack <key>..."))
			}
			res, err := client.Call("alerts.ack", ipc.AlertsAckParams{Keys: keys})
			if err != nil {
				fatal(err)
			}
			printResult(jsonOut, jsonRaw(res), func() {
				fmt.Printf("acked %d\n", len(keys))
			})
		default:
			fatal(fmt.Errorf("unknown alerts subcommand %q", args[1]))
		}
	case "calendars":
		res, err := ipc.CallDecode[ipc.CalendarsResult](client, "calendars.list", nil)
		if err != nil {
			fatal(err)
		}
		printResult(jsonOut, res, func() {
			for _, c := range res.Calendars {
				mark := " "
				if c.Primary {
					mark = "*"
				}
				fmt.Printf("%s %s  %s\n", mark, c.ID, c.Summary)
			}
		})
	case "sync":
		res, err := ipc.CallDecode[ipc.SyncResult](client, "sync", nil)
		if err != nil {
			fatal(err)
		}
		printResult(jsonOut, res, func() {
			when := res.LastSync
			if t, err := time.Parse(time.RFC3339, res.LastSync); err == nil {
				when = t.Local().Format(time.RFC3339)
			}
			if when == "" {
				when = "ok"
			}
			fmt.Printf("synced %d events (%s)\n", res.Events, when)
		})
	case "join":
		noInteractive := false
		for _, a := range args[1:] {
			if a == "--no-interactive" {
				noInteractive = true
			}
		}

		res, err := ipc.CallDecode[ipc.UpcomingResult](client, "upcoming", ipc.UpcomingParams{Hours: 1})
		if err != nil {
			fatal(err)
		}

		var running []ipc.Event
		for _, ev := range res.Events {
			if ev.MinutesUntil != nil && *ev.MinutesUntil <= 0 {
				running = append(running, ev)
			}
		}

		if len(running) == 0 {
			fmt.Fprintln(os.Stderr, "error: no meeting is currently running")
			os.Exit(1)
		}

		var chosen ipc.Event
		if len(running) == 1 {
			chosen = running[0]
		} else {
			if noInteractive {
				fmt.Fprintln(os.Stderr, "error: multiple meetings are running; use interactive mode to select")
				os.Exit(1)
			}
			fmt.Fprintln(os.Stderr, "Multiple meetings are running. Pick one:")
			for i, ev := range running {
				start := ev.Start
				if t, err := time.Parse(time.RFC3339, ev.Start); err == nil {
					start = t.Local().Format("15:04")
				}
				fmt.Fprintf(os.Stderr, "  %d) %s  %s\n", i+1, start, ev.Title)
			}
			fmt.Fprintf(os.Stderr, "Choice [1-%d]: ", len(running))
			var choice int
			if _, err := fmt.Fscan(os.Stdin, &choice); err != nil || choice < 1 || choice > len(running) {
				fmt.Fprintln(os.Stderr, "error: invalid choice")
				os.Exit(1)
			}
			chosen = running[choice-1]
		}

		if chosen.JoinURL == "" {
			fmt.Fprintln(os.Stderr, "error: no join URL for this meeting")
			os.Exit(1)
		}

		if err := exec.Command("xdg-open", chosen.JoinURL).Start(); err != nil {
			fatal(fmt.Errorf("xdg-open: %w", err))
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runServe() {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx, log); err != nil {
		log.Error("failed", "err", err)
		os.Exit(1)
	}
}

func printResult(asJSON bool, v any, human func()) {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(v)
		return
	}
	human()
}

func jsonRaw(raw json.RawMessage) any {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return map[string]string{"raw": string(raw)}
	}
	return v
}

func fatal(err error) {
	msg := err.Error()
	if strings.Contains(msg, "daemon offline") {
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
		fmt.Fprintf(os.Stderr, "hint: start with `meetbarctl serve` or enable systemd user unit meetbard\n")
		if _, err := exec.LookPath("systemctl"); err == nil {
			_ = exec.Command("systemctl", "--user", "start", "meetbard").Run()
		}
	} else {
		fmt.Fprintf(os.Stderr, "error: %s\n", msg)
	}
	os.Exit(1)
}

func usage() {
	fmt.Fprintf(os.Stderr, `meetbarctl — Google Calendar meeting assistant

Usage:
  meetbarctl serve
  meetbarctl status [--json]
  meetbarctl login [--json]
  meetbarctl logout [--json]
  meetbarctl upcoming [--hours N] [--json]
  meetbarctl sync [--json]
  meetbarctl alerts poll [--json]
  meetbarctl alerts ack <key>... [--json]
  meetbarctl calendars [--json]
  meetbarctl join [--no-interactive]
`)
}
