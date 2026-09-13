import QtQuick
import QtQuick.Controls

AccessibleButton {
    id: root
    property bool compact: false
    leftPadding: Theme.spacingSmall
    rightPadding: Theme.spacingSmall
    topPadding: compact ? 1 : Theme.spacingSmall / 2
    bottomPadding: compact ? 1 : Theme.spacingSmall / 2

    background: Rectangle {
        radius: 5
        color: Theme.darkPalette ? Qt.darker(Theme.accent, 1.8) : Qt.lighter(Theme.accent, 1.84)
        border.width: 1
        border.color: Qt.darker(Theme.accent, 1.08)
    }

    contentItem: Label {
        text: root.text
        color: Theme.textPrimary
        elide: Text.ElideRight
        maximumLineCount: root.compact ? 1 : 2
        wrapMode: root.compact ? Text.NoWrap : Text.Wrap
        verticalAlignment: Text.AlignVCenter
    }
}
