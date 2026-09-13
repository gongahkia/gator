import SwiftUI

struct EventHoverPreview: View {
    @Environment(AppModel.self) private var model
    let event: CalendarEventMirror

    private var calendarStore: CalendarStore { model.calendarStore }

    private var hydratedEvent: CalendarEventMirror {
        calendarStore.event(id: event.id) ?? event
    }

    var body: some View {
        let event = hydratedEvent
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                RoundedRectangle(cornerRadius: 3)
                    .fill(accent)
                    .hcbScaledFrame(width: 4, height: 20)
                Text(event.summary)
                    .hcbFont(.headline)
                    .lineLimit(2)
            }
            VStack(alignment: .leading, spacing: 4) {
                Label(timeLabel, systemImage: "clock")
                    .hcbFont(.subheadline)
                if let cal = calendarStore.calendars.first(where: { $0.id == event.calendarID }) {
                    Label(cal.summary, systemImage: "calendar")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                if event.location.isEmpty == false {
                    Label(event.location, systemImage: "mappin.and.ellipse")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
            }
            if event.details.isEmpty == false {
                Divider()
                Text(event.details)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(6)
            }
        }
        .hcbScaledPadding(12)
        .hcbScaledFrame(minWidth: 260, idealWidth: 300, maxWidth: 420, alignment: .leading)
        .fixedSize(horizontal: false, vertical: true)
    }

    private var accent: Color {
        let event = hydratedEvent
        if let hex = CalendarEventColor.from(colorId: event.colorId).hex {
            return Color(hex: hex)
        }
        if let cal = calendarStore.calendars.first(where: { $0.id == event.calendarID }) {
            return Color(hex: cal.colorHex)
        }
        return AppColor.blue
    }

    private var timeLabel: String {
        let event = hydratedEvent
        if event.isAllDay {
            return "All day · \(event.startDate.formatted(.dateTime.weekday(.wide).month(.abbreviated).day()))"
        }
        let sameDay = Calendar.current.isDate(event.startDate, inSameDayAs: event.endDate)
        if sameDay {
            return "\(event.startDate.formatted(.dateTime.weekday(.abbreviated).month(.abbreviated).day().hour().minute())) – \(event.endDate.formatted(.dateTime.hour().minute()))"
        }
        return "\(event.startDate.formatted(.dateTime.month(.abbreviated).day().hour().minute())) – \(event.endDate.formatted(.dateTime.month(.abbreviated).day().hour().minute()))"
    }
}

// Popover shown on click for events in day/week/month grids. Includes an
// "Open" button to escalate to the full detail view. Agenda view keeps its
// existing row-tap-navigates behavior.
struct CalendarEventPreviewButton<Label: View>: View {
    @Environment(\.routerPath) private var router
    @Environment(AppModel.self) private var model
    let event: CalendarEventMirror
    @ViewBuilder let label: () -> Label
    @State private var isPresented = false
    @State private var isConfirmingDelete = false

    private var calendarStore: CalendarStore { model.calendarStore }

    private var currentEvent: CalendarEventMirror {
        calendarStore.event(id: event.id) ?? event
    }

    var body: some View {
        Button {
            isPresented = true
        } label: {
            label()
        }
        .buttonStyle(.plain)
        .contextMenu {
            EventContextMenu(
                event: currentEvent,
                onOpen: { router?.present(.editEvent(event.id)) },
                onConvertToTask: { router?.present(.convertEventToTask(event.id)) },
                onConvertToNote: { router?.present(.convertEventToNote(event.id)) },
                onDelete: { isConfirmingDelete = true }
            )
        }
        .confirmationDialog(
            "Delete \"\(event.summary)\"?",
            isPresented: $isConfirmingDelete,
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                Task { _ = await model.deleteEvent(currentEvent) }
            }
            Button("Cancel", role: .cancel) {}
        }
        .popover(isPresented: $isPresented, arrowEdge: .trailing) {
            CalendarPreviewPopoverShell(kind: .event, contentKey: event.id) {
                VStack(alignment: .leading, spacing: 10) {
                    EventHoverPreview(event: currentEvent)
                    HStack(spacing: 8) {
                        // Left-aligned circle button mirrors the task completion
                        // affordance. Hidden on read-only calendars.
                        CalendarEventDismissButton(event: currentEvent, size: 16)
                        Spacer(minLength: 0)
                        Button("Open") {
                            isPresented = false
                            router?.present(.editEvent(event.id))
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AppColor.ember)
                    }
                    .hcbScaledPadding(.horizontal, 12)
                    .hcbScaledPadding(.bottom, 10)
                }
            }
        }
    }
}

