import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var notesModel: null
    property bool loading: false
    property string statusMessage: ""
    property alias noteRows: noteRows
    property alias noteCreateButton: noteCreateButton
    signal noteCreateRequested()
    signal noteEditRequested(string taskId, string taskListId, string title, string body)
    signal noteCompletionRequested(string taskId, bool completed)
    signal noteDeleteRequested(string taskId, string title)
    signal noteMoveRequested(string taskId, string taskListId, string title)

    function selectNote(noteId) {
        for (let index = 0; index < noteRows.count; ++index) {
            const item = noteRows.itemAtIndex(index)
            if (item !== null && item.noteId === noteId) {
                noteRows.currentIndex = index
                item.forceActiveFocus()
                return
            }
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingMedium

        Label {
            text: "Notes"
            font.pixelSize: Theme.titleFontSize
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        RowLayout {
            Layout.fillWidth: true

            Item { Layout.fillWidth: true }

            Button {
                id: noteCreateButton
                text: "New note"
                enabled: !root.loading
                Accessible.name: text
                Accessible.description: "Create an undated Google Task shown as a note"
                onClicked: root.noteCreateRequested()
            }
        }

        Label {
            Layout.fillWidth: true
            visible: root.statusMessage.length > 0
            text: root.statusMessage
            color: Theme.textSecondary
            wrapMode: Text.WordWrap
            Accessible.name: text
        }

        ListView {
            id: noteRows
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: root.notesModel
            spacing: Theme.spacingSmall

            delegate: RowLayout {
                required property string id
                required property string taskListId
                required property string taskListTitle
                required property string title
                required property string body
                required property bool completed
                width: ListView.view.width
                property string noteId: id

                Button {
                    text: completed ? "Reopen" : "Complete"
                    enabled: !root.loading
                    Accessible.name: text + " " + title
                    onClicked: root.noteCompletionRequested(id, !completed)
                }

                Button {
                    text: "Edit"
                    enabled: !root.loading
                    Accessible.name: text + " " + title
                    onClicked: root.noteEditRequested(id, taskListId, title, body)
                }

                Button {
                    text: "Move"
                    enabled: !root.loading
                    Accessible.name: text + " " + title
                    onClicked: root.noteMoveRequested(id, taskListId, title)
                }

                Button {
                    text: "Delete"
                    enabled: !root.loading
                    Accessible.name: text + " " + title
                    onClicked: root.noteDeleteRequested(id, title)
                }

                AccessibleButton {
                    Layout.fillWidth: true
                    text: (completed ? "✓ " : "") + title + "\n" + taskListTitle +
                          (body.length > 0 ? " — " + body : "")
                    accessibleName: title
                    accessibleDescription: taskListTitle + (body.length > 0 ? ". " + body : "")
                    onClicked: root.noteEditRequested(id, taskListId, title, body)
                }
            }
        }

        Label {
            Layout.fillWidth: true
            visible: noteRows.count === 0
            text: root.loading ? "Loading notes…" : "No undated tasks to show as notes."
            color: Theme.textSecondary
            horizontalAlignment: Text.AlignHCenter
        }
    }
}
