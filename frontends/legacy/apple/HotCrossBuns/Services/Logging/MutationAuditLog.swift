import Foundation
import CryptoKit

// Append-only ledger of user-originated mutations. Separate from
// AppLogger (which captures general debug info) because this is the
// "I need to answer 'when did I mark that done?' six months from now"
// record. Kept to a generous cap so even a year of active use fits
// in a single file.
struct MutationAuditEntry: Codable, Hashable, Sendable, Identifiable {
    let timestamp: Date
    let kind: String
    let resourceID: String
    let summary: String
    let metadata: [String: String]
    // JSON encodings of the pre/post resource state. Optional for backwards
    // compatibility with entries written before the history expansion — old
    // entries decode with nil and simply can't offer snapshot-copy.
    let priorSnapshotJSON: String?
    let postSnapshotJSON: String?

    var id: String { "\(timestamp.timeIntervalSince1970)-\(resourceID)-\(kind)" }

    enum CodingKeys: String, CodingKey {
        case timestamp, kind, resourceID, summary, metadata, priorSnapshotJSON, postSnapshotJSON
    }

    init(
        timestamp: Date,
        kind: String,
        resourceID: String,
        summary: String,
        metadata: [String: String],
        priorSnapshotJSON: String? = nil,
        postSnapshotJSON: String? = nil
    ) {
        self.timestamp = timestamp
        self.kind = kind
        self.resourceID = resourceID
        self.summary = summary
        self.metadata = metadata
        self.priorSnapshotJSON = priorSnapshotJSON
        self.postSnapshotJSON = postSnapshotJSON
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        timestamp = try c.decode(Date.self, forKey: .timestamp)
        kind = try c.decode(String.self, forKey: .kind)
        resourceID = try c.decode(String.self, forKey: .resourceID)
        summary = try c.decode(String.self, forKey: .summary)
        metadata = try c.decode([String: String].self, forKey: .metadata)
        priorSnapshotJSON = try c.decodeIfPresent(String.self, forKey: .priorSnapshotJSON)
        postSnapshotJSON = try c.decodeIfPresent(String.self, forKey: .postSnapshotJSON)
    }
}

