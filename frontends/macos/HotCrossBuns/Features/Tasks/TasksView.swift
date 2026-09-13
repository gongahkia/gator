import SwiftUI

struct AddTaskSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model
    @State private var selectedTaskListID: TaskListMirror.ID?
    @State private var title = ""
    @State private var notes = ""
    @State private var hasDueDate = false
    @State private var dueDate = Date()
    @State private var isSaving = false
    @State private var isCreatingList = false
    @State private var newListTitle = ""
    // Non-nil when editing an existing task — flips title + primary button,
    // dispatches updateTask instead of createTask. Mirrors the Event sheet's
    // dual-mode pattern so the same "New … / Edit …" visual serves both paths.
    @State private var editingTask: TaskMirror?
    @State private var isConfirmingDelete = false
    // View-only / edit toggle: create mode is always live; edit-mode starts
    // view-only (per user-requested Open flow) and flips true on Edit.
    @State private var isEditing: Bool

    init() {
        _isEditing = State(initialValue: true)
    }

    init(existingTask: TaskMirror) {
        _title = State(initialValue: existingTask.title)
        _notes = State(initialValue: existingTask.notes)
        _hasDueDate = State(initialValue: existingTask.dueDate != nil)
        _dueDate = State(initialValue: existingTask.dueDate ?? Date())
        _selectedTaskListID = State(initialValue: existingTask.taskListID)
        _editingTask = State(initialValue: existingTask)
        _isEditing = State(initialValue: false)
    }

    var body: some View {
        NavigationStack {
            Group {
                if model.taskLists.isEmpty {
                    ContentUnavailableView(
                        "No task lists loaded",
                        systemImage: "checklist",
                        description: Text("Connect Google and refresh before creating a task.")
                    )
                } else if editingTask != nil, isEditing == false {
                    // Read-only card — only populated fields; notes render as
                    // markdown. Edit button in the toolbar flips into the
                    // Form-based editor below.
                    ScrollView {
                        viewOnlyBody
                            .hcbScaledPadding(18)
                    }
                } else {
                    Form {
                        Section("Task") {
                            TextField("Title", text: $title)
                            MarkdownEditor(text: $notes, placeholder: "Notes (markdown supported)", minHeight: 90, maxHeight: 200)
                        }

                        Section("Destination") {
                            taskListMenu
                        }

                        Section("Due date") {
                            Toggle("Set due date", isOn: $hasDueDate)
                            if hasDueDate {
                                DatePicker("Due", selection: $dueDate, displayedComponents: [.date])
                            }
                        }

                    }
                    .formStyle(.grouped)
                    .hcbScaledPadding(.horizontal, 4)
                }
            }
            .navigationTitle(navTitle)
            .task {
                selectedTaskListID = selectedTaskListID ?? model.taskLists.first?.id
                applyDeepLinkPrefillIfAny()
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") {
                        dismiss()
                    }
                    .disabled(isSaving)
                }

                // Delete on the view-only pass sits alongside Cancel / Edit so
                // users can remove a task without going through the edit
                // overflow menu. Tap routes through the existing confirmation
                // dialog so the destructive action stays behind a deliberate
                // second click.
                if editingTask != nil, isEditing == false {
                    ToolbarItem(placement: .destructiveAction) {
                        Button(role: .destructive) {
                            isConfirmingDelete = true
                        } label: {
                            Text("Delete")
                        }
                        .disabled(isSaving)
                    }
                }

                ToolbarItem(placement: .confirmationAction) {
                    if editingTask != nil, isEditing == false {
                        Button("Edit") {
                            HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.12)) {
                                isEditing = true
                            }
                        }
                    } else {
                        Button(editingTask == nil ? "Create" : "Save") {
                            Task { await createOrUpdateTask() }
                        }
                        .disabled(canCreate == false || isSaving)
                    }
                }

                // Overflow only surfaces in active-edit mode so view-only
                // can't accidentally Complete/Delete via a half-click.
                if let existing = editingTask, isEditing {
                    ToolbarItem(placement: .primaryAction) {
                        Menu {
                            Button {
                                Task { await toggleCompletion(for: existing) }
                            } label: {
                                Label(
                                    existing.isCompleted ? "Mark Needs Action" : "Mark Complete",
                                    systemImage: existing.isCompleted
                                        ? "arrow.uturn.backward.circle"
                                        : "checkmark.circle.fill"
                                )
                            }
                            Divider()
                            Button(role: .destructive) {
                                isConfirmingDelete = true
                            } label: {
                                Label("Delete Task", systemImage: "trash")
                            }
                        } label: {
                            Label("More", systemImage: "ellipsis.circle")
                        }
                        .disabled(isSaving)
                    }
                }
            }
            .confirmationDialog(
                "Delete this task?",
                isPresented: $isConfirmingDelete,
                titleVisibility: .visible,
                presenting: editingTask
            ) { task in
                Button("Delete Task", role: .destructive) {
                    Task { await deleteTaskAndDismiss(task) }
                }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text("This deletes the task from Google Tasks.")
            }
            .sheet(isPresented: $isCreatingList) {
                NewTaskListInlineSheet(
                    title: $newListTitle,
                    onCancel: {
                        isCreatingList = false
                        newListTitle = ""
                    },
                    onCreate: {
                        Task { await createListInline() }
                    }
                )
            }
        }
        .hcbScaledFrame(minWidth: 520, idealWidth: 560, minHeight: 520, idealHeight: 640)
        .interactiveDismissDisabled(isSaving)
    }

    private var taskListMenu: some View {
        HStack {
            Text("Task list")
            Spacer(minLength: 12)
            Menu {
                ForEach(model.taskLists) { taskList in
                    Button {
                        selectedTaskListID = taskList.id
                    } label: {
                        HStack {
                            Text(taskList.title)
                            if selectedTaskListID == taskList.id {
                                Spacer()
                                Image(systemName: "checkmark")
                            }
                        }
                    }
                }
                Divider()
                Button {
                    newListTitle = ""
                    isCreatingList = true
                } label: {
                    Label("Create new list…", systemImage: "plus")
                }
                .disabled(model.account == nil)
            } label: {
                HStack(spacing: 6) {
                    Text(selectedListTitle)
                        .lineLimit(1)
                    Image(systemName: "chevron.up.chevron.down")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            .menuStyle(.borderlessButton)
            .fixedSize()
        }
    }

    private var selectedListTitle: String {
        guard let id = selectedTaskListID else {
            return "Select list"
        }
        return model.taskListTitle(for: id, fallback: "Select list")
    }

    private func createListInline() async {
        let trimmed = newListTitle.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return }
        isSaving = true
        defer { isSaving = false }
        let didCreate = await model.createTaskList(title: trimmed)
        if didCreate {
            // Wait a beat for the model to ingest the new list, then select it.
            if let match = model.taskLists.first(where: { $0.title == trimmed }) {
                selectedTaskListID = match.id
            }
            isCreatingList = false
            newListTitle = ""
        }
    }

    private var canCreate: Bool {
        title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
            && selectedTaskListID != nil
            && model.account != nil
    }

    private func createOrUpdateTask() async {
        if let existing = editingTask {
            await updateExistingTask(existing)
        } else {
            await createTask()
        }
    }

    private func createTask() async {
        guard let selectedTaskListID else {
            return
        }

        isSaving = true
        defer { isSaving = false }

        let didCreate = await model.createTask(
            title: title,
            notes: notes,
            dueDate: hasDueDate ? Calendar.current.startOfDay(for: dueDate) : nil,
            taskListID: selectedTaskListID
        )

        if didCreate {
            dismiss()
        }
    }

    private func updateExistingTask(_ existing: TaskMirror) async {
        isSaving = true
        defer { isSaving = false }
        let didUpdate = await model.updateTask(
            existing,
            title: title,
            notes: notes,
            dueDate: hasDueDate ? Calendar.current.startOfDay(for: dueDate) : nil
        )
        // Move list separately if the user changed it — updateTask doesn't
        // relocate across lists, so we fire moveToList after the edit lands.
        if didUpdate,
           let targetListID = selectedTaskListID,
           targetListID != existing.taskListID,
           let refreshed = model.task(id: existing.id) {
            _ = await model.moveTaskToList(refreshed, toTaskListID: targetListID)
        }
        if didUpdate {
            dismiss()
        }
    }

    private func deleteTaskAndDismiss(_ task: TaskMirror) async {
        isSaving = true
        defer { isSaving = false }
        if await model.deleteTask(task) {
            dismiss()
        }
    }

    private func toggleCompletion(for task: TaskMirror) async {
        isSaving = true
        defer { isSaving = false }
        _ = await model.setTaskCompleted(!task.isCompleted, task: task)
    }

    private var navTitle: String {
        guard editingTask != nil else { return "New Task" }
        return isEditing ? "Edit Task" : "Task"
    }

    private func applyDeepLinkPrefillIfAny() {
        // hotcrossbuns://new/task?… stages a prefill struct on AppModel. The
        // sheet consumes it here and nils it so a subsequent plain New Task
        // (⌘N) doesn't inherit stale values.
        guard let prefill = model.pendingTaskPrefill else { return }
        defer { model.pendingTaskPrefill = nil }

        if let t = prefill.title, t.isEmpty == false, title.isEmpty {
            title = t
        }
        // Tags round-trip through the task title as #foo (TagExtractor format).
        if prefill.tags.isEmpty == false {
            let hashed = prefill.tags
                .filter { $0.isEmpty == false }
                .map { "#\($0)" }
                .joined(separator: " ")
            title = title.isEmpty ? hashed : "\(title) \(hashed)"
        }
        if let n = prefill.notes, n.isEmpty == false, notes.isEmpty {
            notes = n
        }
        if let due = prefill.dueDate {
            hasDueDate = true
            dueDate = due
        }
        if let listRef = prefill.listIdOrTitle, listRef.isEmpty == false,
           let match = resolveTaskList(listRef) {
            selectedTaskListID = match.id
        }
    }

    private func resolveTaskList(_ ref: String) -> TaskListMirror? {
        if let exact = model.taskLists.first(where: { $0.id == ref }) { return exact }
        return model.taskLists.first(where: { $0.title.localizedCaseInsensitiveCompare(ref) == .orderedSame })
    }

    // MARK: - View-only card (read-only surface for the Open flow).

    private var viewOnlyBody: some View {
        VStack(alignment: .leading, spacing: 14) {
            readTaskCard
            if notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                readNotesCard
            }
            readListCard
            if hasDueDate { readDueCard }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }

    @ViewBuilder
    private func readCard<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title.uppercased())
                .hcbFont(.caption2, weight: .bold)
                .foregroundStyle(.secondary)
            content()
        }
        .hcbScaledPadding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 12)
                .fill(AppColor.cream.opacity(0.5))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.6)
        )
    }

    private var readTaskCard: some View {
        readCard("Task") {
            Text(title.isEmpty ? "Untitled" : title)
                .hcbFont(.title3, weight: .semibold)
                .foregroundStyle(AppColor.ink)
                .textSelection(.enabled)
        }
    }

    private var readNotesCard: some View {
        readCard("Notes") {
            MarkdownBlock(source: notes)
                .hcbFont(.body)
                .foregroundStyle(AppColor.ink)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var readListCard: some View {
        readCard("List") {
            Label(selectedListTitle, systemImage: "tray")
                .hcbFont(.subheadline)
                .foregroundStyle(AppColor.ink)
        }
    }

    private var readDueCard: some View {
        readCard("Due") {
            Label(dueDate.formatted(.dateTime.weekday(.wide).day().month(.wide).year()), systemImage: "calendar")
                .hcbFont(.subheadline)
                .foregroundStyle(AppColor.ink)
        }
    }

}

private struct NewTaskListInlineSheet: View {
    @Binding var title: String
    let onCancel: () -> Void
    let onCreate: () -> Void
    @FocusState private var focused: Bool

    private var trimmed: String {
        title.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    var body: some View {
        // Compact popover body — matches ListCreateSheet. Apple Reminders /
        // Finder rename-tag idiom, no nested NavigationStack inside a popover.
        VStack(alignment: .leading, spacing: 12) {
            Text("New Task List")
                .font(.headline)
            TextField("Name", text: $title)
                .textFieldStyle(.roundedBorder)
                .focused($focused)
                .onSubmit {
                    if trimmed.isEmpty == false { onCreate() }
                }
            HStack(spacing: 8) {
                Spacer(minLength: 0)
                Button("Cancel", action: onCancel)
                    .keyboardShortcut(.cancelAction)
                Button("Create", action: onCreate)
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
                    .disabled(trimmed.isEmpty)
            }
        }
        .padding(16)
        .frame(width: 280)
        .onAppear { focused = true }
    }
}

struct DetailField: View {
    let label: String
    let value: String

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(label.uppercased())
                .hcbFont(.caption, weight: .bold)
                .foregroundStyle(.secondary)
            Text(value)
                .font(.body.monospaced())
                .textSelection(.enabled)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .cardSurface(cornerRadius: 18)
    }
}
