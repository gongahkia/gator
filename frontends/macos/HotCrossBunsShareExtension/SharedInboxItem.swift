import Foundation

// Duplicated from HotCrossBuns/Services/SharedInbox/SharedInboxItem.swift
// so the Share Extension target can reference the same types without
// pulling in the entire main-app module. Keep the two files in sync —
// they must agree on the suite name, the key, and the SharedInboxItem
// schema (including the `source` field) to round-trip. The Share
// Extension only writes; read-side hardening lives in the main-app copy.
struct SharedInboxItem: Codable, Hashable, Sendable {
    var text: String
    var createdAt: Date
    var source: String? // bundle id of the writer; main app rejects items missing this
}

enum SharedInboxDefaults {
    static let appGroupID = "group.com.gongahkia.hotcrossbuns"
    static let trustedSourcePrefix = "com.gongahkia.hotcrossbuns"
    static let freshnessWindowSeconds: TimeInterval = 600
    static let maxTextBytes = 8 * 1024
    static let maxQueuedItems = 50
    private static let itemsKey = "sharedInbox.items"

    static var suite: UserDefaults? {
        UserDefaults(suiteName: appGroupID)
    }

    static func append(_ item: SharedInboxItem) {
        guard let suite else { return }
        guard let sanitized = sanitizedForWrite(item) else { return }
        var items = load(suite: suite)
        items = items.filter { isTrusted($0) }
        items.append(sanitized)
        if items.count > maxQueuedItems {
            items.removeFirst(items.count - maxQueuedItems)
        }
        save(items, suite: suite)
    }

    static func isTrusted(_ item: SharedInboxItem, now: Date = Date()) -> Bool {
        guard let source = item.source,
              source.hasPrefix(trustedSourcePrefix) else {
            return false
        }
        let age = now.timeIntervalSince(item.createdAt)
        if age < -60 || age > freshnessWindowSeconds { return false }
        if item.text.utf8.count > maxTextBytes { return false }
        return true
    }

    static func sanitizedForWrite(_ item: SharedInboxItem) -> SharedInboxItem? {
        guard let source = item.source,
              source.hasPrefix(trustedSourcePrefix),
              item.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false else {
            return nil
        }
        return SharedInboxItem(
            text: truncateText(item.text, maxBytes: maxTextBytes),
            createdAt: item.createdAt,
            source: source
        )
    }

    private static func load(suite: UserDefaults) -> [SharedInboxItem] {
        guard let data = suite.data(forKey: itemsKey) else { return [] }
        return (try? JSONDecoder().decode([SharedInboxItem].self, from: data)) ?? []
    }

    private static func save(_ items: [SharedInboxItem], suite: UserDefaults) {
        guard let data = try? JSONEncoder().encode(items) else { return }
        suite.set(data, forKey: itemsKey)
    }

    private static func truncateText(_ text: String, maxBytes: Int) -> String {
        guard text.utf8.count > maxBytes else { return text }
        var output = ""
        var usedBytes = 0
        for character in text {
            let characterBytes = String(character).utf8.count
            guard usedBytes + characterBytes <= maxBytes else { break }
            output.append(character)
            usedBytes += characterBytes
        }
        return output
    }
}
