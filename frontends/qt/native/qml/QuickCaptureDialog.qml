import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Quick Capture"
    primaryText: captureKind === 1 ? "Create event" : "Create task"
    primaryEnabled: titleField.text.trim().length > 0 && destinationId().length > 0 &&
                    (captureKind === 0 || preview.eventReady === true)
    property var appController: null
    property var taskListModel: null
    property var calendarSourceModel: null
    property string defaultTaskListId: ""
    property string defaultCalendarId: ""
    property int taskListRevision: taskListModel !== null && taskListModel.revision !== undefined
                                   ? taskListModel.revision : 0
    property int calendarSourceRevision: calendarSourceModel !== null && calendarSourceModel.revision !== undefined
                                        ? calendarSourceModel.revision : 0
    property int captureKind: 1 // event by default
    property var disabledRecognitionIds: []
    property var preview: ({ kind: 1, savedTitle: "", eventReady: false, recognitions: [] })
    property alias taskTitle: titleField.text
    property alias taskTitleField: titleField
    property alias taskDestinationPicker: taskDestinationPicker
    property alias eventDestinationPicker: eventDestinationPicker
    signal taskRequested(string title)
    signal captureRequested(string title, int kind, string destinationId, var disabledRecognitionIds)

    function activeTaskLists() {
        const revision = taskListRevision
        if (taskListModel !== null && typeof taskListModel.selectedTaskLists === "function") {
            return taskListModel.selectedTaskLists()
        }
        return []
    }

    function destinationId() {
        return captureKind === 0 ? (taskDestinationPicker.currentValue || "")
                                 : (eventDestinationPicker.currentValue || "")
    }

    function selectDefaultDestination() {
        const calendarRevision = calendarSourceRevision
        if (captureKind === 0) {
            taskDestinationPicker.currentIndex = taskDestinationPicker.indexOfValue(defaultTaskListId)
        } else {
            eventDestinationPicker.currentIndex = eventDestinationPicker.indexOfValue(defaultCalendarId)
        }
    }

    function refreshPreview() {
        if (appController !== null && typeof appController.previewQuickCapture === "function") {
            const next = appController.previewQuickCapture(titleField.text, captureKind, disabledRecognitionIds)
            preview = next || ({ kind: captureKind, savedTitle: titleField.text, eventReady: captureKind === 0,
                                 recognitions: [] })
            if (typeof preview.kind === "number" && preview.kind !== captureKind) {
                captureKind = preview.kind
            }
            return
        }
        preview = { kind: captureKind, rawTitle: titleField.text, savedTitle: titleField.text,
                    parsedTitle: titleField.text, eventReady: captureKind === 0, recognitions: [] }
    }

    function setCaptureKind(kind) {
        if (captureKind === kind) return
        captureKind = kind
        disabledRecognitionIds = []
        selectDefaultDestination()
        refreshPreview()
    }

    onOpened: {
        captureKind = 1
        disabledRecognitionIds = []
        titleField.clear()
        selectDefaultDestination()
        refreshPreview()
        titleField.forceActiveFocus()
    }
    onCaptureKindChanged: {
        selectDefaultDestination()
        refreshPreview()
    }
    onPrimaryAction: {
        taskRequested(titleField.text.trim())
        captureRequested(titleField.text.trim(), captureKind, destinationId(), disabledRecognitionIds)
    }

    RowLayout {
        Layout.fillWidth: true

        Button {
            text: "Event"
            checkable: true
            checked: root.captureKind === 1
            Accessible.name: "Create an event"
            onClicked: root.setCaptureKind(1)
        }

        Button {
            text: "Task"
            checkable: true
            checked: root.captureKind === 0
            Accessible.name: "Create a task"
            onClicked: root.setCaptureKind(0)
        }

        Item { Layout.fillWidth: true }

        Label {
            text: root.captureKind === 1 ? "Event" : "Task"
            color: Theme.textSecondary
        }
    }

    EmojiTextField {
        id: titleField
        Layout.fillWidth: true
        placeholderText: root.captureKind === 1 ? "Team sync tomorrow at 9am" : "Call Sam tomorrow P1"
        Accessible.name: root.captureKind === 1 ? "Event quick capture" : "Task quick capture"
        selectByMouse: true
        onTextChanged: {
            root.disabledRecognitionIds = []
            root.refreshPreview()
        }
        Keys.onReturnPressed: {
            if (root.primaryEnabled) root.primaryButton.click()
        }
    }

    ComboBox {
        id: taskDestinationPicker
        Layout.fillWidth: true
        visible: root.captureKind === 0
        model: root.activeTaskLists()
        textRole: "title"
        valueRole: "id"
        displayText: currentIndex >= 0 ? currentText : "Choose Google Task list"
        Accessible.name: "Quick capture Google Task list"
    }

    ComboBox {
        id: eventDestinationPicker
        Layout.fillWidth: true
        visible: root.captureKind === 1
        model: root.calendarSourceModel
        textRole: "title"
        valueRole: "id"
        displayText: currentIndex >= 0 ? currentText : "Choose Google Calendar"
        Accessible.name: "Quick capture Google Calendar"
    }

    Label {
        Layout.fillWidth: true
        visible: root.captureKind === 1 && preview.eventReady !== true
        text: "Add a date or time, or switch to Task for an unscheduled item."
        wrapMode: Text.WordWrap
        color: Theme.textSecondary
    }

    Label {
        Layout.fillWidth: true
        visible: preview.savedTitle !== undefined && preview.savedTitle.length > 0 &&
                 preview.savedTitle !== titleField.text.trim()
        text: "Saved title: " + preview.savedTitle
        wrapMode: Text.WordWrap
        color: Theme.textSecondary
    }

    Flow {
        Layout.fillWidth: true
        spacing: Theme.spacingSmall
        visible: Array.isArray(preview.recognitions) && preview.recognitions.length > 0

        Repeater {
            model: Array.isArray(root.preview.recognitions) ? root.preview.recognitions : []
            delegate: Button {
                required property var modelData
                text: modelData.removable ? modelData.label + " ×" : modelData.label
                flat: true
                enabled: modelData.removable === true
                Accessible.name: modelData.removable ? "Ignore " + modelData.label : modelData.label
                onClicked: {
                    const next = root.disabledRecognitionIds.slice()
                    if (next.indexOf(modelData.id) < 0) next.push(modelData.id)
                    root.disabledRecognitionIds = next
                    root.refreshPreview()
                }
            }
        }
    }
}
