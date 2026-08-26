import QtQuick
import QtQuick.Layouts
import Quickshell
import Quickshell.Wayland
import qs.Commons
import qs.Widgets

PanelWindow {
  id: root

  property var pluginApi: null
  property string meetingTitle: ""
  property string joinUrl: ""

  signal joinRequested(string url)
  signal closed

  anchors.top: true
  anchors.bottom: true
  anchors.left: true
  anchors.right: true
  color: "transparent"

  WlrLayershell.layer: WlrLayer.Overlay
  WlrLayershell.keyboardFocus: WlrKeyboardFocus.None
  WlrLayershell.exclusionMode: ExclusionMode.Ignore
  WlrLayershell.namespace: "noctalia-meetbar-alert"

  Rectangle {
    anchors.fill: parent
    color: Qt.alpha(Color.mShadow, 0.55)

    MouseArea {
      anchors.fill: parent
      onClicked: root.closed()
    }
  }

  Rectangle {
    anchors.centerIn: parent
    width: 420 * Style.uiScaleRatio
    height: card.implicitHeight + Style.margin2XL
    radius: Style.radiusL
    color: Color.mSurface
    border.color: Color.mOutline
    border.width: 1

    MouseArea {
      anchors.fill: parent
    }

    ColumnLayout {
      id: card
      anchors.left: parent.left
      anchors.right: parent.right
      anchors.verticalCenter: parent.verticalCenter
      anchors.margins: Style.marginXL
      spacing: Style.marginM

      NIcon {
        Layout.alignment: Qt.AlignHCenter
        icon: "video"
        color: Color.mPrimary
        pointSize: Style.fontSizeXXL
      }

      NText {
        Layout.fillWidth: true
        text: root.meetingTitle.length > 0
              ? root.meetingTitle
              : (root.pluginApi?.tr("notify.meeting") || "Meeting")
        pointSize: Style.fontSizeL
        font.weight: Style.fontWeightBold
        color: Color.mOnSurface
        horizontalAlignment: Text.AlignHCenter
        wrapMode: Text.WordWrap
      }

      NText {
        Layout.fillWidth: true
        text: root.pluginApi?.tr("notify.starting") || "Starting now"
        color: Color.mOnSurfaceVariant
        horizontalAlignment: Text.AlignHCenter
      }

      RowLayout {
        Layout.fillWidth: true
        Layout.topMargin: Style.marginS
        spacing: Style.marginM

        NButton {
          visible: root.joinUrl.length > 0
          Layout.fillWidth: true
          text: root.pluginApi?.tr("notify.join") || "Join meeting"
          icon: "video"
          onClicked: {
            root.joinRequested(root.joinUrl)
            root.closed()
          }
        }

        NButton {
          Layout.fillWidth: true
          outlined: true
          text: root.pluginApi?.tr("overlay.dismiss") || "Dismiss"
          onClicked: root.closed()
        }
      }
    }
  }

  Timer {
    interval: 30000
    running: true
    onTriggered: root.closed()
  }
}
