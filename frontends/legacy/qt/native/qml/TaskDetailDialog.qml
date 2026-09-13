import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Task"
    primaryText: "Edit task"
    secondaryText: "Close"
    destructiveText: "Delete task"
    property var task: ({})
    signal editRequested(var task)
    signal deleteRequested(string taskId, string title, bool managedRecurrence)
    signal externalLinkRequested(string url)

    function openForTask(value) {
        task = value || ({})
        open()
    }

    function dueLabel() {
        if (!task.dueAt) return "No due date"
        const date = new Date(task.dueAt.length === 10 ? task.dueAt + "T12:00:00" : task.dueAt)
        return Number.isFinite(date.getTime()) ? Qt.locale().toString(date, "ddd, d MMM yyyy") : task.dueAt
    }

    onPrimaryAction: editRequested(task)

    ColumnLayout {
        Layout.fillWidth: true
        spacing: Theme.spacingMedium

        ExternalLinkText {
            Layout.fillWidth: true
            plainText: task.title || "Untitled task"
            font.pixelSize: Theme.titleFontSize
            font.bold: true
            onLinkRequested: url => root.externalLinkRequested(url)
        }

        Label { text: task.completed ? "Completed" : "Open"; color: Theme.textSecondary }
        Label { text: "List · " + (task.taskListTitle || "Unknown list"); color: Theme.textSecondary }
        Label { text: "Due · " + root.dueLabel(); color: Theme.textSecondary }
        Label { visible: task.priority > 0; text: "Priority · " + ["", "Low", "Medium", "High"][task.priority]; color: Theme.textSecondary }
        Label { visible: task.recurrenceSummary && task.recurrenceSummary.length > 0; text: task.recurrenceSummary; color: Theme.textSecondary }

        Label { visible: task.notes && task.notes.length > 0; text: "Notes"; font.bold: true }
        ExternalLinkText {
            Layout.fillWidth: true
            visible: task.notes && task.notes.length > 0
            plainText: task.notes || ""
            onLinkRequested: url => root.externalLinkRequested(url)
        }
    }

    onDestructiveAction: root.deleteRequested(task.id || "", task.title || "", task.managedRecurrence === true)
}
