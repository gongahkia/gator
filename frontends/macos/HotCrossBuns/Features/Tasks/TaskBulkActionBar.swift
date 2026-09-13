import SwiftUI

// Floating bulk-action bar for the Store tab, parallel to EventBulkActionBar.
// Appears when >1 task is selected. Buttons queue BulkTaskOperations which the
// optimizer in AppModel coalesces/dedupes before dispatching to Google.
//
// Data-safety notes:
//  - Every action goes through AppModel.performBulkTaskOperations, which
//    routes each op through the existing optimistic-write helpers — offline
//    queue, etag conflict handling, undo snapshots all apply unchanged.
//  - Delete requires an explicit confirmation dialog (matches EventBulkActionBar).
//  - Partial failures return in BulkTaskExecutionResult and surface via the
//    `onFinished` callback so the caller can render a concrete toast.
struct TaskBulkActionBar: View {
    @Environment(AppModel.self) private var model
    @Binding var selection: Set<TaskMirror.ID>
    let tasks: [TaskMirror]
    // Called once a batch finishes dispatching. Always fires — even on empty /
    // all-dropped batches — so callers can show a "nothing to do" toast instead
    // of leaving the user wondering.
    var onFinished: (BulkTaskExecutionResult) -> Void

    @State private var isConfirmingDelete = false
    @State private var isMutating = false
    @State private var tagInputMode: BulkTagMode?
    @State private var datePickerMode: BulkDatePickerMode?
    private var taskStore: TaskStore { model.taskStore }

    var body: some View {
        HStack(spacing: 12) {
            Text("\(tasks.count) task\(tasks.count == 1 ? "" : "s")")
                .hcbFont(.subheadline, weight: .semibold)

            Divider().hcbScaledFrame(height: 20)

            completionMenu
            rescheduleMenu
            moveMenu
            tagMenu
            Divider().hcbScaledFrame(height: 20)
            Button(role: .destructive) {
                isConfirmingDelete = true
            } label: {
                Label("Delete", systemImage: "trash")
            }
            .disabled(isMutating)
            .help("Delete selected tasks (requires confirmation)")

            Button {
                selection.removeAll()
            } label: {
                Label("Clear", systemImage: "xmark.circle")
            }
            .disabled(isMutating)
            .help("Clear selection")
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 10)
        .background(.thinMaterial, in: Capsule())
        .overlay(Capsule().strokeBorder(AppColor.cardStroke, lineWidth: 0.6))
        .shadow(radius: 6, y: 2)
        .confirmationDialog(
            "Delete \(tasks.count) tasks?",
            isPresented: $isConfirmingDelete,
            titleVisibility: .visible
        ) {
            Button("Delete \(tasks.count)", role: .destructive) {
                Task { await runBulk(tasks.map { .delete(taskId: $0.id) }) }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Removes these tasks from Google Tasks. Subtasks of deleted tasks are deleted too. This can't be undone from here.")
        }
        .sheet(item: $tagInputMode) { mode in
            TagInputSheet(mode: mode, tasks: tasks) { tag in
                tagInputMode = nil
                let ops: [BulkTaskOperation] = tasks.map {
                    mode == .add ? .addTag(taskId: $0.id, tag: tag) : .removeTag(taskId: $0.id, tag: tag)
                }
                Task { await runBulk(ops) }
            } onCancel: {
                tagInputMode = nil
            }
        }
        .sheet(item: $datePickerMode) { _ in
            CustomDueDateSheet { newDate in
                datePickerMode = nil
                let ops: [BulkTaskOperation] = tasks.map { .setDue(taskId: $0.id, dueDate: newDate) }
                Task { await runBulk(ops) }
            } onCancel: {
                datePickerMode = nil
            }
        }
    }

    // MARK: subviews

    private var completionMenu: some View {
        Menu {
            Button("Mark complete") {
                Task { await runBulk(tasks.map { .complete(taskId: $0.id) }) }
            }
            .disabled(tasks.allSatisfy(\.isCompleted))
            Button("Reopen") {
                Task { await runBulk(tasks.map { .reopen(taskId: $0.id) }) }
            }
            .disabled(tasks.allSatisfy { $0.isCompleted == false })
        } label: {
            Label("Status", systemImage: "checkmark.circle")
        }
        .disabled(isMutating)
    }

    private var rescheduleMenu: some View {
        Menu {
            Button("Today") { applyDate(daysFromToday: 0) }
            Button("Tomorrow") { applyDate(daysFromToday: 1) }
            Button("In 2 days") { applyDate(daysFromToday: 2) }
            Button("Next week") { applyDate(daysFromToday: 7) }
            Divider()
            Button("Pick a date…") { datePickerMode = .customDue }
            Divider()
            Button("Clear due date", role: .destructive) {
                Task { await runBulk(tasks.map { .setDue(taskId: $0.id, dueDate: nil) }) }
            }
            .disabled(tasks.allSatisfy { $0.dueDate == nil })
        } label: {
            Label("Reschedule", systemImage: "calendar.badge.clock")
        }
        .disabled(isMutating)
    }

    private var moveMenu: some View {
        Menu {
            if taskStore.taskLists.isEmpty {
                Button("No lists available") {}.disabled(true)
            } else {
                ForEach(taskStore.taskLists) { list in
                    Button(list.title) {
                        Task { await runBulk(tasks.map { .moveToList(taskId: $0.id, targetListId: list.id) }) }
                    }
                }
            }
        } label: {
            Label("Move", systemImage: "arrow.right.circle")
        }
        .disabled(isMutating || taskStore.taskLists.isEmpty)
    }

    private var tagMenu: some View {
        Menu {
            Button("Add tag…") { tagInputMode = .add }
            Button("Remove tag…") { tagInputMode = .remove }
                .disabled(tagsInSelection.isEmpty)
        } label: {
            Label("Tag", systemImage: "number")
        }
        .disabled(isMutating)
    }

    // MARK: helpers

    private var tagsInSelection: [String] {
        let set = tasks.flatMap { TagExtractor.tags(in: $0.title) }
        var seen = Set<String>()
        var out: [String] = []
        for tag in set {
            let key = tag.lowercased()
            if seen.insert(key).inserted { out.append(tag) }
        }
        return out.sorted(by: { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending })
    }

    private func applyDate(daysFromToday days: Int) {
        let cal = Calendar.current
        let target = cal.date(byAdding: .day, value: days, to: cal.startOfDay(for: Date())) ?? Date()
        let ops = tasks.map { BulkTaskOperation.setDue(taskId: $0.id, dueDate: target) }
        Task { await runBulk(ops) }
    }

    private func runBulk(_ ops: [BulkTaskOperation]) async {
        isMutating = true
        defer { isMutating = false }
        let result = await model.performBulkTaskOperations(ops)
        if result.allSucceeded || (result.submitted == 0 && result.droppedAsNoOp == ops.count) {
            selection.removeAll()
        } else if result.failedCount > 0 {
            // Leave the failing items selected so the user can retry / inspect.
            let failingIds = Set(result.failures.map(\.operation.taskId))
            selection = selection.intersection(failingIds)
        } else {
            selection.removeAll()
        }
        onFinished(result)
    }
}

fileprivate enum BulkTagMode: Identifiable {
    case add, remove
    var id: String { self == .add ? "add" : "remove" }
}

fileprivate enum BulkDatePickerMode: Identifiable {
    case customDue
    var id: String { "customDue" }
}

private struct TagInputSheet: View {
    @Environment(\.dismiss) private var dismiss
    let mode: BulkTagMode
    let tasks: [TaskMirror]
    let onApply: (String) -> Void
    let onCancel: () -> Void

