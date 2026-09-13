import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var monthGridModel: null
    property var calendarVisibility: null
    property var selectedEventIds: []
    property bool selectionMode: false
    property int weekStartDay: 0
    property string calendarDate: ""
    property var scheduledTaskIndex: null
    property string monthLabel: ""
    property int visibleAllDayLanes: 3
    property int allDayLaneHeight: 22
    property string selectedDate: calendarDate
    property bool dragPreviewActive: false
    property string dragPreviewTitle: ""
    property string dragPreviewStartDate: ""
    property string dragPreviewEndDate: ""
    property var overflowAllDayEvents: []
    property var overflowTimedEvents: []
    property var overflowTasks: []
    property string overflowDate: ""
    property alias cells: cells
    signal dateSelected(string date)
    signal eventSelectionRequested(string eventId, bool selected)
    signal eventCreateRequested(string date)
    signal quickCreateRequested(string startAt, string endAt, bool allDay)
    signal eventEditRequested(var event)
    signal eventDetailRequested(var event)
    signal eventMoveScopeRequested(var event, string startAt, string endAt, bool allDay)
    signal eventAllDayResizeScopeRequested(var event, string startAt, string endAt)
    signal taskMoveRequested(string taskId, string dueDate)
    signal taskDetailRequested(var task)
    signal periodNavigationRequested(int direction)

    function isCalendarVisible(calendarId) {
        return calendarVisibility === null || calendarVisibility.isVisible(calendarId)
    }

    function selectDate(date) {
        selectedDate = date
        dateSelected(date)
    }

    function eventSummary(events) {
        const visible = events.filter(function(event) { return root.isCalendarVisible(event.calendarId) })
        return visible.length === 0 ? "" : visible[0].title + (visible.length > 1 ? " +" + (visible.length - 1) : "")
    }

    function eventIds(events) {
        return events.filter(function(event) {
            return root.isCalendarVisible(event.calendarId) && typeof event.id === "string" && event.id.length > 0
        }).map(function(event) { return event.id })
    }

    function isEventSelected(eventId) { return selectedEventIds.indexOf(eventId) >= 0 }

    function eventColor(calendarId, colorId) {
        const fallback = calendarVisibility !== null && typeof calendarVisibility.calendarColor === "function"
                       ? calendarVisibility.calendarColor(calendarId) : Theme.calendarFallback
        return Theme.calendarColor(colorId, fallback)
    }

    function dateForGridPoint(x, y) {
        return monthGridModel !== null && typeof monthGridModel.dateForPoint === "function"
             ? monthGridModel.dateForPoint(x, y, gridArea.width, gridArea.height) : ""
    }

    function dateIndex(date) {
        return monthGridModel !== null && typeof monthGridModel.dateIndex === "function"
             ? monthGridModel.dateIndex(date) : -1
    }

    function dateForGridIndex(index) {
        return monthGridModel !== null && typeof monthGridModel.dateForIndex === "function"
             ? monthGridModel.dateForIndex(index) : ""
    }

    function dateInRange(date, firstDate, lastDate) {
        const index = dateIndex(date)
        const first = dateIndex(firstDate)
        const last = dateIndex(lastDate)
        return index >= 0 && first >= 0 && last >= 0 && index >= Math.min(first, last) &&
               index <= Math.max(first, last)
    }

    function requestEventMove(event, targetDate) {
        if (monthGridModel === null || typeof monthGridModel.moveInput !== "function") return
        const move = monthGridModel.moveInput(event, dateIndex(targetDate))
        if (typeof move.startAt === "string" && typeof move.endAt === "string") {
            eventMoveScopeRequested(event, move.startAt, move.endAt, move.allDay === true)
        }
    }

    function requestAllDayResize(event, firstDate, lastDate) {
        if (monthGridModel === null || typeof monthGridModel.resizeAllDayRangeInput !== "function") return
        const resize = monthGridModel.resizeAllDayRangeInput(event, dateIndex(firstDate), dateIndex(lastDate))
        if (typeof resize.startAt === "string" && typeof resize.endAt === "string") {
            eventAllDayResizeScopeRequested(event, resize.startAt, resize.endAt)
        }
    }

    function tasksForDate(date) {
        const revision = scheduledTaskIndex !== null && scheduledTaskIndex !== undefined &&
                       typeof scheduledTaskIndex.revision === "number"
                       ? scheduledTaskIndex.revision : 0
        if (revision < 0) return []
        return scheduledTaskIndex !== null && scheduledTaskIndex !== undefined &&
               typeof scheduledTaskIndex.tasksForDate === "function"
               ? scheduledTaskIndex.tasksForDate(date) : []
    }

    function quickCreateForRange(firstDate, lastDate) {
        if (monthGridModel === null || typeof monthGridModel.allDayRangeInput !== "function") return
        const input = monthGridModel.allDayRangeInput(dateIndex(firstDate), dateIndex(lastDate))
        if (typeof input.startAt === "string" && typeof input.endAt === "string") {
            quickCreateRequested(input.startAt, input.endAt, true)
        }
    }

    function moreCount(hiddenAllDayCount, visibleTimedEvents, date) {
        return hiddenAllDayCount + Math.max(0, visibleTimedEvents.length - 2) +
               Math.max(0, tasksForDate(date).length - 1)
    }

    function openOverflow(date, visibleAllDayEvents, visibleTimedEvents, x, y) {
        overflowDate = date
        overflowAllDayEvents = visibleAllDayEvents
        overflowTimedEvents = visibleTimedEvents
        overflowTasks = tasksForDate(date)
        overflowPopup.x = Math.max(0, Math.min(gridArea.width - overflowPopup.width, x))
        overflowPopup.y = Math.max(0, Math.min(gridArea.height - overflowPopup.height, y))
        overflowPopup.open()
    }

    function previewSegments() {
        if (!dragPreviewActive) return []
        let first = dateIndex(dragPreviewStartDate)
        let last = dateIndex(dragPreviewEndDate)
        if (first < 0 || last < 0) return []
        if (first > last) { const swap = first; first = last; last = swap }
        const segments = []
        for (let start = first; start <= last;) {
            const end = Math.min(last, Math.floor(start / 7) * 7 + 6)
            segments.push({ weekIndex: Math.floor(start / 7), startColumn: start % 7,
                            daySpan: end - start + 1 })
            start = end + 1
        }
        return segments
    }

    function showPreview(title, firstDate, lastDate) {
        dragPreviewActive = true
        dragPreviewTitle = title
        dragPreviewStartDate = firstDate
        dragPreviewEndDate = lastDate
    }

    function clearPreview() {
        dragPreviewActive = false
        dragPreviewTitle = ""
        dragPreviewStartDate = ""
        dragPreviewEndDate = ""
    }

    function syncPresentationIndex() {
        if (monthGridModel === null) return
        if (typeof monthGridModel.visibleAllDayLanes === "number") {
            monthGridModel.visibleAllDayLanes = visibleAllDayLanes
        }
        if (calendarVisibility !== null && calendarVisibility.visibleCalendarIds !== undefined) {
            monthGridModel.visibleCalendarIds = calendarVisibility.visibleCalendarIds
        }
    }

    onMonthGridModelChanged: syncPresentationIndex()
    onCalendarVisibilityChanged: syncPresentationIndex()
    onVisibleAllDayLanesChanged: syncPresentationIndex()
    Component.onCompleted: syncPresentationIndex()

    Connections {
        target: root.calendarVisibility
        function onVisibleCalendarIdsChanged() { root.syncPresentationIndex() }
    }

    WheelHandler {
        target: null
        onWheel: function(event) {
            if (event.angleDelta.y === 0) return
            root.periodNavigationRequested(event.angleDelta.y < 0 ? 1 : -1)
            event.accepted = true
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingMedium

        Label {
            text: root.monthLabel.length > 0 ? root.monthLabel : "Month"
            font.pixelSize: Theme.titleFontSize
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        RowLayout {
            Layout.fillWidth: true
            Repeater {
                model: root.weekStartDay === 1
                       ? ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]
                       : ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]
                delegate: Label {
                    required property string modelData
                    Layout.fillWidth: true
                    text: modelData
                    horizontalAlignment: Text.AlignHCenter
                }
            }
        }

        Item {
            id: gridArea
            objectName: "monthGridArea"
            Layout.fillWidth: true
            Layout.fillHeight: true

            DropArea {
                anchors.fill: parent
                z: 8
                keys: ["hcb-task"]
                onEntered: function(drag) {
                    root.showPreview(drag.source.title || "Task", root.dateForGridPoint(drag.x, drag.y),
                                     root.dateForGridPoint(drag.x, drag.y))
                }
                onPositionChanged: function(drag) {
                    root.showPreview(drag.source.title || "Task", root.dateForGridPoint(drag.x, drag.y),
                                     root.dateForGridPoint(drag.x, drag.y))
                }
                onExited: root.clearPreview()
                onDropped: function(drop) {
                    if (drop.source !== null && typeof drop.source.taskId === "string") {
                        root.taskMoveRequested(drop.source.taskId, root.dateForGridPoint(drop.x, drop.y))
                    }
                    root.clearPreview()
                }
            }

            TableView {
                id: cells
                anchors.fill: parent
                clip: true
                columnSpacing: 1
                rowSpacing: 1
                acceptedButtons: Qt.NoButton
                model: root.monthGridModel
                columnWidthProvider: function() { return width / 7 }
                rowHeightProvider: function() { return height / 6 }

                delegate: Item {
                    required property string date
                    required property int day
                    required property bool outsideMonth
                    required property var visibleTimedEvents
                    required property var visibleAllDayEvents
                    required property int hiddenAllDayCount
                    opacity: outsideMonth ? 0.5 : 1

                    Rectangle {
                        anchors.fill: parent
                        visible: root.dragPreviewActive && root.dateInRange(date,
                                                                             root.dragPreviewStartDate,
                                                                             root.dragPreviewEndDate)
                        color: Qt.rgba(Theme.accent.r, Theme.accent.g, Theme.accent.b, 0.10)
                        border.color: Theme.accent
                        border.width: 1
                        radius: 4
                    }

                    Rectangle {
                        anchors.fill: parent
                        visible: root.selectedDate === date
                        color: "transparent"
                        border.color: Theme.accent
                        border.width: 1
                        radius: 4
                    }

                    Item {
                        id: cellQuickCreateArea
                        objectName: "monthCellQuickCreate:" + date
                        anchors.fill: parent
                        z: -1

                        HoverHandler { cursorShape: Qt.CrossCursor }

                        TapHandler {
                            acceptedButtons: Qt.LeftButton
                            onTapped: root.selectedDate = date
                            onDoubleTapped: function() {
                                if (!root.selectionMode) root.quickCreateForRange(date, date)
                            }
                        }

                        DragHandler {
                            id: monthCreateDrag
                            enabled: !root.selectionMode
                            target: null
                            acceptedButtons: Qt.LeftButton
                            cursorShape: Qt.CrossCursor
                            grabPermissions: PointerHandler.CanTakeOverFromItems |
                                             PointerHandler.ApprovesTakeOverByAnything
                            property bool creating: false
                            property string firstDate: ""
                            onActiveChanged: {
                                if (active) {
                                    creating = true
                                    const start = parent.mapToItem(gridArea, centroid.pressPosition.x,
                                                                   centroid.pressPosition.y)
                                    const current = parent.mapToItem(gridArea, centroid.position.x,
                                                                     centroid.position.y)
                                    firstDate = root.dateForGridPoint(start.x, start.y)
                                    root.showPreview("New event", firstDate,
                                                     root.dateForGridPoint(current.x, current.y))
                                } else if (creating) {
                                    const point = parent.mapToItem(gridArea, centroid.position.x,
                                                                   centroid.position.y)
                                    root.quickCreateForRange(firstDate,
                                                             root.dateForGridPoint(point.x, point.y))
                                    root.clearPreview()
                                    creating = false
                                }
                            }
                            onTranslationChanged: {
                                if (active) {
                                    const point = parent.mapToItem(gridArea, centroid.position.x,
                                                                   centroid.position.y)
                                    root.showPreview("New event", firstDate,
                                                     root.dateForGridPoint(point.x, point.y))
                                }
                            }
                            onCanceled: function() {
                                creating = false
                                root.clearPreview()
                            }
                        }
                    }

                    AccessibleButton {
                        id: dayButton
                        anchors.top: parent.top
                        anchors.left: parent.left
                        anchors.margins: Theme.spacingSmall
                        width: 30
                        height: 24
                        padding: 0
                        text: day
                        accessibleName: date
                        onClicked: root.selectDate(date)
                        background: Item {}
                    }

                    Column {
                        anchors.top: parent.top
                        anchors.topMargin: 28 + root.visibleAllDayLanes * root.allDayLaneHeight
                        anchors.left: parent.left
                        anchors.right: parent.right
                        anchors.bottom: parent.bottom
                        anchors.margins: Theme.spacingSmall
                        spacing: 1
                        clip: true

                        Repeater {
                            model: visibleTimedEvents.slice(0, 2)
                            delegate: CalendarEventButton {
                                required property var modelData
                                width: parent.width
                                height: root.allDayLaneHeight
                                compact: true
                                eventColor: root.eventColor(modelData.calendarId, modelData.colorId || "")
                                text: modelData.title
                                onClicked: {
                                    if (root.selectionMode) root.eventSelectionRequested(modelData.id, !root.isEventSelected(modelData.id))
                                    else root.eventDetailRequested(modelData)
                                }
                                DragHandler {
                                    enabled: !root.selectionMode
                                    target: null
                                    onTranslationChanged: {
                                        const point = parent.mapToItem(gridArea, parent.width / 2 + activeTranslation.x,
                                                                       parent.height / 2 + activeTranslation.y)
                                        const target = root.dateForGridPoint(point.x, point.y)
                                        root.showPreview(modelData.title, target, target)
                                    }
                                    onActiveChanged: {
                                        const point = parent.mapToItem(gridArea, parent.width / 2 + activeTranslation.x,
                                                                       parent.height / 2 + activeTranslation.y)
                                        if (!active) {
                                            root.requestEventMove(modelData, root.dateForGridPoint(point.x, point.y))
                                            root.clearPreview()
                                        }
                                    }
                                }
                            }
                        }

                        Repeater {
                            model: root.tasksForDate(date).slice(0, 1)
                            delegate: CalendarTaskButton {
                                required property var modelData
                                width: parent.width
                                height: root.allDayLaneHeight
                                compact: true
                                text: modelData.title
                                accessibleName: "Task: " + modelData.title
                                onClicked: root.taskDetailRequested(modelData)
                                DragHandler {
                                    enabled: !root.selectionMode
                                    target: null
                                    onTranslationChanged: {
                                        const point = parent.mapToItem(gridArea, parent.width / 2 + activeTranslation.x,
                                                                       parent.height / 2 + activeTranslation.y)
                                        const target = root.dateForGridPoint(point.x, point.y)
                                        root.showPreview(modelData.title, target, target)
                                    }
                                    onActiveChanged: {
                                        const point = parent.mapToItem(gridArea, parent.width / 2 + activeTranslation.x,
                                                                       parent.height / 2 + activeTranslation.y)
                                        if (!active) {
                                            root.taskMoveRequested(modelData.id, root.dateForGridPoint(point.x, point.y))
                                            root.clearPreview()
                                        }
                                    }
                                }
                            }
                        }

                        AccessibleButton {
                            id: moreButton
                            property int hiddenCount: root.moreCount(hiddenAllDayCount, visibleTimedEvents, date)
                            width: parent.width
                            height: root.allDayLaneHeight
                            visible: hiddenCount > 0
                            padding: 2
                            text: "+" + hiddenCount + " more"
                            onClicked: {
                                const point = mapToItem(gridArea, x, y + height)
                                root.openOverflow(date, visibleAllDayEvents, visibleTimedEvents, point.x, point.y)
                            }
                        }
                    }
                }
            }

            Repeater {
                model: root.previewSegments()
                delegate: CalendarEventButton {
                    required property var modelData
                    x: modelData.startColumn * gridArea.width / 7 + Theme.spacingSmall
                    y: modelData.weekIndex * gridArea.height / 6 + 28
                    width: Math.max(1, modelData.daySpan * gridArea.width / 7 - Theme.spacingSmall * 2)
                    height: root.allDayLaneHeight - 1
                    z: 5
                    enabled: false
                    opacity: 0.48
                    compact: true
                    eventColor: Theme.accent
                    text: root.dragPreviewTitle
                }
            }

            Repeater {
                model: root.monthGridModel !== null && root.monthGridModel.visibleAllDaySpans !== undefined
                       ? root.monthGridModel.visibleAllDaySpans : []
                delegate: CalendarEventButton {
                    required property var modelData
                    x: modelData.startColumn * gridArea.width / 7 + Theme.spacingSmall
                    y: modelData.weekIndex * gridArea.height / 6 + 28 + modelData.laneIndex * root.allDayLaneHeight
                    width: Math.max(1, modelData.daySpan * gridArea.width / 7 - Theme.spacingSmall * 2)
                    height: root.allDayLaneHeight - 1
                    z: 6
                    compact: true
                    eventColor: root.eventColor(modelData.calendarId, modelData.colorId || "")
                    text: (modelData.startsBeforeRange ? "‹ " : "") + modelData.title +
                          (modelData.endsAfterRange ? " ›" : "")
                    accessibleName: modelData.title
                    onClicked: {
                        if (root.selectionMode) root.eventSelectionRequested(modelData.id, !root.isEventSelected(modelData.id))
                        else root.eventDetailRequested(modelData)
                    }

                    HoverHandler { id: allDayHover }

                    DragHandler {
                        id: allDayMoveHandler
                        enabled: !root.selectionMode && !modelData.startsBeforeRange
                        target: null
                        onTranslationChanged: {
                            const point = parent.mapToItem(gridArea, parent.x + activeTranslation.x,
                                                           parent.y + parent.height / 2 + activeTranslation.y)
                            const target = root.dateForGridPoint(point.x, point.y)
                            const span = Math.max(1, modelData.daySpan)
                            const targetIndex = root.dateIndex(target)
                            root.showPreview(modelData.title, target,
                                             root.dateForGridIndex(targetIndex + span - 1))
                        }
                        onActiveChanged: {
                            const point = parent.mapToItem(gridArea, parent.x + activeTranslation.x,
                                                           parent.y + parent.height / 2 + activeTranslation.y)
                            if (!active && !allDayStartResizeHandler.active && !allDayEndResizeHandler.active) {
                                root.requestEventMove(modelData, root.dateForGridPoint(point.x, point.y))
                            }
                            if (!active) root.clearPreview()
                        }
                    }

                    AccessibleButton {
                        id: allDayStartResizeHandle
                        anchors.left: parent.left
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        width: 12
                        visible: allDayHover.hovered || allDayStartResizeHandler.active
                        enabled: !root.selectionMode && !modelData.startsBeforeRange
                        padding: 0
                        text: ""
                        accessibleName: "Resize " + modelData.title + " start"
                        background: Rectangle { color: root.eventColor(modelData.calendarId, modelData.colorId || ""); opacity: 0.7; radius: 2 }
                        DragHandler {
                            id: allDayStartResizeHandler
                            target: null
                            enabled: !root.selectionMode && !modelData.startsBeforeRange
                            cursorShape: Qt.SizeHorCursor
                            grabPermissions: PointerHandler.CanTakeOverFromAnything
                            onTranslationChanged: {
                                const point = parent.parent.mapToItem(gridArea, parent.parent.x + activeTranslation.x,
                                                                      parent.parent.y + parent.parent.height / 2)
                                root.showPreview(modelData.title, root.dateForGridPoint(point.x, point.y),
                                                 (modelData.endAt || "").slice(0, 10))
                            }
                            onActiveChanged: {
                                const point = parent.parent.mapToItem(gridArea, parent.parent.x + activeTranslation.x,
                                                                      parent.parent.y + parent.parent.height / 2)
                                if (!active) {
                                    const target = root.dateForGridPoint(point.x, point.y)
                                    const currentEnd = root.dateForGridIndex(
                                                root.dateIndex((modelData.endAt || "").slice(0, 10)) - 1)
                                    if (currentEnd !== "") root.requestAllDayResize(modelData, target, currentEnd)
                                    root.clearPreview()
                                }
                            }
                        }
                    }

                    AccessibleButton {
                        id: allDayEndResizeHandle
                        anchors.right: parent.right
                        anchors.top: parent.top
                        anchors.bottom: parent.bottom
                        width: 12
                        visible: allDayHover.hovered || allDayEndResizeHandler.active
                        enabled: !root.selectionMode && !modelData.endsAfterRange
                        padding: 0
                        text: ""
                        accessibleName: "Resize " + modelData.title + " end"
                        background: Rectangle { color: root.eventColor(modelData.calendarId, modelData.colorId || ""); opacity: 0.7; radius: 2 }
                        DragHandler {
                            id: allDayEndResizeHandler
                            target: null
                            enabled: !root.selectionMode && !modelData.endsAfterRange
                            cursorShape: Qt.SizeHorCursor
                            grabPermissions: PointerHandler.CanTakeOverFromAnything
                            onTranslationChanged: {
                                const point = parent.parent.mapToItem(gridArea, parent.parent.x + parent.parent.width - 1 +
                                                                      activeTranslation.x, parent.parent.y + parent.parent.height / 2)
                                root.showPreview(modelData.title, (modelData.startAt || "").slice(0, 10),
                                                 root.dateForGridPoint(point.x, point.y))
                            }
                            onActiveChanged: {
                                const point = parent.parent.mapToItem(gridArea, parent.parent.x + parent.parent.width - 1 +
                                                                      activeTranslation.x, parent.parent.y + parent.parent.height / 2)
                                if (!active) {
                                    root.requestAllDayResize(modelData, (modelData.startAt || "").slice(0, 10),
                                                             root.dateForGridPoint(point.x, point.y))
                                    root.clearPreview()
                                }
                            }
                        }
                    }
                }
            }

            Popup {
                id: overflowPopup
                parent: gridArea
                width: Math.min(360, gridArea.width)
                height: Math.min(420, gridArea.height)
                modal: false
                focus: true
                padding: Theme.spacingMedium
                onClosed: {
                    root.overflowAllDayEvents = []
                    root.overflowTimedEvents = []
                    root.overflowTasks = []
                }

                contentItem: ColumnLayout {
                    spacing: Theme.spacingSmall
                    Label {
                        text: root.overflowDate
                        font.bold: true
                        Accessible.role: Accessible.Heading
                    }
                    ScrollView {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        clip: true
                        Column {
                            width: parent.width
                            spacing: Theme.spacingSmall
                            Repeater {
                                model: root.overflowAllDayEvents
                                delegate: CalendarEventButton {
                                    required property var modelData
                                    width: parent.width
                                    compact: true
                                    eventColor: root.eventColor(modelData.calendarId, modelData.colorId || "")
                                    text: modelData.title + " — All day"
                                    onClicked: { overflowPopup.close(); root.eventDetailRequested(modelData) }
                                }
                            }
                            Repeater {
                                model: root.overflowTimedEvents
                                delegate: CalendarEventButton {
                                    required property var modelData
                                    width: parent.width
                                    compact: true
                                    eventColor: root.eventColor(modelData.calendarId, modelData.colorId || "")
                                    text: modelData.title
                                    onClicked: { overflowPopup.close(); root.eventDetailRequested(modelData) }
                                }
                            }
                            Repeater {
                                model: root.overflowTasks
                                delegate: CalendarTaskButton {
                                    required property var modelData
                                    width: parent.width
                                    compact: true
                                    text: modelData.title
                                    onClicked: { overflowPopup.close(); root.taskDetailRequested(modelData) }
                                }
                            }
                        }
                    }
                    AccessibleButton {
                        Layout.alignment: Qt.AlignRight
                        text: "Open day"
                        onClicked: { overflowPopup.close(); root.selectDate(root.overflowDate) }
                    }
                }
            }
        }
    }

    Label {
        anchors.centerIn: parent
        visible: monthGridModel === null || cells.rows === 0
        text: "No month selected."
        color: Theme.textSecondary
    }
}
