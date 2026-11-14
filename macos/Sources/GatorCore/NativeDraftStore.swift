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
    var correction: NativeCorrectionDraft?
    var planFirst: Bool?
}

public struct PendingSubmission: Codable, Sendable, Equatable {
    public var id: String
    public var context: String
    public var text: String
    var draftRevision: UUID
    public var phase: SubmissionPhase
    public var planFirst: Bool?
    var correctionTarget: NativeCorrectionTarget?
}

struct NativeArchive: Codable, Sendable {
    var version = 1
    var selectedConversationID = ""
    var drafts: [String: NativeDraft] = [:]
    var pending: PendingSubmission?
    // Optional for compatibility with Tranche 01 archives. A single actor merges
    // owner-scoped writes so a late result cannot overwrite another chat's draft.
    var pendingByContext: [String: PendingSubmission]?
    var recoveryPreparationsByContext: [String: NativeRecoveryPreparation]?
    var receiptOperationsByContext: [String: NativeReceiptOperation]?
    var planOperationsByContext: [String: NativePlanOperation]?
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
        guard [1, 2, 3, 4, 5, 6].contains(archive.version), archive.drafts.count <= 128, archive.drafts.allSatisfy({ validContext($0.key) && $0.value.text.utf8.count <= 128 * 1024 && ($0.value.correction?.text.utf8.count ?? 0) <= 120 * 1024 }), archive.pending.map({ validContext($0.context) && !$0.id.isEmpty && $0.id.count <= 64 && $0.text.utf8.count <= 128 * 1024 }) ?? true else { throw BridgeFailure("Native drafts could not be read safely; the saved file was kept unchanged.") }
        if archive.pending != nil { archive.pending?.phase = .uncertain }
        if let pending = archive.pendingByContext {
            guard pending.count <= 128, pending.allSatisfy({ validContext($0.key) && $0.value.context == $0.key && !$0.value.id.isEmpty && $0.value.id.count <= 64 && $0.value.text.utf8.count <= 128 * 1024 }) else { throw BridgeFailure("Invalid native pending submissions; original file kept.") }
            archive.pendingByContext = pending.mapValues { var value = $0; value.phase = .uncertain; return value }
        }
        if let operations = archive.planOperationsByContext {
            guard operations.count <= 128, operations.allSatisfy({ validContext($0.key) && $0.key == "conversation:" + $0.value.identity.conversationID && !$0.value.id.isEmpty && $0.value.id.count <= 64 }) else { throw BridgeFailure("Invalid retained native plan operation; original file kept.") }
            archive.planOperationsByContext = operations.mapValues { original in
                var value = original
                if value.phase.awaitingResult || value.phase == .accepted || value.phase == .working || value.phase == .waitingReview { value.phase = .uncertain }
                return value
            }
        }
        if let receipts = archive.receiptOperationsByContext {
            guard receipts.count <= 128, receipts.allSatisfy({ validContext($0.key) && $0.key == "conversation:" + $0.value.identity.conversationID && !$0.value.id.isEmpty && $0.value.id.count <= 64 }) else { throw BridgeFailure("Invalid native receipt operation; original evidence kept.") }
            archive.receiptOperationsByContext = receipts.mapValues { original in var value = original; if value.unresolved { value.phase = "uncertain" }; return value }
        }
        if let preparations = archive.recoveryPreparationsByContext {
            guard preparations.count <= 128, preparations.allSatisfy({ validContext($0.key) && $0.key == "conversation:" + $0.value.conversationID && !$0.value.id.isEmpty && $0.value.id.count <= 64 && $0.value.action != .reviewAccess && ($0.value.connections?.count ?? 0) <= 16 }) else { throw BridgeFailure("Invalid retained recovery preparation; original file kept.") }
            archive.recoveryPreparationsByContext = preparations.mapValues { original in var value = original; if value.blocks { value.phase = "uncertain" }; return value }
        }
        return archive
    }
    func saveOwner(_ original: NativeArchive, context: String) throws {
        guard validContext(context) else { return }
        var merged = try load()
        // Older native bundles must fail closed instead of ignoring the new
        // per-owner pending IDs and offering a fresh submission identity.
        merged.version = max(2, merged.version)
        if let legacy = merged.pending { merged.pendingByContext = (merged.pendingByContext ?? [:]).merging([legacy.context: legacy]) { current, _ in current }; merged.pending = nil }
        if let draft = original.drafts[context] { merged.drafts[context] = draft }
        if merged.pendingByContext == nil { merged.pendingByContext = [:] }
        merged.pendingByContext?[context] = original.pending
        if merged.drafts.values.contains(where: { $0.correction != nil || $0.planFirst == true }) || merged.pendingByContext?.values.contains(where: { $0.planFirst == true }) == true { merged.version = max(3, merged.version) }
        if merged.planOperationsByContext == nil { merged.planOperationsByContext = [:] }
        merged.planOperationsByContext?[context] = original.planOperationsByContext?[context]
        if !merged.planOperationsByContext!.isEmpty { merged.version = max(4, merged.version) }
        if merged.receiptOperationsByContext == nil { merged.receiptOperationsByContext = [:] }
        merged.receiptOperationsByContext?[context] = original.receiptOperationsByContext?[context]
        if merged.recoveryPreparationsByContext == nil { merged.recoveryPreparationsByContext = [:] }
        merged.recoveryPreparationsByContext?[context] = original.recoveryPreparationsByContext?[context]
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
        if archive.planOperationsByContext?.isEmpty == false { archive.version = max(4, archive.version) }
        if archive.drafts.values.contains(where: { $0.correction != nil || $0.planFirst == true }) { archive.version = max(3, archive.version) }
        if let operations = archive.planOperationsByContext {
            // Never let terminal UI receipts grow into an unreadable archive.
            // Unconfirmed authorizations are retained, not evicted for space.
            let retained = operations.filter { $0.value.blocksAuthorization || [.accepted, .working, .waitingReview].contains($0.value.phase) }
            guard retained.count <= 128 else { throw BridgeFailure("Too many unconfirmed native plan operations. Inspect their original owners before starting more; saved data was kept.") }
            let selected = "conversation:" + archive.selectedConversationID
            let terminal = operations.filter { retained[$0.key] == nil }.sorted {
                if $0.key == selected { return true }; if $1.key == selected { return false }; return $0.key > $1.key
            }
            archive.planOperationsByContext = retained.merging(Dictionary(uniqueKeysWithValues: terminal.prefix(min(32, 128 - retained.count)).map { ($0.key, $0.value) })) { current, _ in current }
        }
        if let receipts = archive.receiptOperationsByContext, !receipts.isEmpty {
            archive.version = max(5, archive.version)
            let unresolved = receipts.filter { $0.value.unresolved }
            guard unresolved.count <= 128 else { throw BridgeFailure("Too many unconfirmed receipt checks; original archive kept.") }
            let terminal = receipts.filter { !($0.value.unresolved) }.sorted { $0.key < $1.key }
            archive.receiptOperationsByContext = unresolved.merging(Dictionary(uniqueKeysWithValues: terminal.prefix(min(32, 128 - unresolved.count)).map { ($0.key, $0.value) })) { current, _ in current }
        }
        if let preparations = archive.recoveryPreparationsByContext, !preparations.isEmpty {
            archive.version = 6
            let pending = preparations.filter { $0.value.blocks }
            guard pending.count <= 128 else { throw BridgeFailure("Too many unconfirmed recovery preparations. Original archive kept.") }
            let terminal = preparations.filter { !$0.value.blocks }.sorted { $0.key < $1.key }
            archive.recoveryPreparationsByContext = pending.merging(Dictionary(uniqueKeysWithValues: terminal.prefix(min(32, 128 - pending.count)).map { ($0.key, $0.value) })) { current, _ in current }
        }
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
