import Foundation
import Testing
@testable import GatorCore

func temporaryDirectory() throws -> URL {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent("gator-native-test-" + UUID().uuidString)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    return root
}

@Test func nativeDraftPersistenceKeepsPendingIdentityAndNeverTouchesTerminalDrafts() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let terminal = root.appendingPathComponent("drafts/terminal.json")
    try FileManager.default.createDirectory(at: terminal.deletingLastPathComponent(), withIntermediateDirectories: true)
    let untouched = Data("terminal-private-original".utf8); try untouched.write(to: terminal)
    let store = NativeDraftStore(directory: root.appendingPathComponent("native/drafts"), profile: "one")
    let draft = NativeDraft(text: "Unsent 🐊")
    var archive = NativeArchive(); archive.selectedConversationID = "one"; archive.drafts["conversation:one"] = draft
    archive.pending = PendingSubmission(id: "durable-id", context: "conversation:one", text: draft.text, draftRevision: draft.revision, phase: .submitted)
    try await store.save(archive)
    let reopened = try await NativeDraftStore(directory: root.appendingPathComponent("native/drafts"), profile: "one").load()
    #expect(reopened.drafts["conversation:one"] == draft)
    #expect(reopened.pending?.id == "durable-id"); #expect(reopened.pending?.phase == .uncertain)
    #expect(try Data(contentsOf: terminal) == untouched)
    let other = try await NativeDraftStore(directory: root.appendingPathComponent("native/drafts"), profile: "two").load()
    #expect(other.drafts.isEmpty)
    let attributes = try FileManager.default.attributesOfItem(atPath: await store.file.path)
    #expect((attributes[.posixPermissions] as? NSNumber)?.intValue == 0o600)
}

@Test func malformedNativeDraftsFailClosedWithoutOverwriting() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "profile"), bytes = Data("broken original".utf8)
    let path = await store.file
    try bytes.write(to: path)
    await #expect(throws: (any Error).self) { try await store.load() }
    #expect(try Data(contentsOf: path) == bytes)
}
