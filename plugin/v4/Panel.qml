import QtQuick
import QtQuick.Layouts
import QtQuick.Controls
import qs.Commons
import qs.Widgets

Item {
  id: root

  property var pluginApi: null
  readonly property var geometryPlaceholder: panelContainer
  readonly property bool allowAttach: true
  readonly property var mainInstance: pluginApi?.mainInstance

  property real contentPreferredWidth: 380 * Style.uiScaleRatio
  property real contentPreferredHeight: 440 * Style.uiScaleRatio

  anchors.fill: parent

  readonly property string auth: mainInstance ? mainInstance.auth : "offline"
  readonly property string email: mainInstance ? mainInstance.email : ""
  readonly property string daemon: mainInstance ? mainInstance.daemon : "down"
  readonly property string errorText: mainInstance ? mainInstance.error : ""
  readonly property var events: mainInstance ? mainInstance.events : []
  readonly property bool busy: mainInstance ? mainInstance.busy : false
  readonly property bool offline: auth === "offline" || daemon === "down"
  readonly property bool connected: auth === "connected"

  function formatWhen(startStr) {
    if (!startStr)
      return ""
    const m = String(startStr).match(/T(\d+):(\d+)/)
    if (m)
      return m[1] + ":" + m[2]
    return startStr
  }

  Rectangle {
    id: panelContainer
    anchors.fill: parent
    color: "transparent"

    ColumnLayout {
      anchors.fill: parent
      anchors.margins: Style.marginL
      spacing: Style.marginM

      RowLayout {
        Layout.fillWidth: true
        spacing: Style.marginS

        Rectangle {
          Layout.preferredWidth: 36 * Style.uiScaleRatio
          Layout.preferredHeight: 36 * Style.uiScaleRatio
          radius: Style.radiusM
          color: Qt.alpha(Color.mPrimary, 0.14)

          NIcon {
            anchors.centerIn: parent
            icon: "calendar"
            color: Color.mPrimary
            pointSize: Style.fontSizeL
          }
        }

        ColumnLayout {
          Layout.fillWidth: true
          spacing: 1

          NText {
            text: pluginApi?.tr("panel.title") || "Meetbar"
            font.weight: Style.fontWeightBold
            pointSize: Style.fontSizeL
            color: Color.mOnSurface
          }

          NText {
            visible: root.connected && root.events.length > 0
            text: pluginApi?.trp("panel.upcoming", root.events.length)
                  || (root.events.length === 1 ? "1 upcoming" : (root.events.length + " upcoming"))
            pointSize: Style.fontSizeXS
            color: Color.mOnSurfaceVariant
          }
        }

        Rectangle {
          visible: root.connected && root.email.length > 0
          Layout.alignment: Qt.AlignTop | Qt.AlignRight
          Layout.maximumWidth: 170 * Style.uiScaleRatio
          implicitWidth: emailLabel.implicitWidth + Style.marginM * 2
          implicitHeight: emailLabel.implicitHeight + Style.marginXS * 2
          radius: Style.radiusL
          color: Qt.alpha(Color.mPrimary, 0.10)
          border.width: Style.borderS
          border.color: Qt.alpha(Color.mPrimary, 0.18)
          clip: true

          NText {
            id: emailLabel
            anchors.centerIn: parent
            width: Math.min(implicitWidth, parent.width - Style.marginM * 2)
            text: root.email
            color: Color.mPrimary
            pointSize: Style.fontSizeXS
            elide: Text.ElideMiddle
          }
        }
      }

      Rectangle {
        Layout.fillWidth: true
        Layout.preferredHeight: 1
        color: Qt.alpha(Color.mOutline, 0.35)
      }

      Item {
        Layout.fillWidth: true
        Layout.fillHeight: true

        ColumnLayout {
          anchors.horizontalCenter: parent.horizontalCenter
          anchors.top: parent.top
          anchors.topMargin: Style.marginL
          width: parent.width
          spacing: Style.marginM
          visible: root.offline

          Rectangle {
            Layout.alignment: Qt.AlignHCenter
            Layout.preferredWidth: 56 * Style.uiScaleRatio
            Layout.preferredHeight: 56 * Style.uiScaleRatio
            radius: width / 2
            color: Qt.alpha(Color.mError, 0.12)

            NIcon {
              anchors.centerIn: parent
              icon: "cloud-off"
              color: Color.mError
              pointSize: Style.fontSizeXXL
            }
          }

          NText {
            Layout.alignment: Qt.AlignHCenter
            text: pluginApi?.tr("panel.offline") || "Daemon offline"
            color: Color.mError
            pointSize: Style.fontSizeM
            font.weight: Style.fontWeightBold
          }

          NText {
            visible: root.errorText.length > 0
            Layout.fillWidth: true
            Layout.alignment: Qt.AlignHCenter
            horizontalAlignment: Text.AlignHCenter
            text: root.errorText
            color: Color.mOnSurfaceVariant
            pointSize: Style.fontSizeS
            wrapMode: Text.Wrap
          }
        }

        ColumnLayout {
          anchors.horizontalCenter: parent.horizontalCenter
          anchors.top: parent.top
          anchors.topMargin: Style.marginL
          width: parent.width
          spacing: Style.marginM
          visible: !root.offline && !root.connected

          Rectangle {
            Layout.alignment: Qt.AlignHCenter
            Layout.preferredWidth: 56 * Style.uiScaleRatio
            Layout.preferredHeight: 56 * Style.uiScaleRatio
            radius: width / 2
            color: Qt.alpha(Color.mPrimary, 0.12)

            NIcon {
              anchors.centerIn: parent
              icon: "link"
              color: Color.mPrimary
              pointSize: Style.fontSizeXXL
            }
          }

          NText {
            Layout.alignment: Qt.AlignHCenter
            text: pluginApi?.tr("panel.disconnected") || "Not connected"
            color: Color.mOnSurface
            pointSize: Style.fontSizeM
            font.weight: Style.fontWeightBold
          }

          NText {
            Layout.alignment: Qt.AlignHCenter
            Layout.fillWidth: true
            horizontalAlignment: Text.AlignHCenter
            text: pluginApi?.tr("panel.connect-hint") || "Link your Google account to see upcoming meetings."
            color: Color.mOnSurfaceVariant
            pointSize: Style.fontSizeS
            wrapMode: Text.Wrap
          }
        }

        ColumnLayout {
          anchors.horizontalCenter: parent.horizontalCenter
          anchors.top: parent.top
          anchors.topMargin: Style.marginL
          width: parent.width
          spacing: Style.marginM
          visible: root.connected && root.events.length === 0

          Rectangle {
            Layout.alignment: Qt.AlignHCenter
            Layout.preferredWidth: 64 * Style.uiScaleRatio
            Layout.preferredHeight: 64 * Style.uiScaleRatio
            radius: width / 2
            color: Qt.alpha(Color.mPrimary, 0.10)

            NIcon {
              anchors.centerIn: parent
              icon: "calendar-check"
              color: Color.mPrimary
              pointSize: Style.fontSizeXXL
            }
          }

          NText {
            Layout.alignment: Qt.AlignHCenter
            text: pluginApi?.tr("panel.no-meetings") || "No upcoming meetings"
            color: Color.mOnSurface
            pointSize: Style.fontSizeM
            font.weight: Style.fontWeightBold
          }

          NText {
            Layout.alignment: Qt.AlignHCenter
            Layout.fillWidth: true
            horizontalAlignment: Text.AlignHCenter
            text: pluginApi?.tr("panel.no-meetings-hint") || "Your calendar is clear. Enjoy the quiet."
            color: Color.mOnSurfaceVariant
            pointSize: Style.fontSizeS
            wrapMode: Text.Wrap
          }
        }

        NScrollView {
          id: agendaScroll
          anchors.fill: parent
          visible: root.connected && root.events.length > 0
          horizontalPolicy: ScrollBar.AlwaysOff
          verticalPolicy: ScrollBar.AsNeeded
          clip: true

          ColumnLayout {
            width: agendaScroll.availableWidth
            spacing: Style.marginS

            Repeater {
              model: {
                const list = root.events || []
                return list.length > 12 ? list.slice(0, 12) : list
              }

              delegate: Rectangle {
                Layout.fillWidth: true
                implicitHeight: row.implicitHeight + Style.marginM * 2
                radius: Style.radiusM
                color: Color.mSurfaceVariant
                border.width: Style.borderS
                border.color: Qt.alpha(Color.mOutline, 0.25)

                Rectangle {
                  anchors.left: parent.left
                  anchors.top: parent.top
                  anchors.bottom: parent.bottom
                  anchors.margins: 6 * Style.uiScaleRatio
                  width: 3 * Style.uiScaleRatio
                  radius: width / 2
                  color: Color.mPrimary
                  opacity: 0.85
                }

                RowLayout {
                  id: row
                  anchors.fill: parent
                  anchors.leftMargin: Style.marginM + 8 * Style.uiScaleRatio
                  anchors.rightMargin: Style.marginM
                  anchors.topMargin: Style.marginM
                  anchors.bottomMargin: Style.marginM
                  spacing: Style.marginM

                  Rectangle {
                    Layout.alignment: Qt.AlignVCenter
                    implicitWidth: timeLabel.implicitWidth + Style.marginS * 2
                    implicitHeight: timeLabel.implicitHeight + Style.marginXS
                    radius: Style.radiusS
                    color: Qt.alpha(Color.mPrimary, 0.14)

                    NText {
                      id: timeLabel
                      anchors.centerIn: parent
                      text: formatWhen(modelData.start)
                      color: Color.mPrimary
                      pointSize: Style.fontSizeS
                      font.weight: Style.fontWeightBold
                      family: Settings.data.ui.fontFixed
                    }
                  }

                  ColumnLayout {
                    Layout.fillWidth: true
                    spacing: 2

                    NText {
                      Layout.fillWidth: true
                      text: modelData.title || "Meeting"
                      color: Color.mOnSurface
                      pointSize: Style.fontSizeM
                      font.weight: Style.fontWeightMedium
                      elide: Text.ElideRight
                    }

                    NText {
                      visible: modelData.location && modelData.location.length > 0
                      Layout.fillWidth: true
                      text: modelData.location
                      color: Color.mOnSurfaceVariant
                      pointSize: Style.fontSizeXS
                      elide: Text.ElideRight
                    }
                  }

                  NButton {
                    visible: modelData.join_url && modelData.join_url.length > 0
                    text: pluginApi?.tr("panel.join") || "Join"
                    onClicked: if (mainInstance) mainInstance.openJoin(modelData.join_url)
                  }
                }
              }
            }
          }
        }
      }

      Rectangle {
        Layout.fillWidth: true
        Layout.preferredHeight: 1
        color: Qt.alpha(Color.mOutline, 0.35)
      }

      RowLayout {
        id: footerRow
        Layout.fillWidth: true
        spacing: Style.marginS

        NButton {
          visible: root.offline
          Layout.fillWidth: true
          text: pluginApi?.tr("panel.refresh") || "Refresh"
          icon: "refresh"
          enabled: !root.busy
          onClicked: if (mainInstance) mainInstance.refresh()
        }

        NButton {
          visible: !root.offline && !root.connected
          Layout.fillWidth: true
          text: pluginApi?.tr("panel.connect") || "Connect Google"
          icon: "link"
          enabled: !root.busy
          onClicked: if (mainInstance) mainInstance.login()
        }

        NButton {
          visible: root.connected
          Layout.fillWidth: true
          text: pluginApi?.tr("panel.refresh") || "Refresh"
          icon: "refresh"
          enabled: !root.busy
          onClicked: if (mainInstance) mainInstance.refresh()
        }

        NButton {
          visible: root.connected
          Layout.fillWidth: true
          text: pluginApi?.tr("panel.disconnect") || "Disconnect"
          icon: "logout"
          enabled: !root.busy
          onClicked: if (mainInstance) mainInstance.logout()
        }
      }
    }
  }
}
