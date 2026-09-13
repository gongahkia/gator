import QtQuick
import QtQuick.Controls

TextArea {
    id: root
    property alias emojiAutocomplete: autocomplete

    EmojiAutocomplete {
        id: autocomplete
        target: root
    }

    Keys.onPressed: function(event) { autocomplete.handleKey(event) }
}
