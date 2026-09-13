import AppKit
import SwiftUI

private struct EmojiAutocompleteToken {
    let range: NSRange
    let query: String
}

private struct EmojiShortcodeEntry: Hashable {
    let shortcode: String
    let emoji: String
    let aliases: [String]

    var completionLabel: String { "\(emoji) :\(shortcode):" }

    func matches(_ query: String) -> Bool {
        shortcode.localizedCaseInsensitiveContains(query)
            || aliases.contains { $0.localizedCaseInsensitiveContains(query) }
    }
}

private enum EmojiShortcodeAutocomplete {
    static let entries: [EmojiShortcodeEntry] = [
        .init(shortcode: "grinning", emoji: "😀", aliases: ["smile", "happy"]),
        .init(shortcode: "smiley", emoji: "😃", aliases: ["happy", "joy"]),
        .init(shortcode: "smile", emoji: "😄", aliases: ["happy", "joy"]),
        .init(shortcode: "grin", emoji: "😁", aliases: ["happy"]),
        .init(shortcode: "laughing", emoji: "😆", aliases: ["lol", "satisfied"]),
        .init(shortcode: "sweat_smile", emoji: "😅", aliases: ["relief"]),
        .init(shortcode: "joy", emoji: "😂", aliases: ["tears", "laugh"]),
        .init(shortcode: "rofl", emoji: "🤣", aliases: ["laugh", "rolling"]),
        .init(shortcode: "slightly_smiling_face", emoji: "🙂", aliases: ["smile"]),
        .init(shortcode: "wink", emoji: "😉", aliases: ["winking"]),
        .init(shortcode: "blush", emoji: "😊", aliases: ["smile"]),
        .init(shortcode: "innocent", emoji: "😇", aliases: ["angel"]),
        .init(shortcode: "heart_eyes", emoji: "😍", aliases: ["love"]),
        .init(shortcode: "star_struck", emoji: "🤩", aliases: ["star", "excited"]),
        .init(shortcode: "kissing_heart", emoji: "😘", aliases: ["kiss"]),
        .init(shortcode: "thinking", emoji: "🤔", aliases: ["think"]),
        .init(shortcode: "neutral_face", emoji: "😐", aliases: ["neutral"]),
        .init(shortcode: "expressionless", emoji: "😑", aliases: ["blank"]),
        .init(shortcode: "unamused", emoji: "😒", aliases: ["meh"]),
        .init(shortcode: "sweat", emoji: "😓", aliases: ["worried"]),
        .init(shortcode: "pensive", emoji: "😔", aliases: ["sad"]),
        .init(shortcode: "confused", emoji: "😕", aliases: ["unsure"]),
        .init(shortcode: "upside_down", emoji: "🙃", aliases: ["upside_down_face"]),
        .init(shortcode: "worried", emoji: "😟", aliases: ["concerned"]),
        .init(shortcode: "cry", emoji: "😢", aliases: ["sad"]),
        .init(shortcode: "sob", emoji: "😭", aliases: ["cry"]),
        .init(shortcode: "angry", emoji: "😠", aliases: ["mad"]),
        .init(shortcode: "rage", emoji: "😡", aliases: ["angry"]),
        .init(shortcode: "exploding_head", emoji: "🤯", aliases: ["mind_blown"]),
        .init(shortcode: "flushed", emoji: "😳", aliases: ["embarrassed"]),
        .init(shortcode: "hot_face", emoji: "🥵", aliases: ["hot"]),
        .init(shortcode: "cold_face", emoji: "🥶", aliases: ["cold"]),
        .init(shortcode: "scream", emoji: "😱", aliases: ["fear"]),
        .init(shortcode: "sleeping", emoji: "😴", aliases: ["sleep"]),
        .init(shortcode: "party", emoji: "🥳", aliases: ["partying", "celebrate"]),
        .init(shortcode: "sunglasses", emoji: "😎", aliases: ["cool"]),
        .init(shortcode: "nerd", emoji: "🤓", aliases: ["nerd_face"]),
        .init(shortcode: "thumbsup", emoji: "👍", aliases: ["plus_one", "+1", "like"]),
        .init(shortcode: "thumbsdown", emoji: "👎", aliases: ["minus_one", "-1", "dislike"]),
        .init(shortcode: "clap", emoji: "👏", aliases: ["applause"]),
        .init(shortcode: "raised_hands", emoji: "🙌", aliases: ["hooray"]),
        .init(shortcode: "pray", emoji: "🙏", aliases: ["please", "thanks"]),
        .init(shortcode: "muscle", emoji: "💪", aliases: ["strong"]),
        .init(shortcode: "ok_hand", emoji: "👌", aliases: ["ok"]),
        .init(shortcode: "wave", emoji: "👋", aliases: ["hello"]),
        .init(shortcode: "eyes", emoji: "👀", aliases: ["look"]),
        .init(shortcode: "brain", emoji: "🧠", aliases: ["mind"]),
        .init(shortcode: "heart", emoji: "❤️", aliases: ["love"]),
        .init(shortcode: "orange_heart", emoji: "🧡", aliases: ["heart"]),
        .init(shortcode: "yellow_heart", emoji: "💛", aliases: ["heart"]),
        .init(shortcode: "green_heart", emoji: "💚", aliases: ["heart"]),
        .init(shortcode: "blue_heart", emoji: "💙", aliases: ["heart"]),
        .init(shortcode: "purple_heart", emoji: "💜", aliases: ["heart"]),
        .init(shortcode: "sparkles", emoji: "✨", aliases: ["magic"]),
        .init(shortcode: "star", emoji: "⭐", aliases: ["favorite"]),
        .init(shortcode: "fire", emoji: "🔥", aliases: ["lit"]),
        .init(shortcode: "zap", emoji: "⚡", aliases: ["lightning"]),
        .init(shortcode: "boom", emoji: "💥", aliases: ["collision"]),
        .init(shortcode: "100", emoji: "💯", aliases: ["hundred"]),
        .init(shortcode: "white_check_mark", emoji: "✅", aliases: ["done", "check"]),
        .init(shortcode: "x", emoji: "❌", aliases: ["cross", "wrong"]),
        .init(shortcode: "warning", emoji: "⚠️", aliases: ["alert"]),
        .init(shortcode: "question", emoji: "❓", aliases: ["help"]),
        .init(shortcode: "exclamation", emoji: "❗", aliases: ["important"]),
        .init(shortcode: "calendar", emoji: "📅", aliases: ["date"]),
        .init(shortcode: "spiral_calendar", emoji: "🗓️", aliases: ["calendar"]),
        .init(shortcode: "memo", emoji: "📝", aliases: ["note"]),
        .init(shortcode: "notebook", emoji: "📓", aliases: ["notes"]),
        .init(shortcode: "bookmark", emoji: "🔖", aliases: ["tag"]),
        .init(shortcode: "pushpin", emoji: "📌", aliases: ["pin"]),
        .init(shortcode: "paperclip", emoji: "📎", aliases: ["attach"]),
        .init(shortcode: "link", emoji: "🔗", aliases: ["url"]),
        .init(shortcode: "email", emoji: "✉️", aliases: ["mail"]),
        .init(shortcode: "phone", emoji: "☎️", aliases: ["call"]),
        .init(shortcode: "hourglass", emoji: "⌛", aliases: ["time"]),
        .init(shortcode: "alarm_clock", emoji: "⏰", aliases: ["alarm"]),
        .init(shortcode: "rocket", emoji: "🚀", aliases: ["launch"]),
        .init(shortcode: "bug", emoji: "🐛", aliases: ["debug"]),
        .init(shortcode: "computer", emoji: "💻", aliases: ["laptop"]),
        .init(shortcode: "keyboard", emoji: "⌨️", aliases: ["type"]),
        .init(shortcode: "books", emoji: "📚", aliases: ["study"]),
        .init(shortcode: "pencil", emoji: "✏️", aliases: ["write"]),
        .init(shortcode: "bulb", emoji: "💡", aliases: ["idea"]),
        .init(shortcode: "mag", emoji: "🔍", aliases: ["search"]),
        .init(shortcode: "lock", emoji: "🔒", aliases: ["secure"]),
        .init(shortcode: "key", emoji: "🔑", aliases: ["password"]),
        .init(shortcode: "coffee", emoji: "☕", aliases: ["cafe"]),
        .init(shortcode: "pizza", emoji: "🍕", aliases: ["food"]),
        .init(shortcode: "tada", emoji: "🎉", aliases: ["celebrate"]),
        .init(shortcode: "gift", emoji: "🎁", aliases: ["present"]),
        .init(shortcode: "medal", emoji: "🏅", aliases: ["award"]),
        .init(shortcode: "soccer", emoji: "⚽", aliases: ["sport"]),
        .init(shortcode: "climbing", emoji: "🧗", aliases: ["climb"]),
        .init(shortcode: "mountain", emoji: "⛰️", aliases: ["hike"]),
        .init(shortcode: "sunny", emoji: "☀️", aliases: ["sun"]),
        .init(shortcode: "moon", emoji: "🌙", aliases: ["night"]),
        .init(shortcode: "cloud", emoji: "☁️", aliases: ["weather"]),
        .init(shortcode: "rainbow", emoji: "🌈", aliases: ["color"])
    ]

