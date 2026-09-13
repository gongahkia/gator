import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    property var taskListModel: null
    property var taskModel: null
    property bool taskListLoading: false
    property string taskListErrorMessage: ""
    property string bulkTaskStatusMessage: ""
    property string bulkTaskPreviewMessage: ""
    property int bulkTaskPreviewRequestToken: -1
    property int bulkTextRecurrenceScope: 2
    property int listPaneWidth: 260
    property bool listPaneWidthInitialized: false
    property var selectedTaskIds: []
    property bool selectionMode: false
    property string highlightedTaskId: ""
    readonly property string preferredTaskListId: root.selectedTaskListId()
    property alias taskRows: taskRows
    property alias taskCreateButton: taskCreateButton
    property alias importButton: importButton
    property alias taskListControls: taskListControls
    property alias bulkSelectAllButton: bulkSelectAllButton
    property alias bulkCompleteButton: bulkCompleteButton
    property alias bulkDeleteButton: bulkDeleteButton
    property alias bulkSelectionStatus: bulkSelectionStatus
    property alias bulkDeleteDialog: bulkDeleteDialog
    property alias bulkMoveDialog: bulkMoveDialog
    property alias bulkEditDialog: bulkEditDialog
    property alias taskListSplitView: taskListSplitView
    signal taskSelected(string taskId)
    signal taskCreateRequested()
    signal importRequested()
    signal taskSubtaskCreateRequested(string parentTaskId, string taskListId)
    signal taskEditRequested(string taskId, string title, string notes, string dueAt,
                             string dueTimeZone, int priority, bool managedRecurrence,
                             string recurrenceSummary, int recurrenceFrequency, int recurrenceInterval,
                             int recurrenceEndKind, string recurrenceEndUntil, int recurrenceEndCount,
                             string recurrenceRule, string recurrenceExclusionDates,
                             string recurrenceAdditionDates)
    signal taskDetailRequested(var task)
    signal taskCompletionRequested(string taskId, bool completed)
    signal taskDeleteRequested(string taskId, string taskTitle, bool managedRecurrence)
    signal taskMoveRequested(string taskId, string taskListId, string taskTitle)
    signal taskReparentRequested(string taskId, string parentTaskId)
    signal taskReorderRequested(string taskId, bool earlier)
    signal taskListCreateRequested()
    signal taskListRenameRequested(string taskListId, string title)
    signal taskListDeleteRequested(string taskListId, string title, int taskCount, var taskTitles)
    signal taskListSelectionRequested(string taskListId, bool selected)
    signal bulkTaskCompletionRequested(var taskIds, bool completed)
    signal bulkTaskDeleteRequested(var taskIds)
    signal bulkTaskMoveRequested(var taskIds, string taskListId)
    signal bulkTaskDueRequested(var taskIds, string dueAt)
    signal bulkTaskClearDueRequested(var taskIds)
    signal bulkTaskPriorityRequested(var taskIds, int priority)
    signal bulkTaskReparentRequested(var taskIds, string parentTaskId)
    signal bulkTaskTextPreviewRequested(var taskIds, string findText, int fields, int recurrenceScope,
                                        int requestToken)
    signal bulkTaskTextReplaceRequested(var taskIds, string findText, string replaceText, int fields,
                                        int recurrenceScope)
    signal listPaneWidthPersistenceRequested(int width)

    Timer {
        id: listPaneWidthSaveTimer
        interval: 180
        repeat: false
        onTriggered: root.listPaneWidthPersistenceRequested(root.listPaneWidth)
    }

    onListPaneWidthChanged: {
        if (listPaneWidthInitialized) listPaneWidthSaveTimer.restart()
    }

    Component.onCompleted: listPaneWidthInitialized = true

    function selectTask(taskId) {
        revealTask(taskId)
    }

    function revealTask(taskId) {
        highlightedTaskId = taskId
        highlightTimer.restart()
        taskSelected(taskId)
        if (taskModel === null || typeof taskModel.taskForId !== "function") return
        const task = taskModel.taskForId(taskId)
        if (task.id === undefined || task.id.length === 0) return
        const row = typeof taskModel.flatRowForTaskId === "function"
                    ? taskModel.flatRowForTaskId(taskId) : -1
        if (row >= 0 && typeof taskRows.positionViewAtRow === "function") {
            taskRows.positionViewAtRow(row, TableView.Center)
        }
        Qt.callLater(function() { root.requestTaskDetail(task) })
    }

    Timer {
        id: highlightTimer
        interval: 5000
        repeat: false
        onTriggered: root.highlightedTaskId = ""
    }

    function resizeListPane(width) {
        listPaneWidth = Math.max(200, Math.min(480, Math.round(width)))
    }

    function requestTaskCreate() {
        taskCreateRequested()
    }

    function requestSubtaskCreate(parentTaskId, taskListId) {
        taskSubtaskCreateRequested(parentTaskId, taskListId)
    }

    function requestTaskEdit(taskId, title, notes, dueAt, dueTimeZone, priority, managedRecurrence,
                             recurrenceSummary, recurrenceFrequency, recurrenceInterval,
                             recurrenceEndKind, recurrenceEndUntil, recurrenceEndCount,
                             recurrenceRule, recurrenceExclusionDates, recurrenceAdditionDates) {
        taskEditRequested(taskId, title, notes, dueAt, dueTimeZone, priority, managedRecurrence,
                          recurrenceSummary, recurrenceFrequency, recurrenceInterval,
                          recurrenceEndKind, recurrenceEndUntil, recurrenceEndCount,
                          recurrenceRule, recurrenceExclusionDates, recurrenceAdditionDates)
    }

    function requestTaskDetail(task) {
        taskDetailRequested(task)
    }

    function requestTaskCompletion(taskId, completed) {
        taskCompletionRequested(taskId, completed)
    }

    function requestTaskDelete(taskId, taskTitle, managedRecurrence) {
        taskDeleteRequested(taskId, taskTitle, managedRecurrence)
    }

    function requestTaskMove(taskId, taskListId, taskTitle) {
        taskMoveRequested(taskId, taskListId, taskTitle)
    }

    function requestTaskReparent(taskId, parentTaskId) {
        taskReparentRequested(taskId, parentTaskId)
    }

    function requestTaskReorder(taskId, earlier) {
        taskReorderRequested(taskId, earlier)
    }

    function isTaskSelected(taskId) {
        return selectedTaskIds.indexOf(taskId) >= 0
    }

    function setTaskSelected(taskId, selected) {
        const next = selectedTaskIds.slice()
        const index = next.indexOf(taskId)
        if (selected && index < 0) {
            next.push(taskId)
        } else if (!selected && index >= 0) {
            next.splice(index, 1)
        }
        selectedTaskIds = next
    }

    function selectAllTasks() {
        if (taskModel !== null && typeof taskModel.taskIds === "function") {
            selectedTaskIds = taskModel.taskIds()
        }
    }

    function clearTaskSelection() {
        selectedTaskIds = []
    }

    function taskHeading() {
        if (taskListModel !== null && typeof taskListModel.selectedTaskLists === "function") {
            const selected = taskListModel.selectedTaskLists()
            if (selected.length === 1) return selected[0].title
        }
        return "All Tasks"
    }

    function selectedTaskListId() {
        if (taskListModel !== null && typeof taskListModel.selectedTaskLists === "function") {
            const selected = taskListModel.selectedTaskLists()
            if (selected.length === 1) return selected[0].id
        }
        return ""
    }

    function formatDueDate(value) {
        if (value.length === 0) return ""
        const parsed = new Date(value.length === 10 ? value + "T12:00:00" : value)
        return Number.isFinite(parsed.getTime()) ? Qt.locale().toString(parsed, "d MMM") : value
    }

    Shortcut {
        sequence: "Ctrl+A"
        autoRepeat: false
        enabled: root.visible
        onActivated: {
            root.selectionMode = true
            root.selectAllTasks()
        }
    }

    Connections {
        target: root.taskModel
        function onModelReset() {
            root.clearTaskSelection()
        }
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingMedium

        RowLayout {
            id: taskToolbar
            Layout.fillWidth: true

            Label {
                text: root.taskHeading()
                font.pixelSize: Theme.titleFontSize
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }

            Item { Layout.fillWidth: true }

            Button {
                id: taskCreateButton
                text: "New task"
                Accessible.name: text
                Accessible.description: "Create a task in an active task list"
                onClicked: root.requestTaskCreate()
            }

            Button {
                id: importButton
                text: "Import"
                Accessible.name: "Import Tasks and events"
                enabled: !root.taskListLoading
                onClicked: root.importRequested()
            }

            Button {
                id: bulkSelectAllButton
                text: root.selectionMode ? "Done" : "Select"
                Accessible.name: root.selectionMode ? "Exit task selection" : "Select tasks"
                enabled: !root.taskListLoading && root.taskModel !== null
                onClicked: {
                    if (root.selectionMode) {
                        root.clearTaskSelection()
                        root.selectionMode = false
                    } else {
                        root.selectionMode = true
                    }
                }
            }
        }

        RowLayout {
            id: taskListSplitView
            Layout.fillWidth: true
            Layout.fillHeight: true
            spacing: 0

            TaskListControls {
                id: taskListControls
                Layout.minimumWidth: 200
                Layout.maximumWidth: 480
                Layout.preferredWidth: root.listPaneWidth
                Layout.fillHeight: true
                taskListModel: root.taskListModel
                loading: root.taskListLoading
                errorMessage: root.taskListErrorMessage

                onTaskListCreateRequested: root.taskListCreateRequested()
                onTaskListRenameRequested: function(taskListId, title) {
                    root.taskListRenameRequested(taskListId, title)
                }
                onTaskListDeleteRequested: function(taskListId, title, taskCount, taskTitles) {
                    root.taskListDeleteRequested(taskListId, title, taskCount, taskTitles)
                }
                onTaskListSelectionRequested: function(taskListId, selected) {
                    root.taskListSelectionRequested(taskListId, selected)
                }
            }

            Item {
                id: listPaneResizeHandle
                Layout.preferredWidth: Theme.spacingMedium
                Layout.minimumWidth: Theme.spacingMedium
                Layout.maximumWidth: Theme.spacingMedium
                Layout.fillHeight: true
                Accessible.role: Accessible.Separator
                Accessible.name: "Resize task lists pane"

                Rectangle {
                    anchors.verticalCenter: parent.verticalCenter
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: 2
                    height: Math.min(parent.height, Theme.spacingLarge * 2)
                    radius: 1
                    color: listPaneResizeHover.hovered || listPaneResizeDrag.active
                           ? Theme.accent : Theme.textSecondary
                    opacity: listPaneResizeHover.hovered || listPaneResizeDrag.active ? 1 : 0.45
                }

                HoverHandler {
                    id: listPaneResizeHover
                    cursorShape: Qt.SplitHCursor
                }

                DragHandler {
                    id: listPaneResizeDrag
                    target: null
                    xAxis.enabled: true
                    yAxis.enabled: false
                    property int pressWidth: 0

                    onActiveChanged: if (active) pressWidth = root.listPaneWidth
                    onActiveTranslationChanged: root.resizeListPane(pressWidth + activeTranslation.x)
                }
            }

            ColumnLayout {
                Layout.fillWidth: true
                Layout.fillHeight: true
                spacing: Theme.spacingSmall

                Flow {
                    Layout.fillWidth: true
                    spacing: Theme.spacingSmall
                    visible: root.selectionMode

                    Label {
                        id: bulkSelectionStatus
                        text: selectedTaskIds.length + " selected · eligibility checked before queueing"
                        color: Theme.textSecondary
                        Accessible.name: text
                    }

                    Button { text: "Select all"; enabled: !root.taskListLoading; onClicked: root.selectAllTasks() }
                    Button { text: "Clear"; enabled: !root.taskListLoading; onClicked: root.clearTaskSelection() }
                    Button {
                        id: bulkCompleteButton
                        text: "Complete"
                        Accessible.name: text + " " + root.selectedTaskIds.length + " tasks"
                        enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                        onClicked: root.bulkTaskCompletionRequested(selectedTaskIds, true)
                    }
                    Button { text: "Reopen"; enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                             onClicked: root.bulkTaskCompletionRequested(selectedTaskIds, false) }
                    Button {
                        id: bulkDeleteButton
                        text: "Delete"
                        Accessible.name: text + " " + root.selectedTaskIds.length + " tasks"
                        enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                        onClicked: bulkDeleteDialog.openForDelete(selectedTaskIds)
                    }
                    Button { text: "Move"; enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                             onClicked: bulkMoveDialog.openForMove(selectedTaskIds) }
                    Button { text: "Set due"; enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                             onClicked: bulkEditDialog.openForDue(selectedTaskIds) }
                    Button { text: "Set priority"; enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                             onClicked: bulkEditDialog.openForPriority(selectedTaskIds) }
                    Button { text: "More"; enabled: !root.taskListLoading && root.selectedTaskIds.length > 0
                             onClicked: bulkMenu.open()
                        Menu {
                            id: bulkMenu
                            MenuItem { text: "Clear due"; onTriggered: root.bulkTaskClearDueRequested(root.selectedTaskIds) }
                            MenuItem { text: "Reparent"; onTriggered: bulkEditDialog.openForReparent(root.selectedTaskIds) }
                            MenuItem { text: "Find and replace"; onTriggered: bulkTextReplaceDialog.openFor(root.selectedTaskIds, root.bulkTextRecurrenceScope) }
                        }
                    }
                }

                Label {
                    Layout.fillWidth: true
                    visible: bulkTaskStatusMessage.length > 0
                    text: bulkTaskStatusMessage
                    color: Theme.textSecondary
                    wrapMode: Text.WordWrap
                    Accessible.name: text
                }

                TreeView {
            id: taskRows
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: root.taskModel
            reuseItems: true
            columnWidthProvider: function() { return width }

                    delegate: Item {
                        implicitWidth: taskRows.width
                        implicitHeight: taskRow.implicitHeight + Theme.spacingSmall
                        required property TreeView treeView
                        required property bool expanded
                        required property bool hasChildren
                        required property bool isTreeNode
                        required property int column
                        required property int depth
                        required property int row
                        required property string id
                        required property string taskListId
                        required property string title
                        required property string notes
                        required property string dueAt
                        required property string dueTimeZone
                        required property int priority
                        required property bool completed
                        required property bool managedRecurrence
                        required property string recurrenceSummary
                        required property int recurrenceFrequency
                        required property int recurrenceInterval
                        required property int recurrenceEndKind
                        required property string recurrenceEndUntil
                        required property int recurrenceEndCount
                        property string recurrenceRule: ""
                        property string recurrenceExclusionDates: ""
                        property string recurrenceAdditionDates: ""
                        property string recurrenceDiagnostic: ""

                        property alias completionButton: completionButton
                        property alias expandButton: expandButton
                        property alias deleteButton: deleteButton
                        property alias editButton: editButton
                        property alias moveButton: moveButton
                        property alias promoteButton: promoteButton
                        property alias subtaskButton: subtaskButton
                        property alias moveEarlierButton: moveEarlierButton
                        property alias moveLaterButton: moveLaterButton

                        required property string taskListTitle

                        HoverHandler { id: taskHover }

                        Rectangle {
                            anchors.fill: parent
                            color: "transparent"
                            border.color: Theme.accent
                            border.width: 2
                            radius: Theme.spacingSmall
                            visible: root.highlightedTaskId === id
                        }

                        ColumnLayout {
                            id: taskRow
                            anchors.left: parent.left
                            anchors.right: parent.right
                            anchors.top: parent.top
                            anchors.leftMargin: depth * Theme.spacingLarge
                            spacing: 2

                            RowLayout {
                                Layout.fillWidth: true
                                spacing: Theme.spacingSmall

                                CheckBox {
                                    id: taskSelectionCheck
                                    visible: root.selectionMode
                                    checked: root.isTaskSelected(id)
                                    Accessible.name: "Select " + title
                                    onToggled: root.setTaskSelected(id, checked)
                                }

                                ToolButton {
                                    id: completionButton
                                    text: completed ? "✓" : "○"
                                    Accessible.name: (completed ? "Reopen " : "Complete ") + title
                                    Accessible.description: completed ? "Mark task active" : "Mark task completed"
                                    onClicked: root.requestTaskCompletion(id, !completed)
                                }

                                ToolButton {
                                    id: expandButton
                                    visible: isTreeNode && hasChildren
                                    text: expanded ? "⌄" : "›"
                                    Accessible.name: (expanded ? "Collapse" : "Expand") + " subtasks for " + title
                                    onClicked: treeView.toggleExpanded(row)
                                }

                                AccessibleButton {
                                    Layout.fillWidth: true
                                    text: title
                                    accessibleName: title
                                    accessibleDescription: completed ? "Completed task" : "View task"
                                    font.strikeout: completed
                                    opacity: completed ? 0.55 : 1
                                    background: Item {}
                                    contentItem: Label {
                                        text: parent.text
                                        elide: Text.ElideRight
                                        font: parent.font
                                        color: Theme.textPrimary
                                        verticalAlignment: Text.AlignVCenter
                                    }
                                    onClicked: root.requestTaskDetail({
                                        id: id, taskListId: taskListId, taskListTitle: taskListTitle,
                                        title: title, notes: notes, dueAt: dueAt, dueTimeZone: dueTimeZone,
                                        priority: priority, completed: completed,
                                        managedRecurrence: managedRecurrence,
                                        recurrenceSummary: recurrenceSummary,
                                        recurrenceFrequency: recurrenceFrequency,
                                        recurrenceInterval: recurrenceInterval,
                                        recurrenceEndKind: recurrenceEndKind,
                                        recurrenceEndUntil: recurrenceEndUntil,
                                        recurrenceEndCount: recurrenceEndCount,
                                        recurrenceRule: recurrenceRule,
                                        recurrenceExclusionDates: recurrenceExclusionDates,
                                        recurrenceAdditionDates: recurrenceAdditionDates
                                    })
                                }

                                Label {
                                    visible: dueAt.length > 0
                                    text: "Due " + root.formatDueDate(dueAt)
                                    color: Theme.textSecondary
                                    elide: Text.ElideRight
                                }

                                ToolButton {
                                    visible: taskHover.hovered || activeFocus
                                    text: "⋯"
                                    Accessible.name: "Task actions for " + title
                                    onClicked: taskMenu.open()
                                }
                            }

                            Label {
                                Layout.leftMargin: root.selectionMode ? 44 : 28
                                visible: managedRecurrence
                                text: recurrenceSummary
                                color: Theme.textSecondary
                                Accessible.name: "Task recurrence: " + recurrenceSummary
                            }
                            Label {
                                Layout.leftMargin: root.selectionMode ? 44 : 28
                                visible: recurrenceDiagnostic.length > 0
                                text: recurrenceDiagnostic
                                color: Theme.destructive
                                wrapMode: Text.WordWrap
                                Accessible.name: "Task recurrence warning: " + recurrenceDiagnostic
                            }
                        }

                        Button {
                            id: editButton
                            visible: false
                            text: "Edit"
                            onClicked: root.requestTaskEdit(id, title, notes, dueAt, dueTimeZone, priority,
                                                            managedRecurrence, recurrenceSummary,
                                                            recurrenceFrequency, recurrenceInterval,
                                                            recurrenceEndKind, recurrenceEndUntil,
                                                            recurrenceEndCount, recurrenceRule,
                                                            recurrenceExclusionDates, recurrenceAdditionDates)
                        }
                        Button { id: deleteButton; visible: false; text: "Delete"; onClicked: root.requestTaskDelete(id, title, managedRecurrence) }
                        Button { id: moveButton; visible: false; text: "Move"; onClicked: root.requestTaskMove(id, taskListId, title) }
                        Button { id: subtaskButton; visible: false; text: "Add subtask"; onClicked: root.requestSubtaskCreate(id, taskListId) }
                        Button { id: promoteButton; visible: false; text: "Promote"; onClicked: root.requestTaskReparent(id, "") }
                        Button { id: moveEarlierButton; visible: false; text: "Move earlier"; onClicked: root.requestTaskReorder(id, true) }
                        Button { id: moveLaterButton; visible: false; text: "Move later"; onClicked: root.requestTaskReorder(id, false) }

                        Menu {
                            id: taskMenu
                            MenuItem { text: "Edit"; onTriggered: editButton.click() }
                            MenuItem { text: "Move"; onTriggered: moveButton.click() }
                            MenuItem { text: depth === 0 ? "Add subtask" : "Promote"; onTriggered: depth === 0 ? subtaskButton.click() : promoteButton.click() }
                            MenuSeparator {}
                            MenuItem { text: "Move earlier"; onTriggered: moveEarlierButton.click() }
                            MenuItem { text: "Move later"; onTriggered: moveLaterButton.click() }
                            MenuSeparator {}
                            MenuItem { text: "Delete"; onTriggered: deleteButton.click() }
                        }
                    }
                }

                Label {
                    Layout.fillWidth: true
                    visible: taskRows.rows === 0
                    text: "No tasks in this view."
                    color: Theme.textSecondary
                    horizontalAlignment: Text.AlignHCenter
                }
            }
        }
    }

    TaskBulkDeleteDialog {
        id: bulkDeleteDialog
        parent: Overlay.overlay
        onBulkDeleteRequested: function(taskIds) {
            root.bulkTaskDeleteRequested(taskIds)
        }
    }

    TaskBulkMoveDialog {
        id: bulkMoveDialog
        parent: Overlay.overlay
        taskListModel: root.taskListModel
        onBulkMoveRequested: function(taskIds, taskListId) {
            root.bulkTaskMoveRequested(taskIds, taskListId)
        }
    }

    TaskBulkEditDialog {
        id: bulkEditDialog
        parent: Overlay.overlay
        taskModel: root.taskModel
        onBulkDueRequested: function(taskIds, dueAt) {
            root.bulkTaskDueRequested(taskIds, dueAt)
        }
        onBulkPriorityRequested: function(taskIds, priority) {
            root.bulkTaskPriorityRequested(taskIds, priority)
        }
        onBulkReparentRequested: function(taskIds, parentTaskId) {
            root.bulkTaskReparentRequested(taskIds, parentTaskId)
        }
    }

    BulkTextReplaceDialog {
        id: bulkTextReplaceDialog
        parent: Overlay.overlay
        kind: "task"
        previewMessage: root.bulkTaskPreviewMessage
        previewResultRequestToken: root.bulkTaskPreviewRequestToken
        onPreviewRequested: function(taskIds, findText, fields, recurrenceScope, requestToken) {
            root.bulkTaskTextPreviewRequested(taskIds, findText, fields, recurrenceScope, requestToken)
        }
        onReplaceRequested: function(taskIds, findText, replaceText, fields, recurrenceScope) {
            root.bulkTaskTextReplaceRequested(taskIds, findText, replaceText, fields, recurrenceScope)
        }
    }
}
