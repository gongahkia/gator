import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var tasks: []
    property bool expanded: false
    signal taskDetailRequested(var task)

    readonly property var filteredTasks: {
        const query = searchField.text.trim().toLowerCase()
        if (query.length === 0) return tasks
        return tasks.filter(function(task) {
            return (task.title || "").toLowerCase().indexOf(query) >= 0 ||
                   (task.taskListTitle || "").toLowerCase().indexOf(query) >= 0
        })
    }

    padding: Theme.spacingSmall
    implicitWidth: expanded ? 248 : collapseButton.implicitWidth

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingSmall

        RowLayout {
            Layout.fillWidth: true
            Button {
                id: collapseButton
                text: root.expanded ? "Hide tasks" : "Tasks (" + root.tasks.length + ")"
                onClicked: root.expanded = !root.expanded
            }
        }

        TextField {
            id: searchField
            Layout.fillWidth: true
            visible: root.expanded
            placeholderText: "Filter unscheduled tasks"
            Accessible.name: placeholderText
        }

        Label {
            Layout.fillWidth: true
            visible: root.expanded && root.filteredTasks.length === 0
            text: root.tasks.length === 0 ? "No unscheduled tasks" : "No matching tasks"
            color: Theme.textSecondary
            wrapMode: Text.WordWrap
        }

        ListView {
            Layout.fillWidth: true
            Layout.fillHeight: true
            Layout.preferredHeight: root.expanded ? 280 : 0
            visible: root.expanded
            clip: true
            spacing: 4
            model: root.filteredTasks
            delegate: Button {
                required property var modelData
                property string taskId: modelData.id
                property var taskData: modelData
                width: ListView.view.width
                text: modelData.title
                display: AbstractButton.TextOnly
                Accessible.name: modelData.title + ", unscheduled task"
                onClicked: root.taskDetailRequested(modelData)

                Drag.active: taskDrag.active
                Drag.keys: ["hcb-task"]
                Drag.hotSpot.x: width / 2
                Drag.hotSpot.y: height / 2

                DragHandler {
                    id: taskDrag
                    target: null
                    onActiveChanged: if (!active) root.forceActiveFocus()
                }
            }
        }
    }
}
