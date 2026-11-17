import Foundation

/// Presentation only. Commands, structured results and exact reviews bypass it.
public struct NativeMessageBlock: Identifiable, Equatable, Sendable {
    public enum Kind: Equatable, Sendable { case paragraph, heading(Int), list, code(String) }
    public struct ListItem: Equatable, Sendable {
        public let depth: Int
        public let marker: String
        public let text: String
    }
    public let id: Int
    public let kind: Kind
    public let text: String
    public let listItem: ListItem?
}
public enum NativeMessageContent {
    public static func blocks(_ text: String) -> [NativeMessageBlock] {
        var result: [NativeMessageBlock] = [], paragraph: [String] = [], code: [String] = []
        var fence: String?, language = "", indents: [Int] = [], blank = false
        func append(_ kind: NativeMessageBlock.Kind, _ text: String, list: NativeMessageBlock.ListItem? = nil) {
            result.append(.init(id: result.count, kind: kind, text: text, listItem: list))
        }
        func flush() { if !paragraph.isEmpty { append(.paragraph, paragraph.joined(separator: "\n")); paragraph = [] } }
        for line in text.components(separatedBy: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if let active = fence {
                if trimmed.hasPrefix(active) { append(.code(language), code.joined(separator: "\n")); code = []; fence = nil }
                else { code.append(line) }
                continue
            }
            if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                flush(); indents = []; fence = String(trimmed.prefix(3)); language = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces); continue
            }
            if trimmed.isEmpty { flush(); blank = true; continue }
            let hashes = trimmed.prefix { $0 == "#" }.count
            if (1...6).contains(hashes), trimmed.dropFirst(hashes).hasPrefix(" ") {
                flush(); indents = []; append(.heading(hashes), String(trimmed.dropFirst(hashes + 1))); blank = false; continue
            }
            let indent = line.prefix { $0 == " " || $0 == "\t" }.reduce(0) { $1 == "\t" ? $0 + 4 - $0 % 4 : $0 + 1 }
            if let (marker, content) = listStart(trimmed) {
                flush()
                while let last = indents.last, last > indent { indents.removeLast() }
                if indents.last != indent { indents.append(indent) }
                let item = NativeMessageBlock.ListItem(depth: min(indents.count - 1, 32), marker: marker, text: content)
                append(.list, marker + " " + content, list: item); blank = false; continue
            }
            if let parentIndent = indents.last, indent > parentIndent, let previous = result.last, let item = previous.listItem {
                let content = item.text + (blank ? "\n\n" : "\n") + trimmed
                result[result.count - 1] = .init(id: previous.id, kind: .list, text: item.marker + " " + content,
                    listItem: .init(depth: item.depth, marker: item.marker, text: content))
            } else { indents = []; paragraph.append(line) }
            blank = false
        }
        flush()
        if fence != nil { append(.code(language), code.joined(separator: "\n")) }
        return result
    }
    private static func listStart(_ line: String) -> (String, String)? {
        let characters = line[...]
        let marker: Substring
        if let first = characters.first, "-*+".contains(first) { marker = characters.prefix(1) }
        else {
            let digits = characters.prefix { $0.isASCII && $0.isNumber }
            guard (1...9).contains(digits.count), let punctuation = characters.dropFirst(digits.count).first, punctuation == "." || punctuation == ")" else { return nil }
            marker = characters.prefix(digits.count + 1)
        }
        let remainder = characters.dropFirst(marker.count)
        guard remainder.first == " " || remainder.first == "\t" else { return nil }
        return (marker == "-" || marker == "*" || marker == "+" ? "•" : String(marker), String(remainder.drop(while: { $0 == " " || $0 == "\t" })))
    }
}
