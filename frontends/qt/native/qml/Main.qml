import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import QtQml.Models

ApplicationWindow {
    id: window
    width: 1200
    height: 760
    minimumWidth: 900
    minimumHeight: 600
    visible: true
    title: "Hot Cross Buns"
    property bool timelineProfile: false
    property string currentPage: timelineProfile ? "Calendar" : "Tasks"
    required property var navigationCommands
    property var agendaModel: null
    property var appController: null
    property var calendarSourceModel: null
    property var monthGridModel: null
    property var notesModel: null
    property var searchResultsModel: null
    property var taskListModel: null
    property var taskModel: null
    property var timelineModel: null
    property var unscheduledTasks: appController !== null && appController.unscheduledTasks !== undefined
                                  ? appController.unscheduledTasks : []
    property string undoLabel: appController !== null && typeof appController.undoLabel === "string"
                               ? appController.undoLabel : ""
    property string redoLabel: appController !== null && typeof appController.redoLabel === "string"
                               ? appController.redoLabel : ""
    property int pendingSyncCount: appController !== null && typeof appController.pendingSyncCount === "number"
                                   ? appController.pendingSyncCount : 0
    property int undoRetentionDays: appController !== null && typeof appController.undoRetentionDays === "number"
                                    ? appController.undoRetentionDays : 30
    property int undoMaximumEntries: appController !== null && typeof appController.undoMaximumEntries === "number"
                                      ? appController.undoMaximumEntries : 200
    property bool notesEnabled: appController !== null && appController.notesEnabled === true
    property int notesProjectionMode: appController !== null &&
                                      typeof appController.notesProjectionMode === "number"
                                      ? appController.notesProjectionMode : 0
    property int appearanceMode: appController !== null && typeof appController.appearanceMode === "number"
                                 ? appController.appearanceMode : 0
    property int visualDensity: appController !== null && typeof appController.visualDensity === "number"
                                ? appController.visualDensity : 1
    property int taskListPaneWidth: appController !== null && typeof appController.taskListPaneWidth === "number"
                                    ? appController.taskListPaneWidth : 260
    property int paletteMode: appController !== null && typeof appController.paletteMode === "number"
                              ? appController.paletteMode : 0
    property string accentColor: appController !== null && typeof appController.accentColor === "string"
                                 ? appController.accentColor : ""
    property string fontFamily: appController !== null && typeof appController.fontFamily === "string"
                                ? appController.fontFamily : ""
    property int fontScale: appController !== null && typeof appController.fontScale === "number"
                            ? appController.fontScale : 1
    property string externalBrowser: appController !== null && typeof appController.externalBrowser === "string"
                                    ? appController.externalBrowser : ""
    property int bulkTextRecurrenceScope: appController !== null &&
                                          typeof appController.bulkTextRecurrenceScope === "number"
                                          ? appController.bulkTextRecurrenceScope : 2
    property int bulkTaskPreviewRequestToken: appController !== null &&
                                              typeof appController.bulkTaskPreviewRequestToken === "number"
                                              ? appController.bulkTaskPreviewRequestToken : -1
    property int bulkEventPreviewRequestToken: appController !== null &&
                                               typeof appController.bulkEventPreviewRequestToken === "number"
                                               ? appController.bulkEventPreviewRequestToken : -1
    property string quickCaptureDefaultTaskListId: appController !== null &&
                                                   typeof appController.quickCaptureDefaultTaskListId === "string"
                                                   ? appController.quickCaptureDefaultTaskListId : ""
    property string quickCaptureDefaultCalendarId: appController !== null &&
                                                   typeof appController.quickCaptureDefaultCalendarId === "string"
                                                   ? appController.quickCaptureDefaultCalendarId : ""
    property int weekStartDay: appController !== null && typeof appController.weekStartDay === "number"
                               ? appController.weekStartDay : 0
    property bool use24HourTime: appController === null || appController.use24HourTime !== false
    property int workdayStartHour: appController !== null && typeof appController.workdayStartHour === "number"
                                   ? appController.workdayStartHour : 9
    property int workdayEndHour: appController !== null && typeof appController.workdayEndHour === "number"
                                 ? appController.workdayEndHour : 17
    property var transitionTimings: null
    property var selectedCalendarEventIds: []
    property bool calendarSelectionMode: false
    property string calendarDate: appController !== null &&
                                  typeof appController.calendarDate === "string" &&
                                  appController.calendarDate.length > 0
                                  ? appController.calendarDate : fallbackCalendarDate()
    property string calendarDateLabel: appController !== null && typeof appController.calendarDateLabel === "string"
                                      ? appController.calendarDateLabel : calendarDate
    property string calendarMonthLabel: appController !== null && typeof appController.calendarMonthLabel === "string"
                                       ? appController.calendarMonthLabel : calendarDate.slice(0, 7)
    property string calendarDayHeading: appController !== null && typeof appController.calendarDayHeading === "string"
                                       ? appController.calendarDayHeading : calendarDate
    property var nativeCalendarWeekLabels: appController !== null && appController.calendarWeekLabels !== undefined
                                           ? appController.calendarWeekLabels : []
    property var sidebarTabIds: appController !== null && appController.sidebarTabIds !== undefined
                                ? appController.sidebarTabIds : ["tasks", "calendar"]
    property alias navigationSidebar: navigationSidebar
    property alias navigationShortcuts: navigationShortcuts
    property alias commandPalette: commandPalette
    property alias commandPaletteQuery: commandPaletteQuery
    property alias commandPaletteResults: commandPaletteResults
    property alias commandPaletteShortcut: commandPaletteShortcut
    property var calendarVisibility: navigationSidebar.calendarVisibility
    property alias calendarViews: calendarViews
    property alias calendarBulkControls: calendarBulkControls
    property alias eventCreateDialog: eventCreateDialog
    property alias calendarQuickCreateDialog: calendarQuickCreateDialog
    property alias eventMutationScopeDialog: eventMutationScopeDialog
    property alias eventDeleteDialog: eventDeleteDialog
    property alias eventEditDialog: eventEditDialog
    property alias eventDetailDialog: eventDetailDialog
    property alias dayTimeline: dayTimeline
    property alias weekTimeline: weekTimeline
    property alias monthGrid: monthGrid
    property alias quickCapture: quickCapture
    property alias quickCaptureShortcut: quickCaptureShortcut
    property alias quickCaptureTaskDefaultSelector: quickCaptureTaskDefaultSelector
    property alias quickCaptureCalendarDefaultSelector: quickCaptureCalendarDefaultSelector
    property alias quickCaptureDurationSelector: quickCaptureDurationSelector
    property alias quickCaptureRemoveParsedTextSwitch: quickCaptureRemoveParsedTextSwitch
    property alias googleOnboarding: googleOnboarding
    property alias invitationInbox: invitationInbox
    property alias genericPageFallback: genericPageFallback
    property alias searchPopup: searchPopup
    property alias headerSearchButton: headerSearchButton
    property alias searchQuery: searchPopup.queryField
    property alias searchResults: searchPopup.resultRows
    property alias searchShortcut: searchShortcut
    property alias noteEditor: noteEditor
    property alias notesList: notesList
    property alias taskCreateDialog: taskCreateDialog
    property alias taskDeleteDialog: taskDeleteDialog
    property alias taskEditDialog: taskEditDialog
    property alias taskDetailDialog: taskDetailDialog
    property alias taskRecurrenceActionDialog: taskRecurrenceActionDialog
    property alias taskList: taskList
    property alias taskListEditorDialog: taskListEditorDialog
    property alias taskListDeleteDialog: taskListDeleteDialog
    property alias taskMoveDialog: taskMoveDialog
    property alias calendarManagerDialog: calendarManagerDialog
    property alias calendarRemovalDialog: calendarRemovalDialog
    property alias importDialog: importDialog
    property alias importPastedText: importPastedText
    property alias importPreviewPasteButton: importPreviewPasteButton
    property alias paletteModeSelector: paletteModeSelector
    property alias accentColorField: accentColorField
    property alias fontFamilySelector: fontFamilySelector
    property alias fontScaleSelector: fontScaleSelector
    property alias displayTimeZoneSelector: displayTimeZoneField
    property alias resetVisualPreferencesButton: resetVisualPreferencesButton
    property alias settingsSearchField: settingsSearchField
    signal quickCaptureRequested(string title)
    signal taskCreateRequested(string taskListId, string parentTaskId, string title)
    signal taskDeleteRequested(string taskId)
    signal taskReparentRequested(string taskId, string parentTaskId)
    signal taskUpdateRequested(string taskId, string title, string notes, string dueAt,
                               string dueTimeZone, int priority)
    signal taskMoveRequested(string taskId, string taskListId)
    signal eventCreateRequested(string calendarId, string title, string startAt, string endAt,
                                bool allDay, string description, string location)
    signal eventUpdateRequested(string eventId, string calendarId, string title, string startAt,
                                string endAt, bool allDay, string description, string location)
    signal eventDeleteRequested(string eventId)
    signal eventMoveRequested(string eventId, string startAt, string endAt, bool allDay)
    signal eventResizeRequested(string eventId, string endAt)
    signal searchResultActivated(string resource, string resultId, string title, string detail,
                                 string scheduledAt)

    Binding {
        target: Theme
        property: "appearanceMode"
        value: window.appearanceMode
    }

    Binding {
        target: Theme
        property: "visualDensity"
        value: window.visualDensity
    }

    Binding {
        target: Theme
        property: "paletteMode"
        value: window.paletteMode
    }

    Binding {
        target: Theme
        property: "accentColor"
        value: window.accentColor
    }

    Binding {
        target: Theme
        property: "fontFamily"
        value: window.fontFamily
    }

    Binding {
        target: Theme
        property: "fontScale"
        value: window.fontScale
    }

    function controllerCall(method, args) {
        if (appController !== null && typeof appController[method] === "function") {
            appController[method].apply(appController, args)
        }
    }

    function fontFamilyOptions() {
        const available = appController !== null && appController.availableFontFamilies !== undefined
                          ? appController.availableFontFamilies : []
        return ["System default"].concat(available)
    }

    function isCalendarEventSelected(eventId) {
        return selectedCalendarEventIds.indexOf(eventId) >= 0
    }

    function setCalendarEventSelected(eventId, selected) {
        const next = selectedCalendarEventIds.slice()
        const index = next.indexOf(eventId)
        if (selected && index < 0) {
            next.push(eventId)
        } else if (!selected && index >= 0) {
            next.splice(index, 1)
        }
        selectedCalendarEventIds = next
    }

    function clearCalendarEventSelection() {
        selectedCalendarEventIds = []
    }

    function fallbackCalendarDate() {
        return new Date().toISOString().slice(0, 10)
    }

    function calendarDisplayDate() {
        return calendarDateLabel
    }

    function shiftCalendarDate(days) {
        const parsed = new Date(calendarDate + "T12:00:00Z")
        if (!Number.isFinite(parsed.getTime())) {
            return calendarDate
        }
        parsed.setUTCDate(parsed.getUTCDate() + days)
        return parsed.toISOString().slice(0, 10)
    }

    function shiftCalendarMonth(months) {
        const parsed = new Date(calendarDate + "T12:00:00Z")
        if (!Number.isFinite(parsed.getTime())) {
            return calendarDate
        }
        parsed.setUTCMonth(parsed.getUTCMonth() + months)
        return parsed.toISOString().slice(0, 10)
    }

    function calendarWeekDayIndex() {
        const parsed = new Date(calendarDate + "T12:00:00Z")
        return Number.isFinite(parsed.getTime()) ? (parsed.getUTCDay() - weekStartDay + 7) % 7 : 0
    }

    function calendarWeekLabels() {
        if (Array.isArray(nativeCalendarWeekLabels) && nativeCalendarWeekLabels.length === 7) {
            return nativeCalendarWeekLabels
        }
        const start = shiftCalendarDate(-calendarWeekDayIndex())
        const labels = []
        for (let index = 0; index < 7; ++index) {
            const date = new Date(start + "T12:00:00Z")
            date.setUTCDate(date.getUTCDate() + index)
            labels.push(["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"][date.getUTCDay()] +
                        " " + date.toISOString().slice(5, 10))
        }
        return labels
    }

    function navigateCalendar(direction) {
        let nextDate = calendarDate
        if (calendarViews.currentIndex === 3) {
            nextDate = shiftCalendarMonth(direction)
        } else if (calendarViews.currentIndex === 2 || calendarViews.currentIndex === 0) {
            nextDate = shiftCalendarDate(direction * 7)
        } else {
            nextDate = shiftCalendarDate(direction)
        }
        controllerCall("setCalendarDate", [nextDate])
    }

    function goToToday() {
        controllerCall("setCalendarDate", [fallbackCalendarDate()])
    }

    function sidebarHasTab(tabId) {
        return sidebarTabIds.indexOf(tabId) >= 0
    }

    function setSidebarTab(tabId, enabled) {
        const next = sidebarTabIds.slice()
        const index = next.indexOf(tabId)
        if (enabled && index < 0) next.push(tabId)
        if (!enabled && index >= 0) next.splice(index, 1)
        controllerCall("saveSidebarTabIds", [next])
    }

    function moveSidebarTab(tabId, direction) {
        const next = sidebarTabIds.slice()
        const index = next.indexOf(tabId)
        const target = index + direction
        if (index < 0 || target < 0 || target >= next.length) return
        const value = next[index]
        next[index] = next[target]
        next[target] = value
        controllerCall("saveSidebarTabIds", [next])
    }

    function controllerString(propertyName, fallback) {
        return appController !== null && typeof appController[propertyName] === "string"
               ? appController[propertyName] : fallback
    }

    function controllerBool(propertyName, fallback) {
        return appController !== null && typeof appController[propertyName] === "boolean"
               ? appController[propertyName] : fallback
    }

    function matchingSettingsSections(query) {
        const normalized = query.trim().toLowerCase()
        if (normalized.length === 0) return []
        const sections = [
            { title: "Google setup", keywords: "oauth client id secret connect sync google", target: googleSetupHeading },
            { title: "Google calendars", keywords: "calendar create subscribe import manage", target: googleCalendarsHeading },
            { title: "Desktop reminders", keywords: "reminder notification alert", target: remindersHeading },
            { title: "Undated task presentation", keywords: "notes undated task mirror", target: notesPresentationHeading },
            { title: "Sidebar", keywords: "tabs task calendar notes invitations", target: sidebarHeading },
            { title: "Appearance and calendar", keywords: "theme dark light colour color accent font density timezone week", target: appearanceHeading },
            { title: "Links", keywords: "browser external links safari chrome firefox", target: linksHeading },
            { title: "Undo history", keywords: "undo retention history actions", target: undoHeading },
            { title: "Bulk rewriting", keywords: "bulk find replace recurrence", target: bulkRewritingHeading },
            { title: "Quick Capture", keywords: "quick capture aliases duration task event", target: quickCaptureHeading }
        ]
        return sections.filter(function(section) {
            return (section.title + " " + section.keywords).toLowerCase().indexOf(normalized) >= 0
        })
    }

    function revealSettingsSection(section) {
        if (section === undefined || section.target === undefined || section.target === null) return
        settingsFlickable.contentY = Math.max(0, Math.min(settingsFlickable.contentHeight - settingsFlickable.height,
                                                           section.target.y - Theme.spacingMedium))
        section.target.forceActiveFocus()
    }

    function hasNavigationPage(pageName) {
        if (pageName === "Notes" && !notesEnabled) {
            return false
        }
        if (pageName !== "Settings" && sidebarTabIds.indexOf(pageName.toLowerCase()) < 0) {
            return false
        }
        const commands = navigationOnlyCommands()
        for (let row = 0; row < commands.length; ++row) {
            if (commands[row].commandLabel === pageName) {
                return true
            }
        }
        return false
    }

    function selectPage(pageName) {
        if (!hasNavigationPage(pageName) || pageName === currentPage) {
            return
        }
        if (!timelineProfile && appController !== null && !appController.googleConnected &&
                pageName === "Settings") {
            openGoogleOnboarding()
            return
        }
        if (!timelineProfile && appController !== null && !appController.googleConnected) {
            return
        }
        const spanName = "navigation." + pageName.toLowerCase()
        const tracked = transitionTimings !== null && transitionTimings.begin(spanName)
        currentPage = pageName
        if (tracked) {
            Qt.callLater(function() {
                transitionTimings.complete(spanName)
            })
        }
    }

    function openGoogleOnboarding() {
        if (timelineProfile || appController === null || appController.googleConnected !== false) {
            return
        }
        currentPage = "Onboarding"
    }

    function matchingCommands(query) {
        let commands = []
        if (typeof navigationCommands.matchingCommands === "function") {
            commands = navigationCommands.matchingCommands(query)
        } else {
            const normalizedQuery = query.trim().toLowerCase()
            for (let row = 0; row < navigationCommands.count; ++row) {
                const command = navigationCommands.get(row)
                if (normalizedQuery === "" ||
                        command.commandId.toLowerCase().indexOf(normalizedQuery) >= 0 ||
                        command.commandLabel.toLowerCase().indexOf(normalizedQuery) >= 0) {
                    commands.push(command)
                }
            }
        }
        const matches = []
        for (let row = 0; row < commands.length; ++row) {
            const command = commands[row]
            if (!command.commandId.startsWith("navigation.") || command.commandLabel !== "Notes" || notesEnabled) {
                matches.push(command)
            }
        }
        return matches
    }

    function navigationOnlyCommands() {
        return matchingCommands("").filter(function(command) {
            return command.commandId.startsWith("navigation.")
        })
    }

    function openCommandPalette() {
        if (!commandPalette.opened) {
            commandPalette.previousFocusItem = window.activeFocusItem
            commandPalette.open()
        }
        commandPaletteQuery.forceActiveFocus()
    }

    function openQuickCapture() {
        quickCapture.open()
    }

    function activateCommand(command) {
        if (command === undefined || command.commandId === undefined) {
            return
        }
        if (command.commandId === "create.quickCapture") {
            openQuickCapture()
        } else if (command.commandId === "import.items") {
            importDialog.open()
        } else if (command.commandId === "create.task") {
            taskCreateDialog.openForCreate("", "")
        } else if (command.commandId === "create.event") {
            openEventCreate(calendarDate)
        } else if (command.commandId.startsWith("navigation.")) {
            selectPage(command.commandLabel)
        }
    }

    function openSearch() {
        if (appController !== null && appController.googleConnected === false) {
            return
        }
        searchPopup.open()
    }

    function openSearchResult(resource, resultId, title, detail, scheduledAt) {
        if (resource === "task" || resource === "taskList") {
            selectPage("Tasks")
        } else if (resource === "note") {
            selectPage("Notes")
        } else if (resource === "calendar" || resource === "event") {
            selectPage("Calendar")
        }
        searchResultActivated(resource, resultId, title, detail, scheduledAt)
        if (resource === "task") {
            Qt.callLater(function() { taskList.revealTask(resultId) })
        } else if (resource === "event") {
            calendarViews.currentIndex = 0
            if (scheduledAt.length >= 10) {
                controllerCall("setCalendarDate", [scheduledAt.slice(0, 10)])
            }
            Qt.callLater(function() { agendaView.revealEvent(resultId) })
        }
        searchPopup.close()
    }

    function openNoteEditor(taskId, taskListId, title, body) {
        noteEditor.openForEdit(taskId, taskListId, title, body)
    }

    function openEventCreate(date) {
        eventCreateDialog.openForCreate(calendarVisibility.preferredCalendarId(), date || calendarDate)
    }

    function openCalendarQuickCreate(startAt, endAt, allDay) {
        controllerCall("dismissCalendarDragCreateHint", [])
        calendarQuickCreateDialog.openForRange(calendarVisibility.preferredCalendarId(), startAt, endAt,
                                               allDay)
    }

    function isRecurringEvent(event) {
        return event !== null && event !== undefined &&
               ((event.recurrenceRule || "").length > 0 || (event.recurringRemoteId || "").length > 0)
    }

    function requestScopedEventMove(event, startAt, endAt, allDay) {
        if (isRecurringEvent(event)) {
            eventMutationScopeDialog.openForMutation("move", event, startAt, endAt, allDay)
            return
        }
        eventMoveRequested(event.id, startAt, endAt, allDay)
        controllerCall("moveEvent", [event.id, startAt, endAt, allDay])
    }

    function requestScopedEventResize(event, endAt) {
        if (isRecurringEvent(event)) {
            eventMutationScopeDialog.openForMutation("resize", event, "", endAt, event.allDay === true)
            return
        }
        eventResizeRequested(event.id, endAt)
        controllerCall("resizeEvent", [event.id, endAt])
    }

    function requestScopedAllDayResize(event, startAt, endAt) {
        if (isRecurringEvent(event)) {
            eventMutationScopeDialog.openForMutation("resize", event, startAt, endAt, true)
            return
        }
        eventMoveRequested(event.id, startAt, endAt, true)
        controllerCall("moveEvent", [event.id, startAt, endAt, true])
    }

    function openEventEdit(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                           startTimeZone, colorId, transparency, visibility, attendeeEmailsJson,
                           remindersJson, remindersUseDefault, recurrenceRule, recurringRemoteId,
                           originalStartAt, eventType, conferenceJson, attachmentsJson,
                           guestPermissionsJson, statusPropertiesJson) {
        eventEditDialog.openForEdit(eventId, calendarId, title, startAt, endAt, allDay, description,
                                    location, startTimeZone, colorId, transparency, visibility,
                                    attendeeEmailsJson, remindersJson, remindersUseDefault,
                                    recurrenceRule, recurringRemoteId, originalStartAt,
                                    eventType || "default", conferenceJson || "",
                                    attachmentsJson || "", guestPermissionsJson || "",
                                    statusPropertiesJson || "")
    }

    function openEventDetail(event) {
        eventDetailDialog.openForEvent(event)
    }

    function openTaskDetail(task) {
        taskDetailDialog.openForTask(task)
    }

    color: Theme.background
    font.family: Theme.fontFamily.length > 0 ? Theme.fontFamily : "Helvetica Neue"
    font.pixelSize: Theme.bodyFontSize
    palette.window: Theme.background
    palette.windowText: Theme.textPrimary
    palette.base: Theme.surface
    palette.alternateBase: Theme.background
    palette.text: Theme.textPrimary
    palette.highlight: Theme.accent
    palette.highlightedText: Theme.onAccent
    palette.button: Theme.surface
    palette.buttonText: Theme.textPrimary
    palette.placeholderText: Theme.textSecondary
    palette.toolTipBase: Theme.surface
    palette.toolTipText: Theme.textPrimary

    Shortcut {
        id: commandPaletteShortcut
        sequence: "Ctrl+P"
        autoRepeat: false
        onActivated: window.openCommandPalette()
    }

    Shortcut {
        id: quickCaptureShortcut
        sequence: "Ctrl+Shift+N"
        autoRepeat: false
        onActivated: window.openQuickCapture()
    }

    Shortcut {
        id: searchShortcut
        sequence: "Ctrl+F"
        autoRepeat: false
        enabled: window.appController === null || window.appController.googleConnected !== false
        onActivated: window.openSearch()
    }

    Shortcut {
        sequence: StandardKey.Undo
        autoRepeat: false
        enabled: window.undoLabel.length > 0
        onActivated: window.controllerCall("undo", [])
    }

    Shortcut {
        sequence: StandardKey.Redo
        autoRepeat: false
        enabled: window.redoLabel.length > 0
        onActivated: window.controllerCall("redo", [])
    }

    Shortcut {
        sequence: "Alt+Left"
        autoRepeat: false
        onActivated: {
            if (window.currentPage === "Calendar") window.navigateCalendar(-1)
        }
    }

    Shortcut {
        sequence: "Alt+Right"
        autoRepeat: false
        onActivated: {
            if (window.currentPage === "Calendar") window.navigateCalendar(1)
        }
    }

    Instantiator {
        id: navigationShortcuts
        model: window.navigationOnlyCommands()

        delegate: Shortcut {
            required property string commandId
            required property string commandLabel
            required property string commandShortcut
            sequence: commandShortcut
            autoRepeat: false
            enabled: commandId.startsWith("navigation.") &&
                     (commandLabel !== "Notes" || window.notesEnabled)
            onActivated: {
                if (commandId.startsWith("navigation.")) window.selectPage(commandLabel)
            }
        }
    }

    Popup {
        id: commandPalette
        parent: Overlay.overlay
        anchors.centerIn: parent
        width: Math.min(560, parent.width - Theme.spacingLarge * 2)
        height: Math.min(420, parent.height - Theme.spacingLarge * 2)
        modal: true
        focus: true
        padding: Theme.spacingLarge
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutsideParent
        property var matchingCommands: window.matchingCommands(commandPaletteQuery.text)
        property var previousFocusItem: null

        function activateCurrentCommand() {
            const command = matchingCommands[commandPaletteResults.currentIndex]
            if (command === undefined) {
                return
            }
            window.activateCommand(command)
            close()
        }

        onOpened: {
            commandPaletteQuery.text = ""
            commandPaletteResults.currentIndex = 0
            commandPaletteQuery.forceActiveFocus()
        }

        onClosed: {
            if (previousFocusItem !== null) {
                previousFocusItem.forceActiveFocus()
            }
            previousFocusItem = null
        }

        background: Rectangle {
            color: Theme.surface
            border.color: Theme.accent
            border.width: 1
            radius: Theme.spacingSmall
        }

        ColumnLayout {
            anchors.fill: parent
            spacing: Theme.spacingMedium
            Accessible.role: Accessible.Dialog
            Accessible.name: "Command palette"

            TextField {
                id: commandPaletteQuery
                objectName: "commandPaletteQuery"
                Layout.fillWidth: true
                focus: true
                placeholderText: "Search commands"
                Accessible.name: "Search commands"
                onTextChanged: commandPaletteResults.currentIndex = commandPalette.matchingCommands.length > 0 ? 0 : -1
                Keys.onPressed: function(event) {
                    if (event.key === Qt.Key_Down && commandPaletteResults.count > 0) {
                        commandPaletteResults.currentIndex = Math.min(commandPaletteResults.count - 1,
                                                                     commandPaletteResults.currentIndex + 1)
                        event.accepted = true
                    } else if (event.key === Qt.Key_Up && commandPaletteResults.count > 0) {
                        commandPaletteResults.currentIndex = Math.max(0,
                                                                     commandPaletteResults.currentIndex - 1)
                        event.accepted = true
                    } else if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter) {
                        commandPalette.activateCurrentCommand()
                        event.accepted = true
                    }
                }
            }

            ListView {
                id: commandPaletteResults
                Layout.fillWidth: true
                Layout.fillHeight: true
                clip: true
                model: commandPalette.matchingCommands
                currentIndex: 0
                visible: count > 0

                delegate: AccessibleButton {
                    required property var modelData
                    width: ListView.view.width
                    text: modelData.commandShortcut.length > 0
                          ? modelData.commandLabel + "    " + modelData.commandShortcut : modelData.commandLabel
                    accessibleName: modelData.commandLabel
                    accessibleDescription: modelData.commandId.startsWith("navigation.")
                                           ? "Navigate to " + modelData.commandLabel
                                           : "Run " + modelData.commandLabel
                    highlighted: ListView.isCurrentItem
                    onClicked: {
                        window.activateCommand(modelData)
                        commandPalette.close()
                    }
                }
            }

            Label {
                Layout.fillWidth: true
                visible: commandPaletteResults.count === 0
                text: "No matching commands"
                color: Theme.textSecondary
                horizontalAlignment: Text.AlignHCenter
            }
        }
    }

    QuickCaptureDialog {
        id: quickCapture
        parent: Overlay.overlay
        anchors.centerIn: parent
        appController: window.appController
        taskListModel: window.taskListModel
        calendarSourceModel: window.calendarSourceModel
        defaultTaskListId: window.quickCaptureDefaultTaskListId
        defaultCalendarId: window.quickCaptureDefaultCalendarId
        onCaptureRequested: function(title, kind, destinationId, disabledRecognitionIds) {
            window.quickCaptureRequested(title)
            window.controllerCall("createQuickCapture", [title, kind, destinationId, disabledRecognitionIds])
        }
    }

    SearchPopup {
        id: searchPopup
        appController: window.appController
        searchResultsModel: window.searchResultsModel
        onResultActivated: function(resource, resultId, title, detail, scheduledAt) {
            window.openSearchResult(resource, resultId, title, detail, scheduledAt)
        }
    }

    NoteEditorDialog {
        id: noteEditor
        parent: Overlay.overlay
        anchors.centerIn: parent
        taskListModel: window.taskListModel
        onTaskSaveRequested: function(taskId, taskListId, title, body) {
            window.controllerCall("saveNoteTask", [taskId, taskListId, title, body])
        }
    }

    TaskDeleteDialog {
        id: taskDeleteDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onTaskDeleteRequested: function(taskId) {
            window.taskDeleteRequested(taskId)
            window.controllerCall("deleteTask", [taskId])
        }
    }

    TaskDetailDialog {
        id: taskDetailDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onEditRequested: function(task) {
            taskEditDialog.openForEdit(task.id, task.title, task.notes || "", task.dueAt || "",
                                       task.dueTimeZone || "", task.priority || 0,
                                       task.managedRecurrence === true, task.recurrenceSummary || "",
                                       task.recurrenceFrequency || -1, task.recurrenceInterval || 1,
                                       task.recurrenceEndKind || 0, task.recurrenceEndUntil || "",
                                       task.recurrenceEndCount || 0, task.recurrenceRule || "",
                                       task.recurrenceExclusionDates || "", task.recurrenceAdditionDates || "")
        }
        onDeleteRequested: function(taskId, title, managedRecurrence) {
            taskDeleteDialog.openForDelete(taskId, title, managedRecurrence)
        }
        onExternalLinkRequested: function(url) { window.controllerCall("openExternalLink", [url]) }
    }

    TaskEditDialog {
        id: taskEditDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        timeZones: window.appController && Array.isArray(window.appController.availableTimeZones)
                   ? window.appController.availableTimeZones : ["", "UTC"]
        onTaskUpdateRequested: function(taskId, title, notes, dueAt, dueTimeZone, priority,
                                        managedRecurrence, recurrenceFrequency, recurrenceInterval,
                                        recurrenceEndKind, recurrenceEndUntil, recurrenceEndCount,
                                        recurrenceRule, exclusionDates, additionDates) {
            window.taskUpdateRequested(taskId, title, notes, dueAt, dueTimeZone, priority)
            window.controllerCall("updateTaskDetailed", [taskId, title, notes, dueAt, dueTimeZone,
                                                            priority, managedRecurrence,
                                                            recurrenceFrequency, recurrenceInterval,
                                                            recurrenceEndKind, recurrenceEndUntil,
                                                            recurrenceEndCount, recurrenceRule,
                                                            exclusionDates, additionDates])
        }
        onTaskRecurrenceActionRequested: function(taskId, title, action) {
            taskRecurrenceActionDialog.openForAction(taskId, title, action)
        }
    }

    TaskCreateDialog {
        id: taskCreateDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        taskListModel: window.taskListModel
        timeZones: window.appController && Array.isArray(window.appController.availableTimeZones)
                   ? window.appController.availableTimeZones : ["", "UTC"]
        onTaskCreateRequested: function(taskListId, parentTaskId, title, notes, dueAt, dueTimeZone,
                                        priority, managedRecurrence, recurrenceFrequency,
                                        recurrenceInterval, recurrenceEndKind, recurrenceEndUntil,
                                        recurrenceEndCount, recurrenceRule, exclusionDates, additionDates) {
            window.taskCreateRequested(taskListId, parentTaskId, title)
            window.controllerCall("createTaskDetailed", [taskListId, parentTaskId, title, notes, dueAt,
                                                            dueTimeZone, priority, managedRecurrence,
                                                            recurrenceFrequency, recurrenceInterval,
                                                            recurrenceEndKind, recurrenceEndUntil,
                                                            recurrenceEndCount, recurrenceRule,
                                                            exclusionDates, additionDates])
        }
    }

    TaskRecurrenceActionDialog {
        id: taskRecurrenceActionDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onRecurrenceActionRequested: function(taskId, action, scope) {
            if (action === 0) {
                window.controllerCall("stopTaskRecurrence", [taskId, scope])
            } else {
                window.controllerCall("splitTaskRecurrence", [taskId])
            }
        }
    }

    TaskMoveDialog {
        id: taskMoveDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        taskListModel: window.taskListModel
        onTaskMoveRequested: function(taskId, taskListId) {
            window.taskMoveRequested(taskId, taskListId)
            window.controllerCall("moveTask", [taskId, taskListId])
        }
    }

    TaskListEditorDialog {
        id: taskListEditorDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onTaskListSaveRequested: function(taskListId, title) {
            if (taskListId.length === 0) {
                window.controllerCall("createTaskList", [title])
            } else {
                window.controllerCall("renameTaskList", [taskListId, title])
            }
        }
    }

    TaskListDeleteDialog {
        id: taskListDeleteDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onTaskListDeleteRequested: function(taskListId) {
            window.controllerCall("deleteTaskList", [taskListId])
        }
    }

    CalendarQuickCreateDialog {
        id: calendarQuickCreateDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        taskListModel: window.taskListModel
        onEventCreateRequested: function(title, startAt, endAt, allDay) {
            const calendarId = calendarVisibility.preferredCalendarId()
            window.eventCreateRequested(calendarId, title, startAt, endAt, allDay, "", "")
            window.controllerCall("createEvent", [calendarId, title, startAt, endAt, allDay, "", ""])
        }
        onTaskCreateRequested: function(taskListId, title, dueDate) {
            window.taskCreateRequested(taskListId, "", title)
            window.controllerCall("createTaskDetailed", [taskListId, "", title, "", dueDate, "", 0,
                                                            false, -1, 1, 0, "", 0, "", "", ""])
        }
    }

    Popup {
        id: calendarDragCreateHint
        parent: Overlay.overlay
        x: Math.max(Theme.spacingMedium, (parent.width - width) / 2)
        y: Math.max(Theme.spacingMedium, parent.height - height - Theme.spacingLarge)
        padding: Theme.spacingMedium
        modal: false
        closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside
        property bool dismissedLocally: false
        property bool shouldShow: window.currentPage === "Calendar" && !dismissedLocally &&
                                  !window.controllerBool("calendarDragCreateHintSeen", false)
        onShouldShowChanged: {
            if (shouldShow && !opened) open()
            else if (!shouldShow && opened) close()
        }
        Component.onCompleted: if (shouldShow) open()
        onClosed: {
            if (shouldShow) {
                dismissedLocally = true
                window.controllerCall("dismissCalendarDragCreateHint", [])
            }
        }

        contentItem: RowLayout {
            spacing: Theme.spacingMedium
            Label { text: "Drag empty space to create" }
            AccessibleButton {
                text: "Got it"
                onClicked: calendarDragCreateHint.close()
            }
        }
    }

    EventMutationScopeDialog {
        id: eventMutationScopeDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onMutationConfirmed: function(operation, event, startAt, endAt, allDay, recurrenceScope) {
            if (operation === "move") {
                window.eventMoveRequested(event.id, startAt, endAt, allDay)
                window.controllerCall("moveEventScoped", [event.id, startAt, endAt, allDay,
                                                            recurrenceScope])
            } else if (allDay) {
                window.eventMoveRequested(event.id, startAt, endAt, true)
                window.controllerCall("moveEventScoped", [event.id, startAt, endAt, true,
                                                            recurrenceScope])
            } else {
                window.eventResizeRequested(event.id, endAt)
                window.controllerCall("resizeEventScoped", [event.id, endAt, recurrenceScope])
            }
        }
    }

    EventCreateDialog {
        id: eventCreateDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        calendarSourceModel: window.calendarSourceModel
        timeZoneConverter: window.appController
        timeZones: window.appController && Array.isArray(window.appController.availableTimeZones)
                   ? window.appController.availableTimeZones : ["", "UTC"]
        driveAttachmentCandidates: window.appController ? window.appController.driveAttachmentCandidates : []
        freeBusyIntervals: window.appController ? window.appController.freeBusyIntervals : []
        onEventCreateRequested: function(calendarId, title, startAt, endAt, allDay, description, location,
                                         timeZone, colorId, available, visibility, attendees,
                                         remindersUseDefault, reminders, recurrenceRule) {
            window.eventCreateRequested(calendarId, title, startAt, endAt, allDay, description, location)
        }
        onRichEventCreateRequested: function(calendarId, title, startAt, endAt, allDay, description, location,
                                         timeZone, colorId, available, visibility, attendees,
                                         remindersUseDefault, reminders, recurrenceRule, createGoogleMeet,
                                         attachmentsJson, guestPermissionsJson, eventType,
                                         statusPropertiesJson, sendUpdates) {
            window.controllerCall("createEventDetailed", [calendarId, title, startAt, endAt, allDay,
                                                            description, location, timeZone, colorId,
                                                            available, visibility, attendees,
                                                            remindersUseDefault, reminders, recurrenceRule,
                                                            createGoogleMeet, attachmentsJson,
                                                            guestPermissionsJson, eventType,
                                                            statusPropertiesJson, sendUpdates])
        }
        onDriveSearchRequested: function(query) { window.controllerCall("searchGoogleDriveAttachments", [query]) }
        onAvailabilityRequested: function(calendarIds, startAt, endAt) {
            window.controllerCall("queryGoogleFreeBusy", [calendarIds, startAt, endAt])
        }
    }

    EventEditDialog {
        id: eventEditDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        calendarSourceModel: window.calendarSourceModel
        timeZoneConverter: window.appController
        timeZones: window.appController && Array.isArray(window.appController.availableTimeZones)
                   ? window.appController.availableTimeZones : ["", "UTC"]
        driveAttachmentCandidates: window.appController ? window.appController.driveAttachmentCandidates : []
        freeBusyIntervals: window.appController ? window.appController.freeBusyIntervals : []
        onEventUpdateRequested: function(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                                         timeZone, colorId, available, visibility, attendees,
                                         remindersUseDefault, reminders, recurrenceRule, recurrenceScope) {
            window.eventUpdateRequested(eventId, calendarId, title, startAt, endAt, allDay, description, location)
        }
        onRichEventUpdateRequested: function(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                                         timeZone, colorId, available, visibility, attendees,
                                         remindersUseDefault, reminders, recurrenceRule, recurrenceScope,
                                         createGoogleMeet, attachmentsJson, guestPermissionsJson,
                                         statusPropertiesJson, sendUpdates) {
            window.controllerCall("updateEventDetailed", [eventId, calendarId, title, startAt, endAt,
                                                            allDay, description, location, timeZone,
                                                            colorId, available, visibility, attendees,
                                                            remindersUseDefault, reminders, recurrenceRule,
                                                            recurrenceScope, createGoogleMeet,
                                                            attachmentsJson, guestPermissionsJson,
                                                            statusPropertiesJson, sendUpdates])
        }
        onDriveSearchRequested: function(query) { window.controllerCall("searchGoogleDriveAttachments", [query]) }
        onAvailabilityRequested: function(calendarIds, startAt, endAt) {
            window.controllerCall("queryGoogleFreeBusy", [calendarIds, startAt, endAt])
        }
        onRsvpRequested: function(eventId, responseStatus, responseComment) {
            window.controllerCall("respondToEvent", [eventId, responseStatus, responseComment])
        }
        onExternalLinkRequested: function(url) { window.controllerCall("openExternalLink", [url]) }
        onEventDeleteRequested: function(eventId, title, recurrenceRule, recurringRemoteId, originalStartAt) {
            eventDeleteDialog.openForDelete(eventId, title, recurrenceRule, recurringRemoteId, originalStartAt)
        }
    }

    EventDeleteDialog {
        id: eventDeleteDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        onEventDeleteRequested: function(eventId, recurrenceScope) {
            window.eventDeleteRequested(eventId)
            window.controllerCall("deleteEvent", [eventId, recurrenceScope])
        }
    }

    EventDetailDialog {
        id: eventDetailDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        calendarSourceModel: window.calendarSourceModel
        onEditRequested: function(event) {
            window.openEventEdit(event.id, event.calendarId, event.title, event.startAt, event.endAt,
                                 event.allDay, event.description || "", event.location || "",
                                 event.startTimeZone || "", event.colorId || "", event.transparency || "",
                                 event.visibility || "", event.attendeeEmailsJson || "[]",
                                 event.remindersJson || "[]", event.remindersUseDefault === true,
                                 event.recurrenceRule || "", event.recurringRemoteId || "",
                                 event.originalStartAt || "", event.eventType || "default",
                                 event.conferenceJson || "", event.attachmentsJson || "[]",
                                 event.guestPermissionsJson || "{}", event.statusPropertiesJson || "{}")
        }
        onDeleteRequested: function(eventId, title, recurrenceRule, recurringRemoteId, originalStartAt) {
            eventDeleteDialog.openForDelete(eventId, title, recurrenceRule, recurringRemoteId, originalStartAt)
        }
        onExternalLinkRequested: function(url) { window.controllerCall("openExternalLink", [url]) }
    }

    HcbDialog {
        id: calendarManagerDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        title: "Calendar settings"
        primaryText: "Save"
        property string calendarId: ""
        property bool ownedSecondary: false
        property bool primaryCalendar: false
        property string initialTitle: ""
        property string initialDescription: ""
        property string initialTimeZone: ""
        property string initialColorId: ""
        property bool initialSelected: true
        property bool initialHidden: false
        primaryEnabled: calendarManagerTitle.text.trim().length > 0 &&
                        window.appController !== null && !window.appController.busy

        function openForCalendar(calendar) {
            calendarId = calendar.id
            ownedSecondary = calendar.accessRole === "owner" && !calendar.primary
            primaryCalendar = calendar.primary === true
            initialTitle = calendar.title || ""
            initialDescription = calendar.description || ""
            initialTimeZone = calendar.timeZone || ""
            initialColorId = calendar.colorId || ""
            initialSelected = calendar.selected !== false
            initialHidden = calendar.hidden === true
            open()
        }

        Label {
            Layout.fillWidth: true
            text: calendarManagerDialog.ownedSecondary
                  ? "This owned secondary calendar can be edited."
                  : "Only Calendar-list preferences can be changed for this calendar."
            wrapMode: Text.WordWrap
            color: Theme.textSecondary
        }

        TextField {
            id: calendarManagerTitle
            Layout.fillWidth: true
            text: calendarManagerDialog.initialTitle
            enabled: calendarManagerDialog.ownedSecondary
            placeholderText: "Calendar name"
            Accessible.name: placeholderText
        }

        TextArea {
            id: calendarManagerDescription
            Layout.fillWidth: true
            text: calendarManagerDialog.initialDescription
            enabled: calendarManagerDialog.ownedSecondary
            placeholderText: "Calendar description"
            Accessible.name: placeholderText
            wrapMode: TextArea.Wrap
        }

        TextField {
            id: calendarManagerTimeZone
            Layout.fillWidth: true
            text: calendarManagerDialog.initialTimeZone
            enabled: calendarManagerDialog.ownedSecondary
            placeholderText: "Calendar time zone (IANA)"
            Accessible.name: placeholderText
        }

        TextField {
            id: calendarManagerColorId
            Layout.fillWidth: true
            text: calendarManagerDialog.initialColorId
            placeholderText: "Google Calendar color ID (blank keeps current)"
            Accessible.name: placeholderText
        }

        Switch {
            id: calendarManagerSelected
            text: "Show in Google Calendar"
            checked: calendarManagerDialog.initialSelected
            Accessible.name: text
        }

        Switch {
            id: calendarManagerHidden
            text: "Hide in Google Calendar"
            checked: calendarManagerDialog.initialHidden
            Accessible.name: text
        }

        onPrimaryAction: {
            window.controllerCall("saveGoogleCalendarSettings",
                                  [calendarId, calendarManagerTitle.text,
                                   calendarManagerDescription.text, calendarManagerTimeZone.text,
                                   calendarManagerSelected.checked, calendarManagerHidden.checked,
                                   calendarManagerColorId.text])
        }
    }

    HcbDialog {
        id: calendarRemovalDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        property string calendarId: ""
        property bool deleteCalendar: false
        property string calendarTitle: ""
        title: deleteCalendar ? "Delete Google Calendar" : "Unsubscribe from Google Calendar"
        primaryText: deleteCalendar ? "Delete calendar" : "Unsubscribe"
        primaryDestructive: true
        primaryEnabled: window.appController !== null && !window.appController.busy

        Label {
            Layout.fillWidth: true
            text: calendarRemovalDialog.deleteCalendar
                  ? "Delete “" + calendarRemovalDialog.calendarTitle + "” permanently for every Google Calendar user with access? This cannot be undone."
                  : "Remove “" + calendarRemovalDialog.calendarTitle + "” from this Google account’s Calendar list? The calendar itself is unchanged."
            wrapMode: Text.WordWrap
            color: Theme.textSecondary
        }

        onPrimaryAction: {
            window.controllerCall(deleteCalendar ? "deleteGoogleCalendar" : "unsubscribeGoogleCalendar",
                                  [calendarId])
        }
    }

    HcbDialog {
        id: importDialog
        parent: Overlay.overlay
        anchors.centerIn: parent
        title: "Import Tasks and events"
        primaryText: window.appController !== null && window.appController.importReadyToCommit
                     ? "Confirm atomic import" : "Validate rows"
        closeOnPrimaryAction: window.appController !== null &&
                              window.appController.importReadyToCommit
        primaryEnabled: window.appController !== null && !window.appController.busy &&
                        window.appController.importPreviewRows !== undefined &&
                        window.appController.importPreviewRows.some(function(row) { return row.accepted })

        Label {
            Layout.fillWidth: true
            text: window.appController !== null && window.appController.importSourceName !== undefined &&
                  window.appController.importSourceName.length > 0
                  ? "Previewing “" + window.appController.importSourceName + "”. Destination names in the file must match exactly and unambiguously; blank destinations use the defaults below."
                  : "Paste delimited lines, or choose a UTF-8 delimited/version-1 CSV file."
            wrapMode: Text.WordWrap
            color: Theme.textSecondary
        }

        TextArea {
            id: importPastedText
            Layout.fillWidth: true
            Layout.preferredHeight: 120
            placeholderText: "task title=\"Buy milk\" list=\"Inbox\" due=\"2026-07-29\"\nevent title=\"Planning\" calendar=\"Work\" start=\"2026-07-29T09:00:00+08:00\" end=\"2026-07-29T10:00:00+08:00\" time_zone=\"Asia/Singapore\""
            Accessible.name: "Delimited Tasks and events to import"
            selectByMouse: true
            wrapMode: TextEdit.Wrap
        }

        Button {
            id: importPreviewPasteButton
            text: "Preview pasted text"
            enabled: window.appController !== null && !window.appController.busy &&
                     importPastedText.text.trim().length > 0
            Accessible.name: text
            onClicked: window.controllerCall("previewDelimitedImport", [importPastedText.text])
        }

        Button {
            text: "Choose import file"
            enabled: window.appController !== null && !window.appController.busy
            Accessible.name: text
            onClicked: window.controllerCall("chooseImportFile", [])
        }

        ComboBox {
            id: importDefaultTaskList
            Layout.fillWidth: true
            property int taskListRevision: window.taskListModel !== null && window.taskListModel.revision !== undefined
                                           ? window.taskListModel.revision : 0
            model: {
                const revision = taskListRevision
                return window.taskListModel !== null && typeof window.taskListModel.selectedTaskLists === "function"
                       ? window.taskListModel.selectedTaskLists() : []
            }
            textRole: "title"
            valueRole: "id"
            displayText: currentIndex >= 0 ? "Default Task list: " + currentText : "Default Task list required"
            Accessible.name: "Default Task list for import rows without list"
            onActivated: window.controllerCall("invalidateImportValidation", [])
        }

        ComboBox {
            id: importDefaultCalendar
            Layout.fillWidth: true
            property int calendarRevision: window.calendarSourceModel !== null && window.calendarSourceModel.revision !== undefined
                                           ? window.calendarSourceModel.revision : 0
            model: window.calendarSourceModel
            textRole: "title"
            valueRole: "id"
            displayText: currentIndex >= 0 ? "Default calendar: " + currentText : "Default calendar required"
            Accessible.name: "Default calendar for import rows without calendar"
            onActivated: window.controllerCall("invalidateImportValidation", [])
        }

        Repeater {
            model: window.appController !== null && window.appController.importPreviewRows !== undefined
                   ? window.appController.importPreviewRows : []
            delegate: Label {
                required property var modelData
                Layout.fillWidth: true
                text: (modelData.line > 0 ? "Line " + modelData.line + ": " : "File: ") +
                      modelData.kind + (modelData.title.length > 0 ? " — " + modelData.title : "") +
                      " · " + modelData.message
                color: modelData.accepted ? Theme.textSecondary : Theme.destructive
                wrapMode: Text.WordWrap
                Accessible.name: text
            }
        }

        onPrimaryAction: window.controllerCall("runImport", [importDefaultTaskList.currentValue || "",
                                                                importDefaultCalendar.currentValue || ""])
        onSecondaryAction: window.controllerCall("cancelImport", [])
    }

    Connections {
        target: window.appController
        function onImportPreviewRowsChanged() {
            if (window.appController !== null && window.appController.importPreviewRows.length > 0) {
                importDialog.open()
            }
        }
        function onGoogleConnectedChanged() {
            if (window.appController !== null && window.appController.googleConnected &&
                    window.currentPage === "Onboarding") {
                window.selectPage("Tasks")
            }
        }
    }

    header: ToolBar {
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: Theme.spacingLarge
            anchors.rightMargin: Theme.spacingLarge

            ToolButton {
                id: headerSearchButton
                visible: window.appController === null || window.appController.googleConnected !== false
                Accessible.name: "Search"
                Accessible.description: "Open local search"
                ToolTip.visible: hovered
                ToolTip.text: "Search"
                display: AbstractButton.IconOnly
                icon.name: "system-search"
                icon.width: 18
                icon.height: 18
                onClicked: window.openSearch()
            }
            Item { Layout.fillWidth: true }
        }
    }

    SplitView {
        anchors.fill: parent

        NavigationSidebar {
            id: navigationSidebar
            visible: window.currentPage !== "Onboarding"
            SplitView.preferredWidth: visible ? Theme.navigationWidth : 0
            commandRegistry: window.navigationOnlyCommands()
            currentPage: window.currentPage
            googleConnected: window.appController === null || window.appController.googleConnected !== false
            notesEnabled: window.notesEnabled
            sidebarTabIds: window.sidebarTabIds
            calendarSourceModel: window.calendarSourceModel
            persistedVisibleCalendarIds: window.appController !== null &&
                                         window.appController.visibleCalendarIds !== undefined
                                         ? window.appController.visibleCalendarIds : []
            calendarVisibilityConfigured: window.appController !== null &&
                                           window.appController.calendarVisibilityConfigured === true
            pendingInvitationCount: window.appController && typeof window.appController.pendingInvitationCount === "number"
                                    ? window.appController.pendingInvitationCount : 0
            onPageSelected: pageName => window.selectPage(pageName)
            onCalendarVisibilitySaveRequested: function(visibleCalendarIds) {
                window.clearCalendarEventSelection()
                window.controllerCall("saveCalendarVisibility", [visibleCalendarIds])
            }
        }

        Pane {
            SplitView.fillWidth: true
            TaskListView {
                id: taskList
                anchors.fill: parent
                visible: window.currentPage === "Tasks"
                taskListModel: window.taskListModel
                taskModel: window.taskModel
                listPaneWidth: window.taskListPaneWidth
                taskListLoading: window.appController !== null && window.appController.busy === true
                taskListErrorMessage: window.appController !== null &&
                                      typeof window.appController.taskListErrorMessage === "string"
                                      ? window.appController.taskListErrorMessage : ""
                bulkTaskStatusMessage: window.appController !== null &&
                                       typeof window.appController.bulkTaskStatusMessage === "string"
                                       ? window.appController.bulkTaskStatusMessage : ""
                bulkTaskPreviewMessage: window.controllerString("bulkTaskPreviewMessage", "")
                bulkTaskPreviewRequestToken: window.bulkTaskPreviewRequestToken
                bulkTextRecurrenceScope: window.bulkTextRecurrenceScope
                onTaskListCreateRequested: taskListEditorDialog.openForCreate()
                onTaskListRenameRequested: function(taskListId, title) {
                    taskListEditorDialog.openForRename(taskListId, title)
                }
                onTaskListDeleteRequested: function(taskListId, title, taskCount, taskTitles) {
                    taskListDeleteDialog.openForDelete(taskListId, title, taskCount, taskTitles)
                }
                onTaskListSelectionRequested: function(taskListId, selected) {
                    window.controllerCall("setTaskListSelected", [taskListId, selected])
                }
                onListPaneWidthPersistenceRequested: function(width) {
                    window.controllerCall("saveTaskListPaneWidth", [width])
                }
                onTaskCreateRequested: taskCreateDialog.openForCreate(taskList.preferredTaskListId, "")
                onImportRequested: importDialog.open()
                onTaskSubtaskCreateRequested: function(parentTaskId, taskListId) {
                    taskCreateDialog.openForCreate(taskListId, parentTaskId)
                }
                onTaskReparentRequested: function(taskId, parentTaskId) {
                    window.taskReparentRequested(taskId, parentTaskId)
                    window.controllerCall("reparentTask", [taskId, parentTaskId])
                }
                onTaskReorderRequested: function(taskId, earlier) {
                    window.controllerCall("reorderTask", [taskId, earlier])
                }
                onTaskCompletionRequested: function(taskId, completed) {
                    window.controllerCall("setTaskCompleted", [taskId, completed])
                }
                onTaskEditRequested: function(taskId, title, notes, dueAt, dueTimeZone, priority,
                                               managedRecurrence, recurrenceSummary,
                                               recurrenceFrequency, recurrenceInterval,
                                               recurrenceEndKind, recurrenceEndUntil,
                                               recurrenceEndCount, recurrenceRule,
                                               recurrenceExclusionDates, recurrenceAdditionDates) {
                    taskEditDialog.openForEdit(taskId, title, notes, dueAt, dueTimeZone, priority,
                                               managedRecurrence, recurrenceSummary,
                                               recurrenceFrequency, recurrenceInterval,
                                               recurrenceEndKind, recurrenceEndUntil,
                                               recurrenceEndCount, recurrenceRule,
                                               recurrenceExclusionDates, recurrenceAdditionDates)
                }
                onTaskDetailRequested: function(task) { window.openTaskDetail(task) }
                onTaskDeleteRequested: function(taskId, taskTitle, managedRecurrence) {
                    taskDeleteDialog.openForDelete(taskId, taskTitle, managedRecurrence)
                }
                onTaskMoveRequested: function(taskId, taskListId, taskTitle) {
                    taskMoveDialog.openForMove(taskId, taskTitle, taskListId)
                }
                onBulkTaskCompletionRequested: function(taskIds, completed) {
                    window.controllerCall("bulkSetTaskCompleted", [taskIds, completed])
                }
                onBulkTaskDeleteRequested: function(taskIds) {
                    window.controllerCall("bulkDeleteTasks", [taskIds])
                }
                onBulkTaskMoveRequested: function(taskIds, taskListId) {
                    window.controllerCall("bulkMoveTasks", [taskIds, taskListId])
                }
                onBulkTaskDueRequested: function(taskIds, dueAt) {
                    window.controllerCall("bulkSetTaskDue", [taskIds, dueAt])
                }
                onBulkTaskClearDueRequested: function(taskIds) {
                    window.controllerCall("bulkClearTaskDue", [taskIds])
                }
                onBulkTaskPriorityRequested: function(taskIds, priority) {
                    window.controllerCall("bulkSetTaskPriority", [taskIds, priority])
                }
                onBulkTaskReparentRequested: function(taskIds, parentTaskId) {
                    window.controllerCall("bulkReparentTasks", [taskIds, parentTaskId])
                }
                onBulkTaskTextPreviewRequested: function(taskIds, findText, fields, recurrenceScope, requestToken) {
                    window.controllerCall("previewBulkTaskText",
                                          [taskIds, findText, fields, recurrenceScope, requestToken])
                }
                onBulkTaskTextReplaceRequested: function(taskIds, findText, replaceText, fields, recurrenceScope) {
                    window.controllerCall("bulkReplaceTaskText",
                                          [taskIds, findText, replaceText, fields, recurrenceScope])
                }
            }

            NotesListView {
                id: notesList
                anchors.fill: parent
                visible: window.currentPage === "Notes"
                notesModel: window.notesModel
                loading: window.appController !== null && window.appController.busy === true
                statusMessage: window.appController !== null &&
                               typeof window.appController.statusMessage === "string"
                               ? window.appController.statusMessage : ""
                onNoteCreateRequested: noteEditor.openForCreate("")
                onNoteEditRequested: function(taskId, taskListId, title, body) {
                    window.openNoteEditor(taskId, taskListId, title, body)
                }
                onNoteCompletionRequested: function(taskId, completed) {
                    window.controllerCall("setTaskCompleted", [taskId, completed])
                }
                onNoteDeleteRequested: function(taskId, title) {
                    taskDeleteDialog.openForDelete(taskId, title)
                }
                onNoteMoveRequested: function(taskId, taskListId, title) {
                    taskMoveDialog.openForMove(taskId, title, taskListId)
                }
            }

            InvitationInbox {
                id: invitationInbox
                anchors.fill: parent
                visible: window.currentPage === "Invitations"
                invitations: window.appController && Array.isArray(window.appController.invitations)
                             ? window.appController.invitations : []
                onResponseRequested: function(eventId, responseStatus, comment) {
                    window.controllerCall("respondToEvent", [eventId, responseStatus, comment])
                }
            }

            GoogleOnboardingView {
                id: googleOnboarding
                anchors.fill: parent
                visible: window.currentPage === "Onboarding"
                clientId: window.controllerString("clientId", "")
                hasClientSecret: window.controllerBool("hasClientSecret", false)
                busy: window.appController !== null && window.appController.busy === true
                statusMessage: window.controllerString("statusMessage", "")
                onSaveClientIdRequested: (clientId, clientSecret) => window.controllerCall("saveClientId", [clientId, clientSecret])
                onConnectGoogleRequested: window.controllerCall("connectGoogle", [])
            }

            ColumnLayout {
                anchors.fill: parent
                visible: window.currentPage === "Calendar"
                spacing: 0

                RowLayout {
                    Layout.fillWidth: true
                    Layout.leftMargin: Theme.spacingMedium
                    Layout.rightMargin: Theme.spacingMedium

                    Label {
                        text: "Calendar"
                        font.pixelSize: Theme.titleFontSize
                        Accessible.role: Accessible.Heading
                        Accessible.name: text
                    }

                    Item { Layout.fillWidth: true }

                    Button {
                        text: "Previous"
                        Accessible.name: "Previous calendar period"
                        onClicked: window.navigateCalendar(-1)
                    }

                    Button {
                        text: "Today"
                        Accessible.name: "Go to today"
                        onClicked: window.goToToday()
                    }

                    Label {
                        text: window.calendarDisplayDate()
                        Accessible.name: "Selected calendar date " + text
                    }

                    Button {
                        text: "Next"
                        Accessible.name: "Next calendar period"
                        onClicked: window.navigateCalendar(1)
                    }

                    Button {
                        id: eventCreateButton
                        text: "New event"
                        enabled: calendarVisibility.calendarIds().length > 0
                        Accessible.name: text + " on " + window.calendarDate
                        onClicked: window.openEventCreate(window.calendarDate)
                    }

                    Button {
                        text: "Import"
                        Accessible.name: "Import Tasks and events"
                        enabled: window.appController === null || !window.appController.busy
                        onClicked: importDialog.open()
                    }

                    Button {
                        text: window.calendarSelectionMode ? "Done" : "Select"
                        Accessible.name: window.calendarSelectionMode
                                         ? "Exit event selection" : "Select events"
                        onClicked: {
                            if (window.calendarSelectionMode) {
                                window.clearCalendarEventSelection()
                                window.calendarSelectionMode = false
                            } else {
                                window.calendarSelectionMode = true
                            }
                        }
                    }
                }

                TabBar {
                    id: calendarViews
                    Layout.fillWidth: true
                    spacing: Theme.spacingSmall

                    background: Rectangle {
                        color: "transparent"
                    }

                    TabButton {
                        text: "Agenda"
                        padding: Theme.spacingMedium
                        implicitHeight: 42
                        palette.buttonText: checked ? Theme.onAccent : Theme.textPrimary
                        background: Rectangle {
                            radius: Theme.spacingSmall
                            color: parent.checked ? Theme.accent : Theme.surface
                            border.color: parent.checked ? Theme.accent : Theme.textSecondary
                            border.width: parent.checked ? 0 : 1
                        }
                    }
                    TabButton {
                        text: "Day"
                        padding: Theme.spacingMedium
                        implicitHeight: 42
                        palette.buttonText: checked ? Theme.onAccent : Theme.textPrimary
                        background: Rectangle {
                            radius: Theme.spacingSmall
                            color: parent.checked ? Theme.accent : Theme.surface
                            border.color: parent.checked ? Theme.accent : Theme.textSecondary
                            border.width: parent.checked ? 0 : 1
                        }
                    }
                    TabButton {
                        text: "Week"
                        padding: Theme.spacingMedium
                        implicitHeight: 42
                        palette.buttonText: checked ? Theme.onAccent : Theme.textPrimary
                        background: Rectangle {
                            radius: Theme.spacingSmall
                            color: parent.checked ? Theme.accent : Theme.surface
                            border.color: parent.checked ? Theme.accent : Theme.textSecondary
                            border.width: parent.checked ? 0 : 1
                        }
                    }
                    TabButton {
                        text: "Month"
                        padding: Theme.spacingMedium
                        implicitHeight: 42
                        palette.buttonText: checked ? Theme.onAccent : Theme.textPrimary
                        background: Rectangle {
                            radius: Theme.spacingSmall
                            color: parent.checked ? Theme.accent : Theme.surface
                            border.color: parent.checked ? Theme.accent : Theme.textSecondary
                            border.width: parent.checked ? 0 : 1
                        }
                    }
                }

                CalendarBulkControls {
                    id: calendarBulkControls
                    Layout.fillWidth: true
                    selectedEventIds: window.selectedCalendarEventIds
                    calendarSourceModel: window.calendarSourceModel
                    statusMessage: window.controllerString("bulkEventStatusMessage", "")
                    previewMessage: window.controllerString("bulkEventPreviewMessage", "")
                    previewRequestToken: window.bulkEventPreviewRequestToken
                    bulkTextRecurrenceScope: window.bulkTextRecurrenceScope
                    onClearSelectionRequested: {
                        window.clearCalendarEventSelection()
                        window.calendarSelectionMode = false
                    }
                    onBulkDeleteRequested: function(eventIds) {
                        window.controllerCall("bulkDeleteEvents", [eventIds])
                        window.clearCalendarEventSelection()
                    }
                    onBulkMoveRequested: function(eventIds, calendarId) {
                        window.controllerCall("bulkMoveEvents", [eventIds, calendarId])
                        window.clearCalendarEventSelection()
                    }
                    onBulkColorRequested: function(eventIds, colorId) {
                        window.controllerCall("bulkSetEventColor", [eventIds, colorId])
                        window.clearCalendarEventSelection()
                    }
                    onBulkAvailabilityRequested: function(eventIds, available) {
                        window.controllerCall("bulkSetEventAvailability", [eventIds, available])
                        window.clearCalendarEventSelection()
                    }
                    onBulkVisibilityRequested: function(eventIds, visibility) {
                        window.controllerCall("bulkSetEventVisibility", [eventIds, visibility])
                        window.clearCalendarEventSelection()
                    }
                    onBulkShiftRequested: function(eventIds, shiftMinutes) {
                        window.controllerCall("bulkShiftEventTimes", [eventIds, shiftMinutes])
                        window.clearCalendarEventSelection()
                    }
                    onBulkTextPreviewRequested: function(eventIds, findText, fields, recurrenceScope, requestToken) {
                        window.controllerCall("previewBulkEventText",
                                              [eventIds, findText, fields, recurrenceScope, requestToken])
                    }
                    onBulkTextReplaceRequested: function(eventIds, findText, replaceText, fields, recurrenceScope) {
                        window.controllerCall("bulkReplaceEventText",
                                              [eventIds, findText, replaceText, fields, recurrenceScope])
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    spacing: Theme.spacingSmall

                    StackLayout {
                        Layout.fillWidth: true
                        Layout.fillHeight: true
                        currentIndex: calendarViews.currentIndex

                    AgendaView {
                        id: agendaView
                        agendaModel: window.agendaModel
                        calendarVisibility: calendarVisibility
                        selectedEventIds: window.selectedCalendarEventIds
                        selectionMode: window.calendarSelectionMode
                        onEventSelectionRequested: function(eventId, selected) {
                            window.setCalendarEventSelected(eventId, selected)
                        }
                        onEventEditRequested: function(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                                                       startTimeZone, colorId, transparency, visibility,
                                                       attendeeEmailsJson, remindersJson, remindersUseDefault,
                                                       recurrenceRule, recurringRemoteId, originalStartAt,
                                                       eventType, conferenceJson, attachmentsJson,
                                                       guestPermissionsJson, statusPropertiesJson) {
                            window.openEventEdit(eventId, calendarId, title, startAt, endAt, allDay,
                                                 description, location, startTimeZone, colorId,
                                                 transparency, visibility, attendeeEmailsJson,
                                                 remindersJson, remindersUseDefault, recurrenceRule,
                                                 recurringRemoteId, originalStartAt, eventType, conferenceJson,
                                                 attachmentsJson, guestPermissionsJson, statusPropertiesJson)
                        }
                        onEventDetailRequested: function(event) { window.openEventDetail(event) }
                    }

                    DayTimelineView {
                        id: dayTimeline
                        timelineModel: window.timelineModel
                        calendarVisibility: calendarVisibility
                        selectedEventIds: window.selectedCalendarEventIds
                        selectionMode: window.calendarSelectionMode
                        dayIndex: window.calendarWeekDayIndex()
                        dateLabel: window.calendarDayHeading
                        dateIso: window.calendarDate
                        scheduledTaskIndex: window.appController !== null
                                            ? window.appController.scheduledTaskDateIndex : null
                        timelineActive: calendarViews.currentIndex === 1
                        bypassCalendarVisibility: window.timelineProfile
                        use24HourTime: window.use24HourTime
                        workdayStartHour: window.workdayStartHour
                        hourHeight: Theme.timelineHourHeight
                        onEventSelectionRequested: function(eventId, selected) {
                            window.setCalendarEventSelected(eventId, selected)
                        }
                        onEventMoveRequested: function(eventId, startAt, endAt, allDay) {
                            window.eventMoveRequested(eventId, startAt, endAt, allDay)
                            window.controllerCall("moveEvent", [eventId, startAt, endAt, allDay])
                        }
                        onEventResizeRequested: function(eventId, endAt) {
                            window.eventResizeRequested(eventId, endAt)
                            window.controllerCall("resizeEvent", [eventId, endAt])
                        }
                        onEventMoveScopeRequested: function(event, startAt, endAt, allDay) {
                            window.requestScopedEventMove(event, startAt, endAt, allDay)
                        }
                        onEventResizeScopeRequested: function(event, endAt) {
                            window.requestScopedEventResize(event, endAt)
                        }
                        onQuickCreateRequested: function(startAt, endAt, allDay) {
                            window.openCalendarQuickCreate(startAt, endAt, allDay)
                        }
                        onTaskMoveRequested: function(taskId, dueDate) {
                            window.controllerCall("bulkSetTaskDue", [[taskId], dueDate])
                        }
                        onTaskDetailRequested: function(task) { window.openTaskDetail(task) }
                        onEventEditRequested: function(eventId, calendarId, title, startAt, endAt, allDay,
                                                        description, location, startTimeZone, colorId,
                                                        transparency, visibility, attendeeEmailsJson,
                                                        remindersJson, remindersUseDefault, recurrenceRule,
                                                        recurringRemoteId, originalStartAt, eventType,
                                                        conferenceJson, attachmentsJson,
                                                        guestPermissionsJson, statusPropertiesJson) {
                            window.openEventEdit(eventId, calendarId, title, startAt, endAt, allDay,
                                                 description, location, startTimeZone, colorId,
                                                 transparency, visibility, attendeeEmailsJson,
                                                 remindersJson, remindersUseDefault, recurrenceRule,
                                                 recurringRemoteId, originalStartAt, eventType, conferenceJson,
                                                 attachmentsJson, guestPermissionsJson, statusPropertiesJson)
                        }
                        onEventDetailRequested: function(event) { window.openEventDetail(event) }
                        onPeriodNavigationRequested: direction => window.navigateCalendar(direction)
                    }

                    WeekTimelineView {
                        id: weekTimeline
                        timelineModel: window.timelineModel
                        calendarVisibility: calendarVisibility
                        selectedEventIds: window.selectedCalendarEventIds
                        selectionMode: window.calendarSelectionMode
                        dayLabels: window.calendarWeekLabels()
                        weekStartDate: window.shiftCalendarDate(-window.calendarWeekDayIndex())
                        scheduledTaskIndex: window.appController !== null
                                            ? window.appController.scheduledTaskDateIndex : null
                        timelineActive: calendarViews.currentIndex === 2
                        bypassCalendarVisibility: window.timelineProfile
                        use24HourTime: window.use24HourTime
                        workdayStartHour: window.workdayStartHour
                        hourHeight: Theme.timelineHourHeight
                        onEventSelectionRequested: function(eventId, selected) {
                            window.setCalendarEventSelected(eventId, selected)
                        }
                        onEventMoveRequested: function(eventId, startAt, endAt, allDay) {
                            window.eventMoveRequested(eventId, startAt, endAt, allDay)
                            window.controllerCall("moveEvent", [eventId, startAt, endAt, allDay])
                        }
                        onEventResizeRequested: function(eventId, endAt) {
                            window.eventResizeRequested(eventId, endAt)
                            window.controllerCall("resizeEvent", [eventId, endAt])
                        }
                        onEventMoveScopeRequested: function(event, startAt, endAt, allDay) {
                            window.requestScopedEventMove(event, startAt, endAt, allDay)
                        }
                        onEventResizeScopeRequested: function(event, endAt) {
                            window.requestScopedEventResize(event, endAt)
                        }
                        onEventAllDayResizeScopeRequested: function(event, startAt, endAt) {
                            window.requestScopedAllDayResize(event, startAt, endAt)
                        }
                        onQuickCreateRequested: function(startAt, endAt, allDay) {
                            window.openCalendarQuickCreate(startAt, endAt, allDay)
                        }
                        onTaskMoveRequested: function(taskId, dueDate) {
                            window.controllerCall("bulkSetTaskDue", [[taskId], dueDate])
                        }
                        onTaskDetailRequested: function(task) { window.openTaskDetail(task) }
                        onEventEditRequested: function(eventId, calendarId, title, startAt, endAt, allDay,
                                                        description, location, startTimeZone, colorId,
                                                        transparency, visibility, attendeeEmailsJson,
                                                        remindersJson, remindersUseDefault, recurrenceRule,
                                                        recurringRemoteId, originalStartAt, eventType,
                                                        conferenceJson, attachmentsJson,
                                                        guestPermissionsJson, statusPropertiesJson) {
                            window.openEventEdit(eventId, calendarId, title, startAt, endAt, allDay,
                                                 description, location, startTimeZone, colorId,
                                                 transparency, visibility, attendeeEmailsJson,
                                                 remindersJson, remindersUseDefault, recurrenceRule,
                                                 recurringRemoteId, originalStartAt, eventType, conferenceJson,
                                                 attachmentsJson, guestPermissionsJson, statusPropertiesJson)
                        }
                        onEventDetailRequested: function(event) { window.openEventDetail(event) }
                        onPeriodNavigationRequested: direction => window.navigateCalendar(direction)
                    }

                    MonthGridView {
                        id: monthGrid
                        monthGridModel: window.monthGridModel
                        calendarVisibility: calendarVisibility
                        selectedEventIds: window.selectedCalendarEventIds
                        selectionMode: window.calendarSelectionMode
                        monthLabel: window.calendarMonthLabel
                        calendarDate: window.calendarDate
                        scheduledTaskIndex: window.appController !== null
                                            ? window.appController.scheduledTaskDateIndex : null
                        weekStartDay: window.weekStartDay
                        onEventSelectionRequested: function(eventId, selected) {
                            window.setCalendarEventSelected(eventId, selected)
                        }
                        onDateSelected: function(date) {
                            window.controllerCall("setCalendarDate", [date])
                            calendarViews.currentIndex = 1
                        }
                        onEventCreateRequested: function(date) {
                            window.openEventCreate(date)
                        }
                        onQuickCreateRequested: function(startAt, endAt, allDay) {
                            window.openCalendarQuickCreate(startAt, endAt, allDay)
                        }
                        onEventMoveScopeRequested: function(event, startAt, endAt, allDay) {
                            window.requestScopedEventMove(event, startAt, endAt, allDay)
                        }
                        onEventAllDayResizeScopeRequested: function(event, startAt, endAt) {
                            window.requestScopedAllDayResize(event, startAt, endAt)
                        }
                        onTaskMoveRequested: function(taskId, dueDate) {
                            window.controllerCall("bulkSetTaskDue", [[taskId], dueDate])
                        }
                        onTaskDetailRequested: function(task) { window.openTaskDetail(task) }
                        onEventEditRequested: function(event) {
                            window.openEventEdit(event.id, event.calendarId, event.title, event.startAt,
                                                 event.endAt, event.allDay, event.description, event.location,
                                                 event.startTimeZone, event.colorId, event.transparency,
                                                 event.visibility, event.attendeeEmailsJson, event.remindersJson,
                                                 event.remindersUseDefault, event.recurrenceRule,
                                                 event.recurringRemoteId, event.originalStartAt, event.eventType,
                                                 event.conferenceJson, event.attachmentsJson,
                                                 event.guestPermissionsJson, event.statusPropertiesJson)
                        }
                        onEventDetailRequested: function(event) { window.openEventDetail(event) }
                        onPeriodNavigationRequested: direction => window.navigateCalendar(direction)
                    }
                    }

                    UnscheduledTaskTray {
                        id: unscheduledTaskTray
                        Layout.fillHeight: true
                        tasks: window.unscheduledTasks
                        onTaskDetailRequested: function(task) { window.openTaskDetail(task) }
                    }
                }
            }

            Flickable {
                id: settingsFlickable
                anchors.fill: parent
                visible: window.currentPage === "Settings"
                clip: true
                boundsBehavior: Flickable.StopAtBounds
                flickableDirection: Flickable.VerticalFlick
                contentWidth: width
                contentHeight: settingsContent.implicitHeight

                ScrollBar.vertical: ScrollBar {
                    policy: parent.contentHeight > parent.height ? ScrollBar.AsNeeded : ScrollBar.AlwaysOff
                }

                ColumnLayout {
                    id: settingsContent
                    width: parent.width
                    spacing: Theme.spacingMedium

                TextField {
                    id: settingsSearchField
                    Layout.fillWidth: true
                    placeholderText: "Find a setting"
                    Accessible.name: placeholderText
                    selectByMouse: true
                }

                Flow {
                    Layout.fillWidth: true
                    spacing: Theme.spacingSmall
                    visible: settingsSearchField.text.trim().length > 0

                    Repeater {
                        model: window.matchingSettingsSections(settingsSearchField.text)

                        delegate: Button {
                            required property var modelData
                            text: modelData.title
                            Accessible.name: "Go to " + text
                            onClicked: window.revealSettingsSection(modelData)
                        }
                    }
                }

                Label {
                    Layout.fillWidth: true
                    visible: settingsSearchField.text.trim().length > 0 &&
                             window.matchingSettingsSections(settingsSearchField.text).length === 0
                    text: "No matching settings section"
                    color: Theme.textSecondary
                }

                Label {
                    id: googleSetupHeading
                    text: "Google setup"
                    font.pixelSize: Theme.titleFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Label {
                    Layout.fillWidth: true
                    text: "Enter the Desktop OAuth client ID and secret from your Google Cloud project. Enable Google Tasks, Google Calendar, and Google Drive APIs. The app opens a local loopback callback while connecting."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                TextField {
                    id: googleClientIdField
                    Layout.fillWidth: true
                    text: window.controllerString("clientId", "")
                    placeholderText: "Desktop OAuth client ID"
                    Accessible.name: placeholderText
                }

                RowLayout {
                    Layout.fillWidth: true

                    TextField {
                        id: googleClientSecretField
                        Layout.fillWidth: true
                        placeholderText: window.controllerBool("hasClientSecret", false)
                                         ? "••••••••••••"
                                         : "Desktop OAuth client secret"
                        echoMode: TextInput.Password
                        Accessible.name: "Desktop OAuth client secret"
                    }

                    Label {
                        visible: googleClientSecretField.text.length === 0
                                 && window.controllerBool("hasClientSecret", false)
                        text: "✓"
                        color: "#16803c"
                        font.pixelSize: Theme.labelFontSize
                        font.bold: true
                        Accessible.name: "Google client secret saved"
                    }
                }

                Connections {
                    target: window.appController

                    function onStatusMessageChanged() {
                        if (window.controllerString("statusMessage", "") === "Google client configuration saved")
                            googleClientSecretField.clear()
                    }
                }

                Button {
                    text: "Save OAuth client"
                    enabled: googleClientIdField.text.trim().length > 0 &&
                             (window.appController === null || !window.appController.busy)
                    Accessible.name: text
                    onClicked: window.controllerCall("saveClientId", [googleClientIdField.text, googleClientSecretField.text])
                }

                Button {
                    text: window.appController !== null && window.appController.googleConnected ? "Reconnect Google" : "Connect Google"
                    enabled: window.appController !== null &&
                             window.controllerString("clientId", "").length > 0 &&
                             !window.appController.busy
                    Accessible.name: text
                    onClicked: window.controllerCall("connectGoogle", [])
                }

                Button {
                    text: "Sync Google now"
                    enabled: window.appController !== null && window.appController.googleConnected &&
                             !window.appController.busy
                    Accessible.name: text
                    onClicked: window.controllerCall("syncGoogle", [])
                }

                Label {
                    id: googleCalendarsHeading
                    text: "Google calendars"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                TextField {
                    id: newGoogleCalendarTitle
                    Layout.fillWidth: true
                    placeholderText: "New calendar name"
                    Accessible.name: "New Google Calendar name"
                    selectByMouse: true
                }

                TextField {
                    id: newGoogleCalendarDescription
                    Layout.fillWidth: true
                    placeholderText: "New calendar description (optional)"
                    Accessible.name: "New Google Calendar description"
                    selectByMouse: true
                }

                Button {
                    text: "Create Google Calendar"
                    enabled: window.appController !== null && window.appController.googleConnected &&
                             newGoogleCalendarTitle.text.trim().length > 0 && !window.appController.busy
                    Accessible.name: text
                    onClicked: window.controllerCall("createGoogleCalendar", [newGoogleCalendarTitle.text,
                                                                                newGoogleCalendarDescription.text,
                                                                                window.appController.displayTimeZone])
                }

                TextField {
                    id: subscribeGoogleCalendarId
                    Layout.fillWidth: true
                    placeholderText: "Calendar ID to subscribe to"
                    Accessible.name: "Google Calendar ID to subscribe to"
                    selectByMouse: true
                }

                Button {
                    text: "Subscribe to Google Calendar"
                    enabled: window.appController !== null && window.appController.googleConnected &&
                             subscribeGoogleCalendarId.text.trim().length > 0 && !window.appController.busy
                    Accessible.name: text
                    onClicked: window.controllerCall("subscribeGoogleCalendar", [subscribeGoogleCalendarId.text])
                }

                Button {
                    text: "Import Tasks and events"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: text
                    onClicked: window.controllerCall("chooseImportFile", [])
                }

                Label {
                    text: "Manage Google calendars"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Label {
                    Layout.fillWidth: true
                    text: "Google visibility is synced separately from HCB’s local calendar-view filters. Owned secondary calendars can be renamed or deleted; subscription and display preferences apply to every accessible calendar."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Repeater {
                    model: window.appController !== null && window.appController.calendarManagementRows !== undefined
                           ? window.appController.calendarManagementRows : []

                    delegate: Frame {
                        required property var modelData
                        Layout.fillWidth: true

                        ColumnLayout {
                            anchors.fill: parent
                            spacing: Theme.spacingSmall

                            RowLayout {
                                Layout.fillWidth: true
                                Label {
                                    Layout.fillWidth: true
                                    text: modelData.title
                                    font.bold: true
                                    elide: Text.ElideRight
                                    Accessible.name: text
                                }
                                Label {
                                    text: modelData.primary ? "Primary" : modelData.accessRole
                                    color: Theme.textSecondary
                                    Accessible.name: text
                                }
                            }

                            Label {
                                Layout.fillWidth: true
                                text: (modelData.hidden ? "Hidden in Google Calendar" : "Visible in Google Calendar") +
                                      " · " + (modelData.selected ? "selected" : "not selected")
                                color: Theme.textSecondary
                                Accessible.name: text
                            }

                            RowLayout {
                                Layout.fillWidth: true

                                Button {
                                    text: "Settings"
                                    enabled: window.appController !== null && !window.appController.busy
                                    Accessible.name: modelData.title + " calendar settings"
                                    onClicked: calendarManagerDialog.openForCalendar(modelData)
                                }

                                Button {
                                    visible: !modelData.primary
                                    text: "Unsubscribe"
                                    enabled: window.appController !== null && !window.appController.busy
                                    Accessible.name: "Unsubscribe from " + modelData.title
                                    onClicked: {
                                        calendarRemovalDialog.calendarId = modelData.id
                                        calendarRemovalDialog.calendarTitle = modelData.title
                                        calendarRemovalDialog.deleteCalendar = false
                                        calendarRemovalDialog.open()
                                    }
                                }

                                Button {
                                    visible: !modelData.primary && modelData.accessRole === "owner"
                                    text: "Delete calendar"
                                    enabled: window.appController !== null && !window.appController.busy
                                    Accessible.name: "Delete " + modelData.title
                                    onClicked: {
                                        calendarRemovalDialog.calendarId = modelData.id
                                        calendarRemovalDialog.calendarTitle = modelData.title
                                        calendarRemovalDialog.deleteCalendar = true
                                        calendarRemovalDialog.open()
                                    }
                                }
                            }
                        }
                    }
                }

                Label {
                    Layout.fillWidth: true
                    text: "Calendar sharing/ACL administration is intentionally deferred; this release creates calendars and subscribes to calendars you can access."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Label {
                    id: remindersHeading
                    text: "Desktop reminders"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Label {
                    Layout.fillWidth: true
                    text: window.controllerString("reminderStatusMessage", "Calendar reminders are unavailable")
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Label {
                    id: notesPresentationHeading
                    text: "Undated task presentation"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Switch {
                    id: notesEnabledSwitch
                    text: "Show undated tasks as Notes"
                    checked: window.notesEnabled
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: text
                    Accessible.description: "Changes only local presentation; Google Tasks are unchanged"
                    onToggled: window.controllerCall("saveNotesEnabled", [checked])
                }

                ComboBox {
                    id: notesProjectionSelector
                    Layout.fillWidth: true
                    visible: window.notesEnabled
                    model: ["Notes only", "Mirror in Tasks and Notes"]
                    currentIndex: window.notesProjectionMode
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Undated task presentation mode"
                    onActivated: index => window.controllerCall("saveNotesProjectionMode", [index])
                }

                Label {
                    Layout.fillWidth: true
                    text: window.notesEnabled
                          ? "Notes are ordinary undated Google Tasks. This setting only filters local views."
                          : "Notes are disabled. Every Google Task remains in Tasks."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Label {
                    id: sidebarHeading
                    text: "Sidebar"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Label {
                    Layout.fillWidth: true
                    text: "Choose and order tabs. Settings always stays last."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Repeater {
                    model: [{id: "tasks", title: "Tasks"}, {id: "calendar", title: "Calendar"},
                            {id: "notes", title: "Notes"}, {id: "invitations", title: "Invitations"}]
                    delegate: RowLayout {
                        required property var modelData
                        Layout.fillWidth: true
                        Switch {
                            checked: window.sidebarHasTab(modelData.id)
                            text: modelData.title
                            enabled: window.appController !== null && !window.appController.busy
                            onToggled: window.setSidebarTab(modelData.id, checked)
                        }
                        Item { Layout.fillWidth: true }
                        Button {
                            text: "↑"
                            enabled: window.sidebarTabIds.indexOf(modelData.id) > 0
                            Accessible.name: "Move " + modelData.title + " earlier"
                            onClicked: window.moveSidebarTab(modelData.id, -1)
                        }
                        Button {
                            text: "↓"
                            enabled: window.sidebarTabIds.indexOf(modelData.id) >= 0 &&
                                     window.sidebarTabIds.indexOf(modelData.id) < window.sidebarTabIds.length - 1
                            Accessible.name: "Move " + modelData.title + " later"
                            onClicked: window.moveSidebarTab(modelData.id, 1)
                        }
                    }
                }

                Label {
                    id: appearanceHeading
                    text: "Appearance and calendar"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                ComboBox {
                    Layout.fillWidth: true
                    model: ["System appearance", "Light", "Dark"]
                    currentIndex: window.appearanceMode
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Appearance"
                    onActivated: index => window.controllerCall("saveAppearanceMode", [index])
                }

                ComboBox {
                    Layout.fillWidth: true
                    model: ["Compact density", "Standard density", "Comfortable density"]
                    currentIndex: window.visualDensity
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Visual density"
                    onActivated: index => window.controllerCall("saveVisualDensity", [index])
                }

                ComboBox {
                    id: paletteModeSelector
                    objectName: "paletteModeSelector"
                    Layout.fillWidth: true
                    model: ["System palette", "Violet", "Blue", "Green", "Rose", "Amber"]
                    currentIndex: window.paletteMode
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Color palette"
                    onActivated: index => window.controllerCall("savePaletteMode", [index])
                }

                TextField {
                    id: accentColorField
                    objectName: "accentColorField"
                    Layout.fillWidth: true
                    text: window.accentColor
                    placeholderText: "Custom accent #RRGGBB (optional)"
                    maximumLength: 7
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Custom accent color"
                    selectByMouse: true
                    onEditingFinished: window.controllerCall("saveAccentColor", [text])
                }

                ComboBox {
                    id: fontFamilySelector
                    objectName: "fontFamilySelector"
                    Layout.fillWidth: true
                    model: window.fontFamilyOptions()
                    currentIndex: Math.max(0, window.fontFamilyOptions().indexOf(window.fontFamily))
                    editable: true
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Font family"
                    onActivated: index => window.controllerCall("saveFontFamily",
                                                                 [index === 0 ? "" : window.fontFamilyOptions()[index]])
                    onAccepted: window.controllerCall("saveFontFamily",
                                                       [editText === "System default" ? "" : editText])
                }

                ComboBox {
                    id: fontScaleSelector
                    objectName: "fontScaleSelector"
                    Layout.fillWidth: true
                    model: ["Small text (90%)", "Standard text (100%)", "Large text (112%)", "Extra-large text (125%)"]
                    currentIndex: window.fontScale
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Font size"
                    onActivated: index => window.controllerCall("saveFontScale", [index])
                }

                Label {
                    id: linksHeading
                    text: "Links"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                TextField {
                    id: externalBrowserField
                    Layout.fillWidth: true
                    text: window.externalBrowser
                    placeholderText: "System default browser"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Browser for external links"
                    Accessible.description: "Enter a macOS application name, such as Safari or Google Chrome. Leave blank for the system default browser."
                    selectByMouse: true
                    onEditingFinished: window.controllerCall("saveExternalBrowser", [text])
                }

                Label {
                    Layout.fillWidth: true
                    text: "Enter Safari, Google Chrome, Firefox, or another macOS browser. Leave blank for the system default; hold Command or Ctrl while clicking an HTTP or HTTPS link to open it."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Label {
                    id: undoHeading
                    text: "Undo history"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                RowLayout {
                    Layout.fillWidth: true

                    Label { text: "Keep" }

                    SpinBox {
                        id: undoRetentionDaysField
                        from: 1
                        to: 3650
                        value: window.undoRetentionDays
                        editable: true
                        Accessible.name: "Undo retention days"
                    }

                    Label { text: "days /" }

                    SpinBox {
                        id: undoMaximumEntriesField
                        from: 50
                        to: 1000
                        value: window.undoMaximumEntries
                        editable: true
                        Accessible.name: "Maximum undo history entries"
                    }

                    Label { text: "actions" }

                    Button {
                        text: "Save undo history"
                        enabled: window.appController !== null && !window.appController.busy
                        onClicked: window.controllerCall("saveUndoHistorySettings",
                                                         [undoRetentionDaysField.value,
                                                          undoMaximumEntriesField.value])
                    }
                }

                Label {
                    Layout.fillWidth: true
                    text: "Undo is stored locally. It stops if the item changed outside Hot Cross Buns."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Button {
                    id: resetVisualPreferencesButton
                    objectName: "resetVisualPreferencesButton"
                    Layout.fillWidth: true
                    text: "Reset visual settings"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: text
                    Accessible.description: "Restores system palette, default font, standard text size, and standard density"
                    onClicked: window.controllerCall("resetVisualPreferences", [])
                }

                Label {
                    id: bulkRewritingHeading
                    text: "Bulk rewriting"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                ComboBox {
                    Layout.fillWidth: true
                    model: ["Skip recurring by default", "Current occurrence by default",
                            "Current + future by default", "Full series by default"]
                    currentIndex: window.bulkTextRecurrenceScope
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Default bulk rewrite recurrence scope"
                    onActivated: index => window.controllerCall("saveBulkTextRecurrenceScope", [index])
                }

                Label {
                    Layout.fillWidth: true
                    text: "Each find-and-replace run can override this default. Google Calendar full-series edits retain explicit instance overrides."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Label {
                    id: quickCaptureHeading
                    text: "Quick Capture"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Label {
                    Layout.fillWidth: true
                    text: "Ctrl+Shift+N opens an event by default. Choose default destinations and how recognized text is saved."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                ComboBox {
                    id: quickCaptureTaskDefaultSelector
                    Layout.fillWidth: true
                    property int taskListRevision: window.taskListModel !== null && window.taskListModel.revision !== undefined
                                                   ? window.taskListModel.revision : 0
                    model: {
                        const revision = taskListRevision
                        return window.taskListModel !== null && typeof window.taskListModel.selectedTaskLists === "function"
                               ? window.taskListModel.selectedTaskLists() : []
                    }
                    textRole: "title"
                    valueRole: "id"
                    currentIndex: indexOfValue(window.quickCaptureDefaultTaskListId)
                    displayText: currentIndex >= 0 ? currentText : "Quick Capture default Google Task list"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture default Google Task list"
                    onActivated: window.controllerCall("saveQuickCaptureDefaultTaskListId", [currentValue])
                }

                Button {
                    Layout.fillWidth: true
                    text: "Clear Quick Capture Task default"
                    enabled: window.appController !== null && !window.appController.busy &&
                             window.quickCaptureDefaultTaskListId.length > 0
                    Accessible.name: text
                    onClicked: window.controllerCall("saveQuickCaptureDefaultTaskListId", [""])
                }

                ComboBox {
                    id: quickCaptureCalendarDefaultSelector
                    Layout.fillWidth: true
                    property int calendarSourceRevision: window.calendarSourceModel !== null &&
                                                         window.calendarSourceModel.revision !== undefined
                                                         ? window.calendarSourceModel.revision : 0
                    model: window.calendarSourceModel
                    textRole: "title"
                    valueRole: "id"
                    currentIndex: {
                        const revision = calendarSourceRevision
                        return indexOfValue(window.quickCaptureDefaultCalendarId)
                    }
                    displayText: currentIndex >= 0 ? currentText : "Quick Capture default Google Calendar"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture default Google Calendar"
                    onActivated: window.controllerCall("saveQuickCaptureDefaultCalendarId", [currentValue])
                }

                Button {
                    Layout.fillWidth: true
                    text: "Clear Quick Capture Calendar default"
                    enabled: window.appController !== null && !window.appController.busy &&
                             window.quickCaptureDefaultCalendarId.length > 0
                    Accessible.name: text
                    onClicked: window.controllerCall("saveQuickCaptureDefaultCalendarId", [""])
                }

                RowLayout {
                    Layout.fillWidth: true

                    SpinBox {
                        id: quickCaptureDurationSelector
                        from: 1
                        to: 1440
                        value: window.appController !== null &&
                               typeof window.appController.quickCaptureEventDurationMinutes === "number"
                               ? window.appController.quickCaptureEventDurationMinutes : 30
                        editable: true
                        enabled: window.appController !== null && !window.appController.busy
                        Accessible.name: "Quick Capture default event duration in minutes"
                    }

                    Label { text: "minutes for events without an end time" }

                    Button {
                        text: "Save duration"
                        enabled: window.appController !== null && !window.appController.busy
                        Accessible.name: text
                        onClicked: window.controllerCall("saveQuickCaptureEventDurationMinutes",
                                                         [quickCaptureDurationSelector.value])
                    }
                }

                Switch {
                    id: quickCaptureRemoveParsedTextSwitch
                    text: "Remove recognized text from Quick Capture titles"
                    checked: window.appController !== null && window.appController.quickCaptureRemoveParsedText === true
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: text
                    onToggled: window.controllerCall("saveQuickCaptureRemoveParsedText", [checked])
                }

                Label {
                    Layout.fillWidth: true
                    text: "Type and priority aliases are comma-separated English words. They cannot overlap."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                TextField {
                    id: quickCaptureTaskAliasesField
                    Layout.fillWidth: true
                    text: window.appController !== null ? window.appController.quickCaptureTaskAliases || "task" : "task"
                    placeholderText: "Task aliases"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture Task aliases"
                    selectByMouse: true
                }

                TextField {
                    id: quickCaptureEventAliasesField
                    Layout.fillWidth: true
                    text: window.appController !== null ? window.appController.quickCaptureEventAliases || "event" : "event"
                    placeholderText: "Event aliases"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture Event aliases"
                    selectByMouse: true
                }

                TextField {
                    id: quickCaptureHighPriorityAliasesField
                    Layout.fillWidth: true
                    text: window.appController !== null ? window.appController.quickCaptureHighPriorityAliases || "p1" : "p1"
                    placeholderText: "High priority aliases"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture high-priority aliases"
                    selectByMouse: true
                }

                TextField {
                    id: quickCaptureMediumPriorityAliasesField
                    Layout.fillWidth: true
                    text: window.appController !== null ? window.appController.quickCaptureMediumPriorityAliases || "p2" : "p2"
                    placeholderText: "Medium priority aliases"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture medium-priority aliases"
                    selectByMouse: true
                }

                TextField {
                    id: quickCaptureLowPriorityAliasesField
                    Layout.fillWidth: true
                    text: window.appController !== null ? window.appController.quickCaptureLowPriorityAliases || "p3" : "p3"
                    placeholderText: "Low priority aliases"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Quick Capture low-priority aliases"
                    selectByMouse: true
                }

                Button {
                    Layout.fillWidth: true
                    text: "Save Quick Capture aliases"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: text
                    onClicked: window.controllerCall("saveQuickCaptureAliases",
                                                     [quickCaptureTaskAliasesField.text,
                                                      quickCaptureEventAliasesField.text,
                                                      quickCaptureHighPriorityAliasesField.text,
                                                      quickCaptureMediumPriorityAliasesField.text,
                                                      quickCaptureLowPriorityAliasesField.text])
                }

                ComboBox {
                    Layout.fillWidth: true
                    model: ["Sunday first", "Monday first"]
                    currentIndex: window.weekStartDay
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "First day of week"
                    onActivated: index => window.controllerCall("saveWeekStartDay", [index])
                }

                Switch {
                    text: "Use 24-hour time"
                    checked: window.use24HourTime
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: text
                    onToggled: window.controllerCall("saveUse24HourTime", [checked])
                }

                ComboBox {
                    id: displayTimeZoneField
                    Layout.fillWidth: true
                    model: window.appController !== null && window.appController.availableTimeZones !== undefined
                           ? window.appController.availableTimeZones : []
                    currentIndex: Math.max(0, model.indexOf(window.appController !== null &&
                                                            typeof window.appController.displayTimeZone === "string"
                                                            ? window.appController.displayTimeZone : ""))
                    editable: true
                    displayText: currentIndex > 0 ? currentText : "Display time zone (IANA)"
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Display time zone"
                    onActivated: index => window.controllerCall("saveDisplayTimeZone", [model[index]])
                    onAccepted: window.controllerCall("saveDisplayTimeZone", [editText])
                }

                RowLayout {
                    Layout.fillWidth: true

                    SpinBox {
                        id: workdayStart
                        from: 0
                        to: 23
                        value: window.workdayStartHour
                        editable: true
                        enabled: window.appController !== null && !window.appController.busy
                        Accessible.name: "Workday starts at hour"
                    }

                    Label { text: "to" }

                    SpinBox {
                        id: workdayEnd
                        from: 1
                        to: 24
                        value: window.workdayEndHour
                        editable: true
                        enabled: window.appController !== null && !window.appController.busy
                        Accessible.name: "Workday ends at hour"
                    }

                    Button {
                        text: "Save work hours"
                        enabled: window.appController !== null && !window.appController.busy
                        Accessible.name: text
                        onClicked: window.controllerCall("saveWorkdayHours", [workdayStart.value, workdayEnd.value])
                    }
                }

                Label {
                    text: "Conflict handling"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                ComboBox {
                    id: conflictPolicySelector
                    Layout.fillWidth: true
                    model: ["Prefer Google", "Prefer Hot Cross Buns", "Ask each time"]
                    currentIndex: window.appController !== null &&
                                  typeof window.appController.conflictPolicy === "number"
                                  ? window.appController.conflictPolicy : 0
                    enabled: window.appController !== null && !window.appController.busy
                    Accessible.name: "Sync conflict handling"
                    onActivated: index => window.controllerCall("saveConflictPolicy", [index])
                }

                Label {
                    Layout.fillWidth: true
                    text: "Google is the default. ‘Ask each time’ keeps conflicting changes pending until you choose."
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                Repeater {
                    model: window.appController !== null && window.appController.unresolvedConflicts !== undefined
                           ? window.appController.unresolvedConflicts : []
                    delegate: RowLayout {
                        required property var modelData
                        Layout.fillWidth: true
                        spacing: Theme.spacingSmall

                        Label {
                            Layout.fillWidth: true
                            text: modelData.resource + ": " + modelData.message
                            wrapMode: Text.WordWrap
                            color: Theme.textSecondary
                        }

                        Button {
                            text: "Keep Google"
                            enabled: !window.appController.busy
                            Accessible.name: text
                            onClicked: window.controllerCall("resolveSyncConflict", [modelData.id, false])
                        }

                        Button {
                            text: "Keep HCB"
                            enabled: modelData.canKeepLocal && !window.appController.busy
                            Accessible.name: text
                            onClicked: window.controllerCall("resolveSyncConflict", [modelData.id, true])
                        }
                    }
                }

                Label {
                    visible: window.appController !== null && window.appController.resolvedConflicts !== undefined &&
                             window.appController.resolvedConflicts.length > 0
                    text: "Recent conflict resolutions"
                    font.pixelSize: Theme.bodyFontSize
                    Accessible.role: Accessible.Heading
                    Accessible.name: text
                }

                Repeater {
                    model: window.appController !== null && window.appController.resolvedConflicts !== undefined
                           ? window.appController.resolvedConflicts : []
                    delegate: Label {
                        required property var modelData
                        Layout.fillWidth: true
                        text: modelData.resource + ": " + modelData.resolution + " — " + modelData.message
                        wrapMode: Text.WordWrap
                        color: Theme.textSecondary
                        Accessible.name: text
                    }
                }

                Label {
                    Layout.fillWidth: true
                    visible: window.controllerString("statusMessage", "").length > 0
                    text: window.controllerString("statusMessage", "")
                    wrapMode: Text.WordWrap
                    color: Theme.textSecondary
                }

                    Item { Layout.fillHeight: true }
                }
            }

            ColumnLayout {
                id: genericPageFallback
                anchors.fill: parent
                visible: window.currentPage !== "Tasks" && window.currentPage !== "Notes" &&
                         window.currentPage !== "Calendar" && window.currentPage !== "Settings" &&
                         window.currentPage !== "Onboarding" && window.currentPage !== "Invitations"
                spacing: Theme.spacingMedium
                Label {
                    text: window.currentPage
                    font.pixelSize: Theme.titleFontSize
                }
                Label {
                    text: "Qt Quick presents small C++ model diffs; sync, search, recurrence, and SQLite stay off the UI thread."
                    wrapMode: Text.WordWrap
                    Layout.fillWidth: true
                    color: Theme.textSecondary
                }
                Item { Layout.fillHeight: true }
            }
        }
    }

    onSearchResultActivated: function(resource, resultId, title, detail, scheduledAt) {
        if (resource === "note") {
            notesList.selectNote(resultId)
        }
    }

    onNotesEnabledChanged: {
        if (!notesEnabled && currentPage === "Notes") {
            currentPage = "Tasks"
        }
    }

    Pane {
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        anchors.margins: Theme.spacingMedium
        z: 3
        visible: window.undoLabel.length > 0 || window.redoLabel.length > 0 || window.pendingSyncCount > 0
        padding: Theme.spacingSmall

        RowLayout {
            spacing: Theme.spacingSmall

            Label {
                visible: window.pendingSyncCount > 0
                text: window.pendingSyncCount + " change" +
                      (window.pendingSyncCount === 1 ? "" : "s") + " pending Google sync"
                color: Theme.textSecondary
            }

            Button {
                visible: window.undoLabel.length > 0
                text: "Undo " + window.undoLabel
                Accessible.name: text
                onClicked: window.controllerCall("undo", [])
            }

            Button {
                visible: window.redoLabel.length > 0
                text: "Redo " + window.redoLabel
                Accessible.name: text
                onClicked: window.controllerCall("redo", [])
            }
        }
    }

    Rectangle {
        anchors.fill: parent
        visible: !timelineProfile && appController !== null && !appController.googleConnected &&
                 currentPage !== "Onboarding"
        z: 1
        color: Theme.background

        ColumnLayout {
            anchors.centerIn: parent
            width: Math.min(parent.width - Theme.spacingLarge * 2, 440)
            spacing: Theme.spacingMedium

            Label {
                Layout.fillWidth: true
                text: "Connect Google to continue"
                font.pixelSize: Theme.titleFontSize
                horizontalAlignment: Text.AlignHCenter
            }

            Button {
                Layout.alignment: Qt.AlignHCenter
                text: "Open Google setup"
                Accessible.name: text
                onClicked: window.openGoogleOnboarding()
            }
        }
    }
}
