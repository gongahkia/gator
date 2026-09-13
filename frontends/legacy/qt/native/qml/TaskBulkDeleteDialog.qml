import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Delete tasks"
    primaryText: "Delete tasks"
    primaryDestructive: true
    primaryEnabled: taskIds.length > 0
    property var taskIds: []
    signal bulkDeleteRequested(var taskIds)

    function openForDelete(ids) {
        taskIds = ids.slice()
        open()
    }

    onPrimaryAction: bulkDeleteRequested(taskIds)

    Label {
        Layout.fillWidth: true
        text: "Delete " + taskIds.length + " selected task" + (taskIds.length === 1 ? "" : "s") +
              "? This queues one Google deletion per eligible task."
        wrapMode: Text.WordWrap
        Accessible.name: text
    }
}
