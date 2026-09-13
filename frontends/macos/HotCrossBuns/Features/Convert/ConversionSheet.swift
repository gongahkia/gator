import SwiftUI

// Direction-agnostic confirmation sheet for the six conversion flows.
// Callers pass an `Intent` that carries the source + target kind; the
// sheet reads HCB's list/calendar pools off AppModel to populate its
// pickers and drives the ConversionService on confirm.
enum ConversionIntent: Equatable {
    case eventToTask(CalendarEventMirror)
    case eventToNote(CalendarEventMirror)
    case taskToEvent(TaskMirror)
    case taskToNote(TaskMirror)
    case noteToTask(TaskMirror)
    case noteToEvent(TaskMirror)

    var sheetTitle: String {
        switch self {
        case .eventToTask: "Convert Event to Task"
        case .eventToNote: "Convert Event to Note"
        case .taskToEvent: "Convert Task to Event"
        case .taskToNote: "Convert Task to Note"
        case .noteToTask: "Convert Note to Task"
        case .noteToEvent: "Convert Note to Event"
        }
    }
}

struct ConversionSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model
    let intent: ConversionIntent

    @State private var selectedListID: TaskListMirror.ID?
    @State private var selectedCalendarID: CalendarListMirror.ID?
    @State private var startDate: Date = Date()
    @State private var endDate: Date = Date().addingTimeInterval(3600)
    @State private var isAllDay: Bool = true
    @State private var dueDate: Date = Date()
    @State private var isConverting: Bool = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            header
            Divider()
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    mappingPreview
                    targetPicker
                    datePrompts
                    lostFields
                    if let errorMessage {
                        Text(errorMessage)
                            .hcbFont(.footnote)
                            .foregroundStyle(AppColor.ember)
                    }
                }
                .hcbScaledPadding(.horizontal, 20)
            }
            Divider()
            footerButtons
        }
        .hcbScaledFrame(minWidth: 520, minHeight: 420)
        .onAppear { initializeDefaults() }
    }

    // MARK: - Sections

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: "arrow.triangle.swap")
                .foregroundStyle(AppColor.ember)
            Text(intent.sheetTitle)
                .hcbFont(.headline)
            Spacer()
        }
        .hcbScaledPadding(16)
    }

    @ViewBuilder
    private var mappingPreview: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Mapping")
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
            switch intent {
            case .eventToTask(let event), .eventToNote(let event):
                Text("Title: \(event.summary)")
                if event.details.isEmpty == false {
                    Text("Notes: \(event.details)").hcbFont(.caption).foregroundStyle(.secondary)
                }
                if event.location.isEmpty == false {
                    Text("Notes include location: \(event.location)").hcbFont(.caption).foregroundStyle(.secondary)
                }
            case .taskToEvent(let task), .noteToEvent(let task):
                Text("Summary: \(task.title)")
                if task.notes.isEmpty == false {
                    Text("Details: \(task.notes)").hcbFont(.caption).foregroundStyle(.secondary)
                }
            case .taskToNote(let task), .noteToTask(let task):
                Text("Title: \(task.title)")
                if task.notes.isEmpty == false {
                    Text("Notes: \(task.notes)").hcbFont(.caption).foregroundStyle(.secondary)
                }
            }
        }
    }

    @ViewBuilder
    private var targetPicker: some View {
        switch intent {
        case .eventToTask, .eventToNote:
            Picker("Target list", selection: Binding(
                get: { selectedListID ?? model.taskLists.first?.id ?? "" },
                set: { selectedListID = $0 }
            )) {
                ForEach(model.taskLists) { list in
                    Text(list.title).tag(list.id)
                }
            }
            .pickerStyle(.menu)
        case .taskToEvent, .noteToEvent:
            Picker("Target calendar", selection: Binding(
                get: { selectedCalendarID ?? model.calendars.first?.id ?? "" },
                set: { selectedCalendarID = $0 }
            )) {
                ForEach(model.calendars) { c in
                    Text(c.summary).tag(c.id)
                }
            }
            .pickerStyle(.menu)
        case .taskToNote, .noteToTask:
            EmptyView()
        }
    }

    @ViewBuilder
    private var datePrompts: some View {
        switch intent {
        case .taskToEvent(let task):
            // Task has a dueDate; seed the event at that date, all-day
            // by default since Google Tasks has no time.
            Toggle("All-day event", isOn: $isAllDay)
            DatePicker("Start", selection: $startDate, displayedComponents: isAllDay ? [.date] : [.date, .hourAndMinute])
            if isAllDay == false {
                DatePicker("End", selection: $endDate, in: startDate..., displayedComponents: [.date, .hourAndMinute])
            }
            Text(task.dueDate != nil ? "Seeded from due date." : "Task has no due date — pick one for the event.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        case .noteToEvent:
            Toggle("All-day event", isOn: $isAllDay)
            DatePicker("Start", selection: $startDate, displayedComponents: isAllDay ? [.date] : [.date, .hourAndMinute])
            if isAllDay == false {
                DatePicker("End", selection: $endDate, in: startDate..., displayedComponents: [.date, .hourAndMinute])
            }
        case .noteToTask:
            DatePicker("Due date", selection: $dueDate, displayedComponents: [.date])
        case .eventToTask, .eventToNote, .taskToNote:
            EmptyView()
        }
    }

    @ViewBuilder
    private var lostFields: some View {
        let items = lostFieldList
        if items.isEmpty == false {
            VStack(alignment: .leading, spacing: 4) {
                Text("Will be lost on Google")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                ForEach(items, id: \.self) { field in
                    Text("• \(field)")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private var lostFieldList: [String] {
        switch intent {
        case .eventToTask(let event):
            ConversionMapper.lostFieldsForEventToTask(event, preserveDue: true)
        case .eventToNote(let event):
            ConversionMapper.lostFieldsForEventToTask(event, preserveDue: false)
        case .taskToEvent(let task):
            ConversionMapper.lostFieldsForTaskToEvent(task, hasDueDate: task.dueDate != nil)
        case .noteToEvent(let note):
            ConversionMapper.lostFieldsForTaskToEvent(note, hasDueDate: false)
        case .taskToNote, .noteToTask:
            []
        }
    }

    private var footerButtons: some View {
        HStack {
            Button("Cancel", role: .cancel) { dismiss() }
                .keyboardShortcut(.cancelAction)
            Spacer()
            Button(action: { Task { await performConversion() } }) {
                if isConverting {
                    ProgressView().controlSize(.small)
                } else {
                    Text("Convert")
                }
            }
            .buttonStyle(.borderedProminent)
            .tint(AppColor.ember)
            .keyboardShortcut(.return)
            .disabled(canConvert == false)
        }
        .hcbScaledPadding(16)
    }

    // MARK: - Defaults + validation

    private var canConvert: Bool {
        guard isConverting == false else { return false }
        switch intent {
        case .eventToTask, .eventToNote:
            return (selectedListID ?? model.taskLists.first?.id) != nil
        case .taskToEvent, .noteToEvent:
            return (selectedCalendarID ?? model.calendars.first?.id) != nil
        case .taskToNote, .noteToTask:
            return true
        }
    }

    private func initializeDefaults() {
        switch intent {
        case .eventToTask, .eventToNote:
            if selectedListID == nil { selectedListID = model.taskLists.first?.id }
        case .taskToEvent(let task):
            if selectedCalendarID == nil { selectedCalendarID = model.calendarSnapshot.selectedCalendars.first?.id ?? model.calendars.first?.id }
            if let due = task.dueDate {
                startDate = Calendar.current.startOfDay(for: due)
                endDate = ConversionMapper.eventEnd(fromTaskStart: startDate, isAllDay: true)
            }
            isAllDay = true
        case .noteToEvent:
            if selectedCalendarID == nil { selectedCalendarID = model.calendarSnapshot.selectedCalendars.first?.id ?? model.calendars.first?.id }
            startDate = Calendar.current.startOfDay(for: Date())
            endDate = ConversionMapper.eventEnd(fromTaskStart: startDate, isAllDay: true)
            isAllDay = true
        case .noteToTask:
            dueDate = Calendar.current.startOfDay(for: Date())
        case .taskToNote:
            break
        }
    }

    private func performConversion() async {
        isConverting = true
        defer { isConverting = false }
        let service = ConversionService(model: model)
        let result: ConversionResult
        switch intent {
        case .eventToTask(let event):
            guard let listID = selectedListID ?? model.taskLists.first?.id else { return }
            result = await service.convertEvent(event, toTaskListID: listID, keepDueDate: true)
        case .eventToNote(let event):
            guard let listID = selectedListID ?? model.taskLists.first?.id else { return }
            result = await service.convertEvent(event, toTaskListID: listID, keepDueDate: false)
        case .taskToEvent(let task):
            guard let calID = selectedCalendarID ?? model.calendars.first?.id else { return }
            let finalEnd = isAllDay ? ConversionMapper.eventEnd(fromTaskStart: startDate, isAllDay: true) : endDate
            result = await service.convertTaskToEvent(task, calendarID: calID, startDate: startDate, endDate: finalEnd, isAllDay: isAllDay)
        case .noteToEvent(let note):
            guard let calID = selectedCalendarID ?? model.calendars.first?.id else { return }
            let finalEnd = isAllDay ? ConversionMapper.eventEnd(fromTaskStart: startDate, isAllDay: true) : endDate
            result = await service.convertTaskToEvent(note, calendarID: calID, startDate: startDate, endDate: finalEnd, isAllDay: isAllDay)
        case .taskToNote(let task):
            result = await service.convertTaskToNote(task)
        case .noteToTask(let note):
            result = await service.convertNoteToTask(note, dueDate: dueDate)
        }
        if result.isSuccess {
            dismiss()
        } else {
            errorMessage = result.userFacingMessage ?? "Conversion failed."
        }
    }
}
