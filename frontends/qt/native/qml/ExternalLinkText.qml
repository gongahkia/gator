import QtQuick

TextEdit {
    id: root
    property string plainText: ""
    signal linkRequested(string url)

    text: plainText
    textFormat: TextEdit.MarkdownText
    readOnly: true
    selectByMouse: true
    wrapMode: Text.WordWrap
    color: Theme.textPrimary
    Accessible.name: plainText

    onLinkActivated: function(link) { root.linkRequested(link) }
}
