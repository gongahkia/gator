import QtQuick
import QtQuick.Controls
import QtTest

TestCase {
    id: testCase
    name: "Main"
    when: windowShown

    property var startedTransitions: []
    property var completedTransitions: []

    ListModel {
        id: navigationCommands
        ListElement { commandId: "navigation.tasks"; commandLabel: "Tasks"; commandShortcut: "Ctrl+1" }
        ListElement { commandId: "navigation.calendar"; commandLabel: "Calendar"; commandShortcut: "Ctrl+2" }
        ListElement { commandId: "navigation.notes"; commandLabel: "Notes"; commandShortcut: "Ctrl+3" }
        ListElement { commandId: "navigation.settings"; commandLabel: "Settings"; commandShortcut: "Ctrl+," }
    }

    SystemPalette {
        id: systemPalette
        colorGroup: SystemPalette.Active
    }

    function begin(name) {
        startedTransitions.push(name)
        return true
    }

    function complete(name) {
        completedTransitions.push(name)
        return true
    }

    function test_loadsApplicationWindow() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        compare(mainWindow.title, "Hot Cross Buns")
        compare(mainWindow.width, 1200)
        compare(mainWindow.height, 760)
        compare(mainWindow.minimumWidth, 900)
        compare(mainWindow.minimumHeight, 600)
        compare(mainWindow.headerSearchButton.text, "")
        compare(mainWindow.headerSearchButton.icon.name, "system-search")
        compare(mainWindow.headerSearchButton.Accessible.name, "Search")
        mainWindow.destroy()
    }

    function test_emojiAutocompleteFindsSlackStyleAliases() {
        const component = Qt.createComponent("../../qml/EmojiAutocomplete.qml")
        compare(component.status, Component.Ready, component.errorString())
        const autocomplete = component.createObject(testCase)
        verify(autocomplete !== null)
        const rocket = autocomplete.search("rocket")
        verify(rocket.length > 0)
        compare(rocket[0].emoji, "🚀")
        const thumbsUp = autocomplete.search("thumb_up")
        verify(thumbsUp.length > 0)
        compare(thumbsUp[0].emoji, "👍")
        const watermelon = autocomplete.search("watermelon")
        verify(watermelon.length > 0)
        compare(watermelon[0].emoji, "🍉")
        const unitedStates = autocomplete.search("flag_united_states")
        verify(unitedStates.length > 0)
        compare(unitedStates[0].emoji, "🇺🇸")
        const mediumSkinTone = autocomplete.search("woman_medium_skin_tone")
        verify(mediumSkinTone.length > 0)
        compare(mediumSkinTone[0].emoji, "👩🏽")
        autocomplete.destroy()
    }

    function test_emojiAutocompleteOnlyTriggersAtTokenBoundary() {
        const component = Qt.createComponent("../../qml/EmojiAutocomplete.qml")
        compare(component.status, Component.Ready, component.errorString())
        const autocomplete = component.createObject(testCase)
        verify(autocomplete !== null)
        const trigger = autocomplete.triggerAt("Ship :rocket", 12)
        verify(trigger !== null)
        compare(trigger.start, 5)
        compare(trigger.query, "rocket")
        compare(autocomplete.triggerAt("https://example.test", 20), null)
        autocomplete.destroy()
    }

    function test_emojiAutocompleteReplacesTheTypedCode() {
        const component = Qt.createComponent("../../qml/EmojiAutocomplete.qml")
        compare(component.status, Component.Ready, component.errorString())
        const autocomplete = component.createObject(testCase)
        verify(autocomplete !== null)
        const input = {
            text: "Ship :rocket",
            cursorPosition: 12,
            remove: function(start, end) {
                this.text = this.text.slice(0, start) + this.text.slice(end)
                this.cursorPosition = start
            },
            insert: function(start, value) {
                this.text = this.text.slice(0, start) + value + this.text.slice(start)
                this.cursorPosition = start + value.length
            },
            forceActiveFocus: function() {}
        }
        autocomplete.target = input
        autocomplete.tokenStart = 5
        autocomplete.results = autocomplete.search("rocket")
        verify(autocomplete.choose(0))
        compare(input.text, "Ship 🚀 ")
        autocomplete.destroy()
    }

    function test_dateTimeEditorUsesUtcDatesForAllDayEvents() {
        const component = Qt.createComponent("../../qml/DateTimeEditor.qml")
        compare(component.status, Component.Ready, component.errorString())
        const editor = component.createObject(testCase, {
            value: "2026-08-01T15:00:00.000Z",
            allDay: false
        })
        verify(editor !== null)
        editor.allDay = true
        compare(editor.value, "2026-08-01T00:00:00.000Z")
        editor.destroy()
    }

    function test_dateTimeEditorUsesSelectedTimeZoneForTimedEvents() {
        const component = Qt.createComponent("../../qml/DateTimeEditor.qml")
        compare(component.status, Component.Ready, component.errorString())
        const converter = {
            dateTimeComponents: function(value, timeZone) {
                compare(value, "2026-08-01T01:30:00.000Z")
                compare(timeZone, "Asia/Singapore")
                return { year: 2026, month: 8, day: 1, hour: 9, minute: 30 }
            },
            dateTimeFromComponents: function(year, month, day, hour, minute, timeZone) {
                compare(year, 2026)
                compare(month, 7)
                compare(day, 1)
                compare(hour, 9)
                compare(minute, 30)
                compare(timeZone, "Asia/Singapore")
                return "2026-08-01T01:30:00.000Z"
            }
        }
        const editor = component.createObject(testCase, {
            value: "2026-08-01T01:30:00.000Z",
            timeZone: "Asia/Singapore",
            timeZoneConverter: converter
        })
        verify(editor !== null)
        editor.commit()
        compare(editor.value, "2026-08-01T01:30:00.000Z")
        editor.destroy()
    }

    function test_dateTimeEditorConvertsAllDayThroughSelectedTimeZone() {
        const component = Qt.createComponent("../../qml/DateTimeEditor.qml")
        compare(component.status, Component.Ready, component.errorString())
        const converter = {
            dateTimeComponents: function(value, timeZone) {
                compare(timeZone, "Asia/Singapore")
                if (value === "2026-08-01T01:30:00.000Z")
                    return { year: 2026, month: 8, day: 1, hour: 9, minute: 30 }
                return { year: 2026, month: 8, day: 1, hour: 8, minute: 0 }
            },
            dateTimeFromComponents: function(year, month, day, hour, minute, timeZone) {
                compare(year, 2026)
                compare(month, 7)
                compare(day, 1)
                compare(timeZone, "Asia/Singapore")
                return hour === 0 && minute === 0 ? "2026-07-31T16:00:00.000Z"
                                                  : "2026-08-01T01:30:00.000Z"
            }
        }
        const editor = component.createObject(testCase, {
            value: "2026-08-01T01:30:00.000Z",
            timeZone: "Asia/Singapore",
            timeZoneConverter: converter
        })
        verify(editor !== null)
        editor.allDay = true
        compare(editor.value, "2026-08-01T00:00:00.000Z")
        editor.allDay = false
        compare(editor.value, "2026-07-31T16:00:00.000Z")
        editor.destroy()
    }

    function test_recordsSidebarTransition() {
        startedTransitions = []
        completedTransitions = []
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            transitionTimings: testCase,
            appController: { googleConnected: true, notesEnabled: true,
                             sidebarTabIds: ["tasks", "calendar", "notes"] }
        })
        verify(mainWindow !== null)
        mainWindow.selectPage("Calendar")
        compare(mainWindow.currentPage, "Calendar")
        compare(startedTransitions, ["navigation.calendar"])
        tryVerify(function() {
            return completedTransitions.length === 1
        })
        compare(completedTransitions, ["navigation.calendar"])
        mainWindow.destroy()
    }

    function test_unconnectedGoogleRestrictsNavigationToOnboarding() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const commands = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        commands.append({ commandId: "navigation.tasks", commandLabel: "Tasks", commandShortcut: "Ctrl+1" })
        commands.append({ commandId: "navigation.calendar", commandLabel: "Calendar", commandShortcut: "Ctrl+2" })
        commands.append({ commandId: "navigation.invitations", commandLabel: "Invitations", commandShortcut: "Ctrl+3" })
        commands.append({ commandId: "navigation.settings", commandLabel: "Settings", commandShortcut: "Ctrl+," })

        const mainWindow = component.createObject(null, {
            navigationCommands: commands,
            appController: { googleConnected: false }
        })
        verify(mainWindow !== null)
        compare(mainWindow.navigationSidebar.pageButtons.count, 3)
        for (let row = 0; row < 2; ++row) {
            verify(!mainWindow.navigationSidebar.pageButtons.itemAt(row).enabled)
            verify(!mainWindow.navigationSidebar.pageButtons.itemAt(row).checked)
        }
        verify(mainWindow.navigationSidebar.pageButtons.itemAt(2).enabled)
        verify(!mainWindow.headerSearchButton.visible)
        verify(!mainWindow.searchShortcut.enabled)
        mainWindow.openSearch()
        verify(!mainWindow.searchPopup.opened)
        mainWindow.selectPage("Calendar")
        compare(mainWindow.currentPage, "Tasks")
        mainWindow.selectPage("Settings")
        compare(mainWindow.currentPage, "Onboarding")
        verify(mainWindow.googleOnboarding.visible)
        verify(!mainWindow.navigationSidebar.visible)
        compare(mainWindow.navigationSidebar.SplitView.preferredWidth, 0)
        mainWindow.destroy()
        commands.destroy()
    }

    function test_googleOnboardingSavesClientIdAndConnectsGoogle() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const calls = []
        const controller = {
            googleConnected: false,
            clientId: "client-id",
            busy: false,
            statusMessage: "SQLite calendar list preparation failed (1)",
            saveClientId: function(clientId, clientSecret) { calls.push(["saveClientId", clientId, clientSecret]) },
            connectGoogle: function() { calls.push(["connectGoogle"]) }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.openGoogleOnboarding()
        compare(mainWindow.currentPage, "Onboarding")
        compare(mainWindow.googleOnboarding.clientIdField.text, "client-id")
        verify(!mainWindow.googleOnboarding.statusLabel.visible)
        mainWindow.googleOnboarding.clientIdField.text = "updated-client-id"
        mainWindow.googleOnboarding.clientSecretField.text = "updated-client-secret"
        mainWindow.googleOnboarding.saveClientIdButton.click()
        mainWindow.googleOnboarding.connectGoogleButton.click()
        compare(calls, [["saveClientId", "updated-client-id", "updated-client-secret"], ["connectGoogle"]])
        mainWindow.destroy()
    }

    function test_googleOnboardingShowsSavedClientIdIndicator() {
        const component = Qt.createComponent("../../qml/GoogleOnboardingView.qml")
        compare(component.status, Component.Ready, component.errorString())
        const view = component.createObject(null, { clientId: "client-id" })
        verify(view !== null)
        view.saveClientIdButton.click()
        view.statusMessage = "Google client configuration saved"
        verify(view.clientIdSaved)
        verify(view.clientIdSavedIndicator.visible)
        verify(!view.statusLabel.visible)
        view.destroy()
    }

    function test_googleOnboardingMasksSavedClientSecret() {
        const component = Qt.createComponent("../../qml/GoogleOnboardingView.qml")
        compare(component.status, Component.Ready, component.errorString())
        const view = component.createObject(null, { clientId: "client-id" })
        verify(view !== null)
        view.clientSecretField.text = "client-secret"
        view.saveClientIdButton.click()
        view.statusMessage = "Google client configuration saved"
        verify(view.clientSecretSavedIndicator.visible)
        compare(view.clientSecretField.text, "")
        compare(view.clientSecretField.placeholderText, "••••••••••••")
        view.destroy()
    }

    function test_invitationsHidesGenericPageFallback() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: { googleConnected: true }
        })
        verify(mainWindow !== null)
        mainWindow.currentPage = "Invitations"
        verify(mainWindow.invitationInbox.visible)
        verify(!mainWindow.genericPageFallback.visible)
        mainWindow.destroy()
    }

    function test_sidebarRoutesSelectionThroughWindow() {
        startedTransitions = []
        completedTransitions = []
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            transitionTimings: testCase,
            appController: { googleConnected: true, notesEnabled: true,
                             sidebarTabIds: ["tasks", "calendar", "notes"] }
        })
        verify(mainWindow !== null)
        compare(mainWindow.navigationSidebar.currentPage, "Tasks")
        compare(navigationCommands.count, 4)
        mainWindow.navigationSidebar.selectPage("Unsupported")
        compare(mainWindow.currentPage, "Tasks")
        compare(startedTransitions, [])
        mainWindow.navigationSidebar.selectPage("Notes")
        compare(mainWindow.currentPage, "Notes")
        compare(mainWindow.navigationSidebar.currentPage, "Notes")
        compare(startedTransitions, ["navigation.notes"])
        tryVerify(function() {
            return completedTransitions.length === 1
        })
        compare(completedTransitions, ["navigation.notes"])
        mainWindow.destroy()
    }

    function test_navigationShortcutRoutesSelection() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: { googleConnected: true, notesEnabled: true,
                             sidebarTabIds: ["tasks", "calendar", "notes"] }
        })
        verify(mainWindow !== null)
        compare(mainWindow.navigationShortcuts.count, 4)
        const notesShortcut = mainWindow.navigationShortcuts.objectAt(2)
        verify(notesShortcut !== null)
        compare(notesShortcut.portableText, "Ctrl+3")
        notesShortcut.activated()
        compare(mainWindow.currentPage, "Notes")
        mainWindow.destroy()
    }

    function test_commandPaletteFiltersAndActivatesCommands() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: { googleConnected: true, notesEnabled: true,
                             sidebarTabIds: ["tasks", "calendar", "notes"] }
        })
        verify(mainWindow !== null)
        mainWindow.commandPaletteShortcut.activated()
        tryVerify(function() {
            return mainWindow.commandPalette.opened
        })
        tryVerify(function() {
            return mainWindow.commandPaletteQuery.activeFocus
        })
        compare(mainWindow.commandPaletteResults.count, 4)
        mainWindow.commandPaletteQuery.text = "notes"
        tryCompare(mainWindow.commandPaletteResults, "count", 1)
        mainWindow.commandPalette.activateCurrentCommand()
        compare(mainWindow.currentPage, "Notes")
        tryVerify(function() {
            return !mainWindow.commandPalette.opened
        })
        mainWindow.destroy()
    }

    function test_commandPaletteLaunchesQuickCaptureAction() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const commands = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        commands.append({ commandId: "navigation.tasks", commandLabel: "Tasks", commandShortcut: "Ctrl+1" })
        commands.append({ commandId: "create.quickCapture", commandLabel: "Quick Capture", commandShortcut: "Ctrl+Shift+N" })
        const calendars = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        calendars.append({ id: "calendar-primary", title: "Primary", accessRole: "owner" })
        const controller = {
            googleConnected: true,
            quickCaptureDefaultCalendarId: "calendar-primary",
            previewQuickCapture: function(text, kind) {
                return { kind: kind, rawTitle: text, parsedTitle: text, savedTitle: text,
                         date: "", time: "", allDay: false, eventReady: false, recognitions: [] }
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: commands,
            calendarSourceModel: calendars,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.openCommandPalette()
        mainWindow.commandPaletteQuery.text = "capture"
        tryCompare(mainWindow.commandPaletteResults, "count", 1)
        mainWindow.commandPalette.activateCurrentCommand()
        tryVerify(function() { return mainWindow.quickCapture.opened })
        compare(mainWindow.quickCapture.captureKind, 1)
        mainWindow.destroy()
        calendars.destroy()
        commands.destroy()
    }

    function test_notesAreHiddenByDefault() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        mainWindow.selectPage("Notes")
        compare(mainWindow.currentPage, "Tasks")
        compare(mainWindow.matchingCommands("").length, 3)
        mainWindow.destroy()
    }

    function test_commandPaletteRestoresPriorFocus() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        mainWindow.navigationSidebar.forceActiveFocus()
        tryVerify(function() {
            return mainWindow.navigationSidebar.activeFocus
        })
        mainWindow.openCommandPalette()
        tryVerify(function() {
            return mainWindow.commandPaletteQuery.activeFocus
        })
        mainWindow.commandPalette.close()
        tryVerify(function() {
            return mainWindow.navigationSidebar.activeFocus
        })
        mainWindow.destroy()
    }

    function test_searchUsesLocalResultsAndRoutesDeepLinks() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const results = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        results.append({ id: "task-1", resource: "task", title: "Release search", detail: "Verify cache",
                         scheduledAt: "2026-08-06", score: 100 })
        const calls = []
        const controller = {
            googleConnected: true,
            clientId: "",
            conflictPolicy: 0,
            unresolvedConflicts: [],
            resolvedConflicts: [],
            statusMessage: "",
            searchQuery: "",
            searchErrorMessage: "",
            searchFilterChips: [],
            searchLoading: false,
            savedSearches: [{ id: "saved-1", name: "Release", query: "release" }],
            busy: false,
            setSearchQuery: function(query) { calls.push({ method: "setSearchQuery", args: [query] }) },
            applySavedSearch: function(id) { calls.push({ method: "applySavedSearch", args: [id] }) },
            saveSearch: function(name, query) { calls.push({ method: "saveSearch", args: [name, query] }) },
            renameSavedSearch: function(id, name) { calls.push({ method: "renameSavedSearch", args: [id, name] }) },
            deleteSavedSearch: function(id) { calls.push({ method: "deleteSavedSearch", args: [id] }) }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller,
            searchResultsModel: results
        })
        verify(mainWindow !== null)
        let activated = null
        mainWindow.searchResultActivated.connect(function(resource, resultId, title, detail, scheduledAt) {
            activated = { resource, resultId, title, detail, scheduledAt }
        })
        mainWindow.searchShortcut.activated()
        tryVerify(function() { return mainWindow.searchPopup.opened && mainWindow.searchQuery.activeFocus })
        keyClick(Qt.Key_R)
        keyClick(Qt.Key_E)
        keyClick(Qt.Key_L)
        keyClick(Qt.Key_E)
        keyClick(Qt.Key_A)
        keyClick(Qt.Key_S)
        keyClick(Qt.Key_E)
        const searchCalls = calls.filter(function(call) { return call.method === "setSearchQuery" })
        verify(searchCalls.length > 0)
        verify(searchCalls[searchCalls.length - 1].args[0].length > 0)
        tryCompare(mainWindow.searchResults, "count", 1)
        mainWindow.searchResults.currentItem.click()
        compare(mainWindow.currentPage, "Tasks")
        compare(activated.resource, "task")
        compare(activated.resultId, "task-1")
        compare(activated.scheduledAt, "2026-08-06")
        tryVerify(function() { return !mainWindow.searchPopup.opened })

        mainWindow.openSearch()
        mainWindow.searchPopup.queryField.text = "release"
        mainWindow.searchPopup.savedSearchNameField.text = "Current"
        mainWindow.searchPopup.saveSearchButton.click()
        compare(calls[calls.length - 1].method, "saveSearch")
        compare(calls[calls.length - 1].args[0], "Current")
        mainWindow.searchPopup.controllerCall("applySavedSearch", ["saved-1"])
        mainWindow.searchPopup.controllerCall("renameSavedSearch", ["saved-1", "Renamed"])
        mainWindow.searchPopup.controllerCall("deleteSavedSearch", ["saved-1"])
        compare(calls[calls.length - 3].method, "applySavedSearch")
        compare(calls[calls.length - 2].method, "renameSavedSearch")
        compare(calls[calls.length - 1].method, "deleteSavedSearch")
        mainWindow.destroy()
        results.destroy()
    }

    function test_settingsSearchMatchesSettingsSections() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: { googleConnected: true }
        })
        verify(mainWindow !== null)

        mainWindow.settingsSearchField.text = "quick capture"
        const matches = mainWindow.matchingSettingsSections(mainWindow.settingsSearchField.text)
        compare(matches.length, 1)
        compare(matches[0].title, "Quick Capture")
        mainWindow.destroy()
    }

    function test_quickCaptureEmitsTaskRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const created = []
        const calendars = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        calendars.append({ id: "calendar-primary", title: "Primary", accessRole: "owner" })
        const controller = {
            googleConnected: true,
            quickCaptureDefaultCalendarId: "calendar-primary",
            previewQuickCapture: function(text, kind) {
                return { kind: kind, rawTitle: text, parsedTitle: text, savedTitle: text,
                         date: "2026-08-01", time: "", allDay: true, eventReady: true,
                         recognitions: [] }
            },
            createQuickCapture: function(title, kind, destinationId, disabledRecognitionIds) {
                created.push([title, kind, destinationId, disabledRecognitionIds])
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            calendarSourceModel: calendars,
            appController: controller
        })
        verify(mainWindow !== null)
        let capturedTitle = ""
        mainWindow.quickCaptureRequested.connect(function(title) { capturedTitle = title })
        mainWindow.quickCaptureShortcut.activated()
        tryVerify(function() {
            return mainWindow.quickCapture.opened && mainWindow.quickCapture.taskTitleField.activeFocus
        })
        mainWindow.quickCapture.taskTitle = "Ship native quick capture"
        verify(mainWindow.quickCapture.primaryEnabled)
        mainWindow.quickCapture.primaryButton.click()
        compare(capturedTitle, "Ship native quick capture")
        compare(created.length, 1)
        compare(created[0][0], "Ship native quick capture")
        compare(created[0][1], 1)
        compare(created[0][2], "calendar-primary")
        tryVerify(function() {
            return !mainWindow.quickCapture.opened
        })
        mainWindow.destroy()
        calendars.destroy()
    }

    function test_keyboardQuickCaptureSubmitsTaskRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const created = []
        const calendars = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        calendars.append({ id: "calendar-primary", title: "Primary", accessRole: "owner" })
        const controller = {
            googleConnected: true,
            quickCaptureDefaultCalendarId: "calendar-primary",
            previewQuickCapture: function(text, kind) {
                return { kind: kind, rawTitle: text, parsedTitle: text, savedTitle: text,
                         date: "2026-08-01", time: "", allDay: true, eventReady: true,
                         recognitions: [] }
            },
            createQuickCapture: function(title, kind, destinationId, disabledRecognitionIds) {
                created.push([title, kind, destinationId, disabledRecognitionIds])
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            calendarSourceModel: calendars,
            appController: controller
        })
        verify(mainWindow !== null)
        let capturedTitle = ""
        mainWindow.quickCaptureRequested.connect(function(title) { capturedTitle = title })
        mainWindow.requestActivate()
        tryCompare(mainWindow, "active", true)
        keyClick(Qt.Key_N, Qt.ControlModifier | Qt.ShiftModifier)
        tryVerify(function() {
            return mainWindow.quickCapture.opened && mainWindow.quickCapture.taskTitleField.activeFocus
        })
        mainWindow.quickCapture.taskTitle = "Ship native quick capture"
        keyClick(Qt.Key_Return)
        compare(capturedTitle, "Ship native quick capture")
        compare(created.length, 1)
        compare(created[0][1], 1)
        compare(created[0][2], "calendar-primary")
        tryVerify(function() {
            return !mainWindow.quickCapture.opened
        })
        mainWindow.destroy()
        calendars.destroy()
    }

    function test_taskListPresentsAndSelectsTasks() {
        const component = Qt.createComponent("../../qml/TaskListView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskModel.append({ id: "inbox-1", taskListId: "list-active", taskListTitle: "Active", title: "Plan release", notes: "Prepare checklist", dueAt: "2026-07-26", dueTimeZone: "Asia/Singapore", priority: 2, completed: false, managedRecurrence: false, recurrenceSummary: "", recurrenceFrequency: -1, recurrenceInterval: 1, recurrenceEndKind: 0, recurrenceEndUntil: "", recurrenceEndCount: 0 })
        taskModel.append({ id: "inbox-2", taskListId: "list-active", taskListTitle: "Active", title: "Review sync", notes: "", dueAt: "", dueTimeZone: "", priority: 0, completed: true, managedRecurrence: false, recurrenceSummary: "", recurrenceFrequency: -1, recurrenceInterval: 1, recurrenceEndKind: 0, recurrenceEndUntil: "", recurrenceEndCount: 0 })
        const taskList = component.createObject(null, {
            taskModel: taskModel,
            width: 480,
            height: 320,
            visible: true
        })
        verify(taskList !== null)
        tryCompare(taskList.taskRows, "rows", 2)
        let selectedId = ""
        taskList.taskSelected.connect(function(taskId) { selectedId = taskId })
        taskList.selectTask("inbox-1")
        compare(selectedId, "inbox-1")
        let createRequested = false
        taskList.taskCreateRequested.connect(function() { createRequested = true })
        taskList.taskCreateButton.click()
        verify(createRequested)
        let subtaskCreate = null
        taskList.taskSubtaskCreateRequested.connect(function(parentTaskId, taskListId) {
            subtaskCreate = { parentTaskId, taskListId }
        })
        let edit = null
        taskList.taskEditRequested.connect(function(taskId, title, notes, dueAt, dueTimeZone, priority) {
            edit = { taskId, title, notes, dueAt, dueTimeZone, priority }
        })
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)).editButton.click()
        compare(edit.taskId, "inbox-1")
        compare(edit.title, "Plan release")
        compare(edit.notes, "Prepare checklist")
        compare(edit.dueAt, "2026-07-26")
        compare(edit.dueTimeZone, "Asia/Singapore")
        compare(edit.priority, 2)
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)).subtaskButton.click()
        compare(subtaskCreate.parentTaskId, "inbox-1")
        compare(subtaskCreate.taskListId, "list-active")
        let completion = null
        taskList.taskCompletionRequested.connect(function(taskId, completed) {
            completion = { taskId, completed }
        })
        tryVerify(function() {
            return taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)) !== null
        })
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)).completionButton.click()
        compare(completion.taskId, "inbox-1")
        compare(completion.completed, true)
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(1, 0)).completionButton.click()
        compare(completion.taskId, "inbox-2")
        compare(completion.completed, false)
        let deletion = null
        taskList.taskDeleteRequested.connect(function(taskId, taskTitle) {
            deletion = { taskId, taskTitle }
        })
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)).deleteButton.click()
        compare(deletion.taskId, "inbox-1")
        compare(deletion.taskTitle, "Plan release")
        let move = null
        taskList.taskMoveRequested.connect(function(taskId, taskListId, taskTitle) {
            move = { taskId, taskListId, taskTitle }
        })
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)).moveButton.click()
        compare(move.taskId, "inbox-1")
        compare(move.taskListId, "list-active")
        compare(move.taskTitle, "Plan release")
        let reorder = null
        taskList.taskReorderRequested.connect(function(taskId, earlier) {
            reorder = { taskId, earlier }
        })
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)).moveEarlierButton.click()
        compare(reorder.taskId, "inbox-1")
        compare(reorder.earlier, true)
        taskList.taskRows.itemAtIndex(taskList.taskRows.index(1, 0)).moveLaterButton.click()
        compare(reorder.taskId, "inbox-2")
        compare(reorder.earlier, false)
        taskList.destroy()
        taskModel.destroy()
    }

    function test_agendaPresentsAndSelectsEvents() {
        const component = Qt.createComponent("../../qml/AgendaView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const agendaModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        agendaModel.append({
            id: "event-1",
            calendarId: "calendar-1",
            title: "Release review",
            startAt: "2026-07-26T10:00:00.000Z",
            endAt: "2026-07-26T11:00:00.000Z",
            description: "Verify the native package",
            location: "Studio",
            allDay: false,
            startTimeZone: "UTC",
            colorId: "",
            transparency: "opaque",
            visibility: "default",
            attendeeEmailsJson: "[]",
            remindersJson: "[]",
            remindersUseDefault: true,
            recurrenceRule: "",
            recurringRemoteId: "",
            originalStartAt: ""
        })
        agendaModel.append({
            id: "event-2",
            calendarId: "calendar-1",
            title: "Team offsite",
            startAt: "2026-07-27",
            endAt: "2026-07-28",
            description: "",
            location: "",
            allDay: true,
            startTimeZone: "UTC",
            colorId: "",
            transparency: "opaque",
            visibility: "default",
            attendeeEmailsJson: "[]",
            remindersJson: "[]",
            remindersUseDefault: true,
            recurrenceRule: "",
            recurringRemoteId: "",
            originalStartAt: ""
        })
        const agenda = component.createObject(null, {
            agendaModel: agendaModel,
            width: 480,
            height: 320,
            visible: true
        })
        verify(agenda !== null)
        tryCompare(agenda.eventRows, "count", 2)
        verify(agenda.scheduleLabel("2026-07-26T10:00:00.000Z", false).indexOf("T") === -1)
        compare(agenda.scheduleLabel("2026-07-27", true), "All day")
        let selectedId = ""
        agenda.eventSelected.connect(function(eventId) { selectedId = eventId })
        agenda.selectEvent("event-1")
        compare(selectedId, "event-1")
        let edit = null
        agenda.eventEditRequested.connect(function(eventId, calendarId, title, startAt, endAt, allDay, description, location) {
            edit = { eventId, calendarId, title, startAt, endAt, allDay, description, location }
        })
        agenda.requestEdit("event-1", "calendar-1", "Release review",
                           "2026-07-26T10:00:00.000Z", "2026-07-26T11:00:00.000Z", false,
                           "Verify the native package", "Studio")
        compare(edit.eventId, "event-1")
        compare(edit.calendarId, "calendar-1")
        compare(edit.title, "Release review")
        agenda.destroy()
        agendaModel.destroy()
    }

    function test_calendarEventSelectionHelpers() {
        const agendaComponent = Qt.createComponent("../../qml/AgendaView.qml")
        compare(agendaComponent.status, Component.Ready, agendaComponent.errorString())
        const agenda = agendaComponent.createObject(null, { selectedEventIds: ["event-1"] })
        verify(agenda !== null)
        verify(agenda.isEventSelected("event-1"))
        verify(!agenda.isEventSelected("event-2"))
        let agendaSelection = null
        agenda.eventSelectionRequested.connect(function(eventId, selected) {
            agendaSelection = { eventId: eventId, selected: selected }
        })
        agenda.eventSelectionRequested("event-2", true)
        compare(agendaSelection.eventId, "event-2")
        compare(agendaSelection.selected, true)
        agenda.destroy()

        const monthComponent = Qt.createComponent("../../qml/MonthGridView.qml")
        compare(monthComponent.status, Component.Ready, monthComponent.errorString())
        const month = monthComponent.createObject(null, { selectedEventIds: ["event-visible"] })
        verify(month !== null)
        compare(month.eventIds([{ id: "event-hidden", calendarId: "hidden" },
                                { id: "event-visible", calendarId: "visible" }]).length, 2)
        verify(month.isEventSelected("event-visible"))
        month.destroy()
    }

    function test_bulkCalendarSelectionAndActions() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const calendars = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        calendars.append({ id: "calendar-owner", title: "Owner", accessRole: "owner", selected: true })
        const calls = []
        const controller = {
            googleConnected: true,
            bulkEventStatusMessage: "",
            bulkDeleteEvents: function(eventIds) {
                calls.push({ action: "delete", eventIds: eventIds })
            },
            bulkMoveEvents: function(eventIds, calendarId) {
                calls.push({ action: "move", eventIds: eventIds, calendarId: calendarId })
            },
            bulkSetEventColor: function(eventIds, colorId) {
                calls.push({ action: "color", eventIds: eventIds, colorId: colorId })
            },
            bulkSetEventAvailability: function(eventIds, available) {
                calls.push({ action: "availability", eventIds: eventIds, available: available })
            },
            bulkSetEventVisibility: function(eventIds, visibility) {
                calls.push({ action: "visibility", eventIds: eventIds, visibility: visibility })
            },
            bulkShiftEventTimes: function(eventIds, shiftMinutes) {
                calls.push({ action: "shift", eventIds: eventIds, shiftMinutes: shiftMinutes })
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller,
            calendarSourceModel: calendars
        })
        verify(mainWindow !== null)
        mainWindow.setCalendarEventSelected("event-a", true)
        mainWindow.setCalendarEventSelected("event-b", true)
        compare(mainWindow.selectedCalendarEventIds.length, 2)
        mainWindow.calendarBulkControls.bulkDeleteRequested(mainWindow.selectedCalendarEventIds)
        compare(calls[0].action, "delete")
        compare(calls[0].eventIds.length, 2)
        compare(mainWindow.selectedCalendarEventIds.length, 0)
        mainWindow.setCalendarEventSelected("event-a", true)
        mainWindow.calendarBulkControls.bulkMoveRequested(mainWindow.selectedCalendarEventIds,
                                                           "calendar-owner")
        compare(calls[1].action, "move")
        compare(calls[1].calendarId, "calendar-owner")
        mainWindow.setCalendarEventSelected("event-a", true)
        mainWindow.calendarBulkControls.bulkColorRequested(mainWindow.selectedCalendarEventIds, "4")
        compare(calls[2].action, "color")
        compare(calls[2].colorId, "4")
        mainWindow.setCalendarEventSelected("event-a", true)
        mainWindow.calendarBulkControls.bulkAvailabilityRequested(mainWindow.selectedCalendarEventIds, true)
        compare(calls[3].action, "availability")
        compare(calls[3].available, true)
        mainWindow.setCalendarEventSelected("event-a", true)
        mainWindow.calendarBulkControls.bulkVisibilityRequested(mainWindow.selectedCalendarEventIds, "private")
        compare(calls[4].action, "visibility")
        compare(calls[4].visibility, "private")
        mainWindow.setCalendarEventSelected("event-a", true)
        mainWindow.calendarBulkControls.bulkShiftRequested(mainWindow.selectedCalendarEventIds, 60)
        compare(calls[5].action, "shift")
        compare(calls[5].shiftMinutes, 60)
        mainWindow.destroy()
        calendars.destroy()
    }

    function test_bulkTextReplaceDialogPreviewsBeforeApply() {
        const component = Qt.createComponent("../../qml/BulkTextReplaceDialog.qml")
        compare(component.status, Component.Ready, component.errorString())
        const dialog = component.createObject(null, { kind: "task" })
        verify(dialog !== null)
        let preview = null
        let replacement = null
        dialog.previewRequested.connect(function(ids, findText, fields, recurrenceScope, requestToken) {
            preview = { ids: ids, findText: findText, fields: fields, recurrenceScope: recurrenceScope,
                        requestToken: requestToken }
        })
        dialog.replaceRequested.connect(function(ids, findText, replaceText, fields, recurrenceScope) {
            replacement = { ids: ids, findText: findText, replaceText: replaceText,
                            fields: fields, recurrenceScope: recurrenceScope }
        })
        dialog.openFor(["task-a", "task-b"], 2)
        dialog.findTextField.text = "Alpha"
        dialog.replacementTextField.text = "Beta"
        verify(!dialog.primaryEnabled)
        dialog.previewRequestToken = 1
        dialog.previewRequested(["task-a", "task-b"], "Alpha", 3, 2, 1)
        verify(preview !== null)
        dialog.findTextField.text = "Gamma"
        dialog.previewMessage = "Preview: 2 records will change; 0 skipped."
        dialog.previewResultRequestToken = 1
        verify(!dialog.primaryEnabled)
        dialog.previewRequestToken = 3
        dialog.previewRequested(["task-a", "task-b"], "Gamma", 3, 2, 3)
        dialog.previewResultRequestToken = 3
        verify(dialog.primaryEnabled)
        dialog.primaryButton.click()
        verify(replacement !== null)
        compare(replacement.findText, "Gamma")
        compare(replacement.replaceText, "Beta")
        compare(replacement.fields, 3)
        compare(replacement.recurrenceScope, 2)
        dialog.destroy()
    }

    function test_calendarSourceControlsFilterVisibleCalendars() {
        const component = Qt.createComponent("../../qml/CalendarSourceControls.qml")
        compare(component.status, Component.Ready, component.errorString())

        const sourceModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        sourceModel.append({ id: "calendar-product", title: "Product", accessRole: "owner", selected: true })
        sourceModel.append({ id: "calendar-engineering", title: "Engineering", accessRole: "reader", selected: false })
        const controls = component.createObject(null, {
            calendarSourceModel: sourceModel,
            width: 480,
            visible: true
        })
        verify(controls !== null)
        tryCompare(controls.sourceRows, "count", 2)
        verify(controls.implicitHeight > 0)
        verify(controls.isVisible("calendar-product"))
        verify(!controls.isVisible("calendar-engineering"))
        const agenda = Qt.createComponent("../../qml/AgendaView.qml").createObject(null, {
            calendarVisibility: controls
        })
        const dayTimeline = Qt.createComponent("../../qml/DayTimelineView.qml").createObject(null, {
            calendarVisibility: controls
        })
        const weekTimeline = Qt.createComponent("../../qml/WeekTimelineView.qml").createObject(null, {
            calendarVisibility: controls
        })
        const monthGrid = Qt.createComponent("../../qml/MonthGridView.qml").createObject(null, {
            calendarVisibility: controls
        })
        verify(agenda !== null)
        verify(dayTimeline !== null)
        verify(weekTimeline !== null)
        verify(monthGrid !== null)
        verify(agenda.isCalendarVisible("calendar-product"))
        verify(!agenda.isCalendarVisible("calendar-engineering"))
        verify(dayTimeline.isCalendarVisible("calendar-product"))
        verify(!dayTimeline.isCalendarVisible("calendar-engineering"))
        verify(weekTimeline.isCalendarVisible("calendar-product"))
        verify(!weekTimeline.isCalendarVisible("calendar-engineering"))
        compare(monthGrid.eventSummary([{ calendarId: "calendar-engineering", title: "Hidden" },
                                        { calendarId: "calendar-product", title: "Visible" }]),
                "Visible")
        controls.setCalendarVisible("calendar-product", false)
        verify(!controls.isVisible("calendar-product"))
        controls.setCalendarVisible("calendar-engineering", true)
        verify(controls.isVisible("calendar-engineering"))
        verify(!agenda.isCalendarVisible("calendar-product"))
        verify(dayTimeline.isCalendarVisible("calendar-engineering"))
        verify(weekTimeline.isCalendarVisible("calendar-engineering"))
        controls.showAll()
        verify(controls.isVisible("calendar-product"))
        verify(controls.isVisible("calendar-engineering"))
        monthGrid.destroy()
        weekTimeline.destroy()
        dayTimeline.destroy()
        agenda.destroy()
        controls.destroy()
        sourceModel.destroy()
    }

    function test_calendarSidebarControlsExpandWithoutChangingNavigation() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const calendars = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        calendars.append({ id: "calendar-primary", title: "Primary", selected: true })
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            calendarSourceModel: calendars,
            appController: { googleConnected: true, sidebarTabIds: ["tasks", "calendar"] }
        })
        verify(mainWindow !== null)
        tryVerify(function() { return mainWindow.calendarVisibility !== null })
        verify(!mainWindow.navigationSidebar.calendarControlsExpanded)

        const calendarEntry = mainWindow.navigationSidebar.pageButtons.itemAt(1)
        calendarEntry.button.secondaryActivated()
        verify(mainWindow.navigationSidebar.calendarControlsExpanded)
        tryVerify(function() { return mainWindow.calendarVisibility.visible })
        compare(mainWindow.currentPage, "Tasks")

        calendarEntry.button.click()
        compare(mainWindow.currentPage, "Calendar")
        verify(!mainWindow.navigationSidebar.calendarControlsExpanded)
        calendarEntry.button.click()
        verify(mainWindow.navigationSidebar.calendarControlsExpanded)
        mainWindow.destroy()
        calendars.destroy()
    }

    function test_taskListPanePersistsResizedWidth() {
        const component = Qt.createComponent("../../qml/TaskListView.qml")
        compare(component.status, Component.Ready, component.errorString())
        const taskList = component.createObject(testCase, { width: 760, height: 420, visible: true })
        verify(taskList !== null)
        let persistedWidths = []
        taskList.listPaneWidthPersistenceRequested.connect(function(width) { persistedWidths.push(width) })

        taskList.listPaneWidth = 200
        tryCompare(taskList.taskListControls, "width", 200)
        taskList.listPaneWidth = 480
        tryCompare(taskList.taskListControls, "width", 480)
        taskList.listPaneWidth = 260
        tryCompare(taskList.taskListControls, "width", 260)
        taskList.resizeListPane(310)
        tryCompare(taskList, "listPaneWidth", 310)
        tryCompare(taskList.taskListControls, "width", 310)
        wait(250)
        compare(persistedWidths[persistedWidths.length - 1], 310)
        taskList.destroy()
    }

    function test_searchOptionsAreCollapsedUntilRequested() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: { googleConnected: true, searchQuery: "", savedSearches: [] }
        })
        verify(mainWindow !== null)
        mainWindow.openSearch()
        tryVerify(function() { return mainWindow.searchPopup.opened })
        verify(!mainWindow.searchPopup.optionsExpanded)
        mainWindow.searchPopup.optionsToggleButton.click()
        verify(mainWindow.searchPopup.optionsExpanded)
        mainWindow.destroy()
    }

    function test_eventCreateDialogEmitsCreateRequest() {
        const component = Qt.createComponent("../../qml/EventCreateDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const sourceModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        sourceModel.append({ id: "calendar-primary", title: "Primary", accessRole: "owner" })
        const dialog = component.createObject(null, {
            calendarSourceModel: sourceModel,
            width: 480,
            visible: true
        })
        verify(dialog !== null)
        dialog.openForCreate("calendar-primary", "2026-07-26")
        compare(dialog.eventCalendarId, "calendar-primary")
        verify(Number.isFinite(Date.parse(dialog.eventStartAt)))
        dialog.eventTitle = "Release review"
        dialog.eventStartAt = "2026-07-26T10:00:00.000Z"
        dialog.eventEndAt = "2026-07-26T10:00:00.000Z"
        verify(!dialog.primaryEnabled)
        dialog.eventEndAt = "2026-07-26T11:00:00.000Z"
        dialog.eventAllDay = false
        dialog.eventDescription = "Verify the native package"
        dialog.eventLocation = "Studio"
        verify(dialog.primaryEnabled)
        let request = null
        dialog.eventCreateRequested.connect(function(calendarId, title, startAt, endAt, allDay, description, location) {
            request = { calendarId, title, startAt, endAt, allDay, description, location }
        })
        dialog.primaryButton.click()
        compare(request.calendarId, "calendar-primary")
        compare(request.title, "Release review")
        compare(request.startAt, "2026-07-26T10:00:00.000Z")
        compare(request.endAt, "2026-07-26T11:00:00.000Z")
        compare(request.allDay, false)
        compare(request.description, "Verify the native package")
        compare(request.location, "Studio")
        dialog.destroy()
        sourceModel.destroy()
    }

    function test_eventEditDialogEmitsUpdateRequest() {
        const component = Qt.createComponent("../../qml/EventEditDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const sourceModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        sourceModel.append({ id: "calendar-primary", title: "Primary", accessRole: "owner" })
        const dialog = component.createObject(null, { calendarSourceModel: sourceModel })
        verify(dialog !== null)
        dialog.openForEdit("event-1", "calendar-primary", "Release review",
                           "2026-07-26T10:00:00.000Z", "2026-07-26T11:00:00.000Z", false,
                           "Verify the native package", "Studio", "Asia/Singapore", "4",
                           "transparent", "confidential", "[\"guest@example.com\"]",
                           "[{\"method\":\"popup\",\"minutes\":10}]", false)
        verify(dialog.primaryEnabled)
        let request = null
        dialog.eventUpdateRequested.connect(function(eventId, calendarId, title, startAt, endAt, allDay, description, location,
                                                     timeZone, colorId, available, visibility, attendees,
                                                     remindersUseDefault, reminders) {
            request = { eventId, calendarId, title, startAt, endAt, allDay, description, location,
                        timeZone, colorId, available, visibility, attendees, remindersUseDefault, reminders }
        })
        dialog.eventTitle = "Revised release review"
        dialog.eventTimeZone = "UTC"
        verify(dialog.primaryEnabled)
        dialog.primaryButton.click()
        compare(request.eventId, "event-1")
        compare(request.calendarId, "calendar-primary")
        compare(request.title, "Revised release review")
        compare(request.startAt, "2026-07-26T10:00:00.000Z")
        compare(request.endAt, "2026-07-26T11:00:00.000Z")
        compare(request.allDay, false)
        compare(request.description, "Verify the native package")
        compare(request.location, "Studio")
        compare(request.timeZone, "UTC")
        compare(request.colorId, "4")
        compare(request.available, true)
        compare(request.visibility, "confidential")
        compare(request.attendees[0], "guest@example.com")
        compare(request.remindersUseDefault, false)
        compare(request.reminders[0].minutes, 10)
        dialog.destroy()
        sourceModel.destroy()
    }

    function test_statusEventPropertiesEditorProvidesFormsAndRoundTripsAdvancedValues() {
        const component = Qt.createComponent("../../qml/StatusEventPropertiesEditor.qml")
        compare(component.status, Component.Ready, component.errorString())

        const editor = component.createObject(null, { eventType: "focusTime" })
        verify(editor !== null)
        let properties = JSON.parse(editor.propertiesJson)
        compare(properties.focusTimeProperties.autoDeclineMode, "declineNone")
        compare(properties.focusTimeProperties.chatStatus, "available")
        verify(editor.validProperties())

        editor.eventType = "outOfOffice"
        properties = JSON.parse(editor.propertiesJson)
        compare(properties.outOfOfficeProperties.autoDeclineMode, "declineNone")
        verify(editor.validProperties())

        editor.eventType = "workingLocation"
        properties = JSON.parse(editor.propertiesJson)
        compare(properties.workingLocationProperties.type, "homeOffice")
        verify(properties.workingLocationProperties.homeOffice !== undefined)
        editor.load('{"workingLocationProperties":{"type":"officeLocation","officeLocation":{"buildingId":"HQ","floorId":"8","deskId":"8-12","label":"HQ"}}}')
        compare(JSON.parse(editor.propertiesJson).workingLocationProperties.officeLocation.deskId, "8-12")
        verify(editor.validProperties())
        editor.destroy()
    }

    function test_eventDeleteDialogEmitsDeleteRequest() {
        const component = Qt.createComponent("../../qml/EventDeleteDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const dialog = component.createObject(null)
        verify(dialog !== null)
        dialog.openForDelete("event-1", "Release review")
        verify(dialog.primaryEnabled)
        let deletedId = ""
        dialog.eventDeleteRequested.connect(function(eventId) { deletedId = eventId })
        dialog.primaryButton.click()
        compare(deletedId, "event-1")
        dialog.destroy()
    }

    function test_taskDeleteDialogEmitsDeleteRequest() {
        const component = Qt.createComponent("../../qml/TaskDeleteDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const dialog = component.createObject(null)
        verify(dialog !== null)
        dialog.openForDelete("task-1", "Plan release")
        verify(dialog.primaryEnabled)
        let deletedId = ""
        dialog.taskDeleteRequested.connect(function(taskId) { deletedId = taskId })
        dialog.primaryButton.click()
        compare(deletedId, "task-1")
        dialog.destroy()
    }

    function test_taskListControlsExposeListActionsAndStates() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-inbox", title: "Inbox", selected: true, taskCount: 2,
                           taskTitles: ["Plan release", "Review sync"] })
        taskLists.append({ id: "list-archive", title: "Archive", selected: false, taskCount: 0,
                           taskTitles: [] })
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            taskListModel: taskLists
        })
        verify(mainWindow !== null)
        const controls = mainWindow.taskList.taskListControls
        verify(controls !== null)
        tryCompare(controls.taskListRows, "count", 2)
        let created = 0
        let renamed = null
        let deleted = null
        let selection = null
        mainWindow.taskList.taskListCreateRequested.connect(function() { created += 1 })
        mainWindow.taskList.taskListRenameRequested.connect(function(taskListId, title) {
            renamed = { taskListId, title }
        })
        mainWindow.taskList.taskListDeleteRequested.connect(function(taskListId, title, taskCount, taskTitles) {
            deleted = { taskListId, title, taskCount, taskTitles }
        })
        mainWindow.taskList.taskListSelectionRequested.connect(function(taskListId, selected) {
            selection = { taskListId, selected }
        })
        controls.newTaskListButton.click()
        compare(created, 1)
        tryVerify(function() {
            return controls.taskListRows.itemAtIndex(0) !== null
        })
        const inbox = controls.taskListRows.itemAtIndex(0)
        inbox.selectionCheck.click()
        compare(selection.taskListId, "list-inbox")
        compare(selection.selected, false)
        inbox.renameButton.click()
        compare(renamed.taskListId, "list-inbox")
        compare(renamed.title, "Inbox")
        inbox.deleteButton.click()
        compare(deleted.taskListId, "list-inbox")
        compare(deleted.taskCount, 2)
        compare(deleted.taskTitles.count, 2)
        mainWindow.destroy()
        taskLists.destroy()

        const controlsComponent = Qt.createComponent("../../qml/TaskListControls.qml")
        compare(controlsComponent.status, Component.Ready, controlsComponent.errorString())
        const loading = controlsComponent.createObject(null, { loading: true, visible: true })
        verify(loading !== null)
        verify(loading.loadingLabel.visible)
        loading.destroy()
        const errored = controlsComponent.createObject(null, {
            errorMessage: "Could not load Google Task lists", visible: true
        })
        verify(errored !== null)
        verify(errored.errorLabel.visible)
        errored.destroy()
    }

    function test_keyboardTaskListCreationOpensEditor() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-inbox", title: "Inbox", selected: true, taskCount: 0,
                           taskTitles: [] })
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            taskListModel: taskLists
        })
        verify(mainWindow !== null)
        mainWindow.requestActivate()
        tryCompare(mainWindow, "active", true)
        mainWindow.taskList.taskListControls.newTaskListButton.forceActiveFocus()
        tryVerify(function() {
            return mainWindow.taskList.taskListControls.newTaskListButton.activeFocus
        })
        keyClick(Qt.Key_Space)
        tryVerify(function() {
            return mainWindow.taskListEditorDialog.opened &&
                    mainWindow.taskListEditorDialog.taskListTitleField.activeFocus
        })
        mainWindow.destroy()
        taskLists.destroy()
    }

    function test_bulkTaskSelectionKeyboardAndActions() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const taskModel = Qt.createQmlObject(
            'import QtQml.Models; ListModel { function taskIds() { return ["task-a", "task-b"] } '
            + 'function topLevelTasks() { return [{ id: "task-parent", title: "Parent" }] } }', testCase)
        const calls = []
        const controller = {
            googleConnected: true,
            busy: false,
            bulkSetTaskCompleted: function(taskIds, completed) {
                calls.push({ action: "complete", taskIds: taskIds, completed: completed })
            },
            bulkDeleteTasks: function(taskIds) {
                calls.push({ action: "delete", taskIds: taskIds })
            },
            bulkSetTaskDue: function(taskIds, dueAt) {
                calls.push({ action: "due", taskIds: taskIds, dueAt: dueAt })
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            taskModel: taskModel,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.requestActivate()
        tryCompare(mainWindow, "active", true)
        mainWindow.taskList.bulkSelectAllButton.forceActiveFocus()
        keyClick(Qt.Key_A, Qt.ControlModifier)
        tryCompare(mainWindow.taskList.selectedTaskIds, "length", 2)
        compare(mainWindow.taskList.bulkSelectionStatus.text, "2 selected · eligibility checked before queueing")
        compare(mainWindow.taskList.bulkCompleteButton.Accessible.name, "Complete 2 tasks")
        mainWindow.taskList.bulkCompleteButton.click()
        compare(calls.length, 1)
        compare(calls[0].action, "complete")
        compare(calls[0].taskIds.length, 2)
        compare(calls[0].completed, true)
        mainWindow.taskList.bulkDeleteButton.click()
        verify(mainWindow.taskList.bulkDeleteDialog.opened)
        verify(mainWindow.taskList.bulkDeleteDialog.primaryEnabled)
        mainWindow.taskList.bulkDeleteDialog.primaryButton.click()
        compare(calls.length, 2)
        compare(calls[1].action, "delete")
        mainWindow.taskList.bulkEditDialog.openForDue(mainWindow.taskList.selectedTaskIds)
        mainWindow.taskList.bulkEditDialog.dueField.text = "2026-08-01"
        mainWindow.taskList.bulkEditDialog.primaryButton.click()
        compare(calls.length, 3)
        compare(calls[2].action, "due")
        compare(calls[2].dueAt, "2026-08-01")
        mainWindow.destroy()
        taskModel.destroy()
    }

    function test_taskListDialogsNameAffectedTasksAndEmitRequests() {
        const editorComponent = Qt.createComponent("../../qml/TaskListEditorDialog.qml")
        compare(editorComponent.status, Component.Ready, editorComponent.errorString())
        const editor = editorComponent.createObject(null)
        verify(editor !== null)
        let saved = null
        editor.taskListSaveRequested.connect(function(taskListId, title) {
            saved = { taskListId, title }
        })
        editor.openForCreate()
        editor.taskListTitleField.text = "Work"
        editor.primaryButton.click()
        compare(saved.taskListId, "")
        compare(saved.title, "Work")
        editor.openForRename("list-work", "Work")
        editor.taskListTitleField.text = "Projects"
        editor.primaryButton.click()
        compare(saved.taskListId, "list-work")
        compare(saved.title, "Projects")
        editor.destroy()

        const deleteComponent = Qt.createComponent("../../qml/TaskListDeleteDialog.qml")
        compare(deleteComponent.status, Component.Ready, deleteComponent.errorString())
        const deleteDialog = deleteComponent.createObject(null)
        verify(deleteDialog !== null)
        deleteDialog.openForDelete("list-work", "Projects", 3,
                                    ["Plan release", "Review sync"])
        tryCompare(deleteDialog.taskRows, "count", 2)
        compare(deleteDialog.taskTitles[0], "Plan release")
        compare(deleteDialog.taskTitles[1], "Review sync")
        let deletedId = ""
        deleteDialog.taskListDeleteRequested.connect(function(taskListId) { deletedId = taskListId })
        deleteDialog.primaryButton.click()
        compare(deletedId, "list-work")
        deleteDialog.destroy()
    }

    function test_taskCreateDialogEmitsCreateRequest() {
        const component = Qt.createComponent("../../qml/TaskCreateDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-active", title: "Inbox" })
        taskLists.append({ id: "list-other", title: "Archive" })
        const dialog = component.createObject(null, { taskListModel: taskLists })
        verify(dialog !== null)
        dialog.openForCreate("list-other")
        compare(dialog.taskListId, "list-other")
        dialog.taskTitle = "Plan release"
        verify(dialog.primaryEnabled)
        let request = null
        dialog.taskCreateRequested.connect(function(taskListId, parentTaskId, title, notes, dueAt,
                                                    dueTimeZone, priority, managedRecurrence,
                                                    recurrenceFrequency, recurrenceInterval,
                                                    recurrenceEndKind, recurrenceEndUntil,
                                                    recurrenceEndCount) {
            request = { taskListId, parentTaskId, title, notes, dueAt, dueTimeZone, priority,
                        managedRecurrence, recurrenceFrequency, recurrenceInterval,
                        recurrenceEndKind, recurrenceEndUntil, recurrenceEndCount }
        })
        dialog.primaryButton.click()
        compare(request.taskListId, "list-other")
        compare(request.parentTaskId, "")
        compare(request.title, "Plan release")
        compare(request.managedRecurrence, false)
        dialog.destroy()
        taskLists.destroy()
    }

    function test_taskCreateDialogEmitsManagedRecurrence() {
        const component = Qt.createComponent("../../qml/TaskCreateDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-active", title: "Inbox" })
        const dialog = component.createObject(null, { taskListModel: taskLists })
        verify(dialog !== null)
        dialog.openForCreate("list-active", "")
        dialog.taskTitle = "Review invoices"
        dialog.taskDueAt = "2026-07-26"
        dialog.managedRecurrenceCheck.checked = true
        dialog.recurrenceFrequencyPicker.currentIndex = 2
        dialog.recurrenceIntervalField.text = "3"
        dialog.recurrenceEndPicker.currentIndex = 2
        dialog.recurrenceEndCountField.text = "4"
        verify(dialog.primaryEnabled)
        let request = null
        dialog.taskCreateRequested.connect(function(taskListId, parentTaskId, title, notes, dueAt,
                                                    dueTimeZone, priority, managedRecurrence,
                                                    recurrenceFrequency, recurrenceInterval,
                                                    recurrenceEndKind, recurrenceEndUntil,
                                                    recurrenceEndCount) {
            request = { taskListId, parentTaskId, title, notes, dueAt, dueTimeZone, priority,
                        managedRecurrence, recurrenceFrequency, recurrenceInterval,
                        recurrenceEndKind, recurrenceEndUntil, recurrenceEndCount }
        })
        dialog.primaryButton.click()
        compare(request.managedRecurrence, true)
        compare(request.recurrenceFrequency, 2)
        compare(request.recurrenceInterval, 3)
        compare(request.recurrenceEndKind, 2)
        compare(request.recurrenceEndCount, 4)
        dialog.destroy()
        taskLists.destroy()
    }

    function test_taskEditDialogEmitsUpdateRequest() {
        const component = Qt.createComponent("../../qml/TaskEditDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const dialog = component.createObject(null)
        verify(dialog !== null)
        dialog.openForEdit("task-1", "Plan release", "Prepare checklist", "2026-07-26",
                           "Asia/Singapore", 2)
        compare(dialog.taskPriority, 2)
        dialog.taskTitle = "Revised release plan"
        dialog.taskNotes = "Confirm rollout"
        dialog.taskDueAt = "2026-07-27"
        dialog.taskPriorityPicker.currentIndex = 3
        verify(dialog.primaryEnabled)
        let request = null
        dialog.taskUpdateRequested.connect(function(taskId, title, notes, dueAt, dueTimeZone, priority,
                                                    managedRecurrence, recurrenceFrequency,
                                                    recurrenceInterval, recurrenceEndKind,
                                                    recurrenceEndUntil, recurrenceEndCount) {
            request = { taskId, title, notes, dueAt, dueTimeZone, priority, managedRecurrence,
                        recurrenceFrequency, recurrenceInterval, recurrenceEndKind,
                        recurrenceEndUntil, recurrenceEndCount }
        })
        dialog.primaryButton.click()
        compare(request.taskId, "task-1")
        compare(request.title, "Revised release plan")
        compare(request.notes, "Confirm rollout")
        compare(request.dueAt, "2026-07-27")
        compare(request.dueTimeZone, "Asia/Singapore")
        compare(request.priority, 3)
        dialog.openForEdit("task-1", "Plan release", "", "2026-07-26", "Asia/Singapore", 0)
        dialog.taskDueAt = ""
        let clearedDueRequest = null
        dialog.taskUpdateRequested.connect(function(taskId, title, notes, dueAt, dueTimeZone, priority) {
            clearedDueRequest = { taskId, title, notes, dueAt, dueTimeZone, priority }
        })
        dialog.primaryButton.click()
        compare(clearedDueRequest.dueAt, "")
        compare(clearedDueRequest.dueTimeZone, "")
        dialog.destroy()
    }

    function test_taskEditDialogExposesManagedRecurrenceActions() {
        const component = Qt.createComponent("../../qml/TaskEditDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const dialog = component.createObject(null)
        verify(dialog !== null)
        dialog.openForEdit("task-1", "Review invoices", "", "2026-07-26", "UTC", 0,
                           true, "Every 2 weeks", 1, 2, 1, "2026-08-30", 0)
        verify(dialog.primaryEnabled)
        let request = null
        dialog.taskUpdateRequested.connect(function(taskId, title, notes, dueAt, dueTimeZone, priority,
                                                    managedRecurrence, recurrenceFrequency,
                                                    recurrenceInterval, recurrenceEndKind,
                                                    recurrenceEndUntil, recurrenceEndCount) {
            request = { taskId, title, notes, dueAt, dueTimeZone, priority, managedRecurrence,
                        recurrenceFrequency, recurrenceInterval, recurrenceEndKind,
                        recurrenceEndUntil, recurrenceEndCount }
        })
        dialog.primaryButton.click()
        compare(request.managedRecurrence, true)
        compare(request.recurrenceFrequency, 1)
        compare(request.recurrenceInterval, 2)
        compare(request.recurrenceEndKind, 1)
        compare(request.recurrenceEndUntil, "2026-08-30")
        dialog.destroy()
    }

    function test_taskRecurrenceActionDialogEmitsScopedAction() {
        const component = Qt.createComponent("../../qml/TaskRecurrenceActionDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const dialog = component.createObject(null)
        verify(dialog !== null)
        dialog.openForAction("task-1", "Review invoices", 0)
        let action = null
        dialog.recurrenceActionRequested.connect(function(taskId, requestedAction, scope) {
            action = { taskId, requestedAction, scope }
        })
        dialog.primaryButton.click()
        compare(action.taskId, "task-1")
        compare(action.requestedAction, 0)
        compare(action.scope, 0)
        dialog.destroy()
    }

    function test_taskMoveDialogShowsAllActiveListsAndEmitsMoveRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-active", title: "Inbox", selected: true, taskCount: 0,
                           taskTitles: [] })
        taskLists.append({ id: "list-other", title: "Archive", selected: false, taskCount: 0,
                           taskTitles: [] })
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            taskListModel: taskLists,
        })
        verify(mainWindow !== null)
        const dialog = mainWindow.taskMoveDialog
        dialog.openForMove("task-1", "Plan release", "list-active")
        tryCompare(dialog.taskListRows, "count", 2)
        tryVerify(function() {
            return dialog.taskListRows.itemAtIndex(0) !== null &&
                    dialog.taskListRows.itemAtIndex(1) !== null
        })
        verify(!dialog.taskListRows.itemAtIndex(0).enabled)
        verify(dialog.taskListRows.itemAtIndex(1).enabled)
        dialog.taskListRows.itemAtIndex(1).click()
        verify(dialog.primaryEnabled)
        let move = null
        dialog.taskMoveRequested.connect(function(taskId, taskListId) {
            move = { taskId, taskListId }
        })
        dialog.primaryButton.click()
        compare(move.taskId, "task-1")
        compare(move.taskListId, "list-other")
        mainWindow.destroy()
        taskLists.destroy()
    }

    function test_dayTimelinePresentsAndSelectsEvents() {
        const component = Qt.createComponent("../../qml/DayTimelineView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const timelineModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        timelineModel.append({
            id: "event-1",
            calendarId: "calendar-1",
            title: "Release review",
            description: "",
            location: "",
            startAt: "2026-07-26T09:00:00.000Z",
            startTimeZone: "UTC",
            endAt: "2026-07-26T10:00:00.000Z",
            transparency: "opaque",
            visibility: "default",
            colorId: "",
            attendeeEmailsJson: "[]",
            remindersJson: "[]",
            remindersUseDefault: true,
            recurrenceRule: "",
            recurringRemoteId: "",
            originalStartAt: "",
            allDay: false,
            dayIndex: 0,
            startMinute: 540,
            durationMinutes: 60,
            laneIndex: 0,
            laneCount: 2
        })
        timelineModel.append({
            id: "event-2",
            calendarId: "calendar-1",
            title: "Team offsite",
            description: "",
            location: "",
            startAt: "2026-07-26T00:00:00.000Z",
            startTimeZone: "UTC",
            endAt: "2026-07-27T00:00:00.000Z",
            transparency: "opaque",
            visibility: "default",
            colorId: "",
            attendeeEmailsJson: "[]",
            remindersJson: "[]",
            remindersUseDefault: true,
            recurrenceRule: "",
            recurringRemoteId: "",
            originalStartAt: "",
            allDay: true,
            dayIndex: 0,
            startMinute: 0,
            durationMinutes: 0,
            laneIndex: 0,
            laneCount: 1
        })
        const timeline = component.createObject(null, {
            timelineModel: timelineModel,
            width: 480,
            height: 640,
            visible: true
        })
        verify(timeline !== null)
        tryCompare(timeline.eventRows, "count", 2)
        compare(timeline.timePosition(540), 576)
        compare(timeline.eventHeight(60), 64)
        compare(timeline.eventHeight(1), 24)
        let selectedId = ""
        timeline.eventSelected.connect(function(eventId) { selectedId = eventId })
        timeline.selectEvent("event-1")
        compare(selectedId, "event-1")
        timeline.destroy()
        timelineModel.destroy()
    }

    function test_dayTimelineRequestsMoves() {
        const component = Qt.createComponent("../../qml/DayTimelineView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const timelineModel = Qt.createQmlObject('import QtQml.Models; ListModel { function moveInput(eventId, dayIndex, minute) { return { id: eventId, startAt: "2026-07-26T10:00:00.000Z", endAt: "2026-07-26T11:00:00.000Z", allDay: false } } function timelinePointInput(x, y, width, column, hourHeight, endPoint) { return { minute: y > 5000 ? (endPoint ? 1440 : 1425) : 0 } } }', testCase)
        const timeline = component.createObject(null, { timelineModel: timelineModel })
        verify(timeline !== null)
        let request = null
        timeline.eventMoveRequested.connect(function(eventId, startAt, endAt, allDay) {
            request = { eventId, startAt, endAt, allDay }
        })
        timeline.requestMove("event-1", 0, 600)
        compare(request.eventId, "event-1")
        compare(request.startAt, "2026-07-26T10:00:00.000Z")
        compare(request.endAt, "2026-07-26T11:00:00.000Z")
        compare(request.allDay, false)
        compare(timeline.dropMinute(-1), 0)
        compare(timeline.dropMinute(99999), 1425)
        timeline.destroy()
        timelineModel.destroy()
    }

    function test_dayTimelineRequestsResizes() {
        const component = Qt.createComponent("../../qml/DayTimelineView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const timelineModel = Qt.createQmlObject('import QtQml.Models; ListModel { function resizeInput(eventId, dayIndex, minute) { return { id: eventId, endAt: "2026-07-26T12:00:00.000Z" } } function timelinePointInput(x, y, width, column, hourHeight, endPoint) { return { minute: y > 5000 ? (endPoint ? 1440 : 1425) : 15 } } }', testCase)
        const timeline = component.createObject(null, { timelineModel: timelineModel })
        verify(timeline !== null)
        let request = null
        timeline.eventResizeRequested.connect(function(eventId, endAt) {
            request = { eventId, endAt }
        })
        timeline.requestResize("event-1", 0, 720)
        compare(request.eventId, "event-1")
        compare(request.endAt, "2026-07-26T12:00:00.000Z")
        compare(timeline.dropEndMinute(-1), 15)
        compare(timeline.dropEndMinute(99999), 1440)
        timeline.destroy()
        timelineModel.destroy()
    }

    function test_calendarQuickCreateDefaultsToEvent() {
        const component = Qt.createComponent("../../qml/CalendarQuickCreateDialog.qml")
        compare(component.status, Component.Ready, component.errorString())
        const dialog = component.createObject(testCase, {
            calendarId: "calendar-1",
            startAt: "2026-08-05T09:00:00.000Z",
            endAt: "2026-08-05T10:00:00.000Z"
        })
        verify(dialog !== null)
        compare(dialog.createKind, 0)
        compare(dialog.dateOnly(dialog.startAt), "2026-08-05")
        dialog.destroy()
    }

    function test_recurringMoveScopeOffersInstanceChoices() {
        const component = Qt.createComponent("../../qml/EventMutationScopeDialog.qml")
        compare(component.status, Component.Ready, component.errorString())
        const dialog = component.createObject(testCase, {
            event: { id: "event-1", title: "Standup", recurringRemoteId: "series-1" }
        })
        verify(dialog !== null)
        const options = dialog.scopeOptions()
        compare(options.length, 3)
        compare(options[0].value, 0)
        compare(options[2].value, 2)
        dialog.destroy()
    }

    function test_weekTimelinePresentsAndSelectsEvents() {
        const component = Qt.createComponent("../../qml/WeekTimelineView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const timelineModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        timelineModel.append({
            id: "event-1",
            calendarId: "calendar-1",
            title: "Release review",
            description: "",
            location: "",
            startAt: "2026-07-29T09:00:00.000Z",
            startTimeZone: "UTC",
            endAt: "2026-07-29T10:00:00.000Z",
            transparency: "opaque",
            visibility: "default",
            colorId: "",
            attendeeEmailsJson: "[]",
            remindersJson: "[]",
            remindersUseDefault: true,
            recurrenceRule: "",
            recurringRemoteId: "",
            originalStartAt: "",
            allDay: false,
            dayIndex: 3,
            startMinute: 540,
            durationMinutes: 60,
            laneIndex: 0,
            laneCount: 2,
            daySpan: 1
        })
        timelineModel.append({
            id: "event-2",
            calendarId: "calendar-1",
            title: "Team offsite",
            description: "",
            location: "",
            startAt: "2026-07-27T00:00:00.000Z",
            startTimeZone: "UTC",
            endAt: "2026-07-29T00:00:00.000Z",
            transparency: "opaque",
            visibility: "default",
            colorId: "",
            attendeeEmailsJson: "[]",
            remindersJson: "[]",
            remindersUseDefault: true,
            recurrenceRule: "",
            recurringRemoteId: "",
            originalStartAt: "",
            allDay: true,
            dayIndex: 1,
            startMinute: 0,
            durationMinutes: 0,
            laneIndex: 0,
            laneCount: 1,
            daySpan: 2,
            startsBeforeRange: false,
            endsAfterRange: false
        })
        const timeline = component.createObject(null, {
            timelineModel: timelineModel,
            width: 764,
            height: 640,
            visible: true
        })
        verify(timeline !== null)
        tryCompare(timeline.eventRows, "count", 2)
        compare(timeline.dayColumnWidth(764), 100)
        compare(timeline.dayPosition(3, 764), 364)
        compare(timeline.timePosition(540), 432)
        let selectedId = ""
        timeline.eventSelected.connect(function(eventId) { selectedId = eventId })
        timeline.selectEvent("event-1")
        compare(selectedId, "event-1")
        timeline.destroy()
        timelineModel.destroy()
    }

    function test_weekTimelineRequestsMoves() {
        const component = Qt.createComponent("../../qml/WeekTimelineView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const timelineModel = Qt.createQmlObject('import QtQml.Models; ListModel { function moveInput(eventId, dayIndex, minute) { return { id: eventId, startAt: "2026-07-27T10:00:00.000Z", endAt: "2026-07-27T11:00:00.000Z", allDay: false } } function moveAllDayInput(eventId, dayIndex) { return { id: eventId, startAt: "2026-07-28T00:00:00.000Z", endAt: "2026-07-30T00:00:00.000Z", allDay: true } } function resizeInput(eventId, dayIndex, minute) { return { id: eventId, endAt: "2026-07-27T12:00:00.000Z" } } function resizeAllDayRangeInput(eventId, startDayIndex, endDayIndex) { return { id: eventId, startAt: "2026-07-28T00:00:00.000Z", endAt: "2026-07-31T00:00:00.000Z", allDay: true } } function timedRangeInput(firstDay, firstMinute, lastDay, lastMinute) { return { startAt: "2026-07-27T10:00:00.000Z", endAt: "2026-07-27T11:00:00.000Z", allDay: false } } function allDayRangeInput(firstDay, lastDay) { return { startAt: "2026-07-27T00:00:00.000Z", endAt: "2026-07-29T00:00:00.000Z", allDay: true } } function timelinePointInput(x, y, width, column, hourHeight, endPoint) { return { dayIndex: x < 0 ? 0 : x > 800 ? 6 : 0, minute: y > 5000 ? (endPoint ? 1440 : 1425) : 0 } } function dateForDayIndex(dayIndex) { return "2026-07-" + (27 + dayIndex) } function dayIndexForDate(date) { return 0 } }', testCase)
        const timeline = component.createObject(null, { timelineModel: timelineModel, width: 764 })
        verify(timeline !== null)
        let request = null
        timeline.eventMoveRequested.connect(function(eventId, startAt, endAt, allDay) {
            request = { eventId, startAt, endAt, allDay }
        })
        timeline.requestMove("event-1", 1, 600)
        compare(request.eventId, "event-1")
        compare(request.startAt, "2026-07-27T10:00:00.000Z")
        compare(request.endAt, "2026-07-27T11:00:00.000Z")
        compare(request.allDay, false)
        timeline.requestAllDayMove("event-2", 2)
        compare(request.eventId, "event-2")
        compare(request.allDay, true)
        let resize = null
        timeline.eventResizeRequested.connect(function(eventId, endAt) {
            resize = { eventId, endAt }
        })
        timeline.requestResize("event-1", 1, 720)
        compare(resize.eventId, "event-1")
        compare(resize.endAt, "2026-07-27T12:00:00.000Z")
        timeline.requestAllDayResize("event-2", 1, 4)
        compare(request.eventId, "event-2")
        compare(request.startAt, "2026-07-28T00:00:00.000Z")
        compare(request.endAt, "2026-07-31T00:00:00.000Z")
        compare(request.allDay, true)
        compare(timeline.dropDayIndex(-1, 764), 0)
        compare(timeline.dropDayIndex(99999, 764), 6)
        compare(timeline.dropMinute(99999), 1425)
        compare(timeline.dropEndMinute(99999), 1440)
        timeline.destroy()
        timelineModel.destroy()
    }

    function test_weekTimelineCreatesNormalizedMultiDayTimedRanges() {
        const component = Qt.createComponent("../../qml/WeekTimelineView.qml")
        compare(component.status, Component.Ready, component.errorString())
        const timelineModel = Qt.createQmlObject('import QtQml; QtObject { function timedRangeInput(firstDay, firstMinute, lastDay, lastMinute) { return { startAt: firstDay + ":" + firstMinute, endAt: lastDay + ":" + lastMinute, allDay: false } } }', testCase)
        const timeline = component.createObject(null, { timelineModel: timelineModel, width: 764 })
        verify(timeline !== null)
        let request = null
        timeline.quickCreateRequested.connect(function(startAt, endAt, allDay) {
            request = { startAt: startAt, endAt: endAt, allDay: allDay }
        })
        timeline.quickCreateTimed(3, 12 * 60, 1, 10 * 60)
        compare(request.startAt, "1:600")
        compare(request.endAt, "3:720")
        verify(!request.allDay)

        timeline.showTimedPreview("New event", 3, 12 * 60, 1, 10 * 60)
        const segments = timeline.timedPreviewSegments()
        compare(segments.length, 3)
        compare(segments[0].dayIndex, 1)
        compare(segments[0].startMinute, 10 * 60)
        compare(segments[0].endMinute, 24 * 60)
        compare(segments[2].dayIndex, 3)
        compare(segments[2].startMinute, 0)
        compare(segments[2].endMinute, 12 * 60)
        timeline.destroy()
        timelineModel.destroy()
    }

    function test_monthGridFormatsCellsAndSelection() {
        const component = Qt.createComponent("../../qml/MonthGridView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const monthGrid = component.createObject(null, {
            width: 700,
            height: 480,
            visible: true
        })
        verify(monthGrid !== null)
        compare(monthGrid.eventSummary([]), "")
        compare(monthGrid.eventSummary([{ title: "Release review" }]), "Release review")
        compare(monthGrid.eventSummary([{ title: "Release review" }, { title: "Offsite" }]),
                "Release review +1")
        let selectedDate = ""
        monthGrid.dateSelected.connect(function(date) { selectedDate = date })
        monthGrid.selectDate("2026-07-26")
        compare(selectedDate, "2026-07-26")
        let createdDate = ""
        monthGrid.eventCreateRequested.connect(function(date) { createdDate = date })
        monthGrid.eventCreateRequested("2026-07-27")
        compare(createdDate, "2026-07-27")
        let edited = null
        monthGrid.eventEditRequested.connect(function(event) { edited = event })
        monthGrid.eventEditRequested({ id: "event-1", title: "Release review" })
        compare(edited.id, "event-1")
        monthGrid.destroy()
    }

    function test_monthGridCreatesRangesAndMovesMultiDayEvents() {
        const component = Qt.createComponent("../../qml/MonthGridView.qml")
        compare(component.status, Component.Ready, component.errorString())
        const model = Qt.createQmlObject('import QtQml.Models; ListModel { function dateIndex(date) { const days = { "2026-08-05": 10, "2026-08-06": 11, "2026-08-07": 12, "2026-08-09": 14, "2026-08-10": 15 }; return days[date] === undefined ? -1 : days[date] } function dateForIndex(index) { const days = { 10: "2026-08-05", 11: "2026-08-06", 12: "2026-08-07", 14: "2026-08-09", 15: "2026-08-10" }; return days[index] || "" } function allDayRangeInput(first, last) { return { startAt: first === 10 ? "2026-08-05T00:00:00.000Z" : "2026-08-06T00:00:00.000Z", endAt: last === 12 ? "2026-08-08T00:00:00.000Z" : "2026-08-10T00:00:00.000Z", allDay: true } } function moveInput(event, target) { return { id: event.id, startAt: "2026-08-07T00:00:00.000Z", endAt: "2026-08-10T00:00:00.000Z", allDay: true } } function resizeAllDayRangeInput(event, first, last) { return { id: event.id, startAt: "2026-08-06T00:00:00.000Z", endAt: "2026-08-10T00:00:00.000Z", allDay: true } } }', testCase)
        const monthGrid = component.createObject(null, {
            width: 700,
            height: 480,
            calendarDate: "2026-08-05",
            monthGridModel: model
        })
        verify(monthGrid !== null)
        let create = null
        monthGrid.quickCreateRequested.connect(function(startAt, endAt, allDay) {
            create = { startAt, endAt, allDay }
        })
        monthGrid.quickCreateForRange("2026-08-05", "2026-08-07")
        compare(create.startAt, "2026-08-05T00:00:00.000Z")
        compare(create.endAt, "2026-08-08T00:00:00.000Z")
        compare(create.allDay, true)
        let move = null
        monthGrid.eventMoveScopeRequested.connect(function(event, startAt, endAt, allDay) {
            move = { event, startAt, endAt, allDay }
        })
        monthGrid.requestEventMove({ id: "event-1", title: "Trip", allDay: true,
                                     startAt: "2026-08-05T00:00:00.000Z",
                                     endAt: "2026-08-08T00:00:00.000Z" }, "2026-08-07")
        compare(move.startAt, "2026-08-07T00:00:00.000Z")
        compare(move.endAt, "2026-08-10T00:00:00.000Z")
        compare(move.allDay, true)
        let resize = null
        monthGrid.eventAllDayResizeScopeRequested.connect(function(event, startAt, endAt) {
            resize = { event, startAt, endAt }
        })
        monthGrid.requestAllDayResize({ id: "event-1", allDay: true }, "2026-08-06", "2026-08-09")
        compare(resize.event.id, "event-1")
        compare(resize.startAt, "2026-08-06T00:00:00.000Z")
        compare(resize.endAt, "2026-08-10T00:00:00.000Z")
        const scheduledTaskIndex = Qt.createQmlObject('import QtQml; QtObject { property int revision: 1; function tasksForDate(date) { return date === "2026-08-05" ? [{ id: "task-1", dueAt: date, title: "First" }, { id: "task-2", dueAt: date, title: "Second" }] : [] } }', testCase)
        monthGrid.scheduledTaskIndex = scheduledTaskIndex
        const hidden = monthGrid.moreCount(0, [
            { id: "all-day", allDay: true, calendarId: "calendar-1", title: "All day" },
            { id: "timed-1", allDay: false, calendarId: "calendar-1", title: "First" },
            { id: "timed-2", allDay: false, calendarId: "calendar-1", title: "Second" },
            { id: "timed-3", allDay: false, calendarId: "calendar-1", title: "Third" }
        ], "2026-08-05")
        compare(hidden, 3)
        monthGrid.showPreview("New event", "2026-08-07", "2026-08-10")
        compare(monthGrid.previewSegments().length, 2)
        monthGrid.clearPreview()
        monthGrid.destroy()
        scheduledTaskIndex.destroy()
        model.destroy()
    }

    function test_mainNavigatesCalendarByCurrentView() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const dates = []
        const controller = {
            googleConnected: true,
            calendarDate: "2026-07-26",
            setCalendarDate: function(date) { dates.push(date) }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller
        })
        verify(mainWindow !== null)
        compare(mainWindow.calendarWeekDayIndex(), 0)
        compare(mainWindow.calendarWeekLabels()[0], "Sun 07-26")
        mainWindow.calendarViews.currentIndex = 1
        mainWindow.navigateCalendar(1)
        compare(dates[0], "2026-07-27")
        mainWindow.calendarViews.currentIndex = 2
        mainWindow.navigateCalendar(1)
        compare(dates[1], "2026-08-02")
        mainWindow.calendarViews.currentIndex = 3
        mainWindow.navigateCalendar(1)
        compare(dates[2], "2026-08-26")
        mainWindow.destroy()
    }

    function test_taskListVirtualizesRows() {
        const component = Qt.createComponent("../../qml/TaskListView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        for (let index = 0; index < 1000; ++index) {
            taskModel.append({
                id: "task-" + index,
                taskListId: "list-active",
                taskListTitle: "Active",
                title: "Task " + index,
                notes: "",
                dueAt: "",
                dueTimeZone: "",
                priority: 0,
                completed: false,
                managedRecurrence: false,
                recurrenceSummary: "",
                recurrenceFrequency: -1,
                recurrenceInterval: 1,
                recurrenceEndKind: 0,
                recurrenceEndUntil: "",
                recurrenceEndCount: 0
            })
        }
        const taskList = component.createObject(testCase, {
            taskModel: taskModel,
            width: 480,
            height: 320
        })
        verify(taskList !== null)
        tryCompare(taskList.taskRows, "rows", 1000)
        verify(taskList.taskRows.reuseItems)
        tryVerify(function() {
            return taskList.taskRows.itemAtIndex(taskList.taskRows.index(0, 0)) !== null
        })
        verify(taskList.taskRows.contentItem.children.length < taskList.taskRows.rows)
        taskList.taskRows.contentY = taskList.taskRows.contentHeight - taskList.taskRows.height
        tryVerify(function() {
            return taskList.taskRows.itemAtIndex(taskList.taskRows.index(999, 0)) !== null
        })
        verify(taskList.taskRows.contentItem.children.length < taskList.taskRows.rows)
        taskList.destroy()
        taskModel.destroy()
    }

    function test_notesListUsesUnderlyingTaskActions() {
        const component = Qt.createComponent("../../qml/NotesListView.qml")
        compare(component.status, Component.Ready, component.errorString())

        const notesModel = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        notesModel.append({
            id: "note-1",
            taskListId: "list-inbox",
            taskListTitle: "Inbox",
            title: "Release notes",
            body: "Verify the package artifacts",
            completed: false
        })
        notesModel.append({
            id: "note-2",
            taskListId: "list-work",
            taskListTitle: "Work",
            title: "Sprint plan",
            body: "Review the current priorities",
            completed: true
        })
        const notesList = component.createObject(null, {
            notesModel: notesModel,
            width: 480,
            height: 320,
            visible: true
        })
        verify(notesList !== null)
        tryCompare(notesList.noteRows, "count", 2)
        let edited = null
        let completed = null
        let moved = null
        let deleted = null
        notesList.noteEditRequested.connect(function(taskId, taskListId, title, body) {
            edited = { taskId: taskId, taskListId: taskListId, title: title, body: body }
        })
        notesList.noteCompletionRequested.connect(function(taskId, isCompleted) {
            completed = { taskId: taskId, isCompleted: isCompleted }
        })
        notesList.noteMoveRequested.connect(function(taskId, taskListId, title) {
            moved = { taskId: taskId, taskListId: taskListId, title: title }
        })
        notesList.noteDeleteRequested.connect(function(taskId, title) {
            deleted = { taskId: taskId, title: title }
        })
        notesList.noteEditRequested("note-1", "list-inbox", "Release notes",
                                    "Verify the package artifacts")
        notesList.noteCompletionRequested("note-1", true)
        notesList.noteMoveRequested("note-1", "list-inbox", "Release notes")
        notesList.noteDeleteRequested("note-1", "Release notes")
        compare(edited.taskId, "note-1")
        compare(edited.taskListId, "list-inbox")
        compare(completed.isCompleted, true)
        compare(moved.title, "Release notes")
        compare(deleted.taskId, "note-1")
        notesList.destroy()
        notesModel.destroy()
    }

    function test_noteEditorEmitsTaskSaveRequest() {
        const component = Qt.createComponent("../../qml/NoteEditorDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-inbox", title: "Inbox", selected: true, taskCount: 0,
                           taskTitles: [] })
        const editor = component.createObject(null, { taskListModel: taskLists })
        verify(editor !== null)
        editor.openForEdit("note-1", "list-inbox", "Release notes", "Verify the package artifacts")
        verify(editor.primaryEnabled)
        let saved = null
        editor.taskSaveRequested.connect(function(taskId, taskListId, title, body) {
            saved = { taskId: taskId, taskListId: taskListId, title: title, body: body }
        })
        editor.noteTitle = " Revised release notes "
        editor.noteBody = "Update the package checklist"
        editor.primaryButton.click()
        compare(saved.taskId, "note-1")
        compare(saved.taskListId, "list-inbox")
        compare(saved.title, "Revised release notes")
        compare(saved.body, "Update the package checklist")
        editor.destroy()
        taskLists.destroy()
    }

    function test_mainSavesNotesThroughTaskMutationApi() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-inbox", title: "Inbox", selected: true, taskCount: 0,
                           taskTitles: [] })
        const calls = []
        const controller = {
            googleConnected: true,
            notesEnabled: true,
            sidebarTabIds: ["tasks", "calendar", "notes"],
            saveNoteTask: function(taskId, taskListId, title, body) {
                calls.push({ taskId: taskId, taskListId: taskListId, title: title, body: body })
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            taskListModel: taskLists,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.openNoteEditor("note-1", "list-inbox", "Release notes", "Verify the package artifacts")
        tryVerify(function() {
            return mainWindow.noteEditor.opened && mainWindow.noteEditor.noteTitleField.activeFocus
        })
        mainWindow.noteEditor.noteTitle = "Revised release notes"
        mainWindow.noteEditor.noteBody = "Update the package checklist"
        mainWindow.noteEditor.primaryButton.click()
        compare(calls.length, 1)
        compare(calls[0].taskId, "note-1")
        compare(calls[0].taskListId, "list-inbox")
        compare(calls[0].title, "Revised release notes")
        compare(calls[0].body, "Update the package checklist")
        mainWindow.destroy()
        taskLists.destroy()
    }

    function test_mainForwardsEventCreateRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.eventCreateRequested.connect(function(calendarId, title, startAt, endAt, allDay, description, location) {
            request = { calendarId, title, startAt, endAt, allDay, description, location }
        })
        mainWindow.eventCreateDialog.eventCreateRequested("calendar-primary", "Release review",
                                                          "2026-07-26T10:00:00.000Z",
                                                          "2026-07-26T11:00:00.000Z", false,
                                                          "Verify the native package", "Studio",
                                                          "Asia/Singapore", "4", true, "private",
                                                          ["guest@example.com"], false,
                                                          [{ method: "popup", minutes: 10 }], "")
        compare(request.calendarId, "calendar-primary")
        compare(request.title, "Release review")
        compare(request.startAt, "2026-07-26T10:00:00.000Z")
        compare(request.endAt, "2026-07-26T11:00:00.000Z")
        compare(request.allDay, false)
        compare(request.description, "Verify the native package")
        compare(request.location, "Studio")
        mainWindow.destroy()
    }

    function test_mainForwardsEventUpdateRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.eventUpdateRequested.connect(function(eventId, calendarId, title, startAt, endAt, allDay, description, location) {
            request = { eventId, calendarId, title, startAt, endAt, allDay, description, location }
        })
        mainWindow.eventEditDialog.eventUpdateRequested("event-1", "calendar-primary", "Release review",
                                                        "2026-07-26T10:00:00.000Z",
                                                        "2026-07-26T11:00:00.000Z", false,
                                                        "Verify the native package", "Studio",
                                                        "Asia/Singapore", "4", true, "private",
                                                        ["guest@example.com"], false,
                                                        [{ method: "popup", minutes: 10 }], "", 0)
        compare(request.eventId, "event-1")
        compare(request.calendarId, "calendar-primary")
        compare(request.title, "Release review")
        compare(request.startAt, "2026-07-26T10:00:00.000Z")
        compare(request.endAt, "2026-07-26T11:00:00.000Z")
        compare(request.allDay, false)
        compare(request.description, "Verify the native package")
        compare(request.location, "Studio")
        mainWindow.destroy()
    }

    function test_mainForwardsEventDeleteRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let deletedId = ""
        mainWindow.eventDeleteRequested.connect(function(eventId) { deletedId = eventId })
        mainWindow.eventDeleteDialog.eventDeleteRequested("event-1", 0)
        compare(deletedId, "event-1")
        mainWindow.destroy()
    }

    function test_mainForwardsTaskDeleteRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let deletedId = ""
        mainWindow.taskDeleteRequested.connect(function(taskId) { deletedId = taskId })
        mainWindow.taskDeleteDialog.taskDeleteRequested("task-1")
        compare(deletedId, "task-1")
        mainWindow.destroy()
    }

    function test_mainForwardsTaskCreateRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.taskCreateRequested.connect(function(taskListId, parentTaskId, title) {
            request = { taskListId, parentTaskId, title }
        })
        mainWindow.taskCreateDialog.taskCreateRequested("list-active", "", "Plan release", "",
                                                        "", "", 0, false, 0, 1, 0, "", 0, "", "", "")
        compare(request.taskListId, "list-active")
        compare(request.parentTaskId, "")
        compare(request.title, "Plan release")
        mainWindow.destroy()
    }

    function test_mainForwardsTaskReparentRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.taskReparentRequested.connect(function(taskId, parentTaskId) {
            request = { taskId, parentTaskId }
        })
        mainWindow.taskList.taskReparentRequested("task-1", "")
        compare(request.taskId, "task-1")
        compare(request.parentTaskId, "")
        mainWindow.destroy()
    }

    function test_mainForwardsTaskUpdateRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.taskUpdateRequested.connect(function(taskId, title, notes, dueAt, dueTimeZone, priority) {
            request = { taskId, title, notes, dueAt, dueTimeZone, priority }
        })
        mainWindow.taskEditDialog.taskUpdateRequested("task-1", "Plan release", "Prepare checklist",
                                                      "2026-07-26", "Asia/Singapore", 2, false,
                                                      0, 1, 0, "", 0, "", "", "")
        compare(request.taskId, "task-1")
        compare(request.title, "Plan release")
        compare(request.notes, "Prepare checklist")
        compare(request.dueAt, "2026-07-26")
        compare(request.dueTimeZone, "Asia/Singapore")
        compare(request.priority, 2)
        mainWindow.destroy()
    }

    function test_mainForwardsTaskMoveRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let move = null
        mainWindow.taskMoveRequested.connect(function(taskId, taskListId) {
            move = { taskId, taskListId }
        })
        mainWindow.taskMoveDialog.taskMoveRequested("task-1", "list-other")
        compare(move.taskId, "task-1")
        compare(move.taskListId, "list-other")
        mainWindow.destroy()
    }

    function test_mainForwardsEventMoveRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.eventMoveRequested.connect(function(eventId, startAt, endAt, allDay) {
            request = { eventId, startAt, endAt, allDay }
        })
        mainWindow.dayTimeline.eventMoveRequested("event-1", "2026-07-26T10:00:00.000Z",
                                                  "2026-07-26T11:00:00.000Z", false)
        compare(request.eventId, "event-1")
        compare(request.startAt, "2026-07-26T10:00:00.000Z")
        compare(request.endAt, "2026-07-26T11:00:00.000Z")
        compare(request.allDay, false)
        mainWindow.destroy()
    }

    function test_mainForwardsEventResizeRequest() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        let request = null
        mainWindow.eventResizeRequested.connect(function(eventId, endAt) {
            request = { eventId, endAt }
        })
        mainWindow.dayTimeline.eventResizeRequested("event-1", "2026-07-26T12:00:00.000Z")
        compare(request.eventId, "event-1")
        compare(request.endAt, "2026-07-26T12:00:00.000Z")
        mainWindow.destroy()
    }

    function test_keyboardNoteCreationSubmitsTaskMutation() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const taskLists = Qt.createQmlObject('import QtQml.Models; ListModel {}', testCase)
        taskLists.append({ id: "list-inbox", title: "Inbox", selected: true, taskCount: 0,
                           taskTitles: [] })
        const calls = []
        const controller = {
            googleConnected: true,
            notesEnabled: true,
            sidebarTabIds: ["tasks", "calendar", "notes"],
            saveNoteTask: function(taskId, taskListId, title, body) {
                calls.push({ taskId: taskId, taskListId: taskListId, title: title, body: body })
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            taskListModel: taskLists,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.requestActivate()
        tryCompare(mainWindow, "active", true)
        keyClick(Qt.Key_3, Qt.ControlModifier)
        tryCompare(mainWindow, "currentPage", "Notes")
        mainWindow.notesList.noteCreateButton.click()
        tryVerify(function() {
            return mainWindow.noteEditor.opened && mainWindow.noteEditor.noteTitleField.activeFocus
        })
        mainWindow.noteEditor.noteTitle = "New release notes"
        mainWindow.noteEditor.noteBody = "Verify the package artifacts"
        keyClick(Qt.Key_Return)
        compare(calls.length, 1)
        compare(calls[0].taskId, "")
        compare(calls[0].taskListId, "list-inbox")
        compare(calls[0].title, "New release notes")
        compare(calls[0].body, "Verify the package artifacts")
        mainWindow.destroy()
        taskLists.destroy()
    }

    function test_usesDesignTokens() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())

        const mainWindow = component.createObject(null, { navigationCommands: navigationCommands })
        verify(mainWindow !== null)
        compare(mainWindow.color.toString(), systemPalette.window.toString())
        compare(mainWindow.palette.window.toString(), systemPalette.window.toString())
        compare(mainWindow.palette.base.toString(), systemPalette.base.toString())
        compare(mainWindow.palette.highlight.toString(), systemPalette.highlight.toString())
        mainWindow.destroy()
    }

    function test_visualSettingsRouteValidatedChoicesToController() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const calls = []
        const controller = {
            googleConnected: true,
            appearanceMode: 1,
            paletteMode: 2,
            accentColor: "#0A84FF",
            fontFamily: "Helvetica",
            availableFontFamilies: ["Helvetica"],
            fontScale: 2,
            displayTimeZone: "",
            availableTimeZones: ["", "Asia/Singapore"],
            savePaletteMode: function(mode) { calls.push(["palette", mode]) },
            saveAccentColor: function(color) { calls.push(["accent", color]) },
            saveFontFamily: function(family) { calls.push(["font", family]) },
            saveFontScale: function(scale) { calls.push(["scale", scale]) },
            saveDisplayTimeZone: function(timeZone) { calls.push(["timeZone", timeZone]) },
            resetVisualPreferences: function() { calls.push(["reset"]) }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller
        })
        verify(mainWindow !== null)
        compare(mainWindow.color.toString(), "#f8faff")
        compare(mainWindow.palette.highlight.toString(), "#0a84ff")
        compare(mainWindow.font.family, "Helvetica")
        compare(mainWindow.font.pixelSize, 16)
        mainWindow.paletteModeSelector.activated(2)
        mainWindow.accentColorField.text = "#0A84FF"
        mainWindow.accentColorField.editingFinished()
        mainWindow.fontFamilySelector.activated(1)
        mainWindow.fontScaleSelector.activated(3)
        mainWindow.displayTimeZoneSelector.activated(1)
        mainWindow.resetVisualPreferencesButton.click()
        compare(calls, [["palette", 2], ["accent", "#0A84FF"], ["font", "Helvetica"],
                        ["scale", 3], ["timeZone", "Asia/Singapore"], ["reset"]])
        mainWindow.destroy()
    }

    function test_accessibleButtonExposesLabelAndPressAction() {
        const component = Qt.createComponent("../../qml/AccessibleButton.qml")
        compare(component.status, Component.Ready, component.errorString())

        const button = component.createObject(null, { text: "Save" })
        verify(button !== null)
        compare(button.accessibleName, "")
        compare(button.Accessible.name, "Save")
        compare(button.Accessible.role, Accessible.Button)
        verify(button.Accessible.focusable)
        button.accessibleName = "Save draft"
        compare(button.Accessible.name, "Save draft")
        button.destroy()
    }

    function test_accessibleNavigationButtonRoutesSelection() {
        const component = Qt.createComponent("../../qml/AccessibleNavigationButton.qml")
        compare(component.status, Component.Ready, component.errorString())

        const button = component.createObject(null, { pageName: "Notes", currentPage: true })
        verify(button !== null)
        let selectedPage = ""
        button.pageSelected.connect(function(pageName) {
            selectedPage = pageName
        })
        compare(button.text, "Notes")
        verify(button.checkable)
        verify(button.checked)
        compare(button.Accessible.name, "Notes")
        compare(button.Accessible.role, Accessible.PageTab)
        verify(button.Accessible.checkable)
        verify(button.Accessible.checked)
        button.click()
        compare(selectedPage, "Notes")
        button.destroy()
    }

    function test_dialogProvidesComposableAccessibleActions() {
        const component = Qt.createComponent("../../qml/HcbDialog.qml")
        compare(component.status, Component.Ready, component.errorString())

        const dialog = component.createObject(null, {
            title: "Edit task",
            primaryText: "Update",
            secondaryText: "Discard",
            primaryDestructive: true
        })
        verify(dialog !== null)
        compare(dialog.primaryButton.text, "Update")
        compare(dialog.secondaryButton.text, "Discard")
        compare(dialog.primaryButton.Accessible.description, "Destructive action")
        compare(dialog.closePolicy, Popup.CloseOnEscape)

        let primaryActions = 0
        let secondaryActions = 0
        dialog.primaryAction.connect(function() { primaryActions += 1 })
        dialog.secondaryAction.connect(function() { secondaryActions += 1 })
        dialog.primaryButton.click()
        dialog.secondaryButton.click()
        compare(primaryActions, 1)
        compare(secondaryActions, 1)
        dialog.destroy()
    }

    function test_calendarManagementRestrictsOwnedAndPrimaryOperations() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const calls = []
        const controller = {
            googleConnected: true,
            busy: false,
            calendarManagementRows: [],
            importPreviewRows: [],
            importReadyToCommit: false,
            saveGoogleCalendarSettings: function(calendarId, title, description, timeZone,
                                                 selected, hidden, colorId) {
                calls.push([calendarId, title, selected, hidden])
            }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.calendarManagerDialog.openForCalendar({
            id: "owned", title: "Owned", accessRole: "owner", primary: false
        })
        verify(mainWindow.calendarManagerDialog.ownedSecondary)
        mainWindow.calendarManagerDialog.primaryButton.click()
        compare(calls, [["owned", "Owned", true, false]])
        mainWindow.calendarManagerDialog.openForCalendar({
            id: "primary", title: "Primary", accessRole: "owner", primary: true
        })
        verify(!mainWindow.calendarManagerDialog.ownedSecondary)
        verify(mainWindow.calendarManagerDialog.primaryCalendar)
        mainWindow.calendarManagerDialog.close()
        mainWindow.calendarManagerDialog.openForCalendar({
            id: "reader", title: "Reader", accessRole: "reader", primary: false
        })
        verify(!mainWindow.calendarManagerDialog.ownedSecondary)
        mainWindow.destroy()
    }

    function test_importCommandOpensPastePreview() {
        const component = Qt.createComponent("../../qml/Main.qml")
        compare(component.status, Component.Ready, component.errorString())
        const calls = []
        const controller = {
            googleConnected: true,
            busy: false,
            importPreviewRows: [],
            importSourceName: "",
            importReadyToCommit: false,
            previewDelimitedImport: function(text) { calls.push(text) },
            cancelImport: function() { calls.push("cancel") }
        }
        const mainWindow = component.createObject(null, {
            navigationCommands: navigationCommands,
            appController: controller
        })
        verify(mainWindow !== null)
        mainWindow.activateCommand({ commandId: "import.items" })
        tryVerify(function() { return mainWindow.importDialog.opened })
        compare(mainWindow.importDialog.primaryText, "Validate rows")
        mainWindow.importPastedText.text = "task title=\"Buy milk\""
        mainWindow.importPreviewPasteButton.click()
        compare(calls, ["task title=\"Buy milk\""])
        mainWindow.importDialog.secondaryButton.click()
        compare(calls, ["task title=\"Buy milk\"", "cancel"])
        mainWindow.destroy()
    }
}
