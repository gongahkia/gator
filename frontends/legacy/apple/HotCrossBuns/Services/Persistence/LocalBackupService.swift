import AppKit
import CryptoKit
import Foundation

actor LocalBackupService {
    struct BackupSummary: Hashable, Sendable {
        var directoryPath: String
        var latestBackupURL: URL?
        var totalBytes: Int64
        var backupCount: Int
    }

    struct EncryptionContext: Sendable {
        var key: SymmetricKey
        var salt: Data
    }

    private struct BackupEnvelope: Codable {
        var formatVersion: Int
        var createdAt: Date
        var appVersion: String
        var state: CachedAppState
    }

    private struct EncryptedBackupEnvelope: Codable {
        var formatVersion: Int
        var createdAt: Date
        var appVersion: String
        var encryptedV1: HCBCacheCrypto.EncryptedBlob
    }

    private let directoryURL: URL?
    private let fileManager: FileManager

    init(
        directoryURL: URL? = LocalBackupService.defaultBackupDirectoryURL,
        fileManager: FileManager = .default
    ) {
        self.directoryURL = directoryURL
        self.fileManager = fileManager
    }

    func writeBackup(
        state: CachedAppState,
        now: Date = Date(),
        retentionCount: Int,
        encryptionContext: EncryptionContext? = nil
    ) throws -> URL {
        guard let directoryURL else {
            throw CocoaError(.fileNoSuchFile)
        }

        try fileManager.createDirectory(at: directoryURL, withIntermediateDirectories: true)
        let stamp = Self.filenameFormatter.string(from: now)
        let destination = uniqueBackupURL(in: directoryURL, stamp: stamp)
        let envelope = BackupEnvelope(
            formatVersion: 1,
            createdAt: now,
            appVersion: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "unknown",
            state: state
        )
        let data: Data
        if let encryptionContext {
            let plaintext = try Self.encoder.encode(envelope)
            let encrypted = try HCBCacheCrypto.encrypt(
                plaintext,
                key: encryptionContext.key,
                salt: encryptionContext.salt
            )
            let encryptedEnvelope = EncryptedBackupEnvelope(
                formatVersion: 2,
                createdAt: now,
                appVersion: envelope.appVersion,
                encryptedV1: encrypted
            )
            data = try Self.encoder.encode(encryptedEnvelope)
        } else {
            data = try Self.encoder.encode(envelope)
        }
        try data.write(to: destination, options: [.atomic])
        try pruneBackups(keeping: retentionCount)
        return destination
    }

    private func uniqueBackupURL(in directoryURL: URL, stamp: String) -> URL {
        var destination = directoryURL.appending(path: "hot-cross-buns-\(stamp).json")
        var suffix = 2
        while fileManager.fileExists(atPath: destination.path) {
            destination = directoryURL.appending(path: "hot-cross-buns-\(stamp)-\(suffix).json")
            suffix += 1
        }
        return destination
    }

    func summary() -> BackupSummary {
        guard let directoryURL else {
            return BackupSummary(directoryPath: "", latestBackupURL: nil, totalBytes: 0, backupCount: 0)
        }

        let backups = backupFiles()
        return BackupSummary(
            directoryPath: directoryURL.path,
            latestBackupURL: backups.first,
            totalBytes: backups.reduce(Int64(0)) { $0 + Self.fileSize(at: $1) },
            backupCount: backups.count
        )
    }

    func openBackupDirectoryInFinder() throws {
        guard let directoryURL else {
            throw CocoaError(.fileNoSuchFile)
        }
        try fileManager.createDirectory(at: directoryURL, withIntermediateDirectories: true)
        Task { @MainActor in
            NSWorkspace.shared.activateFileViewerSelecting([directoryURL])
        }
    }

    private func pruneBackups(keeping retentionCount: Int) throws {
        let clamped = max(1, min(90, retentionCount))
        let backups = backupFiles()
        guard backups.count > clamped else { return }
        for url in backups.dropFirst(clamped) {
            try? fileManager.removeItem(at: url)
        }
    }

    private func backupFiles() -> [URL] {
        guard let directoryURL,
              let urls = try? fileManager.contentsOfDirectory(
                at: directoryURL,
                includingPropertiesForKeys: [.contentModificationDateKey, .fileSizeKey],
                options: [.skipsHiddenFiles]
              ) else {
            return []
        }

        return urls
            .filter { $0.pathExtension == "json" && $0.lastPathComponent.hasPrefix("hot-cross-buns-") }
            .sorted { lhs, rhs in
                let leftDate = (try? lhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
                let rightDate = (try? rhs.resourceValues(forKeys: [.contentModificationDateKey]).contentModificationDate) ?? .distantPast
                return leftDate > rightDate
            }
    }
}

private extension LocalBackupService {
    static var defaultBackupDirectoryURL: URL? {
        guard let appSupportURL = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else {
            return nil
        }

        let appDirectoryName = Bundle.main.bundleIdentifier ?? "HotCrossBuns"
        return appSupportURL
            .appending(path: appDirectoryName, directoryHint: .isDirectory)
            .appending(path: "Backups", directoryHint: .isDirectory)
    }

    static var encoder: JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .iso8601
        #if DEBUG
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        #endif
        return encoder
    }

    static let filenameFormatter: DateFormatter = {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = .current
        formatter.dateFormat = "yyyy-MM-dd-HHmmss"
        return formatter
    }()

    static func fileSize(at url: URL) -> Int64 {
        guard let values = try? url.resourceValues(forKeys: [.fileSizeKey]) else { return 0 }
        return Int64(values.fileSize ?? 0)
    }
}
