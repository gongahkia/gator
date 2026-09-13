import QtQuick
import QtQuick.Controls

Button {
    id: root
    property string accessibleName: ""
    property string accessibleDescription: ""

    activeFocusOnTab: true
    Accessible.name: root.accessibleName === "" ? root.text : root.accessibleName
    Accessible.description: root.accessibleDescription
    Accessible.role: Accessible.Button
    Accessible.focusable: true
    Accessible.focused: root.activeFocus
    Accessible.onPressAction: root.click()
}
