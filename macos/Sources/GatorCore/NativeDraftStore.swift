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
    // Optional for compatibility with Tranche 01 archives. A single actor merges
    // owner-scoped writes so a late result cannot overwrite another chat's draft.
    var pendingByContext: [String: PendingSubmission]?
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
        guard [1, 2].contains(archive.version), archive.drafts.count <= 128, archive.drafts.allSatisfy({ validContext($0.key) && $0.value.text.utf8.count <= 128 * 1024 }), archive.pending.map({ validContext($0.context) && !$0.id.isEmpty && $0.id.count <= 64 && $0.text.utf8.count <= 128 * 1024 }) ?? true else { throw BridgeFailure("Native drafts could not be read safely; the saved file was kept unchanged.") }
        if archive.pending != nil { archive.pending?.phase = .uncertain }
        if let pending = archive.pendingByContext {
            guard pending.count <= 128, pending.allSatisfy({ validContext($0.key) && $0.value.context == $0.key && !$0.value.id.isEmpty && $0.value.id.count <= 64 && $0.value.text.utf8.count <= 128 * 1024 }) else { throw BridgeFailure("Invalid native pending submissions; original file kept.") }
            archive.pendingByContext = pending.mapValues { var value = $0; value.phase = .uncertain; return value }
        }
        return archive
    }
    func saveOwner(_ original: NativeArchive, context: String) throws {
        guard validContext(context) else { return }
        var merged = try load()
        // Older native bundles must fail closed instead of ignoring the new
        // per-owner pending IDs and offering a fresh submission identity.
        merged.version = 2
        if let legacy = merged.pending { merged.pendingByContext = (merged.pendingByContext ?? [:]).merging([legacy.context: legacy]) { current, _ in current }; merged.pending = nil }
        if let draft = original.drafts[context] { merged.drafts[context] = draft }
        if merged.pendingByContext == nil { merged.pendingByContext = [:] }
        merged.pendingByContext?[context] = original.pending
        try save(merged)
    }
    func selectConversation(_ id: String) throws {
        var merged = try load(); merged.selectedConversationID = id; try save(merged)
    }
    private func validContext(_ context: String) -> Bool {
        (context.hasPrefix("conversation:") || context.hasPrefix("workspace:")) && context.utf8.count <= 8192
    }
    func save(_ original: NativeArchive) throws {
        var archive = original
        let cutoff = Date().addingTimeInterval(-7 * 86400)
        let keep = archive.drafts.filter { $0.value.updated >= cutoff || $0.key == archive.pending?.context || archive.pendingByContext?[$0.key] != nil }
            .sorted { $0.value.updated > $1.value.updated }
        archive.drafts = Dictionary(uniqueKeysWithValues: keep.prefix(32).map { ($0.key, $0.value) })
        if let pending = archive.pending, let draft = original.drafts[pending.context] { archive.drafts[pending.context] = draft }
        for context in archive.pendingByContext?.keys ?? Dictionary<String, PendingSubmission>().keys {
            if let draft = original.drafts[context] { archive.drafts[context] = draft }
        }
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
