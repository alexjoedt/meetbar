--!nonstrict
-- Meetbar panel: agenda + connect/disconnect

local status = { auth = "offline", email = "", error = "" }
local events = {}
local busy = false

local function cfg(key)
  return noctalia.getConfig(key)
end

local function cli()
  local p = cfg("cli_path")
  if type(p) == "string" and p ~= "" then
    return p
  end
  return "meetbarctl"
end

local function runCli(args, onDone)
  local cmd = cli()
  for _, a in ipairs(args) do
    cmd ..= " " .. a
  end
  noctalia.runAsync(cmd, function(res)
    if not res or res.exitCode ~= 0 then
      local err = (res and res.stderr) or "failed"
      onDone(nil, err)
      return
    end
    local decoded = nil
    if type(res.stdout) == "string" and res.stdout ~= "" then
      decoded = noctalia.json.decode(res.stdout)
    end
    onDone(decoded, nil)
  end, 120000)
end

local function formatWhen(startStr)
  if type(startStr) ~= "string" then
    return ""
  end
  local h, mi = startStr:match("T(%d+):(%d+)")
  if h then
    return h .. ":" .. mi
  end
  return startStr
end

local function render()
  local children = {}

  table.insert(children, ui.label({
    text = noctalia.tr("title"),
    fontWeight = "bold",
    color = "on_surface",
  }))

  if status.daemon == "down" or status.auth == "offline" then
    table.insert(children, ui.label({
      text = noctalia.tr("ui.offline"),
      color = "error",
    }))
    if type(status.error) == "string" and status.error ~= "" then
      table.insert(children, ui.label({
        text = status.error,
        color = "on_surface_variant",
      }))
    end
  elseif status.auth ~= "connected" then
    table.insert(children, ui.label({
      text = noctalia.tr("ui.disconnected"),
      color = "on_surface_variant",
    }))
    table.insert(children, ui.button({
      text = noctalia.tr("ui.connect"),
      onClick = "onConnect",
      disabled = busy,
    }))
  else
    local email = status.email or ""
    table.insert(children, ui.label({
      text = email,
      color = "on_surface_variant",
    }))

    if type(events) ~= "table" or #events == 0 then
      table.insert(children, ui.label({
        text = noctalia.tr("ui.no_meetings"),
        color = "on_surface_variant",
      }))
    else
      for i, ev in ipairs(events) do
        if i > 12 then
          break
        end
        local when = formatWhen(ev.start)
        local title = ev.title or "Meeting"
        local rowKids = {
          ui.column({ gap = 2, flexGrow = 1 }, {
            ui.label({ text = when .. "  " .. title, color = "on_surface" }),
          }),
        }
        if type(ev.join_url) == "string" and ev.join_url ~= "" then
          table.insert(rowKids, ui.button({
            text = noctalia.tr("ui.join"),
            onClick = "onJoin" .. tostring(i),
          }))
        end
        table.insert(children, ui.row({ gap = 8, align = "center" }, rowKids))
      end
    end

    table.insert(children, ui.row({ gap = 8 }, {
      ui.button({ text = noctalia.tr("ui.refresh"), onClick = "onRefresh", disabled = busy }),
      ui.button({ text = noctalia.tr("ui.disconnect"), onClick = "onDisconnect", disabled = busy }),
    }))
  end

  panel.render(ui.column({ gap = 12, flexGrow = 1, padding = 16 }, children))
end

-- Dynamic join handlers for first 12 events
local function shellQuote(s)
  return "'" .. tostring(s):gsub("'", "'\\''") .. "'"
end

for i = 1, 12 do
  _G["onJoin" .. tostring(i)] = function()
    local ev = events[i]
    if type(ev) == "table" and type(ev.join_url) == "string" and ev.join_url ~= "" then
      noctalia.runAsync("xdg-open " .. shellQuote(ev.join_url))
    end
  end
end

function onConnect()
  busy = true
  render()
  runCli({ "login", "--json" }, function(res, err)
    busy = false
    if err ~= nil then
      noctalia.notifyError(noctalia.tr("title"), err)
      render()
      return
    end
    noctalia.state.set("cmd", "refresh")
    noctalia.notify(noctalia.tr("title"), "Connected")
    render()
  end)
end

function onDisconnect()
  busy = true
  render()
  runCli({ "logout", "--json" }, function(_, err)
    busy = false
    if err ~= nil then
      noctalia.notifyError(noctalia.tr("title"), err)
    end
    noctalia.state.set("status", { auth = "disconnected", daemon = "ok", email = "" })
    noctalia.state.set("events", {})
    noctalia.state.set("next", nil)
    render()
  end)
end

function onRefresh()
  busy = true
  render()
  noctalia.state.set("service_cmd", "refresh")
  runCli({ "sync", "--json" }, function(_, err)
    if err ~= nil then
      noctalia.notifyError(noctalia.tr("title"), err)
    end
    noctalia.runAsync(cli() .. " upcoming --hours " .. tostring(cfg("horizon_hours") or 12) .. " --json", function(res)
      busy = false
      if res and res.exitCode == 0 and type(res.stdout) == "string" then
        local decoded = noctalia.json.decode(res.stdout)
        if type(decoded) == "table" and type(decoded.events) == "table" then
          events = decoded.events
          noctalia.state.set("events", events)
          if #events > 0 then
            noctalia.state.set("next", events[1])
          else
            noctalia.state.set("next", nil)
          end
        end
      end
      render()
    end)
  end)
end

function onOpen()
  local st = noctalia.state.get("status")
  if type(st) == "table" then
    status = st
  end
  local ev = noctalia.state.get("events")
  if type(ev) == "table" then
    events = ev
  end
  render()
end

noctalia.state.watch("status", function(v)
  if type(v) == "table" then
    status = v
    render()
  end
end)

noctalia.state.watch("events", function(v)
  if type(v) == "table" then
    events = v
    render()
  end
end)

function onClose() end
