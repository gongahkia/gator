import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

HcbDialog {
    id: root
    title: kind === "task" ? "Find and replace Tasks" : "Find and replace events"
    primaryText: "Apply replacement"
    primaryEnabled: previewed && findField.text.length > 0 && selectedFields() > 0
    property string kind: "task"
    property var recordIds: []
    property int defaultRecurrenceScope: 2
    property string previewMessage: "Enter find text, select fields, then preview."
    property int previewRequestToken: 0
    property int previewResultRequestToken: -1
    property bool previewed: false
    property alias findTextField: findField
    property alias replacementTextField: replacementField
    signal previewRequested(var recordIds, string findText, int fields, int recurrenceScope,
                            int requestToken)
    signal replaceRequested(var recordIds, string findText, string replaceText, int fields,
                           int recurrenceScope)

    function selectedFields() {
        let fields = titleField.checked ? 1 : 0
        if (kind === "task") return fields | (notesField.checked ? 2 : 0)
        return fields | (descriptionField.checked ? 2 : 0) | (locationField.checked ? 4 : 0)
    }

    function openFor(ids, scope) {
        recordIds = ids.slice()
        defaultRecurrenceScope = scope
        findField.clear()
        replacementField.clear()
        titleField.checked = true
        notesField.checked = true
        descriptionField.checked = true
        locationField.checked = true
        recurrenceScopePicker.currentIndex = scope
        clearPreview()
        open()
    }

    function clearPreview() {
        previewed = false
        ++previewRequestToken
    }

    onPreviewResultRequestTokenChanged: {
        previewed = previewResultRequestToken === previewRequestToken &&
                    previewMessage.startsWith("Preview:")
    }

    onPrimaryAction: replaceRequested(recordIds, findField.text, replacementField.text,
                                      selectedFields(), recurrenceScopePicker.currentIndex)

    Label {
        Layout.fillWidth: true
        text: root.recordIds.length + " selected " + (root.kind === "task" ? "task" : "event") +
              (root.recordIds.length === 1 ? "" : "s")
        color: Theme.textSecondary
    }

    TextField {
        id: findField
        Layout.fillWidth: true
        placeholderText: "Find literal text"
        Accessible.name: placeholderText
        selectByMouse: true
        onTextChanged: root.clearPreview()
    }

    TextField {
        id: replacementField
        Layout.fillWidth: true
        placeholderText: "Replace with (empty removes text)"
        Accessible.name: placeholderText
        selectByMouse: true
        onTextChanged: root.clearPreview()
    }

    Label { text: "Fields"; font.bold: true; Accessible.role: Accessible.Heading }

    CheckBox {
        id: titleField
        text: "Title"
        Accessible.name: text
        onToggled: root.clearPreview()
    }

    CheckBox {
        id: notesField
        visible: root.kind === "task"
        text: "Notes"
        Accessible.name: text
        onToggled: root.clearPreview()
    }

    CheckBox {
        id: descriptionField
        visible: root.kind === "event"
        text: "Description"
        Accessible.name: text
        onToggled: root.clearPreview()
    }

    CheckBox {
        id: locationField
        visible: root.kind === "event"
        text: "Location"
        Accessible.name: text
        onToggled: root.clearPreview()
    }

    Label { text: "Recurring records"; font.bold: true; Accessible.role: Accessible.Heading }

    ComboBox {
        id: recurrenceScopePicker
        Layout.fillWidth: true
        model: ["Skip recurring", "Current occurrence", "Current + future", "Full series"]
        Accessible.name: "Recurring rewrite scope"
        onActivated: root.clearPreview()
    }

    Label {
        Layout.fillWidth: true
        text: root.kind === "event" && recurrenceScopePicker.currentIndex === 3
              ? "Full series uses Google Calendar’s series semantics; explicit exceptions retain their own overrides."
              : "Literal, case-sensitive replacement. Managed Task recurrence metadata is preserved."
        wrapMode: Text.WordWrap
        color: Theme.textSecondary
    }

    Button {
        Layout.fillWidth: true
        text: "Preview affected records"
        enabled: findField.text.length > 0 && root.selectedFields() > 0
        Accessible.name: text
        onClicked: {
            root.previewed = false
            ++root.previewRequestToken
            root.previewRequested(root.recordIds, findField.text, root.selectedFields(),
                                  recurrenceScopePicker.currentIndex, root.previewRequestToken)
        }
    }

    Label {
        Layout.fillWidth: true
        text: root.previewMessage
        color: Theme.textSecondary
        wrapMode: Text.WordWrap
        Accessible.name: text
    }
}
