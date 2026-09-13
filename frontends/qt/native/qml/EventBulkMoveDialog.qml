import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Move events"
    primaryText: "Move events"
    primaryEnabled: eventIds.length > 0 && destinationCalendarId.length > 0
    property var calendarSourceModel: null
    property var eventIds: []
    property string destinationCalendarId: ""
    signal bulkMoveRequested(var eventIds, string calendarId)

    function openForMove(ids) {
        eventIds = ids.slice()
        destinationCalendarId = ""
        open()
    }

    onPrimaryAction: bulkMoveRequested(eventIds, destinationCalendarId)

    Label {
        Layout.fillWidth: true
        text: "Move " + eventIds.length + " selected event" + (eventIds.length === 1 ? "" : "s") + " to:"
        wrapMode: Text.WordWrap
        Accessible.name: text
    }

    ListView {
        id: calendarRows
        Layout.fillWidth: true
        Layout.preferredHeight: Math.min(contentHeight, 280)
        clip: true
        model: root.calendarSourceModel

        delegate: RadioButton {
            required property string id
            required property string title
            required property string accessRole
            width: ListView.view.width
            text: title + (accessRole === "owner" || accessRole.length === 0 ? "" : " (read-only)")
            enabled: accessRole === "owner" || accessRole.length === 0
            checked: id === root.destinationCalendarId
            Accessible.name: title
            Accessible.description: enabled ? "Move selected events to this calendar"
                                            : "Read-only calendar"
            onClicked: root.destinationCalendarId = id
        }
    }
}
