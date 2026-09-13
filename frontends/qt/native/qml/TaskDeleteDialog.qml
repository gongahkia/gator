import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Delete task"
    primaryText: "Delete task"
    primaryDestructive: true
    primaryEnabled: taskId.length > 0
    property string taskId: ""
    property string taskTitle: ""
    property bool taskManagedRecurrence: false
    signal taskDeleteRequested(string taskId)

    function openForDelete(taskId, taskTitle, taskManagedRecurrence) {
        root.taskId = taskId
        root.taskTitle = taskTitle
        root.taskManagedRecurrence = taskManagedRecurrence === true
        open()
    }

    onPrimaryAction: taskDeleteRequested(taskId)

    Label {
        Layout.fillWidth: true
        text: taskTitle.length > 0 ? "Delete \"" + taskTitle + "\"?" : "Delete this task?"
        wrapMode: Text.WordWrap
        Accessible.name: text
    }

    Label {
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        text: "This deletes only this Google Task. Use Stop repeating to change the HCB-managed series."
        color: Theme.textSecondary
        wrapMode: Text.WordWrap
        Accessible.name: text
    }
}
