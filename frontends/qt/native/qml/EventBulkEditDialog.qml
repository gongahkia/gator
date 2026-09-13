import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: mode === "color" ? "Set event color" : mode === "availability" ? "Set availability" :
           mode === "visibility" ? "Set visibility" : "Shift events"
    primaryText: title
    primaryEnabled: eventIds.length > 0
    property string mode: "color"
    property var eventIds: []
    property alias colorPicker: colorPicker
    property alias availabilityPicker: availabilityPicker
    property alias visibilityPicker: visibilityPicker
    property alias shiftPicker: shiftPicker
    signal bulkColorRequested(var eventIds, string colorId)
    signal bulkAvailabilityRequested(var eventIds, bool available)
    signal bulkVisibilityRequested(var eventIds, string visibility)
    signal bulkShiftRequested(var eventIds, int shiftMinutes)

    function openForColor(ids) {
        mode = "color"
        eventIds = ids.slice()
        colorPicker.currentIndex = 0
        open()
    }

    function openForAvailability(ids) {
        mode = "availability"
        eventIds = ids.slice()
        availabilityPicker.currentIndex = 0
        open()
    }

    function openForVisibility(ids) {
        mode = "visibility"
        eventIds = ids.slice()
        visibilityPicker.currentIndex = 0
        open()
    }

    function openForShift(ids) {
        mode = "shift"
        eventIds = ids.slice()
        shiftPicker.currentIndex = 2
        open()
    }

    onPrimaryAction: {
        if (mode === "color") {
            bulkColorRequested(eventIds, colorPicker.currentValue)
        } else if (mode === "availability") {
            bulkAvailabilityRequested(eventIds, availabilityPicker.currentValue)
        } else if (mode === "visibility") {
            bulkVisibilityRequested(eventIds, visibilityPicker.currentValue)
        } else {
            bulkShiftRequested(eventIds, shiftPicker.currentValue)
        }
    }

    Label {
        Layout.fillWidth: true
        text: eventIds.length + " selected event" + (eventIds.length === 1 ? "" : "s")
        color: Theme.textSecondary
    }

    ComboBox {
        id: colorPicker
        Layout.fillWidth: true
        visible: root.mode === "color"
        model: [
            { label: "Color 1", value: "1" }, { label: "Color 2", value: "2" },
            { label: "Color 3", value: "3" }, { label: "Color 4", value: "4" },
            { label: "Color 5", value: "5" }, { label: "Color 6", value: "6" },
            { label: "Color 7", value: "7" }, { label: "Color 8", value: "8" },
            { label: "Color 9", value: "9" }, { label: "Color 10", value: "10" },
            { label: "Color 11", value: "11" }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Event color"
    }

    ComboBox {
        id: availabilityPicker
        Layout.fillWidth: true
        visible: root.mode === "availability"
        model: [{ label: "Busy", value: false }, { label: "Free", value: true }]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Event availability"
    }

    ComboBox {
        id: visibilityPicker
        Layout.fillWidth: true
        visible: root.mode === "visibility"
        model: [
            { label: "Default visibility", value: "default" },
            { label: "Private", value: "private" }, { label: "Public", value: "public" }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Event visibility"
    }

    ComboBox {
        id: shiftPicker
        Layout.fillWidth: true
        visible: root.mode === "shift"
        model: [
            { label: "Earlier one day", value: -1440 }, { label: "Earlier one hour", value: -60 },
            { label: "Later one hour", value: 60 }, { label: "Later one day", value: 1440 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Relative time shift"
    }
}
