import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

ColumnLayout {
    id: root
    Layout.fillWidth: true
    visible: eventType.length > 0 && eventType !== "default"
    Layout.preferredHeight: visible ? implicitHeight : 0
    property string eventType: "default"
    property alias propertiesJson: advancedPropertiesField.text
    property bool syncing: false

    function propertyKey() {
        if (eventType === "focusTime") return "focusTimeProperties"
        if (eventType === "outOfOffice") return "outOfOfficeProperties"
        if (eventType === "workingLocation") return "workingLocationProperties"
        return ""
    }

    function objectFromJson(json) {
        try {
            const value = JSON.parse(json)
            return value !== null && typeof value === "object" && !Array.isArray(value) ? value : null
        } catch (error) {
            return null
        }
    }

    function selectValue(control, value, fallback) {
        const index = control.indexOfValue(value)
        control.currentIndex = index >= 0 ? index : fallback
    }

    function applyControls(value) {
        const key = propertyKey()
        if (value === null || key.length === 0 || value[key] === undefined ||
                value[key] === null || typeof value[key] !== "object" || Array.isArray(value[key])) return
        const properties = value[key]
        syncing = true
        selectValue(autoDeclinePicker, properties.autoDeclineMode, 0)
        declineMessageField.text = typeof properties.declineMessage === "string" ? properties.declineMessage : ""
        selectValue(chatStatusPicker, properties.chatStatus, 0)
        selectValue(workingLocationTypePicker, properties.type, 0)
        const custom = properties.customLocation || {}
        const office = properties.officeLocation || {}
        workingLocationLabelField.text = typeof custom.label === "string" ? custom.label
                                      : typeof office.label === "string" ? office.label : ""
        buildingIdField.text = typeof office.buildingId === "string" ? office.buildingId : ""
        floorIdField.text = typeof office.floorId === "string" ? office.floorId : ""
        floorSectionIdField.text = typeof office.floorSectionId === "string" ? office.floorSectionId : ""
        deskIdField.text = typeof office.deskId === "string" ? office.deskId : ""
        syncing = false
    }

    function updateFromControls() {
        if (syncing || eventType === "default") return
        let properties = {}
        if (eventType === "focusTime" || eventType === "outOfOffice") {
            properties.autoDeclineMode = autoDeclinePicker.currentValue
            if (declineMessageField.text.trim().length > 0) properties.declineMessage = declineMessageField.text.trim()
            if (eventType === "focusTime") properties.chatStatus = chatStatusPicker.currentValue
        } else if (eventType === "workingLocation") {
            properties.type = workingLocationTypePicker.currentValue
            if (properties.type === "customLocation") {
                properties.customLocation = {}
                if (workingLocationLabelField.text.trim().length > 0)
                    properties.customLocation.label = workingLocationLabelField.text.trim()
            } else if (properties.type === "officeLocation") {
                properties.officeLocation = {}
                if (buildingIdField.text.trim().length > 0) properties.officeLocation.buildingId = buildingIdField.text.trim()
                if (floorIdField.text.trim().length > 0) properties.officeLocation.floorId = floorIdField.text.trim()
                if (floorSectionIdField.text.trim().length > 0) properties.officeLocation.floorSectionId = floorSectionIdField.text.trim()
                if (deskIdField.text.trim().length > 0) properties.officeLocation.deskId = deskIdField.text.trim()
                if (workingLocationLabelField.text.trim().length > 0) properties.officeLocation.label = workingLocationLabelField.text.trim()
            } else {
                properties.homeOffice = {}
            }
        }
        const payload = {}
        payload[propertyKey()] = properties
        propertiesJson = JSON.stringify(payload)
    }

    function resetForEventType() {
        syncing = true
        autoDeclinePicker.currentIndex = autoDeclinePicker.indexOfValue("declineNone")
        declineMessageField.clear()
        chatStatusPicker.currentIndex = chatStatusPicker.indexOfValue("available")
        workingLocationTypePicker.currentIndex = workingLocationTypePicker.indexOfValue("homeOffice")
        workingLocationLabelField.clear()
        buildingIdField.clear()
        floorIdField.clear()
        floorSectionIdField.clear()
        deskIdField.clear()
        syncing = false
        if (eventType === "default" || propertyKey().length === 0) propertiesJson = "{}"
        else updateFromControls()
    }

    function load(json) {
        syncing = true
        propertiesJson = json && json.length > 0 ? json : "{}"
        applyControls(objectFromJson(propertiesJson))
        syncing = false
    }

    function validProperties() {
        const value = objectFromJson(propertiesJson)
        if (value === null) return false
        if (eventType === "default") return Object.keys(value).length === 0
        const key = propertyKey()
        return key.length > 0 && Object.keys(value).length === 1 && value[key] !== null &&
               typeof value[key] === "object" && !Array.isArray(value[key])
    }

    onEventTypeChanged: resetForEventType()

    Label {
        Layout.fillWidth: true
        text: "Status event options"
        font.bold: true
        visible: root.eventType !== "default"
    }

    ComboBox {
        id: autoDeclinePicker
        Layout.fillWidth: true
        visible: root.eventType === "focusTime" || root.eventType === "outOfOffice"
        model: [{ text: "Do not decline overlaps", value: "declineNone" },
                { text: "Decline all overlaps", value: "declineAllConflictingInvitations" },
                { text: "Decline new overlaps", value: "declineOnlyNewConflictingInvitations" }]
        textRole: "text"
        valueRole: "value"
        Accessible.name: "Automatic invitation decline"
        onCurrentValueChanged: root.updateFromControls()
    }

    ComboBox {
        id: chatStatusPicker
        Layout.fillWidth: true
        visible: root.eventType === "focusTime"
        model: [{ text: "Chat available", value: "available" },
                { text: "Chat do not disturb", value: "doNotDisturb" }]
        textRole: "text"
        valueRole: "value"
        Accessible.name: "Focus time chat status"
        onCurrentValueChanged: root.updateFromControls()
    }

    TextField {
        id: declineMessageField
        Layout.fillWidth: true
        visible: root.eventType === "focusTime" || root.eventType === "outOfOffice"
        placeholderText: "Automatic-decline message (optional)"
        Accessible.name: "Automatic-decline message"
        selectByMouse: true
        onTextChanged: root.updateFromControls()
    }

    ComboBox {
        id: workingLocationTypePicker
        Layout.fillWidth: true
        visible: root.eventType === "workingLocation"
        model: [{ text: "Home", value: "homeOffice" }, { text: "Office", value: "officeLocation" },
                { text: "Custom", value: "customLocation" }]
        textRole: "text"
        valueRole: "value"
        Accessible.name: "Working location type"
        onCurrentValueChanged: root.updateFromControls()
    }

    TextField {
        id: workingLocationLabelField
        Layout.fillWidth: true
        visible: root.eventType === "workingLocation" && workingLocationTypePicker.currentValue !== "homeOffice"
        placeholderText: workingLocationTypePicker.currentValue === "officeLocation" ? "Office label (optional)" : "Custom location label (optional)"
        Accessible.name: "Working location label"
        selectByMouse: true
        onTextChanged: root.updateFromControls()
    }

    TextField {
        id: buildingIdField
        Layout.fillWidth: true
        visible: root.eventType === "workingLocation" && workingLocationTypePicker.currentValue === "officeLocation"
        placeholderText: "Building ID (optional)"
        Accessible.name: "Office building ID"
        selectByMouse: true
        onTextChanged: root.updateFromControls()
    }

    TextField {
        id: floorIdField
        Layout.fillWidth: true
        visible: root.eventType === "workingLocation" && workingLocationTypePicker.currentValue === "officeLocation"
        placeholderText: "Floor ID (optional)"
        Accessible.name: "Office floor ID"
        selectByMouse: true
        onTextChanged: root.updateFromControls()
    }

    TextField {
        id: floorSectionIdField
        Layout.fillWidth: true
        visible: root.eventType === "workingLocation" && workingLocationTypePicker.currentValue === "officeLocation"
        placeholderText: "Floor section ID (optional)"
        Accessible.name: "Office floor section ID"
        selectByMouse: true
        onTextChanged: root.updateFromControls()
    }

    TextField {
        id: deskIdField
        Layout.fillWidth: true
        visible: root.eventType === "workingLocation" && workingLocationTypePicker.currentValue === "officeLocation"
        placeholderText: "Desk ID (optional)"
        Accessible.name: "Office desk ID"
        selectByMouse: true
        onTextChanged: root.updateFromControls()
    }

    Label {
        Layout.fillWidth: true
        text: "Advanced event-type properties"
        color: Theme.textSecondary
    }

    TextArea {
        id: advancedPropertiesField
        Layout.fillWidth: true
        Layout.preferredHeight: 72
        placeholderText: "Google Calendar event-type properties JSON"
        Accessible.name: "Advanced status event properties"
        selectByMouse: true
        wrapMode: TextEdit.Wrap
        text: "{}"
        onTextChanged: {
            if (!root.syncing) root.applyControls(root.objectFromJson(text))
        }
    }
}
