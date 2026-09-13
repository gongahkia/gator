import Foundation

// Data handed off from the macOS Share Extension (and Services menu)
// to the main app via an App Group UserDefaults suite. The main app
// reads + clears this on activation and presents the queued items in
// QuickAdd.
//
// §10 hardening: every item carries a `source` (bundle ID of the writer)
// and `createdAt` is treated as a freshness token. Any process sharing
// the same App Group entitlement can still write to the suite, but
// `consumeAll()` drops items whose source is not on the allowlist,
// whose text exceeds the size cap, or whose createdAt is outside the
// trust window. The duplicated copy in the Share Extension target must
// be kept in sync.
struct SharedInboxItem: Codable, Hashable, Sendable {
    var text: String
    var createdAt: Date
    var source: String? // bundle id of the writer; legacy items decode as nil
}

enum SharedInboxDefaults {
    // Both the main app and the Share Extension must declare this App
    // Group id in their entitlements for the suite to be accessible.
    static let appGroupID = "group.com.gongahkia.hotcrossbuns"
    // Only accept items written by a bundle whose id starts with this
    // prefix. A malicious process with the same App Group entitlement
    // can still spoof this string, but requires deliberate packaging
    // rather than accidentally land payloads via an unrelated entitlement.
    static let trustedSourcePrefix = "com.gongahkia.hotcrossbuns"
    // Reject items older than this when consuming. Share → open app
    // typically takes a few seconds at most; 10 minutes covers slow
    // restarts without leaving stale attacker-planted payloads readable.
    static let freshnessWindowSeconds: TimeInterval = 600
    // Hard ceiling on the text blob so a malicious writer can't OOM the
    // main app or stuff a multi-megabyte payload into QuickAdd.
    static let maxTextBytes = 8 * 1024
    // Keep the App Group suite bounded even if many shares happen before
    // the main app is opened to consume them.
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

    // §10 — applies source-allowlist, freshness, and size checks before
    // returning. Untrusted items are silently dropped (not surfaced as
    // errors) so a background attacker can't use the inbox as a signal
    // channel into the UI.
    static func consumeAll(now: Date = Date()) -> [SharedInboxItem] {
        guard let suite else { return [] }
        let raw = load(suite: suite)
        guard raw.isEmpty == false else { return [] }
        // Always clear storage, even for items we reject — otherwise a
        // permanently-untrusted payload would sit in the suite forever.
        suite.removeObject(forKey: itemsKey)
        return raw.filter { isTrusted($0, now: now) }
    }

    static func isTrusted(_ item: SharedInboxItem, now: Date = Date()) -> Bool {
        if let source = item.source {
            guard source.hasPrefix(trustedSourcePrefix) else { return false }
        }
        // Legacy items (nil source) are rejected — any pre-hardening
        // payload sitting in the suite after upgrade must be re-shared.
        else {
            return false
        }
        let age = now.timeIntervalSince(item.createdAt)
        if age < -60 || age > freshnessWindowSeconds { return false } // clock-skew tolerance
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
