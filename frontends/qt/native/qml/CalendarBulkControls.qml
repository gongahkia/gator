import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var selectedEventIds: []
    property var calendarSourceModel: null
    property string statusMessage: ""
    property string previewMessage: ""
    property int previewRequestToken: -1
    property int bulkTextRecurrenceScope: 2
    signal clearSelectionRequested()
    signal bulkDeleteRequested(var eventIds)
    signal bulkMoveRequested(var eventIds, string calendarId)
    signal bulkColorRequested(var eventIds, string colorId)
    signal bulkAvailabilityRequested(var eventIds, bool available)
    signal bulkVisibilityRequested(var eventIds, string visibility)
    signal bulkShiftRequested(var eventIds, int shiftMinutes)
    signal bulkTextPreviewRequested(var eventIds, string findText, int fields, int recurrenceScope,
                                    int requestToken)
    signal bulkTextReplaceRequested(var eventIds, string findText, string replaceText, int fields,
                                   int recurrenceScope)

    visible: selectedEventIds.length > 0 || statusMessage.length > 0
    padding: Theme.spacingMedium

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingSmall

        Flow {
            Layout.fillWidth: true
            spacing: Theme.spacingSmall
            visible: root.selectedEventIds.length > 0

            Label {
                text: root.selectedEventIds.length + " selected · eligibility checked before queueing"
                color: Theme.textSecondary
                Accessible.name: text
            }

            Button {
                text: "Clear selection"
                Accessible.name: text
                onClicked: root.clearSelectionRequested()
            }

            Button {
                text: "Delete"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: deleteDialog.openForDelete(root.selectedEventIds)
            }

            Button {
                text: "Move"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: moveDialog.openForMove(root.selectedEventIds)
            }

            Button {
                text: "Set color"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: editDialog.openForColor(root.selectedEventIds)
            }

            Button {
                text: "Set availability"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: editDialog.openForAvailability(root.selectedEventIds)
            }

            Button {
                text: "Set visibility"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: editDialog.openForVisibility(root.selectedEventIds)
            }

            Button {
                text: "Shift time"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: editDialog.openForShift(root.selectedEventIds)
            }

            Button {
                text: "Find and replace"
                Accessible.name: text + " " + root.selectedEventIds.length + " events"
                onClicked: bulkTextReplaceDialog.openFor(root.selectedEventIds,
                                                         root.bulkTextRecurrenceScope)
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
    }

    EventBulkDeleteDialog {
        id: deleteDialog
        parent: Overlay.overlay
        onBulkDeleteRequested: function(eventIds) { root.bulkDeleteRequested(eventIds) }
    }

    EventBulkMoveDialog {
        id: moveDialog
        parent: Overlay.overlay
        calendarSourceModel: root.calendarSourceModel
        onBulkMoveRequested: function(eventIds, calendarId) {
            root.bulkMoveRequested(eventIds, calendarId)
        }
    }

    EventBulkEditDialog {
        id: editDialog
        parent: Overlay.overlay
        onBulkColorRequested: function(eventIds, colorId) { root.bulkColorRequested(eventIds, colorId) }
        onBulkAvailabilityRequested: function(eventIds, available) {
            root.bulkAvailabilityRequested(eventIds, available)
        }
        onBulkVisibilityRequested: function(eventIds, visibility) {
            root.bulkVisibilityRequested(eventIds, visibility)
        }
        onBulkShiftRequested: function(eventIds, shiftMinutes) {
            root.bulkShiftRequested(eventIds, shiftMinutes)
        }
    }

    BulkTextReplaceDialog {
        id: bulkTextReplaceDialog
        parent: Overlay.overlay
        kind: "event"
        previewMessage: root.previewMessage
        previewResultRequestToken: root.previewRequestToken
        onPreviewRequested: function(eventIds, findText, fields, recurrenceScope, requestToken) {
            root.bulkTextPreviewRequested(eventIds, findText, fields, recurrenceScope, requestToken)
        }
        onReplaceRequested: function(eventIds, findText, replaceText, fields, recurrenceScope) {
            root.bulkTextReplaceRequested(eventIds, findText, replaceText, fields, recurrenceScope)
        }
    }
}
