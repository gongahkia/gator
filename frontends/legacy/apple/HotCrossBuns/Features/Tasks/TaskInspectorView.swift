import AppKit
import SwiftUI

struct TaskDraft: Equatable {
    var title: String
    var notes: String
    var hasDueDate: Bool
    var dueDate: Date

    init(task: TaskMirror, fallbackDueDate: Date = Date()) {
        self.title = task.title
        self.notes = task.notes
        self.hasDueDate = task.dueDate != nil
        self.dueDate = task.dueDate ?? fallbackDueDate
    }

    func differs(from task: TaskMirror) -> Bool {
        // Trim BOTH sides. Asymmetric trim (draft trimmed, task raw) incorrectly
        // flagged any task whose Google-stored title/notes has incidental leading
        // or trailing whitespace as "dirty" on mount — causing ghost "Edited X"
        // toasts to fire on inspector close or on any subsequent sync refresh
        // even when the user never typed a character.
        let trimmedDraftTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedDraftNotes = notes.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedTaskTitle = task.title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedTaskNotes = HCBTextMarkup.markdownSource(from: task.notes).trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmedDraftTitle != trimmedTaskTitle { return true }
        if trimmedDraftNotes != trimmedTaskNotes { return true }
        let currentDue = task.dueDate
        if hasDueDate == false && currentDue != nil { return true }
        if hasDueDate && currentDue == nil { return true }
        if hasDueDate, let currentDue {
            let calendar = Calendar.current
            if calendar.startOfDay(for: dueDate) != calendar.startOfDay(for: currentDue) { return true }
        }
        return false
    }

    func resolvedDueDate() -> Date? {
        guard hasDueDate else { return nil }
        return Calendar.current.startOfDay(for: dueDate)
    }

    var hasUsableTitle: Bool {
        title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
    }
}

