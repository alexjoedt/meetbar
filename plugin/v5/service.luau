--!nonstrict
-- Meetbar service: poll CLI, fire Noctalia notifications, publish state

local tick = 0
local pollEvery = 30
local inFlight = false
local startCooldown = 0
local notified = {} -- key -> true for this process lifetime

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

local function notifyPath()
  local p = cfg("notify_path")
  if type(p) == "string" and p ~= "" then
    return p
  end
  return "noctalia"
end

local function wantsNoctaliaNotify()
  local p = notifyPath()
  return p == "noctalia" or p == "both"
end

local function horizonHours()
  local h = cfg("horizon_hours")
  if type(h) == "number" and h > 0 then
    return math.floor(h)
  end
  return 12
end

local function setPollInterval()
  local sec = cfg("poll_seconds")
  if type(sec) ~= "number" or sec < 5 then
    sec = 30
  end
  pollEvery = math.floor(sec)
  -- tick every 1s for cooldowns
  noctalia.setUpdateInterval(1000)
end

local function publish(status, events, nextEv)
  noctalia.state.set("status", status or {})
  noctalia.state.set("events", events or {})
  noctalia.state.set("next", nextEv)
end

local function runCli(args, onDone)
  local cmd = cli()
  for _, a in ipairs(args) do
    cmd ..= " " .. a
  end
  noctalia.runAsync(cmd, function(res)
    if not res or res.timedOut then
      onDone(nil, "timeout")
      return
    end
    if res.exitCode ~= 0 then
      local err = res.stderr
      if type(err) ~= "string" or err == "" then
        err = res.stdout
      end
      if type(err) ~= "string" or err == "" then
        err = "exit " .. tostring(res.exitCode)
      end
      onDone(nil, err)
      return
    end
    local body = res.stdout
    if type(body) ~= "string" or body == "" then
      onDone(nil, "empty output")
      return
    end
    local decoded, err = noctalia.json.decode(body)
    if err ~= nil or type(decoded) ~= "table" then
      onDone(nil, "invalid json")
      return
    end
    onDone(decoded, nil)
  end, 120000)
end

local function tryStartDaemon()
  if startCooldown > 0 then
    return
  end
  startCooldown = 15
  noctalia.runAsync("systemctl --user start meetbard >/dev/null 2>&1 || true")
end

local function shellQuote(s)
  return "'" .. tostring(s):gsub("'", "'\\''") .. "'"
end

local function formatNotify(threshold, title)
  local meeting = title
  if type(meeting) ~= "string" or meeting == "" then
    meeting = noctalia.tr("notify.meeting")
  end
  local urgency
  if threshold == nil or threshold <= 0 then
    urgency = noctalia.tr("notify.starting")
  elseif threshold == 1 then
    urgency = noctalia.tr("notify.in_one")
  else
    urgency = noctalia.tr("notify.in_n", { n = tostring(threshold) })
  end
  return meeting, urgency
end

-- v5 noctalia.notify has no action buttons. When a join URL is present, use
-- notify-send with a Join action so the meeting is one click away.
local function notifyMeeting(title, body, joinURL)
  if type(joinURL) == "string" and joinURL ~= "" then
    local cmd = "action=$(notify-send --app-name=Meetbar --urgency=critical"
      .. " --expire-time=15000 --wait --action=join="
      .. shellQuote(noctalia.tr("notify.join"))
      .. " " .. shellQuote(title)
      .. " " .. shellQuote(body)
      .. "); [ \"$action\" = join ] && xdg-open " .. shellQuote(joinURL)
    noctalia.runAsync(cmd)
    return
  end
  noctalia.notify(title, body)
end

local function handleAlerts(alerts)
  if type(alerts) ~= "table" then
    return
  end
  local keys = {}
  for _, a in ipairs(alerts) do
    if type(a) == "table" and type(a.key) == "string" then
      if not notified[a.key] then
        if wantsNoctaliaNotify() then
          local title, body = formatNotify(a.threshold_min, a.title or "")
          notifyMeeting(title, body, a.join_url)
        end
        notified[a.key] = true
        table.insert(keys, a.key)
      end
    end
  end
  if #keys == 0 then
    return
  end
  local parts = { "alerts", "ack" }
  for _, k in ipairs(keys) do
    table.insert(parts, k)
  end
  table.insert(parts, "--json")
  runCli(parts, function() end)
end

local function pickNext(events)
  if type(events) ~= "table" or #events == 0 then
    return nil
  end
  return events[1]
end

local function poll()
  if inFlight then
    return
  end
  inFlight = true

  runCli({ "status", "--json" }, function(status, err)
    if err ~= nil then
      inFlight = false
      publish({
        auth = "offline",
        daemon = "down",
        error = err,
      }, {}, nil)
      tryStartDaemon()
      return
    end

    local hours = tostring(horizonHours())
    runCli({ "upcoming", "--hours", hours, "--json" }, function(up, upErr)
      local events = {}
      if upErr == nil and type(up) == "table" and type(up.events) == "table" then
        events = up.events
      end

      local nextEv = pickNext(events)
      local st = {
        auth = status.auth or "disconnected",
        email = status.email or "",
        daemon = status.daemon or "ok",
        last_sync = status.last_sync or "",
        sync_error = status.sync_error or "",
        error = "",
      }
      publish(st, events, nextEv)

      if st.auth == "connected" then
        runCli({ "alerts", "poll", "--json" }, function(al, alErr)
          inFlight = false
          if alErr == nil and type(al) == "table" then
            handleAlerts(al.alerts)
          end
        end)
      else
        inFlight = false
      end
    end)
  end)
end

function update()
  setPollInterval()
  if startCooldown > 0 then
    startCooldown -= 1
  end
  tick += 1
  if tick >= pollEvery then
    tick = 0
    poll()
  end
end

function onConfigChanged()
  setPollInterval()
  tick = pollEvery
end

local function forceSyncThenPoll()
  runCli({ "sync", "--json" }, function(_, err)
    if err ~= nil then
      noctalia.notifyError(noctalia.tr("title"), err)
    end
    tick = pollEvery
    poll()
  end)
end

function onIpc(event, payload)
  if event == "refresh" then
    forceSyncThenPoll()
  elseif event == "login" then
    runCli({ "login", "--json" }, function(res, err)
      if err ~= nil then
        noctalia.notifyError(noctalia.tr("title"), err)
        return
      end
      local email = ""
      if type(res) == "table" and type(res.email) == "string" then
        email = res.email
      end
      noctalia.notify(noctalia.tr("title"), "Connected " .. email)
      tick = pollEvery
    end)
  elseif event == "logout" then
    runCli({ "logout", "--json" }, function(_, err)
      if err ~= nil then
        noctalia.notifyError(noctalia.tr("title"), err)
        return
      end
      tick = pollEvery
    end)
  end
end

noctalia.state.watch("service_cmd", function(cmd)
  if cmd == "refresh" then
    forceSyncThenPoll()
  end
end)

-- seed
publish({ auth = "offline", daemon = "down", error = "" }, {}, nil)
setPollInterval()
tick = pollEvery
