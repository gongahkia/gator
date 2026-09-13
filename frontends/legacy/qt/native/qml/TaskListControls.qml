import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var taskListModel: null
    property bool loading: false
    property string errorMessage: ""
    property alias taskListRows: taskListRows
    property alias newTaskListButton: newTaskListButton
    property alias loadingLabel: loadingLabel
    property alias errorLabel: errorLabel
    property alias emptyLabel: emptyLabel
    signal taskListCreateRequested()
    signal taskListRenameRequested(string taskListId, string title)
    signal taskListDeleteRequested(string taskListId, string title, int taskCount, var taskTitles)
    signal taskListSelectionRequested(string taskListId, bool selected)

    padding: Theme.spacingSmall

    ColumnLayout {
        anchors.fill: parent
            spacing: Theme.spacingSmall

        RowLayout {
            Layout.fillWidth: true

            Label {
                text: "Lists"
                font.pixelSize: Theme.labelFontSize
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }

            Item { Layout.fillWidth: true }

            Button {
                id: newTaskListButton
                text: "New list"
                Accessible.name: text
                Accessible.description: "Create a Google Task list"
                onClicked: root.taskListCreateRequested()
            }
        }

        Label {
            id: loadingLabel
            Layout.fillWidth: true
            visible: root.loading
            text: "Loading task lists…"
            color: Theme.textSecondary
            Accessible.name: text
        }

        Label {
            id: errorLabel
            Layout.fillWidth: true
            visible: !root.loading && root.errorMessage.length > 0
            text: root.errorMessage
            color: Theme.destructive
            wrapMode: Text.WordWrap
            Accessible.name: "Task-list error: " + text
        }

        ListView {
            id: taskListRows
            Layout.fillWidth: true
            Layout.preferredHeight: Math.min(contentHeight, 360)
            clip: true
            visible: count > 0
            model: root.taskListModel

            delegate: Item {
                required property string id
                required property string title
                required property bool selected
                required property int taskCount
                required property var taskTitles
                width: taskListRows.width
                height: listRow.implicitHeight + Theme.spacingSmall
                property alias selectionCheck: selectionCheck
                property alias renameButton: renameButton
                property alias deleteButton: deleteButton

                RowLayout {
                    id: listRow
                    anchors.left: parent.left
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter

                    CheckBox {
                        id: selectionCheck
                        checked: selected
                        text: title + " (" + taskCount + ")"
                        Layout.fillWidth: true
                        Accessible.name: title
                        Accessible.description: checked ? "Task list selected" : "Task list deselected"
                        onToggled: root.taskListSelectionRequested(id, checked)
                    }

                    ToolButton {
                        text: "⋯"
                        Accessible.name: "List actions for " + title
                        onClicked: listMenu.open()

                        Menu {
                            id: listMenu
                            MenuItem { text: "Rename"; onTriggered: renameButton.click() }
                            MenuItem { text: "Delete"; onTriggered: deleteButton.click() }
                        }
                    }
                }

                Button {
                    id: renameButton
                    visible: false
                    text: "Rename"
                    Accessible.name: text + " " + title
                    onClicked: root.taskListRenameRequested(id, title)
                }

                Button {
                    id: deleteButton
                    visible: false
                    text: "Delete"
                    Accessible.name: text + " " + title
                    Accessible.description: "Delete this Google Task list and its tasks"
                    onClicked: root.taskListDeleteRequested(id, title, taskCount, taskTitles)
                }
            }
        }

        Label {
            id: emptyLabel
            Layout.fillWidth: true
            visible: !root.loading && taskListRows.count === 0 && root.errorMessage.length === 0
            text: "No task lists yet. Create one to add tasks."
            color: Theme.textSecondary
            horizontalAlignment: Text.AlignHCenter
            Accessible.name: text
        }

    }
}
