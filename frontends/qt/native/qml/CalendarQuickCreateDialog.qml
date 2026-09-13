import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: createKind === 0 ? "New event" : "New task"
    primaryText: createKind === 0 ? "Create event" : "Create task"
    primaryEnabled: titleField.text.trim().length > 0 &&
                    (createKind === 1 || calendarId.length > 0)
    property int createKind: 0 // 0 event, 1 task
    property string calendarId: ""
    property string startAt: ""
    property string endAt: ""
    property bool allDay: false
    property string taskDueDate: ""
    property var taskListModel: null
    property var activeTaskLists: taskListModel !== null && typeof taskListModel.selectedTaskLists === "function"
                                  ? taskListModel.selectedTaskLists() : []
    property alias titleField: titleField
    signal eventCreateRequested(string title, string startAt, string endAt, bool allDay)
    signal taskCreateRequested(string taskListId, string title, string dueDate)

    function dateOnly(value) {
        return typeof value === "string" && value.length >= 10 ? value.slice(0, 10) : ""
    }

    function localRangeLabel() {
        if (allDay) return taskDueDate
        const start = new Date(startAt)
        const end = new Date(endAt)
        if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime())) return ""
        return Qt.locale().toString(start, "ddd, d MMM HH:mm") + " – " +
               Qt.locale().toString(end, "HH:mm")
    }

    function openForRange(initialCalendarId, initialStartAt, initialEndAt, initialAllDay) {
        createKind = 0
        calendarId = initialCalendarId || ""
        startAt = initialStartAt || ""
        endAt = initialEndAt || ""
        allDay = initialAllDay === true
        taskDueDate = dateOnly(startAt)
        titleField.clear()
        taskListPicker.currentIndex = taskListPicker.count > 0 ? 0 : -1
        open()
    }

    onOpened: titleField.forceActiveFocus()
    onPrimaryAction: {
        if (createKind === 0) {
            eventCreateRequested(titleField.text.trim(), startAt, endAt, allDay)
        } else {
            taskCreateRequested(taskListPicker.currentValue || "", titleField.text.trim(), taskDueDate)
        }
    }

    TabBar {
        id: kindTabs
        Layout.fillWidth: true
        currentIndex: root.createKind
        onCurrentIndexChanged: root.createKind = currentIndex

        TabButton { text: "Event"; Accessible.name: text }
        TabButton { text: "Task"; Accessible.name: text }
    }

    EmojiTextField {
        id: titleField
        Layout.fillWidth: true
        placeholderText: root.createKind === 0 ? "Event title" : "Task title"
        Accessible.name: placeholderText
        selectByMouse: true
        Keys.onReturnPressed: {
            if (root.primaryEnabled) root.primaryButton.click()
        }
    }

    Label {
        Layout.fillWidth: true
        visible: root.createKind === 0
        text: root.localRangeLabel()
        color: Theme.textSecondary
        Accessible.name: text
    }

    DateEditor {
        id: taskDueField
        Layout.fillWidth: true
        visible: root.createKind === 1
        value: root.taskDueDate
        accessibleName: "Task due date"
        onValueChanged: root.taskDueDate = value
    }

    ComboBox {
        id: taskListPicker
        Layout.fillWidth: true
        visible: root.createKind === 1
        model: root.activeTaskLists
        textRole: "title"
        valueRole: "id"
        Accessible.name: "Task list"
    }
}
