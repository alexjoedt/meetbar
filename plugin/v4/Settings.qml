import QtQuick
import QtQuick.Layouts
import qs.Commons
import qs.Widgets

ColumnLayout {
  id: root

  property var pluginApi: null

  property var cfg: pluginApi?.pluginSettings || ({})
  property var defaults: pluginApi?.manifest?.metadata?.defaultSettings || ({})

  property string notifyPath: cfg.notifyPath ?? defaults.notifyPath ?? "noctalia"
  property int pollSeconds: cfg.pollSeconds ?? defaults.pollSeconds ?? 30
  property string cliPath: cfg.cliPath ?? defaults.cliPath ?? "meetbarctl"
  property int horizonHours: cfg.horizonHours ?? defaults.horizonHours ?? 12
  property bool showIdle: cfg.showIdle ?? defaults.showIdle ?? true
  property string idleText: cfg.idleText ?? defaults.idleText ?? "No meetings"

  spacing: Style.marginL

  onPluginApiChanged: {
    if (pluginApi)
      loadSettings()
  }

  Component.onCompleted: {
    if (pluginApi)
      loadSettings()
  }

  function loadSettings() {
    const settings = pluginApi?.pluginSettings
    const defs = pluginApi?.manifest?.metadata?.defaultSettings
    root.notifyPath = settings?.notifyPath ?? defs?.notifyPath ?? "noctalia"
    root.pollSeconds = settings?.pollSeconds ?? defs?.pollSeconds ?? 30
    root.cliPath = settings?.cliPath ?? defs?.cliPath ?? "meetbarctl"
    root.horizonHours = settings?.horizonHours ?? defs?.horizonHours ?? 12
    root.showIdle = settings?.showIdle ?? defs?.showIdle ?? true
    root.idleText = settings?.idleText ?? defs?.idleText ?? "No meetings"
  }

  ColumnLayout {
    Layout.fillWidth: true
    spacing: Style.marginM

    NComboBox {
      Layout.fillWidth: true
      label: pluginApi?.tr("settings.notify-path") || "Notification path"
      description: pluginApi?.tr("settings.notify-path-desc") || "Where meeting reminders are shown"
      model: [
        {
          "key": "noctalia",
          "name": pluginApi?.tr("settings.notify-noctalia") || "Noctalia only"
        },
        {
          "key": "daemon",
          "name": pluginApi?.tr("settings.notify-daemon") || "Desktop (daemon) only"
        },
        {
          "key": "both",
          "name": pluginApi?.tr("settings.notify-both") || "Noctalia and desktop"
        }
      ]
      currentKey: root.notifyPath
      onSelected: key => root.notifyPath = key
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: Style.marginS

      NLabel {
        label: pluginApi?.tr("settings.poll-seconds") || "Poll interval (seconds)"
        description: pluginApi?.tr("settings.poll-seconds-desc") || "How often the plugin queries the meetbar daemon"
      }

      NSpinBox {
        from: 5
        to: 600
        stepSize: 5
        value: root.pollSeconds
        onValueChanged: if (value !== root.pollSeconds) root.pollSeconds = value
      }
    }

    NTextInput {
      Layout.fillWidth: true
      label: pluginApi?.tr("settings.cli-path") || "CLI path"
      description: pluginApi?.tr("settings.cli-path-desc") || "Path to the meetbarctl binary"
      text: root.cliPath
      onTextChanged: root.cliPath = text
    }

    ColumnLayout {
      Layout.fillWidth: true
      spacing: Style.marginS

      NLabel {
        label: pluginApi?.tr("settings.horizon-hours") || "Horizon (hours)"
        description: pluginApi?.tr("settings.horizon-hours-desc") || "How far ahead to load meetings"
      }

      NSpinBox {
        from: 1
        to: 168
        stepSize: 1
        value: root.horizonHours
        onValueChanged: if (value !== root.horizonHours) root.horizonHours = value
      }
    }

    NToggle {
      label: pluginApi?.tr("settings.show-idle") || "Show when free"
      description: pluginApi?.tr("settings.show-idle-desc") || "Show idle text in the bar when there is no upcoming meeting"
      checked: root.showIdle
      onToggled: checked => root.showIdle = checked
    }

    NTextInput {
      Layout.fillWidth: true
      label: pluginApi?.tr("settings.idle-text") || "Idle text"
      description: pluginApi?.tr("settings.idle-text-desc") || "Bar text when no meeting is upcoming"
      text: root.idleText
      onTextChanged: root.idleText = text
    }
  }

  function saveSettings() {
    if (!pluginApi) {
      Logger.e("Meetbar", "Cannot save settings: pluginApi is null")
      return
    }

    pluginApi.pluginSettings.notifyPath = root.notifyPath
    pluginApi.pluginSettings.pollSeconds = root.pollSeconds
    pluginApi.pluginSettings.cliPath = root.cliPath
    pluginApi.pluginSettings.horizonHours = root.horizonHours
    pluginApi.pluginSettings.showIdle = root.showIdle
    pluginApi.pluginSettings.idleText = root.idleText
    pluginApi.saveSettings()

    if (pluginApi.mainInstance)
      pluginApi.mainInstance.settingsVersion++
  }
}
