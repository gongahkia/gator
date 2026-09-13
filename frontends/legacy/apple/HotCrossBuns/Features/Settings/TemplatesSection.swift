import AppKit
import SwiftUI

// Settings UI for §6.13 / §6.13b templates. Templates are stored locally
// and never written to Google; instantiation creates a real task / event
// with fully expanded field values, so the saved entry on google.com looks
// indistinguishable from a manually-created one.
struct TemplatesSection: View {
    @Environment(AppModel.self) private var model
    var highlightedAnchor: SettingsSectionAnchor? = nil
    @State private var taskEditor: TaskTemplate?
    @State private var isCreatingTask = false
    @State private var eventEditor: EventTemplate?
    @State private var isCreatingEvent = false

    var body: some View {
        taskTemplatesSection
        eventTemplatesSection
    }

    // MARK: - Task templates

    private var taskTemplatesSection: some View {
        Section("Task templates") {
            SettingsHighlightRow(anchor: .templates, highlightedAnchor: highlightedAnchor)
            SettingsFeatureFlow(
                systemImage: "doc.text",
                title: "Task blueprint",
                steps: [
                    "Define reusable fields",
                    "Fill variables on insert",
                    "Create a real Google task"
                ]
            )
            if model.settings.taskTemplates.isEmpty {
                Text("No templates yet. Create one to pre-fill title, notes, due, and list using variables like {{today}}, {{+7d}}, {{nextWeekday:mon}}, {{prompt:Owner}}, or {{clipboard}}.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.settings.taskTemplates) { template in
                    Button {
                        taskEditor = template
                    } label: {
                        HStack {
                            Label(template.name, systemImage: "doc.text")
                            Spacer()
                            Text(template.title.isEmpty ? "(no title)" : template.title)
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                    }
                    .buttonStyle(.plain)
                    .swipeActions(edge: .trailing) {
                        Button(role: .destructive) {
                            model.deleteTaskTemplate(template.id)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                    .contextMenu {
                        Button {
                            taskEditor = template
                        } label: {
                            Label("Edit", systemImage: "pencil")
                        }
                        Button {
                            model.duplicateTaskTemplate(template)
                        } label: {
                            Label("Duplicate", systemImage: "plus.square.on.square")
                        }
                        Button {
                            taskEditor = model.duplicateTaskTemplate(template)
                        } label: {
                            Label("Duplicate and Edit", systemImage: "pencil.and.list.clipboard")
                        }
                        Divider()
                        Button(role: .destructive) {
                            model.deleteTaskTemplate(template.id)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
            }
            Button {
                isCreatingTask = true
            } label: {
                Label("New Task Template", systemImage: "plus")
            }
            Text("Instantiate from the command palette: \"Insert Task Template…\". Variables in {{…}} are expanded before the task is created; unknown variables are left visible so typos don't silently drop values.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .sheet(isPresented: $isCreatingTask) {
            TaskTemplateEditor(draft: TaskTemplate(name: "New Template", title: "")) { updated in
                model.upsertTaskTemplate(updated)
                isCreatingTask = false
            } onCancel: {
                isCreatingTask = false
            }
        }
        .sheet(item: $taskEditor) { current in
            TaskTemplateEditor(draft: current) { updated in
                model.upsertTaskTemplate(updated)
                taskEditor = nil
            } onCancel: { taskEditor = nil }
        }
    }

    // MARK: - Event templates (§6.13b)

    private var eventTemplatesSection: some View {
        Section("Event templates") {
            SettingsHighlightRow(anchor: .templates, highlightedAnchor: highlightedAnchor)
            SettingsFeatureFlow(
                systemImage: "calendar.badge.plus",
                title: "Event blueprint",
                steps: [
                    "Define time and guests",
                    "Fill variables on insert",
                    "Create a real calendar event"
                ]
            )
            if model.settings.eventTemplates.isEmpty {
                Text("No event templates yet. Create one to pre-fill title, time, location, attendees, and recurrence using variables like {{today}}, {{nextWeekday:mon}}, {{prompt:Topic}}.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.settings.eventTemplates) { template in
                    Button {
                        eventEditor = template
                    } label: {
                        HStack {
                            Label(template.name, systemImage: "calendar")
                            Spacer()
                            Text(template.summary.isEmpty ? "(no title)" : template.summary)
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                    }
                    .buttonStyle(.plain)
                    .swipeActions(edge: .trailing) {
                        Button(role: .destructive) {
                            model.deleteEventTemplate(template.id)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                    .contextMenu {
                        Button {
                            eventEditor = template
                        } label: {
                            Label("Edit", systemImage: "pencil")
                        }
                        Button {
                            model.duplicateEventTemplate(template)
                        } label: {
                            Label("Duplicate", systemImage: "plus.square.on.square")
                        }
                        Button {
                            eventEditor = model.duplicateEventTemplate(template)
                        } label: {
                            Label("Duplicate and Edit", systemImage: "pencil.and.list.clipboard")
                        }
                        Divider()
                        Button(role: .destructive) {
                            model.deleteEventTemplate(template.id)
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
            }
            Button {
                isCreatingEvent = true
            } label: {
                Label("New Event Template", systemImage: "plus")
            }
            Text("Instantiate from the command palette: \"Insert Event Template…\". Same variable set as task templates; dateAnchor resolves to YYYY-MM-DD, timeAnchor is literal HH:mm. Unknown variables stay visible so typos don't silently drop values.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .sheet(isPresented: $isCreatingEvent) {
            EventTemplateEditor(
                draft: EventTemplate(name: "New Event Template", summary: "")
            ) { updated in
                model.upsertEventTemplate(updated)
                isCreatingEvent = false
            } onCancel: {
                isCreatingEvent = false
            }
        }
        .sheet(item: $eventEditor) { current in
            EventTemplateEditor(draft: current) { updated in
                model.upsertEventTemplate(updated)
                eventEditor = nil
            } onCancel: { eventEditor = nil }
        }
    }
}

private struct TaskTemplateEditor: View {
    @Environment(\.dismiss) private var dismiss
    @State var draft: TaskTemplate
    @FocusState private var isNameFocused: Bool
    let onSave: (TaskTemplate) -> Void
    let onCancel: () -> Void

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 20) {
                        SettingsSheetSection("Template") {
                            SettingsSheetRow("Name") {
                                TextField("", text: $draft.name)
                                    .textFieldStyle(.roundedBorder)
                                    .focused($isNameFocused)
                            }
                            UsedByRow(items: ["Command Palette", "Insert Task Template"])
                        }

                        SettingsSheetSection("Task") {
                            SettingsSheetRow("Title") {
                                TextField("Required", text: $draft.title)
                                    .textFieldStyle(.roundedBorder)
                            }
                            SettingsSheetRow("Notes") {
                                TextField("", text: $draft.notes, axis: .vertical)
                                    .lineLimit(3 ... 8)
                                    .textFieldStyle(.roundedBorder)
                            }
                            SettingsSheetRow("Due") {
                                TextField("{{today}}, {{+7d}}, or 2026-05-01", text: $draft.due)
                                    .textFieldStyle(.roundedBorder)
                            }
                            SettingsSheetRow("List") {
                                TextField("ID, title, or empty for default", text: $draft.listIdOrTitle)
                                    .textFieldStyle(.roundedBorder)
                            }
                        }

                        TaskTemplateOutcomePreview(template: draft)

                        SettingsSheetSection("Variables") {
                            let prompts = draft.requiredPrompts()
                            if prompts.isEmpty == false {
                                Text("Prompts at instantiation: \(prompts.joined(separator: ", "))")
                                    .hcbFont(.caption)
                                    .foregroundStyle(.secondary)
                            } else {
                                Text("No {{prompt:Label}} placeholders — the template instantiates without asking for input.")
                                    .hcbFont(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Text("{{today}} {{tomorrow}} {{yesterday}} {{+Nd/-Nd/w/m/y}} {{nextWeekday:mon}} {{clipboard}} {{cursor}} {{prompt:Label}}")
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                    .padding(24)
                    .frame(maxWidth: .infinity, alignment: .topLeading)
                }

                Divider()

                templateValidationSummary(taskValidationErrors)

                SettingsSheetActions(cancelTitle: "Cancel", onCancel: onCancel) {
                    Button("Save") { onSave(draft) }
                        .keyboardShortcut(.defaultAction)
                        .buttonStyle(.borderedProminent)
                        .disabled(taskValidationErrors.isEmpty == false)
                }
            }
            .navigationTitle("Task Template")
        }
        .frame(width: 680, height: 540)
        .onAppear {
            isNameFocused = true
        }
    }

    private var taskValidationErrors: [String] {
        var errors: [String] = []
        if draft.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            errors.append("Name is required.")
        }
        if draft.title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            errors.append("Task title is required.")
        }
        let due = taskPreview.resolvedDue.trimmingCharacters(in: .whitespacesAndNewlines)
        if due.isEmpty == false
            && hasPromptPlaceholder(due) == false
            && due.range(of: "^\\d{4}-\\d{2}-\\d{2}$", options: .regularExpression) == nil {
            errors.append("Due must resolve to YYYY-MM-DD or stay empty.")
        }
        return errors
    }

    private var taskPreview: TaskTemplatePreview {
        TaskTemplatePreview(template: draft)
    }
}

// §6.13b — Event template editor. Provides a form for every templated
// event field. Attendee list is edited as newline-separated addresses.
private struct EventTemplateEditor: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model
    @State var draft: EventTemplate
    @FocusState private var isNameFocused: Bool
    let onSave: (EventTemplate) -> Void
    let onCancel: () -> Void

    // Newline-separated editor for the attendees array. Round-trips into
    // draft.attendees on every keystroke so requiredPrompts() stays accurate.
    @State private var attendeesRaw: String = ""

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 20) {
                        SettingsSheetSection("Template") {
                            SettingsSheetRow("Name") {
                                TextField("", text: $draft.name)
                                    .textFieldStyle(.roundedBorder)
                                    .focused($isNameFocused)
                            }
                            UsedByRow(items: ["Command Palette", "Insert Event Template", "Calendar editor"])
                        }

                        SettingsSheetSection("Event") {
                            SettingsSheetRow("Title") {
                                TextField("Required", text: $draft.summary)
                                    .textFieldStyle(.roundedBorder)
                            }
                            SettingsSheetRow("Details") {
                                TextField("", text: $draft.details, axis: .vertical)
                                    .lineLimit(3 ... 8)
                                    .textFieldStyle(.roundedBorder)
                            }
                            SettingsSheetRow("Location") {
                                TextField("", text: $draft.location)
                                    .textFieldStyle(.roundedBorder)
                            }
                        }

                        SettingsSheetSection("When") {
                            SettingsSheetRow("") {
                                Toggle("All-day", isOn: $draft.isAllDay)
                                    .toggleStyle(.checkbox)
                            }
                            SettingsSheetRow("Date anchor") {
                                TextField("{{today}}, {{nextWeekday:mon}}, or 2026-05-01", text: $draft.dateAnchor)
                                    .textFieldStyle(.roundedBorder)
                            }
                            if draft.isAllDay == false {
                                SettingsSheetRow("Time") {
                                    TextField("24h HH:mm; empty rounds up to the next 15m", text: $draft.timeAnchor)
                                        .textFieldStyle(.roundedBorder)
                                }
                            }
                            SettingsSheetRow("Duration") {
                                Stepper(
                                    value: $draft.durationMinutes,
                                    in: 5 ... 24 * 60,
                                    step: 5
                                ) {
                                    Text("\(draft.durationMinutes) min")
                                }
                            }
                        }

                        SettingsSheetSection("Recurrence") {
                            SettingsSheetRow("RRULE body") {
                                TextField("FREQ=WEEKLY;BYDAY=MO", text: $draft.recurrenceRule)
                                    .textFieldStyle(.roundedBorder)
                            }
                            Text("Leave empty for a one-off event. The \"RRULE:\" prefix is added automatically if missing.")
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                        }

                        SettingsSheetSection("Destination") {
                            if writableCalendars.isEmpty {
                                SettingsSheetRow("Calendar") {
                                    TextField("ID or title", text: $draft.calendarIdOrTitle)
                                        .textFieldStyle(.roundedBorder)
                                }
                                Text("No calendars loaded yet. Leave empty to fall back to the first writable calendar at instantiation.")
                                    .hcbFont(.caption2)
                                    .foregroundStyle(.secondary)
                            } else {
                                SettingsSheetRow("Calendar") {
                                    Picker("", selection: calendarPickerBinding) {
                                        Text("First writable (default)").tag("")
                                        ForEach(writableCalendars) { cal in
                                            Text(cal.summary).tag(cal.id)
                                        }
                                        if draft.calendarIdOrTitle.isEmpty == false
                                            && writableCalendars.contains(where: { $0.id == draft.calendarIdOrTitle }) == false {
                                            Text("Custom: \(draft.calendarIdOrTitle)").tag(draft.calendarIdOrTitle)
                                        }
                                    }
                                    .labelsHidden()
                                    .pickerStyle(.menu)
                                }
                            }
                        }

                        SettingsSheetSection("Guests") {
                            SettingsSheetRow("Attendees") {
                                TextField("One email per line", text: $attendeesRaw, axis: .vertical)
                                    .lineLimit(2 ... 6)
                                    .textFieldStyle(.roundedBorder)
                                    .onChange(of: attendeesRaw) { _, newValue in
                                        draft.attendees = newValue
                                            .split(whereSeparator: { $0.isNewline })
                                            .map { String($0).trimmingCharacters(in: .whitespacesAndNewlines) }
                                            .filter { $0.isEmpty == false }
                                    }
                            }
                            SettingsSheetRow("") {
                                Toggle("Add Google Meet link", isOn: $draft.addGoogleMeet)
                                    .toggleStyle(.checkbox)
                            }
                        }

                        SettingsSheetSection("Reminder") {
                            SettingsSheetRow("Remind me") {
                                Picker("", selection: reminderBinding) {
                                    Text("None").tag(-1)
                                    Text("At time of event").tag(0)
                                    Text("5 min before").tag(5)
                                    Text("10 min before").tag(10)
                                    Text("15 min before").tag(15)
                                    Text("30 min before").tag(30)
                                    Text("1 hour before").tag(60)
                                    Text("1 day before").tag(1440)
                                }
                                .labelsHidden()
                                .pickerStyle(.menu)
                            }
                        }

                        EventTemplateOutcomePreview(template: draft)

                        SettingsSheetSection("Variables") {
                            let prompts = draft.requiredPrompts()
                            if prompts.isEmpty == false {
                                Text("Prompts at instantiation: \(prompts.joined(separator: ", "))")
                                    .hcbFont(.caption)
                                    .foregroundStyle(.secondary)
                            } else {
                                Text("No {{prompt:Label}} placeholders — the template instantiates without asking for input.")
                                    .hcbFont(.caption)
                                    .foregroundStyle(.secondary)
                            }
                            Text("{{today}} {{tomorrow}} {{yesterday}} {{+Nd/-Nd/w/m/y}} {{nextWeekday:mon}} {{clipboard}} {{cursor}} {{prompt:Label}}")
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                                .fixedSize(horizontal: false, vertical: true)
                        }
                    }
                    .padding(24)
                    .frame(maxWidth: .infinity, alignment: .topLeading)
                }

                Divider()

                templateValidationSummary(eventValidationErrors)

                SettingsSheetActions(cancelTitle: "Cancel", onCancel: onCancel) {
                    Button("Save") { onSave(draft) }
                        .keyboardShortcut(.defaultAction)
                        .buttonStyle(.borderedProminent)
                        .disabled(eventValidationErrors.isEmpty == false)
                }
            }
            .navigationTitle("Event Template")
        }
        .frame(width: 760, height: 640)
        .onAppear {
            isNameFocused = true
            attendeesRaw = draft.attendees.joined(separator: "\n")
        }
    }

    // -1 = nil (no reminder); 0+ = minutes before.
    private var reminderBinding: Binding<Int> {
        Binding(
            get: { draft.reminderMinutes ?? -1 },
            set: { draft.reminderMinutes = $0 < 0 ? nil : $0 }
        )
    }

    private var writableCalendars: [CalendarListMirror] {
        model.calendars.filter { $0.accessRole == "owner" || $0.accessRole == "writer" }
    }

    private var calendarPickerBinding: Binding<String> {
        Binding(
            get: { draft.calendarIdOrTitle },
            set: { draft.calendarIdOrTitle = $0 }
        )
    }

    private var eventValidationErrors: [String] {
        var errors: [String] = []
        if draft.name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            errors.append("Name is required.")
        }
        if draft.summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            errors.append("Event title is required.")
        }
        if draft.durationMinutes < 5 {
            errors.append("Duration must be at least 5 minutes.")
        }
        let date = eventPreview.resolvedDate.trimmingCharacters(in: .whitespacesAndNewlines)
        if date.isEmpty == false
            && hasPromptPlaceholder(date) == false
            && date.range(of: "^\\d{4}-\\d{2}-\\d{2}$", options: .regularExpression) == nil {
            errors.append("Date anchor must resolve to YYYY-MM-DD or stay empty.")
        }
        let time = draft.timeAnchor.trimmingCharacters(in: .whitespacesAndNewlines)
        if draft.isAllDay == false && time.isEmpty == false {
            let pieces = time.split(separator: ":").compactMap { Int($0) }
            if pieces.count != 2 || (0...23).contains(pieces[0]) == false || (0...59).contains(pieces[1]) == false {
                errors.append("Time must use 24-hour HH:mm.")
            }
        }
        return errors
    }

    private var eventPreview: EventTemplatePreview {
        EventTemplatePreview(template: draft)
    }
}

private struct TaskTemplateOutcomePreview: View {
    let template: TaskTemplate

    var body: some View {
        let preview = TaskTemplatePreview(template: template)
        SettingsSheetSection("Outcome") {
            HStack(spacing: 10) {
                SettingsOutcomeCard(
                    systemImage: "text.cursor",
                    title: trimmed(preview.resolvedTitle).isEmpty ? "Needs title" : trimmed(preview.resolvedTitle),
                    detail: "task title"
                )
                SettingsOutcomeCard(
                    systemImage: "calendar",
                    title: trimmed(preview.resolvedDue).isEmpty ? "No due date" : trimmed(preview.resolvedDue),
                    detail: "resolved due"
                )
                SettingsOutcomeCard(
                    systemImage: promptCount == 0 ? "checkmark.circle" : "questionmark.bubble",
                    title: promptCount == 0 ? "No prompts" : "\(promptCount) prompt\(promptCount == 1 ? "" : "s")",
                    detail: trimmed(preview.resolvedList).isEmpty ? "default list" : trimmed(preview.resolvedList)
                )
            }
            TemplateResolvedPreviewRows(rows: [
                ("Title", preview.resolvedTitle),
                ("Notes", preview.resolvedNotes),
                ("Due", preview.resolvedDue),
                ("List", preview.resolvedList)
            ])
        }
    }

    private var promptCount: Int {
        template.requiredPrompts().count
    }
}

private struct EventTemplateOutcomePreview: View {
    let template: EventTemplate

    var body: some View {
        let preview = EventTemplatePreview(template: template)
        SettingsSheetSection("Outcome") {
            HStack(spacing: 10) {
                SettingsOutcomeCard(
                    systemImage: "calendar.badge.plus",
                    title: trimmed(preview.resolvedSummary).isEmpty ? "Needs title" : trimmed(preview.resolvedSummary),
                    detail: "\(template.durationMinutes) min\(template.isAllDay ? " all-day" : "")"
                )
                SettingsOutcomeCard(
                    systemImage: template.attendees.isEmpty ? "person" : "person.2",
                    title: guestTitle,
                    detail: template.addGoogleMeet ? "Google Meet included" : "no Meet link"
                )
                SettingsOutcomeCard(
                    systemImage: promptCount == 0 ? "checkmark.circle" : "questionmark.bubble",
                    title: promptCount == 0 ? "No prompts" : "\(promptCount) prompt\(promptCount == 1 ? "" : "s")",
                    detail: trimmed(preview.resolvedCalendar).isEmpty ? destinationTitle : trimmed(preview.resolvedCalendar)
                )
            }
            TemplateResolvedPreviewRows(rows: [
                ("Title", preview.resolvedSummary),
                ("Details", preview.resolvedDetails),
                ("Location", preview.resolvedLocation),
                ("Date", preview.resolvedDate.isEmpty ? "{{today}}" : preview.resolvedDate),
                ("Calendar", preview.resolvedCalendar)
            ])
        }
    }

    private var guestTitle: String {
        template.attendees.isEmpty ? "No guests" : "\(template.attendees.count) guest\(template.attendees.count == 1 ? "" : "s")"
    }

    private var destinationTitle: String {
        let destination = trimmed(template.calendarIdOrTitle)
        return destination.isEmpty ? "first writable calendar" : destination
    }

    private var promptCount: Int {
        template.requiredPrompts().count
    }
}

private struct TaskTemplatePreview {
    let resolvedTitle: String
    let resolvedNotes: String
    let resolvedDue: String
    let resolvedList: String

    init(template: TaskTemplate) {
        let context = HCBTemplateContext.previewContext(prompts: template.requiredPrompts())
        resolvedTitle = HCBTemplateExpander.expand(template.title, context: context)
        resolvedNotes = HCBTemplateExpander.expand(template.notes, context: context)
        resolvedDue = HCBTemplateExpander.expand(template.due, context: context)
        resolvedList = HCBTemplateExpander.expand(template.listIdOrTitle, context: context)
    }
}

private struct EventTemplatePreview {
    let resolvedSummary: String
    let resolvedDetails: String
    let resolvedLocation: String
    let resolvedDate: String
    let resolvedCalendar: String

    init(template: EventTemplate) {
        let context = HCBTemplateContext.previewContext(prompts: template.requiredPrompts())
        resolvedSummary = HCBTemplateExpander.expand(template.summary, context: context)
        resolvedDetails = HCBTemplateExpander.expand(template.details, context: context)
        resolvedLocation = HCBTemplateExpander.expand(template.location, context: context)
        resolvedDate = HCBTemplateExpander.expand(template.dateAnchor, context: context)
        resolvedCalendar = HCBTemplateExpander.expand(template.calendarIdOrTitle, context: context)
    }
}

private struct TemplateResolvedPreviewRows: View {
    let rows: [(String, String)]

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text("Resolved preview")
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
            ForEach(rows.filter { trimmed($0.1).isEmpty == false }, id: \.0) { label, value in
                let prompts = promptLabels(in: value)
                HStack(alignment: .top, spacing: 8) {
                    Text(label)
                        .hcbFont(.caption2, weight: .semibold)
                        .foregroundStyle(.secondary)
                        .frame(width: 58, alignment: .trailing)
                    VStack(alignment: .leading, spacing: 4) {
                        Text(value)
                            .font(.caption.monospaced())
                            .foregroundStyle(prompts.isEmpty ? .secondary : AppColor.ember)
                            .lineLimit(2)
                        if prompts.isEmpty == false {
                            VStack(alignment: .leading, spacing: 4) {
                                ForEach(prompts, id: \.self) { prompt in
                                    Text("Prompt: \(prompt)")
                                        .hcbFont(.caption2, weight: .semibold)
                                        .foregroundStyle(AppColor.ember)
                                        .lineLimit(1)
                                        .padding(.horizontal, 6)
                                        .padding(.vertical, 2)
                                        .background(Capsule().fill(AppColor.ember.opacity(0.12)))
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@ViewBuilder
private func templateValidationSummary(_ errors: [String]) -> some View {
    if errors.isEmpty == false {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
            VStack(alignment: .leading, spacing: 2) {
                ForEach(errors, id: \.self) { error in
                    Text(error)
                }
            }
            .hcbFont(.caption)
            .foregroundStyle(.orange)
            Spacer()
        }
        .padding(.horizontal, 24)
        .padding(.vertical, 10)
        .background(.bar)
    }
}

private extension HCBTemplateContext {
    static func previewContext(prompts: [String]) -> HCBTemplateContext {
        HCBTemplateContext(
            now: Date(),
            calendar: .current,
            clipboard: NSPasteboard.general.string(forType: .string),
            prompts: Dictionary(uniqueKeysWithValues: prompts.map { ($0, "{{prompt:\($0)}}") })
        )
    }
}

private func trimmed(_ value: String) -> String {
    value.trimmingCharacters(in: .whitespacesAndNewlines)
}

private func hasPromptPlaceholder(_ value: String) -> Bool {
    value.range(of: #"\{\{prompt:[^}]+\}\}"#, options: .regularExpression) != nil
}

private func promptLabels(in value: String) -> [String] {
    guard let regex = try? NSRegularExpression(pattern: #"\{\{prompt:([^}]+)\}\}"#) else {
        return []
    }
    let range = NSRange(value.startIndex..<value.endIndex, in: value)
    var labels: [String] = []
    regex.enumerateMatches(in: value, range: range) { match, _, _ in
        guard
            let match,
            let labelRange = Range(match.range(at: 1), in: value)
        else { return }
        let label = trimmed(String(value[labelRange]))
        if label.isEmpty == false, labels.contains(label) == false {
            labels.append(label)
        }
    }
    return labels
}