struct CalendarTaskPreviewButton<Label: View>: View {
    @Environment(\.routerPath) private var router
    @Environment(AppModel.self) private var model
    let task: TaskMirror
    @ViewBuilder let label: () -> Label
    @State private var isPresented = false
    @State private var isConfirmingDelete = false
    @State private var snoozeCustomTask: TaskMirror?

    private var calendarStore: CalendarStore { model.calendarStore }

    private var listName: String {
        calendarStore.taskListTitle(for: task.taskListID)
    }

    var body: some View {
        Button {
            isPresented = true
        } label: {
            label()
        }
        .buttonStyle(.plain)
        .contextMenu {
            TaskContextMenu(
                task: task,
                onOpen: { router?.present(.editTask(task.id)) },
                onCustomSnooze: { snoozeCustomTask = task },
                onConvertToEvent: { router?.present(.convertTaskToEvent(task.id)) },
                onConvertToTaskOrNote: {
                    if task.dueDate == nil {
                        router?.present(.convertNoteToTask(task.id))
                    } else {
                        router?.present(.convertTaskToNote(task.id))
                    }
                },
                onDelete: { isConfirmingDelete = true }
            )
        }
        .confirmationDialog(
            "Delete \"\(task.title)\"?",
            isPresented: $isConfirmingDelete,
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                Task { _ = await model.deleteTask(task) }
            }
            Button("Cancel", role: .cancel) {}
        }
        .sheet(item: $snoozeCustomTask) { selectedTask in
            SnoozePickerSheet(task: selectedTask) { newDate in
                Task {
                    _ = await model.updateTask(
                        selectedTask,
                        title: selectedTask.title,
                        notes: selectedTask.notes,
                        dueDate: newDate
                    )
                }
            }
        }
        .popover(isPresented: $isPresented, arrowEdge: .trailing) {
            CalendarPreviewPopoverShell(kind: .task, contentKey: task.id) {
                VStack(alignment: .leading, spacing: 10) {
                    TaskHoverPreview(task: task)
                    HStack {
                        Spacer(minLength: 0)
                        Button("Open") {
                            isPresented = false
                            router?.present(.editTask(task.id))
                        }
                        .buttonStyle(.borderedProminent)
                        .tint(AppColor.ember)
                    }
                    .hcbScaledPadding(.horizontal, 12)
                    .hcbScaledPadding(.bottom, 10)
                }
            }
        }
    }
}

private struct CalendarPreviewPopoverShell<Content: View>: View {
    let kind: CalendarPreviewPopoverKind
    let contentKey: String
    @ViewBuilder let content: () -> Content

    var body: some View {
        content()
    }
}

private enum CalendarPreviewPopoverKind {
    case event
    case task

    var title: String {
        switch self {
        case .event: "Opening event"
        case .task: "Opening task"
        }
    }

    var systemImage: String {
        switch self {
        case .event: "calendar"
        case .task: "checklist"
        }
    }
}

