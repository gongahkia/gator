import SwiftUI

// Single unified popover for click-to-create + drag-to-create events and
// tasks. Apple-Calendar-inspired layout: title row with color swatch,
// location row, collapsible date section, invitees, notes.
//
// Default: all top-level fields are visible up-front. The date section
// itself toggles on click. Users who want the pre-§7.01 compact
// behaviour (only title + destination + [+ More]) can flip
// `quickCreateExpandedByDefault` off in Settings.
//
// Google coverage of each field:
// - Color (event.colorId) ✅
// - Location (event.location) ✅
// - Google Meet (event.conferenceData.createRequest) ✅
// - Start / end / all-day ✅
// - Repeat (event.recurrence RRULE) ✅
// - Alert (event.reminders.overrides) ✅
// - Invitees (event.attendees) ✅
// - Notes (event.description) ✅
// - End Repeat UNTIL/COUNT ✅
// - Travel Time — NOT in Google Calendar API, intentionally omitted.
// - Attachments — requires a Google Drive file picker; out of scope here.
// - Task priority / location / time-of-day — not in Google Tasks API.
struct QuickCreatePopover: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model

    let initialDate: Date
    let initialEnd: Date?
    let initiallyAllDay: Bool
    let taskOnly: Bool
    let initialTaskListID: TaskListMirror.ID?
    // Flips the sheet's identity to "New Note" and defaults hasDueDate off.
    // A note is the same Google Task underneath; tasks with note text appear
    // in Notes, and tasks with due dates also remain visible in Tasks.
    let noteMode: Bool

    @State private var mode: CreateMode = .event
    @State private var summary: String = ""
    @State private var selectedListID: TaskListMirror.ID?
    @State private var autoAppliedTaskListTag: String? = nil
    @State private var userManuallyPickedTaskList: Bool = false
    @State private var selectedCalendarID: CalendarListMirror.ID?
    @State private var autoAppliedCalendarTag: String? = nil
    @State private var userManuallyPickedCalendar: Bool = false
    @State private var isSaving = false
    @FocusState private var summaryFocused: Bool
    // Outer compact fallback — only honoured when
    // AppSettings.quickCreateExpandedByDefault is false. True by default so
    // the popover starts fully detailed.
    @State private var showOptionalFields: Bool = true
    // Inner date-panel expansion. Independent of the outer compact mode.
    @State private var isDatePanelExpanded: Bool = false

    // Editable event fields
    @State private var isAllDay: Bool = false
    @State private var startDate: Date = Date()
    @State private var endDate: Date = Date()
    @State private var eventTimeZoneID: String = TimezoneSupport.currentIdentifier
    @State private var userManuallyPickedEventTimeZone: Bool = false
    @State private var eventColor: CalendarEventColor = .defaultColor
    @State private var isColorPickerPresented = false
    // Color-tag auto-apply bookkeeping. `autoAppliedTag` remembers the
    // exact tag spelling that drove the current `eventColor` so we can
    // strip it from the title on submit. `userManuallyPickedColor` latches
    // once the user touches the color menu — from that point the auto
    // pass won't overwrite the user's choice for the rest of this popover
    // session.
    @State private var autoAppliedTag: String? = nil
    @State private var userManuallyPickedColor: Bool = false
    @State private var location: String = ""
    @State private var addGoogleMeet: Bool = false
    @State private var recurrenceRule: RecurrenceRule?
    @State private var reminderMinutes: Int? = nil
    @State private var attendeesRaw: String = ""
    @State private var notes: String = ""

    // Editable task fields
    @State private var taskDueDate: Date = Date()
    @State private var hasDueDate: Bool = true
    // Per-section expansion for the task popover (Apple Reminders style).
    @State private var isTaskDateCardExpanded: Bool = false

    private struct TaskListTagMatch {
        let list: TaskListMirror
        let tag: String
    }

    private var calendarStore: CalendarStore { model.calendarStore }
    private var taskStore: TaskStore { model.taskStore }

    enum CreateMode: String, Hashable { case event, task, birthday }

    init(
        initialDate: Date,
        isAllDay: Bool,
        initialEnd: Date? = nil,
        taskOnly: Bool = false,
        initialTaskListID: TaskListMirror.ID? = nil,
        noteMode: Bool = false
    ) {
        self.initialDate = initialDate
        self.initialEnd = initialEnd
        self.initiallyAllDay = isAllDay
        self.taskOnly = taskOnly
        self.initialTaskListID = initialTaskListID
        self.noteMode = noteMode
        // seed @State from args so first frame matches target (no flash). model-dependent seeds (list/calendar/settings) still run in .task below.
        let cal = Calendar.current
        _mode = State(initialValue: taskOnly ? .task : .event)
        _hasDueDate = State(initialValue: noteMode ? false : true)
        _isAllDay = State(initialValue: isAllDay)
        if isAllDay {
            let s = cal.startOfDay(for: initialDate)
            _startDate = State(initialValue: s)
            if let e = initialEnd {
                _endDate = State(initialValue: cal.date(byAdding: .day, value: -1, to: e) ?? s)
            } else {
                _endDate = State(initialValue: s)
            }
        } else {
            _startDate = State(initialValue: initialDate)
            _endDate = State(initialValue: initialEnd ?? initialDate.addingTimeInterval(3600))
        }
        _taskDueDate = State(initialValue: cal.startOfDay(for: initialDate))
    }

    var body: some View {
        VStack(spacing: 0) {
            topBar
            Divider()
            ScrollView {
                VStack(alignment: .leading, spacing: 10) {
                    summaryRow
                    if effectiveExpanded {
                        Group {
                            switch mode {
                            case .event: eventFields
                            case .task: taskFields
                            case .birthday: birthdayFields
                            }
                        }
                    }
                }
                .hcbScaledPadding(.horizontal, 14)
                .hcbScaledPadding(.vertical, 12)
            }
            .frame(maxHeight: 520)
            Divider()
            bottomBar
        }
        .hcbScaledFrame(width: 440)
        .background(.regularMaterial)
        .task {
            // mode/hasDueDate/isAllDay/startDate/endDate/taskDueDate are seeded in init() to avoid a first-frame flash. Only model-dependent seeds run here.
            selectedListID = initialTaskListID ?? taskStore.taskLists.first?.id
            selectedCalendarID = calendarStore.calendarSnapshot.selectedCalendars.first?.id ?? calendarStore.calendars.first?.id
            applyDefaultEventTimeZoneIfNeeded()
            summaryFocused = true
            showOptionalFields = calendarStore.settings.quickCreateExpandedByDefault
        }
        .onChange(of: summary) { _, newValue in
            applyColorTagAutoColor(title: newValue)
            applyCalendarTagAutoSelection(title: newValue)
            applyTaskListAutoSelection(title: newValue)
        }
        .onChange(of: mode) { _, newValue in
            if newValue == .task {
                applyTaskListAutoSelection(title: summary)
                autoAppliedCalendarTag = nil
            } else {
                autoAppliedTaskListTag = nil
                applyCalendarTagAutoSelection(title: summary)
                applyDefaultEventTimeZoneIfNeeded()
            }
        }
        .onChange(of: selectedCalendarID) { _, _ in
            applyDefaultEventTimeZoneIfNeeded()
        }
    }

    // Event-only color-tag auto-apply. Scans the title for a #tag that
    // matches a user-configured binding and mirrors the bound color into
    // eventColor. Skipped entirely once the user has made a manual pick
    // so we never overwrite their choice for the rest of the session.
    private func applyColorTagAutoColor(title: String) {
        guard mode == .event,
              calendarStore.settings.colorTagAutoApplyEnabled,
              userManuallyPickedColor == false
        else { return }
        if let resolution = ColorTagResolver.resolve(
            title: title,
            bindings: calendarStore.settings.colorTagBindings,
            policy: calendarStore.settings.colorTagMatchPolicy
        ) {
            let newColor = CalendarEventColor.from(colorId: resolution.colorId)
            if newColor != eventColor { eventColor = newColor }
            autoAppliedTag = resolution.matchedTag
        } else if autoAppliedTag != nil {
            // User removed the tag from the title — release the color back
            // to calendar default so the picker doesn't lie about source.
            eventColor = .defaultColor
            autoAppliedTag = nil
        }
    }

    private func applyTaskListAutoSelection(title: String) {
        guard mode == .task, userManuallyPickedTaskList == false else { return }
        if let match = taskListTagMatch(in: title) {
            if selectedListID != match.list.id { selectedListID = match.list.id }
            autoAppliedTaskListTag = match.tag
        } else if autoAppliedTaskListTag != nil {
            selectedListID = initialTaskListID ?? taskStore.taskLists.first?.id
            autoAppliedTaskListTag = nil
        }
    }

    private func applyCalendarTagAutoSelection(title: String) {
        guard mode == .event, userManuallyPickedCalendar == false else { return }
        if let resolution = calendarTagResolution(in: title) {
            if selectedCalendarID != resolution.calendar.id {
                selectedCalendarID = resolution.calendar.id
            }
            autoAppliedCalendarTag = resolution.matchedTag
        } else if autoAppliedCalendarTag != nil {
            selectedCalendarID = calendarStore.calendarSnapshot.selectedCalendars.first?.id ?? calendarStore.calendars.first?.id
            autoAppliedCalendarTag = nil
        }
    }

    private func calendarTagResolution(in title: String) -> CalendarTagResolver.Resolution? {
        CalendarTagResolver.resolve(title: title, calendars: calendarStore.calendarSnapshot.selectedCalendars)
            ?? CalendarTagResolver.resolve(title: title, calendars: calendarStore.calendars)
    }

    private func taskListTagMatch(in title: String) -> TaskListTagMatch? {
        let tags = TagExtractor.tags(in: title)
        guard tags.isEmpty == false else { return nil }
        let listsByTagKey = taskStore.taskLists.reduce(into: [String: TaskListMirror]()) { result, list in
            let key = taskListTagKey(for: list.title)
            guard key.isEmpty == false, result[key] == nil else { return }
            result[key] = list
        }
        for tag in tags {
            let key = taskListTagKey(for: tag)
            if let list = listsByTagKey[key] {
                return TaskListTagMatch(list: list, tag: tag)
            }
        }
        return nil
    }

    private func taskListTagKey(for value: String) -> String {
        value
            .folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
            .unicodeScalars
            .filter { CharacterSet.alphanumerics.contains($0) }
            .map(String.init)
            .joined()
    }

    private func selectTaskListManually(_ id: TaskListMirror.ID) {
        selectedListID = id
        userManuallyPickedTaskList = true
        autoAppliedTaskListTag = nil
    }

    private func selectCalendarManually(_ id: CalendarListMirror.ID) {
        selectedCalendarID = id
        userManuallyPickedCalendar = true
        autoAppliedCalendarTag = nil
    }

    private var effectiveExpanded: Bool {
        calendarStore.settings.quickCreateExpandedByDefault || showOptionalFields
    }

    private var nativeControlFill: Color {
        Color(nsColor: .controlBackgroundColor).opacity(0.72)
    }

    // MARK: - Top bar (Event | Task toggle)

    private var topBar: some View {
        HStack(spacing: 10) {
            if taskOnly {
                // Task-only entry (e.g. clicking empty Kanban space) — the
                // Event/Task switcher is omitted so the popover can't slip
                // back into event mode from the Tasks tab. Note-mode tweaks
                // the label + icon to make the Notes-tab origin obvious.
                Label(noteMode ? "New Note" : "New Task",
                      systemImage: noteMode ? "note.text" : "checklist")
                    .hcbFont(.subheadline, weight: .semibold)
                    .foregroundStyle(AppColor.ink)
            } else {
                Picker("", selection: $mode) {
                    Text("Event").tag(CreateMode.event)
                    Text("Task").tag(CreateMode.task)
                    Text("Birthday").tag(CreateMode.birthday)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
            }
            Spacer(minLength: 0)
            destinationMenu
        }
        .hcbScaledPadding(.horizontal, 14)
        .hcbScaledPadding(.vertical, 10)
    }

    // Title row with inline color swatch (events only)
    private var summaryRow: some View {
        HStack(alignment: .top, spacing: 8) {
            TextField(summaryPlaceholder, text: $summary, axis: .vertical)
                .textFieldStyle(.plain)
                .hcbFont(.title3, weight: .medium)
                .lineLimit(1...4)
                .focused($summaryFocused)
                .onSubmit { Task { await save() } }
            if mode == .event {
                Button {
                    isColorPickerPresented.toggle()
                } label: {
                    colorSwatch(eventColor)
                }
                .buttonStyle(.plain)
                .fixedSize()
                .help("Event color")
                .popover(isPresented: $isColorPickerPresented, arrowEdge: .trailing) {
                    VStack(alignment: .leading, spacing: 10) {
                        Text("Event color")
                            .hcbFont(.caption, weight: .semibold)
                            .foregroundStyle(.secondary)
                        EventColorPicker(selection: Binding(
                            get: { eventColor },
                            set: { color in
                                eventColor = color
                                userManuallyPickedColor = true
                                autoAppliedTag = nil
                                isColorPickerPresented = false
                            }
                        ))
                    }
                    .hcbScaledPadding(12)
                    .frame(width: 240)
                }
            }
        }
    }

    private func colorSwatch(_ color: CalendarEventColor) -> some View {
        HStack(spacing: 4) {
            if let hex = color.hex {
                Circle()
                    .fill(Color(hex: hex))
                    .hcbScaledFrame(width: 16, height: 16)
            } else {
                Circle()
                    .strokeBorder(AppColor.cardStroke, lineWidth: 1)
                    .hcbScaledFrame(width: 16, height: 16)
            }
            Image(systemName: "chevron.up.chevron.down")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    // MARK: - Event fields

    @ViewBuilder
    private var eventFields: some View {
        locationRow
        datePanel
        attendeesRow
        notesRow
    }

    private var locationRow: some View {
        HStack(spacing: 8) {
            TextField("Add Location", text: $location)
                .textFieldStyle(.plain)
                .hcbFont(.body)
            Toggle(isOn: $addGoogleMeet) {
                Image(systemName: "video")
                    .hcbFont(.body)
            }
            .toggleStyle(.button)
            .tint(Color.accentColor)
            .help(addGoogleMeet ? "Google Meet link will be created" : "Add Google Meet")
        }
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(nativeControlFill)
        )
    }

    // Collapsed: one-line summary + hint. Expanded: editable fields with an
    // explicit collapse control for keyboard and VoiceOver users.
    private var datePanel: some View {
        VStack(alignment: .leading, spacing: 0) {
            if isDatePanelExpanded == false {
                Button {
                    HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.14)) { isDatePanelExpanded = true }
                } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(compactDateLine)
                            .hcbFont(.body)
                            .foregroundStyle(AppColor.ink)
                        Text(compactHintLine)
                            .hcbFont(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .hcbScaledPadding(10)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
            } else {
                expandedDatePanel
                    .hcbScaledPadding(10)
            }
        }
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(nativeControlFill)
        )
    }

    private var compactDateLine: String {
        let cal = Calendar.current
        let startStr = startDate.formatted(.dateTime.day().month(.abbreviated).year())
        if isAllDay {
            if cal.isDate(startDate, inSameDayAs: endDate) { return "\(startStr) · all-day" }
            let endStr = endDate.formatted(.dateTime.day().month(.abbreviated).year())
            return "\(startStr) → \(endStr) · all-day"
        }
        let startTime = startDate.formatted(.dateTime.hour().minute())
        let endTime = endDate.formatted(.dateTime.hour().minute())
        if cal.isDate(startDate, inSameDayAs: endDate) {
            return "\(startStr)  \(startTime) – \(endTime)"
        }
        let endStr = endDate.formatted(.dateTime.day().month(.abbreviated).year())
        return "\(startStr) \(startTime) → \(endStr) \(endTime)"
    }

    private var compactHintLine: String {
        var parts: [String] = []
        if let rule = recurrenceRule { parts.append(rule.summary) }
        if let m = reminderMinutes { parts.append(alertLabel(for: m)) }
        return parts.isEmpty ? "Add Alert or Repeat" : parts.joined(separator: " · ")
    }

    private var expandedDatePanel: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Label("Date and time", systemImage: "calendar")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                Spacer(minLength: 8)
                collapseEditorButton("Collapse date options") {
                    isDatePanelExpanded = false
                }
            }
            Toggle("All Day", isOn: $isAllDay)
                .toggleStyle(.switch)
                .tint(Color.accentColor)
            HStack {
                Text("Starts")
                    .hcbFont(.subheadline)
                    .frame(width: 90, alignment: .leading)
                if isAllDay {
                    DatePicker("", selection: $startDate, displayedComponents: [.date])
                        .labelsHidden()
                } else {
                    DatePicker("", selection: $startDate, displayedComponents: [.date, .hourAndMinute])
                        .labelsHidden()
                        .environment(\.timeZone, eventTimeZone)
                }
            }
            HStack {
                Text("Ends")
                    .hcbFont(.subheadline)
                    .frame(width: 90, alignment: .leading)
                if isAllDay {
                    DatePicker("", selection: $endDate, in: startDate..., displayedComponents: [.date])
                        .labelsHidden()
                } else {
                    HStack(spacing: 6) {
                        DatePicker("", selection: $endDate, in: startDate..., displayedComponents: [.date, .hourAndMinute])
                            .labelsHidden()
                            .environment(\.timeZone, eventTimeZone)
                        EventEndTimePickerMenu(
                            startDate: startDate,
                            endDate: $endDate,
                            timeZoneID: eventTimeZoneID
                        )
                    }
                }
            }
            if isAllDay == false {
                HStack {
                    Spacer().frame(width: 90)
                    EventTimeZonePickerRow(title: "Time zone", selection: eventTimeZoneBinding)
                }
            }
            RecurrenceEditor(rule: $recurrenceRule, startDate: startDate)
            HStack {
                Text("Alert")
                    .hcbFont(.subheadline)
                    .frame(width: 90, alignment: .leading)
                Picker("", selection: reminderBinding) {
                    Text("None").tag(Int?.none)
                    Text("At time of event").tag(Int?.some(0))
                    Text("5 minutes before").tag(Int?.some(5))
                    Text("10 minutes before").tag(Int?.some(10))
                    Text("15 minutes before").tag(Int?.some(15))
                    Text("30 minutes before").tag(Int?.some(30))
                    Text("1 hour before").tag(Int?.some(60))
                    Text("2 hours before").tag(Int?.some(120))
                    Text("1 day before").tag(Int?.some(60 * 24))
                    Text("2 days before").tag(Int?.some(60 * 24 * 2))
                }
                .labelsHidden()
                .pickerStyle(.menu)
            }
        }
    }

    private func collapseEditorButton(_ title: String, action: @escaping () -> Void) -> some View {
        Button {
            HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.14), action)
        } label: {
            Label(title, systemImage: "chevron.up")
                .labelStyle(.iconOnly)
        }
        .buttonStyle(.borderless)
        .controlSize(.small)
        .help(title)
        .accessibilityLabel(title)
    }

    private func alertLabel(for minutes: Int) -> String {
        switch minutes {
        case 0: return "At time"
        case 60 * 24: return "1 day before"
        case 60 * 24 * 2: return "2 days before"
        case ..<60: return "\(minutes) min before"
        default: return "\(minutes / 60) h before"
        }
    }

    private var attendeesRow: some View {
        HStack {
            TextField("Add Invitees (comma-separated emails)", text: $attendeesRaw)
                .textFieldStyle(.plain)
                .hcbFont(.body)
        }
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(nativeControlFill)
        )
    }

    private var notesRow: some View {
        MarkdownEditor(
            text: $notes,
            placeholder: "Add Notes or URL",
            minHeight: 90,
            maxHeight: 180,
            showInlinePreview: false
        )
    }

    // MARK: - Task fields (Apple-Reminders-style card layout)

    @ViewBuilder
    private var taskFields: some View {
        taskNotesCard
        sectionHeader("Date & Time")
        taskDateCard
        sectionHeader("Organisation")
        taskListCard
    }

    private func sectionHeader(_ text: String) -> some View {
        Text(text)
            .hcbFont(.caption, weight: .semibold)
            .foregroundStyle(.secondary)
            .textCase(.uppercase)
            .hcbScaledPadding(.horizontal, 4)
            .hcbScaledPadding(.top, 4)
    }

    private var taskNotesCard: some View {
        MarkdownEditor(
            text: $notes,
            placeholder: "Notes",
            minHeight: 90,
            maxHeight: 180,
            showInlinePreview: false
        )
    }

    // Date card — collapsed shows Date label + toggle + subtitle; the
    // disclosure button expands the inline DatePicker.
    private var taskDateCard: some View {
        VStack(spacing: 0) {
            HStack(spacing: 10) {
                Button {
                    HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.14)) {
                        isTaskDateCardExpanded.toggle()
                    }
                } label: {
                    HStack(spacing: 10) {
                        Image(systemName: "calendar")
                            .hcbFont(.body)
                            .foregroundStyle(.secondary)
                            .hcbScaledFrame(width: 22)
                        VStack(alignment: .leading, spacing: 2) {
                            Text("Date")
                                .hcbFont(.body)
                                .foregroundStyle(AppColor.ink)
                            if hasDueDate {
                                Text(taskDueDate.formatted(.dateTime.weekday(.wide).day().month(.wide).year()))
                                    .hcbFont(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        Spacer(minLength: 8)
                        Image(systemName: isTaskDateCardExpanded ? "chevron.up" : "chevron.down")
                            .hcbFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(isTaskDateCardExpanded ? "Collapse task date" : "Expand task date")

                Toggle("", isOn: $hasDueDate)
                    .labelsHidden()
                    .toggleStyle(.switch)
                    .tint(Color.accentColor)
            }
            .hcbScaledPadding(10)
            if isTaskDateCardExpanded, hasDueDate {
                Divider().hcbScaledPadding(.horizontal, 10)
                HStack {
                    Spacer(minLength: 0)
                    DatePicker("", selection: $taskDueDate, displayedComponents: [.date])
                        .labelsHidden()
                        .datePickerStyle(.stepperField)
                }
                .hcbScaledPadding(10)
            }
        }
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(nativeControlFill)
        )
    }


    private var taskListCard: some View {
        HStack(spacing: 10) {
            Image(systemName: "list.bullet")
                .hcbFont(.body)
                .foregroundStyle(.secondary)
                .hcbScaledFrame(width: 22)
            Text("List")
                .hcbFont(.body)
                .foregroundStyle(AppColor.ink)
            Spacer(minLength: 8)
            Menu {
                ForEach(taskStore.taskLists) { list in
                    Button(list.title) { selectTaskListManually(list.id) }
                }
            } label: {
                HStack(spacing: 4) {
                    Circle()
                        .fill(AppColor.blue)
                        .hcbScaledFrame(width: 8, height: 8)
                    Text(currentListTitle)
                        .hcbFont(.body)
                    Image(systemName: "chevron.up.chevron.down")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(nativeControlFill)
        )
    }

    // MARK: - Birthday fields

    // A birthday is a yearly all-day event under the hood (which is exactly
    // how Google's auto-generated birthday events are represented). The UI
    // hides time / location / Meet / attendees / recurrence — those are
    // implied. User picks a single date and an optional destination
    // calendar via the existing destinationMenu.
    @ViewBuilder
    private var birthdayFields: some View {
        HStack(spacing: 10) {
            Image(systemName: "gift")
                .hcbFont(.body)
                .foregroundStyle(AppColor.ember)
                .hcbScaledFrame(width: 22)
            Text("Date")
                .hcbFont(.body)
                .foregroundStyle(AppColor.ink)
            Spacer(minLength: 8)
            DatePicker("", selection: $startDate, displayedComponents: [.date])
                .labelsHidden()
                .datePickerStyle(.stepperField)
        }
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(nativeControlFill)
        )
        Text("Repeats yearly. Saved as an all-day event on the selected calendar.")
            .hcbFont(.caption2)
            .foregroundStyle(.secondary)
    }

    private var summaryPlaceholder: String {
        if noteMode { return "New Note" }
        switch mode {
        case .event: return "New Event"
        case .task: return "New Task"
        case .birthday: return "Whose birthday?"
        }
    }

    // MARK: - Bottom bar

    private var bottomBar: some View {
        VStack(spacing: 0) {
            // In note-mode, flipping the date toggle on also makes this a
            // dated task. It can still appear in Notes when it has note text,
            // but Tasks owns dated scheduling.
            if noteMode && hasDueDate {
                HStack(spacing: 8) {
                    Image(systemName: "info.circle")
                        .foregroundStyle(AppColor.blue)
                    Text("This will be created as a dated task and will also appear in Notes when it has note text.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                    Spacer(minLength: 0)
                }
                .hcbScaledPadding(.horizontal, 12)
                .hcbScaledPadding(.vertical, 8)
                .background(AppColor.blue.opacity(0.08))
            }
            HStack(spacing: 10) {
                if calendarStore.settings.quickCreateExpandedByDefault == false {
                    Button {
                        HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.14)) { showOptionalFields.toggle() }
                    } label: {
                        Label(showOptionalFields ? "Less" : "More", systemImage: showOptionalFields ? "chevron.up" : "plus")
                            .hcbFont(.caption)
                    }
                    .buttonStyle(.borderless)
                    .help("Compact mode — toggle the setting in Preferences to default to detailed")
                }
                Spacer(minLength: 8)
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button(primaryActionLabel) { Task { await save() } }
                    .buttonStyle(.borderedProminent)
                    .tint(Color.accentColor)
                    .keyboardShortcut(.defaultAction)
                    .disabled(canCreate == false)
            }
            .hcbScaledPadding(.horizontal, 12)
            .hcbScaledPadding(.vertical, 10)
        }
    }

    // Primary-button label tracks the actual outcome. In note mode we flip
    // between "Create Note" (undated) and "Create Task" (dated) so the user
    // isn't surprised by where the thing lands.
    private var primaryActionLabel: String {
        if noteMode {
            return hasDueDate ? "Create Task" : "Create Note"
        }
        return "Create"
    }

    @ViewBuilder
    private var destinationMenu: some View {
        switch mode {
        case .event, .birthday:
            Menu {
                ForEach(calendarStore.calendars) { cal in
                    Button(cal.summary) { selectCalendarManually(cal.id) }
                }
            } label: {
                Label(currentCalendarTitle, systemImage: "calendar.circle")
                    .hcbFont(.caption)
                    .lineLimit(1)
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        case .task:
            Menu {
                ForEach(taskStore.taskLists) { list in
                    Button(list.title) { selectTaskListManually(list.id) }
                }
            } label: {
                Label(currentListTitle, systemImage: "tray")
                    .hcbFont(.caption)
                    .lineLimit(1)
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
    }

    // MARK: - Bindings

    private var reminderBinding: Binding<Int?> {
        Binding(get: { reminderMinutes }, set: { reminderMinutes = $0 })
    }

    // MARK: - Derived

    private var currentListTitle: String {
        guard let selectedListID else { return "Inbox" }
        return calendarStore.taskListTitle(for: selectedListID, fallback: "Inbox")
    }

    private var currentCalendarTitle: String {
        calendarStore.calendars.first(where: { $0.id == selectedCalendarID })?.summary ?? "Calendar"
    }

    private var eventTimeZone: TimeZone {
        TimezoneSupport.timeZone(for: eventTimeZoneID)
    }

    private var eventTimeZoneBinding: Binding<String> {
        Binding(
            get: { eventTimeZoneID },
            set: { newValue in
                let normalized = TimezoneSupport.validatedIdentifier(newValue) ?? TimezoneSupport.currentIdentifier
                let oldValue = eventTimeZoneID
                guard normalized != oldValue else { return }
                if isAllDay == false {
                    startDate = TimezoneSupport.reinterpretingWallClock(startDate, from: oldValue, to: normalized)
                    endDate = TimezoneSupport.reinterpretingWallClock(endDate, from: oldValue, to: normalized)
                }
                eventTimeZoneID = normalized
                userManuallyPickedEventTimeZone = true
            }
        )
    }

    private func applyDefaultEventTimeZoneIfNeeded() {
        guard mode == .event,
              userManuallyPickedEventTimeZone == false
        else { return }
        let calendarTimeZoneID = selectedCalendarID
            .flatMap { id in calendarStore.calendars.first(where: { $0.id == id })?.timeZoneID }
        let resolved = TimezoneSupport.validatedIdentifier(calendarTimeZoneID) ?? TimezoneSupport.currentIdentifier
        let oldValue = eventTimeZoneID
        if oldValue != resolved, isAllDay == false {
            startDate = TimezoneSupport.reinterpretingWallClock(startDate, from: oldValue, to: resolved)
            endDate = TimezoneSupport.reinterpretingWallClock(endDate, from: oldValue, to: resolved)
        }
        eventTimeZoneID = resolved
    }

    private var canCreate: Bool {
        let trimmed = summary.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false, isSaving == false else { return false }
        switch mode {
        case .event, .birthday: return selectedCalendarID != nil
        case .task: return selectedListID != nil
        }
    }

    // MARK: - Save

    @MainActor
    private func save() async {
        let rawTrimmed = summary.trimmingCharacters(in: .whitespacesAndNewlines)
        guard rawTrimmed.isEmpty == false else { return }
        let trimmed = submissionTitle(from: rawTrimmed)
        guard trimmed.isEmpty == false else { return }
        isSaving = true
        defer { isSaving = false }
        switch mode {
        case .event:
            guard let calID = selectedCalendarID else { return }
            let attendees = attendeesRaw
                .split(separator: ",")
                .map { $0.trimmingCharacters(in: .whitespaces) }
                .filter { $0.isEmpty == false }
            let recurrence: [String] = recurrenceRule.map { [$0.rruleString()] } ?? []
            let didCreate = await model.createEvent(
                summary: trimmed,
                details: notes,
                startDate: startDate,
                endDate: endDate,
                isAllDay: isAllDay,
                reminderMinutes: reminderMinutes,
                calendarID: calID,
                location: location,
                recurrence: recurrence,
                attendeeEmails: attendees,
                notifyGuests: false,
                addGoogleMeet: addGoogleMeet,
                colorId: eventColor.wireValue,
                startTimeZoneID: eventTimeZoneID,
                endTimeZoneID: eventTimeZoneID
            )
            if didCreate { dismiss() }
        case .task:
            guard let listID = selectedListID else { return }
            let didCreate = await model.createTask(
                title: trimmed,
                notes: notes,
                dueDate: hasDueDate ? Calendar.current.startOfDay(for: taskDueDate) : nil,
                taskListID: listID
            )
            if didCreate { dismiss() }
        case .birthday:
            guard let calID = selectedCalendarID else { return }
            // All-day, single-day, yearly recurring. start == end is a valid
            // all-day Google Event when isAllDay = true (Google interprets
            // it as a 1-day span). RRULE:FREQ=YEARLY produces the annual
            // birthday repeat that mirrors what the Google Calendar mobile
            // app creates for native birthday entries.
            let day = Calendar.current.startOfDay(for: startDate)
            let didCreate = await model.createEvent(
                summary: trimmed,
                details: notes,
                startDate: day,
                endDate: day,
                isAllDay: true,
                reminderMinutes: nil,
                calendarID: calID,
                location: "",
                recurrence: ["RRULE:FREQ=YEARLY"],
                attendeeEmails: [],
                notifyGuests: false,
                addGoogleMeet: false,
                colorId: nil
            )
            if didCreate { dismiss() }
        }
    }

    private func submissionTitle(from rawTitle: String) -> String {
        switch mode {
        case .event:
            // Strip routing tags that drove implicit color/calendar choices
            // so Google receives the clean event title.
            var stripped = rawTitle
            for tag in [autoAppliedTag, autoAppliedCalendarTag].compactMap({ $0 }) {
                stripped = ColorTagResolver.stripTag(tag, from: stripped)
            }
            return stripped.isEmpty ? rawTitle : stripped
        case .task:
            // `#ListName` is a routing hint for quick task/note creation,
            // not task content. Keep a lone tag as the title so the button
            // does not silently create an empty task.
            guard let tag = autoAppliedTaskListTag else { return rawTitle }
            let stripped = ColorTagResolver.stripTag(tag, from: rawTitle)
            return stripped.isEmpty ? rawTitle : stripped
        case .birthday:
            return rawTitle
        }
    }
}
