import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: taskId.length > 0 ? "Edit note" : "New note"
    primaryText: taskId.length > 0 ? "Save note" : "Create note"
    primaryEnabled: taskListId.length > 0 && titleField.text.trim().length > 0
    property var taskListModel: null
    property int taskListRevision: taskListModel !== null && taskListModel.revision !== undefined
                                   ? taskListModel.revision : 0
    property var activeTaskLists: {
        const revision = taskListRevision
        if (taskListModel !== null && typeof taskListModel.selectedTaskLists === "function") {
            return taskListModel.selectedTaskLists()
        }
        return taskListModel
    }
    property string taskId: ""
    property string taskListId: ""
    property alias noteTitle: titleField.text
    property alias noteBody: bodyField.text
    property alias noteTitleField: titleField
    property alias taskListPicker: taskListPicker
    signal taskSaveRequested(string taskId, string taskListId, string title, string body)

    function openForCreate(initialTaskListId) {
        root.taskId = ""
        titleField.clear()
        bodyField.clear()
        taskListPicker.currentIndex = taskListPicker.indexOfValue(initialTaskListId || "")
        if (taskListPicker.currentIndex < 0 && taskListPicker.count > 0) {
            taskListPicker.currentIndex = 0
        }
        root.taskListId = taskListPicker.currentValue || ""
        open()
    }

    function openForEdit(taskId, taskListId, title, body) {
        root.taskId = taskId
        titleField.text = title
        bodyField.text = body
        taskListPicker.currentIndex = taskListPicker.indexOfValue(taskListId)
        root.taskListId = taskListPicker.currentValue || taskListId
        open()
    }

    onOpened: titleField.forceActiveFocus()
    onPrimaryAction: taskSaveRequested(taskId, taskListId, titleField.text.trim(), bodyField.text)

    EmojiTextField {
        id: titleField
        Layout.fillWidth: true
        placeholderText: "Note title"
        Accessible.name: "Note title"
        selectByMouse: true
        Keys.onReturnPressed: {
            if (root.primaryEnabled) {
                root.primaryButton.click()
            }
        }
    }

    EmojiTextArea {
        id: bodyField
        Layout.fillWidth: true
        Layout.preferredHeight: 240
        placeholderText: "Write your note"
        Accessible.name: "Note body"
        selectByMouse: true
        wrapMode: TextEdit.Wrap
    }

    ComboBox {
        id: taskListPicker
        Layout.fillWidth: true
        model: root.activeTaskLists
        textRole: "title"
        valueRole: "id"
        Accessible.name: "Task list"
        onCurrentValueChanged: root.taskListId = currentValue || ""
    }

    Label {
        Layout.fillWidth: true
        visible: taskListPicker.count === 0
        text: "No active task lists are available."
        color: Theme.textSecondary
        horizontalAlignment: Text.AlignHCenter
    }
}
