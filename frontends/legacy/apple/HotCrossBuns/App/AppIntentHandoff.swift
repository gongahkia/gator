import Foundation

enum AppIntentRoute: String, Sendable {
    case addTask
    case addEvent
    case store
    case calendar
}

// Array-backed queue instead of a scalar key so rapid-fire intents
// (e.g. two dock-menu clicks before the app foregrounds) don't
// overwrite each other. `consumeAll` drains in the order the intents
// fired so the main app can route them one by one.
enum AppIntentHandoff {
    private static let pendingRoutesKey = "appIntent.pendingRoutes"
    // Legacy scalar key retained for one migration pass; see consumeAll.
    private static let legacyRouteKey = "appIntent.pendingRoute"

    static func save(_ route: AppIntentRoute) {
        var existing = UserDefaults.standard.stringArray(forKey: pendingRoutesKey) ?? []
        existing.append(route.rawValue)
        UserDefaults.standard.set(existing, forKey: pendingRoutesKey)
    }

    static var hasPendingRoutes: Bool {
        if let queued = UserDefaults.standard.stringArray(forKey: pendingRoutesKey),
           queued.isEmpty == false {
            return true
        }
        return UserDefaults.standard.string(forKey: legacyRouteKey) != nil
    }

    static func consumeAll() -> [AppIntentRoute] {
        var rawValues: [String] = []
        if let queued = UserDefaults.standard.stringArray(forKey: pendingRoutesKey) {
            rawValues.append(contentsOf: queued)
            UserDefaults.standard.removeObject(forKey: pendingRoutesKey)
        }
        // One-shot migration: if the old scalar key still has a value
        // (app just updated from a pre-queue build), drain it too.
        if let legacy = UserDefaults.standard.string(forKey: legacyRouteKey) {
            rawValues.append(legacy)
            UserDefaults.standard.removeObject(forKey: legacyRouteKey)
        }
        return rawValues.compactMap(AppIntentRoute.init(rawValue:))
    }

    // Kept for any caller that only wants the first pending route; now
    // just returns the front of the queue and leaves the rest alone for
    // the next call. Prefer consumeAll for new callers.
    static func consumePendingRoute() -> AppIntentRoute? {
        let all = consumeAll()
        guard let first = all.first else { return nil }
        // Re-queue the leftovers so a caller that only wants one doesn't
        // silently discard the rest.
        let remainder = Array(all.dropFirst())
        if remainder.isEmpty == false {
            UserDefaults.standard.set(remainder.map(\.rawValue), forKey: pendingRoutesKey)
        }
        return first
    }
}
