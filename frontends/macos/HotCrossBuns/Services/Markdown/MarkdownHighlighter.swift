import AppKit

// Obsidian-style live-preview highlighter. Takes a plain-text markdown
// source in an NSTextStorage and applies attributes so syntax delimiters
// (**, #, `, [, ]) are dimmed but kept visible, while the surrounding
// content adopts the formatting the syntax describes (bold, italic, mono,
// headline size, etc.). View mode still uses MarkdownText/MarkdownBlock
// to hide syntax entirely — this path is only for edit mode.
//
// The theme is palette-agnostic: callers pass current scheme colors so
// dim/accent resolve correctly for every HCBColorScheme.
struct MarkdownHighlightTheme {
    var baseFont: NSFont
    var baseColor: NSColor
    var dimColor: NSColor         // syntax markers + urls
    var accentColor: NSColor      // link text
    var codeBackground: NSColor   // inline/block code tint
    var quoteColor: NSColor       // blockquote body / completed task

    static func make(baseFont: NSFont, ink: NSColor, accent: NSColor, stroke: NSColor) -> MarkdownHighlightTheme {
        MarkdownHighlightTheme(
            baseFont: baseFont,
            baseColor: ink,
            dimColor: ink.withAlphaComponent(0.38),
            accentColor: accent,
            codeBackground: stroke.withAlphaComponent(0.55),
            quoteColor: ink.withAlphaComponent(0.72)
        )
    }
}

enum MarkdownHighlighter {
    // Apply attributes to the full range of `storage`. Non-destructive to
    // the underlying characters. Safe to call on every keystroke.
    static func apply(to storage: NSTextStorage, theme: MarkdownHighlightTheme) {
        let text = storage.string
        let fullRange = NSRange(location: 0, length: (text as NSString).length)
        storage.beginEditing()
        storage.setAttributes([
            .font: theme.baseFont,
            .foregroundColor: theme.baseColor
        ], range: fullRange)

        let fenceRanges = applyBlockLevel(storage: storage, text: text, theme: theme)
        applyInline(storage: storage, text: text, theme: theme, fenceRanges: fenceRanges)

        storage.endEditing()
    }

    // MARK: - Block pass

    // Returns the set of character ranges covered by fenced-code regions,
    // so the inline pass can skip them.
    private static func applyBlockLevel(storage: NSTextStorage, text: String, theme: MarkdownHighlightTheme) -> [NSRange] {
        let ns = text as NSString
        var lineStart = 0
        var inFence = false
        var fenceStart: Int = 0
        var fenceRanges: [NSRange] = []
        let monoFont = FontVariant.mono(size: theme.baseFont.pointSize)

        while lineStart < ns.length {
            var lineEnd: Int = 0
            var contentsEnd: Int = 0
            ns.getLineStart(nil, end: &lineEnd, contentsEnd: &contentsEnd, for: NSRange(location: lineStart, length: 0))
            let contentRange = NSRange(location: lineStart, length: contentsEnd - lineStart)
            let lineWithTerminator = NSRange(location: lineStart, length: lineEnd - lineStart)
            let line = ns.substring(with: contentRange)

            if inFence {
                storage.addAttribute(.font, value: monoFont, range: contentRange)
                storage.addAttribute(.backgroundColor, value: theme.codeBackground, range: lineWithTerminator)
                if isFence(line) {
                    storage.addAttribute(.foregroundColor, value: theme.dimColor, range: contentRange)
                    fenceRanges.append(NSRange(location: fenceStart, length: lineEnd - fenceStart))
                    inFence = false
                }
            } else if isFence(line) {
                fenceStart = lineStart
                inFence = true
                storage.addAttribute(.foregroundColor, value: theme.dimColor, range: contentRange)
                storage.addAttribute(.font, value: monoFont, range: contentRange)
                storage.addAttribute(.backgroundColor, value: theme.codeBackground, range: lineWithTerminator)
            } else if let header = headerInfo(line: line) {
                let hashRange = NSRange(location: lineStart, length: header.syntaxCount)
                let size = theme.baseFont.pointSize * header.sizeMultiplier
                let font = FontVariant.bold(base: theme.baseFont, size: size)
                storage.addAttribute(.font, value: font, range: contentRange)
                storage.addAttribute(.foregroundColor, value: theme.dimColor, range: hashRange)
            } else if isHorizontalRule(line) {
                storage.addAttribute(.foregroundColor, value: theme.dimColor, range: contentRange)
            } else if let quoteCount = blockquoteSyntaxCount(line: line) {
                let marker = NSRange(location: lineStart, length: quoteCount)
                storage.addAttribute(.foregroundColor, value: theme.dimColor, range: marker)
                let bodyStart = lineStart + quoteCount
                let bodyLen = contentsEnd - bodyStart
                if bodyLen > 0 {
                    let bodyRange = NSRange(location: bodyStart, length: bodyLen)
                    storage.addAttribute(.font, value: FontVariant.italic(base: theme.baseFont), range: bodyRange)
                    storage.addAttribute(.foregroundColor, value: theme.quoteColor, range: bodyRange)
                }
            } else if let marker = listMarker(line: line, lineStart: lineStart) {
                storage.addAttribute(.foregroundColor, value: theme.dimColor, range: marker.range)
                let afterMarker = marker.range.location + marker.range.length
                if let check = taskCheckbox(in: ns, at: afterMarker, contentsEnd: contentsEnd) {
                    let checkRange = NSRange(location: afterMarker, length: check.length)
                    storage.addAttribute(.foregroundColor, value: theme.dimColor, range: checkRange)
                    if check.isChecked {
                        let remainderStart = afterMarker + check.length
                        let remainderLen = contentsEnd - remainderStart
                        if remainderLen > 0 {
                            let rem = NSRange(location: remainderStart, length: remainderLen)
                            storage.addAttribute(.strikethroughStyle, value: NSUnderlineStyle.single.rawValue, range: rem)
                            storage.addAttribute(.foregroundColor, value: theme.quoteColor, range: rem)
                        }
                    }
                }
            }

            if lineEnd == lineStart { break }
            lineStart = lineEnd
        }

        if inFence {
            fenceRanges.append(NSRange(location: fenceStart, length: ns.length - fenceStart))
        }
        return fenceRanges
    }

