import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Delete events"
    primaryText: "Delete events"
    primaryDestructive: true
    primaryEnabled: eventIds.length > 0
    property var eventIds: []
    signal bulkDeleteRequested(var eventIds)

    function openForDelete(ids) {
        eventIds = ids.slice()
        open()
    }

    onPrimaryAction: bulkDeleteRequested(eventIds)

    Label {
        Layout.fillWidth: true
        text: "Delete " + eventIds.length + " selected event" + (eventIds.length === 1 ? "" : "s") +
              "? Each eligible event is queued separately for Google sync."
        wrapMode: Text.WordWrap
        Accessible.name: text
    }
}
