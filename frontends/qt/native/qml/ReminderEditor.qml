import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property bool valid: true
    ListModel { id: reminderModel }

    function values() {
        const values = []
        for (let index = 0; index < reminderModel.count; ++index) {
            const reminder = reminderModel.get(index)
            if ((reminder.method !== "popup" && reminder.method !== "email") ||
                    !Number.isInteger(reminder.minutes) || reminder.minutes < 0 || reminder.minutes > 40320) {
                return null
            }
            values.push({ method: reminder.method, minutes: reminder.minutes })
        }
        return values
    }
    function setValues(values) {
        reminderModel.clear()
        if (!Array.isArray(values)) return
        for (let index = 0; index < values.length && index < 5; ++index) {
            const reminder = values[index]
            if (reminder && (reminder.method === "popup" || reminder.method === "email") &&
                    Number.isInteger(reminder.minutes) && reminder.minutes >= 0 && reminder.minutes <= 40320) {
                reminderModel.append({ method: reminder.method, minutes: reminder.minutes })
            }
        }
        advanced.text = values.map(function(reminder) { return reminder.method + ":" + reminder.minutes }).join(", ")
    }
    function parseAdvanced() {
        const text = advanced.text.trim()
        if (text.length === 0) { reminderModel.clear(); valid = true; return }
        const parsed = []
        const parts = text.split(/[\n,;]/).map(function(value) { return value.trim() })
            .filter(function(value) { return value.length > 0 })
        for (let index = 0; index < parts.length; ++index) {
            const match = /^(email|popup)\s*:\s*(\d+)$/.exec(parts[index])
            if (match === null || parsed.length >= 5 || Number(match[2]) > 40320) { valid = false; return }
            parsed.push({ method: match[1], minutes: Number(match[2]) })
        }
        valid = true
        setValues(parsed)
    }

    Repeater {
        model: reminderModel
        delegate: RowLayout {
            required property string method
            required property int minutes
            required property int index
            Layout.fillWidth: true
            enabled: root.enabled
            ComboBox {
                model: ["popup", "email"]
                currentIndex: indexOfValue(method)
                Accessible.name: "Reminder method"
                onActivated: reminderModel.setProperty(index, "method", currentValue)
            }
            SpinBox {
                from: 0
                to: 40320
                value: minutes
                editable: true
                Accessible.name: "Reminder minutes before event"
                onValueModified: reminderModel.setProperty(index, "minutes", value)
            }
            Label { text: "minutes before"; color: Theme.textSecondary }
            Button { text: "Remove"; onClicked: reminderModel.remove(index) }
        }
    }

    Button {
        text: "Add reminder"
        enabled: root.enabled && reminderModel.count < 5
        onClicked: reminderModel.append({ method: "popup", minutes: 10 })
    }

    TextField {
        id: advanced
        Layout.fillWidth: true
        enabled: root.enabled
        placeholderText: "Advanced Google reminders, e.g. popup:10, email:60"
        Accessible.name: "Advanced event reminders"
        selectByMouse: true
        onEditingFinished: root.parseAdvanced()
    }

    Label {
        visible: !root.valid
        text: "Advanced reminders must be popup or email entries with 0–40320 minutes."
        color: Theme.destructive
        wrapMode: Text.WordWrap
    }
}
