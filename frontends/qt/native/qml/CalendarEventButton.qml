import QtQuick
import QtQuick.Controls

AccessibleButton {
    id: root
    property color eventColor: Theme.calendarFallback
    property bool compact: false
    property bool outlined: false

    leftPadding: Theme.spacingSmall
    rightPadding: Theme.spacingSmall
    topPadding: compact ? 1 : Theme.spacingSmall / 2
    bottomPadding: compact ? 1 : Theme.spacingSmall / 2

    background: Rectangle {
        radius: 5
        color: Theme.darkPalette ? Qt.darker(root.eventColor, 1.8) : Qt.lighter(root.eventColor, 1.72)
        border.width: root.outlined ? 2 : 1
        border.color: root.outlined ? Theme.accent : Qt.darker(root.eventColor, 1.08)
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
