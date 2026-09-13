import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: "Edit task"
    primaryText: "Save task"
    primaryEnabled: taskId.length > 0 && titleField.text.trim().length > 0 && recurrenceInputValid()
    property string taskId: ""
    property string taskDueTimeZone: ""
    property var timeZones: ["", "UTC", "America/Los_Angeles", "America/New_York", "Asia/Singapore", "Europe/London"]
    property int taskPriority: 0
    property bool taskManagedRecurrence: false
    property string taskRecurrenceSummary: ""
    property alias taskTitle: titleField.text
    property alias taskNotes: notesField.text
    property alias taskDueAt: dueField.value
    property alias taskPriorityPicker: priorityPicker
    property alias taskTitleField: titleField
    signal taskUpdateRequested(string taskId, string title, string notes, string dueAt,
                               string dueTimeZone, int priority, bool managedRecurrence,
                               int recurrenceFrequency, int recurrenceInterval, int recurrenceEndKind,
                               string recurrenceEndUntil, int recurrenceEndCount, string recurrenceRule,
                               string exclusionDates, string additionDates)
    signal taskRecurrenceActionRequested(string taskId, string title, int action)

    function recurrenceInputValid() {
        if (!taskManagedRecurrence) return true
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

    function openForEdit(taskId, title, notes, dueAt, dueTimeZone, priority, managedRecurrence,
                         recurrenceSummary, recurrenceFrequency, recurrenceInterval,
                         recurrenceEndKind, recurrenceEndUntil, recurrenceEndCount, recurrenceRule,
                         exclusionDates, additionDates) {
        root.taskId = taskId
        titleField.text = title
        notesField.text = notes
        dueField.value = dueAt
        root.taskDueTimeZone = dueTimeZone
        priorityPicker.currentIndex = priorityPicker.indexOfValue(priority)
        root.taskPriority = priorityPicker.currentValue
        root.taskManagedRecurrence = managedRecurrence === true
        root.taskRecurrenceSummary = recurrenceSummary || ""
        frequencyPicker.currentIndex = Math.max(0, frequencyPicker.indexOfValue(
                                                    typeof recurrenceFrequency === "number"
                                                    ? recurrenceFrequency : 0))
        intervalField.text = String(recurrenceInterval > 0 ? recurrenceInterval : 1)
        endPicker.currentIndex = Math.max(0, endPicker.indexOfValue(
                                              typeof recurrenceEndKind === "number"
                                              ? recurrenceEndKind : 0))
        untilField.text = recurrenceEndUntil || ""
        countField.text = String(recurrenceEndCount > 0 ? recurrenceEndCount : 1)
        ruleField.text = recurrenceRule || ""
        exclusionDatesField.text = exclusionDates || ""
        additionDatesField.text = additionDates || ""
        open()
    }

    onOpened: titleField.forceActiveFocus()
    onPrimaryAction: taskUpdateRequested(taskId, titleField.text.trim(), notesField.text,
                                         dueField.value.trim(), dueField.value.trim().length > 0
                                         ? taskDueTimeZone : "", taskPriority, taskManagedRecurrence,
                                         frequencyPicker.currentValue, Number(intervalField.text),
                                         endPicker.currentValue, untilField.text.trim(), Number(countField.text),
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
        Layout.preferredHeight: 120
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

    TimeZonePicker {
        id: timeZonePicker
        timeZones: root.timeZones
        timeZone: root.taskDueTimeZone
        onTimeZoneChanged: root.taskDueTimeZone = timeZone
    }

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
        onCurrentValueChanged: root.taskPriority = currentValue
    }

    Label {
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        text: taskRecurrenceSummary
        color: Theme.textSecondary
        wrapMode: Text.WordWrap
        Accessible.name: "Task recurrence: " + text
    }

    ComboBox {
        id: frequencyPicker
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        model: [
            { label: "Daily", value: 0 },
            { label: "Weekly", value: 1 },
            { label: "Monthly", value: 2 },
            { label: "Yearly", value: 3 }
        ]
        textRole: "label"
        valueRole: "value"
        Accessible.name: "Task repeat frequency"
    }

    TextField {
        id: intervalField
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        placeholderText: "Repeat interval"
        inputMethodHints: Qt.ImhDigitsOnly
        Accessible.name: "Task repeat interval"
        selectByMouse: true
    }

    RowLayout {
        id: weekdayChecks
        Layout.fillWidth: true
        visible: taskManagedRecurrence
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
        visible: taskManagedRecurrence
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
                const frequencies = ["DAILY", "WEEKLY", "MONTHLY", "YEARLY"]
                let rule = "FREQ=" + frequencies[frequencyPicker.currentValue] + ";INTERVAL=" + interval
                const days = weekdayChecks.selectedDays()
                if (frequencyPicker.currentValue === 1 && days.length > 0) rule += ";BYDAY=" + days.join(",")
                if (frequencyPicker.currentValue === 2 && monthlyOrdinal.currentValue.length > 0 && days.length === 1)
                    rule += ";BYDAY=" + monthlyOrdinal.currentValue + days[0]
                ruleField.text = rule
            }
        }
    }

    TextField {
        id: ruleField
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        placeholderText: "Advanced rule (e.g. FREQ=WEEKLY;BYDAY=MO,WE)"
        Accessible.name: "Advanced task recurrence rule"
        selectByMouse: true
    }

    TextField {
        id: exclusionDatesField
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        placeholderText: "Skip dates (YYYY-MM-DD, comma separated)"
        Accessible.name: "Task recurrence skip dates"
        selectByMouse: true
    }

    TextField {
        id: additionDatesField
        Layout.fillWidth: true
        visible: taskManagedRecurrence
        placeholderText: "Add dates (YYYY-MM-DD, comma separated)"
        Accessible.name: "Task recurrence additional dates"
        selectByMouse: true
    }

    ComboBox {
        id: endPicker
        Layout.fillWidth: true
        visible: taskManagedRecurrence
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
        visible: taskManagedRecurrence && endPicker.currentValue === 1
        placeholderText: "End date (YYYY-MM-DD)"
        Accessible.name: "Task repeat end date"
        selectByMouse: true
    }

    TextField {
        id: countField
        Layout.fillWidth: true
        visible: taskManagedRecurrence && endPicker.currentValue === 2
        placeholderText: "Number of occurrences"
        inputMethodHints: Qt.ImhDigitsOnly
        Accessible.name: "Task repeat occurrence count"
        selectByMouse: true
    }

    RowLayout {
        Layout.fillWidth: true
        visible: taskManagedRecurrence

        Button {
            text: "Stop repeating"
            Accessible.name: text
            onClicked: {
                root.close()
                root.taskRecurrenceActionRequested(root.taskId, titleField.text, 0)
            }
        }

        Button {
            text: "New series from here"
            Accessible.name: text
            onClicked: {
                root.close()
                root.taskRecurrenceActionRequested(root.taskId, titleField.text, 1)
            }
        }
    }
}
