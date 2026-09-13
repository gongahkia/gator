import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: mode === "due" ? "Set due date" : mode === "priority" ? "Set priority" : "Reparent tasks"
    primaryText: mode === "due" ? "Set due date" : mode === "priority" ? "Set priority" : "Reparent tasks"
    primaryEnabled: taskIds.length > 0 && (mode !== "due" || dueField.text.trim().length > 0)
    property var taskModel: null
    property string mode: "due"
    property var taskIds: []
    property string parentTaskId: ""
    property int priority: 0
    property alias dueField: dueField
    property alias priorityPicker: priorityPicker
    property alias parentPicker: parentPicker
    signal bulkDueRequested(var taskIds, string dueAt)
    signal bulkPriorityRequested(var taskIds, int priority)
    signal bulkReparentRequested(var taskIds, string parentTaskId)

    function openForDue(ids) {
        mode = "due"
        taskIds = ids.slice()
        dueField.clear()
        open()
    }

    function openForPriority(ids) {
        mode = "priority"
        taskIds = ids.slice()
        priorityPicker.currentIndex = 0
        priority = priorityPicker.currentValue
        open()
    }

    function openForReparent(ids) {
        mode = "reparent"
        taskIds = ids.slice()
        parentPicker.currentIndex = 0
        parentTaskId = parentPicker.currentValue || ""
        open()
    }

    onPrimaryAction: {
        if (mode === "due") {
            bulkDueRequested(taskIds, dueField.text.trim())
        } else if (mode === "priority") {
            bulkPriorityRequested(taskIds, priority)
        } else {
            bulkReparentRequested(taskIds, parentTaskId)
        }
    }

    Label {
        Layout.fillWidth: true
        text: taskIds.length + " selected task" + (taskIds.length === 1 ? "" : "s")
        color: Theme.textSecondary
    }

    TextField {
        id: dueField
        Layout.fillWidth: true
        visible: root.mode === "due"
        placeholderText: "Due date (YYYY-MM-DD)"
        Accessible.name: placeholderText
        selectByMouse: true
    }

    ComboBox {
        id: priorityPicker
        Layout.fillWidth: true
        visible: root.mode === "priority"
        model: [
            { label: "No priority", value: 0 },
            { label: "Low priority", value: 1 },
            { label: "Medium priority", value: 2 },
            { label: "High priority", value: 3 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Task priority"
        onCurrentValueChanged: root.priority = currentValue
    }

    ComboBox {
        id: parentPicker
        Layout.fillWidth: true
        visible: root.mode === "reparent"
        model: {
            if (root.taskModel === null || typeof root.taskModel.topLevelTasks !== "function") {
                return [{ id: "", title: "Top level" }]
            }
            return [{ id: "", title: "Top level" }].concat(root.taskModel.topLevelTasks())
        }
        textRole: "title"
        valueRole: "id"
        Accessible.name: "New parent task"
        onCurrentValueChanged: root.parentTaskId = currentValue || ""
    }
}
