import Foundation
import Testing
@testable import GatorCore

@MainActor func attachmentApp(_ root: URL, _ fixtures: [SessionFixture]) async -> ManagedSessionCoordinator {
    let pool = SessionFixturePool(fixtures)
    let config = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root, environment: ["HOME": root.path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path])
    let app = ManagedSessionCoordinator(configuration: config, store: NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "attachments"), operationTimeout: .seconds(2), admissionTimeout: .milliseconds(200), factory: { pool.next() })
    await app.start(); return app
}
@MainActor func attach(_ owner: GatorApplicationState, _ urls: [URL]) async throws {
    let request = try #require(owner.beginAttachmentImport()); await owner.importFiles(urls, request: request)
    #expect(owner.attachmentIssue.isEmpty)
}
@Test @MainActor func attachmentDraftsShareSurfacesRemainOwnerScopedAndRestoreAcrossRestart() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await attachmentApp(root, [first, second])
    let compact = app, full = app, parent = app.current, source = try uploadFile(root)
    parent.editDraft("Parent text"); parent.setPlanFirst(true); parent.setReplyToUpdateID("update-one")
    try await attach(parent, [source]); let files = parent.draftUploads
    #expect(compact.current.draftUploads == full.current.draftUploads)
    let importRequest = try #require(parent.beginAttachmentImport())
    await app.newConversation(); #expect(app.current !== parent); #expect(app.current.draftUploads.isEmpty)
    await parent.importImage(try attachmentPNG(), request: importRequest)
    #expect(parent.draftUploads.count == 2); #expect(app.current.draftUploads.isEmpty)
    app.current.editDraft("Second text"); await app.openConversation("parent")
    #expect(app.current === parent); #expect(parent.draftText == "Parent text"); #expect(parent.draftUploads.first == files.first)
    #expect(parent.planFirst); #expect(parent.draftReplyToUpdateID == "update-one")
    #expect(await app.shutdown())
    let restored = await attachmentApp(root, [SessionFixture()])
    #expect(restored.current.draftUploads.count == 2); #expect(restored.current.draftText == "Parent text")
    #expect(restored.current.planFirst); #expect(restored.current.draftReplyToUpdateID == "update-one")
    await restored.current.validateDraftAttachments(); #expect(restored.current.attachmentIssue.isEmpty)
    let copy = root.appendingPathComponent("state/gator/native-uploads").appendingPathComponent(files[0].id)
    restored.current.removeAttachment(files[0].id)
    #expect(FileManager.default.fileExists(atPath: copy.path)); #expect(try Data(contentsOf: source) == Data("Original jade heron".utf8))
    #expect(await restored.shutdown())
}
@Test @MainActor func attachmentAdmissionRequiresExactResultAndPreservesNewerDraftAndFiles() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(); await fixture.setAutoAdmit(false)
    let app = await attachmentApp(root, [fixture]), owner = app.current
    try await attach(owner, [try uploadFile(root)]); owner.setPlanFirst(true); owner.setReplyToUpdateID("update-one")
    let captured = owner.draftUploads
    await app.submit(from: owner); let pending = try #require(owner.pendingSubmission)
    let frame = try #require(await fixture.sent("upload_submit").last)
    #expect(frame["upload"]["text"].string.isEmpty); #expect(frame["upload"]["files"].array == captured.map { .string($0.id) })
    #expect(frame["upload"]["plan_first"].bool); #expect(frame["upload"]["reply_to_update_id"].string == "update-one")
    #expect(pending.ownerGeneration == owner.generation); #expect(pending.uploads == captured)
    #expect(owner.draftUploads == captured) // synchronous action_applied is insufficient
    await fixture.patch(["upload_result": .object(["request_id": .string("unrelated")])])
    try await Task.sleep(for: .milliseconds(20)); #expect(owner.pendingSubmission?.id == pending.id)
    owner.editDraft("Newer draft"); try await attach(owner, [try uploadFile(root, "next.txt")])
    await fixture.patch(["upload_result": .object(["request_id": .string(pending.id)])])
    try await waitUntil { owner.pendingSubmission == nil }
    #expect(owner.draftText == "Newer draft"); #expect(owner.draftUploads.count == 2)
    await fixture.setAutoAdmit(true); await app.submit(from: owner)
    try await waitUntil { owner.pendingSubmission == nil }
    #expect(owner.draftText.isEmpty); #expect(owner.draftUploads.isEmpty)
    #expect(await app.shutdown())
}
@Test @MainActor func missingAndRejectedUploadsRemainRepairableAndUncertainRequestsNeverReplay() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(); await fixture.setAutoAdmit(false)
    let replacement = SessionFixture(), app = await attachmentApp(root, [fixture, replacement]), owner = app.current
    try await attach(owner, [try uploadFile(root)])
    await app.submit(from: owner); let pending = try #require(owner.pendingSubmission)
    await fixture.patch(["upload_result": .object(["request_id": .string(pending.id), "error": .string("Input is unavailable")])])
    try await waitUntil { owner.submission?.phase == .failed }
    #expect(owner.draftUploads.count == 1)
    await app.submit(from: owner); let uncertain = try #require(owner.pendingSubmission)
    await fixture.disconnect(); try await waitUntil { owner.connectionStatus == .disconnected }
    await app.reconnect(); #expect(owner.draftUploads.count == 1)
    #expect(owner.pendingSubmission?.id == uncertain.id); #expect(await replacement.sent("upload_submit").isEmpty)
    #expect(await app.shutdown(confirmed: true))
    let restored = await attachmentApp(root, [SessionFixture()])
    #expect(restored.current.pendingSubmission?.id == uncertain.id); #expect(restored.current.draftUploads.count == 1)
    let copy = root.appendingPathComponent("state/gator/native-uploads").appendingPathComponent(restored.current.draftUploads[0].id)
    try FileManager.default.removeItem(at: copy); await restored.current.validateDraftAttachments()
    #expect(!restored.current.attachmentIssue.isEmpty); #expect(await restored.shutdown(confirmed: true))
}
@Test @MainActor func cancelledOrStaleImportCannotAttachToReplacementContext() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await attachmentApp(root, [fixture]), owner = app.current
    owner.editDraft("kept"); let request = try #require(owner.beginAttachmentImport())
    #expect(owner.quitGuard != nil); owner.cancelAttachmentImport(request)
    await owner.importFiles([try uploadFile(root)], request: request); #expect(owner.draftUploads.isEmpty)
    let stale = try #require(owner.beginAttachmentImport())
    await fixture.patch(["conversation_id": .string("changed")]); try await waitUntil { owner.conversationID == "changed" }
    await owner.importFiles([try uploadFile(root)], request: stale)
    #expect(owner.draftUploads.isEmpty); #expect(owner.attachmentIssue.contains("destination changed"))
    #expect(await app.shutdown(confirmed: true))
}
@Test func attachmentArchiveRetainsAgedDraftsAndFailsClosedWithoutOverwriting() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let files = try await NativeUploadStore(root: root.appendingPathComponent("uploads")).importFiles([try uploadFile(root)], existing: [])
    let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "attachments")
    var archive = NativeArchive(); archive.version = 6
    var old = NativeDraft(); old.text = "Original v6 state"; old.updated = .distantPast; old.uploads = files
    archive.drafts["conversation:parent"] = old
    try await store.save(archive); let loaded = try await store.load()
    #expect(loaded.version == 7); #expect(loaded.drafts["conversation:parent"]?.uploads == files)
    let file = await store.file, before = try Data(contentsOf: file)
    var invalid = loaded; invalid.version = 99; try JSONEncoder().encode(invalid).write(to: file)
    await #expect(throws: (any Error).self) { try await store.load() }
    #expect(try Data(contentsOf: file) != before)
    try Data("{broken".utf8).write(to: file)
    await #expect(throws: (any Error).self) { try await store.load() }
    #expect(try Data(contentsOf: file) == Data("{broken".utf8))
    let oldFile = root.appendingPathComponent("legacy.json"); try JSONEncoder().encode(NativeArchive()).write(to: oldFile)
    let legacy = try JSONDecoder().decode(NativeArchive.self, from: Data(contentsOf: oldFile)); #expect(legacy.drafts.isEmpty)
}
@Test @MainActor func firstWorkspaceAttachmentSurvivesRestartAndDurableConversationPromotion() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    func initial() async -> SessionFixture {
        let fixture = SessionFixture(id: "")
        await fixture.patch(["source": .string(root.path), "primary": .bool(true)])
        return fixture
    }
    let first = await initial(), app = await attachmentApp(root, [first])
    #expect(app.issue.isEmpty); #expect(app.current.conversationID.isEmpty)
    app.current.editDraft("Before first admission")
    try await attach(app.current, [try uploadFile(root)])
    let files = app.current.draftUploads; #expect(await app.shutdown())
    let replacement = await initial(); await replacement.setAutoAdmit(false)
    let restored = await attachmentApp(root, [replacement]), owner = restored.current
    #expect(owner.draftUploads == files); #expect(owner.draftText == "Before first admission")
    await restored.submit(from: owner); let pending = try #require(owner.pendingSubmission)
    owner.editDraft("Newer workspace edit")
    await replacement.patch(["conversation_id": .string("durable"), "upload_result": .object(["request_id": .string(pending.id)])])
    try await waitUntil { owner.pendingSubmission == nil && owner.conversationID == "durable" }
    await restored.settled(); #expect(owner.draftUploads == files); #expect(owner.draftText == "Newer workspace edit")
    #expect(await restored.shutdown())
    let final = await attachmentApp(root, [SessionFixture(id: "durable")])
    #expect(final.current.conversationID == "durable"); #expect(final.current.draftUploads == files)
    #expect(final.current.draftText == "Newer workspace edit"); #expect(await final.shutdown())
}
@Test func attachedDialogueUsesInstantsAcrossUTCLocalOffsetsAndPreservesInputIdentity() {
    let snapshot = BridgeSnapshot(.object(["conversation_id": .string("owner"), "messages": .array([
        .object(["role": .string("You"), "text": .string("Older main work"), "created_at": .string("2026-11-16T13:37:00.123456+08:00")]),
        .object(["role": .string("Gator"), "text": .string("Later main result"), "created_at": .string("2026-11-16T13:40:00+08:00")])]), "dialogue": .array([
        .object(["id": .string("turn"), "sequence": .number(1), "text": .string("New attached question"), "response": .string("Independent answer"), "created_at": .string("2026-11-16T05:38:00.123456789Z"), "input_ids": .array([.string(String(repeating: "a", count: 32) + "/notes.md")])])])]))
    #expect(snapshot.transcript.map(\.text) == ["Older main work", "New attached question", "Independent answer", "Later main result"])
    #expect(snapshot.transcript[1].value["input_ids"].array.count == 1)
    #expect(snapshot.messages[1].id == "conversation:owner:1") // approval indexes unchanged
}
@Test @MainActor func uncertainFirstAdmissionRetainsWorkspaceRequestIdentityInDurableOwnerAfterRestart() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(id: ""); await first.setAutoAdmit(false)
    await first.patch(["source": .string(root.path), "primary": .bool(true)])
    let app = await attachmentApp(root, [first]), owner = app.current
    try await attach(owner, [try uploadFile(root)]); await app.submit(from: owner)
    let pending = try #require(owner.pendingSubmission)
    await first.patch(["conversation_id": .string("durable")])
    try await waitUntil { owner.conversationID == "durable" }; await app.settled()
    #expect(owner.pendingSubmission?.ownerConversationID == "durable")
    #expect(await app.shutdown(confirmed: true))
    let transport = SessionFixture(id: "durable"), restored = await attachmentApp(root, [transport])
    #expect(restored.current.pendingSubmission?.id == pending.id)
    #expect(restored.current.pendingSubmission?.context == pending.context)
    #expect(restored.current.pendingSubmission?.uploads == pending.uploads)
    #expect(restored.current.draftUploads == pending.uploads)
    #expect(await transport.sent("upload_submit").isEmpty)
    #expect(await restored.shutdown(confirmed: true))
}
