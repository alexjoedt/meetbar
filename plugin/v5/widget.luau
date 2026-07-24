--!nonstrict
-- Meetbar bar widget: next meeting countdown

local status = { auth = "offline" }
local nextEv = nil

local function cfg(key)
  return noctalia.getConfig(key)
end

local function countdownLabel(ev)
  if type(ev) ~= "table" then
    return nil
  end
  local title = ev.title or "Meeting"
  if #title > 28 then
    title = title:sub(1, 27) .. "…"
  end
  local mins = ev.minutes_until
  if type(mins) == "number" then
    if mins <= 0 then
      return title .. " · now"
    end
    if mins < 60 then
      return title .. " · " .. tostring(mins) .. "m"
    end
    local h = math.floor(mins / 60)
    local m = mins % 60
    return title .. " · " .. tostring(h) .. "h" .. tostring(m) .. "m"
  end
  local startStr = ev.start or ""
  local clock = startStr:match("T(%d+:%d+)")
  if clock then
    return title .. " · " .. clock
  end
  return title
end

local function render()
  barWidget.setGlyph("calendar")
  if status.auth == "offline" or status.daemon == "down" then
    barWidget.setText("…")
    return
  end
  if status.auth ~= "connected" then
    barWidget.setText(noctalia.tr("ui.disconnected"))
    return
  end
  local label = countdownLabel(nextEv)
  if label then
    barWidget.setText(label)
    return
  end
  if cfg("show_idle") == false then
    barWidget.setText("")
    return
  end
  local idle = cfg("idle_text")
  if type(idle) ~= "string" or idle == "" then
    idle = noctalia.tr("ui.no_meetings")
  end
  barWidget.setText(idle)
end

noctalia.state.watch("status", function(v)
  if type(v) == "table" then
    status = v
  end
  render()
end)

noctalia.state.watch("next", function(v)
  nextEv = v
  render()
end)

function onClick()
  noctalia.togglePanel("alex/meetbar:panel")
end

function update()
  noctalia.setUpdateInterval(30000)
  render()
end

render()