    static func token(in string: String, selectedRange: NSRange) -> EmojiAutocompleteToken? {
        guard selectedRange.length == 0, selectedRange.location > 0 else { return nil }
        let nsString = string as NSString
        guard selectedRange.location <= nsString.length else { return nil }

        var tokenStart = selectedRange.location
        while tokenStart > 0 {
            let previous = nsString.substring(with: NSRange(location: tokenStart - 1, length: 1))
            guard isShortcodeCharacter(previous) else { break }
            tokenStart -= 1
        }

        let colonLocation = tokenStart - 1
        guard colonLocation >= 0 else { return nil }
        guard nsString.substring(with: NSRange(location: colonLocation, length: 1)) == ":" else { return nil }
        guard colonCanStartShortcode(in: nsString, at: colonLocation) else { return nil }

        let queryRange = NSRange(location: tokenStart, length: selectedRange.location - tokenStart)
        guard queryRange.length > 0 else { return nil }
        let query = nsString.substring(with: queryRange)
        return EmojiAutocompleteToken(
            range: NSRange(location: colonLocation, length: selectedRange.location - colonLocation),
            query: query
        )
    }

    static func completions(for query: String, limit: Int = 12) -> [String] {
        entries
            .filter { $0.matches(query) }
            .sorted { lhs, rhs in
                let lhsRank = rank(lhs, for: query)
                let rhsRank = rank(rhs, for: query)
                if lhsRank == rhsRank {
                    return lhs.shortcode.localizedCaseInsensitiveCompare(rhs.shortcode) == .orderedAscending
                }
                return lhsRank < rhsRank
            }
            .prefix(limit)
            .map(\.completionLabel)
    }

