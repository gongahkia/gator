import Foundation

/// Presentation only. Commands, structured results and exact reviews bypass it.
public struct NativeMessageBlock: Identifiable, Equatable, Sendable {
    public enum Kind: Equatable, Sendable { case paragraph, heading(Int), list, code(String) }
    public let id: Int
    public let kind: Kind
    public let text: String
}
public enum NativeMessageContent {
    public static func blocks(_ text: String) -> [NativeMessageBlock] {
        var result: [NativeMessageBlock] = [], paragraph: [String] = [], code: [String] = []
        var fence: String?, language = ""
        func append(_ kind: NativeMessageBlock.Kind, _ text: String) { result.append(.init(id: result.count, kind: kind, text: text)) }
        func flush() { if !paragraph.isEmpty { append(.paragraph, paragraph.joined(separator: "\n")); paragraph = [] } }
        for line in text.components(separatedBy: "\n") {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            if let active = fence {
                if trimmed.hasPrefix(active) { append(.code(language), code.joined(separator: "\n")); code = []; fence = nil }
                else { code.append(line) }
                continue
            }
            if trimmed.hasPrefix("```") || trimmed.hasPrefix("~~~") {
                flush(); fence = String(trimmed.prefix(3)); language = String(trimmed.dropFirst(3)).trimmingCharacters(in: .whitespaces); continue
            }
            if trimmed.isEmpty { flush(); continue }
            let hashes = trimmed.prefix { $0 == "#" }.count
            if (1...6).contains(hashes), trimmed.dropFirst(hashes).hasPrefix(" ") { flush(); append(.heading(hashes), String(trimmed.dropFirst(hashes + 1))); continue }
            if trimmed.hasPrefix("- ") || trimmed.hasPrefix("* ") { flush(); append(.list, "• " + trimmed.dropFirst(2)); continue }
            // Numbered list labels are retained literally; no generated steps.
            if let dot = trimmed.firstIndex(of: "."), !trimmed[..<dot].isEmpty, trimmed[..<dot].allSatisfy(\.isNumber), trimmed[trimmed.index(after: dot)...].hasPrefix(" ") { flush(); append(.list, trimmed); continue }
            paragraph.append(line)
        }
        flush()
        if fence != nil { append(.code(language), code.joined(separator: "\n")) }
        return result
    }
}

let sample = "- Parent task\n  - Child task\n    - Grandchild task\n\n> A quoted statement\n\n| Option | Cost |\n| --- | --- |\n| Train | 12 |"
let blocks = NativeMessageContent.blocks(sample)
let rows = blocks.map { ["kind": String(describing: $0.kind), "text": $0.text] }
print(String(decoding: try JSONSerialization.data(withJSONObject: rows, options: [.prettyPrinted, .sortedKeys]), as: UTF8.self))
