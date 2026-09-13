import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    required property var commandRegistry
    required property string currentPage
    property bool googleConnected: true
    property bool notesEnabled: false
    property int pendingInvitationCount: 0
    property var sidebarTabIds: ["tasks", "calendar"]
    property var calendarSourceModel: null
    property var persistedVisibleCalendarIds: []
    property bool calendarVisibilityConfigured: false
    property bool calendarControlsExpanded: false
    property var calendarVisibility: null
    property alias pageButtons: pageButtons
    signal pageSelected(string pageName)
    signal calendarVisibilitySaveRequested(var visibleCalendarIds)

    SplitView.preferredWidth: Theme.navigationWidth

    function hasNavigationPage(pageName) {
        if (Array.isArray(commandRegistry)) {
            return commandRegistry.some(function(command) {
                return command.commandId.startsWith("navigation.") && command.commandLabel === pageName
            })
        }
        if (typeof commandRegistry.containsLabel === "function") {
            return commandRegistry.containsLabel(pageName)
        }
        for (let row = 0; row < commandRegistry.count; ++row) {
            if (commandRegistry.get(row).commandLabel === pageName) {
                return true
            }
        }
        return false
    }

    function selectPage(pageName) {
        if (hasNavigationPage(pageName) && (googleConnected || pageName === "Settings")) {
            pageSelected(pageName)
        }
    }

    function tabIdForLabel(label) {
        return label.toLowerCase()
    }

    function navigationCommands() {
        const commands = Array.isArray(commandRegistry) ? commandRegistry : []
        const result = []
        sidebarTabIds.forEach(function(tabId) {
            commands.forEach(function(command) {
                if (command.commandId.startsWith("navigation.") &&
                        tabIdForLabel(command.commandLabel) === tabId &&
                        (command.commandLabel !== "Notes" || root.notesEnabled)) {
                    result.push(command)
                }
            })
        })
        commands.forEach(function(command) {
            if (command.commandId === "navigation.settings") result.push(command)
        })
        return result
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingSmall

        Repeater {
            id: pageButtons
            model: root.navigationCommands()

            delegate: ColumnLayout {
                required property string commandId
                required property string commandLabel
                property alias enabled: navigationButton.enabled
                property alias checked: navigationButton.checked
                property alias button: navigationButton
                Layout.fillWidth: true
                spacing: Theme.spacingSmall

                AccessibleNavigationButton {
                    id: navigationButton
                    Layout.fillWidth: true
                    pageName: commandLabel
                    badgeText: commandLabel === "Invitations" && root.pendingInvitationCount > 0
                               ? String(root.pendingInvitationCount) : ""
                    currentPage: root.currentPage === commandLabel
                    enabled: root.googleConnected || commandLabel === "Settings"
                    visible: commandId.startsWith("navigation.")
                    accessibleDescription: commandLabel === "Calendar"
                                           ? "Calendar page. Click again to show or hide calendar visibility controls."
                                           : ""
                    onPageSelected: {
                        if (commandLabel === "Calendar") {
                            root.calendarControlsExpanded = !root.calendarControlsExpanded
                        }
                        root.selectPage(pageName)
                    }
                    onSecondaryActivated: {
                        if (commandLabel === "Calendar") root.calendarControlsExpanded = !root.calendarControlsExpanded
                    }
                }

                CalendarSourceControls {
                    id: calendarVisibility
                    Layout.fillWidth: true
                    Layout.preferredHeight: commandLabel === "Calendar" ? implicitHeight : 0
                    visible: commandLabel === "Calendar"
                    expanded: root.calendarControlsExpanded
                    calendarSourceModel: root.calendarSourceModel
                    persistedVisibleCalendarIds: root.persistedVisibleCalendarIds
                    calendarVisibilityConfigured: root.calendarVisibilityConfigured
                    Component.onCompleted: {
                        if (commandLabel === "Calendar") root.calendarVisibility = calendarVisibility
                    }
                    onVisibleCalendarIdsChanged: {
                        if (commandLabel === "Calendar" && visibilityInitialized)
                            root.calendarVisibilitySaveRequested(visibleCalendarIds)
                    }
                }
            }
        }

        Item { Layout.fillHeight: true }
    }
}
