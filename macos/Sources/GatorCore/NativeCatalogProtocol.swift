import Foundation

public enum NativeCatalogScope: String, Sendable, CaseIterable, Identifiable {
    case active, archived, all
    public var id: String { rawValue }
    public var label: String { self == .all ? "All conversations" : rawValue.capitalized }
}
public struct NativeCatalogItem: Decodable, Sendable, Equatable, Identifiable {
    public let id, title, source, version: String
    public let updatedAt: String
    public let pinned, archived: Bool
    public let snippet: String?
    enum CodingKeys: String, CodingKey { case id, title, source, version, pinned, archived, snippet; case updatedAt = "updated_at" }
    public var date: Date? {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let date = formatter.date(from: updatedAt) { return date }
        formatter.formatOptions = [.withInternetDateTime]
        return formatter.date(from: updatedAt)
    }
}
public struct NativeCatalogPage: Decodable, Sendable, Equatable {
    public let items: [NativeCatalogItem]
    public let next: String?
    public let incomplete: Bool
    public let skipped: Int
    public let indexFallback: Bool
    enum CodingKeys: String, CodingKey { case items, next, incomplete, skipped; case indexFallback = "index_fallback" }
    func replacing(items: [NativeCatalogItem]) -> Self { Self(items: items, next: next, incomplete: incomplete, skipped: skipped, indexFallback: indexFallback) }
    private init(items: [NativeCatalogItem], next: String?, incomplete: Bool, skipped: Int, indexFallback: Bool) { self.items = items; self.next = next; self.incomplete = incomplete; self.skipped = skipped; self.indexFallback = indexFallback }
    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        items = try c.decodeIfPresent([NativeCatalogItem].self, forKey: .items) ?? []
        next = try c.decodeIfPresent(String.self, forKey: .next)
        incomplete = try c.decodeIfPresent(Bool.self, forKey: .incomplete) ?? false
        skipped = try c.decodeIfPresent(Int.self, forKey: .skipped) ?? 0
        indexFallback = try c.decodeIfPresent(Bool.self, forKey: .indexFallback) ?? false
        guard items.count <= 50, Set(items.map(\.id)).count == items.count,
              items.allSatisfy({ validConversationID($0.id) && !$0.version.isEmpty }), (next?.utf8.count ?? 0) <= 1024 else { throw BridgeFailure("Invalid catalog page. Refresh before selecting a conversation.") }
    }
    public var coverageNotice: String {
        var notices: [String] = []
        if incomplete || skipped > 0 { notices.append("Search coverage is incomplete. Some records changed, could not be read, or exceed the bounded index. Refresh to continue indexing.") }
        if indexFallback { notices.append("The chat index is unavailable. Showing results from retained conversation files.") }
        return notices.joined(separator: " ")
    }
}
struct NativeCatalogResult: Decodable, Sendable {
    let requestID, action: String
    let pageID: String?
    let page: NativeCatalogPage?
    let item: NativeCatalogItem?
    let error: String?
    let conflict: Bool?
    enum CodingKeys: String, CodingKey { case action, page, item, error, conflict; case requestID = "request_id"; case pageID = "page_id" }
    init(_ value: JSONValue) throws { self = try JSONDecoder().decode(Self.self, from: JSONEncoder().encode(value)) }
}
struct NativeCatalogRequest: Sendable {
    enum Action: String, Sendable { case query, cancel, open, inspect, rename, verify }
    let id: String
    let action: Action
    var text = ""
    var scope = NativeCatalogScope.active
    var after = ""
    var pageID = ""
    var item: NativeCatalogItem?
    var title = ""
    var cancelID = ""
    init(_ action: Action, id: String = UUID().uuidString) { self.id = id; self.action = action }
    var fields: [String: JSONValue] {
        var result: [String: JSONValue] = ["request_id": .string(id), "action": .string(action.rawValue)]
        if action == .query { result["text"] = .string(text); result["scope"] = .string(scope.rawValue); result["after"] = .string(after) }
        if let item { result["id"] = .string(item.id); result["version"] = .string(item.version); result["page_id"] = .string(pageID) }
        if action == .rename { result["title"] = .string(title) }
        if action == .cancel { result["cancel_id"] = .string(cancelID) }
        return result
    }
}

extension GatorApplicationState {
    func nativeCatalog(_ request: NativeCatalogRequest) async throws -> NativeCatalogResult {
        guard !closing else { throw BridgeFailure("Gator is closing. No catalog operation was sent.") }
        if request.action == .cancel {
            try await host(.catalog(request)); return try NativeCatalogResult(.object(["request_id": .string(request.id), "action": .string("cancel")]))
        }
        let key = request.action == .query ? "native_catalog_query_result" : "native_catalog_action_result"
        let originalGeneration = generation
        try await host(.catalog(request), timeout: .seconds(12)) { $0.value[key]["request_id"].string == request.id && $0.value[key]["action"].string == request.action.rawValue }
        guard generation == originalGeneration, let snapshot else { throw BridgeFailure("The catalog owner changed. Refresh and review before trying again.") }
        return try NativeCatalogResult(snapshot.value[key])
    }
}
