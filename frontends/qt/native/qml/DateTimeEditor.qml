import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    property string value: ""
    property bool allDay: false
    property string timeZone: ""
    property var timeZoneConverter: null
    property string accessibleName: "Date and time"
    property bool updating: false

    function daysInMonth(year, month) { return new Date(Date.UTC(year, month, 0)).getUTCDate() }
    function hasTimeZoneConversion() {
        return timeZoneConverter !== null && timeZoneConverter !== undefined &&
               typeof timeZoneConverter.dateTimeComponents === "function" &&
               typeof timeZoneConverter.dateTimeFromComponents === "function"
    }
    function timedComponents() {
        const parsed = new Date(value)
        if (!Number.isFinite(parsed.getTime())) return null
        if (hasTimeZoneConversion()) return timeZoneConverter.dateTimeComponents(value, timeZone)
        return { year: parsed.getFullYear(), month: parsed.getMonth() + 1, day: parsed.getDate(),
                 hour: parsed.getHours(), minute: parsed.getMinutes() }
    }
    function setComponents(components) {
        if (components === null || components.year === undefined || components.month === undefined ||
            components.day === undefined || components.hour === undefined || components.minute === undefined) return false
        updating = true
        year.value = components.year
        month.value = components.month
        day.to = daysInMonth(year.value, month.value)
        day.value = components.day
        hour.value = components.hour
        minute.value = components.minute
        updating = false
        return true
    }
    function syncFromValue() {
        const parsed = new Date(value)
        if (!Number.isFinite(parsed.getTime())) return
        if (!allDay && setComponents(timedComponents())) return
        setComponents({ year: parsed.getUTCFullYear(), month: parsed.getUTCMonth() + 1,
                        day: parsed.getUTCDate(), hour: 0, minute: 0 })
    }
    function commit() {
        if (updating) return
        day.to = daysInMonth(year.value, month.value)
        if (day.value > day.to) day.value = day.to
        const next = allDay
                     ? new Date(Date.UTC(year.value, month.value - 1, day.value, 0, 0, 0, 0)).toISOString()
                     : (hasTimeZoneConversion()
                        ? timeZoneConverter.dateTimeFromComponents(year.value, month.value - 1, day.value,
                                                                     hour.value, minute.value, timeZone)
                        : new Date(year.value, month.value - 1, day.value, hour.value, minute.value,
                                   0, 0).toISOString())
        if (next.length > 0) value = next
    }
    onValueChanged: syncFromValue()
    onTimeZoneChanged: syncFromValue()
    onAllDayChanged: {
        if (allDay) {
            if (setComponents(timedComponents())) commit()
        } else {
            const parsed = new Date(value)
            if (Number.isFinite(parsed.getTime()) &&
                    setComponents({ year: parsed.getUTCFullYear(), month: parsed.getUTCMonth() + 1,
                                    day: parsed.getUTCDate(), hour: 0, minute: 0 })) commit()
        }
    }
    Component.onCompleted: syncFromValue()

    RowLayout {
        Layout.fillWidth: true
        Label { text: root.accessibleName; Layout.preferredWidth: 72 }
        SpinBox { id: year; from: 1970; to: 2100; value: new Date().getFullYear(); editable: true
                  Accessible.name: root.accessibleName + " year"; onValueModified: root.commit() }
        SpinBox { id: month; from: 1; to: 12; value: 1; editable: true
                  Accessible.name: root.accessibleName + " month"; onValueModified: root.commit() }
        SpinBox { id: day; from: 1; to: 31; value: 1; editable: true
                  Accessible.name: root.accessibleName + " day"; onValueModified: root.commit() }
    }
    RowLayout {
        Layout.fillWidth: true
        visible: !root.allDay
        Label { text: "Time"; Layout.preferredWidth: 72 }
        SpinBox { id: hour; from: 0; to: 23; editable: true; Accessible.name: root.accessibleName + " hour"
                  onValueModified: root.commit() }
        Label { text: ":" }
        SpinBox { id: minute; from: 0; to: 59; editable: true; Accessible.name: root.accessibleName + " minute"
                  onValueModified: root.commit() }
    }
    TextField {
        id: advancedDateTime
        Layout.fillWidth: true
        placeholderText: "Advanced ISO 8601 (optional)"
        text: root.value
        selectByMouse: true
        Accessible.name: root.accessibleName + " advanced ISO time"
        onEditingFinished: {
            if (Number.isFinite(new Date(text.trim()).getTime())) root.value = text.trim()
        }
    }
}
