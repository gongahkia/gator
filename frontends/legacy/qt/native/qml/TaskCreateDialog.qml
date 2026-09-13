import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "New task"
    primaryText: "Create task"
    primaryEnabled: taskListId.length > 0 && titleField.text.trim().length > 0 && recurrenceInputValid()
    property var taskListModel: null
    property int taskListRevision: taskListModel !== null && taskListModel.revision !== undefined
                                   ? taskListModel.revision : 0
    property var activeTaskLists: {
        const revision = taskListRevision
        if (taskListModel !== null && typeof taskListModel.selectedTaskLists === "function") {
            return taskListModel.selectedTaskLists()
        }
        return taskListModel
    }
    property string taskListId: ""
    property string parentTaskId: ""
    property var timeZones: ["", "UTC", "America/Los_Angeles", "America/New_York", "Asia/Singapore", "Europe/London"]
    property alias taskTitle: titleField.text
    property alias taskListPicker: taskListPicker
    property alias taskTitleField: titleField
    property alias taskNotes: notesField.text
    property alias taskDueAt: dueField.value
    property alias managedRecurrenceCheck: recurrenceCheck
    property alias recurrenceFrequencyPicker: frequencyPicker
    property alias recurrenceIntervalField: intervalField
    property alias recurrenceEndPicker: endPicker
    property alias recurrenceEndUntilField: untilField
    property alias recurrenceEndCountField: countField
    property alias recurrenceRuleField: ruleField
    property alias recurrenceExclusionDatesField: exclusionDatesField
    property alias recurrenceAdditionDatesField: additionDatesField
    signal taskCreateRequested(string taskListId, string parentTaskId, string title, string notes,
                               string dueAt, string dueTimeZone, int priority, bool managedRecurrence,
                               int recurrenceFrequency, int recurrenceInterval, int recurrenceEndKind,
                               string recurrenceEndUntil, int recurrenceEndCount, string recurrenceRule,
                               string exclusionDates, string additionDates)

    function recurrenceInputValid() {
        if (!recurrenceCheck.checked) return true
        if (!/^\d{4}-\d{2}-\d{2}$/.test(dueField.value.trim())) return false
        const interval = Number(intervalField.text)
        if (!Number.isInteger(interval) || interval < 1 || interval > 1000) return false
        if (endPicker.currentValue === 1) return /^\d{4}-\d{2}-\d{2}$/.test(untilField.text.trim())
        if (endPicker.currentValue === 2) {
            const count = Number(countField.text)
            return Number.isInteger(count) && count >= 1 && count <= 10000
        }
        return true
    }

    function openForCreate(initialTaskListId, initialParentTaskId) {
        titleField.clear()
        notesField.clear()
        dueField.value = ""
        timeZonePicker.timeZone = ""
        priorityPicker.currentIndex = 0
        recurrenceCheck.checked = false
        frequencyPicker.currentIndex = 1
        intervalField.text = "1"
        endPicker.currentIndex = 0
        untilField.clear()
        countField.text = "1"
        ruleField.clear()
        exclusionDatesField.clear()
        additionDatesField.clear()
        taskListPicker.currentIndex = taskListPicker.indexOfValue(initialTaskListId || "")
        if (taskListPicker.currentIndex < 0 && taskListPicker.count > 0) {
            taskListPicker.currentIndex = 0
        }
        taskListId = taskListPicker.currentValue || ""
        parentTaskId = initialParentTaskId || ""
        open()
    }

    onOpened: titleField.forceActiveFocus()
    onPrimaryAction: taskCreateRequested(taskListId, parentTaskId, titleField.text.trim(), notesField.text,
                                         dueField.value.trim(), timeZonePicker.timeZone.trim(), priorityPicker.currentValue,
                                         recurrenceCheck.checked, frequencyPicker.currentValue,
                                         Number(intervalField.text), endPicker.currentValue,
                                         untilField.text.trim(), Number(countField.text),
                                         ruleField.text.trim(), exclusionDatesField.text.trim(),
                                         additionDatesField.text.trim())

    EmojiTextField {
        id: titleField
        Layout.fillWidth: true
        placeholderText: "Task title"
        Accessible.name: "Task title"
        selectByMouse: true
        Keys.onReturnPressed: {
            if (root.primaryEnabled) {
                root.primaryButton.click()
            }
        }
    }

    EmojiTextArea {
        id: notesField
        Layout.fillWidth: true
        Layout.preferredHeight: 100
        placeholderText: "Notes"
        Accessible.name: "Task notes"
        selectByMouse: true
        wrapMode: TextEdit.Wrap
    }

    DateEditor {
        id: dueField
        Layout.fillWidth: true
        accessibleName: "Task due date"
    }

    TimeZonePicker { id: timeZonePicker; timeZones: root.timeZones }

    ComboBox {
        id: priorityPicker
        Layout.fillWidth: true
        model: [
            { label: "No priority", value: 0 },
            { label: "Low priority", value: 1 },
            { label: "Medium priority", value: 2 },
            { label: "High priority", value: 3 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Task priority"
    }

    CheckBox {
        id: recurrenceCheck
        text: "Repeat task with HCB"
        enabled: root.parentTaskId.length === 0
        Accessible.name: text
        Accessible.description: enabled ? "Adds a visible HCB recurrence marker to Google Task notes"
                                       : "Subtasks cannot use managed recurrence"
    }

    ComboBox {
        id: frequencyPicker
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        model: [
            { label: "Daily", value: 0 },
            { label: "Weekly", value: 1 },
            { label: "Monthly", value: 2 },
            { label: "Yearly", value: 3 },
            { label: "Business days (Mon–Fri)", value: 4 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Task repeat frequency"
    }

    TextField {
        id: intervalField
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        placeholderText: "Repeat interval"
        inputMethodHints: Qt.ImhDigitsOnly
        Accessible.name: "Task repeat interval"
        selectByMouse: true
    }

    RowLayout {
        id: weekdayChecks
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        function selectedDays() {
            const days = []
            for (let index = 0; index < weekdayRepeater.count; ++index) {
                if (weekdayRepeater.itemAt(index).checked) days.push(weekdayRepeater.itemAt(index).day)
            }
            return days
        }
        Repeater {
            id: weekdayRepeater
            model: [{ label: "Mon", value: "MO" }, { label: "Tue", value: "TU" }, { label: "Wed", value: "WE" },
                    { label: "Thu", value: "TH" }, { label: "Fri", value: "FR" }, { label: "Sat", value: "SA" },
                    { label: "Sun", value: "SU" }]
            delegate: CheckBox { required property var modelData; property string day: modelData.value
                                 text: modelData.label; Accessible.name: "Repeat on " + text }
        }
    }

    RowLayout {
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        ComboBox {
            id: monthlyOrdinal
            model: [{ text: "Monthly date", value: "" }, { text: "First weekday", value: "1" },
                    { text: "Second weekday", value: "2" }, { text: "Third weekday", value: "3" },
                    { text: "Fourth weekday", value: "4" }, { text: "Last weekday", value: "-1" }]
            textRole: "text"; valueRole: "value"; Accessible.name: "Monthly repeat pattern"
        }
        Button {
            text: "Apply structured rule"
            onClicked: {
                const interval = Math.max(1, Number(intervalField.text) || 1)
                const days = weekdayChecks.selectedDays()
                if (frequencyPicker.currentValue === 4) ruleField.text = "FREQ=DAILY;INTERVAL=" + interval + ";BYDAY=MO,TU,WE,TH,FR"
                else {
                    const frequencies = ["DAILY", "WEEKLY", "MONTHLY", "YEARLY"]
                    let rule = "FREQ=" + frequencies[frequencyPicker.currentValue] + ";INTERVAL=" + interval
                    if (frequencyPicker.currentValue === 1 && days.length > 0) rule += ";BYDAY=" + days.join(",")
                    if (frequencyPicker.currentValue === 2 && monthlyOrdinal.currentValue.length > 0 && days.length === 1)
                        rule += ";BYDAY=" + monthlyOrdinal.currentValue + days[0]
                    ruleField.text = rule
                }
            }
        }
    }

    TextField {
        id: ruleField
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        placeholderText: "Advanced rule (e.g. FREQ=WEEKLY;INTERVAL=1;BYDAY=MO,WE)"
        Accessible.name: "Advanced task recurrence rule"
        Accessible.description: "Optional date-only HCB rule. It must match the selected frequency and interval."
        selectByMouse: true
    }

    TextField {
        id: exclusionDatesField
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        placeholderText: "Skip dates (YYYY-MM-DD, comma separated)"
        Accessible.name: "Task recurrence skip dates"
        selectByMouse: true
    }

    TextField {
        id: additionDatesField
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        placeholderText: "Add dates (YYYY-MM-DD, comma separated)"
        Accessible.name: "Task recurrence additional dates"
        selectByMouse: true
    }

    ComboBox {
        id: endPicker
        Layout.fillWidth: true
        visible: recurrenceCheck.checked
        model: [
            { label: "Never ends", value: 0 },
            { label: "Ends on date", value: 1 },
            { label: "Ends after count", value: 2 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Task repeat end condition"
    }

    TextField {
        id: untilField
        Layout.fillWidth: true
        visible: recurrenceCheck.checked && endPicker.currentValue === 1
        placeholderText: "End date (YYYY-MM-DD)"
        Accessible.name: "Task repeat end date"
        selectByMouse: true
    }

    TextField {
        id: countField
        Layout.fillWidth: true
        visible: recurrenceCheck.checked && endPicker.currentValue === 2
        placeholderText: "Number of occurrences"
        inputMethodHints: Qt.ImhDigitsOnly
        Accessible.name: "Task repeat occurrence count"
        selectByMouse: true
    }

    ComboBox {
        id: taskListPicker
        Layout.fillWidth: true
        model: root.activeTaskLists
        textRole: "title"
        valueRole: "id"
        Accessible.name: "Task list"
        onCurrentValueChanged: root.taskListId = currentValue || ""
    }

    Label {
        Layout.fillWidth: true
        visible: taskListPicker.count === 0
        text: "No active task lists are available."
        color: Theme.textSecondary
        horizontalAlignment: Text.AlignHCenter
    }
}
