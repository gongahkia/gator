import QtQuick
import QtQuick.Controls
import QtQuick.Layouts
import "EmojiCatalog.js" as EmojiCatalog

Popup {
    id: root
    property var target: null
    property int maxResults: 8
    property int selectedIndex: 0
    property int tokenStart: -1
    property var results: []
    property bool suppressRefresh: false
    property string triggerQuery: ""
    property bool hasResults: results.length > 0

    parent: target && target.window ? target.window.contentItem : null
    width: Math.min(360, parent ? parent.width - Theme.spacingLarge * 2 : 360)
    height: Math.min(280, resultList.contentHeight + Theme.spacingSmall * 2)
    padding: Theme.spacingSmall
    modal: false
    focus: false
    closePolicy: Popup.CloseOnEscape | Popup.CloseOnPressOutside

    background: Rectangle {
        color: Theme.surface
        border.color: Theme.textSecondary
        border.width: 1
        radius: 6
    }

    function clear() {
        tokenStart = -1
        triggerQuery = ""
        results = []
        selectedIndex = 0
        if (opened) close()
    }

    function updatePosition() {
        if (!target || !parent) return
        const point = target.mapToItem(parent, 0, target.height)
        x = Math.max(Theme.spacingSmall, Math.min(point.x, parent.width - width - Theme.spacingSmall))
        const below = point.y + target.height + Theme.spacingSmall
        const above = point.y - height - Theme.spacingSmall
        y = below + height <= parent.height ? below : Math.max(Theme.spacingSmall, above)
    }

    function triggerAt(text, cursor) {
        const source = String(text || "")
        if (!Number.isInteger(cursor) || cursor < 0 || cursor > source.length) return null
        const match = /(^|[\s([{\"'])(:[A-Za-z0-9_+-]*)$/.exec(source.slice(0, cursor))
        if (!match || match[2].length > 49) return null
        return { start: cursor - match[2].length, query: match[2].slice(1) }
    }

    function search(query) {
        return EmojiCatalog.search(query, maxResults)
    }

    function refresh() {
        if (suppressRefresh || !target || !target.activeFocus || target.inputMethodComposing === true ||
                target.selectionStart !== target.selectionEnd) {
            clear()
            return
        }
        const text = String(target.text || "")
        const cursor = Number(target.cursorPosition)
        if (!Number.isInteger(cursor) || cursor < 0 || cursor > text.length) {
            clear()
            return
        }
        const trigger = triggerAt(text, cursor)
        if (trigger === null) {
            clear()
            return
        }
        tokenStart = trigger.start
        triggerQuery = trigger.query
        results = search(triggerQuery)
        selectedIndex = 0
        if (!hasResults) {
            if (opened) close()
            return
        }
        updatePosition()
        if (!opened) open()
    }

    function choose(index) {
        if (!target || index < 0 || index >= results.length || tokenStart < 0) return false
        const choice = results[index]
        const cursor = Number(target.cursorPosition)
        const text = String(target.text || "")
        if (!Number.isInteger(cursor) || cursor < tokenStart) return false
        const suffix = text.slice(cursor, cursor + 1)
        suppressRefresh = true
        target.remove(tokenStart, cursor)
        target.insert(tokenStart, choice.emoji)
        target.cursorPosition = tokenStart + choice.emoji.length
        if (suffix.length === 0 || /\s/.test(suffix)) {
            target.insert(target.cursorPosition, " ")
            target.cursorPosition += 1
        }
        suppressRefresh = false
        target.forceActiveFocus()
        clear()
        return true
    }

    function handleKey(event) {
        if (!visible || !hasResults || event.accepted) return false
        if (event.key === Qt.Key_Down) {
            selectedIndex = (selectedIndex + 1) % results.length
        } else if (event.key === Qt.Key_Up) {
            selectedIndex = (selectedIndex + results.length - 1) % results.length
        } else if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter || event.key === Qt.Key_Tab) {
            choose(selectedIndex)
        } else if (event.key === Qt.Key_Escape) {
            clear()
        } else {
            return false
        }
        event.accepted = true
        return true
    }

    Connections {
        target: root.target
        function onTextChanged() { Qt.callLater(root.refresh) }
        function onCursorPositionChanged() { Qt.callLater(root.refresh) }
        function onSelectionStartChanged() { Qt.callLater(root.refresh) }
        function onSelectionEndChanged() { Qt.callLater(root.refresh) }
        function onActiveFocusChanged() {
            if (!root.target || !root.target.activeFocus) root.clear()
        }
    }

    onParentChanged: Qt.callLater(updatePosition)
    onWidthChanged: Qt.callLater(updatePosition)
    onHeightChanged: Qt.callLater(updatePosition)

    contentItem: ListView {
        id: resultList
        implicitHeight: contentHeight
        clip: true
        model: root.results
        interactive: contentHeight > 264
        currentIndex: root.selectedIndex
        boundsBehavior: Flickable.StopAtBounds

        delegate: ItemDelegate {
            required property var modelData
            required property int index
            width: resultList.width
            text: modelData.emoji + "   :" + modelData.name + ":"
            highlighted: index === root.selectedIndex
            Accessible.name: "Insert emoji " + modelData.name
            onHoveredChanged: if (hovered) root.selectedIndex = index
            onClicked: root.choose(index)
        }
    }
}