    static func emoji(for completionLabel: String) -> String? {
        entries.first { $0.completionLabel == completionLabel }?.emoji
    }

    private static func rank(_ entry: EmojiShortcodeEntry, for query: String) -> Int {
        let lowerQuery = query.lowercased()
        if entry.shortcode.localizedCaseInsensitiveCompare(query) == .orderedSame { return 0 }
        if entry.shortcode.lowercased().hasPrefix(lowerQuery) { return 1 }
        if entry.aliases.contains(where: { $0.localizedCaseInsensitiveCompare(query) == .orderedSame }) { return 2 }
        if entry.aliases.contains(where: { $0.lowercased().hasPrefix(lowerQuery) }) { return 3 }
        return 4
    }

    private static func isShortcodeCharacter(_ character: String) -> Bool {
        character.rangeOfCharacter(from: allowedShortcodeCharacters.inverted) == nil
    }

    private static var allowedShortcodeCharacters: CharacterSet {
        var set = CharacterSet.alphanumerics
        set.insert(charactersIn: "_+-")
        return set
    }

    private static func colonCanStartShortcode(in string: NSString, at colonLocation: Int) -> Bool {
        guard colonLocation > 0 else { return true }
        let previous = string.substring(with: NSRange(location: colonLocation - 1, length: 1))
        if previous.rangeOfCharacter(from: .whitespacesAndNewlines) != nil {
            return true
        }
        return "([{<\"'".contains(previous)
    }
}

