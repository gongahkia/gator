import QtQuick
import QtQuick.Controls
import QtQuick.Layouts

Pane {
    id: root
    required property string clientId
    property bool hasClientSecret: false
    property bool busy: false
    property string statusMessage: ""
    property string setupStatusMessage: ""
    property string statusBeforeSetupAction: ""
    property string setupAction: ""
    property bool awaitingSetupStatus: false
    property bool clientIdSaved: false
    property bool clientSecretSaved: false
    property bool savingClientSecret: false
    property alias clientIdField: clientIdField
    property alias clientSecretField: clientSecretField
    property alias clientIdSavedIndicator: clientIdSavedIndicator
    property alias clientSecretSavedIndicator: clientSecretSavedIndicator
    property alias saveClientIdButton: saveClientIdButton
    property alias connectGoogleButton: connectGoogleButton
    property alias statusLabel: statusLabel
    signal saveClientIdRequested(string clientId, string clientSecret)
    signal connectGoogleRequested()

    Component.onCompleted: clientSecretSaved = hasClientSecret

    onHasClientSecretChanged: clientSecretSaved = hasClientSecret

    function awaitSetupStatus(action) {
        statusBeforeSetupAction = statusMessage
        setupStatusMessage = ""
        setupAction = action
        awaitingSetupStatus = true
        if (action === "save") {
            clientIdSaved = false
            clientSecretSaved = false
        }
    }

    onStatusMessageChanged: {
        if (awaitingSetupStatus && statusMessage !== statusBeforeSetupAction) {
            awaitingSetupStatus = false
            if (setupAction === "save" && statusMessage === "Google client configuration saved") {
                clientIdSaved = true
                clientSecretSaved = savingClientSecret
                clientSecretField.clear()
                return
            }
            setupStatusMessage = statusMessage
        }
    }

    Flickable {
        anchors.fill: parent
        contentWidth: width
        contentHeight: onboardingContent.implicitHeight
        boundsBehavior: Flickable.StopAtBounds
        clip: true

        ScrollBar.vertical: ScrollBar {
            policy: parent.contentHeight > parent.height ? ScrollBar.AsNeeded : ScrollBar.AlwaysOff
        }

        ColumnLayout {
            id: onboardingContent
            anchors.horizontalCenter: parent.horizontalCenter
            width: Math.min(parent.width - Theme.spacingLarge * 2, 560)
            spacing: Theme.spacingLarge

            Item { Layout.preferredHeight: Theme.spacingLarge * 3 }

            Label {
                Layout.fillWidth: true
                text: "Welcome to Hot Cross Buns"
                font.pixelSize: Theme.titleFontSize
                horizontalAlignment: Text.AlignHCenter
                Accessible.role: Accessible.Heading
                Accessible.name: text
            }

            Label {
                Layout.fillWidth: true
                text: "Connect Google to bring your Tasks and Calendar into one place."
                wrapMode: Text.WordWrap
                horizontalAlignment: Text.AlignHCenter
                color: Theme.textSecondary
            }

            Frame {
                Layout.fillWidth: true

                ColumnLayout {
                    anchors.fill: parent
                    spacing: Theme.spacingSmall

                    Label {
                        text: "1. Prepare Google Cloud"
                        font.pixelSize: Theme.labelFontSize
                        font.bold: true
                        Accessible.role: Accessible.Heading
                        Accessible.name: text
                    }

                    Label {
                        Layout.fillWidth: true
                        text: "Configure OAuth consent, enable Google Tasks, Google Calendar, and Google Drive APIs, then create a Desktop app OAuth client."
                        wrapMode: Text.WordWrap
                        color: Theme.textSecondary
                    }
                }
            }

            Frame {
                Layout.fillWidth: true

                ColumnLayout {
                    anchors.fill: parent
                    spacing: Theme.spacingMedium

                    Label {
                        text: "2. Add your OAuth client"
                        font.pixelSize: Theme.labelFontSize
                        font.bold: true
                        Accessible.role: Accessible.Heading
                        Accessible.name: text
                    }

                    Label {
                        Layout.fillWidth: true
                        text: root.hasClientSecret
                              ? "Enter a replacement client ID and secret together. Leave the secret blank to keep the saved value."
                              : "Paste the client ID and client secret from your downloaded Desktop OAuth client JSON."
                        wrapMode: Text.WordWrap
                        color: Theme.textSecondary
                    }

                    RowLayout {
                        Layout.fillWidth: true

                        TextField {
                            id: clientIdField
                            Layout.fillWidth: true
                            text: root.clientId
                            placeholderText: "Desktop OAuth client ID"
                            Accessible.name: placeholderText
                            selectByMouse: true
                            onTextEdited: {
                                root.clientIdSaved = false
                                root.clientSecretSaved = false
                            }
                        }

                        Label {
                            id: clientIdSavedIndicator
                            visible: root.clientIdSaved
                            text: "✓"
                            color: "#16803c"
                            font.pixelSize: Theme.labelFontSize
                            font.bold: true
                            Accessible.name: "Google client ID saved"
                        }
                    }

                    RowLayout {
                        Layout.fillWidth: true

                        TextField {
                            id: clientSecretField
                            Layout.fillWidth: true
                            placeholderText: root.clientSecretSaved ? "••••••••••••" : "Desktop OAuth client secret"
                            echoMode: TextInput.Password
                            Accessible.name: "Desktop OAuth client secret"
                            selectByMouse: true
                            onTextEdited: root.clientSecretSaved = false
                        }

                        Label {
                            id: clientSecretSavedIndicator
                            visible: root.clientSecretSaved
                            text: "✓"
                            color: "#16803c"
                            font.pixelSize: Theme.labelFontSize
                            font.bold: true
                            Accessible.name: "Google client secret saved"
                        }
                    }

                    Button {
                        id: saveClientIdButton
                        text: "Save OAuth client"
                        enabled: clientIdField.text.trim().length > 0 && !root.busy
                        Accessible.name: text
                        onClicked: {
                            root.savingClientSecret = clientSecretField.text.trim().length > 0
                                                       || (root.hasClientSecret
                                                           && clientIdField.text.trim() === root.clientId)
                            root.awaitSetupStatus("save")
                            root.saveClientIdRequested(clientIdField.text, clientSecretField.text)
                        }
                    }
                }
            }

            Frame {
                Layout.fillWidth: true

                ColumnLayout {
                    anchors.fill: parent
                    spacing: Theme.spacingMedium

                    Label {
                        text: "3. Connect Google"
                        font.pixelSize: Theme.labelFontSize
                        font.bold: true
                        Accessible.role: Accessible.Heading
                        Accessible.name: text
                    }

                    Label {
                        Layout.fillWidth: true
                        text: "After saving, select Connect Google and finish consent in your browser. Hot Cross Buns uses a temporary local callback; return to the app after the browser confirms connection."
                        wrapMode: Text.WordWrap
                        color: Theme.textSecondary
                    }

                    Button {
                        id: connectGoogleButton
                        text: "Connect Google"
                        enabled: root.clientId.trim().length > 0 && !root.busy
                        Accessible.name: text
                        onClicked: {
                            root.awaitSetupStatus("connect")
                            root.connectGoogleRequested()
                        }
                    }
                }
            }

            Label {
                id: statusLabel
                Layout.fillWidth: true
                visible: root.setupStatusMessage.length > 0
                text: root.setupStatusMessage
                wrapMode: Text.WordWrap
                color: Theme.textSecondary
                Accessible.name: text
            }

            Item { Layout.preferredHeight: Theme.spacingLarge * 3 }
        }
    }
}
