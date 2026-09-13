import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var timelineModel: null
    property var calendarVisibility: null
    property var selectedEventIds: []
    property bool selectionMode: false
    property var dayLabels: []
    property string weekStartDate: ""
    property var scheduledTaskIndex: null
    property int dayCount: 7
    property int hourHeight: 48
    property bool use24HourTime: true
    property int workdayStartHour: 9
    property int timeColumnWidth: 64
    property int allDayLaneHeight: 28
    property int allDayLaneCount: 2
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
    signal eventAllDayResizeScopeRequested(var event, string startAt, string endAt)
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
    property bool dragPreviewAllDay: false
    property int dragPreviewStartDay: 0
    property int dragPreviewEndDay: 0
    property int dragPreviewStartMinute: 0
    property int dragPreviewEndMinute: 15
    property string dragPreviewTitle: ""

    function dayColumnWidth(availableWidth) {
        return (availableWidth - timeColumnWidth) / dayCount
    }

    function dayPosition(dayIndex, availableWidth) {
        return timeColumnWidth + dayIndex * dayColumnWidth(availableWidth)
    }

    function timePosition(minute) {
        return minute * hourHeight / 60
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

    function dropDayIndex(x, availableWidth) {
        const point = timelineModel !== null && typeof timelineModel.timelinePointInput === "function"
                ? timelineModel.timelinePointInput(x, 0, availableWidth, timeColumnWidth, hourHeight, false) : null
        return point !== null && typeof point.dayIndex === "number" ? point.dayIndex : 0
    }

    function dropMinute(y) {
        const point = timelineModel !== null && typeof timelineModel.timelinePointInput === "function"
                ? timelineModel.timelinePointInput(timeColumnWidth, y, timeColumnWidth + dayCount,
                                                   timeColumnWidth, hourHeight, false) : null
        return point !== null && typeof point.minute === "number" ? point.minute : 0
    }

    function dropEndMinute(y) {
        const point = timelineModel !== null && typeof timelineModel.timelinePointInput === "function"
                ? timelineModel.timelinePointInput(timeColumnWidth, y, timeColumnWidth + dayCount,
                                                   timeColumnWidth, hourHeight, true) : null
        return point !== null && typeof point.minute === "number" ? point.minute : 15
    }

    function normalisedTimedRange(firstDay, firstMinute, lastDay, lastMinute) {
        let startDay = Math.max(0, Math.min(dayCount - 1, firstDay))
        let endDay = Math.max(0, Math.min(dayCount - 1, lastDay))
        let startMinute = Math.max(0, Math.min(24 * 60 - 15, firstMinute))
        let endMinute = Math.max(0, Math.min(24 * 60, lastMinute))
        if (endDay < startDay || (endDay === startDay && endMinute < startMinute)) {
            const day = startDay
            const minute = startMinute
            startDay = endDay
            startMinute = endMinute
            endDay = day
            endMinute = minute
        }
        if (endDay === startDay && endMinute === startMinute) {
            endMinute = Math.min(24 * 60, startMinute + 15)
        }
        return { startDay: startDay, startMinute: startMinute, endDay: endDay,
                 endMinute: endMinute }
    }

    function quickCreateTimed(firstDay, firstMinute, lastDay, lastMinute) {
        if (timelineModel === null || typeof timelineModel.timedRangeInput !== "function") return
        const range = normalisedTimedRange(firstDay, firstMinute, lastDay, lastMinute)
        const input = timelineModel.timedRangeInput(range.startDay, range.startMinute,
                                                    range.endDay, range.endMinute)
        if (typeof input.startAt === "string" && typeof input.endAt === "string") {
            quickCreateRequested(input.startAt, input.endAt, false)
        }
    }

    function quickCreateAllDay(firstDay, lastDay) {
        if (timelineModel === null || typeof timelineModel.allDayRangeInput !== "function") return
        const input = timelineModel.allDayRangeInput(firstDay, lastDay)
        if (typeof input.startAt === "string" && typeof input.endAt === "string") {
            quickCreateRequested(input.startAt, input.endAt, true)
        }
    }

    function dateForTimelineDay(dayIndex) {
        return timelineModel !== null && typeof timelineModel.dateForDayIndex === "function"
                ? timelineModel.dateForDayIndex(dayIndex) : ""
    }

    function taskDayIndexForDate(dueAt) {
        return timelineModel !== null && typeof timelineModel.dayIndexForDate === "function"
                ? timelineModel.dayIndexForDate((dueAt || "").slice(0, 10)) : -1
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

    function tasksForWeek() {
        const revision = scheduledTaskIndex !== null && scheduledTaskIndex !== undefined &&
                       typeof scheduledTaskIndex.revision === "number"
                       ? scheduledTaskIndex.revision : 0
        if (revision < 0) return []
        return scheduledTaskIndex !== null && scheduledTaskIndex !== undefined &&
               typeof scheduledTaskIndex.tasksForRange === "function"
               ? scheduledTaskIndex.tasksForRange(weekStartDate, dateForTimelineDay(dayCount - 1)) : []
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

    function requestAllDayMove(eventId, targetDayIndex, sourceEvent) {
        if (timelineModel === null || typeof timelineModel.moveAllDayInput !== "function") {
            return
        }
        const move = timelineModel.moveAllDayInput(eventId, targetDayIndex)
        if (move === null || typeof move.id !== "string" || typeof move.startAt !== "string" ||
                typeof move.endAt !== "string" || move.allDay !== true) {
            return
        }
        if (sourceEvent !== undefined) eventMoveScopeRequested(sourceEvent, move.startAt, move.endAt, true)
        else eventMoveRequested(move.id, move.startAt, move.endAt, true)
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

    function requestAllDayResize(eventId, targetStartDayIndex, targetEndDayIndex, sourceEvent) {
        if (timelineModel === null || typeof timelineModel.resizeAllDayRangeInput !== "function") {
            return
        }
        const resize = timelineModel.resizeAllDayRangeInput(eventId, targetStartDayIndex,
                                                             targetEndDayIndex)
        if (resize === null || typeof resize.id !== "string" || typeof resize.startAt !== "string" ||
                typeof resize.endAt !== "string") {
            return
        }
        if (sourceEvent !== undefined) {
            eventAllDayResizeScopeRequested(sourceEvent, resize.startAt, resize.endAt)
        } else {
            eventMoveRequested(resize.id, resize.startAt, resize.endAt, true)
        }
    }

    function showAllDayPreview(title, startDay, endDay) {
        dragPreviewActive = true
        dragPreviewAllDay = true
        dragPreviewTitle = title
        dragPreviewStartDay = Math.max(0, Math.min(dayCount - 1, startDay))
        dragPreviewEndDay = Math.max(dragPreviewStartDay, Math.min(dayCount - 1, endDay))
    }

    function showTimedPreview(title, firstDay, firstMinute, lastDay, lastMinute) {
        const range = normalisedTimedRange(firstDay, firstMinute, lastDay, lastMinute)
        dragPreviewActive = true
        dragPreviewAllDay = false
        dragPreviewTitle = title
        dragPreviewStartDay = range.startDay
        dragPreviewEndDay = range.endDay
        dragPreviewStartMinute = range.startMinute
        dragPreviewEndMinute = range.endMinute
    }

    function timedPreviewSegments() {
        if (!dragPreviewActive || dragPreviewAllDay) return []
        const segments = []
        for (let day = dragPreviewStartDay; day <= dragPreviewEndDay; ++day) {
            const startMinute = day === dragPreviewStartDay ? dragPreviewStartMinute : 0
            const endMinute = day === dragPreviewEndDay ? dragPreviewEndMinute : 24 * 60
            if (endMinute > startMinute) {
                segments.push({ dayIndex: day, startMinute: startMinute, endMinute: endMinute })
            }
        }
        return segments
    }

    function clearDragPreview() {
        dragPreviewActive = false
        dragPreviewTitle = ""
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
            allDayEventRows.firstDayIndex = 0
            allDayEventRows.dayCount = dayCount
            allDayEventRows.allDay = true
            allDayEventRows.active = timelineActive
            allDayEventRows.filterCalendarVisibility = !bypassCalendarVisibility &&
                                                       calendarVisibility !== null
            allDayEventRows.visibleCalendarIds = calendarVisibility !== null
                                                 ? calendarVisibility.visibleCalendarIds : []
        }
        if (timedEventRows !== null && typeof timedEventRows.firstDayIndex === "number") {
            timedEventRows.firstDayIndex = 0
            timedEventRows.dayCount = dayCount
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
    onDayCountChanged: updateViewportFilters()
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
            text: "Week"
            font.pixelSize: Theme.titleFontSize
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        Item {
            id: dayHeader
            Layout.fillWidth: true
            Layout.preferredHeight: root.allDayLaneHeight * (root.allDayLaneCount + 2)

            DropArea {
                x: root.timeColumnWidth
                width: parent.width - root.timeColumnWidth
                height: parent.height
                keys: ["hcb-task"]
                onDropped: function(drop) {
                    if (drop.source !== null && typeof drop.source.taskId === "string") {
                        root.taskMoveRequested(drop.source.taskId,
                                               root.dateForTimelineDay(root.dropDayIndex(drop.x, parent.width)))
                    }
                }
            }

            Repeater {
                model: root.dayCount

                delegate: Label {
                    required property int index
                    x: root.dayPosition(index, dayHeader.width)
                    width: root.dayColumnWidth(dayHeader.width)
                    text: root.dayLabels.length === root.dayCount ? root.dayLabels[index]
                                                                  : "Day " + (index + 1)
                    horizontalAlignment: Text.AlignHCenter
                    Accessible.name: text
                }
            }

            Rectangle {
                visible: root.dragPreviewActive && root.dragPreviewAllDay
                x: root.dayPosition(root.dragPreviewStartDay, dayHeader.width)
                y: root.allDayLaneHeight
                width: (root.dragPreviewEndDay - root.dragPreviewStartDay + 1) *
                       root.dayColumnWidth(dayHeader.width)
                height: root.allDayLaneHeight
                color: Qt.rgba(Theme.accent.r, Theme.accent.g, Theme.accent.b, 0.12)
                border.color: Theme.accent
                border.width: 1
                radius: 4
            }

            CalendarEventButton {
                visible: root.dragPreviewActive && root.dragPreviewAllDay
                x: root.dayPosition(root.dragPreviewStartDay, dayHeader.width) + 2
                y: root.allDayLaneHeight + 2
                width: Math.max(1, (root.dragPreviewEndDay - root.dragPreviewStartDay + 1) *
                                root.dayColumnWidth(dayHeader.width) - 4)
                height: root.allDayLaneHeight - 4
                z: 4
                compact: true
                enabled: false
                opacity: 0.48
                eventColor: Theme.accent
                text: root.dragPreviewTitle
            }

            Repeater {
                model: allDayEventRows

                delegate: CalendarEventButton {
                    required property string id
                    required property string calendarId
                    required property string title
                    required property bool allDay
                    required property int dayIndex
                    required property int laneIndex
                    required property int daySpan
                    required property bool startsBeforeRange
                    required property bool endsAfterRange
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
                    x: root.dayPosition(dayIndex, dayHeader.width)
                    y: root.allDayLaneHeight + laneIndex * root.allDayLaneHeight
                    width: daySpan * root.dayColumnWidth(dayHeader.width)
                    height: root.allDayLaneHeight
                    compact: true
                    eventColor: root.eventColor(calendarId, colorId)
                    text: (startsBeforeRange ? "‹ " : "") + title + (endsAfterRange ? " ›" : "")
                    accessibleName: title
                    accessibleDescription: "All-day event, starting day " + (dayIndex + 1)
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

                    HoverHandler { id: allDayHover }

                    DragHandler {
                        id: allDayMoveHandler
                        enabled: !root.selectionMode && !endsAfterRange
                        target: null
                        onTranslationChanged: {
                            root.showAllDayPreview(title,
                                                   root.dropDayIndex(parent.x + activeTranslation.x,
                                                                     dayHeader.width),
                                                   root.dropDayIndex(parent.x + activeTranslation.x,
                                                                     dayHeader.width) + daySpan - 1)
                        }
                        onActiveChanged: {
                            const targetDay = root.dropDayIndex(parent.x + activeTranslation.x,
                                                                dayHeader.width)
                            if (!active && !allDayStartResizeHandler.active &&
                                    !allDayEndResizeHandler.active && targetDay !== dayIndex) {
                                root.requestAllDayMove(id, targetDay,
                                                       {id: id, title: title, allDay: allDay,
                                                        recurrenceRule: recurrenceRule,
                                                        recurringRemoteId: recurringRemoteId,
                                                        originalStartAt: originalStartAt})
                            }
                            if (!active) root.clearDragPreview()
                        }
                    }

                    AccessibleButton {
                        id: allDayStartResizeHandle
                        anchors.top: parent.top
                        anchors.left: parent.left
                        anchors.bottom: parent.bottom
                        width: 12
                        visible: allDayHover.hovered || allDayStartResizeHandler.active
                        padding: 0
                        text: ""
                        enabled: !root.selectionMode && !startsBeforeRange && !endsAfterRange
                        accessibleName: "Resize " + title + " start"
                        accessibleDescription: "Drag to change the all-day start date"
                        background: Rectangle { color: root.eventColor(calendarId, colorId); opacity: 0.7; radius: 2 }

                        DragHandler {
                            id: allDayStartResizeHandler
                            enabled: !root.selectionMode && !startsBeforeRange && !endsAfterRange
                            target: null
                            cursorShape: Qt.SizeHorCursor
                            grabPermissions: PointerHandler.CanTakeOverFromAnything
                            onTranslationChanged: {
                                root.showAllDayPreview(title,
                                                       root.dropDayIndex(parent.parent.x + activeTranslation.x,
                                                                         dayHeader.width),
                                                       dayIndex + daySpan - 1)
                            }
                            onActiveChanged: {
                                const targetDay = root.dropDayIndex(parent.parent.x + activeTranslation.x,
                                                                    dayHeader.width)
                                const initialStartDay = dayIndex
                                if (!active && targetDay !== initialStartDay) {
                                    root.requestAllDayResize(id, targetDay, dayIndex + daySpan - 1,
                                                              {id: id, title: title, allDay: allDay,
                                                               recurrenceRule: recurrenceRule,
                                                               recurringRemoteId: recurringRemoteId,
                                                               originalStartAt: originalStartAt})
                                }
                                if (!active) root.clearDragPreview()
                            }
                        }
                    }

                    AccessibleButton {
                        id: allDayEndResizeHandle
                        anchors.top: parent.top
                        anchors.right: parent.right
                        anchors.bottom: parent.bottom
                        width: 14
                        visible: allDayHover.hovered || allDayEndResizeHandler.active
                        padding: 0
                        text: ""
                        enabled: !root.selectionMode && !startsBeforeRange && !endsAfterRange
                        accessibleName: "Resize " + title + " end"
                        accessibleDescription: "Drag to change the all-day end date"
                        background: Rectangle { color: root.eventColor(calendarId, colorId); opacity: 0.7; radius: 2 }

                        DragHandler {
                            id: allDayEndResizeHandler
                            enabled: !root.selectionMode && !startsBeforeRange && !endsAfterRange
                            target: null
                            cursorShape: Qt.SizeHorCursor
                            grabPermissions: PointerHandler.CanTakeOverFromAnything
                            onTranslationChanged: {
                                root.showAllDayPreview(title, dayIndex,
                                                       root.dropDayIndex(parent.parent.x + parent.parent.width - 1 +
                                                                         activeTranslation.x, dayHeader.width))
                            }
                            onActiveChanged: {
                                const targetDay = root.dropDayIndex(
                                            parent.parent.x + parent.parent.width - 1 + activeTranslation.x,
                                            dayHeader.width)
                                const initialEndDay = dayIndex + daySpan - 1
                                if (!active && targetDay !== initialEndDay) {
                                    root.requestAllDayResize(id, dayIndex, targetDay,
                                                              {id: id, title: title, allDay: allDay,
                                                               recurrenceRule: recurrenceRule,
                                                               recurringRemoteId: recurringRemoteId,
                                                               originalStartAt: originalStartAt})
                                }
                                if (!active) root.clearDragPreview()
                            }
                        }
                    }

                    CheckBox {
                        anchors.top: parent.top
                        anchors.right: parent.right
                        anchors.margins: 2
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
                model: root.tasksForWeek()

                delegate: CalendarTaskButton {
                    required property var modelData
                    property int taskDayIndex: root.taskDayIndexForDate(modelData.dueAt)
                    visible: !root.selectionMode && taskDayIndex >= 0 && taskDayIndex < root.dayCount
                    x: root.dayPosition(taskDayIndex, dayHeader.width) + 2
                    y: root.allDayLaneHeight * (root.allDayLaneCount + 1)
                    width: Math.max(1, root.dayColumnWidth(dayHeader.width) - 4)
                    height: root.allDayLaneHeight - 1
                    compact: true
                    text: modelData.title
                    accessibleName: "Task: " + modelData.title
                    onClicked: root.taskDetailRequested(modelData)

                    DragHandler {
                        enabled: !root.selectionMode
                        target: null
                        onActiveChanged: {
                            const targetDay = root.dropDayIndex(parent.x + activeTranslation.x,
                                                                dayHeader.width)
                            if (!active && targetDay !== taskDayIndex) {
                                root.taskMoveRequested(modelData.id, root.dateForTimelineDay(targetDay))
                            }
                        }
                    }
                }
            }

            Item {
                id: allDayQuickCreateArea
                objectName: "weekAllDayQuickCreateArea"
                x: root.timeColumnWidth
                width: dayHeader.width - root.timeColumnWidth
                height: dayHeader.height
                z: -1

                HoverHandler { cursorShape: Qt.CrossCursor }

                TapHandler {
                    enabled: !root.selectionMode
                    acceptedButtons: Qt.LeftButton
                    onTapped: function(eventPoint) {
                        const day = root.dropDayIndex(eventPoint.position.x + parent.x, dayHeader.width)
                        root.quickCreateAllDay(day, day)
                    }
                }

                DragHandler {
                    id: weekAllDayCreateDrag
                    enabled: !root.selectionMode
                    target: null
                    acceptedButtons: Qt.LeftButton
                    cursorShape: Qt.CrossCursor
                    grabPermissions: PointerHandler.CanTakeOverFromItems |
                                     PointerHandler.ApprovesTakeOverByAnything
                    property bool creating: false
                    property int pressDay: 0
                    onActiveChanged: {
                        if (active) {
                            creating = true
                            pressDay = root.dropDayIndex(centroid.pressPosition.x + parent.x, dayHeader.width)
                            root.showAllDayPreview("New event", pressDay,
                                                   root.dropDayIndex(centroid.position.x + parent.x,
                                                                     dayHeader.width))
                        } else if (creating) {
                            root.quickCreateAllDay(pressDay,
                                                   root.dropDayIndex(centroid.position.x + parent.x,
                                                                     dayHeader.width))
                            root.clearDragPreview()
                            creating = false
                        }
                    }
                    onTranslationChanged: {
                        if (active) root.showAllDayPreview("New event", pressDay,
                                                           root.dropDayIndex(centroid.position.x + parent.x,
                                                                             dayHeader.width))
                    }
                    onCanceled: function() {
                        creating = false
                        root.clearDragPreview()
                    }
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
                            root.taskMoveRequested(drop.source.taskId,
                                                   root.dateForTimelineDay(root.dropDayIndex(drop.x, parent.width)))
                        }
                    }
                }

                Repeater {
                    model: 24

                    delegate: Item {
                        required property int index
                        y: index * root.hourHeight
                        width: timelineCanvas.width
                        height: root.hourHeight

                        Label {
                            width: root.timeColumnWidth
                            text: root.hourLabel(index)
                            color: Theme.textSecondary
                        }

                        Repeater {
                            model: root.dayCount

                            delegate: Rectangle {
                                required property int index
                                x: root.dayPosition(index, timelineCanvas.width)
                                width: root.dayColumnWidth(timelineCanvas.width)
                                height: 1
                                color: Theme.textSecondary
                                opacity: 0.25
                            }
                        }
                    }
                }

                Item {
                    id: timedQuickCreateArea
                    objectName: "weekTimedQuickCreateArea"
                    x: root.timeColumnWidth
                    width: timelineCanvas.width - root.timeColumnWidth
                    height: timelineCanvas.height

                    HoverHandler { cursorShape: Qt.CrossCursor }

                    TapHandler {
                        enabled: !root.selectionMode
                        acceptedButtons: Qt.LeftButton
                        onTapped: function(eventPoint) {
                            const day = root.dropDayIndex(eventPoint.position.x + parent.x, timelineCanvas.width)
                            const minute = root.dropMinute(eventPoint.position.y)
                            root.quickCreateTimed(day, minute, day, minute + 15)
                        }
                    }

                    DragHandler {
                        id: weekTimedCreateDrag
                        enabled: !root.selectionMode
                        target: null
                        acceptedButtons: Qt.LeftButton
                        cursorShape: Qt.CrossCursor
                        grabPermissions: PointerHandler.CanTakeOverFromItems |
                                         PointerHandler.ApprovesTakeOverByAnything
                        property bool creating: false
                        property int pressDay: 0
                        property int pressMinute: 0
                        onActiveChanged: {
                            if (active) {
                                creating = true
                                pressDay = root.dropDayIndex(centroid.pressPosition.x + parent.x,
                                                             timelineCanvas.width)
                                pressMinute = root.dropMinute(centroid.pressPosition.y)
                                root.showTimedPreview("New event", pressDay, pressMinute,
                                                      root.dropDayIndex(centroid.position.x + parent.x,
                                                                        timelineCanvas.width),
                                                      root.dropEndMinute(centroid.position.y))
                            } else if (creating) {
                                root.quickCreateTimed(pressDay, pressMinute,
                                                      root.dropDayIndex(centroid.position.x + parent.x,
                                                                        timelineCanvas.width),
                                                      root.dropEndMinute(centroid.position.y))
                                root.clearDragPreview()
                                creating = false
                            }
                        }
                        onTranslationChanged: {
                            if (active) root.showTimedPreview(
                                            "New event", pressDay, pressMinute,
                                            root.dropDayIndex(centroid.position.x + parent.x,
                                                              timelineCanvas.width),
                                            root.dropEndMinute(centroid.position.y))
                        }
                        onCanceled: function() {
                            creating = false
                            root.clearDragPreview()
                        }
                    }
                }

                Repeater {
                    model: root.timedPreviewSegments()
                    delegate: Rectangle {
                        required property var modelData
                        x: root.dayPosition(modelData.dayIndex, timelineCanvas.width)
                        y: root.timePosition(modelData.startMinute)
                        width: root.dayColumnWidth(timelineCanvas.width)
                        height: Math.max(15, root.timePosition(modelData.endMinute) - y)
                        color: Qt.rgba(Theme.accent.r, Theme.accent.g, Theme.accent.b, 0.12)
                        border.color: Theme.accent
                        border.width: 1
                        radius: 4
                        z: 1
                    }
                }

                Repeater {
                    model: root.timedPreviewSegments()
                    delegate: CalendarEventButton {
                        required property var modelData
                        x: root.dayPosition(modelData.dayIndex, timelineCanvas.width) + 2
                        y: root.timePosition(modelData.startMinute) + 2
                        width: Math.max(1, root.dayColumnWidth(timelineCanvas.width) - 4)
                        height: Math.max(20, root.timePosition(modelData.endMinute) -
                                         root.timePosition(modelData.startMinute) - 4)
                        z: 4
                        enabled: false
                        opacity: 0.48
                        eventColor: Theme.accent
                        text: root.dragPreviewTitle
                    }
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
                        x: root.dayPosition(dayIndex, timelineCanvas.width) + laneIndex *
                           root.dayColumnWidth(timelineCanvas.width) / Math.max(1, laneCount) + 2
                        y: root.timePosition(startMinute)
                        width: Math.max(1, root.dayColumnWidth(timelineCanvas.width) /
                                        Math.max(1, laneCount) - 4)
                        height: Math.max(24, durationMinutes * root.hourHeight / 60)
                        eventColor: root.eventColor(calendarId, colorId)
                        text: title + (height >= 42 ? "\n" + root.timeRange(startAt, endAt) : "")
                        accessibleName: title
                        accessibleDescription: "Timed event, day " + (dayIndex + 1)
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
                                const targetDay = root.dropDayIndex(parent.x + activeTranslation.x,
                                                                    timelineCanvas.width)
                                const targetMinute = root.dropMinute(parent.y + activeTranslation.y)
                                root.showTimedPreview(title, targetDay, targetMinute, targetDay,
                                                      targetMinute + durationMinutes)
                            }
                            onActiveChanged: {
                                const targetDay = root.dropDayIndex(parent.x + activeTranslation.x,
                                                                    timelineCanvas.width)
                                const targetMinute = root.dropMinute(parent.y + activeTranslation.y)
                                if (!active && !resizeHandler.active &&
                                        (targetDay !== dayIndex || targetMinute !== startMinute)) {
                                    root.requestMove(id, targetDay, targetMinute,
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
                            anchors.margins: 2
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
                            onClicked: root.requestResize(id, dayIndex,
                                                          Math.min(24 * 60,
                                                                   startMinute + durationMinutes + 15),
                                                          {id: id, title: title, allDay: allDay,
                                                           recurrenceRule: recurrenceRule,
                                                           recurringRemoteId: recurringRemoteId,
                                                           originalStartAt: originalStartAt})

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
                                    const targetDay = root.dropDayIndex(
                                                parent.parent.x + parent.parent.width - 1 + activeTranslation.x,
                                                timelineCanvas.width)
                                    const targetEndMinute = root.dropEndMinute(
                                                parent.parent.y + parent.parent.height + activeTranslation.y)
                                    root.showTimedPreview(title, dayIndex, startMinute, targetDay,
                                                          targetEndMinute)
                                }
                                onActiveChanged: {
                                    const targetEndDay = root.dropDayIndex(
                                                parent.parent.x + parent.parent.width - 1 + activeTranslation.x,
                                                timelineCanvas.width)
                                    const targetEndMinute = root.dropEndMinute(
                                                parent.parent.y + parent.parent.height + activeTranslation.y)
                                    const initialEndMinute = Math.min(24 * 60,
                                                                      startMinute + durationMinutes)
                                    if (!active && (targetEndDay !== dayIndex ||
                                                    targetEndMinute !== initialEndMinute)) {
                                        root.requestResize(id, targetEndDay, targetEndMinute,
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
        text: "No events this week."
        color: Theme.textSecondary
    }
}