// Edit-mode markdown surface that renders formatting live as the user
// types, keeping the raw syntax visible but dimmed (Obsidian-style live
// preview). Wraps an NSTextView so we can style arbitrary character
// ranges via NSTextStorage — SwiftUI's TextEditor can't do per-character
// attributes without resetting typing state.
//
// Public API mirrors SwiftUI's TextEditor: a text binding, optional
// placeholder, and size constraints. Callers don't need to know about
// NSText internals.
struct MarkdownLiveEditor: NSViewRepresentable {
    @Binding var text: String
    var placeholder: String
    var minHeight: CGFloat
    var maxHeight: CGFloat
    var baseFont: NSFont
    var theme: MarkdownHighlightTheme
    @Binding var selectedRange: NSRange
    var onFocusChange: ((Bool) -> Void)?
    var onPasteAttachments: (() -> Bool)?

    func makeCoordinator() -> Coordinator {
        Coordinator(self)
    }

    func makeNSView(context: Context) -> NSScrollView {
        let scroll = NSScrollView()
        scroll.hasVerticalScroller = true
        scroll.hasHorizontalScroller = false
        scroll.autohidesScrollers = true
        scroll.drawsBackground = false
        scroll.borderType = .noBorder

        let textStorage = NSTextStorage()
        let layoutManager = NSLayoutManager()
        textStorage.addLayoutManager(layoutManager)
        let textContainer = NSTextContainer(containerSize: NSSize(width: 0, height: CGFloat.greatestFiniteMagnitude))
        textContainer.widthTracksTextView = true
        textContainer.heightTracksTextView = false
        layoutManager.addTextContainer(textContainer)

        let tv = AttachmentPasteTextView(frame: .zero, textContainer: textContainer)
        tv.delegate = context.coordinator
        tv.handleAttachmentPaste = { [weak coordinator = context.coordinator] in
            coordinator?.parent.onPasteAttachments?() ?? false
        }
        tv.isRichText = false
        tv.isEditable = true
        tv.isSelectable = true
        tv.allowsUndo = true
        tv.drawsBackground = false
        tv.textContainerInset = NSSize(width: 4, height: 6)
        tv.font = baseFont
        tv.textColor = theme.baseColor
        tv.insertionPointColor = theme.accentColor
        tv.usesAdaptiveColorMappingForDarkAppearance = false
        tv.usesFindBar = true
        tv.minSize = NSSize(width: 0, height: minHeight)
        tv.maxSize = NSSize(width: CGFloat.greatestFiniteMagnitude, height: CGFloat.greatestFiniteMagnitude)
        tv.isVerticallyResizable = true
        tv.isHorizontallyResizable = false
        tv.autoresizingMask = [.width]
        tv.textContainer?.containerSize = NSSize(width: scroll.contentSize.width, height: CGFloat.greatestFiniteMagnitude)
        tv.textContainer?.widthTracksTextView = true
        tv.isAutomaticQuoteSubstitutionEnabled = false
        tv.isAutomaticDashSubstitutionEnabled = false
        tv.isAutomaticTextReplacementEnabled = false
        tv.isAutomaticSpellingCorrectionEnabled = false
        tv.isAutomaticLinkDetectionEnabled = false
        tv.smartInsertDeleteEnabled = false
        tv.typingAttributes = [
            .font: baseFont,
            .foregroundColor: theme.baseColor
        ]
        tv.string = text
        if let storage = tv.textStorage {
            MarkdownHighlighter.apply(to: storage, theme: theme)
        }
        context.coordinator.placeholderLabel = attachPlaceholder(to: tv)
        updatePlaceholderVisibility(coordinator: context.coordinator, isEmpty: text.isEmpty)
        scroll.documentView = tv
        return scroll
    }

