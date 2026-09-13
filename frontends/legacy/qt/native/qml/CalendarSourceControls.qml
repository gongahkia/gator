import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var calendarSourceModel: null
    property var visibleCalendarIds: []
    property var persistedVisibleCalendarIds: []
    property bool calendarVisibilityConfigured: false
    property bool visibilityInitialized: false
    property bool expanded: true
    property int sourceRevision: calendarSourceModel !== null && calendarSourceModel.revision !== undefined
                                 ? calendarSourceModel.revision : 0
    property alias sourceRows: sourceRows

    function calendarIds() {
        const revision = sourceRevision
        if (calendarSourceModel === null) {
            return []
        }
        if (typeof calendarSourceModel.calendarIds === "function") {
            return calendarSourceModel.calendarIds()
        }
        const ids = []
        for (let row = 0; row < calendarSourceModel.count; ++row) {
            ids.push(calendarSourceModel.get(row).id)
        }
        return ids
    }

    function selectedCalendarIds() {
        if (calendarSourceModel === null) {
            return []
        }
        if (typeof calendarSourceModel.selectedCalendarIds === "function") {
            return calendarSourceModel.selectedCalendarIds()
        }
        const ids = []
        for (let row = 0; row < calendarSourceModel.count; ++row) {
            const calendar = calendarSourceModel.get(row)
            if (calendar.selected) {
                ids.push(calendar.id)
            }
        }
        return ids
    }

    function initializeVisibility() {
        const ids = calendarIds()
        if (ids.length === 0) {
            visibleCalendarIds = []
            visibilityInitialized = false
            return
        }
        const persisted = persistedVisibleCalendarIds.filter(function(calendarId) {
            return ids.indexOf(calendarId) >= 0
        })
        const selected = selectedCalendarIds()
        visibleCalendarIds = calendarVisibilityConfigured ? persisted
                                                     : selected.length > 0 ? selected : ids
        visibilityInitialized = true
    }

    function reconcileVisibility() {
        const ids = calendarIds()
        visibleCalendarIds = visibleCalendarIds.filter(function(calendarId) {
            return ids.indexOf(calendarId) >= 0
        })
    }

    function isVisible(calendarId) {
        return visibleCalendarIds.indexOf(calendarId) >= 0
    }

    function preferredCalendarId() {
        return visibleCalendarIds.length > 0 ? visibleCalendarIds[0] : calendarIds()[0] || ""
    }

    function calendarColor(calendarId) {
        if (calendarSourceModel === null) {
            return Theme.calendarFallback
        }
        if (typeof calendarSourceModel.calendarBackgroundColor === "function") {
            const color = calendarSourceModel.calendarBackgroundColor(calendarId)
            return color.length > 0 ? color : Theme.calendarFallback
        }
        if (typeof calendarSourceModel.get === "function") {
            for (let index = 0; index < calendarSourceModel.count; ++index) {
                const calendar = calendarSourceModel.get(index)
                if (calendar.id === calendarId) return calendar.backgroundColor || Theme.calendarFallback
            }
        }
        return Theme.calendarFallback
    }

    function setCalendarVisible(calendarId, visible) {
        const next = visibleCalendarIds.filter(function(currentId) {
            return currentId !== calendarId
        })
        if (visible) {
            next.push(calendarId)
        }
        visibleCalendarIds = next
    }

    function showAll() {
        visibleCalendarIds = calendarIds()
    }

    onCalendarSourceModelChanged: initializeVisibility()
    onCalendarVisibilityConfiguredChanged: initializeVisibility()
    onPersistedVisibleCalendarIdsChanged: {
        if (calendarVisibilityConfigured) initializeVisibility()
    }
    onSourceRevisionChanged: {
        if (visibilityInitialized) {
            reconcileVisibility()
        } else {
            initializeVisibility()
        }
    }

    clip: true
    padding: Theme.spacingSmall
    implicitHeight: expanded && sourceRows.count > 0
                    ? controlsColumn.implicitHeight + topPadding + bottomPadding : 0

    Behavior on implicitHeight {
        NumberAnimation { duration: 150; easing.type: Easing.OutCubic }
    }

    Behavior on opacity {
        NumberAnimation { duration: 120; easing.type: Easing.OutCubic }
    }

    opacity: expanded ? 1 : 0

    ColumnLayout {
        id: controlsColumn
        anchors.fill: parent
        spacing: Theme.spacingSmall

        RowLayout {
            Layout.fillWidth: true

            Label {
                text: "Calendars"
                font.pixelSize: Theme.bodyFontSize
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }

            Item { Layout.fillWidth: true }

            Button {
                text: "Show all"
                enabled: root.visibleCalendarIds.length < root.calendarIds().length
                Accessible.name: text
                onClicked: root.showAll()
            }
        }

        Flow {
            Layout.fillWidth: true
            spacing: Theme.spacingSmall

            Repeater {
                id: sourceRows
                model: root.calendarSourceModel

                delegate: CheckBox {
                    required property string id
                    required property string title
                    checked: root.isVisible(id)
                    text: title
                    indicator: Rectangle {
                        implicitWidth: 12
                        implicitHeight: 12
                        radius: 6
                        color: root.calendarColor(id)
                        border.width: checked ? 0 : 1
                        border.color: Theme.textSecondary
                        opacity: checked ? 1 : 0.35
                    }
                    Accessible.name: title
                    Accessible.description: checked ? "Calendar visible" : "Calendar hidden"
                    onToggled: root.setCalendarVisible(id, checked)
                }
            }
        }
    }
}
