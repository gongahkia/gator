import AppKit
import SwiftUI

struct DuplicateReviewWindow: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow

    private var groups: [DuplicateGroupSummary] {
        model.duplicateIndex.groups.compactMap { key, ids in
            let tasks = ids.compactMap { model.task(id: $0) }
            guard tasks.isEmpty == false else { return nil }
            return DuplicateGroupSummary(groupKey: key, tasks: tasks.sorted(by: Self.sortTasks))
        }
        .sorted { lhs, rhs in
            if lhs.tasks.count != rhs.tasks.count {
                return lhs.tasks.count > rhs.tasks.count
            }
            return lhs.primaryTitle.localizedCaseInsensitiveCompare(rhs.primaryTitle) == .orderedAscending
        }
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                header

                if groups.isEmpty {
                    emptyState
                } else {
                    ForEach(groups) { group in
                        DuplicateGroupCard(
                            group: group,
                            onOpenTask: { revealTask($0) },
                            onDeleteTask: { task in
                                Task { _ = await model.deleteTask(task) }
                            },
                            onDismissGroup: { model.dismissDuplicateGroup(group.groupKey) }
                        )
                    }
                }
            }
            .hcbScaledPadding(20)
        }
        .appBackground()
        .navigationTitle("Review Duplicates")
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Review tasks that currently match on exact title and notes.")
                .hcbFont(.headline)
            Text("This is a local review surface only. It uses the existing duplicate index and dismissal rules; it does not change how tasks sync or mutate.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)

            if model.settings.dismissedDuplicateGroups.isEmpty == false {
                HStack(spacing: 10) {
                    Text("\(model.settings.dismissedDuplicateGroups.count) dismissed group\(model.settings.dismissedDuplicateGroups.count == 1 ? "" : "s") hidden")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                    Button("Restore dismissed groups") {
                        model.clearAllDuplicateDismissals()
                    }
                    .buttonStyle(.bordered)
                }
            }
        }
    }

    @ViewBuilder
    private var emptyState: some View {
        ContentUnavailableView(
            "No active duplicate groups",
            systemImage: "checkmark.shield",
            description: Text("Potential duplicate tasks will appear here when exact title-and-notes matches are found.")
        )
        .frame(maxWidth: .infinity)
        .hcbScaledPadding(.top, 40)
    }

    private func revealTask(_ taskID: TaskMirror.ID) {
        openWindow(id: "main")
        NotificationCenter.default.post(name: .hcbRevealTaskInStore, object: taskID)
        NSApp.activate(ignoringOtherApps: true)
    }

    private static func sortTasks(_ lhs: TaskMirror, _ rhs: TaskMirror) -> Bool {
        if lhs.isCompleted != rhs.isCompleted {
            return lhs.isCompleted == false
        }
        switch (lhs.dueDate, rhs.dueDate) {
        case let (.some(a), .some(b)):
            if a != b { return a < b }
        case (.some, .none):
            return true
        case (.none, .some):
            return false
        case (.none, .none):
            break
        }
        return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending
    }
}

private struct DuplicateGroupSummary: Identifiable {
    let groupKey: String
    let tasks: [TaskMirror]

    var id: String { groupKey }
    var primaryTitle: String {
        guard let first = tasks.first else { return "Untitled" }
        let trimmed = first.title.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? "Untitled" : first.title
    }
}

private struct DuplicateGroupCard: View {
    @Environment(AppModel.self) private var model

    let group: DuplicateGroupSummary
    let onOpenTask: (TaskMirror.ID) -> Void
    let onDeleteTask: (TaskMirror) -> Void
    let onDismissGroup: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "exclamationmark.2")
                    .foregroundStyle(AppColor.ember)
                    .hcbFont(.headline, weight: .semibold)
                VStack(alignment: .leading, spacing: 4) {
                    Text(groupTitle)
                        .hcbFont(.headline)
                    Text("Exact-match title and notes across \(group.tasks.count) active tasks.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 8)
                Button("Dismiss group", action: onDismissGroup)
                    .buttonStyle(.bordered)
            }

            ForEach(group.tasks, id: \.id) { task in
                DuplicateTaskRow(
                    task: task,
                    listTitle: model.taskListTitle(for: task.taskListID),
                    onOpen: { onOpenTask(task.id) },
                    onDelete: { onDeleteTask(task) }
                )
            }
        }
        .hcbScaledPadding(16)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(.ultraThinMaterial)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 16)
                .stroke(AppColor.cardStroke, lineWidth: 0.8)
        )
    }

    private var groupTitle: String {
        let stripped = TagExtractor.stripped(from: group.primaryTitle).trimmingCharacters(in: .whitespacesAndNewlines)
        return stripped.isEmpty ? "Untitled duplicate group" : stripped
    }
}

private struct DuplicateTaskRow: View {
    let task: TaskMirror
    let listTitle: String
    let onOpen: () -> Void
    let onDelete: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(alignment: .top, spacing: 10) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(taskTitle)
                        .hcbFont(.subheadline, weight: .semibold)
                    HStack(spacing: 8) {
                        Text(listTitle)
                        if let dueText {
                            Text("•")
                            Text(dueText)
                        }
                        if task.isCompleted {
                            Text("•")
                            Text("Completed")
                        }
                    }
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                }
                Spacer(minLength: 8)
                Button("Open", action: onOpen)
                    .buttonStyle(.borderedProminent)
                Button(role: .destructive, action: onDelete) {
                    Text("Delete")
                }
                .buttonStyle(.bordered)
            }

            if task.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                Text(task.notes)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(3)
                    .textSelection(.enabled)
            }
        }
        .hcbScaledPadding(12)
        .background(
            RoundedRectangle(cornerRadius: 12)
                .fill(AppColor.cardSurface)
        )
    }

    private var taskTitle: String {
        let stripped = TagExtractor.stripped(from: task.title).trimmingCharacters(in: .whitespacesAndNewlines)
        return stripped.isEmpty ? "Untitled" : stripped
    }

    private var dueText: String? {
        guard let due = task.dueDate else { return nil }
        return "Due \(due.formatted(date: .abbreviated, time: .omitted))"
    }
}