private struct CalendarPreviewPopoverPlaceholder: View {
    let kind: CalendarPreviewPopoverKind

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: kind.systemImage)
                    .foregroundStyle(AppColor.ember)
                    .hcbScaledFrame(width: 18)
                Text(kind.title)
                    .hcbFont(.headline)
                    .foregroundStyle(AppColor.ink)
                Spacer(minLength: 16)
                ProgressView()
                    .controlSize(.small)
            }

            placeholderLine(widthScale: 0.86)
            placeholderLine(widthScale: 0.62)
            Divider()
            placeholderLine(widthScale: 0.72)
            HStack {
                Spacer(minLength: 0)
                RoundedRectangle(cornerRadius: 7, style: .continuous)
                    .fill(AppColor.cardSurface.opacity(0.68))
                    .hcbScaledFrame(width: 72, height: 28)
            }
        }
        .hcbScaledPadding(12)
        .hcbScaledFrame(width: 300, alignment: .leading)
        .background(.regularMaterial)
        .accessibilityLabel("\(kind.title), preparing preview")
    }

    private func placeholderLine(widthScale: CGFloat) -> some View {
        GeometryReader { proxy in
            RoundedRectangle(cornerRadius: 5, style: .continuous)
                .fill(AppColor.cardSurface.opacity(0.72))
                .frame(width: proxy.size.width * widthScale, height: 10)
        }
        .frame(height: 10)
        .redacted(reason: .placeholder)
    }
}

// Clickable circle-checkmark on calendar event tiles. Google Calendar has
// no completion concept, so clicking this deletes the event on Google —
// but the history log + undo toast record it as .eventDismissed so the
// user sees "Marked 'X' as done" rather than a delete, and Undo recreates
// the event from the snapshot. Hidden when the event's calendar is not
// writeable by the user (reader / freeBusyReader access role).
struct CalendarEventDismissButton: View {
    @Environment(AppModel.self) private var model
    let event: CalendarEventMirror
    var size: CGFloat = 10

    private var calendarStore: CalendarStore { model.calendarStore }

    private var currentEvent: CalendarEventMirror {
        calendarStore.event(id: event.id) ?? event
    }

    private var canWrite: Bool {
        guard let calendar = calendarStore.calendars.first(where: { $0.id == currentEvent.calendarID }) else { return false }
        return calendar.accessRole == "owner" || calendar.accessRole == "writer"
    }

    var body: some View {
        if canWrite {
            Button {
                let event = currentEvent
                Task { _ = await model.dismissEvent(event) }
            } label: {
                Image(systemName: "circle")
                    .hcbFontSystem(size: size)
                    .foregroundStyle(AppColor.blue)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .help("Mark event as done (⌘K)")
            .keyboardShortcut("k", modifiers: [.command])
        }
    }
}

// §7.01 Phase D1 — clickable checkbox on calendar task tiles. Lets users
// complete / reopen a task without opening the preview or sheet. Routes
// through the same setTaskCompleted path as list-mode completion.
struct CalendarTaskCheckbox: View {
    @Environment(AppModel.self) private var model
    let task: TaskMirror
    var size: CGFloat = 10

    private var calendarStore: CalendarStore { model.calendarStore }

    private var currentTask: TaskMirror {
        calendarStore.task(id: task.id) ?? task
    }

    var body: some View {
        Button {
            let task = currentTask
            Task { await model.setTaskCompleted(!task.isCompleted, task: task) }
        } label: {
            Image(systemName: currentTask.isCompleted ? "checkmark.circle.fill" : "circle")
                .hcbFontSystem(size: size)
                .foregroundStyle(currentTask.isCompleted ? AppColor.moss : AppColor.ember)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help(currentTask.isCompleted ? "Reopen task" : "Complete task")
    }
}

struct EventHoverPreviewModifier: ViewModifier {
    let event: CalendarEventMirror
    var isEnabled = true
    @State private var showPreview = false
    @State private var hoverTask: Task<Void, Never>?

    @ViewBuilder
    func body(content: Content) -> some View {
        if isEnabled {
            content
                .onHover { hovering in
                    hoverTask?.cancel()
                    if hovering {
                        hoverTask = Task {
                            try? await Task.sleep(for: .milliseconds(600))
                            guard Task.isCancelled == false else { return }
                            showPreview = true
                        }
                    } else {
                        showPreview = false
                    }
                }
                .popover(isPresented: $showPreview, arrowEdge: .trailing) {
                    EventHoverPreview(event: event)
                }
        } else {
            content
                .onAppear {
                    hoverTask?.cancel()
                    showPreview = false
                }
        }
    }
}