    func updateNSView(_ nsView: NSScrollView, context: Context) {
        guard let tv = nsView.documentView as? NSTextView else { return }
        context.coordinator.parent = self
        if let tv = tv as? AttachmentPasteTextView {
            tv.handleAttachmentPaste = { [weak coordinator = context.coordinator] in
                coordinator?.parent.onPasteAttachments?() ?? false
            }
        }
        tv.insertionPointColor = theme.accentColor
        if tv.font != baseFont {
            tv.font = baseFont
        }
        tv.typingAttributes = [
            .font: baseFont,
            .foregroundColor: theme.baseColor
        ]
        if tv.string != text {
            context.coordinator.isApplyingViewUpdate = true
            tv.string = text
            context.coordinator.isApplyingViewUpdate = false
        }
        if let storage = tv.textStorage {
            MarkdownHighlighter.apply(to: storage, theme: theme)
        }
        let desiredSelection = MarkdownEditorTextMutation.clamped(selectedRange, textLength: (tv.string as NSString).length)
        if NSEqualRanges(tv.selectedRange(), desiredSelection) == false {
            context.coordinator.isApplyingViewUpdate = true
            tv.setSelectedRange(desiredSelection)
            context.coordinator.isApplyingViewUpdate = false
        }
        updatePlaceholderVisibility(coordinator: context.coordinator, isEmpty: text.isEmpty)
        if let placeholderLabel = context.coordinator.placeholderLabel {
            placeholderLabel.stringValue = placeholder
            placeholderLabel.textColor = theme.dimColor
            placeholderLabel.font = baseFont
        }
    }

    private func attachPlaceholder(to tv: NSTextView) -> NSTextField {
        let label = NSTextField(labelWithString: placeholder)
        label.translatesAutoresizingMaskIntoConstraints = false
        label.textColor = theme.dimColor
        label.font = baseFont
        label.isBezeled = false
        label.drawsBackground = false
        label.isEditable = false
        label.isSelectable = false
        tv.addSubview(label)
        NSLayoutConstraint.activate([
            label.leadingAnchor.constraint(equalTo: tv.leadingAnchor, constant: tv.textContainerInset.width + 5),
            label.topAnchor.constraint(equalTo: tv.topAnchor, constant: tv.textContainerInset.height + 1)
        ])
        return label
    }

    private func updatePlaceholderVisibility(coordinator: Coordinator, isEmpty: Bool) {
        coordinator.placeholderLabel?.isHidden = !isEmpty
    }

    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: MarkdownLiveEditor
        weak var placeholderLabel: NSTextField?
        private var isApplyingEmojiCompletion = false
        var isApplyingViewUpdate = false

        init(_ parent: MarkdownLiveEditor) {
            self.parent = parent
        }

        func textDidChange(_ notification: Notification) {
            guard let tv = notification.object as? NSTextView else { return }
            guard isApplyingViewUpdate == false else { return }
            let newString = tv.string
            if parent.text != newString {
                parent.text = newString
            }
            updateSelectedRange(from: tv)
            if let storage = tv.textStorage {
                MarkdownHighlighter.apply(to: storage, theme: parent.theme)
            }
            placeholderLabel?.isHidden = !newString.isEmpty
            // typingAttributes is reset on every keystroke so new input
            // inherits base styling rather than whatever run the cursor
            // happened to be sitting inside.
            tv.typingAttributes = [
                .font: parent.baseFont,
                .foregroundColor: parent.theme.baseColor
            ]
            scheduleEmojiCompletionIfNeeded(in: tv)
        }

