import Foundation

struct NativeWorkspaceRequest: Sendable {
    let id: String
    let path: String
    init(path: String, id: String = UUID().uuidString) { self.path = path; self.id = id }
    var fields: [String: JSONValue] { ["request_id": .string(id), "path": .string(path)] }
}

struct NativeWorkspaceResult: Decodable, Sendable {
    let requestID, conversationID: String
    let source: String?
    let new: Bool
    let error: String?
    enum CodingKeys: String, CodingKey { case source, new, error; case requestID = "request_id"; case conversationID = "conversation_id" }
    init(_ value: JSONValue) throws { self = try JSONDecoder().decode(Self.self, from: JSONEncoder().encode(value)) }
}

extension GatorApplicationState {
    func resolveWorkspace(_ request: NativeWorkspaceRequest) async throws -> NativeWorkspaceResult {
        guard !closing, snapshot?.value["native_workspace_result"]["request_id"].string != request.id else { throw BridgeFailure("Choose the workspace again using a fresh request.") }
        let originalGeneration = generation, originalID = conversationID, originalSource = snapshot?.value["source"].string ?? ""
        try await host(.workspace(request), timeout: .seconds(12)) { $0.value["native_workspace_result"]["request_id"].string == request.id }
        guard generation == originalGeneration, let snapshot else { throw BridgeFailure("The workspace owner changed. Original workspace kept.") }
        let result = try NativeWorkspaceResult(snapshot.value["native_workspace_result"])
        guard result.requestID == request.id, result.conversationID == originalID else { throw BridgeFailure("The workspace result belongs to another conversation. Original workspace kept.") }
        if let error = result.error, !error.isEmpty { throw BridgeFailure(error) }
        guard let source = result.source, source.hasPrefix("/"), source.utf8.count <= 8192, !source.unicodeScalars.contains(where: { CharacterSet.controlCharacters.contains($0) }) else { throw BridgeFailure("Go did not confirm a supported canonical workspace path.") }
        // Go retains the current source spelling for the same directory. Compare
        // wire bytes here, since Swift equality folds Unicode normalization.
        guard result.new == !source.utf8.elementsEqual(originalSource.utf8) else { throw BridgeFailure("The workspace transition does not match the original source. Choose again.") }
        return result
    }
}
