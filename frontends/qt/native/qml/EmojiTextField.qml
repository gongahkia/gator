import QtQuick
import QtQuick.Controls

TextField {
    id: root
    property alias emojiAutocomplete: autocomplete

    EmojiAutocomplete {
        id: autocomplete
        target: root
    }

    Keys.onPressed: function(event) { autocomplete.handleKey(event) }
}
