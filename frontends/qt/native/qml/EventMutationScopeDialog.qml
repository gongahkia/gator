import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: operation === "resize" ? "Resize recurring event" : "Move recurring event"
    primaryText: operation === "resize" ? "Resize event" : "Move event"
    primaryEnabled: event.id !== undefined && String(event.id).length > 0
    property string operation: "move"
    property var event: ({})
    property string startAt: ""
    property string endAt: ""
    property bool allDay: false
    signal mutationConfirmed(string operation, var event, string startAt, string endAt, bool allDay,
                             int recurrenceScope)

    function scopeOptions() {
        const rule = event.recurrenceRule || ""
        const recurringId = event.recurringRemoteId || ""
        const originalStart = event.originalStartAt || ""
        if (recurringId.length > 0 || (rule.length > 0 && originalStart.length > 0)) {
            return [{ text: "This instance", value: 0 },
                    { text: "This and following", value: 1 },
                    { text: "Entire series", value: 2 }]
        }
        return [{ text: "Entire series", value: 2 }]
    }

    function openForMutation(kind, sourceEvent, newStartAt, newEndAt, newAllDay) {
        operation = kind
        event = sourceEvent || ({})
        startAt = newStartAt || ""
        endAt = newEndAt || ""
        allDay = newAllDay === true
        scopePicker.model = scopeOptions()
        scopePicker.currentIndex = 0
        open()
    }

    onPrimaryAction: mutationConfirmed(operation, event, startAt, endAt, allDay,
                                       scopePicker.currentValue)

    Label {
        Layout.fillWidth: true
        text: "Choose how this change applies to \"" + (root.event.title || "event") + "\"."
        wrapMode: Text.WordWrap
    }

    ComboBox {
        id: scopePicker
        Layout.fillWidth: true
        model: root.scopeOptions()
        textRole: "text"
        valueRole: "value"
        Accessible.name: "Recurrence change scope"
    }
}