actor MutationAuditLog {
    static let shared = MutationAuditLog()
    // Absolute ceiling enforced regardless of user setting to keep file
    // size bounded. Settings slider caps out at this value.
    static let absoluteCeiling = 50000
    private var retentionLimit: Int = 5000
    private var buffer: [MutationAuditEntry] = []
    private var hasLoaded = false
    private let storageURL: URL?
    private var encryptionKey: SymmetricKey?
    private var encryptionSalt: Data?
    private var encryptionEnabled = false

    init(fileURL: URL? = MutationAuditLog.fileURL()) {
        storageURL = fileURL
    }

    func configureEncryption(enabled: Bool, key: SymmetricKey?, salt: Data?) {
        if hasLoaded == false, enabled || key != nil {
            encryptionKey = key ?? encryptionKey
            ensureLoaded()
        }
        encryptionEnabled = enabled
        encryptionKey = enabled ? key : nil
        encryptionSalt = enabled ? salt : nil
        if hasLoaded {
            rewriteEntireFile()
        }
    }

    func setRetentionLimit(_ limit: Int) {
        retentionLimit = max(1, min(Self.absoluteCeiling, limit))
        if buffer.count > retentionLimit {
            buffer.removeFirst(buffer.count - retentionLimit)
            rewriteEntireFile()
        }
    }

    func record(
        kind: String,
        resourceID: String,
        summary: String,
        metadata: [String: String] = [:],
        priorSnapshotJSON: String? = nil,
        postSnapshotJSON: String? = nil
    ) {
        ensureLoaded()
        let entry = MutationAuditEntry(
            timestamp: Date(),
            kind: kind,
            resourceID: resourceID,
            summary: summary,
            metadata: metadata,
            priorSnapshotJSON: priorSnapshotJSON,
            postSnapshotJSON: postSnapshotJSON
        )
        buffer.append(entry)
        // Fast path: O(1) append to JSONL file. Retention trim triggers a
        // full rewrite, but only when the buffer actually exceeds the cap —
        // which is rare compared to the common case of appending one entry.
        if buffer.count > retentionLimit {
            buffer.removeFirst(buffer.count - retentionLimit)
            rewriteEntireFile()
        } else {
            persistLastEntry()
        }
    }

    func recentEntries(limit: Int = 100) -> [MutationAuditEntry] {
        ensureLoaded()
        return Array(buffer.suffix(limit).reversed())
    }

    func allEntries() -> [MutationAuditEntry] {
        ensureLoaded()
        return buffer.reversed()
    }

    func delete(id entryID: String) {
        ensureLoaded()
        buffer.removeAll { $0.id == entryID }
        rewriteEntireFile()
    }

    func clear() {
        buffer = []
        rewriteEntireFile()
    }

    private func ensureLoaded() {
        if hasLoaded { return }
        hasLoaded = true
        guard
            let url = storageURL,
            FileManager.default.fileExists(atPath: url.path),
            let data = try? Data(contentsOf: url),
            data.isEmpty == false
        else { return }
        if let envelope = try? JSONDecoder().decode(EncryptedEnvelope.self, from: data) {
            guard let key = encryptionKey else {
                AppLogger.warn("audit log is encrypted but no key is available", category: .cache)
                return
            }
            do {
                let plaintext = try HCBCacheCrypto.decrypt(envelope.encryptedV2, key: key)
                buffer = Self.decodeJSONL(plaintext)
            } catch {
                AppLogger.warn("audit log decrypt failed", category: .cache, metadata: ["error": String(describing: error)])
            }
            return
        }
        // Back-compat: legacy v1 format wrote the whole buffer as a JSON array.
        // New v2 format is JSONL (one entry per line, O(1) append on record).
        // Detect v1 by leading '[' and rewrite once into v2 on next persist.
        if data.first == UInt8(ascii: "[") {
            if let legacy = try? JSONDecoder().decode([MutationAuditEntry].self, from: data) {
                buffer = legacy
                rewriteEntireFile()
            }
            return
        }
        buffer = Self.decodeJSONL(data)
        if encryptionEnabled {
            rewriteEntireFile()
        }
    }

    private static func decodeJSONL(_ data: Data) -> [MutationAuditEntry] {
        let decoder = JSONDecoder()
        var decoded: [MutationAuditEntry] = []
        decoded.reserveCapacity(4096)
        data.withUnsafeBytes { raw in
            let bytes = raw.bindMemory(to: UInt8.self)
            var start = 0
            for i in 0..<bytes.count where bytes[i] == UInt8(ascii: "\n") {
                if i > start {
                    let slice = Data(bytes[start..<i])
                    if let entry = try? decoder.decode(MutationAuditEntry.self, from: slice) {
                        decoded.append(entry)
                    }
                }
                start = i + 1
            }
            if start < bytes.count {
                let slice = Data(bytes[start..<bytes.count])
                if let entry = try? decoder.decode(MutationAuditEntry.self, from: slice) {
                    decoded.append(entry)
                }
            }
        }
        return decoded
    }

    // Shared encoder instance — reused across every record() call so we
    // don't pay the encoder-construction cost per entry.
    private static let entryEncoder: JSONEncoder = {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .deferredToDate
        return e
    }()

    // Single-entry append. O(1) cost: encode one entry, open the file in
    // append mode, write the bytes + newline, close. Replaces the old
    // "re-encode the entire buffer on every record" hot path that turned
    // a user-facing mutation into a ~1 MB atomic disk write.
    private func persistLastEntry() {
        guard encryptionEnabled == false else {
            rewriteEntireFile()
            return
        }
        guard let url = storageURL, let entry = buffer.last else { return }
        do {
            try FileManager.default.createDirectory(
                at: url.deletingLastPathComponent(),
                withIntermediateDirectories: true
            )
            if FileManager.default.fileExists(atPath: url.path) == false {
                FileManager.default.createFile(atPath: url.path, contents: nil)
            }
            var line = try Self.entryEncoder.encode(entry)
            line.append(UInt8(ascii: "\n"))
            let handle = try FileHandle(forWritingTo: url)
            defer { try? handle.close() }
            try handle.seekToEnd()
            try handle.write(contentsOf: line)
        } catch {
            AppLogger.warn("audit log append failed", category: .cache, metadata: ["error": String(describing: error)])
        }
    }

    // Used for deletes, retention trims, clears, and the v1→v2 migration.
    // A full rewrite is O(n) but these are rare compared to append.
    private func rewriteEntireFile() {
        guard let url = storageURL else { return }
        do {
            try FileManager.default.createDirectory(
                at: url.deletingLastPathComponent(),
                withIntermediateDirectories: true
            )
            let plaintext = try encodedJSONL()
            let data: Data
            if encryptionEnabled {
                guard let key = encryptionKey, let salt = encryptionSalt else {
                    AppLogger.warn("audit log encryption enabled without key", category: .cache)
                    return
                }
                let blob = try HCBCacheCrypto.encrypt(plaintext, key: key, salt: salt)
                data = try JSONEncoder().encode(EncryptedEnvelope(encryptedV2: blob))
            } else {
                data = plaintext
            }
            try data.write(to: url, options: [.atomic])
        } catch {
            AppLogger.warn("audit log rewrite failed", category: .cache, metadata: ["error": String(describing: error)])
        }
    }

    private func encodedJSONL() throws -> Data {
        var data = Data()
        data.reserveCapacity(buffer.count * 256)
        for entry in buffer {
            let line = try Self.entryEncoder.encode(entry)
            data.append(line)
            data.append(UInt8(ascii: "\n"))
        }
        return data
    }

    private struct EncryptedEnvelope: Codable {
        let encryptedV2: HCBCacheCrypto.EncryptedBlob
    }

    static func fileURL() -> URL? {
        guard let support = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else { return nil }
        let bundle = Bundle.main.bundleIdentifier ?? "HotCrossBuns"
        return support
            .appending(path: bundle, directoryHint: .isDirectory)
            .appending(path: "audit.log")
    }
}
