import QtQuick
import Quickshell
import Quickshell.Io
import qs.Commons
import qs.Services.UI

Item {
  id: root

  property var pluginApi: null

  property string auth: "offline"
  property string email: ""
  property string daemon: "down"
  property string lastSync: ""
  property string syncError: ""
  property string error: ""
  property var events: []
  property var nextEvent: null
  property bool busy: false
  property int settingsVersion: 0
  property string barLabel: "…"

  property var notifiedKeys: ({})
  property int startCooldown: 0
  property bool pollInFlight: false

  property var cfg: pluginApi?.pluginSettings || ({})
  property var defaults: pluginApi?.manifest?.metadata?.defaultSettings || ({})

  readonly property string notifyPath: cfg.notifyPath ?? defaults.notifyPath ?? "noctalia"
  readonly property int pollSeconds: Math.max(5, cfg.pollSeconds ?? defaults.pollSeconds ?? 30)
  readonly property string cliPath: {
    const p = cfg.cliPath ?? defaults.cliPath ?? "meetbarctl"
    return (p && p.length > 0) ? p : "meetbarctl"
  }
  readonly property int horizonHours: Math.max(1, cfg.horizonHours ?? defaults.horizonHours ?? 12)
  readonly property bool showIdle: cfg.showIdle ?? defaults.showIdle ?? true
  readonly property string idleText: cfg.idleText ?? defaults.idleText ?? "No meetings"
  readonly property bool overlayAlert: cfg.overlayAlert ?? defaults.overlayAlert ?? true

  readonly property bool wantsNoctaliaNotify: notifyPath === "noctalia" || notifyPath === "both"

  onPluginApiChanged: {
    if (pluginApi)
      settingsVersion++
  }

  onSettingsVersionChanged: {
    pollTimer.interval = pollSeconds * 1000
    refreshBarLabel()
    pollNow()
  }

  onAuthChanged: refreshBarLabel()
  onDaemonChanged: refreshBarLabel()
  onNextEventChanged: refreshBarLabel()
  onShowIdleChanged: refreshBarLabel()
  onIdleTextChanged: refreshBarLabel()

  Timer {
    id: pollTimer
    interval: root.pollSeconds * 1000
    repeat: true
    running: true
    triggeredOnStart: true
    onTriggered: root.pollNow()
  }

  Timer {
    id: cooldownTimer
    interval: 1000
    repeat: true
    running: root.startCooldown > 0
    onTriggered: {
      if (root.startCooldown > 0)
        root.startCooldown -= 1
    }
  }

  Loader {
    id: alertLoader
    active: false

    property var alertScreen: null
    property var pending: null

    sourceComponent: AlertOverlay {
      screen: alertLoader.alertScreen
      pluginApi: root.pluginApi
      onJoinRequested: url => root.openJoin(url)
      onClosed: alertLoader.active = false
    }

    onStatusChanged: {
      if (status === Loader.Ready && pending) {
        item.meetingTitle = pending.title
        item.joinUrl = pending.joinUrl
        pending = null
      }
    }
  }

  Process {
    id: statusProc
    running: false
    command: [root.cliPath, "status", "--json"]
    stdout: StdioCollector {
      id: statusOut
    }
    stderr: StdioCollector {
      id: statusErr
    }
    onExited: function (exitCode, exitStatus) {
      if (exitCode !== 0) {
        root.pollInFlight = false
        root.applyOffline(statusErr.text || statusOut.text || ("exit " + exitCode))
        root.tryStartDaemon()
        return
      }
      try {
        const st = JSON.parse(statusOut.text)
        root.auth = st.auth || "disconnected"
        root.email = st.email || ""
        root.daemon = st.daemon || "ok"
        root.lastSync = st.last_sync || ""
        root.syncError = st.sync_error || ""
        root.error = ""
      } catch (e) {
        root.pollInFlight = false
        root.applyOffline("invalid status json")
        return
      }
      upcomingProc.command = [root.cliPath, "upcoming", "--hours", String(root.horizonHours), "--json"]
      upcomingProc.running = true
    }
  }

  Process {
    id: upcomingProc
    running: false
    command: [root.cliPath, "upcoming", "--json"]
    stdout: StdioCollector {
      id: upcomingOut
    }
    stderr: StdioCollector {
      id: upcomingErr
    }
    onExited: function (exitCode, exitStatus) {
      let list = []
      if (exitCode === 0) {
        try {
          const up = JSON.parse(upcomingOut.text)
          if (up && up.events)
            list = up.events
        } catch (e) {
          Logger.w("Meetbar", "upcoming parse failed: " + e)
        }
      }
      root.events = list
      root.nextEvent = list.length > 0 ? list[0] : null

      if (root.auth === "connected") {
        alertsProc.command = [root.cliPath, "alerts", "poll", "--json"]
        alertsProc.running = true
      } else {
        root.pollInFlight = false
      }
    }
  }

  Process {
    id: alertsProc
    running: false
    command: [root.cliPath, "alerts", "poll", "--json"]
    stdout: StdioCollector {
      id: alertsOut
    }
    onExited: function (exitCode, exitStatus) {
      root.pollInFlight = false
      if (exitCode !== 0)
        return
      try {
        const al = JSON.parse(alertsOut.text)
        root.handleAlerts(al.alerts || [])
      } catch (e) {
        Logger.w("Meetbar", "alerts parse failed: " + e)
      }
    }
  }

  Process {
    id: ackProc
    running: false
    command: [root.cliPath, "alerts", "ack", "--json"]
    stdout: StdioCollector {}
  }

  Process {
    id: syncProc
    running: false
    command: [root.cliPath, "sync", "--json"]
    stdout: StdioCollector {
      id: syncOut
    }
    stderr: StdioCollector {
      id: syncErr
    }
    onExited: function (exitCode, exitStatus) {
      root.busy = false
      if (exitCode !== 0) {
        const msg = (syncErr.text || syncOut.text || "sync failed").toString().trim()
        if (msg.length > 0)
          ToastService.showError(pluginApi?.tr("toast.title") || "Meetbar", msg)
      }
      root.pollNow()
    }
  }

  Process {
    id: loginProc
    running: false
    command: [root.cliPath, "login", "--json"]
    stdout: StdioCollector {
      id: loginOut
    }
    stderr: StdioCollector {
      id: loginErr
    }
    onExited: function (exitCode, exitStatus) {
      root.busy = false
      if (exitCode !== 0) {
        ToastService.showError(pluginApi?.tr("toast.title") || "Meetbar", loginErr.text || loginOut.text || "login failed")
        return
      }
      try {
        const res = JSON.parse(loginOut.text)
        if (res && res.email)
          root.email = res.email
      } catch (e) {}
      ToastService.showNotice(pluginApi?.tr("toast.title") || "Meetbar", pluginApi?.tr("notify.connected") || "Connected", "calendar")
      root.pollNow()
    }
  }

  Process {
    id: logoutProc
    running: false
    command: [root.cliPath, "logout", "--json"]
    stdout: StdioCollector {
      id: logoutOut
    }
    stderr: StdioCollector {
      id: logoutErr
    }
    onExited: function (exitCode, exitStatus) {
      root.busy = false
      if (exitCode !== 0) {
        ToastService.showError(pluginApi?.tr("toast.title") || "Meetbar", logoutErr.text || logoutOut.text || "logout failed")
        return
      }
      root.auth = "disconnected"
      root.email = ""
      root.events = []
      root.nextEvent = null
    }
  }

  function applyOffline(err) {
    auth = "offline"
    daemon = "down"
    error = (err || "").toString().trim()
    events = []
    nextEvent = null
    refreshBarLabel()
  }

  function tryStartDaemon() {
    if (startCooldown > 0)
      return
    startCooldown = 15
    Quickshell.execDetached(["systemctl", "--user", "start", "meetbard"])
  }

  function pollNow() {
    if (pollInFlight || statusProc.running || upcomingProc.running || alertsProc.running)
      return
    pollInFlight = true
    statusProc.command = [cliPath, "status", "--json"]
    statusProc.running = true
  }

  function login() {
    if (busy || loginProc.running)
      return
    busy = true
    loginProc.command = [cliPath, "login", "--json"]
    loginProc.running = true
  }

  function logout() {
    if (busy || logoutProc.running)
      return
    busy = true
    logoutProc.command = [cliPath, "logout", "--json"]
    logoutProc.running = true
  }

  function refresh() {
    if (syncProc.running || pollInFlight)
      return
    if (auth === "connected") {
      busy = true
      syncProc.command = [cliPath, "sync", "--json"]
      syncProc.running = true
      return
    }
    pollNow()
  }

  function openJoin(url) {
    if (!url || url.length === 0)
      return
    Quickshell.execDetached(["xdg-open", url])
  }

  function showStartingOverlay(title, joinUrl) {
    if (alertLoader.active && alertLoader.item) {
      alertLoader.item.meetingTitle = title
      alertLoader.item.joinUrl = joinUrl
      return
    }
    const open = function (screen) {
      alertLoader.alertScreen = screen || Quickshell.screens[0]
      alertLoader.pending = {
        "title": title,
        "joinUrl": joinUrl
      }
      alertLoader.active = true
    }
    if (pluginApi?.withCurrentScreen)
      pluginApi.withCurrentScreen(open)
    else
      open(Quickshell.screens[0])
  }

  function formatNotify(threshold, title) {
    const meeting = (title && title.length > 0) ? title : (pluginApi?.tr("notify.meeting") || "Meeting")
    let urgency
    if (threshold === undefined || threshold === null || threshold <= 0)
      urgency = pluginApi?.tr("notify.starting") || "Starting now"
    else if (threshold === 1)
      urgency = pluginApi?.tr("notify.in-one") || "Starts in 1 minute"
    else {
      const tpl = pluginApi?.tr("notify.in-n") || "Starts in {n} minutes"
      urgency = tpl.replace("{n}", String(threshold))
    }
    return {
      "title": meeting,
      "body": urgency,
      "imminent": threshold === undefined || threshold === null || threshold <= 1
    }
  }

  function handleAlerts(alerts) {
    if (!alerts || alerts.length === 0)
      return
    const keys = []
    const seen = Object.assign({}, notifiedKeys)
    for (let i = 0; i < alerts.length; i++) {
      const a = alerts[i]
      if (!a || !a.key)
        continue
      if (seen[a.key])
        continue
      if (wantsNoctaliaNotify) {
        const msg = formatNotify(a.threshold_min, a.title || "")
        const joinUrl = (a.join_url && a.join_url.length > 0) ? a.join_url : ""
        const actionLabel = joinUrl.length > 0
              ? (pluginApi?.tr("notify.join") || "Join meeting")
              : ""
        const actionCb = joinUrl.length > 0
              ? (function (url) {
                  return function () {
                    root.openJoin(url)
                  }
                })(joinUrl)
              : null
        const duration = msg.imminent ? 15000 : 12000
        const isNow = a.threshold_min === undefined || a.threshold_min === null || a.threshold_min <= 0
        if (isNow && overlayAlert) {
          showStartingOverlay(msg.title, joinUrl)
        } else if (msg.imminent) {
          ToastService.showWarning(msg.title, msg.body, duration, actionLabel, actionCb)
        } else {
          ToastService.showNotice(msg.title, msg.body, "video", duration, actionLabel, actionCb)
        }
      }
      seen[a.key] = true
      keys.push(a.key)
    }
    notifiedKeys = seen
    if (keys.length === 0)
      return
    const cmd = [cliPath, "alerts", "ack"].concat(keys)
    cmd.push("--json")
    ackProc.command = cmd
    ackProc.running = true
  }

  function countdownLabel(ev) {
    if (!ev)
      return ""
    let title = ev.title || "Meeting"
    if (title.length > 28)
      title = title.substring(0, 27) + "…"
    const mins = ev.minutes_until
    if (typeof mins === "number") {
      if (mins <= 0)
        return title + " · now"
      if (mins < 60)
        return title + " · " + mins + "m"
      const h = Math.floor(mins / 60)
      const m = mins % 60
      return title + " · " + h + "h" + m + "m"
    }
    const startStr = ev.start || ""
    const m = startStr.match(/T(\d+:\d+)/)
    if (m)
      return title + " · " + m[1]
    return title
  }

  function refreshBarLabel() {
    if (auth === "offline" || daemon === "down") {
      barLabel = pluginApi?.tr("bar.offline") || "…"
      return
    }
    if (auth !== "connected") {
      barLabel = pluginApi?.tr("bar.disconnected") || "Not connected"
      return
    }
    const label = countdownLabel(nextEvent)
    if (label.length > 0) {
      barLabel = label
      return
    }
    if (!showIdle) {
      barLabel = ""
      return
    }
    barLabel = idleText && idleText.length > 0 ? idleText : (pluginApi?.tr("panel.no-meetings") || "No meetings")
  }

  Component.onCompleted: {
    applyOffline("")
    refreshBarLabel()
  }
}
