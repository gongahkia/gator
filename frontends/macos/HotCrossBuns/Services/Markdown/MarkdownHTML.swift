import Foundation

// Google Calendar event description accepts a limited HTML subset
// (a, b, i, u, br, ul, ol, li). This file converts between markdown
// and that subset so our editor can stay markdown-first while the
// Calendar web UI keeps rendering rich text.
//
// Unknown HTML encountered on read is preserved as-is in the markdown
// body so that formatting authored elsewhere is not destroyed on the
// next write.
enum MarkdownHTML {
    static func markdownToCalendarHTML(_ markdown: String) -> String {
        guard markdown.isEmpty == false else { return "" }
        var lines = markdown.components(separatedBy: "\n")
        lines = groupListsInLines(lines)
        return lines.joined(separator: "<br>")
    }

    static func calendarHTMLToMarkdown(_ html: String) -> String {
        guard html.isEmpty == false else { return "" }
        var working = html
        working = working.replacingOccurrences(of: "\r\n", with: "\n")
        // Some Calendar clients save rich descriptions with the HTML tags
        // escaped as text (`&lt;p&gt;...&lt;a&gt;...`). Decode before tag
        // conversion so those common Calendar tags do not leak into the
        // read-only event view as literal markup.
        working = decodeBasicEntities(working)
        working = replaceLists(in: working)
        working = replaceInline(in: working)
        working = replaceParagraphsAndDivs(in: working)
        working = replaceBreaks(in: working)
        // Google often wraps bare URLs in <a>URL</a> (no href); strip the
        // empty anchor but keep the inner text. Runs AFTER replaceInline so
        // proper <a href="x">y</a> links have already been rewritten.
        working = stripEmptyAnchors(in: working)
        working = decodeBasicEntities(working)
        // Collapse 3+ consecutive blank lines (can happen when every <p> has
        // its own paragraph break next to a <br>) down to the standard double
        // newline Markdown paragraph separator.
        working = working.replacingMatches(of: #"\n{3,}"#) { _ in "\n\n" }
        return working.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    // MARK: markdown → HTML

    private static func groupListsInLines(_ lines: [String]) -> [String] {
        var result: [String] = []
        var buffer: [String] = []
        var currentKind: ListKind?

        func flush() {
            guard let kind = currentKind else {
                result.append(contentsOf: buffer)
                buffer.removeAll()
                return
            }
            let items = buffer.map { "<li>\(applyInline($0))</li>" }.joined()
            let tag = kind == .unordered ? "ul" : "ol"
            result.append("<\(tag)>\(items)</\(tag)>")
            buffer.removeAll()
            currentKind = nil
        }

        for line in lines {
            if let stripped = stripListMarker(line, kind: .unordered) {
                if currentKind != .unordered { flush() }
                currentKind = .unordered
                buffer.append(stripped)
            } else if let stripped = stripListMarker(line, kind: .ordered) {
                if currentKind != .ordered { flush() }
                currentKind = .ordered
                buffer.append(stripped)
            } else {
                flush()
                result.append(applyInline(line))
            }
        }
        flush()
        return result
    }

    private static func stripListMarker(_ line: String, kind: ListKind) -> String? {
        let trimmed = line.drop(while: { $0 == " " || $0 == "\t" })
        switch kind {
        case .unordered:
            if trimmed.hasPrefix("- ") { return String(trimmed.dropFirst(2)) }
            if trimmed.hasPrefix("* ") { return String(trimmed.dropFirst(2)) }
            return nil
        case .ordered:
            var iter = trimmed.makeIterator()
            var digits = ""
            while let ch = iter.next(), ch.isNumber {
                digits.append(ch)
            }
            guard digits.isEmpty == false else { return nil }
            let afterDigits = trimmed.dropFirst(digits.count)
            guard afterDigits.hasPrefix(". ") else { return nil }
            return String(afterDigits.dropFirst(2))
        }
    }

    private enum ListKind { case unordered, ordered }

    private static func applyInline(_ text: String) -> String {
        var output = ""
        var plain = ""
        var index = text.startIndex

        func flushPlain() {
            guard plain.isEmpty == false else { return }
            output += escapeText(plain)
            plain.removeAll(keepingCapacity: true)
        }

        while index < text.endIndex {
            if let parsed = parseLink(in: text, at: index) {
                flushPlain()
                if linkURLIsAllowed(parsed.url) {
                    output += "<a href=\"\(escapeAttr(parsed.url))\">\(escapeText(parsed.label))</a>"
                } else {
                    output += escapeText(String(text[index..<parsed.endIndex]))
                }
                index = parsed.endIndex
                continue
            }

            if text[index...].hasPrefix("**"),
               let end = text.range(of: "**", range: text.index(index, offsetBy: 2)..<text.endIndex) {
                flushPlain()
                output += "<b>\(applyInline(String(text[text.index(index, offsetBy: 2)..<end.lowerBound])))</b>"
                index = end.upperBound
                continue
            }

            if text[index...].hasPrefix("__"),
               let end = text.range(of: "__", range: text.index(index, offsetBy: 2)..<text.endIndex) {
                flushPlain()
                output += "<u>\(applyInline(String(text[text.index(index, offsetBy: 2)..<end.lowerBound])))</u>"
                index = end.upperBound
                continue
            }

            if text[index] == "*",
               text[text.index(after: index)...].hasPrefix("*") == false,
               let end = text.range(of: "*", range: text.index(after: index)..<text.endIndex),
               text[index..<end.lowerBound].contains("\n") == false {
                flushPlain()
                output += "<i>\(applyInline(String(text[text.index(after: index)..<end.lowerBound])))</i>"
                index = end.upperBound
                continue
            }

            plain.append(text[index])
            index = text.index(after: index)
        }

        flushPlain()
        return output
    }

    private static func escapeAttr(_ value: String) -> String {
        value
            .replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "\"", with: "&quot;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: ">", with: "&gt;")
    }

    private static func escapeText(_ value: String) -> String {
        value
            .replacingOccurrences(of: "&", with: "&amp;")
            .replacingOccurrences(of: "<", with: "&lt;")
            .replacingOccurrences(of: ">", with: "&gt;")
    }

    private static func parseLink(in text: String, at index: String.Index) -> (label: String, url: String, endIndex: String.Index)? {
        guard text[index] == "[" else { return nil }
        guard let labelEnd = text[index...].firstIndex(of: "]") else { return nil }
        let openParen = text.index(after: labelEnd)
        guard openParen < text.endIndex, text[openParen] == "(" else { return nil }
        let urlStart = text.index(after: openParen)
        guard let closeParen = text[urlStart...].firstIndex(of: ")") else { return nil }
        return (
            label: String(text[text.index(after: index)..<labelEnd]),
            url: String(text[urlStart..<closeParen]),
            endIndex: text.index(after: closeParen)
        )
    }

    private static func linkURLIsAllowed(_ rawURL: String) -> Bool {
        let trimmed = rawURL.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false,
              let components = URLComponents(string: trimmed),
              let scheme = components.scheme?.lowercased() else {
            return false
        }
        return ["https", "http", "mailto", "file"].contains(scheme)
    }

    // MARK: HTML → markdown

    private static func decodeBasicEntities(_ value: String) -> String {
        value
            .replacingOccurrences(of: "&nbsp;", with: " ")
            .replacingOccurrences(of: "&quot;", with: "\"")
            .replacingOccurrences(of: "&#39;", with: "'")
            .replacingOccurrences(of: "&apos;", with: "'")
            .replacingOccurrences(of: "&#34;", with: "\"")
            .replacingOccurrences(of: "&lt;", with: "<")
            .replacingOccurrences(of: "&gt;", with: ">")
            // &amp; last so we don't re-decode previously-decoded entities.
            .replacingOccurrences(of: "&amp;", with: "&")
    }

    private static func replaceBreaks(in value: String) -> String {
        value.replacingMatches(of: #"<br\s*/?>"#, options: [.caseInsensitive]) { _ in "\n" }
    }

    // Google Calendar wraps paragraphs in <p>...</p> and sometimes uses
    // <div> for line groupings. Convert both to a trailing double newline so
    // the rendered markdown reads as proper paragraphs. Empty <p></p> / <div></div>
    // collapse to a single newline so rows of whitespace aren't ballooned out.
    private static func replaceParagraphsAndDivs(in value: String) -> String {
        var working = value
        working = working.replacingMatches(of: #"<p[^>]*>\s*</p>"#, options: [.caseInsensitive]) { _ in "\n" }
        working = working.replacingMatches(of: #"<p[^>]*>(.*?)</p>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return groups[1] + "\n\n"
        }
        working = working.replacingMatches(of: #"<div[^>]*>\s*</div>"#, options: [.caseInsensitive]) { _ in "\n" }
        working = working.replacingMatches(of: #"<div[^>]*>(.*?)</div>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return groups[1] + "\n"
        }
        return working
    }

    // Google's Calendar web UI emits <a>https://...</a> (no href) for bare
    // URLs in pasted text. The main replaceInline regex requires href="…"
    // so these were passing through with the literal tags. Strip the
    // attribute-less anchor wrapper here — the URL inside becomes plain text
    // that the OS's link-detection can still render live.
    private static func stripEmptyAnchors(in value: String) -> String {
        value.replacingMatches(of: #"<a[^>]*>([^<]*)</a>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return groups[1]
        }
    }

    private static func replaceInline(in value: String) -> String {
        var working = value
        working = working.replacingMatches(of: #"<a[^>]*href=\"([^\"]*)\"[^>]*>(.*?)</a>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 3 else { return nil }
            return "[\(groups[2])](\(groups[1]))"
        }
        working = working.replacingMatches(of: #"<b>(.*?)</b>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return "**\(groups[1])**"
        }
        working = working.replacingMatches(of: #"<strong>(.*?)</strong>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return "**\(groups[1])**"
        }
        working = working.replacingMatches(of: #"<u>(.*?)</u>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return "__\(groups[1])__"
        }
        working = working.replacingMatches(of: #"<i>(.*?)</i>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return "*\(groups[1])*"
        }
        working = working.replacingMatches(of: #"<em>(.*?)</em>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return "*\(groups[1])*"
        }
        return working
    }

    private static func replaceLists(in value: String) -> String {
        var working = value
        working = working.replacingMatches(of: #"<ul[^>]*>(.*?)</ul>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return convertListItems(groups[1], ordered: false)
        }
        working = working.replacingMatches(of: #"<ol[^>]*>(.*?)</ol>"#, options: [.caseInsensitive, .dotMatchesLineSeparators]) { groups in
            guard groups.count == 2 else { return nil }
            return convertListItems(groups[1], ordered: true)
        }
        return working
    }

    private static func convertListItems(_ body: String, ordered: Bool) -> String {
        let itemPattern = #"<li[^>]*>(.*?)</li>"#
        let regex = try? NSRegularExpression(pattern: itemPattern, options: [.caseInsensitive, .dotMatchesLineSeparators])
        let nsBody = body as NSString
        let range = NSRange(location: 0, length: nsBody.length)
        var items: [String] = []
        regex?.enumerateMatches(in: body, options: [], range: range) { match, _, _ in
            guard let match, match.numberOfRanges >= 2 else { return }
            let content = nsBody.substring(with: match.range(at: 1))
            items.append(content.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        let lines: [String] = items.enumerated().map { index, content in
            ordered ? "\(index + 1). \(content)" : "- \(content)"
        }
        return "\n" + lines.joined(separator: "\n") + "\n"
    }
}

private extension String {
    func replacingMatches(
        of pattern: String,
        options: NSRegularExpression.Options = [],
        replacement: (_ groups: [String]) -> String?
    ) -> String {
        guard let regex = try? NSRegularExpression(pattern: pattern, options: options) else {
            return self
        }
        let nsSelf = self as NSString
        var result = ""
        var lastEnd = 0
        let range = NSRange(location: 0, length: nsSelf.length)
        regex.enumerateMatches(in: self, options: [], range: range) { match, _, _ in
            guard let match else { return }
            let matchRange = match.range
            if matchRange.location > lastEnd {
                result += nsSelf.substring(with: NSRange(location: lastEnd, length: matchRange.location - lastEnd))
            }
            var groups: [String] = []
            for i in 0..<match.numberOfRanges {
                let r = match.range(at: i)
                if r.location == NSNotFound {
                    groups.append("")
                } else {
                    groups.append(nsSelf.substring(with: r))
                }
            }
            if let sub = replacement(groups) {
                result += sub
            } else {
                result += nsSelf.substring(with: matchRange)
            }
            lastEnd = matchRange.location + matchRange.length
        }
        if lastEnd < nsSelf.length {
            result += nsSelf.substring(with: NSRange(location: lastEnd, length: nsSelf.length - lastEnd))
        }
        return result
    }
}
