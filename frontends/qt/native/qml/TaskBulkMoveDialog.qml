import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Move tasks"
    primaryText: "Move tasks"
    primaryEnabled: taskIds.length > 0 && destinationTaskListId.length > 0
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
    property var taskIds: []
    property string destinationTaskListId: ""
    signal bulkMoveRequested(var taskIds, string taskListId)

    function openForMove(ids) {
        taskIds = ids.slice()
        destinationTaskListId = ""
        open()
    }

    onPrimaryAction: bulkMoveRequested(taskIds, destinationTaskListId)

    Label {
        Layout.fillWidth: true
        text: "Move " + taskIds.length + " selected task" + (taskIds.length === 1 ? "" : "s") + " to:"
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
            checked: id === root.destinationTaskListId
            Accessible.name: title
            Accessible.description: "Move selected tasks to this list"
            onClicked: root.destinationTaskListId = id
        }
    }
}
