import Foundation
import CryptoKit

/// Read-only search over the public, retained projection, never a protocol action.
public struct NativeTranscriptSearch: Sendable {
    public struct Scope: Sendable, Equatable {
        public let context: String
        public let generation: UUID
        public init(context: String, generation: UUID) { self.context = context; self.generation = generation }
    }
    public struct Match: Sendable, Equatable, Identifiable {
        public struct ID: Sendable, Hashable { public let message: String; public let offset: Int }
        public let id: ID
        public let scope: Scope
        public let ordinal: Int
        public let role: String
        public let excerpt: String
        public let range: NSRange
        fileprivate let fingerprint: Data
        public var messageID: String { id.message }
    }
    public struct Results: Sendable {
        public let matches: [Match]
        public let limited: Bool
    }
    private struct Entry: Sendable {
        let id: String
        let ordinal: Int
        let role: String
        let text: String
        let fingerprint: Data
    }
    public let scope: Scope
    public let limited: Bool
    private let entries: [Entry]
    private let byID: [String: Int]
    public static let maximumQueryLength = 256
    public static let maximumMatches = 500
    public static let maximumTextBytes = 8 * 1024 * 1024
    public static let maximumMessages = 10_000

    public init(scope: Scope, messages: [BridgeMessage]) {
        self.scope = scope
        var entries: [Entry] = [], byID: [String: Int] = [:], bytes = 0, limited = messages.count > Self.maximumMessages
        for (offset, message) in messages.prefix(Self.maximumMessages).enumerated() {
            guard let text = Self.searchableText(message), !text.isEmpty, byID[message.id] == nil else { continue }
            let size = text.utf8.count
            guard size <= Self.maximumTextBytes - bytes else { limited = true; continue }
            bytes += size
            byID[message.id] = entries.count
            entries.append(Entry(id: message.id, ordinal: offset + 1, role: message.isUser ? "You" : "Gator", text: text, fingerprint: Self.fingerprint(text)))
        }
        self.entries = entries; self.byID = byID; self.limited = limited
    }

    /// Mirrors the native row's visible prose/title, excluding collapsed raw details.
    public static func searchableText(_ message: BridgeMessage) -> String? {
        guard message.hasVisibleContent, ["you", "user", "gator", "assistant"].contains(message.role.lowercased()),
              !message.value["hidden"].bool, !message.value["private"].bool,
              !["analysis", "reasoning"].contains(message.value["channel"].string.lowercased()) else { return nil }
        if message.isStructured { return message.result["title"].string.isEmpty ? "Result details" : message.result["title"].string }
        return message.text.isEmpty ? nil : message.text
    }

    public func search(_ query: String) -> Results {
        guard !query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { return Results(matches: [], limited: limited) }
        guard query.utf16.count <= Self.maximumQueryLength else { return Results(matches: [], limited: true) }
        var matches: [Match] = []
        for entry in entries {
            let source = entry.text as NSString
            var start = 0
            while start < source.length {
                let found = source.range(of: query, options: [.caseInsensitive, .diacriticInsensitive, .widthInsensitive], range: NSRange(location: start, length: source.length - start), locale: Locale(identifier: "en_US_POSIX"))
                guard found.location != NSNotFound, found.length > 0 else { break }
                let range = source.rangeOfComposedCharacterSequences(for: found)
                if matches.count == Self.maximumMatches { return Results(matches: matches, limited: true) }
                let lower = max(0, range.location - 55), upper = min(source.length, NSMaxRange(range) + 85)
                let excerptRange = source.rangeOfComposedCharacterSequences(for: NSRange(location: lower, length: upper - lower))
                let excerpt = (excerptRange.location > 0 ? "…" : "") + source.substring(with: excerptRange).replacingOccurrences(of: "\n", with: " ") + (NSMaxRange(excerptRange) < source.length ? "…" : "")
                matches.append(Match(id: .init(message: entry.id, offset: range.location), scope: scope, ordinal: entry.ordinal, role: entry.role, excerpt: excerpt, range: range, fingerprint: entry.fingerprint))
                start = NSMaxRange(range)
            }
        }
        return Results(matches: matches, limited: limited)
    }

    public func resolve(_ match: Match) -> String? {
        guard match.scope == scope, let index = byID[match.messageID], entries[index].fingerprint == match.fingerprint else { return nil }
        return entries[index].id
    }

    /// Recheck against the live owner projection at the final navigation boundary.
    public static func revealTarget(_ match: Match, scope: Scope, messages: [BridgeMessage]) -> String? {
        guard match.scope == scope, let message = messages.first(where: { $0.id == match.messageID }),
              let text = searchableText(message), text.utf8.count <= maximumTextBytes,
              fingerprint(text) == match.fingerprint else { return nil }
        return message.id
    }
    private static func fingerprint(_ text: String) -> Data { Data(SHA256.hash(data: Data(text.utf8))) }
}
