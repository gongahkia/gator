import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var timelineModel: null
    property var calendarVisibility: null
    property var selectedEventIds: []
    property bool selectionMode: false
    property int dayIndex: 0
    property string dateLabel: ""
    property string dateIso: ""
    property var scheduledTaskIndex: null
    property int hourHeight: 64
    property bool use24HourTime: true
    property int workdayStartHour: 9
    property int timeColumnWidth: 64
    property bool timelineActive: true
    property bool bypassCalendarVisibility: false
    property var allDayEventRows: null
    property var timedEventRows: null
    property var viewportSource: null
    property alias eventRows: eventRows
    signal eventSelected(string eventId)
    signal eventSelectionRequested(string eventId, bool selected)
    signal eventMoveRequested(string eventId, string startAt, string endAt, bool allDay)
    signal eventResizeRequested(string eventId, string endAt)
    signal eventMoveScopeRequested(var event, string startAt, string endAt, bool allDay)
    signal eventResizeScopeRequested(var event, string endAt)
    signal quickCreateRequested(string startAt, string endAt, bool allDay)
    signal taskMoveRequested(string taskId, string dueDate)
    signal taskDetailRequested(var task)
    signal eventEditRequested(string eventId, string calendarId, string title, string startAt,
                              string endAt, bool allDay, string description, string location,
                              string startTimeZone, string colorId, string transparency,
                              string visibility, string attendeeEmailsJson, string remindersJson,
                              bool remindersUseDefault, string recurrenceRule,
                              string recurringRemoteId, string originalStartAt, string eventType,
                              string conferenceJson, string attachmentsJson,
                              string guestPermissionsJson, string statusPropertiesJson)
    signal eventDetailRequested(var event)
    signal periodNavigationRequested(int direction)
    property bool dragPreviewActive: false
    property int dragPreviewStartMinute: 0
    property int dragPreviewEndMinute: 15
    property string dragPreviewTitle: ""

    function timePosition(minute) {
        return minute * hourHeight / 60
    }

    function eventHeight(durationMinutes) {
        return Math.max(24, durationMinutes * hourHeight / 60)
    }

    function eventColor(calendarId, colorId) {
        const fallback = calendarVisibility !== null && typeof calendarVisibility.calendarColor === "function"
                       ? calendarVisibility.calendarColor(calendarId) : Theme.calendarFallback
        return Theme.calendarColor(colorId, fallback)
    }

    function timeRange(startAt, endAt) {
        const start = new Date(startAt)
        const end = new Date(endAt)
        if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime())) return ""
        return Qt.locale().toString(start, "HH:mm") + "–" + Qt.locale().toString(end, "HH:mm")
    }

    function hourLabel(hour) {
        if (use24HourTime) return String(hour).padStart(2, "0") + ":00"
        const suffix = hour < 12 ? "AM" : "PM"
        const display = hour % 12 === 0 ? 12 : hour % 12
        return display + " " + suffix
    }

    function dropMinute(y) {
        const point = timelineModel !== null && typeof timelineModel.timelinePointInput === "function"
                ? timelineModel.timelinePointInput(timeColumnWidth, y, timelineCanvas.width,
                                                   timeColumnWidth, hourHeight, false) : null
        return point !== null && typeof point.minute === "number" ? point.minute : 0
    }

    function dropEndMinute(y) {
        const point = timelineModel !== null && typeof timelineModel.timelinePointInput === "function"
                ? timelineModel.timelinePointInput(timeColumnWidth, y, timelineCanvas.width,
                                                   timeColumnWidth, hourHeight, true) : null
        return point !== null && typeof point.minute === "number" ? point.minute : 15
    }

    function quickCreateAt(startMinute, endMinute) {
        if (timelineModel === null || typeof timelineModel.timedRangeInput !== "function") return
        const input = timelineModel.timedRangeInput(dayIndex, Math.min(startMinute, endMinute), dayIndex,
                                                    Math.max(Math.min(startMinute, endMinute) + 15, endMinute))
        if (typeof input.startAt === "string" && typeof input.endAt === "string") {
            quickCreateRequested(input.startAt, input.endAt, false)
        }
    }

    function showTimedPreview(title, startMinute, endMinute) {
        dragPreviewActive = true
        dragPreviewTitle = title
        dragPreviewStartMinute = Math.max(0, Math.min(24 * 60 - 15, startMinute))
        dragPreviewEndMinute = Math.max(dragPreviewStartMinute + 15,
                                        Math.min(24 * 60, endMinute))
    }

    function clearDragPreview() {
        dragPreviewActive = false
        dragPreviewTitle = ""
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

    function requestMove(eventId, targetDayIndex, targetMinute, sourceEvent) {
        if (timelineModel === null || typeof timelineModel.moveInput !== "function") {
            return
        }
        const move = timelineModel.moveInput(eventId, targetDayIndex, targetMinute)
        if (move === null || typeof move.id !== "string" || typeof move.startAt !== "string" ||
                typeof move.endAt !== "string" || typeof move.allDay !== "boolean") {
            return
        }
        if (sourceEvent !== undefined) eventMoveScopeRequested(sourceEvent, move.startAt, move.endAt, move.allDay)
        else eventMoveRequested(move.id, move.startAt, move.endAt, move.allDay)
    }

    function requestResize(eventId, targetEndDayIndex, targetEndMinute, sourceEvent) {
        if (timelineModel === null || typeof timelineModel.resizeInput !== "function") {
            return
        }
        const resize = timelineModel.resizeInput(eventId, targetEndDayIndex, targetEndMinute)
        if (resize === null || typeof resize.id !== "string" || typeof resize.endAt !== "string") {
            return
        }
        if (sourceEvent !== undefined) eventResizeScopeRequested(sourceEvent, resize.endAt)
        else eventResizeRequested(resize.id, resize.endAt)
    }

    function selectEvent(eventId) {
        eventSelected(eventId)
    }

    function requestEdit(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                         startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                         remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                         originalStartAt, eventType, conferenceJson, attachmentsJson,
                         guestPermissionsJson, statusPropertiesJson) {
        eventEditRequested(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                           startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                           remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                           originalStartAt, eventType, conferenceJson, attachmentsJson,
                           guestPermissionsJson, statusPropertiesJson)
    }

    function requestDetail(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                           startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                           remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                           originalStartAt, eventType, conferenceJson, attachmentsJson,
                           guestPermissionsJson, statusPropertiesJson) {
        eventDetailRequested({id: eventId, calendarId: calendarId, title: title, startAt: startAt,
                              endAt: endAt, allDay: allDay, description: description, location: location,
                              startTimeZone: startTimeZone, colorId: colorId, transparency: transparency,
                              visibility: visibility, attendeeEmailsJson: attendeeEmailsJson,
                              remindersJson: remindersJson, remindersUseDefault: remindersUseDefault,
                              recurrenceRule: recurrenceRule, recurringRemoteId: recurringRemoteId,
                              originalStartAt: originalStartAt, eventType: eventType,
                              conferenceJson: conferenceJson, attachmentsJson: attachmentsJson,
                              guestPermissionsJson: guestPermissionsJson,
                              statusPropertiesJson: statusPropertiesJson})
    }

    function isCalendarVisible(calendarId) {
        return bypassCalendarVisibility || calendarVisibility === null || calendarVisibility.isVisible(calendarId)
    }

    function isEventSelected(eventId) {
        return selectedEventIds.indexOf(eventId) >= 0
    }

    function configureViewportModels() {
        if (viewportSource === timelineModel) {
            updateViewportFilters()
            return
        }
        viewportSource = timelineModel
        if (timelineModel !== null && typeof timelineModel.createViewport === "function") {
            allDayEventRows = timelineModel.createViewport()
            timedEventRows = timelineModel.createViewport()
        } else {
            allDayEventRows = timelineModel
            timedEventRows = timelineModel
        }
        updateViewportFilters()
    }

    function updateViewportFilters() {
        if (allDayEventRows !== null && typeof allDayEventRows.firstDayIndex === "number") {
            allDayEventRows.firstDayIndex = dayIndex
            allDayEventRows.dayCount = 1
            allDayEventRows.allDay = true
            allDayEventRows.active = timelineActive
            allDayEventRows.filterCalendarVisibility = !bypassCalendarVisibility &&
                                                       calendarVisibility !== null
            allDayEventRows.visibleCalendarIds = calendarVisibility !== null
                                                 ? calendarVisibility.visibleCalendarIds : []
        }
        if (timedEventRows !== null && typeof timedEventRows.firstDayIndex === "number") {
            timedEventRows.firstDayIndex = dayIndex
            timedEventRows.dayCount = 1
            timedEventRows.allDay = false
            timedEventRows.active = timelineActive
            timedEventRows.filterCalendarVisibility = !bypassCalendarVisibility &&
                                                     calendarVisibility !== null
            timedEventRows.visibleCalendarIds = calendarVisibility !== null
                                               ? calendarVisibility.visibleCalendarIds : []
        }
    }

    Component.onCompleted: configureViewportModels()
    onTimelineModelChanged: {
        configureViewportModels()
        Qt.callLater(timelineViewport.updateVisibleMinuteRange)
    }
    onDayIndexChanged: updateViewportFilters()
    onCalendarVisibilityChanged: updateViewportFilters()
    onBypassCalendarVisibilityChanged: updateViewportFilters()
    onTimelineActiveChanged: {
        updateViewportFilters()
        if (timelineActive) Qt.callLater(timelineViewport.updateVisibleMinuteRange)
    }

    Connections {
        target: root.calendarVisibility
        function onVisibleCalendarIdsChanged() { root.updateViewportFilters() }
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
            text: root.dateLabel.length > 0 ? root.dateLabel : "Day"
            font.pixelSize: Theme.titleFontSize
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        Column {
            id: allDayColumn
            Layout.fillWidth: true
            spacing: Theme.spacingSmall

            DropArea {
                anchors.fill: parent
                keys: ["hcb-task"]
                onDropped: function(drop) {
                    if (drop.source !== null && typeof drop.source.taskId === "string") {
                        root.taskMoveRequested(drop.source.taskId, root.dateIso)
                    }
                }
            }

            Repeater {
                model: allDayEventRows

                delegate: CalendarEventButton {
                    required property string id
                    required property string calendarId
                    required property string title
                    required property bool allDay
                    required property int dayIndex
                    required property string description
                    required property string location
                    required property string startAt
                    required property string startTimeZone
                    required property string endAt
                    required property string transparency
                    required property string visibility
                    required property string colorId
                    required property string attendeeEmailsJson
                    required property string remindersJson
                    required property bool remindersUseDefault
                    required property string recurrenceRule
                    required property string recurringRemoteId
                    required property string originalStartAt
                    property string eventType: "default"
                    property string conferenceJson: ""
                    property string attachmentsJson: "[]"
                    property string guestPermissionsJson: "{}"
                    property string statusPropertiesJson: "{}"
                    visible: root.isCalendarVisible(calendarId)
                    width: parent.width
                    compact: true
                    eventColor: root.eventColor(calendarId, colorId)
                    text: title + " — All day"
                    accessibleName: title
                    accessibleDescription: "All-day event"
                    onClicked: {
                        if (root.selectionMode) {
                            root.eventSelectionRequested(id, !root.isEventSelected(id))
                        } else {
                            root.selectEvent(id)
                            root.requestDetail(id, calendarId, title, startAt, endAt, allDay, description,
                                             location, startTimeZone, colorId, transparency, visibility,
                                             attendeeEmailsJson, remindersJson, remindersUseDefault,
                                             recurrenceRule, recurringRemoteId, originalStartAt, eventType,
                                             conferenceJson, attachmentsJson, guestPermissionsJson,
                                             statusPropertiesJson)
                        }
                    }

                    CheckBox {
                        anchors.top: parent.top
                        anchors.right: parent.right
                        anchors.margins: Theme.spacingSmall
                        z: 1
                        visible: root.selectionMode
                        checked: root.isEventSelected(id)
                        Accessible.name: "Select " + title
                        Accessible.description: checked ? "Event selected" : "Event not selected"
                        onClicked: root.eventSelectionRequested(id, checked)
                    }
                }
            }

            Repeater {
                model: root.tasksForDate(root.dateIso)

                delegate: CalendarTaskButton {
                    required property var modelData
                    visible: !root.selectionMode
                    width: parent.width
                    compact: true
                    text: modelData.title
                    accessibleName: "Task: " + modelData.title
                    onClicked: root.taskDetailRequested(modelData)
                }
            }
        }

        Flickable {
            id: timelineViewport
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            contentWidth: width
            contentHeight: timelineCanvas.height
            acceptedButtons: Qt.NoButton

            function revealWorkday() {
                contentY = Math.max(0, Math.min(contentHeight - height,
                                                 root.workdayStartHour * root.hourHeight))
                updateVisibleMinuteRange()
            }

            function updateVisibleMinuteRange() {
                if (!root.timelineActive || timedEventRows === null ||
                        typeof timedEventRows.visibleStartMinute !== "number" ||
                        root.hourHeight <= 0 || height <= 0) {
                    return
                }
                const startMinute = Math.max(0, Math.floor(
                                               (contentY * 60 / root.hourHeight - 60) / 30) * 30)
                const endMinute = Math.min(24 * 60, Math.ceil(
                                             ((contentY + height) * 60 / root.hourHeight + 60) / 30) * 30)
                if (endMinute > startMinute) {
                    timedEventRows.visibleStartMinute = startMinute
                    timedEventRows.visibleEndMinute = endMinute
                }
            }

            Component.onCompleted: Qt.callLater(revealWorkday)
            onHeightChanged: {
                Qt.callLater(revealWorkday)
                Qt.callLater(updateVisibleMinuteRange)
            }
            onContentYChanged: updateVisibleMinuteRange()
            Connections {
                target: root
                function onWorkdayStartHourChanged() { Qt.callLater(timelineViewport.revealWorkday) }
                function onHourHeightChanged() { Qt.callLater(timelineViewport.updateVisibleMinuteRange) }
            }

            Item {
                id: timelineCanvas
                width: timelineViewport.width
                height: root.hourHeight * 24

                DropArea {
                    x: root.timeColumnWidth
                    width: parent.width - root.timeColumnWidth
                    height: parent.height
                    keys: ["hcb-task"]
                    onDropped: function(drop) {
                        if (drop.source !== null && typeof drop.source.taskId === "string") {
                            root.taskMoveRequested(drop.source.taskId, root.dateIso)
                        }
                    }
                }

                Repeater {
                    model: 24

                    delegate: Item {
                        required property int index
                        x: 0
                        y: index * root.hourHeight
                        width: timelineCanvas.width
                        height: root.hourHeight

                        Label {
                            width: root.timeColumnWidth
                            text: root.hourLabel(index)
                            color: Theme.textSecondary
                        }

                        Rectangle {
                            anchors.left: parent.left
                            anchors.leftMargin: root.timeColumnWidth
                            anchors.right: parent.right
                            anchors.top: parent.top
                            height: 1
                            color: Theme.textSecondary
                            opacity: 0.25
                        }
                    }
                }

                Item {
                    id: quickCreateArea
                    objectName: "dayTimedQuickCreateArea"
                    x: root.timeColumnWidth
                    width: timelineCanvas.width - root.timeColumnWidth
                    height: timelineCanvas.height

                    HoverHandler { cursorShape: Qt.CrossCursor }

                    TapHandler {
                        enabled: !root.selectionMode
                        acceptedButtons: Qt.LeftButton
                        onTapped: function(eventPoint) {
                            const minute = root.dropMinute(eventPoint.position.y)
                            root.quickCreateAt(minute, minute + 15)
                        }
                    }

                    DragHandler {
                        id: dayTimedCreateDrag
                        enabled: !root.selectionMode
                        target: null
                        acceptedButtons: Qt.LeftButton
                        cursorShape: Qt.CrossCursor
                        grabPermissions: PointerHandler.CanTakeOverFromItems |
                                         PointerHandler.ApprovesTakeOverByAnything
                        property bool creating: false
                        property int pressMinute: 0
                        onActiveChanged: {
                            if (active) {
                                creating = true
                                pressMinute = root.dropMinute(centroid.pressPosition.y)
                                root.showTimedPreview("New event", pressMinute,
                                                      root.dropEndMinute(centroid.position.y))
                            } else if (creating) {
                                root.quickCreateAt(pressMinute, root.dropEndMinute(centroid.position.y))
                                root.clearDragPreview()
                                creating = false
                            }
                        }
                        onTranslationChanged: {
                            if (active) root.showTimedPreview("New event", pressMinute,
                                                              root.dropEndMinute(centroid.position.y))
                        }
                        onCanceled: function() {
                            creating = false
                            root.clearDragPreview()
                        }
                    }
                }

                Rectangle {
                    visible: root.dragPreviewActive
                    x: root.timeColumnWidth
                    y: root.timePosition(root.dragPreviewStartMinute)
                    width: timelineCanvas.width - root.timeColumnWidth
                    height: Math.max(15, root.timePosition(root.dragPreviewEndMinute) - y)
                    color: Qt.rgba(Theme.accent.r, Theme.accent.g, Theme.accent.b, 0.12)
                    border.color: Theme.accent
                    border.width: 1
                    radius: 4
                    z: 1
                }

                CalendarEventButton {
                    visible: root.dragPreviewActive
                    x: root.timeColumnWidth + 2
                    y: root.timePosition(root.dragPreviewStartMinute) + 2
                    width: Math.max(1, timelineCanvas.width - root.timeColumnWidth - 4)
                    height: Math.max(20, root.timePosition(root.dragPreviewEndMinute) -
                                     root.timePosition(root.dragPreviewStartMinute) - 4)
                    z: 4
                    enabled: false
                    opacity: 0.48
                    eventColor: Theme.accent
                    text: root.dragPreviewTitle
                }

                Repeater {
                    id: eventRows
                    model: timedEventRows

                    delegate: CalendarEventButton {
                        required property string id
                        required property string calendarId
                        required property string title
                        required property bool allDay
                        required property int dayIndex
                        required property int startMinute
                        required property int durationMinutes
                        required property int laneIndex
                        required property int laneCount
                        required property string description
                        required property string location
                        required property string startAt
                        required property string startTimeZone
                        required property string endAt
                        required property string transparency
                        required property string visibility
                        required property string colorId
                        required property string attendeeEmailsJson
                        required property string remindersJson
                        required property bool remindersUseDefault
                        required property string recurrenceRule
                        required property string recurringRemoteId
                        required property string originalStartAt
                        property string eventType: "default"
                        property string conferenceJson: ""
                        property string attachmentsJson: "[]"
                        property string guestPermissionsJson: "{}"
                        property string statusPropertiesJson: "{}"
                        visible: root.isCalendarVisible(calendarId)
                        x: root.timeColumnWidth + laneIndex *
                           (timelineCanvas.width - root.timeColumnWidth) / Math.max(1, laneCount) + 2
                        y: root.timePosition(startMinute)
                        width: Math.max(1, (timelineCanvas.width - root.timeColumnWidth) /
                                        Math.max(1, laneCount) - 4)
                        height: root.eventHeight(durationMinutes)
                        eventColor: root.eventColor(calendarId, colorId)
                        text: title + (height >= 42 ? "\n" + root.timeRange(startAt, endAt) : "")
                        accessibleName: title
                        accessibleDescription: "Timed event"
                        onClicked: {
                            if (root.selectionMode) {
                                root.eventSelectionRequested(id, !root.isEventSelected(id))
                            } else {
                                root.selectEvent(id)
                                root.requestDetail(id, calendarId, title, startAt, endAt, allDay, description,
                                                 location, startTimeZone, colorId, transparency, visibility,
                                                 attendeeEmailsJson, remindersJson, remindersUseDefault,
                                                 recurrenceRule, recurringRemoteId, originalStartAt, eventType,
                                                 conferenceJson, attachmentsJson, guestPermissionsJson,
                                                 statusPropertiesJson)
                            }
                        }

                        HoverHandler { id: eventHover }

                        DragHandler {
                            id: moveHandler
                            enabled: !root.selectionMode
                            target: null
                            onTranslationChanged: {
                                const targetMinute = root.dropMinute(parent.y + activeTranslation.y)
                                root.showTimedPreview(title, targetMinute,
                                                      targetMinute + durationMinutes)
                            }
                            onActiveChanged: {
                                const targetMinute = root.dropMinute(parent.y + activeTranslation.y)
                                if (!active && !resizeHandler.active && targetMinute !== startMinute) {
                                    root.requestMove(id, root.dayIndex, targetMinute,
                                                     {id: id, title: title, allDay: allDay,
                                                      recurrenceRule: recurrenceRule,
                                                      recurringRemoteId: recurringRemoteId,
                                                      originalStartAt: originalStartAt})
                                }
                                if (!active) root.clearDragPreview()
                            }
                        }

                        opacity: moveHandler.active ? 0.7 : 1

                        CheckBox {
                            anchors.top: parent.top
                            anchors.right: parent.right
                            anchors.margins: Theme.spacingSmall
                            z: 1
                            visible: root.selectionMode
                            checked: root.isEventSelected(id)
                            Accessible.name: "Select " + title
                            Accessible.description: checked ? "Event selected" : "Event not selected"
                            onClicked: root.eventSelectionRequested(id, checked)
                        }

                        AccessibleButton {
                            id: resizeHandle
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.bottom: parent.bottom
                            height: 14
                            visible: eventHover.hovered || resizeHandler.active
                            padding: 0
                            text: ""
                            accessibleName: "Resize " + title + " end"
                            accessibleDescription: "Drag to change the end time"
                            onClicked: root.requestResize(id, root.dayIndex,
                                                          Math.min(24 * 60, startMinute + durationMinutes + 15))

                            background: Rectangle {
                                color: root.eventColor(calendarId, colorId)
                                opacity: resizeHandler.active ? 1 : 0.65
                                radius: 1
                            }

                            DragHandler {
                                id: resizeHandler
                                enabled: !root.selectionMode
                                target: null
                                cursorShape: Qt.SizeVerCursor
                                grabPermissions: PointerHandler.CanTakeOverFromAnything
                                onTranslationChanged: {
                                    root.showTimedPreview(title, startMinute, root.dropEndMinute(
                                                              parent.parent.y + parent.parent.height +
                                                              activeTranslation.y))
                                }
                                onActiveChanged: {
                                    const targetEndMinute = root.dropEndMinute(
                                                parent.parent.y + parent.parent.height + activeTranslation.y)
                                    const initialEndMinute = Math.min(24 * 60,
                                                                      startMinute + durationMinutes)
                                    if (!active && targetEndMinute !== initialEndMinute) {
                                        root.requestResize(id, root.dayIndex, targetEndMinute,
                                                           {id: id, title: title, allDay: allDay,
                                                            recurrenceRule: recurrenceRule,
                                                            recurringRemoteId: recurringRemoteId,
                                                            originalStartAt: originalStartAt})
                                    }
                                    if (!active) root.clearDragPreview()
                                }
                            }
                        }
                    }
                }
            }
        }
    }

    Label {
        anchors.centerIn: parent
        visible: root.timelineModel === null || root.timelineModel.totalItemCount === 0
        text: "No events today."
        color: Theme.textSecondary
    }
}
