import Foundation
import CryptoKit

public enum SubmissionPhase: String, Codable, Sendable {
    case submitted, acknowledged, accepted, running, completed, failed, stopped, uncertain
    var awaitingAcceptance: Bool { self == .submitted || self == .acknowledged || self == .uncertain }
}

struct NativeDraft: Codable, Sendable, Equatable {
    var text = ""
    var revision = UUID()
    var updated = Date()
}

public struct PendingSubmission: Codable, Sendable, Equatable {
    public var id: String
    public var context: String
    public var text: String
    var draftRevision: UUID
    public var phase: SubmissionPhase
}

struct NativeArchive: Codable, Sendable {
    var version = 1
    var selectedConversationID = ""
    var drafts: [String: NativeDraft] = [:]
    var pending: PendingSubmission?
}

/// Native-only namespace. Never opens or migrates the TUI's drafts/ directory.
actor NativeDraftStore {
    let file: URL
    init(directory: URL, profile: String) {
        let digest = SHA256.hash(data: Data(profile.utf8)).map { String(format: "%02x", $0) }.joined()
        file = directory.appendingPathComponent(digest + ".json")
    }
    func load() throws -> NativeArchive {
        guard FileManager.default.fileExists(atPath: file.path) else { return NativeArchive() }
        let data = try Data(contentsOf: file)
        guard data.count <= 6 * 1024 * 1024 else { throw BridgeFailure("Native drafts are oversized; the saved file was kept unchanged.") }
        var archive = try JSONDecoder().decode(NativeArchive.self, from: data)
        guard archive.version == 1, archive.drafts.count <= 128, archive.drafts.allSatisfy({ validContext($0.key) && $0.value.text.utf8.count <= 128 * 1024 }), archive.pending.map({ validContext($0.context) && !$0.id.isEmpty && $0.id.count <= 64 && $0.text.utf8.count <= 128 * 1024 }) ?? true else { throw BridgeFailure("Native drafts could not be read safely; the saved file was kept unchanged.") }
        if archive.pending != nil { archive.pending?.phase = .uncertain }
        return archive
    }
    private func validContext(_ context: String) -> Bool {
        (context.hasPrefix("conversation:") || context.hasPrefix("workspace:")) && context.utf8.count <= 8192
    }
    func save(_ original: NativeArchive) throws {
        var archive = original
        let cutoff = Date().addingTimeInterval(-7 * 86400)
        let keep = archive.drafts.filter { $0.value.updated >= cutoff || $0.key == archive.pending?.context }
            .sorted { $0.value.updated > $1.value.updated }
        archive.drafts = Dictionary(uniqueKeysWithValues: keep.prefix(32).map { ($0.key, $0.value) })
        if let pending = archive.pending, let draft = original.drafts[pending.context] { archive.drafts[pending.context] = draft }
        let directory = file.deletingLastPathComponent()
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let temporary = directory.appendingPathComponent(".\(UUID().uuidString).tmp")
        let bytes = try JSONEncoder().encode(archive)
        guard bytes.count <= 6 * 1024 * 1024 else { throw BridgeFailure("Native drafts exceed the storage limit.") }
        let descriptor = open(temporary.path, O_CREAT | O_EXCL | O_WRONLY, 0o600)
        guard descriptor >= 0 else { throw CocoaError(.fileWriteUnknown) }
        let handle = FileHandle(fileDescriptor: descriptor, closeOnDealloc: true)
        defer { try? handle.close(); try? FileManager.default.removeItem(at: temporary) }
        try handle.write(contentsOf: bytes); try handle.synchronize(); try handle.close()
        guard rename(temporary.path, file.path) == 0 else { throw CocoaError(.fileWriteUnknown) }
    }
}