        func textDidBeginEditing(_ notification: Notification) {
            parent.onFocusChange?(true)
        }

        func textDidEndEditing(_ notification: Notification) {
            parent.onFocusChange?(false)
        }

        func textViewDidChangeSelection(_ notification: Notification) {
            guard isApplyingViewUpdate == false else { return }
            guard let tv = notification.object as? NSTextView else { return }
            updateSelectedRange(from: tv)
        }

        func textView(
            _ textView: NSTextView,
            completions words: [String],
            forPartialWordRange charRange: NSRange,
            indexOfSelectedItem index: UnsafeMutablePointer<Int>?
        ) -> [String] {
            guard let token = EmojiShortcodeAutocomplete.token(in: textView.string, selectedRange: textView.selectedRange()) else {
                return []
            }
            let completions = EmojiShortcodeAutocomplete.completions(for: token.query)
            index?.pointee = completions.isEmpty ? -1 : 0
            return completions
        }

        func textView(
            _ textView: NSTextView,
            insertCompletion word: String,
            forPartialWordRange charRange: NSRange,
            movement: Int,
            isFinal flag: Bool
        ) {
            guard flag else { return }
            guard let emoji = EmojiShortcodeAutocomplete.emoji(for: word) else { return }
            guard let token = EmojiShortcodeAutocomplete.token(in: textView.string, selectedRange: textView.selectedRange()) else { return }

            isApplyingEmojiCompletion = true
            textView.replaceCharacters(in: token.range, with: emoji)
            parent.text = textView.string
            if let storage = textView.textStorage {
                MarkdownHighlighter.apply(to: storage, theme: parent.theme)
            }
            textView.typingAttributes = [
                .font: parent.baseFont,
                .foregroundColor: parent.theme.baseColor
            ]
            placeholderLabel?.isHidden = !textView.string.isEmpty
            isApplyingEmojiCompletion = false
        }

        private func scheduleEmojiCompletionIfNeeded(in textView: NSTextView) {
            guard isApplyingEmojiCompletion == false else { return }
            guard let token = EmojiShortcodeAutocomplete.token(in: textView.string, selectedRange: textView.selectedRange()) else { return }
            guard EmojiShortcodeAutocomplete.completions(for: token.query).isEmpty == false else { return }

            DispatchQueue.main.async { [weak self, weak textView] in
                guard let self, self.isApplyingEmojiCompletion == false, let textView else { return }
                guard let token = EmojiShortcodeAutocomplete.token(in: textView.string, selectedRange: textView.selectedRange()) else { return }
                guard EmojiShortcodeAutocomplete.completions(for: token.query).isEmpty == false else { return }
                textView.complete(nil)
            }
        }

        private func updateSelectedRange(from textView: NSTextView) {
            let range = textView.selectedRange()
            guard NSEqualRanges(parent.selectedRange, range) == false else { return }
            parent.selectedRange = range
        }
    }
}

private final class AttachmentPasteTextView: NSTextView {
    var handleAttachmentPaste: (() -> Bool)?

    override func paste(_ sender: Any?) {
        if handleAttachmentPaste?() == true {
            return
        }
        super.paste(sender)
    }

    override func keyDown(with event: NSEvent) {
        if event.modifierFlags.contains(.control),
           event.charactersIgnoringModifiers?.lowercased() == "v",
           handleAttachmentPaste?() == true {
            return
        }
        super.keyDown(with: event)
    }
}

// Convenience wrapper that resolves the current AppColor scheme + resolved
// editor font into a theme. Kept on this layer so MarkdownEditor stays a
// one-line call.
extension MarkdownHighlightTheme {
    @MainActor
    static func current(baseFont: NSFont) -> MarkdownHighlightTheme {
        let scheme = HCBColorSchemeStore.current
        return .make(
            baseFont: baseFont,
            ink: scheme.ink.nsColor,
            accent: scheme.ember.nsColor,
            stroke: scheme.cardStroke.nsColor
        )
    }
}
