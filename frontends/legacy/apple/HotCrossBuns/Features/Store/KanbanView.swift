import SwiftUI

// Kanban board host. Renders one scrollable column per KanbanColumn; each
// card is a DraggedTask that routes back to AppModel.performBulkTaskOperations
// via the column's KanbanDropIntent.
//
// Post-refactor responsibilities:
//  - Group-by picker (byList | byDueBucket | byTag)
//  - Column header dot + ⋯ menu (rename / delete), only
//    shown in `byList` mode where a header maps 1:1 to a Google task list.
//  - Empty-space tap inside a column → QuickCreatePopover in task-only mode
//    pre-filled with that column's list.
//  - Optional "New List…" button in the group-by header (byList only).
//
// Data-safety: drop-to-same-column is a no-op; invalid drops fail closed;
// every drop still routes through optimistic-write helpers so offline queue
// + etag conflict + undo paths apply unchanged.
struct KanbanView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.hcbAppBackgroundConfiguration) private var backgroundConfiguration
    let snapshot: TaskBoardDisplaySnapshot
    @Binding var columnMode: KanbanColumnMode
    @Binding var selection: Set<TaskMirror.ID>
    var availableColumnModes: [KanbanColumnMode] = KanbanColumnMode.allCases
    var onResult: (BulkTaskExecutionResult) -> Void = { _ in }
    var onRenameList: (TaskListMirror) -> Void = { _ in }
    var onDeleteList: (TaskListMirror) -> Void = { _ in }
    var onNewList: () -> Void = {}
    var onCustomSnooze: (TaskMirror) -> Void = { _ in }
    var onCreateTaskInList: (TaskListMirror.ID?) -> Void = { _ in }
    // Callers can override what happens when a card is tapped. Default is
    // to set `selection` to just that task id. Notes tab overrides this to
    // open its side inspector instead.
    var onCardTap: ((TaskMirror) -> Void)? = nil

    @State private var dropHighlightColumnID: String?
    private var usesReadableTopBar: Bool {
        backgroundConfiguration.customImagePath != nil || backgroundConfiguration.isTranslucent
    }
    private var itemLabel: String {
        snapshot.surface == .notes ? "note" : "task"
    }
    private var taskStore: TaskStore { model.taskStore }

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            ScrollView(.horizontal) {
                // Lazy horizontal stack so columns off-screen don't
                // instantiate their row subtrees on every board render.
                LazyHStack(alignment: .top, spacing: 14) {
                    ForEach(snapshot.columns) { column in
                        KanbanColumnView(
                            column: column,
                            surface: snapshot.surface,
                            mode: columnMode,
                            taskList: taskList(for: column),
                            isDropTargeted: dropHighlightColumnID == column.id,
                            onDrop: { dropped in handleDrop(dropped, on: column) },
                            onDropTargetChanged: { isTargeted in
                                dropHighlightColumnID = isTargeted ? column.id : nil
                            },
                            selection: $selection,
                            onCardTap: { task in
                                if let onCardTap {
                                    onCardTap(task)
                                } else {
                                    selection = [task.id]
                                }
                            },
                            onRenameList: onRenameList,
                            onDeleteList: onDeleteList,
                            onCustomSnooze: onCustomSnooze,
                            onCreateTask: { onCreateTaskInList(taskList(for: column)?.id) }
                        )
                    }
                    if columnMode == .byList {
                        newListColumn
                    }
                }
                .hcbScaledPadding(.horizontal, 16)
                .hcbScaledPadding(.vertical, 12)
            }
        }
    }

    private func taskList(for column: PreparedKanbanColumn) -> TaskListMirror? {
        // Column IDs under byList follow "list-<id>" — see KanbanGrouping.
        guard columnMode == .byList, column.id.hasPrefix("list-") else { return nil }
        let listID = String(column.id.dropFirst("list-".count))
        return taskStore.taskList(id: listID)
    }

    private var header: some View {
        HStack(spacing: 10) {
            Text("Group by")
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
            Picker("Group", selection: $columnMode) {
                ForEach(availableColumnModes, id: \.self) { m in
                    Label(m.title, systemImage: m.systemImage).tag(m)
                }
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .fixedSize()
            Spacer(minLength: 0)
            Text("\(snapshot.taskCount) \(itemLabel)\(snapshot.taskCount == 1 ? "" : "s")")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 10)
        .background {
            if usesReadableTopBar {
                Rectangle()
                    .fill(.regularMaterial)
                    .overlay(AppColor.cream.opacity(0.24))
            }
        }
    }

    private var newListColumn: some View {
        Button(action: onNewList) {
            VStack(spacing: 8) {
                Image(systemName: "plus.circle")
                    .hcbFont(.title2)
                Text("New List")
                    .hcbFont(.caption, weight: .medium)
            }
            .frame(width: 120, height: 80)
            .foregroundStyle(AppColor.ember)
            .background(
                RoundedRectangle(cornerRadius: 12)
                    .stroke(AppColor.ember.opacity(0.4), style: StrokeStyle(lineWidth: 1.2, dash: [4]))
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help("Create a new Google Tasks list")
    }

    private func handleDrop(_ dropped: DraggedTask, on column: PreparedKanbanColumn) {
        guard let intent = column.dropIntent,
              let op = intent.operation(for: dropped.taskID) else { return }
        Task {
            let result = await model.performBulkTaskOperations([op])
            onResult(result)
        }
    }
}

private struct KanbanColumnView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.routerPath) private var router
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let column: PreparedKanbanColumn
    let surface: TaskBoardSurface
    let mode: KanbanColumnMode
    let taskList: TaskListMirror?
    let isDropTargeted: Bool
    let onDrop: (DraggedTask) -> Void
    let onDropTargetChanged: (Bool) -> Void
    @Binding var selection: Set<TaskMirror.ID>
    let onCardTap: (TaskMirror) -> Void
    let onRenameList: (TaskListMirror) -> Void
    let onDeleteList: (TaskListMirror) -> Void
    let onCustomSnooze: (TaskMirror) -> Void
    let onCreateTask: () -> Void

    @State private var isCompletedExpanded = false

    private let columnWidth: CGFloat = 260
    private var taskStore: TaskStore { model.taskStore }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            header
            ScrollView(.vertical) {
                // Lazy vertical stack — columns can contain thousands of
                // cards; eager VStack instantiated every card even the ones
                // scrolled off-screen. LazyVStack materializes cards as they
                // approach the viewport.
                LazyVStack(spacing: 8) {
                    ForEach(column.openTasks) { card in
                        taskCard(card)
                    }
                    // Inline add-task affordance — clicking anywhere in this
                    // zone opens QuickCreatePopover pre-filled with the
                    // column's list. Keeps the "click-to-create" feel from
                    // Apple Calendar's quick-create.
                    Button(action: onCreateTask) {
                        HStack(spacing: 6) {
                            Image(systemName: "plus.circle")
                            Text(surface == .notes ? "Add note" : "Add task")
                        }
                        .hcbFont(.caption, weight: .medium)
                        .foregroundStyle(AppColor.ember.opacity(0.85))
                        .frame(maxWidth: .infinity)
                        .hcbScaledPadding(.vertical, 10)
                        .background(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(AppColor.ember.opacity(0.35), style: StrokeStyle(lineWidth: 1, dash: [3]))
                        )
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .hcbScaledPadding(.horizontal, 2)
                    if completedTasks.isEmpty == false {
                        completedDisclosure
                        if isCompletedExpanded {
                            ForEach(column.completedTasks) { card in
                                completedTaskRow(card)
                            }
                        }
                    }
                    if column.taskCount == 0, column.dropIntent != nil {
                        Text("Drop here")
                            .hcbFont(.caption)
                            .foregroundStyle(.tertiary)
                            .frame(maxWidth: .infinity, minHeight: 32)
                    }
                }
                .hcbScaledPadding(.vertical, 4)
            }
            .scrollIndicators(.hidden)
            .frame(maxHeight: .infinity)
        }
        .frame(width: columnWidth, alignment: .top)
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 12)
                .fill(AppColor.cardSurface.opacity(isDropTargeted ? 0.9 : 0.55))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(isDropTargeted ? AppColor.ember.opacity(0.6) : AppColor.cardStroke, lineWidth: isDropTargeted ? 1.4 : 0.6)
        )
        .dropDestination(for: DraggedTask.self) { items, _ in
            guard column.dropIntent != nil, let dropped = items.first else { return false }
            onDrop(dropped)
            return true
        } isTargeted: { targeted in
            guard column.dropIntent != nil else {
                onDropTargetChanged(false)
                return
            }
            onDropTargetChanged(targeted)
        }
    }

    private var completedTasks: [PreparedTaskCard] { column.completedTasks }

    private var completedDisclosure: some View {
        Button {
            HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.16)) {
                isCompletedExpanded.toggle()
            }
        } label: {
            HStack(spacing: 8) {
                Image(systemName: isCompletedExpanded ? "chevron.down" : "chevron.right")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                    .frame(width: 14)
                Text("Completed (\(completedTasks.count))")
                    .hcbFont(.subheadline, weight: .semibold)
                    .foregroundStyle(.secondary)
                Spacer(minLength: 0)
            }
            .hcbScaledPadding(.horizontal, 6)
            .hcbScaledPadding(.vertical, 8)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(isCompletedExpanded ? "Collapse completed tasks" : "Expand completed tasks")
    }

    private func taskCard(_ card: PreparedTaskCard) -> some View {
        KanbanCardView(
            card: card,
            isSelected: selection.contains(card.id),
            onTap: { openTask(card) }
        )
        .draggable(DraggedTask(taskID: card.id, taskListID: card.taskListID, title: card.title))
        .contextMenu {
            if let task = taskStore.task(id: card.id) {
                taskContextMenu(for: task)
            }
        }
    }

    private func completedTaskRow(_ card: PreparedTaskCard) -> some View {
        KanbanCompletedTaskRowView(
            card: card,
            isSelected: selection.contains(card.id),
            onTap: { openTask(card) }
        )
        .draggable(DraggedTask(taskID: card.id, taskListID: card.taskListID, title: card.title))
        .contextMenu {
            if let task = taskStore.task(id: card.id) {
                taskContextMenu(for: task)
            }
        }
    }

    private func openTask(_ card: PreparedTaskCard) {
        guard let task = taskStore.task(id: card.id) else { return }
        onCardTap(task)
    }

    @ViewBuilder
    private func taskContextMenu(for task: TaskMirror) -> some View {
        TaskContextMenu(
            task: task,
            onOpen: { onCardTap(task) },
            onCustomSnooze: { onCustomSnooze(task) },
            onConvertToEvent: { router?.present(.convertTaskToEvent(task.id)) },
            onConvertToTaskOrNote: {
                if task.dueDate == nil {
                    router?.present(.convertNoteToTask(task.id))
                } else {
                    router?.present(.convertTaskToNote(task.id))
                }
            },
            onDelete: {
                Task { _ = await model.deleteTask(task) }
            }
        )
    }

    private var header: some View {
        HStack(spacing: 8) {
            if mode == .byList {
                Circle()
                    .fill(AppColor.ember)
                    .hcbScaledFrame(width: 10, height: 10)
            }
            VStack(alignment: .leading, spacing: 2) {
                Text(column.title)
                    .hcbFont(.subheadline, weight: .semibold)
                    .foregroundStyle(AppColor.ink)
                    .lineLimit(1)
                if let subtitle = column.subtitle {
                    Text(subtitle)
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 4)
            if let list = taskList {
                Menu {
                    Button {
                        onRenameList(list)
                    } label: {
                        Label("Rename", systemImage: "pencil")
                    }
                    Divider()
                    Button(role: .destructive) {
                        onDeleteList(list)
                    } label: {
                        Label("Delete List", systemImage: "trash")
                    }
                } label: {
                    Image(systemName: "ellipsis")
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                        .frame(width: 22, height: 22)
                        .background(
                            Circle().fill(AppColor.cardStroke.opacity(0.25))
                        )
                }
                .menuStyle(.borderlessButton)
                .menuIndicator(.hidden)
                .fixedSize()
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

private struct KanbanCompletedTaskRowView: View {
    let card: PreparedTaskCard
    let isSelected: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "checkmark")
                    .hcbFont(.headline, weight: .semibold)
                    .foregroundStyle(AppColor.ember)
                    .frame(width: 18)
                VStack(alignment: .leading, spacing: 3) {
                    Text(card.strippedTitle)
                        .hcbFont(.subheadline)
                        .foregroundStyle(AppColor.ink)
                        .strikethrough(true, color: AppColor.ink)
                        .lineLimit(3)
                        .multilineTextAlignment(.leading)
                    Text(card.completedText ?? "Completed")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Spacer(minLength: 0)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .hcbScaledPadding(.horizontal, 8)
            .hcbScaledPadding(.vertical, 7)
            .background(
                RoundedRectangle(cornerRadius: 8)
                    .fill(isSelected ? AppColor.ember.opacity(0.10) : Color.clear)
            )
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(completedTaskAccessibilityLabel)
    }

    private var completedTaskAccessibilityLabel: String {
        card.accessibilityLabel
    }
}

private struct KanbanCardView: View {
    let card: PreparedTaskCard
    let isSelected: Bool
    let onTap: () -> Void

    var body: some View {
        Button(action: onTap) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(alignment: .top, spacing: 6) {
                    Image(systemName: card.isCompleted ? "checkmark.circle.fill" : "circle")
                        .foregroundStyle(card.isCompleted ? AppColor.moss : AppColor.ember)
                        .hcbFont(.subheadline)
                    VStack(alignment: .leading, spacing: 4) {
                        HStack(spacing: 4) {
                            Text(card.strippedTitle)
                                .hcbFont(.subheadline, weight: .medium)
                                .foregroundStyle(AppColor.ink)
                                .lineLimit(2)
                                .multilineTextAlignment(.leading)
                        }
                        tagsRow
                        metaRow
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .hcbScaledPadding(10)
            .background(
                RoundedRectangle(cornerRadius: 10)
                    .fill(AppColor.cardSurface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 10)
                    .strokeBorder(
                        isSelected ? AppColor.ember : AppColor.cardStroke,
                        lineWidth: isSelected ? 1.4 : 0.6
                    )
            )
            .overlay(alignment: .topTrailing) {
                if card.isDuplicate {
                    duplicateBadge
                        .hcbScaledPadding(6)
                }
            }
        }
        .buttonStyle(.plain)
        .accessibilityLabel(taskAccessibilityLabel)
    }

    private var taskAccessibilityLabel: String {
        card.accessibilityLabel
    }

    private var duplicateBadge: some View {
        // small ember-tinted pill: Apple Reminders + Calendar use similar SF Symbols for emphasis. Click lands on the card (button wraps everything) which opens inspector where the user can dismiss or delete.
        Image(systemName: "exclamationmark.2")
            .hcbFontSystem(size: 10, weight: .bold)
            .foregroundStyle(.white)
            .hcbScaledFrame(width: 18, height: 18)
            .background(Circle().fill(AppColor.ember))
            .overlay(Circle().strokeBorder(.white.opacity(0.6), lineWidth: 0.6))
            .help("Possible duplicate. Open to review.")
            .accessibilityLabel("Possible duplicate")
    }

    @ViewBuilder
    private var tagsRow: some View {
        if card.tags.isEmpty == false {
            HStack(spacing: 4) {
                ForEach(card.tags.prefix(4), id: \.self) { tag in
                    Text("#\(tag)")
                        .hcbFont(.caption2, weight: .medium)
                        .hcbScaledPadding(.horizontal, 6)
                        .hcbScaledPadding(.vertical, 1)
                        .background(Capsule().fill(AppColor.blue.opacity(0.15)))
                        .foregroundStyle(AppColor.blue)
                        .accessibilityLabel("Tag \(tag)")
                }
                if card.tags.count > 4 {
                    Text("+\(card.tags.count - 4)")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                        .accessibilityLabel("\(card.tags.count - 4) more tags")
                }
            }
        }
    }

    @ViewBuilder
    private var metaRow: some View {
        HStack(spacing: 6) {
            if let dueDateBadge = card.dueDateBadge {
                Label {
                    Text(dueDateBadge)
                } icon: {
                    Image(systemName: "calendar")
                }
                .hcbFont(.caption2)
                .foregroundStyle(dueDateColor(card.dueDateTone))
            }
            if card.listTitle.isEmpty == false {
                Text(card.listTitle)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        }
    }

    private func dueDateColor(_ tone: PreparedDueDateTone?) -> Color {
        if tone == .overdue { return AppColor.ember }
        if tone == .today { return AppColor.moss }
        return .secondary
    }
}
