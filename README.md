# Meetbar

Google Calendar meeting reminders for [Noctalia](https://noctalia.dev) shell, backed by a small Go daemon.

- **Daemon** (`meetbard`) OAuth, Calendar API sync, alert thresholds, Unix socket IPC  
- **CLI** (`meetbarctl`) status, login, upcoming, sync, alerts  
- **Plugin** bar countdown, agenda panel, Noctalia notifications  

## Install (daemon + CLI)

```bash
go build -o ~/.local/bin/meetbarctl ./cmd/meetbarctl
go build -o ~/.local/bin/meetbard ./cmd/meetbard

mkdir -p ~/.config/systemd/user
cp systemd/meetbard.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now meetbard
```

### Google OAuth credentials

Create a Desktop OAuth client in Google Cloud (enable Calendar API), then either:

```bash
mkdir -p ~/.config/meetbar
cp credentials.example.json ~/.config/meetbar/credentials.json
# edit client_id / client_secret
```

or build with embedded credentials:

```bash
go build -ldflags "-X github.com/alex/meetbar/internal/auth.embeddedClientID=... -X github.com/alex/meetbar/internal/auth.embeddedClientSecret=..." \
  -o ~/.local/bin/meetbarctl ./cmd/meetbarctl
```

Env overrides: `MEETBAR_CLIENT_ID`, `MEETBAR_CLIENT_SECRET`, `MEETBAR_CREDENTIALS`.

### Login

```bash
meetbarctl login
meetbarctl upcoming
meetbarctl status --json
```

## Config

`~/.config/meetbar/config.toml` (all optional):

```toml
warn_minutes     = [15, 5, 0]
sync_interval    = "2m"
horizon          = "24h"
notify_path      = "noctalia"   # noctalia | daemon | both
calendar_filter  = "primary"    # primary | owned | all
# calendars      = ["primary"]  # optional explicit IDs; ["*"] = all
```

`calendar_filter` default is **primary** (only your main calendar). Use `owned` for calendars you own/write, or `all` for every subscribed calendar (holidays, shared, etc.).

- Token: `~/.local/share/meetbar/token.json` (`0600`)  
- Socket: `$XDG_RUNTIME_DIR/meetbar/meetbar.sock`

## Noctalia plugin

The plugin ships two builds:

| Path | Shell |
| --- | --- |
| `plugin/v4/` | Noctalia v3/v4 (QML + `manifest.json`) |
| `plugin/v5/` | Noctalia v5 (Luau + `plugin.toml`) |

### v4 (current Noctalia / Quickshell)

```bash
mkdir -p "$HOME/.config/noctalia/plugins"
ln -sfn "$(pwd)/plugin/v4" "$HOME/.config/noctalia/plugins/meetbar"
```

Enable **Meetbar** in Noctalia settings, place the bar widget, open the panel to connect (or use `meetbarctl login`).

### v5

```bash
mkdir -p "${XDG_DATA_HOME:-$HOME/.local/share}/noctalia/plugins"
ln -sfn "$(pwd)/plugin/v5" "${XDG_DATA_HOME:-$HOME/.local/share}/noctalia/plugins/meetbar"
```

## CLI

```
meetbarctl serve                 # foreground daemon
meetbarctl status [--json]
meetbarctl login | logout
meetbarctl upcoming [--hours 12]
meetbarctl sync                      # force Google Calendar pull
meetbarctl alerts poll | ack <key>...
meetbarctl calendars
```

## Development

```bash
go test ./...
go run ./cmd/meetbarctl serve
```
