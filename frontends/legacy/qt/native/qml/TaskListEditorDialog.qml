import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: taskListId.length > 0 ? "Rename task list" : "New task list"
    primaryText: taskListId.length > 0 ? "Rename list" : "Create list"
    primaryEnabled: titleField.text.trim().length > 0
    property string taskListId: ""
    property string taskListTitle: ""
    property alias taskListTitleField: titleField
    signal taskListSaveRequested(string taskListId, string title)

    function openForCreate() {
        taskListId = ""
        taskListTitle = ""
        titleField.clear()
        open()
    }

    function openForRename(taskListId, taskListTitle) {
        root.taskListId = taskListId
        root.taskListTitle = taskListTitle
        titleField.text = taskListTitle
        open()
    }

    onOpened: titleField.forceActiveFocus()
    onPrimaryAction: taskListSaveRequested(taskListId, titleField.text.trim())

    TextField {
        id: titleField
        Layout.fillWidth: true
        placeholderText: "Task list name"
        Accessible.name: "Task list name"
        selectByMouse: true
        Keys.onReturnPressed: {
            if (root.primaryEnabled) {
                root.primaryButton.click()
            }
        }
    }
}
