import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Popup {
    id: root
    parent: Overlay.overlay
    anchors.centerIn: parent
    width: Math.min(720, parent.width - Theme.spacingLarge * 2)
    height: Math.min(640, parent.height - Theme.spacingLarge * 2)
    modal: true
    focus: true
    padding: Theme.spacingLarge
    closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutsideParent
    property var appController: null
    property var searchResultsModel: null
    property string editingSavedSearchId: ""
    property string editingSavedSearchName: ""
    property bool optionsExpanded: false
    property alias queryField: queryField
    property alias resultRows: resultRows
    property alias savedSearchNameField: savedSearchNameField
    property alias saveSearchButton: saveSearchButton
    property alias savedSearchRows: savedSearchRows
    property alias optionsToggleButton: optionsToggleButton
    signal resultActivated(string resource, string resultId, string title, string detail, string scheduledAt)

    function controllerCall(method, args) {
        if (appController !== null && typeof appController[method] === "function") {
            appController[method].apply(appController, args)
        }
    }

    function savedSearches() {
        return appController !== null && appController.savedSearches !== undefined
               ? appController.savedSearches : []
    }

    onOpened: {
        optionsExpanded = false
        queryField.text = appController !== null && typeof appController.searchQuery === "string"
                          ? appController.searchQuery : ""
        queryField.forceActiveFocus()
    }

    onClosed: {
        editingSavedSearchId = ""
        editingSavedSearchName = ""
    }

    background: Rectangle {
        color: Theme.surface
        border.color: Theme.accent
        border.width: 1
        radius: Theme.spacingSmall
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: Theme.spacingMedium
        Accessible.role: Accessible.Dialog
        Accessible.name: "Search"

        RowLayout {
            Layout.fillWidth: true

            TextField {
                id: queryField
                objectName: "searchQuery"
                Layout.fillWidth: true
                placeholderText: "Search tasks, events, notes, lists, and calendars"
                Accessible.name: placeholderText
                onTextEdited: root.controllerCall("setSearchQuery", [text])
                Keys.onPressed: function(event) {
                    if ((event.key === Qt.Key_Return || event.key === Qt.Key_Enter) && resultRows.count > 0) {
                        resultRows.currentIndex = Math.max(0, resultRows.currentIndex)
                        resultRows.currentItem.click()
                        event.accepted = true
                    }
                }
            }

            ToolButton {
                id: optionsToggleButton
                text: root.optionsExpanded ? "Options ▴" : "Options ▾"
                checkable: true
                checked: root.optionsExpanded
                Accessible.name: root.optionsExpanded ? "Hide search options" : "Show search options"
                onClicked: root.optionsExpanded = !root.optionsExpanded
            }
        }

        Label {
            Layout.fillWidth: true
            visible: root.appController !== null && root.appController.searchErrorMessage !== undefined &&
                     root.appController.searchErrorMessage.length > 0
            text: root.appController !== null && typeof root.appController.searchErrorMessage === "string"
                  ? root.appController.searchErrorMessage : ""
            color: Theme.destructive
            wrapMode: Text.WordWrap
            Accessible.name: text
        }

        ListView {
            id: resultRows
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            spacing: Theme.spacingSmall
            model: root.searchResultsModel
            currentIndex: count > 0 ? 0 : -1

            delegate: AccessibleButton {
                required property string id
                required property string resource
                required property string title
                required property string detail
                required property string scheduledAt
                width: ListView.view.width
                text: title + "\n" + resource + (detail.length > 0 ? " — " + detail : "")
                accessibleName: title
                accessibleDescription: resource + (detail.length > 0 ? ". " + detail : "")
                highlighted: ListView.isCurrentItem
                onClicked: root.resultActivated(resource, id, title, detail, scheduledAt)
            }
        }

        Label {
            Layout.fillWidth: true
            visible: queryField.text.trim().length > 0 && resultRows.count === 0 &&
                     !(root.appController !== null && root.appController.searchLoading === true)
            text: "No cached matches"
            color: Theme.textSecondary
            horizontalAlignment: Text.AlignHCenter
        }

        Label {
            visible: root.appController !== null && root.appController.searchLoading === true
            text: "Searching locally…"
            color: Theme.textSecondary
            Accessible.name: text
        }

        ColumnLayout {
            Layout.fillWidth: true
            visible: root.optionsExpanded
            spacing: Theme.spacingMedium

            Flow {
                Layout.fillWidth: true
                spacing: Theme.spacingSmall
                visible: root.appController !== null && root.appController.searchFilterChips !== undefined &&
                         root.appController.searchFilterChips.length > 0

                Repeater {
                    model: root.appController !== null && root.appController.searchFilterChips !== undefined
                           ? root.appController.searchFilterChips : []
                    delegate: Rectangle {
                        required property var modelData
                        implicitWidth: chipLabel.implicitWidth + Theme.spacingMedium
                        implicitHeight: chipLabel.implicitHeight + Theme.spacingSmall
                        color: Theme.background
                        border.color: Theme.accent
                        radius: Theme.spacingSmall

                        Label {
                            id: chipLabel
                            anchors.centerIn: parent
                            text: modelData
                        }
                    }
                }
            }

            Label {
                Layout.fillWidth: true
                text: "Filters: source:, status:, due:, start:, priority:, list:, calendar:, body:"
                color: Theme.textSecondary
                wrapMode: Text.WordWrap
            }

            RowLayout {
                Layout.fillWidth: true

                TextField {
                    id: savedSearchNameField
                    Layout.fillWidth: true
                    placeholderText: "Save current search as"
                    Accessible.name: placeholderText
                    onAccepted: root.controllerCall("saveSearch", [text, queryField.text])
                }

                Button {
                    id: saveSearchButton
                    text: "Save"
                    enabled: savedSearchNameField.text.trim().length > 0 && queryField.text.trim().length > 0 &&
                             !(root.appController !== null && root.appController.busy === true)
                    Accessible.name: text
                    onClicked: root.controllerCall("saveSearch", [savedSearchNameField.text, queryField.text])
                }
            }

            Repeater {
                id: savedSearchRows
                model: root.savedSearches()

                delegate: RowLayout {
                    required property var modelData
                    Layout.fillWidth: true

                    TextField {
                        Layout.fillWidth: true
                        text: root.editingSavedSearchId === modelData.id
                              ? root.editingSavedSearchName : modelData.name
                        readOnly: root.editingSavedSearchId !== modelData.id
                        Accessible.name: "Saved search " + modelData.name
                        onTextEdited: root.editingSavedSearchName = text
                    }

                    Button {
                        text: "Apply"
                        enabled: !(root.appController !== null && root.appController.busy === true)
                        Accessible.name: text + " " + modelData.name
                        onClicked: {
                            queryField.text = modelData.query
                            root.controllerCall("applySavedSearch", [modelData.id])
                        }
                    }

                    Button {
                        text: root.editingSavedSearchId === modelData.id ? "Done" : "Rename"
                        enabled: !(root.appController !== null && root.appController.busy === true)
                        Accessible.name: text + " " + modelData.name
                        onClicked: {
                            if (root.editingSavedSearchId === modelData.id) {
                                root.controllerCall("renameSavedSearch", [modelData.id, root.editingSavedSearchName])
                                root.editingSavedSearchId = ""
                                root.editingSavedSearchName = ""
                            } else {
                                root.editingSavedSearchId = modelData.id
                                root.editingSavedSearchName = modelData.name
                            }
                        }
                    }

                    Button {
                        text: "Delete"
                        enabled: !(root.appController !== null && root.appController.busy === true)
                        Accessible.name: text + " " + modelData.name
                        onClicked: root.controllerCall("deleteSavedSearch", [modelData.id])
                    }
                }
            }
        }
    }
}
