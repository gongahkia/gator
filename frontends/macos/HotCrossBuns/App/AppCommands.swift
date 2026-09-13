import SwiftUI

extension Notification.Name {
    static let hcbZoomIn = Notification.Name("hcb.zoom.in")
    static let hcbZoomOut = Notification.Name("hcb.zoom.out")
    static let hcbZoomReset = Notification.Name("hcb.zoom.reset")
    static let hcbFocusCalendarSearch = Notification.Name("hcb.calendar.focusSearch")
}

@MainActor
final class AppCommandActions {
    var newTask: () -> Void = {}
    var newNote: () -> Void = {}
    var newEvent: () -> Void = {}
    var refresh: () -> Void = {}
    var forceResync: () -> Void = {}
    var switchTo: (SidebarItem) -> Void = { _ in }
    var toggleSidebar: () -> Void = {}
    var openSettingsWindow: () -> Void = {}
    var openDiagnostics: () -> Void = {}
    var toggleActionCenter: () -> Void = {}
    var openCommandPalette: () -> Void = {}
    var openHelp: () -> Void = {}
    var openHistory: () -> Void = {}
    var printToday: () -> Void = {}
    var exportDayICS: () -> Void = {}
    var exportWeekICS: () -> Void = {}
    var exportSettings: () -> Void = {}
    var importSettings: () -> Void = {}
    var zoomIn: () -> Void = {}
    var zoomOut: () -> Void = {}
    var zoomReset: () -> Void = {}

    // Routes a canonical HCBShortcutCommand to the corresponding closure.
    // Used by the leader-chord state machine (§6.9) so chord execution reuses
    // the same action plumbing the menu bar and keyboard shortcuts already do.
    // Commands that have no corresponding AppCommandActions closure (calendar
    // navigation, task-inspector keys, store-specific) no-op — those live in
    // views that own their own focused handlers.
    func execute(_ command: HCBShortcutCommand) {
        switch command {
        case .newTask: newTask()
        case .newNote: newNote()
        case .newEvent: newEvent()
        case .commandPalette: openCommandPalette()
        case .refresh: refresh()
        case .forceResync: forceResync()
        case .diagnostics: openDiagnostics()
        case .help: openHelp()
        case .goToCalendar: switchTo(.calendar)
        case .goToStore: switchTo(.store)
        case .goToNotes: switchTo(.notes)
        case .goToSettings: openSettingsWindow()
        case .zoomIn: zoomIn()
        case .zoomOut: zoomOut()
        case .zoomReset: zoomReset()
        case .printToday: printToday()
        case .openHistory: openHistory()
        default: break
        }
    }
}

private struct AppCommandActionsKey: FocusedValueKey {
    typealias Value = AppCommandActions
}

@MainActor
struct StoreCommandActions {
    var toggleInspector: () -> Void
    var deleteSelectedTasks: () -> Void
    var canDeleteSelectedTasks: Bool
}

@MainActor
struct CalendarCommandActions {
    var previous: () -> Void
    var today: () -> Void
    var next: () -> Void
    var jumpBack: () -> Void
    var jumpForward: () -> Void
    var goToDate: () -> Void
    var focusSearch: () -> Void
    var shareAvailability: () -> Void
    var showAgenda: () -> Void
    var showDay: () -> Void
    var showWeek: () -> Void
    var showMonth: () -> Void
    var showMultiDay: () -> Void
    var showYear: () -> Void
    var canNavigate: Bool
    var canShareAvailability: Bool
    var canShowAgenda: Bool
    var canShowDay: Bool
    var canShowWeek: Bool
    var canShowMonth: Bool
    var canShowMultiDay: Bool
    var canShowYear: Bool
}

@MainActor
struct CalendarEventEditorCommandActions {
    var duplicateEvent: () -> Void
    var canDuplicateEvent: Bool
}

@MainActor
struct TaskInspectorCommandActions {
    var saveAndClose: () -> Void
    var toggleCompletion: () -> Void
    var delete: () -> Void
    var duplicate: () -> Void
}

private struct StoreCommandActionsKey: FocusedValueKey {
    typealias Value = StoreCommandActions
}

private struct CalendarCommandActionsKey: FocusedValueKey {
    typealias Value = CalendarCommandActions
}

private struct CalendarEventEditorCommandActionsKey: FocusedValueKey {
    typealias Value = CalendarEventEditorCommandActions
}

private struct TaskInspectorCommandActionsKey: FocusedValueKey {
    typealias Value = TaskInspectorCommandActions
}

