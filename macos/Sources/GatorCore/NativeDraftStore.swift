import Foundation
import CryptoKit
import Darwin

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
    var uploads: [NativeUpload]?
    var replyToUpdateID: String?
}

public struct PendingSubmission: Codable, Sendable, Equatable {
    public var id: String
    public var context: String
    public var text: String
    var draftRevision: UUID
    public var phase: SubmissionPhase
    public var planFirst: Bool?
    public var uploads: [NativeUpload]?
    public var replyToUpdateID: String?
    public var ownerGeneration: UUID?
    public var ownerConversationID: String?
    var correctionTarget: NativeCorrectionTarget?
}

struct NativeArchive: Codable, Sendable {
    var version = 1
    var selectedConversationID = ""
    var drafts: [String: NativeDraft] = [:]
    var pending: PendingSubmission?
    // optional for compatibility with Tranche 01 archives.
    var pendingByContext: [String: PendingSubmission]?
    var recoveryPreparationsByContext: [String: NativeRecoveryPreparation]?
    var receiptOperationsByContext: [String: NativeReceiptOperation]?
    var planOperationsByContext: [String: NativePlanOperation]?
    // read expectations travel with each snapshot, never into the persisted schema.
    var baseline = NativeArchiveBaseline()
    private enum CodingKeys: String, CodingKey {
        case version, selectedConversationID, drafts, pending, pendingByContext
        case recoveryPreparationsByContext, receiptOperationsByContext, planOperationsByContext
    }
}

struct NativeArchiveBaseline: Sendable {
    var contexts: [String: Data] = [:]
    var selection: String?
    mutating func apply(_ receipt: NativeArchiveWriteReceipt) {
        for context in receipt.contexts { contexts[context] = receipt.baseline.contexts[context] }
        if receipt.selected { selection = receipt.baseline.selection }
    }
}
struct NativeArchiveWriteReceipt: Sendable {
    let baseline: NativeArchiveBaseline
    let contexts: Set<String>
    let selected: Bool
}
private struct NativeOwnerArchive: Codable {
    var draft: NativeDraft?
    var pending: PendingSubmission?
    var plan: NativePlanOperation?
    var receipt: NativeReceiptOperation?
    var recovery: NativeRecoveryPreparation?
    var empty: Bool { draft == nil && pending == nil && plan == nil && receipt == nil && recovery == nil }
}
enum NativeArchiveWriteStage: Sendable { case beforeWrite, beforeReplace, afterReplace }


