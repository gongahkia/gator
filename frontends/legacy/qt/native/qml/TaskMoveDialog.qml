import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Move task"
    primaryText: "Move task"
    primaryEnabled: taskId.length > 0 && destinationTaskListId.length > 0
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
    property alias taskListRows: taskListRows
    property string taskId: ""
    property string taskTitle: ""
    property string currentTaskListId: ""
    property string destinationTaskListId: ""
    signal taskMoveRequested(string taskId, string taskListId)

    function openForMove(taskId, taskTitle, currentTaskListId) {
        root.taskId = taskId
        root.taskTitle = taskTitle
        root.currentTaskListId = currentTaskListId
        root.destinationTaskListId = ""
        open()
    }

    onPrimaryAction: taskMoveRequested(taskId, destinationTaskListId)

    Label {
        Layout.fillWidth: true
        text: taskTitle.length > 0 ? "Move \"" + taskTitle + "\" to:" : "Move task to:"
        wrapMode: Text.WordWrap
        Accessible.name: text
    }

    ListView {
        id: taskListRows
        Layout.fillWidth: true
        Layout.preferredHeight: Math.min(contentHeight, 280)
        clip: true
        model: root.activeTaskLists

        delegate: RadioButton {
            required property string id
            required property string title
            width: ListView.view.width
            text: title
            enabled: id !== root.currentTaskListId
            checked: id === root.destinationTaskListId
            Accessible.name: title
            Accessible.description: enabled ? "Move task to this list" : "Current task list"
            onClicked: root.destinationTaskListId = id
        }
    }

    Label {
        Layout.fillWidth: true
        visible: taskListRows.count <= 1
        text: "No other active task lists."
        color: Theme.textSecondary
        horizontalAlignment: Text.AlignHCenter
    }
}
