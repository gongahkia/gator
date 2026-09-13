import SwiftUI

struct GlobalQuickCaptureView: View {
    @Environment(AppModel.self) private var model

    enum CaptureMode: String, CaseIterable, Identifiable {
        case task
        case event
        case note

        var id: String { rawValue }

        var title: String {
            switch self {
            case .task: "Task"
            case .event: "Event"
            case .note: "Note"
            }
        }

        var icon: String {
            switch self {
            case .task: "checklist"
            case .event: "calendar.badge.plus"
            case .note: "note.text"
            }
        }
    }

    let onClose: () -> Void

    @State private var mode: CaptureMode = .task
    @State private var input = ""
    @State private var parsedTask = ParsedQuickAddTask(title: "", dueDate: nil, taskListHint: nil, matchedTokens: [])
    @State private var parsedEvent = ParsedQuickAddEvent(summary: "", startDate: nil, endDate: nil, location: nil, isAllDay: false, matchedTokens: [])
    @State private var selectedTaskListID: TaskListMirror.ID?
    @State private var selectedCalendarID: CalendarListMirror.ID?
    @State private var autoAppliedCalendarTag: String?
    @State private var userManuallyPickedCalendar = false
    @State private var isSubmitting = false
    @State private var errorMessage: String?
    @FocusState private var isInputFocused: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header
            inputField
            previewStrip
            if let errorMessage {
                Text(errorMessage)
                    .hcbFont(.caption)
                    .foregroundStyle(AppColor.ember)
            }
            footer
        }
        .hcbScaledPadding(18)
        .frame(width: 620)
        .background(.regularMaterial)
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
        .shadow(color: .black.opacity(0.18), radius: 24, x: 0, y: 14)
        .onAppear {
            selectedTaskListID = selectedTaskListID ?? defaultTaskListID
            selectedCalendarID = selectedCalendarID ?? defaultCalendarID
            reparse(input)
            isInputFocused = true
        }
        .onChange(of: mode) { _, _ in
            errorMessage = nil
            reparse(input)
            isInputFocused = true
        }
        .onExitCommand {
            onClose()
        }
    }

    private var header: some View {
        HStack(spacing: 10) {
            Picker("", selection: $mode) {
                ForEach(CaptureMode.allCases) { mode in
                    Text(mode.title).tag(mode)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .frame(width: 210)

            Label(mode.title, systemImage: mode.icon)
                .hcbFont(.subheadline, weight: .semibold)
                .foregroundStyle(AppColor.ink)

            Spacer(minLength: 0)

            Text("Return to add, Esc to close")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var inputField: some View {
        TextField(placeholder, text: $input, axis: .vertical)
            .textFieldStyle(.plain)
            .font(.system(.title3, design: .rounded, weight: .medium))
            .lineLimit(1...4)
            .focused($isInputFocused)
            .onSubmit { Task { await submit() } }
            .onChange(of: input) { _, newValue in reparse(newValue) }
            .hcbScaledPadding(14)
            .background(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .fill(AppColor.cream.opacity(0.72))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 14, style: .continuous)
                    .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
            )
    }

    private var previewStrip: some View {
        HStack(spacing: 8) {
            if primaryText.isEmpty {
                Label("Type \(mode.title.lowercased()) text", systemImage: "text.cursor")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                chip(icon: "text.alignleft", text: primaryText, tint: AppColor.ink)
            }

            switch mode {
            case .task:
                if let due = parsedTask.dueDate {
                    chip(icon: "calendar", text: taskTokenDisplay(.dueDate) ?? due.formatted(date: .abbreviated, time: .omitted), tint: AppColor.moss)
                }
                if parsedTask.taskListHint != nil {
                    chip(icon: "number", text: resolvedTaskListName, tint: AppColor.blue)
                }
            case .event:
                if let start = parsedEvent.startDate {
                    chip(icon: "clock", text: formattedEventTime(start), tint: AppColor.moss)
                }
                if let color = matchedEventColor {
                    chip(icon: "circle.fill", text: color.title, tint: eventColorTint(color))
                }
                if let calendar = matchedCalendar {
                    chip(icon: "calendar.circle", text: calendar.summary, tint: Color(hex: calendar.colorHex))
                }
                if let location = parsedEvent.location, location.isEmpty == false {
                    chip(icon: "mappin.and.ellipse", text: location, tint: AppColor.blue)
                }
            case .note:
                chip(icon: "note.text", text: "No due date", tint: AppColor.moss)
                if parsedTask.taskListHint != nil {
                    chip(icon: "number", text: resolvedTaskListName, tint: AppColor.blue)
                }
            }

            Spacer(minLength: 0)
        }
        .frame(minHeight: 26)
    }

    private var footer: some View {
        HStack(spacing: 10) {
            destinationPicker
            Spacer(minLength: 8)
            Button("Cancel") {
                onClose()
            }
            .keyboardShortcut(.cancelAction)

            Button {
                Task { await submit() }
            } label: {
                if isSubmitting {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Text(primaryActionLabel)
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(AppColor.ember)
            .keyboardShortcut(.defaultAction)
            .disabled(canSubmit == false)
        }
    }

    @ViewBuilder
    private var destinationPicker: some View {
        switch mode {
        case .task, .note:
            Picker("List", selection: Binding(
                get: { selectedTaskListID ?? defaultTaskListID },
                set: { selectedTaskListID = $0 }
            )) {
                ForEach(model.taskLists) { list in
                    Text(list.title).tag(Optional(list.id))
                }
            }
            .pickerStyle(.menu)
            .fixedSize()
            .labelsHidden()
        case .event:
            Picker("Calendar", selection: Binding(
                get: { selectedCalendarID ?? defaultCalendarID },
                set: { newValue in
                    selectedCalendarID = newValue
                    userManuallyPickedCalendar = true
                    autoAppliedCalendarTag = nil
                }
            )) {
                ForEach(model.calendars) { calendar in
                    Text(calendar.summary).tag(Optional(calendar.id))
                }
            }
            .pickerStyle(.menu)
            .fixedSize()
            .labelsHidden()
        }
    }

    private var placeholder: String {
        switch mode {
        case .task:
            "Add a task - try \"email rent receipt tmr #personal\""
        case .event:
            "Add an event - try \"Lunch with Bob tomorrow 1pm at Philz\""
        case .note:
            "Add a note - try \"follow up on pricing #ideas\""
        }
    }

    private var primaryText: String {
        switch mode {
        case .task:
            parsedTask.title
        case .event:
            summaryForSubmission()
        case .note:
            input.trimmingCharacters(in: .whitespacesAndNewlines)
        }
    }

    private var primaryActionLabel: String {
        switch mode {
        case .task: "Add Task"
        case .event: "Add Event"
        case .note: "Add Note"
        }
    }

    private var canSubmit: Bool {
        guard model.account != nil, isSubmitting == false, primaryText.isEmpty == false else {
            return false
        }
        switch mode {
        case .task, .note:
            return (selectedTaskListID ?? defaultTaskListID) != nil
        case .event:
            return (selectedCalendarID ?? defaultCalendarID) != nil
        }
    }

    private var defaultTaskListID: TaskListMirror.ID? {
        if let hint = parsedTask.taskListHint {
            if let match = model.taskLists.first(where: { $0.title.localizedCaseInsensitiveCompare(hint) == .orderedSame }) {
                return match.id
            }
            if let match = model.taskLists.first(where: { $0.title.localizedCaseInsensitiveContains(hint) }) {
                return match.id
            }
        }
        return model.taskLists.first?.id
    }

    private var defaultCalendarID: CalendarListMirror.ID? {
        model.calendarSnapshot.selectedCalendars.first?.id ?? model.calendars.first?.id
    }

    private var resolvedTaskListName: String {
        if let id = selectedTaskListID ?? defaultTaskListID {
            return model.taskListTitle(for: id, fallback: parsedTask.taskListHint ?? "")
        }
        return parsedTask.taskListHint ?? ""
    }

    private var colorTagResolution: ColorTagResolver.Resolution? {
        guard model.settings.colorTagAutoApplyEnabled else { return nil }
        return ColorTagResolver.resolve(
            title: parsedEvent.summary,
            bindings: model.settings.colorTagBindings,
            policy: model.settings.colorTagMatchPolicy
        )
    }

    private var matchedEventColor: CalendarEventColor? {
        guard let colorId = colorTagResolution?.colorId else { return nil }
        return CalendarEventColor.from(colorId: colorId)
    }

    private var calendarTagResolution: CalendarTagResolver.Resolution? {
        CalendarTagResolver.resolve(title: parsedEvent.summary, calendars: model.calendarSnapshot.selectedCalendars)
            ?? CalendarTagResolver.resolve(title: parsedEvent.summary, calendars: model.calendars)
    }

    private var matchedCalendar: CalendarListMirror? {
        calendarTagResolution?.calendar
    }

    private func chip(icon: String, text: String, tint: Color) -> some View {
        Label {
            Text(text).lineLimit(1)
        } icon: {
            Image(systemName: icon)
        }
        .hcbFont(.caption, weight: .medium)
        .hcbScaledPadding(.horizontal, 10)
        .hcbScaledPadding(.vertical, 5)
        .background(Capsule().fill(tint.opacity(0.15)))
        .foregroundStyle(tint)
    }

    private func reparse(_ text: String) {
        parsedTask = NaturalLanguageTaskParser().parse(text)
        parsedEvent = NaturalLanguageEventParser().parse(text)
        if selectedTaskListID == nil, let id = defaultTaskListID {
            selectedTaskListID = id
        }
        applyCalendarTagAutoSelection()
        if selectedCalendarID == nil, let id = defaultCalendarID {
            selectedCalendarID = id
        }
    }

    private func applyCalendarTagAutoSelection() {
        guard mode == .event, userManuallyPickedCalendar == false else { return }
        if let resolution = calendarTagResolution {
            if selectedCalendarID != resolution.calendar.id {
                selectedCalendarID = resolution.calendar.id
            }
            autoAppliedCalendarTag = resolution.matchedTag
        } else if autoAppliedCalendarTag != nil {
            selectedCalendarID = defaultCalendarID
            autoAppliedCalendarTag = nil
        }
    }

    private func taskTokenDisplay(_ kind: ParsedQuickAddTask.MatchedToken.Kind) -> String? {
        parsedTask.matchedTokens.first(where: { $0.kind == kind })?.display
    }

    private func formattedEventTime(_ start: Date) -> String {
        if parsedEvent.isAllDay {
            return start.formatted(.dateTime.month(.abbreviated).day())
        }
        guard let end = parsedEvent.endDate else {
            return start.formatted(.dateTime.month(.abbreviated).day().hour().minute())
        }
        if Calendar.current.isDate(start, inSameDayAs: end) {
            return "\(start.formatted(.dateTime.month(.abbreviated).day().hour().minute()))-\(end.formatted(.dateTime.hour().minute()))"
        }
        return "\(start.formatted(.dateTime.month(.abbreviated).day().hour().minute()))-\(end.formatted(.dateTime.month(.abbreviated).day().hour().minute()))"
    }

    private func eventColorTint(_ color: CalendarEventColor) -> Color {
        if let hex = color.hex {
            return Color(hex: hex)
        }
        return AppColor.ember
    }

    private func summaryForSubmission() -> String {
        let raw = parsedEvent.summary.trimmingCharacters(in: .whitespacesAndNewlines)
        var stripped = raw
        for tag in [colorTagResolution?.matchedTag, autoAppliedCalendarTag].compactMap({ $0 }) {
            stripped = ColorTagResolver.stripTag(tag, from: stripped)
        }
        return stripped.isEmpty ? raw : stripped
    }

    private func submit() async {
        guard canSubmit else { return }
        isSubmitting = true
        errorMessage = nil
        let didCreate: Bool
        switch mode {
        case .task:
            guard let listID = selectedTaskListID ?? defaultTaskListID else {
                isSubmitting = false
                return
            }
            didCreate = await model.createTask(
                title: parsedTask.title,
                notes: "",
                dueDate: parsedTask.dueDate,
                taskListID: listID
            )
        case .event:
            guard let calendarID = selectedCalendarID ?? defaultCalendarID else {
                isSubmitting = false
                return
            }
            let start = parsedEvent.startDate ?? Date()
            let end = parsedEvent.endDate ?? start.addingTimeInterval(3600)
            didCreate = await model.createEvent(
                summary: summaryForSubmission(),
                details: "",
                startDate: start,
                endDate: end,
                isAllDay: parsedEvent.isAllDay,
                reminderMinutes: nil,
                calendarID: calendarID,
                location: parsedEvent.location ?? "",
                recurrence: [],
                attendeeEmails: [],
                notifyGuests: false,
                addGoogleMeet: false,
                colorId: matchedEventColor?.wireValue
            )
        case .note:
            guard let listID = selectedTaskListID ?? defaultTaskListID else {
                isSubmitting = false
                return
            }
            didCreate = await model.createTask(
                title: primaryText,
                notes: "",
                dueDate: nil,
                taskListID: listID
            )
        }
        isSubmitting = false
        if didCreate {
            onClose()
        } else {
            errorMessage = model.lastMutationError ?? "Couldn't add \(mode.title.lowercased())."
        }
    }
}
