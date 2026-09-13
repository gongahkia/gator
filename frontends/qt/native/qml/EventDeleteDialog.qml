import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Delete event"
    primaryText: "Delete event"
    primaryDestructive: true
    primaryEnabled: eventId.length > 0
    property string eventId: ""
    property string eventTitle: ""
    property string recurrenceRule: ""
    property string recurringRemoteId: ""
    property string originalStartAt: ""
    signal eventDeleteRequested(string eventId, int recurrenceScope)

    function recurrenceScopeOptions() {
        if (recurringRemoteId.length > 0) {
            return [{ text: "This instance", value: 0 },
                    { text: "This and following", value: 1 },
                    { text: "Entire series", value: 2 }]
        }
        if (recurrenceRule.length > 0 && originalStartAt.length > 0) {
            return [{ text: "This instance", value: 0 },
                    { text: "This and following", value: 1 },
                    { text: "Entire series", value: 2 }]
        }
        if (recurrenceRule.length > 0) return [{ text: "Entire series", value: 2 }]
        return [{ text: "This event", value: 0 }]
    }

    function openForDelete(eventId, eventTitle, recurrenceRule, recurringRemoteId, originalStartAt) {
        root.eventId = eventId
        root.eventTitle = eventTitle
        root.recurrenceRule = recurrenceRule || ""
        root.recurringRemoteId = recurringRemoteId || ""
        root.originalStartAt = originalStartAt || ""
        recurrenceScopePicker.model = recurrenceScopeOptions()
        recurrenceScopePicker.currentIndex = 0
        open()
    }

    onPrimaryAction: eventDeleteRequested(eventId, recurrenceScopePicker.currentValue)

    Label {
        Layout.fillWidth: true
        text: eventTitle.length > 0 ? "Delete \"" + eventTitle + "\"?" : "Delete this event?"
        wrapMode: Text.WordWrap
        Accessible.name: text
    }

    ComboBox {
        id: recurrenceScopePicker
        Layout.fillWidth: true
        visible: root.recurrenceRule.length > 0 || root.recurringRemoteId.length > 0
        model: root.recurrenceScopeOptions()
        textRole: "text"
        valueRole: "value"
        Accessible.name: "Recurrence deletion scope"
    }
}
