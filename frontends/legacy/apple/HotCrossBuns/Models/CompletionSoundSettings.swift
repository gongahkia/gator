import Foundation

struct CompletionSoundAsset: Identifiable, Hashable, Codable, Sendable {
    let id: UUID
    var displayName: String
    var storedFilename: String
    var importedAt: Date
}

struct CompletionSoundChoice: Hashable, Codable, Sendable {
    enum Source: String, Hashable, Codable, Sendable {
        case system
        case custom
    }

    var source: Source
    var identifier: String

    static let defaultTask = CompletionSoundChoice(source: .system, identifier: "Glass")
    static let defaultEvent = CompletionSoundChoice(source: .system, identifier: "Pop")

    static func system(_ name: String) -> CompletionSoundChoice {
        CompletionSoundChoice(source: .system, identifier: name)
    }

    static func custom(_ assetID: UUID) -> CompletionSoundChoice {
        CompletionSoundChoice(source: .custom, identifier: assetID.uuidString)
    }

    var customAssetID: UUID? {
        guard source == .custom else { return nil }
        return UUID(uuidString: identifier)
    }
}
