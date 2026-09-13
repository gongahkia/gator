import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var invitations: []
    signal responseRequested(string eventId, string responseStatus, string comment)

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingMedium

        Label {
            text: "Invitations" + (Array.isArray(root.invitations) && root.invitations.length > 0
                                    ? " (" + root.invitations.length + ")" : "")
            font.pixelSize: Theme.titleFontSize
            Accessible.role: Accessible.Heading
            Accessible.name: text
        }

        ListView {
            id: invitationRows
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: Theme.spacingMedium
            model: root.invitations

            delegate: Frame {
                required property var modelData
                width: ListView.view.width

                ColumnLayout {
                    anchors.fill: parent
                    spacing: Theme.spacingSmall

                    Label {
                        Layout.fillWidth: true
                        text: modelData.title
                        font.bold: true
                        wrapMode: Text.WordWrap
                    }
                    Label {
                        Layout.fillWidth: true
                        text: modelData.allDay ? "All day · " + modelData.startAt.slice(0, 10) : modelData.startAt
                        color: Theme.textSecondary
                    }
                    EmojiTextArea {
                        id: comment
                        Layout.fillWidth: true
                        Layout.preferredHeight: 64
                        placeholderText: "RSVP comment (optional)"
                        text: modelData.comment || ""
                        selectByMouse: true
                        wrapMode: TextEdit.Wrap
                        Accessible.name: "RSVP comment for " + modelData.title
                    }
                    RowLayout {
                        Layout.fillWidth: true
                        Button {
                            text: "Accept"
                            onClicked: root.responseRequested(modelData.eventId, "accepted", comment.text.trim())
                        }
                        Button {
                            text: "Tentative"
                            onClicked: root.responseRequested(modelData.eventId, "tentative", comment.text.trim())
                        }
                        Button {
                            text: "Decline"
                            palette.button: Theme.destructive
                            onClicked: root.responseRequested(modelData.eventId, "declined", comment.text.trim())
                        }
                    }
                }
            }
        }

        Label {
            Layout.fillWidth: true
            visible: invitationRows.count === 0
            text: "No invitations need a response."
            color: Theme.textSecondary
            horizontalAlignment: Text.AlignHCenter
        }
    }
}
