import XCTest
@testable import HotCrossBunsMac

final class HCBCacheCryptoTests: XCTestCase {
    func testRoundTripRestoresPlaintext() throws {
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "correct horse battery staple", salt: salt, iterations: 10_000)
        let plaintext = Data("hello encrypted cache".utf8)

        let blob = try HCBCacheCrypto.encrypt(plaintext, key: key, salt: salt, iterations: 10_000)
        let decoded = try HCBCacheCrypto.decrypt(blob, key: key)

        XCTAssertEqual(decoded, plaintext)
    }

    func testWrongPassphraseFailsToDecrypt() throws {
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "right", salt: salt, iterations: 10_000)
        let wrong = try HCBCacheCrypto.deriveKey(passphrase: "wrong", salt: salt, iterations: 10_000)

        let blob = try HCBCacheCrypto.encrypt(Data("secret".utf8), key: key, salt: salt, iterations: 10_000)
        XCTAssertThrowsError(try HCBCacheCrypto.decrypt(blob, key: wrong), "wrong key must not decrypt")
    }

    func testFreshNoncePerCall() throws {
        // AES-GCM must never reuse a nonce under the same key — every encrypt
        // call gets a fresh one. Two encrypts of the same plaintext under the
        // same key produce distinct ciphertexts.
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "x", salt: salt, iterations: 10_000)
        let plaintext = Data("hello".utf8)

        let a = try HCBCacheCrypto.encrypt(plaintext, key: key, salt: salt, iterations: 10_000)
        let b = try HCBCacheCrypto.encrypt(plaintext, key: key, salt: salt, iterations: 10_000)

        XCTAssertNotEqual(a.nonce, b.nonce)
        XCTAssertNotEqual(a.ciphertext, b.ciphertext)
    }

    func testSaltLengthMatchesConstant() {
        XCTAssertEqual(HCBCacheCrypto.randomSalt().count, HCBCacheCrypto.saltBytes)
    }

    func testBlobEncodesAsJSON() throws {
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "x", salt: salt, iterations: 10_000)
        let blob = try HCBCacheCrypto.encrypt(Data("hi".utf8), key: key, salt: salt, iterations: 10_000)
        let data = try JSONEncoder().encode(blob)
        let decoded = try JSONDecoder().decode(HCBCacheCrypto.EncryptedBlob.self, from: data)
        XCTAssertEqual(decoded, blob)
    }

    func testRejectsUnknownVersion() throws {
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "x", salt: salt, iterations: 10_000)
        let real = try HCBCacheCrypto.encrypt(Data("hi".utf8), key: key, salt: salt, iterations: 10_000)
        let bumped = HCBCacheCrypto.EncryptedBlob(
            version: 999,
            iterations: real.iterations,
            salt: real.salt,
            nonce: real.nonce,
            ciphertext: real.ciphertext
        )
        XCTAssertThrowsError(try HCBCacheCrypto.decrypt(bumped, key: key))
    }

    func testMutationAuditLogEncryptsAtRestAndReloads() async throws {
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let fileURL = directory.appending(path: "audit.log")
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "history", salt: salt, iterations: 10_000)

        let log = MutationAuditLog(fileURL: fileURL)
        await log.configureEncryption(enabled: true, key: key, salt: salt)
        await log.record(kind: "task.create", resourceID: "task-1", summary: "Private dentist appointment")

        let raw = try Data(contentsOf: fileURL)
        XCTAssertFalse(String(decoding: raw, as: UTF8.self).contains("Private dentist appointment"))
        XCTAssertTrue(String(decoding: raw, as: UTF8.self).contains("encryptedV2"))

        let reloader = MutationAuditLog(fileURL: fileURL)
        await reloader.configureEncryption(enabled: true, key: key, salt: salt)
        let entries = await reloader.recentEntries(limit: 10)
        XCTAssertEqual(entries.map(\.summary), ["Private dentist appointment"])
    }

    func testMutationAuditLogMigratesPlaintextWhenEncryptionEnabled() async throws {
        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let fileURL = directory.appending(path: "audit.log")
        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "history", salt: salt, iterations: 10_000)

        let plaintext = MutationAuditLog(fileURL: fileURL)
        await plaintext.record(kind: "event.update", resourceID: "event-1", summary: "Plaintext title")

        let encrypted = MutationAuditLog(fileURL: fileURL)
        await encrypted.configureEncryption(enabled: true, key: key, salt: salt)
        let entries = await encrypted.recentEntries(limit: 10)

        XCTAssertEqual(entries.map(\.summary), ["Plaintext title"])
        let raw = try Data(contentsOf: fileURL)
        XCTAssertFalse(String(decoding: raw, as: UTF8.self).contains("Plaintext title"))
        XCTAssertTrue(String(decoding: raw, as: UTF8.self).contains("encryptedV2"))
    }

    func testLocalBackupServiceEncryptsBackupsWhenContextIsSupplied() async throws {
        struct EncryptedBackupEnvelope: Decodable {
            let encryptedV1: HCBCacheCrypto.EncryptedBlob
        }

        let directory = FileManager.default.temporaryDirectory.appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }

        let salt = HCBCacheCrypto.randomSalt()
        let key = try HCBCacheCrypto.deriveKey(passphrase: "backup", salt: salt, iterations: 10_000)
        let state = CachedAppState(
            account: nil,
            taskLists: [TaskListMirror(id: "list-1", title: "Inbox", updatedAt: nil, etag: nil)],
            tasks: [
                TaskMirror(
                    id: "task-1",
                    taskListID: "list-1",
                    parentID: nil,
                    title: "Private backup title",
                    notes: "Confidential notes",
                    status: .needsAction,
                    dueDate: nil,
                    completedAt: nil,
                    isDeleted: false,
                    isHidden: false,
                    position: nil,
                    etag: nil,
                    updatedAt: nil
                )
            ],
            calendars: [],
            events: [],
            settings: .default
        )
        let service = LocalBackupService(directoryURL: directory)

        let backupURL = try await service.writeBackup(
            state: state,
            retentionCount: 14,
            encryptionContext: LocalBackupService.EncryptionContext(key: key, salt: salt)
        )

        let raw = try Data(contentsOf: backupURL)
        let rawText = String(decoding: raw, as: UTF8.self)
        XCTAssertFalse(rawText.contains("Private backup title"))
        XCTAssertTrue(rawText.contains("encryptedV1"))

        let envelope = try JSONDecoder().decode(EncryptedBackupEnvelope.self, from: raw)
        let plaintext = try HCBCacheCrypto.decrypt(envelope.encryptedV1, key: key)
        XCTAssertTrue(String(decoding: plaintext, as: UTF8.self).contains("Private backup title"))
    }
}
