import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    property string taskId: ""
    property string taskTitle: ""
    property int action: 0
    title: action === 0 ? "Stop repeating" : "Start a new series"
    primaryText: action === 0 ? "Stop repeating" : "Split series"
    primaryDestructive: action === 0
    primaryEnabled: taskId.length > 0
    signal recurrenceActionRequested(string taskId, int action, int scope)

    function openForAction(taskId, taskTitle, action) {
        root.taskId = taskId
        root.taskTitle = taskTitle
        root.action = action
        scopePicker.currentIndex = action === 0 ? 0 : 1
        open()
    }

    onPrimaryAction: recurrenceActionRequested(taskId, action, scopePicker.currentValue)

    Label {
        Layout.fillWidth: true
        text: action === 0
              ? "Stop HCB-managed recurrence for \"" + taskTitle + "\". The task remains in Google Tasks."
              : "Split \"" + taskTitle + "\" and its following occurrences into a new HCB-managed series."
        wrapMode: Text.WordWrap
        Accessible.name: text
    }

    ComboBox {
        id: scopePicker
        Layout.fillWidth: true
        visible: root.action === 0
        model: [
            { label: "This occurrence", value: 0 },
            { label: "This and following", value: 1 },
            { label: "Entire series", value: 2 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Task recurrence scope"
    }

    Label {
        Layout.fillWidth: true
        visible: root.action === 1
        text: "Google Tasks has no recurrence API. HCB will write a new visible marker series for this occurrence and following ones."
        color: Theme.textSecondary
        wrapMode: Text.WordWrap
        Accessible.name: text
    }
}
