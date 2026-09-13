import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ComboBox {
    id: root
    property string colorId: ""
    model: [
        { label: "Default calendar color", value: "" }, { label: "Lavender", value: "1" },
        { label: "Sage", value: "2" }, { label: "Grape", value: "3" }, { label: "Flamingo", value: "4" },
        { label: "Banana", value: "5" }, { label: "Tangerine", value: "6" }, { label: "Peacock", value: "7" },
        { label: "Graphite", value: "8" }, { label: "Blueberry", value: "9" }, { label: "Basil", value: "10" },
        { label: "Tomato", value: "11" }
    ]
    textRole: "label"
    valueRole: "value"
    Layout.fillWidth: true
    Accessible.name: "Event color"
    Component.onCompleted: currentIndex = Math.max(0, indexOfValue(colorId))
    onColorIdChanged: { const index = indexOfValue(colorId); if (index >= 0) currentIndex = index }
    onActivated: root.colorId = currentValue || ""
}