    // MARK: - Inline pass

    private static func applyInline(storage: NSTextStorage, text: String, theme: MarkdownHighlightTheme, fenceRanges: [NSRange]) {
        var consumed = IndexSet()
        for r in fenceRanges {
            consumed.insert(integersIn: r.location..<(r.location + r.length))
        }

        // 1. Inline code — masks everything inside.
        for m in matches(pattern: #"`([^`\n]+)`"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            let open = NSRange(location: m.range.location, length: 1)
            let close = NSRange(location: m.range.location + m.range.length - 1, length: 1)
            storage.addAttribute(.font, value: FontVariant.mono(size: theme.baseFont.pointSize), range: m.range)
            storage.addAttribute(.backgroundColor, value: theme.codeBackground, range: m.range)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: open)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: close)
            consume(&consumed, m.range)
        }

        // 2. Images before links.
        for m in matches(pattern: #"!\[([^\]\n]*)\]\(([^)\n]+)\)"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            styleLink(storage: storage, match: m, theme: theme)
            consume(&consumed, m.range)
        }

        // 3. Links.
        for m in matches(pattern: #"\[([^\]\n]+)\]\(([^)\n]+)\)"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            styleLink(storage: storage, match: m, theme: theme)
            consume(&consumed, m.range)
        }

        // 4. Bold+italic.
        for m in matches(pattern: #"(\*\*\*|___)([^*_\n]+)(\*\*\*|___)"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            let open = NSRange(location: m.range.location, length: 3)
            let close = NSRange(location: m.range.location + m.range.length - 3, length: 3)
            let inner = NSRange(location: m.range.location + 3, length: m.range.length - 6)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: open)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: close)
            storage.addAttribute(.font, value: FontVariant.boldItalic(base: theme.baseFont), range: inner)
            consume(&consumed, m.range)
        }

        // 5. Bold.
        for m in matches(pattern: #"(\*\*|__)([^*_\n]+)(\*\*|__)"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            let open = NSRange(location: m.range.location, length: 2)
            let close = NSRange(location: m.range.location + m.range.length - 2, length: 2)
            let inner = NSRange(location: m.range.location + 2, length: m.range.length - 4)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: open)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: close)
            storage.addAttribute(.font, value: FontVariant.bold(base: theme.baseFont), range: inner)
            consume(&consumed, m.range)
        }

        // 6. Italic.
        for m in matches(pattern: #"(?<![\*_])([\*_])([^\*_\n]+)\1(?![\*_])"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            let open = NSRange(location: m.range.location, length: 1)
            let close = NSRange(location: m.range.location + m.range.length - 1, length: 1)
            let inner = NSRange(location: m.range.location + 1, length: m.range.length - 2)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: open)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: close)
            storage.addAttribute(.font, value: FontVariant.italic(base: theme.baseFont), range: inner)
            consume(&consumed, m.range)
        }

        // 7. Strikethrough.
        for m in matches(pattern: #"~~([^~\n]+)~~"#, in: text) {
            if overlaps(m.range, consumed) { continue }
            let open = NSRange(location: m.range.location, length: 2)
            let close = NSRange(location: m.range.location + m.range.length - 2, length: 2)
            let inner = NSRange(location: m.range.location + 2, length: m.range.length - 4)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: open)
            storage.addAttribute(.foregroundColor, value: theme.dimColor, range: close)
            storage.addAttribute(.strikethroughStyle, value: NSUnderlineStyle.single.rawValue, range: inner)
            consume(&consumed, m.range)
        }
    }

    private static func styleLink(storage: NSTextStorage, match: NSTextCheckingResult, theme: MarkdownHighlightTheme) {
        let full = match.range
        let textGroup = match.range(at: 1)
        storage.addAttribute(.foregroundColor, value: theme.dimColor, range: full)
        if textGroup.location != NSNotFound, textGroup.length > 0 {
            storage.addAttribute(.foregroundColor, value: theme.accentColor, range: textGroup)
            storage.addAttribute(.underlineStyle, value: NSUnderlineStyle.single.rawValue, range: textGroup)
        }
    }

    // MARK: - Helpers

    private static func matches(pattern: String, in text: String) -> [NSTextCheckingResult] {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: []) else { return [] }
        let range = NSRange(location: 0, length: (text as NSString).length)
        return regex.matches(in: text, options: [], range: range)
    }

    private static func overlaps(_ range: NSRange, _ consumed: IndexSet) -> Bool {
        for i in range.location..<(range.location + range.length) {
            if consumed.contains(i) { return true }
        }
        return false
    }

    private static func consume(_ consumed: inout IndexSet, _ range: NSRange) {
        consumed.insert(integersIn: range.location..<(range.location + range.length))
    }

    private static func isFence(_ line: String) -> Bool {
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        return trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~")
    }

    private static func isHorizontalRule(_ line: String) -> Bool {
        let trimmed = line.trimmingCharacters(in: .whitespaces)
        guard trimmed.count >= 3 else { return false }
        return trimmed.allSatisfy({ $0 == "-" }) || trimmed.allSatisfy({ $0 == "*" }) || trimmed.allSatisfy({ $0 == "_" })
    }

    private struct HeaderInfo {
        let syntaxCount: Int
        let sizeMultiplier: CGFloat
    }

    private static func headerInfo(line: String) -> HeaderInfo? {
        var idx = line.startIndex
        var hashes = 0
        while idx < line.endIndex, line[idx] == "#", hashes < 6 {
            hashes += 1
            idx = line.index(after: idx)
        }
        guard hashes >= 1, idx < line.endIndex, line[idx] == " " else { return nil }
        let multipliers: [CGFloat] = [1.6, 1.4, 1.22, 1.1, 1.05, 1.0]
        return HeaderInfo(syntaxCount: hashes + 1, sizeMultiplier: multipliers[hashes - 1])
    }

    private static func blockquoteSyntaxCount(line: String) -> Int? {
        var leading = 0
        for c in line {
            if c == " " || c == "\t" { leading += 1; continue }
            break
        }
        let trimmed = line.drop(while: { $0 == " " || $0 == "\t" })
        if trimmed.hasPrefix("> ") { return leading + 2 }
        if trimmed == ">" { return leading + 1 }
        return nil
    }

    private struct ListMarkerInfo { let range: NSRange }

    private static func listMarker(line: String, lineStart: Int) -> ListMarkerInfo? {
        var leading = 0
        for c in line {
            if c == " " || c == "\t" { leading += 1; continue }
            break
        }
        let remaining = line.dropFirst(leading)
        if remaining.hasPrefix("- ") || remaining.hasPrefix("* ") || remaining.hasPrefix("+ ") {
            return ListMarkerInfo(range: NSRange(location: lineStart + leading, length: 2))
        }
        var digits = 0
        for c in remaining {
            if c.isNumber { digits += 1 } else { break }
        }
        guard digits > 0 else { return nil }
        let afterDigits = remaining.dropFirst(digits)
        guard let first = afterDigits.first, first == "." || first == ")" else { return nil }
        let afterPunct = afterDigits.dropFirst()
        guard afterPunct.first == " " else { return nil }
        return ListMarkerInfo(range: NSRange(location: lineStart + leading, length: digits + 2))
    }

    private struct TaskCheckbox { let length: Int; let isChecked: Bool }

    private static func taskCheckbox(in ns: NSString, at location: Int, contentsEnd: Int) -> TaskCheckbox? {
        guard contentsEnd - location >= 4 else { return nil }
        let head = ns.substring(with: NSRange(location: location, length: 4))
        if head == "[ ] " { return TaskCheckbox(length: 4, isChecked: false) }
        if head.lowercased() == "[x] " { return TaskCheckbox(length: 4, isChecked: true) }
        return nil
    }
}

