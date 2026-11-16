import Foundation
import Testing
@testable import GatorCore

@MainActor final class NativeAttachmentFixture {
    let root: URL, provider: Process, configuration: BridgeLaunchConfiguration
    let pool = RecoveryGoPool()
    var app: ManagedSessionCoordinator
    init() async throws {
        root = try temporaryDirectory(); provider = Process(); let output = Pipe()
        let path = ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin"
        provider.executableURL = URL(fileURLWithPath: "/usr/bin/env"); provider.arguments = ["node", try #require(Bundle.module.url(forResource: "attachment-provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
        provider.environment = ["PATH": path, "GATOR_FIXTURE_CONTROL": root.path]; provider.standardOutput = output; provider.standardError = FileHandle.standardError
        try provider.run(); var line = Data()
        while let byte = try output.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
        let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
        let config = root.appendingPathComponent("config/gator"), workspace = root.appendingPathComponent("workspace")
        try FileManager.default.createDirectory(at: config, withIntermediateDirectories: true); try FileManager.default.createDirectory(at: workspace, withIntermediateDirectories: true)
        let settings: JSONValue = .object(["version": .number(4), "defaults": .object(["provider": .string("fixture"), "model": .string("fixture")]), "custom_providers": .array([.object(["id": .string("fixture"), "base_url": .string("http://127.0.0.1:\(port)/chat"), "models": .array([.string("fixture")]), "default_model": .string("fixture")])])])
        try JSONEncoder().encode(settings).write(to: config.appendingPathComponent("config.json"))
        let env = ["PATH": path, "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
        configuration = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"), workspace: workspace, environment: env)
        let pool = pool
        app = ManagedSessionCoordinator(configuration: configuration, store: NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "attachment-go"), factory: { pool.next() })
        await app.start(); try await managedEventually("attachment owner ready") { app.current.isConnected }
        await app.current.selectWorkMode(.inspect)
    }
    func trace() -> [JSONValue] { ((try? String(contentsOf: root.appendingPathComponent("attachment-trace.jsonl"), encoding: .utf8)) ?? "").split(separator: "\n").compactMap { try? JSONDecoder().decode(JSONValue.self, from: Data($0.utf8)) } }
    func send(_ text: String = "") async throws {
        let owner = app.current; owner.editDraft(text)
        try await managedEventually("attachment composer ready") { app.canSubmit }
        await app.submit(from: owner); try await managedEventually("matching Go admission") { owner.pendingSubmission == nil }
        #expect(owner.submission?.phase != .failed)
    }
    func close() async { try? Data().write(to: root.appendingPathComponent("release-main")); _ = await app.shutdown(confirmed: true); if provider.isRunning { provider.terminate(); provider.waitUntilExit() }; try? FileManager.default.removeItem(at: root) }
}
@Test @MainActor func realGoReceivesImmutableDocumentAndFilesOnlyImageThroughManagedOwner() async throws {
    let f = try await NativeAttachmentFixture()
    do {
        let owner = f.app.current, original = try uploadFile(f.root)
        try await attach(owner, [original]); let document = try #require(owner.draftUploads.first)
        try Data("Changed source is never sent".utf8).write(to: original); try FileManager.default.removeItem(at: original)
        try await f.send("Read the document")
        try await managedEventually("immutable provider content") { f.trace().contains { $0["kind"].string == "work" && $0.pretty.contains("Original jade heron") } && owner.snapshot?.running == false }
        #expect(!f.trace().contains { $0.pretty.contains("Changed source is never sent") })
        let id = owner.conversationID; #expect(!id.isEmpty)
        let request = try #require(owner.beginAttachmentImport()); await owner.importImage(try attachmentPNG(), request: request)
        let image = try #require(owner.draftUploads.first)
        try await f.send()
        try await managedEventually("files-only pixels delivered") { f.trace().contains { $0["kind"].string == "work" && $0.pretty.contains("data:image/png;base64,") } && owner.snapshot?.running == false }
        #expect(f.trace().contains { $0.pretty.contains("Please take a look at the attached files.") })
        let journal = try Data(contentsOf: f.root.appendingPathComponent("state/gator/conversations/" + id + "/requests.json"))
        let retained = String(decoding: journal, as: UTF8.self)
        #expect(retained.contains(document.id)); #expect(retained.contains(image.id))
        #expect(FileManager.default.fileExists(atPath: f.configuration.nativeUploadDirectory.appendingPathComponent(document.id).path))
        #expect(owner.draftUploads.isEmpty); #expect(owner.draftText.isEmpty)
        await f.close()
    } catch { print("Attachment fixture:", f.app.issue, f.app.current.issue, f.trace().map(\.pretty)); await f.close(); throw error }
}
@Test @MainActor func realGoAttachmentDialogueRetainsInputsWhileOriginalWorkContinues() async throws {
    let f = try await NativeAttachmentFixture()
    do {
        try await f.send("Establish a durable conversation before restart")
        try await managedEventually("first work completed") { f.app.current.snapshot?.running == false }
        #expect(await f.app.shutdown())
        f.app = ManagedSessionCoordinator(configuration: f.configuration, store: NativeDraftStore(directory: f.root.appendingPathComponent("native"), profile: "attachment-go"), factory: { f.pool.next() })
        await f.app.start(); await f.app.current.selectWorkMode(.inspect)
        let parent = f.app.current
        try await f.send("Hold original task")
        try await managedEventually("held main work") { parent.snapshot?.running == true && parent.snapshot?.value["dialogue_active"].bool == true }
        try await attach(parent, [try uploadFile(f.root)])
        let files = parent.draftUploads
        try await f.send("Tell me about this attachment")
        try await managedEventually("independent attachment reply") { parent.transcript.contains { $0.text.contains("Attached input retained") } }
        #expect(parent.snapshot?.running == true); #expect(parent.snapshot?.value["queued"].int == 0)
        #expect(parent.snapshot?.value["dialogue"].array.contains { $0["input_ids"].array == files.map { .string($0.id) } } == true)
        try await attach(parent, [try uploadFile(f.root, "separate.md")])
        try await f.send("Read separate attachment")
        try await managedEventually("delegated input on independent owner") { f.app.ownerCount == 2 && f.app.owners.values.contains { $0 !== parent && $0.snapshot?.running == false && $0.messages.contains { $0.text.contains("Original jade heron") } } }
        #expect(parent.snapshot?.running == true); #expect(f.app.current === parent)
        #expect(f.trace().contains { $0["kind"].string == "work" && $0.pretty.contains("Original jade heron") })
        try Data().write(to: f.root.appendingPathComponent("release-main"))
        try await managedEventually("main completion retained") { parent.snapshot?.running == false }
        await f.close()
    } catch { print("Attachment dialogue fixture:", f.app.issue, f.app.current.issue, f.trace().map(\.pretty)); await f.close(); throw error }
}