    @State private var tag: String = ""

    var body: some View {
        NavigationStack {
            Form {
                Section(mode == .add ? "Add tag" : "Remove tag") {
                    TextField("tag (no leading #)", text: $tag)
                        .textFieldStyle(.roundedBorder)
                        .onSubmit(apply)
                    if mode == .remove {
                        let options = existingTags
                        if options.isEmpty {
                            Text("No tags present in the selection.")
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        } else {
                            Text("Existing: \(options.map { "#\($0)" }.joined(separator: " "))")
                                .hcbFont(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    Text("Applies to \(tasks.count) task\(tasks.count == 1 ? "" : "s"). A \(mode == .add ? "duplicate" : "missing") tag per task is silently skipped by the optimizer.")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle(mode == .add ? "Add tag" : "Remove tag")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: onCancel)
                        .keyboardShortcut(.cancelAction)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Apply", action: apply)
                        .keyboardShortcut(.defaultAction)
                        .disabled(sanitized.isEmpty)
                }
            }
        }
        .hcbScaledFrame(minWidth: 360, minHeight: 200)
    }

    private var sanitized: String {
        tag.trimmingCharacters(in: CharacterSet(charactersIn: "# "))
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var existingTags: [String] {
        var seen = Set<String>()
        var out: [String] = []
        for t in tasks {
            for tag in TagExtractor.tags(in: t.title) {
                let key = tag.lowercased()
                if seen.insert(key).inserted { out.append(tag) }
            }
        }
        return out.sorted(by: { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending })
    }

    private func apply() {
        guard sanitized.isEmpty == false else { return }
        onApply(sanitized)
    }
}

private struct CustomDueDateSheet: View {
    @Environment(\.dismiss) private var dismiss
    let onPick: (Date) -> Void
    let onCancel: () -> Void
    @State private var date: Date = Calendar.current.startOfDay(for: Date())

    var body: some View {
        NavigationStack {
            Form {
                DatePicker("Due", selection: $date, displayedComponents: [.date])
            }
            .navigationTitle("Pick a due date")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: onCancel)
                        .keyboardShortcut(.cancelAction)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Apply") {
                        onPick(Calendar.current.startOfDay(for: date))
                    }
                    .keyboardShortcut(.defaultAction)
                }
            }
        }
        .hcbScaledFrame(minWidth: 360, minHeight: 220)
    }
}
