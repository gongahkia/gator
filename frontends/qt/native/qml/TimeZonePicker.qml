import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ComboBox {
    id: root
    property string timeZone: ""
    property var timeZones: ["", "UTC", "America/Los_Angeles", "America/New_York", "Asia/Singapore", "Asia/Tokyo", "Australia/Sydney", "Europe/London", "Europe/Paris"]
    model: timeZones
    editable: true
    Layout.fillWidth: true
    Accessible.name: "Time zone"
    Component.onCompleted: currentIndex = indexOfValue(timeZone)
    onTimeZoneChanged: {
        const index = indexOfValue(timeZone)
        if (index >= 0 && currentIndex !== index) currentIndex = index
        else if (editText !== timeZone) editText = timeZone
    }
    onActivated: root.timeZone = currentValue || ""
    onEditTextChanged: root.timeZone = editText.trim()
}
