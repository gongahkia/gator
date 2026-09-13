import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Dialog {
    id: root
    default property alias body: bodyLayout.data
    property string primaryText: "Save"
    property string secondaryText: "Cancel"
    property string destructiveText: ""
    property bool primaryDestructive: false
    property bool closeOnPrimaryAction: true
    property bool closeOnDestructiveAction: true
    property alias primaryButton: primaryButton
    property alias primaryEnabled: primaryButton.enabled
    property alias secondaryButton: secondaryButton
    signal primaryAction()
    signal secondaryAction()
    signal destructiveAction()

    modal: true
    focus: true
    padding: Theme.spacingLarge
    closePolicy: Popup.CloseOnEscape
    width: Math.min(520, parent ? parent.width - Theme.spacingLarge * 2 : 520)

    header: Label {
        text: root.title
        visible: text.length > 0
        padding: Theme.spacingLarge
        font.bold: true
        font.pixelSize: Theme.labelFontSize
        Accessible.role: Accessible.Heading
        Accessible.name: text
    }

    contentItem: Flickable {
        id: bodyFlickable
        implicitWidth: bodyLayout.implicitWidth
        implicitHeight: Math.min(bodyLayout.implicitHeight, root.parent ? root.parent.height * 0.65 : 600)
        contentWidth: width
        contentHeight: bodyLayout.implicitHeight
        clip: true
        boundsBehavior: Flickable.StopAtBounds
        flickableDirection: Flickable.VerticalFlick

        ScrollBar.vertical: ScrollBar {
            policy: bodyFlickable.contentHeight > bodyFlickable.height ? ScrollBar.AsNeeded : ScrollBar.AlwaysOff
        }

        ColumnLayout {
            id: bodyLayout
            width: bodyFlickable.width
            spacing: Theme.spacingMedium
        }
    }

    footer: DialogButtonBox {
        padding: Theme.spacingLarge

        Button {
            id: secondaryButton
            text: root.secondaryText
            Accessible.name: text
            onClicked: {
                root.secondaryAction()
                root.reject()
            }
        }

        Item { Layout.fillWidth: true }

        Button {
            id: primaryButton
            text: root.primaryText
            Accessible.name: text
            Accessible.description: root.primaryDestructive ? "Destructive action" : "Primary action"
            palette.button: root.primaryDestructive ? Theme.destructive : Theme.accent
            onClicked: {
                const shouldClose = root.closeOnPrimaryAction
                root.primaryAction()
                if (shouldClose) root.accept()
            }
        }

        Button {
            visible: root.destructiveText.length > 0
            text: root.destructiveText
            Accessible.name: text
            Accessible.description: "Destructive action"
            palette.button: Theme.destructive
            onClicked: {
                root.destructiveAction()
                if (root.closeOnDestructiveAction) root.accept()
            }
        }
    }
}
