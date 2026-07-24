# AGENTS.md — meetbar

Guidelines for AI coding agents working in this repository.

---

## Project Overview

**meetbar** is Google Calendar meeting reminders for [Noctalia](https://noctalia.dev), backed by a small Go daemon.

| Piece | Role |
| --- | --- |
| `meetbard` | Long-running daemon: OAuth, Calendar sync, alert thresholds, UDS IPC |
| `meetbarctl` | Control CLI: status, login, upcoming, sync, alerts, calendars; also `serve` for foreground daemon |
| `plugin/v4` | Noctalia v3/v4 bar widget + panel (QML + `manifest.json`) |
| `plugin/v5` | Noctalia v5 bar widget + panel (Luau + `plugin.toml`) |

Module: `github.com/alex/meetbar` (Go 1.26+).

### Layout

```
cmd/
  meetbarctl/     # CLI entry (+ serve)
  meetbard/       # Daemon entry
internal/
  auth/           # OAuth PKCE, credentials resolve, token store
  calendar/       # Google Calendar sync, join URL parse
  alert/          # Threshold keys + ack state
  config/         # XDG paths + TOML config
  daemon/         # App loop, IPC handlers, notify path
  ipc/            # UDS JSON request/response protocol + client/server
  notify/         # notify-send helper
plugin/
  v4/             # Noctalia Quickshell (QML)
  v5/             # Noctalia Luau
systemd/          # User unit for meetbard
```

Keep logic in `internal/`. `cmd/*` stays thin: flags, wiring, process lifecycle.

---

## Build & Development

```sh
make build          # bin/meetbarctl, bin/meetbard
make test           # go test ./...
make install        # install binaries + user systemd unit
make serve          # build + foreground daemon via meetbarctl serve
```

Or directly:

```sh
go build -o bin/meetbarctl ./cmd/meetbarctl
go build -o bin/meetbard ./cmd/meetbard
go test ./...
go test ./internal/alert/ -run TestName -v
```

Embedded OAuth (optional):

```sh
go build -ldflags "-X github.com/alex/meetbar/internal/auth.embeddedClientID=... -X github.com/alex/meetbar/internal/auth.embeddedClientSecret=..." \
  -o bin/meetbarctl ./cmd/meetbarctl
```

Run `make test` (or `go test ./...`) after Go changes. Fix failures before finishing.

---

## Runtime paths

| What | Path |
| --- | --- |
| Config | `~/.config/meetbar/config.toml` |
| Credentials | `~/.config/meetbar/credentials.json` (or env / ldflags) |
| Token | `~/.local/share/meetbar/token.json` (`0600`) |
| Socket | `$XDG_RUNTIME_DIR/meetbar/meetbar.sock` |

Env overrides: `MEETBAR_CLIENT_ID`, `MEETBAR_CLIENT_SECRET`, `MEETBAR_CREDENTIALS`.

Config keys and defaults live in `config.example.toml` and `internal/config`.

---

## Architecture notes

- **IPC**: length-delimited JSON over a Unix domain socket. Types and methods live in `internal/ipc`. Add new RPC methods there first (protocol + client helpers), then wire handlers in `internal/daemon`.
- **Auth**: loopback OAuth with PKCE. Credential resolution order is env → file paths → ldflags embed. Token file must stay mode `0600`.
- **Sync**: `internal/calendar` polls on `sync_interval` within `horizon`. Calendar selection: explicit `calendars` list, else `calendar_filter` (`primary` \| `owned` \| `all`).
- **Alerts**: threshold minutes from config; keys are stable per event+threshold; ack is durable so restarts do not re-fire.
- **Notify path**: `noctalia` (default, plugin notifies + acks), `daemon` (`notify-send`), or `both` (daemon notifies without ack so the plugin can still notify+ack).
- **Concurrency**: daemon background loop and IPC handlers share auth/syncer state under `sync.RWMutex` in `internal/daemon`. Prefer small interfaces (`authManager`, `eventSyncer`) for test fakes.
- **Logging**: `log/slog` only.

---

## Coding conventions

### Go (required)

Apply idiomatic, effective Go on every change. Prefer clarity over cleverness. Match surrounding package style.

- Small packages under `internal/` with clear ownership; no new top-level packages without a strong reason.
- Accept interfaces, return concrete types. Define interfaces next to the consumer when they exist for testing (see `daemon`).
- Propagate `context.Context` through I/O, OAuth, Calendar API, and IPC handlers. Honor cancel and deadlines.
- Wrap errors with `%w` and useful context (`fmt.Errorf("sync calendars: %w", err)`). Handle each error once.
- No panics in library/daemon paths. Reserve `log.Fatal` / `os.Exit` for `main`.
- Table-driven tests beside the code (`*_test.go`). Prefer fakes/interfaces over hitting Google APIs.
- Use the standard library first. Existing deps: `BurntSushi/toml`, `golang.org/x/oauth2`, `google.golang.org/api`.
- `gofmt` / `goimports` clean. Exported symbols get short godoc sentences.
- Secrets: never log tokens, client secrets, or raw credential files. Keep token file permissions tight.
- Modern Go: use current language features already in the module (range-over-int, clearer `errors`, etc.) when they simplify code.

### Plugin

- **v4 (QML)**: current Noctalia/Quickshell install path. Poll `meetbarctl` for status/upcoming/alerts.
- **v5 (Luau)**: Noctalia v5 layout (`plugin.toml`, services/widgets). Keep behavior aligned with v4 when changing product features.
- When editing plugins, follow Noctalia plugin conventions for that major version.

### Commits

- Conventional, concise messages. No `Co-Authored-By` trailers.
- Do not commit secrets, real `credentials.json`, or token files.

---

## What to preserve

- Dual plugin trees (`plugin/v4` and `plugin/v5`); do not drop one when fixing the other.
- UDS + JSON IPC as the only daemon control plane (CLI and plugins both go through it).
- Single Google account; MeetingBar-style embedded OAuth with BYO override.
- XDG-based paths and the `meetbar` / `meetbard` / `meetbarctl` naming.
- Default notify path `noctalia` and default calendar filter `primary`.