/// Native-only namespace. Never opens or migrates the TUI's drafts/ directory.
actor NativeDraftStore {
    let file: URL
    private let writeHook: (@Sendable (NativeArchiveWriteStage) throws -> Void)?
    init(directory: URL, profile: String, writeHook: (@Sendable (NativeArchiveWriteStage) throws -> Void)? = nil) {
        self.writeHook = writeHook
        let digest = SHA256.hash(data: Data(profile.utf8)).map { String(format: "%02x", $0) }.joined()
        file = directory.appendingPathComponent(digest + ".json")
    }
    func load() async throws -> NativeArchive {
        try await locked {
            var archive = try readArchive()
            archive.baseline = try baseline(archive)
            return restored(archive)
        }
    }
    private func readArchive() throws -> NativeArchive {
        guard FileManager.default.fileExists(atPath: file.path) else { return NativeArchive() }
        let fd = open(file.path, O_RDONLY | O_NOFOLLOW | O_NONBLOCK | O_CLOEXEC)
        guard fd >= 0 else { throw CocoaError(.fileReadUnknown) }; defer { close(fd) }
        var info = stat()
        guard fstat(fd, &info) == 0, info.st_mode & S_IFMT == S_IFREG, info.st_size <= 6 * 1024 * 1024 else { throw BridgeFailure("Native drafts are oversized or not an ordinary file; the saved file was kept unchanged.") }
        var data = Data(), buffer = [UInt8](repeating: 0, count: 64 * 1024)
        while data.count <= 6 * 1024 * 1024 {
            let n = Darwin.read(fd, &buffer, min(buffer.count, 6 * 1024 * 1024 + 1 - data.count))
            if n < 0 && errno == EINTR { continue }
            guard n >= 0 else { throw CocoaError(.fileReadUnknown) }
            if n == 0 { break }; data.append(contentsOf: buffer.prefix(n))
        }
        guard data.count <= 6 * 1024 * 1024 else { throw BridgeFailure("Native drafts are oversized; the saved file was kept unchanged.") }
        return try decode(data)
    }
    private func decode(_ data: Data) throws -> NativeArchive {
        let archive = try JSONDecoder().decode(NativeArchive.self, from: data)
        guard [1, 2, 3, 4, 5, 6, 7].contains(archive.version), archive.drafts.count <= 128, archive.drafts.allSatisfy({ validContext($0.key) && $0.value.text.utf8.count <= 128 * 1024 && ($0.value.correction?.text.utf8.count ?? 0) <= 120 * 1024 }), archive.pending.map({ validContext($0.context) && !$0.id.isEmpty && $0.id.count <= 64 && $0.text.utf8.count <= 128 * 1024 }) ?? true else { throw BridgeFailure("Native drafts could not be read safely; the saved file was kept unchanged.") }
        guard archive.drafts.values.allSatisfy({ NativeUploadLimits.validSelection($0.uploads ?? []) && validReply($0.replyToUpdateID) }), archive.pending.map({ NativeUploadLimits.validSelection($0.uploads ?? []) && validReply($0.replyToUpdateID) }) ?? true, archive.pendingByContext?.values.allSatisfy({ NativeUploadLimits.validSelection($0.uploads ?? []) && validReply($0.replyToUpdateID) }) ?? true else { throw BridgeFailure("Saved attachment drafts are invalid. The original archive was kept.") }
        if let pending = archive.pendingByContext {
            guard pending.count <= 128, pending.allSatisfy({ validContext($0.key) && $0.value.context == $0.key && !$0.value.id.isEmpty && $0.value.id.count <= 64 && $0.value.text.utf8.count <= 128 * 1024 }) else { throw BridgeFailure("Invalid native pending submissions; original file kept.") }
        }
        if let operations = archive.planOperationsByContext {
            guard operations.count <= 128, operations.allSatisfy({ validContext($0.key) && $0.key == "conversation:" + $0.value.identity.conversationID && !$0.value.id.isEmpty && $0.value.id.count <= 64 }) else { throw BridgeFailure("Invalid retained native plan operation; original file kept.") }

        }
        if let receipts = archive.receiptOperationsByContext {
            guard receipts.count <= 128, receipts.allSatisfy({ validContext($0.key) && $0.key == "conversation:" + $0.value.identity.conversationID && !$0.value.id.isEmpty && $0.value.id.count <= 64 }) else { throw BridgeFailure("Invalid native receipt operation; original evidence kept.") }
        }
        if let preparations = archive.recoveryPreparationsByContext {
            guard preparations.count <= 128, preparations.allSatisfy({ validContext($0.key) && $0.key == "conversation:" + $0.value.conversationID && !$0.value.id.isEmpty && $0.value.id.count <= 64 && $0.value.action != .reviewAccess && ($0.value.connections?.count ?? 0) <= 16 }) else { throw BridgeFailure("Invalid retained recovery preparation; original file kept.") }
        }
        return archive
    }
    private func restored(_ original: NativeArchive) -> NativeArchive {
        var archive = original
        if archive.pending != nil { archive.pending?.phase = .uncertain }
        archive.pendingByContext = archive.pendingByContext?.mapValues { var value = $0; value.phase = .uncertain; return value }
        archive.planOperationsByContext = archive.planOperationsByContext?.mapValues { original in
            var value = original
            if value.phase.awaitingResult || [.accepted, .working, .waitingReview].contains(value.phase) { value.phase = .uncertain }
            return value
        }
        archive.receiptOperationsByContext = archive.receiptOperationsByContext?.mapValues { original in var value = original; if value.unresolved { value.phase = "uncertain" }; return value }
        archive.recoveryPreparationsByContext = archive.recoveryPreparationsByContext?.mapValues { original in var value = original; if value.blocks { value.phase = "uncertain" }; return value }
        return archive
    }
    @discardableResult func saveOwner(_ original: NativeArchive, context: String) async throws -> NativeArchiveWriteReceipt {
        try await saveOwners(original, contexts: [context])
    }
    @discardableResult func saveOwners(_ original: NativeArchive, contexts: Set<String>) async throws -> NativeArchiveWriteReceipt {
        try await commit(original, contexts: contexts, selected: false, ownerWrite: true)
    }
    func selectConversation(_ id: String) async throws {
        try await locked {
            var archive = try readArchive()
            archive.selectedConversationID = id
            _ = try publish(archive)
        }
    }
    @discardableResult func save(_ original: NativeArchive) async throws -> NativeArchiveWriteReceipt {
        let changed = try Set(contexts(original).union(original.baseline.contexts.keys).filter {
            try fingerprint(record(original, context: $0)) != original.baseline.contexts[$0]
        })
        return try await commit(original, contexts: changed, selected: original.selectedConversationID != (original.baseline.selection ?? ""), ownerWrite: false)
    }
    private func commit(_ original: NativeArchive, contexts: Set<String>, selected: Bool, ownerWrite: Bool) async throws -> NativeArchiveWriteReceipt {
        guard (1...7).contains(original.version), contexts.allSatisfy(validContext) else { throw BridgeFailure("Invalid native archive transaction. Original file kept.") }
        return try await locked {
            var current = try readArchive()
            for context in contexts {
                let desired = record(original, context: context, ownerWrite: ownerWrite)
                let actual = try fingerprint(record(current, context: context)), next = try fingerprint(desired)
                guard actual == original.baseline.contexts[context] || actual == next else {
                    throw BridgeFailure("Another native writer changed this conversation's saved draft or operation. Your input is kept in memory; reopen and reconcile the original owner before saving over it.")
                }
                replace(desired, in: &current, context: context, legacy: !ownerWrite && original.pending?.context == context)
            }
            if selected {
                guard current.selectedConversationID == (original.baseline.selection ?? "") || current.selectedConversationID == original.selectedConversationID else { throw BridgeFailure("Another native writer changed the selected conversation. Saved data was kept.") }
                current.selectedConversationID = original.selectedConversationID
            }
            current.version = max(current.version, original.version, ownerWrite ? 2 : 1)
            let saved = try publish(current)
            return NativeArchiveWriteReceipt(baseline: try baseline(saved), contexts: contexts, selected: selected)
        }
    }
    private func contexts(_ archive: NativeArchive) -> Set<String> {
        var keys = Set(archive.drafts.keys)
        keys.formUnion(archive.pendingByContext?.keys ?? [:].keys)
        keys.formUnion(archive.planOperationsByContext?.keys ?? [:].keys)
        keys.formUnion(archive.receiptOperationsByContext?.keys ?? [:].keys)
        keys.formUnion(archive.recoveryPreparationsByContext?.keys ?? [:].keys)
        if let pending = archive.pending { keys.insert(pending.context) }
        return keys
    }
    private func record(_ archive: NativeArchive, context: String, ownerWrite: Bool = false) -> NativeOwnerArchive {
        let pending = archive.pending?.context == context ? archive.pending : ownerWrite ? nil : archive.pendingByContext?[context]
        return NativeOwnerArchive(draft: archive.drafts[context], pending: pending, plan: archive.planOperationsByContext?[context], receipt: archive.receiptOperationsByContext?[context], recovery: archive.recoveryPreparationsByContext?[context])
    }
    private func fingerprint(_ record: NativeOwnerArchive) throws -> Data? {
        guard !record.empty else { return nil }
        let encoder = JSONEncoder(); encoder.outputFormatting = [.sortedKeys]
        return Data(SHA256.hash(data: try encoder.encode(record)))
    }
    private func baseline(_ archive: NativeArchive) throws -> NativeArchiveBaseline {
        var result = NativeArchiveBaseline(selection: archive.selectedConversationID)
        for context in contexts(archive) { result.contexts[context] = try fingerprint(record(archive, context: context)) }
        return result
    }
    private func replace(_ record: NativeOwnerArchive, in archive: inout NativeArchive, context: String, legacy: Bool) {
        archive.drafts[context] = record.draft
        if archive.pending?.context == context { archive.pending = nil }
        if legacy, let pending = record.pending, archive.pending == nil {
            archive.pending = pending; archive.pendingByContext?[context] = nil
        } else if record.pending != nil || archive.pendingByContext != nil {
            if archive.pendingByContext == nil { archive.pendingByContext = [:] }
            archive.pendingByContext?[context] = record.pending
        }
        if record.plan != nil || archive.planOperationsByContext != nil {
            if archive.planOperationsByContext == nil { archive.planOperationsByContext = [:] }
            archive.planOperationsByContext?[context] = record.plan
        }
        if record.receipt != nil || archive.receiptOperationsByContext != nil {
            if archive.receiptOperationsByContext == nil { archive.receiptOperationsByContext = [:] }
            archive.receiptOperationsByContext?[context] = record.receipt
        }
        if record.recovery != nil || archive.recoveryPreparationsByContext != nil {
            if archive.recoveryPreparationsByContext == nil { archive.recoveryPreparationsByContext = [:] }
            archive.recoveryPreparationsByContext?[context] = record.recovery
        }
    }
    private func locked<T>(_ transaction: () throws -> T) async throws -> T {
        let directory = file.deletingLastPathComponent()
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        // never lock the replaceable archive inode or unlink this stable lock.
        let fd = open(file.path + ".lock", O_CREAT | O_RDWR | O_NOFOLLOW | O_CLOEXEC, 0o600)
        guard fd >= 0 else { throw CocoaError(.fileWriteUnknown) }; defer { close(fd) }
        var info = stat()
        guard fstat(fd, &info) == 0, info.st_mode & S_IFMT == S_IFREG, info.st_uid == geteuid(), fchmod(fd, 0o600) == 0 else { throw BridgeFailure("Private native archive lock is unavailable.") }
        let deadline = ContinuousClock.now + .seconds(5)
        while flock(fd, LOCK_EX | LOCK_NB) != 0 {
            guard errno == EWOULDBLOCK || errno == EINTR else { throw CocoaError(.fileWriteUnknown) }
            guard ContinuousClock.now < deadline else { throw BridgeFailure("Another native writer is holding draft storage. Your input is kept; try saving again after it finishes.") }
            try await Task.sleep(for: .milliseconds(10))
        }
        defer { _ = flock(fd, LOCK_UN) }
        try Task.checkCancellation()
        return try transaction()
    }
    private func validReply(_ id: String?) -> Bool { id == nil || id!.range(of: "^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$", options: .regularExpression) != nil }
    private func validContext(_ context: String) -> Bool {
        (context.hasPrefix("conversation:") || context.hasPrefix("workspace:")) && context.utf8.count <= 8192
    }
    private func publish(_ original: NativeArchive) throws -> NativeArchive {
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
            archive.version = max(6, archive.version)
            let pending = preparations.filter { $0.value.blocks }
            guard pending.count <= 128 else { throw BridgeFailure("Too many unconfirmed recovery preparations. Original archive kept.") }
            let terminal = preparations.filter { !$0.value.blocks }.sorted { $0.key < $1.key }
            archive.recoveryPreparationsByContext = pending.merging(Dictionary(uniqueKeysWithValues: terminal.prefix(min(32, 128 - pending.count)).map { ($0.key, $0.value) })) { current, _ in current }
        }
        if archive.drafts.values.contains(where: { !($0.uploads ?? []).isEmpty || $0.replyToUpdateID != nil }) || !(archive.pending?.uploads ?? []).isEmpty || archive.pendingByContext?.values.contains(where: { !($0.uploads ?? []).isEmpty || $0.replyToUpdateID != nil || $0.ownerConversationID != nil }) == true { archive.version = 7 }
        let cutoff = Date().addingTimeInterval(-7 * 86400)
        let pinned = archive.drafts.filter { !($0.value.uploads ?? []).isEmpty || $0.key == archive.pending?.context || archive.pendingByContext?[$0.key] != nil }
        guard pinned.count <= 128 else { throw BridgeFailure("Too many retained attachment drafts. Original archive kept.") }
        let recent = archive.drafts.filter { pinned[$0.key] == nil && $0.value.updated >= cutoff }.sorted { $0.value.updated > $1.value.updated }
        archive.drafts = pinned.merging(Dictionary(uniqueKeysWithValues: recent.prefix(min(32, 128 - pinned.count)).map { ($0.key, $0.value) })) { current, _ in current }
        if let pending = archive.pending, let draft = original.drafts[pending.context] { archive.drafts[pending.context] = draft }
        for context in archive.pendingByContext?.keys ?? Dictionary<String, PendingSubmission>().keys {
            if let draft = original.drafts[context] { archive.drafts[context] = draft }
        }
        try writeHook?(.beforeWrite)
        let directory = file.deletingLastPathComponent()
        let temporary = directory.appendingPathComponent(".\(UUID().uuidString).tmp")
        let bytes = try JSONEncoder().encode(archive)
        guard bytes.count <= 6 * 1024 * 1024 else { throw BridgeFailure("Native drafts exceed the storage limit.") }
        _ = try decode(bytes)
        let descriptor = open(temporary.path, O_CREAT | O_EXCL | O_WRONLY | O_NOFOLLOW | O_CLOEXEC, 0o600)
        guard descriptor >= 0 else { throw CocoaError(.fileWriteUnknown) }
        let handle = FileHandle(fileDescriptor: descriptor, closeOnDealloc: true)
        defer { try? handle.close(); try? FileManager.default.removeItem(at: temporary) }
        try handle.write(contentsOf: bytes); try handle.synchronize(); try handle.close()
        try writeHook?(.beforeReplace)
        guard rename(temporary.path, file.path) == 0 else { throw CocoaError(.fileWriteUnknown) }
        try writeHook?(.afterReplace)
        let parent = open(directory.path, O_RDONLY | O_DIRECTORY | O_CLOEXEC)
        guard parent >= 0 else { throw CocoaError(.fileWriteUnknown) }; defer { close(parent) }
        guard fsync(parent) == 0 else { throw CocoaError(.fileWriteUnknown) }
        return archive
    }
}