// MARK: - Font variants

private enum FontVariant {
    static func bold(base: NSFont, size: CGFloat? = nil) -> NSFont {
        variant(of: base, traits: .bold, size: size)
    }
    static func italic(base: NSFont, size: CGFloat? = nil) -> NSFont {
        variant(of: base, traits: .italic, size: size)
    }
    static func boldItalic(base: NSFont, size: CGFloat? = nil) -> NSFont {
        variant(of: base, traits: [.bold, .italic], size: size)
    }
    static func mono(size: CGFloat) -> NSFont {
        NSFont.monospacedSystemFont(ofSize: size, weight: .regular)
    }

    private static func variant(of font: NSFont, traits: NSFontDescriptor.SymbolicTraits, size: CGFloat?) -> NSFont {
        let pt = size ?? font.pointSize
        let descriptor = font.fontDescriptor.withSymbolicTraits(traits)
        if let f = NSFont(descriptor: descriptor, size: pt) {
            return f
        }
        var fallback = NSFont.systemFont(ofSize: pt)
        if traits.contains(.bold) {
            fallback = NSFontManager.shared.convert(fallback, toHaveTrait: .boldFontMask)
        }
        if traits.contains(.italic) {
            fallback = NSFontManager.shared.convert(fallback, toHaveTrait: .italicFontMask)
        }
        return fallback
    }
}
