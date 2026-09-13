import SwiftUI

// Past cleanup surface — controls visibility/deletion of past events,
// overdue-open tasks, and completed tasks. Deletion modes gate behind
// a blast-radius modal that shows the current dry-run count before the
// setting is committed. A manual "Run now" button issues the same
// preview + confirmation loop.
struct PastCleanupSection: View {
    @Environment(AppModel.self) private var model
    @State private var pendingConfirmation: PendingConfirmation?
    @State private var runNowPreview: PastCleanupPreview?
    @State private var lastRunSummary: String?

    var body: some View {
        Section("Past cleanup") {
            sectionIntro
            eventBlock
            taskBlock
            manualRunBlock
            if let lastRunSummary {
                Text(lastRunSummary)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .sheet(item: $pendingConfirmation) { confirmation in
            BlastRadiusConfirmSheet(
                confirmation: confirmation,
                onCancel: { pendingConfirmation = nil },
                onAcknowledge: { alsoRun in
                    apply(confirmation)
                    if alsoRun, confirmation.runImmediately {
                        Task { await runCleanupNow() }
                    }
                    pendingConfirmation = nil
                }
            )
            .environment(model)
        }
    }

    // MARK: - Intro

    private var sectionIntro: some View {
        Text("Visibility rules are local. Deletion modes issue Google API deletes — events go to Google Calendar's 30-day web trash, tasks are tombstoned and not user-recoverable.")
            .hcbFont(.caption)
            .foregroundStyle(.secondary)
    }

    // MARK: - Events

    @ViewBuilder
    private var eventBlock: some View {
        Picker("Past events", selection: eventBehaviorBinding) {
            ForEach(PastEventBehavior.allCases, id: \.self) { mode in
                Text(mode.title).tag(mode)
            }
        }
        .pickerStyle(.menu)
        Text(model.settings.pastEventBehavior.subtitle)
            .hcbFont(.caption2)
            .foregroundStyle(.secondary)
        if model.settings.pastEventBehavior.isDeletion {
            Picker("Delete older than", selection: eventThresholdBinding) {
                ForEach([1, 7, 30, 90, 180, 365], id: \.self) { days in
                    Text(thresholdLabel(days: days)).tag(days)
                }
            }
            .pickerStyle(.menu)
            Toggle("Allow deleting events with attendees", isOn: attendeeOptInBinding)
            if model.settings.allowDeletingAttendeeEvents {
                Text("Deleting an event with other attendees may remove it from their calendars and send cancellations. Proceed carefully.")
                    .hcbFont(.caption2)
                    .foregroundStyle(AppColor.ember)
            }
        }
    }

    // MARK: - Tasks

    @ViewBuilder
    private var taskBlock: some View {
        Picker("Overdue tasks", selection: overdueBehaviorBinding) {
            ForEach(OverdueTaskBehavior.allCases, id: \.self) { mode in
                Text(mode.title).tag(mode)
            }
        }
        .pickerStyle(.menu)
        Text(model.settings.overdueTaskBehavior.subtitle)
            .hcbFont(.caption2)
            .foregroundStyle(.secondary)

        Picker("Completed tasks", selection: completedBehaviorBinding) {
            ForEach(CompletedTaskBehavior.allCases, id: \.self) { mode in
                Text(mode.title).tag(mode)
            }
        }
        .pickerStyle(.menu)
        Text(model.settings.completedTaskBehavior.subtitle)
            .hcbFont(.caption2)
            .foregroundStyle(.secondary)
        if model.settings.completedTaskBehavior.isDeletion {
            Picker("Delete older than", selection: taskThresholdBinding) {
                ForEach([1, 7, 30, 90, 180, 365], id: \.self) { days in
                    Text(thresholdLabel(days: days)).tag(days)
                }
            }
            .pickerStyle(.menu)
        }
    }

    private var manualRunBlock: some View {
        HStack {
            Button {
                Task { await prepareRunNow() }
            } label: {
                Label("Run cleanup now", systemImage: "arrow.trianglehead.2.clockwise.rotate.90")
            }
            .disabled(cleanupEnabled == false)
            Spacer()
        }
    }

    private var cleanupEnabled: Bool {
        model.settings.pastEventBehavior.isDeletion || model.settings.completedTaskBehavior.isDeletion
    }

    private func thresholdLabel(days: Int) -> String {
        switch days {
        case 1: "1 day"
        case 7: "7 days"
        case 30: "30 days"
        case 90: "90 days"
        case 180: "180 days"
        case 365: "1 year"
        default: "\(days) days"
        }
    }

    // MARK: - Bindings

    // Event behavior pickers: when the user selects .delete, we queue a
    // confirmation sheet BEFORE saving the new mode. The sheet's OK
    // button is what commits the change. Cancel leaves settings intact.
    private var eventBehaviorBinding: Binding<PastEventBehavior> {
        Binding(
            get: { model.settings.pastEventBehavior },
            set: { newValue in
                if newValue.isDeletion, model.settings.pastEventBehavior.isDeletion == false {
                    // Toggling ON a deletion mode — gate behind the modal.
                    pendingConfirmation = PendingConfirmation(kind: .eventDeletion, targetMode: newValue)
                } else {
                    model.setPastEventBehavior(newValue)
                }
            }
        )
    }

    private var eventThresholdBinding: Binding<Int> {
        Binding(
            get: { model.settings.pastEventDeleteThresholdDays },
            set: { model.setPastEventDeleteThresholdDays($0) }
        )
    }

    private var attendeeOptInBinding: Binding<Bool> {
        Binding(
            get: { model.settings.allowDeletingAttendeeEvents },
            set: { newValue in
                if newValue, model.settings.allowDeletingAttendeeEvents == false {
                    pendingConfirmation = PendingConfirmation(kind: .attendeeDeletion, targetMode: nil)
                } else {
                    model.setAllowDeletingAttendeeEvents(newValue)
                }
            }
        )
    }

    private var overdueBehaviorBinding: Binding<OverdueTaskBehavior> {
        Binding(
            get: { model.settings.overdueTaskBehavior },
            set: { model.setOverdueTaskBehavior($0) }
        )
    }

    private var completedBehaviorBinding: Binding<CompletedTaskBehavior> {
        Binding(
            get: { model.settings.completedTaskBehavior },
            set: { newValue in
                if newValue.isDeletion, model.settings.completedTaskBehavior.isDeletion == false {
                    pendingConfirmation = PendingConfirmation(kind: .taskDeletion, targetMode: nil)
                } else {
                    model.setCompletedTaskBehavior(newValue)
                }
            }
        )
    }

    private var taskThresholdBinding: Binding<Int> {
        Binding(
            get: { model.settings.completedTaskDeleteThresholdDays },
            set: { model.setCompletedTaskDeleteThresholdDays($0) }
        )
    }

    // MARK: - Apply + run

    private func apply(_ confirmation: PendingConfirmation) {
        switch confirmation.kind {
        case .eventDeletion:
            if let mode = confirmation.targetMode {
                model.setPastEventBehavior(mode)
            }
            model.acknowledgeEventDeletion()
        case .attendeeDeletion:
            model.setAllowDeletingAttendeeEvents(true)
            model.acknowledgeAttendeeDeletion()
        case .taskDeletion:
            model.setCompletedTaskBehavior(.delete)
            model.acknowledgeTaskDeletion()
        case .runNow:
            break
        }
    }

    private func prepareRunNow() async {
        let preview = model.pastCleanupCoordinator.currentPreview()
        if preview.isEmpty {
            lastRunSummary = "Nothing to clean up."
            return
        }
        pendingConfirmation = PendingConfirmation(kind: .runNow, targetMode: nil, cachedPreview: preview)
    }

    private func runCleanupNow() async {
        let preview = pendingConfirmation?.cachedPreview ?? model.pastCleanupCoordinator.currentPreview()
        let result = await model.pastCleanupCoordinator.execute(preview)
        lastRunSummary = "Last run: \(result.eventsDeleted) event\(result.eventsDeleted == 1 ? "" : "s"), \(result.tasksDeleted) task\(result.tasksDeleted == 1 ? "" : "s") deleted."
    }
}

// MARK: - Pending confirmation

private struct PendingConfirmation: Identifiable {
    enum Kind {
        case eventDeletion
        case attendeeDeletion
        case taskDeletion
        case runNow
    }

    let kind: Kind
    let targetMode: PastEventBehavior?
    var cachedPreview: PastCleanupPreview? = nil

    var id: String {
        switch kind {
        case .eventDeletion: "eventDeletion"
        case .attendeeDeletion: "attendeeDeletion"
        case .taskDeletion: "taskDeletion"
        case .runNow: "runNow"
        }
    }

    var runImmediately: Bool { kind == .runNow }

    var title: String {
        switch kind {
        case .eventDeletion: "Delete past events on Google?"
        case .attendeeDeletion: "Allow deleting events with attendees?"
        case .taskDeletion: "Delete completed tasks on Google?"
        case .runNow: "Run cleanup now?"
        }
    }

    var body: String {
        switch kind {
        case .eventDeletion:
            return "Hot Cross Buns will issue events.delete on Google for past events older than your threshold. Events go to Google Calendar's 30-day web trash. Recurring series masters are never touched — only their past instances. You can restore within 30 days via calendar.google.com → Trash."
        case .attendeeDeletion:
            return "Deleting an event with attendees removes it from their calendars and Google sends cancellation emails. Leave this off unless you organize a lot of meetings and want historical ones gone for everyone."
        case .taskDeletion:
            return "Hot Cross Buns will issue tasks.delete on Google for completed tasks older than your threshold. Tombstones are internal-only — Google Tasks web will show them gone, with no user-facing restore path."
        case .runNow:
            return "The dry run above is what will be deleted. This cannot be undone for tasks; events can be restored from Google Calendar's web trash within 30 days."
        }
    }
}

private struct BlastRadiusConfirmSheet: View {
    @Environment(AppModel.self) private var model
    let confirmation: PendingConfirmation
    let onCancel: () -> Void
    let onAcknowledge: (Bool) -> Void

    private var preview: PastCleanupPreview {
        confirmation.cachedPreview ?? model.pastCleanupCoordinator.currentPreview()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            Label(confirmation.title, systemImage: "exclamationmark.triangle.fill")
                .hcbFont(.title3, weight: .semibold)
                .foregroundStyle(AppColor.ember)
            Text(confirmation.body)
                .hcbFont(.body)
                .foregroundStyle(AppColor.ink)
            Divider()
            previewSection
            Spacer(minLength: 0)
            HStack {
                Button("Cancel", role: .cancel, action: onCancel)
                    .keyboardShortcut(.cancelAction)
                Spacer()
                if confirmation.runImmediately {
                    Button("Delete Now") { onAcknowledge(true) }
                        .buttonStyle(.borderedProminent)
                        .tint(AppColor.ember)
                        .keyboardShortcut(.return)
                } else {
                    Button("Enable (don't run yet)") { onAcknowledge(false) }
                    Button("Enable and run now") { onAcknowledge(true) }
                        .buttonStyle(.borderedProminent)
                        .tint(AppColor.ember)
                        .keyboardShortcut(.return)
                }
            }
        }
        .hcbScaledPadding(22)
        .hcbScaledFrame(minWidth: 520, minHeight: 360)
    }

    @ViewBuilder
    private var previewSection: some View {
        if preview.isEmpty {
            Text("Nothing currently matches the rules — enabling this won't delete anything today, but future items will be cleaned up as they age past the threshold.")
                .hcbFont(.callout)
                .foregroundStyle(.secondary)
        } else {
            VStack(alignment: .leading, spacing: 8) {
                Text("This will delete on Google:")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                if preview.events.isEmpty == false {
                    Text("• \(preview.events.count) event\(preview.events.count == 1 ? "" : "s")")
                }
                if preview.completedTasks.isEmpty == false {
                    Text("• \(preview.completedTasks.count) completed task\(preview.completedTasks.count == 1 ? "" : "s")")
                }
                if preview.attendeeEventsSkipped.isEmpty == false {
                    Text("Skipping \(preview.attendeeEventsSkipped.count) attendee-event\(preview.attendeeEventsSkipped.count == 1 ? "" : "s") (attendee deletion off).")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                if preview.recurringMastersSkipped.isEmpty == false {
                    Text("Skipping \(preview.recurringMastersSkipped.count) recurring series master\(preview.recurringMastersSkipped.count == 1 ? "" : "s") — series stay alive.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }
}