extension FocusedValues {
    var appCommandActions: AppCommandActions? {
        get { self[AppCommandActionsKey.self] }
        set { self[AppCommandActionsKey.self] = newValue }
    }

    var storeCommandActions: StoreCommandActions? {
        get { self[StoreCommandActionsKey.self] }
        set { self[StoreCommandActionsKey.self] = newValue }
    }

    var calendarCommandActions: CalendarCommandActions? {
        get { self[CalendarCommandActionsKey.self] }
        set { self[CalendarCommandActionsKey.self] = newValue }
    }

    var calendarEventEditorCommandActions: CalendarEventEditorCommandActions? {
        get { self[CalendarEventEditorCommandActionsKey.self] }
        set { self[CalendarEventEditorCommandActionsKey.self] = newValue }
    }

    var taskInspectorCommandActions: TaskInspectorCommandActions? {
        get { self[TaskInspectorCommandActionsKey.self] }
        set { self[TaskInspectorCommandActionsKey.self] = newValue }
    }
}

struct AppCommands: Commands {
    @FocusedValue(\.appCommandActions) private var actions
    @FocusedValue(\.storeCommandActions) private var storeActions
    @FocusedValue(\.calendarCommandActions) private var calendarActions
    @FocusedValue(\.calendarEventEditorCommandActions) private var calendarEventEditorActions
    @FocusedValue(\.taskInspectorCommandActions) private var taskInspectorActions
    // @AppStorage invalidates the Commands body when the JSON string
    // changes, so user-edited bindings take effect immediately without a
    // relaunch. AppModel.setShortcutBinding writes to this same key.
    @AppStorage(HCBShortcutStorage.userDefaultsKey) private var overridesJSON: String = "{}"

    private var overrides: [String: HCBKeyBinding] {
        HCBShortcutStorage.decode(overridesJSON)
    }

    private func binding(_ cmd: HCBShortcutCommand) -> HCBKeyBinding {
        overrides[cmd.rawValue] ?? cmd.defaultBinding
    }

