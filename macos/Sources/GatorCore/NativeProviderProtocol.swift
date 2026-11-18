import Foundation

public struct NativeProviderRow: Decodable, Sendable, Equatable, Identifiable {
    public let id, model, kind, issue, destination, apiVersion: String
    public let stored, ambient, expired, configured, ready: Bool
    public let fields: [String]
    public let options: [String: String]?
    enum CodingKeys: String, CodingKey { case id, model, kind, issue, destination, stored, ambient, expired, configured, ready, fields, options; case apiVersion = "api_version" }
}
public struct NativeProviderReview: Decodable, Sendable, Equatable {
    public let id, provider, operation, model, destination, endpointMode, credentialAction: String
    public let setsDefaults: Bool
    public let options: [String: String]?
    enum CodingKeys: String, CodingKey { case id, provider, operation, model, destination, options; case endpointMode = "endpoint_mode"; case credentialAction = "credential_action"; case setsDefaults = "sets_defaults" }
}
public struct NativeProviderForm: Sendable, Equatable {
    public var model = ""
    public var endpointMode = "retain"
    public var endpoint = ""
    public var apiVersion = "v1"
    public var options: [String: String] = [:]
    public init() {}
    init(_ row: NativeProviderRow) { model = row.model; apiVersion = row.apiVersion; options = row.options ?? [:] }
    var value: JSONValue { .object(["model": .string(model), "endpoint_mode": .string(endpointMode), "endpoint": .string(endpoint), "api_version": .string(apiVersion), "options": .object(options.mapValues(JSONValue.string))]) }
}
struct NativeProviderResult: Decodable, Sendable {
    let requestID, action: String
    let provider: String?
    let rows: [NativeProviderRow]?
    let review: NativeProviderReview?
    let confirmed, changed, conflict: Bool
    let error, notice: String?
    enum CodingKeys: String, CodingKey { case action, provider, rows, review, confirmed, changed, conflict, error, notice; case requestID = "request_id" }
    init(_ value: JSONValue) throws { self = try JSONDecoder().decode(Self.self, from: JSONEncoder().encode(value)) }
}
struct NativeProviderRequest: Sendable, CustomStringConvertible {
    enum Action: String, Sendable { case status, review, review_remove, save, remove, cancel }
    let id: String
    let action: Action
    var provider = ""
    var reviewID = ""
    var form: NativeProviderForm?
    var replaceKey = false
    var apiKey = ""
    init(_ action: Action, id: String = UUID().uuidString) { self.action = action; self.id = id }
    var description: String { "NativeProviderRequest(\(action.rawValue), credential redacted)" }
    var fields: [String: JSONValue] {
        var value: [String: JSONValue] = ["request_id": .string(id), "action": .string(action.rawValue), "provider": .string(provider), "review_id": .string(reviewID), "replace_key": .bool(replaceKey)]
        if let form { value["input"] = form.value }
        if action == .save && !apiKey.isEmpty { value["api_key"] = .string(apiKey) }
        return value
    }
}
extension GatorApplicationState {
    func nativeProvider(_ request: NativeProviderRequest) async throws -> NativeProviderResult {
        guard !closing else { throw BridgeFailure("Gator is closing. No provider change was sent.") }
        let key = "native_provider_result", original = generation
        guard snapshot?.value[key]["request_id"].string != request.id else { throw BridgeFailure("Refresh and review using a fresh provider request identity.") }
        try await host(.provider(request), timeout: .seconds(20)) { $0.value[key]["request_id"].string == request.id && $0.value[key]["action"].string == request.action.rawValue }
        guard generation == original, let snapshot else { throw BridgeFailure("Provider owner changed. Refresh status; no change will be replayed.") }
        return try NativeProviderResult(snapshot.value[key])
    }
}
