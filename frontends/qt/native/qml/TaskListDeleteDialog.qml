import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Delete task list"
    primaryText: "Delete list"
    primaryDestructive: true
    primaryEnabled: taskListId.length > 0
    property string taskListId: ""
    property string taskListTitle: ""
    property int taskCount: 0
    property var taskTitles: []
    property alias taskRows: taskRows
    signal taskListDeleteRequested(string taskListId)

    function openForDelete(taskListId, taskListTitle, taskCount, taskTitles) {
        root.taskListId = taskListId
        root.taskListTitle = taskListTitle
        root.taskCount = taskCount
        root.taskTitles = taskTitles || []
        open()
    }

    onPrimaryAction: taskListDeleteRequested(taskListId)

    Label {
        Layout.fillWidth: true
        text: taskListTitle.length > 0 ? "Delete \"" + taskListTitle + "\" and its " +
                                        taskCount + (taskCount === 1 ? " task?" : " tasks?")
                                      : "Delete this task list and its tasks?"
        wrapMode: Text.WordWrap
        Accessible.name: text
    }

    Label {
        Layout.fillWidth: true
        visible: taskRows.count > 0
        text: "Affected tasks"
        Accessible.role: Accessible.Heading
        Accessible.name: text
    }

    ListView {
        id: taskRows
        Layout.fillWidth: true
        Layout.preferredHeight: Math.min(contentHeight, 160)
        clip: true
        model: root.taskTitles

        delegate: Label {
            required property string modelData
            width: ListView.view.width
            text: "• " + modelData
            Accessible.name: "Affected task " + modelData
        }
    }

    Label {
        Layout.fillWidth: true
        visible: taskCount > taskRows.count
        text: "and " + (taskCount - taskRows.count) + " more"
        color: Theme.textSecondary
        Accessible.name: text
    }
}