    var body: some Commands {
        CommandGroup(replacing: .newItem) {
            let newTask = binding(.newTask)
            Button("New Task") { actions?.newTask() }
                .keyboardShortcut(newTask.key.keyEquivalent, modifiers: newTask.modifiers.eventModifiers)
                .disabled(actions == nil)
            let newNote = binding(.newNote)
            Button("New Note") { actions?.newNote() }
                .keyboardShortcut(newNote.key.keyEquivalent, modifiers: newNote.modifiers.eventModifiers)
                .disabled(actions == nil)
            let newEvent = binding(.newEvent)
            Button("New Event") { actions?.newEvent() }
                .keyboardShortcut(newEvent.key.keyEquivalent, modifiers: newEvent.modifiers.eventModifiers)
                .disabled(actions == nil)
        }

        CommandGroup(replacing: .printItem) {
            let palette = binding(.commandPalette)
            Button("Command Palette…") { actions?.openCommandPalette() }
                .keyboardShortcut(palette.key.keyEquivalent, modifiers: palette.modifiers.eventModifiers)
                .disabled(actions == nil)
            let print = binding(.printToday)
            Button("Print Today…") { actions?.printToday() }
                .keyboardShortcut(print.key.keyEquivalent, modifiers: print.modifiers.eventModifiers)
                .disabled(actions == nil)
            Divider()
            Button("Export Day as .ics…") { actions?.exportDayICS() }
                .disabled(actions == nil)
            Button("Export Week as .ics…") { actions?.exportWeekICS() }
                .disabled(actions == nil)
            Divider()
            Button("Export Settings…") { actions?.exportSettings() }
                .disabled(actions == nil)
            Button("Import Settings…") { actions?.importSettings() }
                .disabled(actions == nil)
        }

        CommandMenu("Sync") {
            let refresh = binding(.refresh)
            Button("Refresh") { actions?.refresh() }
                .keyboardShortcut(refresh.key.keyEquivalent, modifiers: refresh.modifiers.eventModifiers)
                .disabled(actions == nil)
            let force = binding(.forceResync)
            Button("Force Full Resync") { actions?.forceResync() }
                .keyboardShortcut(force.key.keyEquivalent, modifiers: force.modifiers.eventModifiers)
                .disabled(actions == nil)
            Button("Action Center") { actions?.toggleActionCenter() }
                .disabled(actions == nil)
            Divider()
            let diag = binding(.diagnostics)
            Button("Diagnostics and Recovery…") { actions?.openDiagnostics() }
                .keyboardShortcut(diag.key.keyEquivalent, modifiers: diag.modifiers.eventModifiers)
                .disabled(actions == nil)
            let hist = binding(.openHistory)
            Button("History…") { actions?.openHistory() }
                .keyboardShortcut(hist.key.keyEquivalent, modifiers: hist.modifiers.eventModifiers)
                .disabled(actions == nil)
        }

        if storeActions != nil || taskInspectorActions != nil {
            CommandMenu("Tasks") {
                if let storeActions {
                    let inspector = binding(.storeShowInspector)
                    Button("Toggle Task Inspector") { storeActions.toggleInspector() }
                        .keyboardShortcut(inspector.key.keyEquivalent, modifiers: inspector.modifiers.eventModifiers)
                }

                if let taskInspectorActions {
                    if storeActions != nil {
                        Divider()
                    }

                    let saveClose = binding(.taskSaveAndClose)
                    Button("Save and Close Task") { taskInspectorActions.saveAndClose() }
                        .keyboardShortcut(saveClose.key.keyEquivalent, modifiers: saveClose.modifiers.eventModifiers)

                    let quickSave = binding(.taskQuickSave)
                    Button("Toggle Complete") { taskInspectorActions.toggleCompletion() }
                        .keyboardShortcut(quickSave.key.keyEquivalent, modifiers: quickSave.modifiers.eventModifiers)

                    let delete = binding(.taskDelete)
                    Button("Delete Task") { taskInspectorActions.delete() }
                        .keyboardShortcut(delete.key.keyEquivalent, modifiers: delete.modifiers.eventModifiers)

                    let duplicate = binding(.taskDuplicate)
                    Button("Duplicate Task") { taskInspectorActions.duplicate() }
                        .keyboardShortcut(duplicate.key.keyEquivalent, modifiers: duplicate.modifiers.eventModifiers)
                } else if let storeActions {
                    Divider()
                    let deleteSelected = binding(.storeClearCompleted)
                    Button("Delete Selected Tasks") { storeActions.deleteSelectedTasks() }
                        .keyboardShortcut(deleteSelected.key.keyEquivalent, modifiers: deleteSelected.modifiers.eventModifiers)
                        .disabled(storeActions.canDeleteSelectedTasks == false)
                }
            }
        }

        if calendarActions != nil || calendarEventEditorActions != nil {
            CommandMenu("Calendar") {
                if let calendarActions {
                    let previous = binding(.calendarPrevious)
                    Button("Previous Period") { calendarActions.previous() }
                        .keyboardShortcut(previous.key.keyEquivalent, modifiers: previous.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    let today = binding(.calendarToday)
                    Button("Jump to Today") { calendarActions.today() }
                        .keyboardShortcut(today.key.keyEquivalent, modifiers: today.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    let next = binding(.calendarNext)
                    Button("Next Period") { calendarActions.next() }
                        .keyboardShortcut(next.key.keyEquivalent, modifiers: next.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    Divider()

                    let jumpBack = binding(.calendarJumpBack)
                    Button("Jump Back") { calendarActions.jumpBack() }
                        .keyboardShortcut(jumpBack.key.keyEquivalent, modifiers: jumpBack.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    let jumpForward = binding(.calendarJumpForward)
                    Button("Jump Forward") { calendarActions.jumpForward() }
                        .keyboardShortcut(jumpForward.key.keyEquivalent, modifiers: jumpForward.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    let goToDate = binding(.calendarGoToDate)
                    Button("Go to Date…") { calendarActions.goToDate() }
                        .keyboardShortcut(goToDate.key.keyEquivalent, modifiers: goToDate.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    let focusSearch = binding(.calendarFocusSearch)
                    Button("Focus Search") { calendarActions.focusSearch() }
                        .keyboardShortcut(focusSearch.key.keyEquivalent, modifiers: focusSearch.modifiers.eventModifiers)
                        .disabled(calendarActions.canNavigate == false)

                    let shareAvailability = binding(.calendarShareAvailability)
                    Button("Share Availability…") { calendarActions.shareAvailability() }
                        .keyboardShortcut(shareAvailability.key.keyEquivalent, modifiers: shareAvailability.modifiers.eventModifiers)
                        .disabled(calendarActions.canShareAvailability == false)

                    Divider()

                    let agenda = binding(.calendarViewAgenda)
                    Button("Agenda View") { calendarActions.showAgenda() }
                        .keyboardShortcut(agenda.key.keyEquivalent, modifiers: agenda.modifiers.eventModifiers)
                        .disabled(calendarActions.canShowAgenda == false)

                    let day = binding(.calendarViewDay)
                    Button("Day View") { calendarActions.showDay() }
                        .keyboardShortcut(day.key.keyEquivalent, modifiers: day.modifiers.eventModifiers)
                        .disabled(calendarActions.canShowDay == false)

                    let week = binding(.calendarViewWeek)
                    Button("Week View") { calendarActions.showWeek() }
                        .keyboardShortcut(week.key.keyEquivalent, modifiers: week.modifiers.eventModifiers)
                        .disabled(calendarActions.canShowWeek == false)

                    let month = binding(.calendarViewMonth)
                    Button("Month View") { calendarActions.showMonth() }
                        .keyboardShortcut(month.key.keyEquivalent, modifiers: month.modifiers.eventModifiers)
                        .disabled(calendarActions.canShowMonth == false)

                    let multiDay = binding(.calendarViewMultiDay)
                    Button("Multi-Day View") { calendarActions.showMultiDay() }
                        .keyboardShortcut(multiDay.key.keyEquivalent, modifiers: multiDay.modifiers.eventModifiers)
                        .disabled(calendarActions.canShowMultiDay == false)

                    let year = binding(.calendarViewYear)
                    Button("Year View") { calendarActions.showYear() }
                        .keyboardShortcut(year.key.keyEquivalent, modifiers: year.modifiers.eventModifiers)
                        .disabled(calendarActions.canShowYear == false)
                }

                if let calendarEventEditorActions {
                    if calendarActions != nil {
                        Divider()
                    }

                    let duplicate = binding(.calendarDuplicateEvent)
                    Button("Duplicate Event") { calendarEventEditorActions.duplicateEvent() }
                        .keyboardShortcut(duplicate.key.keyEquivalent, modifiers: duplicate.modifiers.eventModifiers)
                        .disabled(calendarEventEditorActions.canDuplicateEvent == false)
                }
            }
        }

        CommandGroup(replacing: .help) {
            let help = binding(.help)
            Button("Hot Cross Buns Help…") { actions?.openHelp() }
                .keyboardShortcut(help.key.keyEquivalent, modifiers: help.modifiers.eventModifiers)
                .disabled(actions == nil)
        }

        CommandMenu("View") {
            ForEach(SidebarItem.allCases) { item in
                if let sidebarCommand = sidebarShortcutCommand(item) {
                    let b = binding(sidebarCommand)
                    Button(item.title) { actions?.switchTo(item) }
                        .keyboardShortcut(b.key.keyEquivalent, modifiers: b.modifiers.eventModifiers)
                        .disabled(actions == nil)
                } else {
                    Button(item.title) { actions?.switchTo(item) }
                        .disabled(actions == nil)
                }
            }
            Divider()
            Button("Toggle Sidebar") { actions?.toggleSidebar() }
                .disabled(actions == nil)
            let zIn = binding(.zoomIn)
            Button("Zoom In") { triggerZoomIn() }
                .keyboardShortcut(zIn.key.keyEquivalent, modifiers: zIn.modifiers.eventModifiers)
            let zOut = binding(.zoomOut)
            Button("Zoom Out") { triggerZoomOut() }
                .keyboardShortcut(zOut.key.keyEquivalent, modifiers: zOut.modifiers.eventModifiers)
            let zReset = binding(.zoomReset)
            Button("Actual Size") { triggerZoomReset() }
                .keyboardShortcut(zReset.key.keyEquivalent, modifiers: zReset.modifiers.eventModifiers)
        }
    }

    private func sidebarShortcutCommand(_ item: SidebarItem) -> HCBShortcutCommand? {
        switch item {
        case .calendar: .goToCalendar
        case .store: .goToStore
        case .notes: .goToNotes
        }
    }

    private func triggerZoomIn() {
        if let actions {
            actions.zoomIn()
        } else {
            NotificationCenter.default.post(name: .hcbZoomIn, object: nil)
        }
    }

    private func triggerZoomOut() {
        if let actions {
            actions.zoomOut()
        } else {
            NotificationCenter.default.post(name: .hcbZoomOut, object: nil)
        }
    }

    private func triggerZoomReset() {
        if let actions {
            actions.zoomReset()
        } else {
            NotificationCenter.default.post(name: .hcbZoomReset, object: nil)
        }
    }
}