struct TaskInspectorView: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    @Environment(\.routerPath) private var router
    let task: TaskMirror
    let close: () -> Void
    // Optional. When non-nil, the duplicate banner's "Jump to sibling" menu
    // reassigns selection via this callback. Tabs that don't want the jump
    // affordance can pass nil; the banner falls back to a disabled menu item.
    var jumpToTask: ((TaskMirror.ID) -> Void)? = nil

    @State private var draft: TaskDraft
    @State private var saveTask: Task<Void, Never>?
    @State private var isConfirmingDelete = false
    @State private var isSavingNow = false
    @State private var savedAt: Date?
    @State private var saveFailureMessage: String?
    @State private var snoozeCustomTask: TaskMirror?
    // View-only-first flow (mirrors AddEventSheet / AddTaskSheet). Clicking
    // a task in the list opens a read-only surface; Edit flips to the live
    // auto-saving form below. Reset to view-only whenever the selected task
    // changes so a subsequent pick doesn't inherit an open edit session.
    @State private var isEditing: Bool = false
    @FocusState private var focusedField: Field?

    private enum Field: Hashable { case title, notes }
    private var taskStore: TaskStore { model.taskStore }

    init(
        task: TaskMirror,
        close: @escaping () -> Void,
        jumpToTask: ((TaskMirror.ID) -> Void)? = nil
    ) {
        self.task = task
        self.close = close
        self.jumpToTask = jumpToTask
        _draft = State(initialValue: TaskDraft(task: task))
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                header
                duplicateBanner

                if isEditing {
                    TextField("Task title", text: $draft.title, axis: .vertical)
                        .textFieldStyle(.plain)
                        .hcbFont(.title3, weight: .semibold)
                        .foregroundStyle(AppColor.ink)
                        .focused($focusedField, equals: .title)
                        .onChange(of: draft.title) { _, _ in scheduleSave() }

                    VStack(alignment: .leading, spacing: 6) {
                        sectionLabel("NOTES")
                        MarkdownEditor(text: $draft.notes, placeholder: "Notes (markdown supported)", minHeight: 120, maxHeight: 240)
                            .onChange(of: draft.notes) { _, _ in scheduleSave() }
                    }

                    dueDateSection
                    taskListSection
                    subtasksSection
                    hierarchyControls
                    actionButtons
                    inlineErrorBanner
                    metadataSection
                } else {
                    viewOnlyBody
                }
            }
            .hcbScaledPadding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .background(AppColor.cream.opacity(0.35))
        .hcbSurface(.inspector) // §6.11 per-surface font override
        .focusedSceneValue(\.taskInspectorCommandActions, taskInspectorCommandActions)
        .onChange(of: task.id) { _, _ in
            commitPending()
            draft = TaskDraft(task: task)
            savedAt = nil
            isEditing = false
        }
        .onChange(of: task.title) { _, _ in refreshDraftIfClean() }
        .onChange(of: task.notes) { _, _ in refreshDraftIfClean() }
        .onChange(of: task.dueDate) { _, _ in refreshDraftIfClean() }
        .onDisappear { commitPending() }
        .confirmationDialog(
            "Delete this task?",
            isPresented: $isConfirmingDelete,
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                Task { await delete() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("This deletes the task from Google Tasks. It cannot be undone.")
        }
        .sheet(item: $snoozeCustomTask) { selectedTask in
            SnoozePickerSheet(task: selectedTask) { newDate in
                Task { await snooze(selectedTask, to: newDate) }
            }
        }
    }

    private var taskInspectorCommandActions: TaskInspectorCommandActions {
        TaskInspectorCommandActions(
            saveAndClose: {
                commitPending()
                close()
            },
            toggleCompletion: {
                Task { await toggleCompletion() }
            },
            delete: {
                isConfirmingDelete = true
            },
            duplicate: {
                Task { _ = await model.duplicateTask(task) }
            }
        )
    }

    // MARK: - View-only body (renders populated fields only).

    private var viewOnlyBody: some View {
        let strippedNotes = task.notes
        let listTitle = taskStore.taskListTitle(for: task.taskListID)
        let subtasks = children

        return VStack(alignment: .leading, spacing: 14) {
            Text(task.title.isEmpty ? "Untitled" : task.title)
                .hcbFont(.title3, weight: .semibold)
                .foregroundStyle(AppColor.ink)
                .textSelection(.enabled)

            if strippedNotes.isEmpty == false {
                readCard("NOTES") {
                    MarkdownBlock(source: strippedNotes)
                        .hcbFont(.body)
                        .foregroundStyle(AppColor.ink)
                        .textSelection(.enabled)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }

            if let due = task.dueDate {
                readCard("DUE DATE") {
                    Label(due.formatted(.dateTime.weekday(.wide).day().month(.wide).year()),
                          systemImage: "calendar")
                        .hcbFont(.subheadline)
                        .foregroundStyle(AppColor.ink)
                }
            }

            readCard("LIST") {
                Label(listTitle, systemImage: "list.bullet")
                    .hcbFont(.subheadline)
                    .foregroundStyle(AppColor.ink)
            }

            if subtasks.isEmpty == false {
                readCard("SUBTASKS") {
                    ForEach(subtasks) { child in
                        HStack(spacing: 8) {
                            Image(systemName: child.isCompleted ? "checkmark.circle.fill" : "circle")
                                .foregroundStyle(child.isCompleted ? AppColor.moss : AppColor.ember)
                            Text(child.title)
                                .hcbFont(.subheadline)
                                .strikethrough(child.isCompleted, color: .secondary)
                                .foregroundStyle(child.isCompleted ? .secondary : AppColor.ink)
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .contextMenu {
            TaskContextMenu(
                task: task,
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
    }

    @ViewBuilder
    private func readCard<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            sectionLabel(title)
            content()
        }
        .hcbScaledPadding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 14)
                .fill(.ultraThinMaterial)
        )
    }

    @ViewBuilder
    private var duplicateBanner: some View {
        if let groupKey = taskStore.duplicateGroupKey(for: task.id) {
            let siblings = taskStore.duplicateSiblings(of: task.id)
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    Image(systemName: "exclamationmark.2")
                        .foregroundStyle(AppColor.ember)
                    Text(siblings.count == 1
                        ? "Possible duplicate of 1 other item"
                        : "Possible duplicate of \(siblings.count) other items")
                        .hcbFont(.subheadline, weight: .semibold)
                    Spacer(minLength: 0)
                }
                Text("Matched by exact title and notes. Jump to a sibling to compare, delete this one, or dismiss if it's a false positive.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                HStack(spacing: 8) {
                    Menu {
                        if siblings.isEmpty {
                            Button("No siblings") {}.disabled(true)
                        } else if jumpToTask == nil {
                            Button("Jump not available here") {}.disabled(true)
                        } else {
                            ForEach(siblings, id: \.id) { sibling in
                                Button(sibling.title.isEmpty ? "Untitled" : sibling.title) {
                                    jumpToTask?(sibling.id)
                                }
                            }
                        }
                    } label: {
                        Label("Jump to sibling", systemImage: "arrow.turn.up.right")
                            .frame(maxWidth: .infinity)
                    }
                    .menuStyle(.borderlessButton)
                    .buttonStyle(.bordered)

                    Button(role: .destructive) {
                        isConfirmingDelete = true
                    } label: {
                        Label("Delete this", systemImage: "trash")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)

                    Button {
                        model.dismissDuplicateGroup(groupKey)
                    } label: {
                        Label("Dismiss", systemImage: "checkmark.shield")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)

                    Button {
                        openWindow(id: "duplicate-review")
                    } label: {
                        Label("Review all", systemImage: "square.stack.3d.up")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.bordered)
                }
            }
            .hcbScaledPadding(14)
            .background(
                RoundedRectangle(cornerRadius: 14)
                    .fill(AppColor.ember.opacity(0.08))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 14)
                    .strokeBorder(AppColor.ember.opacity(0.35), lineWidth: 0.8)
            )
        }
    }

    private var header: some View {
        HStack(spacing: 10) {
            statusDot
            Text(statusLabel)
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
            Spacer(minLength: 0)
            if isEditing, isSavingNow {
                ProgressView().controlSize(.small)
            } else if isEditing, let savedAt {
                Text("Saved \(savedAt.formatted(.relative(presentation: .numeric)))")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
            if isEditing == false {
                Button("Edit") {
                    HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.12)) {
                        draft.notes = HCBTextMarkup.markdownSource(from: draft.notes)
                        isEditing = true
                    }
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .accessibilityLabel("Edit task")
            }
            Button {
                commitPending()
                close()
            } label: {
                Image(systemName: "xmark")
                    .hcbFontSystem(size: 11, weight: .semibold)
                    .hcbScaledPadding(6)
                    .background(Circle().fill(AppColor.cream))
            }
            .buttonStyle(.plain)
            .keyboardShortcut(.cancelAction)
            .accessibilityLabel("Close task")
        }
    }

    private var statusDot: some View {
        Circle()
            .fill(task.isCompleted ? AppColor.moss : AppColor.ember)
            .hcbScaledFrame(width: 10, height: 10)
    }

    private var statusLabel: String {
        if task.isCompleted {
            if let completedAt = task.completedAt {
                return "Completed \(completedAt.formatted(date: .abbreviated, time: .omitted))"
            }
            return "Completed"
        }
        if let due = task.dueDate {
            let days = Calendar.current.dateComponents([.day], from: Calendar.current.startOfDay(for: Date()), to: Calendar.current.startOfDay(for: due)).day ?? 0
            if days < 0 { return "Overdue by \(-days) day\(days == -1 ? "" : "s")" }
            if days == 0 { return "Due today" }
            if days == 1 { return "Due tomorrow" }
            return "Due in \(days) days"
        }
        return "No due date"
    }

    private var dueDateSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            sectionLabel("DUE DATE")
            Toggle(isOn: $draft.hasDueDate) {
                Text(draft.hasDueDate ? draft.dueDate.formatted(date: .complete, time: .omitted) : "No due date")
                    .hcbFont(.subheadline)
            }
            .toggleStyle(.switch)
            .onChange(of: draft.hasDueDate) { _, _ in scheduleSave() }

            if draft.hasDueDate {
                DatePicker("", selection: $draft.dueDate, displayedComponents: [.date])
                    .datePickerStyle(.graphical)
                    .labelsHidden()
                    .onChange(of: draft.dueDate) { _, _ in scheduleSave() }
            }

            Text("Google Tasks stores dates only; reminder times must be set as calendar events.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
        .hcbScaledPadding(14)
        .background(
            RoundedRectangle(cornerRadius: 14)
                .fill(.ultraThinMaterial)
        )
    }

    @State private var pendingMoveListID: TaskListMirror.ID?

    private var taskListSection: some View {
        HStack(spacing: 10) {
            Image(systemName: "list.bullet")
                .foregroundStyle(.secondary)
            VStack(alignment: .leading, spacing: 2) {
                Text("LIST")
                    .hcbFont(.caption2, weight: .bold)
                    .foregroundStyle(.secondary)
                Picker("Task list", selection: Binding(
                    get: { task.taskListID },
                    set: { newID in
                        if newID != task.taskListID { pendingMoveListID = newID }
                    }
                )) {
                    ForEach(taskStore.taskLists) { list in
                        Text(list.title).tag(list.id)
                    }
                }
                .labelsHidden()
                .pickerStyle(.menu)
            }
            Spacer(minLength: 0)
        }
        .hcbScaledPadding(14)
        .background(
            RoundedRectangle(cornerRadius: 14)
                .fill(.ultraThinMaterial)
        )
        .confirmationDialog(
            "Move task to another list?",
            isPresented: Binding(
                get: { pendingMoveListID != nil },
                set: { if $0 == false { pendingMoveListID = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let newID = pendingMoveListID,
               let target = taskStore.taskList(id: newID) {
                Button("Move to \(target.title)") {
                    Task {
                        _ = await model.moveTaskToList(task, toTaskListID: newID)
                        pendingMoveListID = nil
                    }
                }
            }
            Button("Cancel", role: .cancel) { pendingMoveListID = nil }
        } message: {
            Text("Google Tasks doesn't support moving between lists natively. The task will be recreated in the new list and the old copy deleted. Its Google ID will change.")
        }
    }

    private var currentListTitle: String {
        taskStore.taskListTitle(for: task.taskListID)
    }

    private var actionButtons: some View {
        VStack(spacing: 8) {
            Button {
                Task { await toggleCompletion() }
            } label: {
                Label(
                    task.isCompleted ? "Mark as Needs Action" : "Mark Complete",
                    systemImage: task.isCompleted ? "arrow.uturn.backward.circle" : "checkmark.circle.fill"
                )
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(task.isCompleted ? AppColor.blue : AppColor.moss)

            Button(role: .destructive) {
                isConfirmingDelete = true
            } label: {
                Label("Delete Task", systemImage: "trash")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)

            Button {
                Task { _ = await model.duplicateTask(task) }
            } label: {
                Label("Duplicate", systemImage: "plus.square.on.square")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)

            Menu {
                Button {
                    router?.present(.convertTaskToEvent(task.id))
                } label: {
                    Label("Convert to Event…", systemImage: "calendar.badge.plus")
                }
                if task.dueDate == nil {
                    Button {
                        router?.present(.convertNoteToTask(task.id))
                    } label: {
                        Label("Convert to Task (set due date)…", systemImage: "calendar")
                    }
                } else {
                    Button {
                        router?.present(.convertTaskToNote(task.id))
                    } label: {
                        Label("Convert to Note (clear due date)", systemImage: "note.text")
                    }
                }
            } label: {
                Label("Convert…", systemImage: "arrow.triangle.swap")
                    .frame(maxWidth: .infinity)
            }
            .menuStyle(.borderlessButton)
            .buttonStyle(.bordered)

            Button {
                copyAsMarkdown()
            } label: {
                Label("Copy as Markdown", systemImage: "doc.on.clipboard")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
        }
        .disabled(taskStore.isMutating)
    }

    private func copyAsMarkdown() {
        let listTitle = taskStore.taskListTitle(for: task.taskListID, fallback: "")
        let markdown = TaskMarkdownExporter.markdown(for: task, taskListTitle: listTitle)
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(markdown, forType: .string)
    }

    private var children: [TaskMirror] {
        taskStore.children(of: task.id)
    }

    private var isSubtask: Bool { task.parentID != nil }

    @State private var newSubtaskTitle: String = ""
    @State private var isAddingSubtask: Bool = false

    @ViewBuilder
    private var subtasksSection: some View {
        if isSubtask {
            EmptyView()
        } else {
            VStack(alignment: .leading, spacing: 10) {
                sectionLabel("SUBTASKS")
                ForEach(Array(children.enumerated()), id: \.element.id) { index, child in
                    HStack(spacing: 10) {
                        Image(systemName: "line.3.horizontal")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                            .help("Drag to reorder")
                        Button {
                            Task { await model.setTaskCompleted(!child.isCompleted, task: child) }
                        } label: {
                            Image(systemName: child.isCompleted ? "checkmark.circle.fill" : "circle")
                                .foregroundStyle(child.isCompleted ? AppColor.moss : AppColor.ember)
                        }
                        .buttonStyle(.plain)
                        Text(child.title)
                            .hcbFont(.subheadline)
                            .strikethrough(child.isCompleted, color: .secondary)
                            .foregroundStyle(child.isCompleted ? .secondary : AppColor.ink)
                        Spacer(minLength: 0)
                        Button(role: .destructive) {
                            Task { await model.deleteTask(child) }
                        } label: {
                            Image(systemName: "xmark.circle")
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                    }
                    .hcbScaledPadding(.vertical, 2)
                    .contentShape(Rectangle())
                    .draggable(SubtaskDragPayload(taskID: child.id))
                    .dropDestination(for: SubtaskDragPayload.self) { items, _ in
                        guard let drop = items.first, drop.taskID != child.id else { return false }
                        Task { await handleSubtaskDrop(draggedID: drop.taskID, targetIndex: index) }
                        return true
                    }
                }

                HStack(spacing: 8) {
                    Image(systemName: "plus.circle")
                        .foregroundStyle(AppColor.ember)
                    TextField("Add a subtask", text: $newSubtaskTitle)
                        .textFieldStyle(.plain)
                        .onSubmit { Task { await addSubtask() } }
                    if isAddingSubtask { ProgressView().controlSize(.small) }
                }
                .hcbScaledPadding(10)
                .background(
                    RoundedRectangle(cornerRadius: 10)
                        .fill(AppColor.cream.opacity(0.5))
                )
            }
            .hcbScaledPadding(14)
            .background(
                RoundedRectangle(cornerRadius: 14)
                    .fill(.ultraThinMaterial)
            )
        }
    }

    @ViewBuilder
    private var hierarchyControls: some View {
        HStack(spacing: 8) {
            if taskStore.canIndent(task) {
                Button {
                    Task { await model.indentTask(task) }
                } label: {
                    Label("Indent", systemImage: "increase.indent")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .keyboardShortcut(.tab, modifiers: [])
            }
            if TaskHierarchy.canOutdent(task) {
                Button {
                    Task { await model.outdentTask(task) }
                } label: {
                    Label("Outdent", systemImage: "decrease.indent")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .keyboardShortcut(.tab, modifiers: [.shift])
            }
        }
        .disabled(taskStore.isMutating)
    }

    private func addSubtask() async {
        let trimmed = newSubtaskTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return }
        isAddingSubtask = true
        let created = await model.createTask(
            title: trimmed,
            notes: "",
            dueDate: nil,
            taskListID: task.taskListID,
            parentID: task.id
        )
        isAddingSubtask = false
        if created { newSubtaskTitle = "" }
    }

    // Task recurrence removed — Google Tasks API has no native recurrence
    // field, so we no longer write one into notes. Event recurrence still
    // works (native Google Calendar field).

    private func handleSubtaskDrop(draggedID: TaskMirror.ID, targetIndex: Int) async {
        let ordered = children
        guard let draggedTask = ordered.first(where: { $0.id == draggedID }) else { return }
        guard let fromIndex = ordered.firstIndex(where: { $0.id == draggedID }) else { return }
        if fromIndex == targetIndex { return }
        // Google's move API expects the "previous sibling" ID. When the target
        // is above the dragged row's original spot, the previous sibling is the
        // row at targetIndex - 1 (or nil when dropping to the top).
        let adjustedPreviousID: TaskMirror.ID? = {
            let insertionIndex = targetIndex
            guard insertionIndex > 0 else { return nil }
            let predecessorIndex = insertionIndex - 1
            let predecessor = ordered[predecessorIndex]
            if predecessor.id == draggedTask.id {
                // Shouldn't happen (we guard above), but keep safe
                return predecessorIndex > 0 ? ordered[predecessorIndex - 1].id : nil
            }
            return predecessor.id
        }()
        _ = await model.reorderTask(draggedTask, afterSiblingID: adjustedPreviousID)
    }

    @ViewBuilder
    private var inlineErrorBanner: some View {
        if let saveFailureMessage {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(AppColor.ember)
                VStack(alignment: .leading, spacing: 3) {
                    Text("Couldn't save")
                        .hcbFont(.caption, weight: .semibold)
                    Text(saveFailureMessage)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 0)
                Button {
                    self.saveFailureMessage = nil
                    Task { await saveIfDirty() }
                } label: {
                    Text("Retry")
                        .hcbFont(.caption, weight: .semibold)
                }
                .buttonStyle(.bordered)
            }
            .hcbScaledPadding(12)
            .background(
                RoundedRectangle(cornerRadius: 12)
                    .fill(AppColor.ember.opacity(0.1))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .strokeBorder(AppColor.ember.opacity(0.4), lineWidth: 0.8)
            )
        }
    }

    private var metadataSection: some View {
        VStack(alignment: .leading, spacing: 6) {
            sectionLabel("GOOGLE ID")
            Text(task.id)
                .font(.caption.monospaced())
                .textSelection(.enabled)
                .foregroundStyle(.secondary)
            if let updated = task.updatedAt {
                Text("Updated \(updated.formatted(date: .abbreviated, time: .shortened))")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .hcbScaledPadding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var notesForPreview: String {
        draft.notes
    }

    private func sectionLabel(_ text: String) -> some View {
        Text(text)
            .hcbFont(.caption2, weight: .bold)
            .foregroundStyle(.secondary)
    }

    private func scheduleSave() {
        saveTask?.cancel()
        guard draft.hasUsableTitle else { return }
        saveTask = Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(600))
            guard Task.isCancelled == false else { return }
            await saveIfDirty()
        }
    }

    private func commitPending() {
        // Cancel the debounce. We must fire-and-forget here because
        // .onDisappear is synchronous — but thanks to the reference-
        // counted mutationCount in AppModel, an overlapping save from
        // this detached Task no longer races with a concurrent mutation
        // elsewhere to prematurely clear isMutating. saveIfDirty itself
        // checks `draft.differs` / `hasUsableTitle` before hitting the
        // network, so late-arriving drafts don't re-submit a stale one.
        saveTask?.cancel()
        saveTask = nil
        guard draft.differs(from: task), draft.hasUsableTitle else { return }
        let snapshot = draft
        let targetTask = task
        Task { @MainActor in
            _ = await model.updateTask(
                targetTask,
                title: snapshot.title,
                notes: snapshot.notes,
                dueDate: snapshot.resolvedDueDate()
            )
        }
    }

    private func refreshDraftIfClean() {
        guard draft.differs(from: task) == false else { return }
        draft = TaskDraft(task: task)
    }

    private func snooze(_ task: TaskMirror, to newDate: Date?) async {
        _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: newDate)
    }

    private func saveIfDirty() async {
        guard draft.differs(from: task) else { return }
        guard draft.hasUsableTitle else { return }
        isSavingNow = true
        let editedNotes = draft.notes.trimmingCharacters(in: .whitespacesAndNewlines)
        let didSave = await model.updateTask(
            task,
            title: draft.title.trimmingCharacters(in: .whitespacesAndNewlines),
            notes: editedNotes,
            dueDate: draft.resolvedDueDate()
        )
        isSavingNow = false
        if didSave {
            savedAt = Date()
            saveFailureMessage = nil
        } else {
            saveFailureMessage = taskStore.lastMutationError ?? "Save failed."
        }
    }

    private func toggleCompletion() async {
        commitPending()
        _ = await model.setTaskCompleted(!task.isCompleted, task: task)
    }

    private func delete() async {
        // Close the inspector IMMEDIATELY. model.deleteTask strips the task
        // from the mirror optimistically; while the Google API call is in
        // flight (can be 5-15s) the inspector's `model.task(id:)` lookup
        // returns nil and falls to TaskInspectorEmptyState — a momentary
        // "Select a task" flash. Same pattern as event delete.
        saveTask?.cancel()
        close()
        _ = await model.deleteTask(task)
    }
}

struct TaskInspectorEmptyState: View {
    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "text.magnifyingglass")
                .hcbFontSystem(size: 36)
                .foregroundStyle(.tertiary)
            Text("Select a task")
                .hcbFont(.headline)
            Text("Pick a task from the list to view and edit its details.")
                .hcbFont(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .hcbScaledPadding(.horizontal, 30)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        // Keep the empty inspector from reintroducing the hidden main-window title.
        .background(AppColor.cream.opacity(0.25))
    }
}
