import AppKit
import SwiftUI

struct TaskContextMenu: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow

    let task: TaskMirror
    var onOpen: (() -> Void)? = nil
    var onCustomSnooze: (() -> Void)? = nil
    var onConvertToEvent: (() -> Void)? = nil
    var onConvertToTaskOrNote: (() -> Void)? = nil
    var onDelete: (() -> Void)? = nil
    private var taskStore: TaskStore { model.taskStore }

    private var otherLists: [TaskListMirror] {
        taskStore.taskLists.filter { $0.id != task.taskListID }
    }

    private var listTitle: String? {
        taskStore.taskListTitle(for: task.taskListID, fallback: "")
    }

    private var duplicatePrimaryLabel: String {
        task.dueDate == nil ? "Duplicate as Note" : "Keep same due date"
    }

    private var convertSecondaryLabel: String {
        task.dueDate == nil ? "Set Due Date…" : "Remove Due Date"
    }

    var body: some View {
        if let onOpen {
            Button("Open…", action: onOpen)
        }

        Button(task.isCompleted ? "Mark as Needs Action" : "Mark Complete") {
            Task { _ = await model.setTaskCompleted(!task.isCompleted, task: task) }
        }

        if task.isCompleted == false, task.dueDate != nil, let onCustomSnooze {
            TaskSnoozeContextMenu(
                onSnoozeTo: { newDate in
                    Task {
                        _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: newDate)
                    }
                },
                onPickCustomDate: onCustomSnooze
            )
        }

        Divider()

        Menu("Duplicate…") {
            Button(duplicatePrimaryLabel) {
                Task { _ = await model.duplicateTask(task) }
            }
            Button("Duplicate for Tomorrow") {
                Task {
                    _ = await model.duplicateTask(
                        task,
                        dueDate: TaskSnoozeSupport.targetDate(daysFromToday: 1)
                    )
                }
            }
            Button("Duplicate for Next Week") {
                Task {
                    _ = await model.duplicateTask(
                        task,
                        dueDate: TaskSnoozeSupport.targetDate(daysFromToday: 7)
                    )
                }
            }
        }

        Menu("Move to List…") {
            if otherLists.isEmpty {
                Button("No other lists") {}
                    .disabled(true)
            } else {
                ForEach(otherLists) { list in
                    Button(list.title) {
                        Task { _ = await model.moveTaskToList(task, toTaskListID: list.id) }
                    }
                }
            }
        }

        if onConvertToEvent != nil || onConvertToTaskOrNote != nil {
            Menu("Convert…") {
                if let onConvertToEvent {
                    Button("Convert to Event…", action: onConvertToEvent)
                }
                if let onConvertToTaskOrNote {
                    Button(convertSecondaryLabel, action: onConvertToTaskOrNote)
                }
            }
        }

        Menu("Share…") {
            let taskURL = HCBDeepLinkBuilder.taskURL(for: task)
            Button("Copy Link") {
                copyToPasteboard(taskURL.absoluteString)
                postCopyToast("Task link copied to clipboard.")
            }
            ShareLink(item: taskURL) {
                Text("Share Link…")
            }
            Button("Copy as Markdown") {
                let markdown = TaskMarkdownExporter.markdown(for: task, taskListTitle: listTitle)
                copyToPasteboard(markdown)
                postCopyToast("Task Markdown copied to clipboard.")
            }
        }

        if taskStore.duplicateGroupKey(for: task.id) != nil {
            Button("Review Duplicates…") {
                openWindow(id: "duplicate-review")
            }
        }

        if let onDelete {
            Divider()
            Button("Delete", role: .destructive, action: onDelete)
        }
    }

    private func copyToPasteboard(_ value: String) {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(value, forType: .string)
    }

    private func postCopyToast(_ message: String) {
        NotificationCenter.default.post(name: .hcbClipboardMessage, object: message)
    }
}
