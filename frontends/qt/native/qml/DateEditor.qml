import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property string value: ""
    property string accessibleName: "Date"
    property bool updating: false

    function daysInMonth(year, month) {
        return new Date(Date.UTC(year, month, 0)).getUTCDate()
    }
    function syncFromValue() {
        const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(value)
        if (match === null) return
        updating = true
        year.value = Number(match[1])
        month.value = Number(match[2])
        day.to = daysInMonth(year.value, month.value)
        day.value = Math.min(Number(match[3]), day.to)
        updating = false
    }
    function commit() {
        if (updating) return
        day.to = daysInMonth(year.value, month.value)
        if (day.value > day.to) day.value = day.to
        value = String(year.value).padStart(4, "0") + "-" + String(month.value).padStart(2, "0") +
                "-" + String(day.value).padStart(2, "0")
    }
    onValueChanged: syncFromValue()
    Component.onCompleted: syncFromValue()

    RowLayout {
        Layout.fillWidth: true
        Label { text: root.accessibleName; Layout.preferredWidth: 72 }
        SpinBox {
            id: year
            from: 1970
            to: 2100
            value: new Date().getFullYear()
            editable: true
            textFromValue: function(value) { return String(value) }
            valueFromText: function(text) { return Number(text.replace(/[^0-9]/g, "")) }
            Accessible.name: root.accessibleName + " year"
            onValueModified: root.commit()
        }
        SpinBox { id: month; from: 1; to: 12; value: 1; editable: true
                  Accessible.name: root.accessibleName + " month"; onValueModified: root.commit() }
        SpinBox { id: day; from: 1; to: 31; value: 1; editable: true
                  Accessible.name: root.accessibleName + " day"; onValueModified: root.commit() }
    }

    TextField {
        id: advancedDate
        Layout.fillWidth: true
        placeholderText: "Advanced ISO date (optional)"
        text: root.value
        selectByMouse: true
        Accessible.name: root.accessibleName + " advanced ISO date"
        onEditingFinished: {
            if (/^\d{4}-\d{2}-\d{2}$/.test(text.trim())) root.value = text.trim()
        }
    }
}
