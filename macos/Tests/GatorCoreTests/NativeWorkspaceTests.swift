import Foundation
import Testing
@testable import GatorCore

@MainActor func chooseWorkspace(_ app: ManagedSessionCoordinator, _ path: String?) async throws {
    let token = try #require(app.beginWorkspaceChoice())
    await app.finishWorkspaceChoice(token, path: path)
}

@Test func workspaceRequestEncodesPathAsDataAndResultIdentity() throws {
    let path = "/tmp/café 日本 ; /source data"
    let frame = HostCommand.workspace(NativeWorkspaceRequest(path: path, id: "exact-request")).fields
    #expect(frame["type"] == .string("native_workspace"))
    #expect(frame["native_workspace"]?["path"] == .string(path))
    #expect(frame["native_workspace"]?["request_id"] == .string("exact-request"))
    #expect(frame["text"] == nil)
    let result = try NativeWorkspaceResult(.object(["request_id": .string("exact-request"), "conversation_id": .string("parent"), "source": .string(path), "new": .bool(true)]))
    #expect(result.new); #expect(result.source == path); #expect(result.conversationID == "parent")
}

@Test @MainActor func workspaceCancelAndSameFolderKeepOwnerDraftAndArchiveBytes() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let transport = SessionFixture(), app = await managedFixture(root, [transport]), original = app.current
    original.editDraft("Unsent original draft"); #expect(await original.flushDrafts())
    let archive = try FileManager.default.contentsOfDirectory(at: root.appendingPathComponent("native"), includingPropertiesForKeys: nil).first { $0.pathExtension == "json" }
    let before = try Data(contentsOf: #require(archive))
    let token = try #require(app.beginWorkspaceChoice())
    #expect(app.workspaceChoosing); #expect(app.beginWorkspaceChoice() == nil)
    await app.finishWorkspaceChoice(UUID(), path: "/wrong-token")
    #expect(app.workspaceChoosing)
    await app.finishWorkspaceChoice(token, path: nil)
    #expect(!app.workspaceChoosing); #expect(app.current === original)
    #expect(await transport.sent("native_workspace").isEmpty)
    try await chooseWorkspace(app, "/fixture")
    #expect(app.current === original); #expect(app.ownerCount == 1); #expect(app.workspaceStatus.contains("Already"))
    #expect(original.draftText == "Unsent original draft"); #expect(try Data(contentsOf: #require(archive)) == before)
    await transport.configureWorkspace(failure: "Unavailable directory")
    try await chooseWorkspace(app, "/missing")
    #expect(app.workspaceIssue.contains("Unavailable")); #expect(!app.workspaceNeedsReview)
    #expect(try Data(contentsOf: #require(archive)) == before)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceDifferentFoldersKeepDraftAttachmentsApprovalsAndWorkingScopeIsolated() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), third = SessionFixture(id: "third")
    let app = await attachmentApp(root, [first, second, third]), original = app.current
    original.editDraft("Original private draft"); try await attach(original, [try uploadFile(root)])
    let uploads = original.draftUploads
    await first.patch(["running": .bool(true), "requests_issue": .string("Original retained uncertainty"), "pending_approval": .object(["token": .string("original-only-token"), "id": .number(1)])])
    try await waitUntil { original.recovery?.unresolved == true }
    try await chooseWorkspace(app, "/tmp/Folder café 日本")
    #expect(app.workspaceIssue.isEmpty); #expect(app.current.conversationID == "second"); #expect(app.ownerCount == 2)
    let next = app.current
    #expect(await second.launch?.startSource == "/tmp/Folder café 日本")
    #expect(await second.launch?.workspace == root); #expect(next.snapshot?.value["source"].string == "/tmp/Folder café 日本")
    #expect(next.draftText.isEmpty); #expect(next.draftUploads.isEmpty); #expect(next.pendingSubmission == nil)
    #expect(next.recovery?.unresolved != true); #expect(next.snapshot?.hasApproval == false); #expect(next.snapshot?.value["queued"].int == 0)
    #expect(original.snapshot?.running == true); #expect(original.snapshot?.value["source"].string == "/fixture")
    #expect(original.snapshot?.value["pending_approval"]["token"].string == "original-only-token")
    #expect(original.draftUploads == uploads); #expect(original.draftText == "Original private draft")
    next.editDraft("Second private draft")
    try await chooseWorkspace(app, "/tmp/Another folder")
    #expect(app.current.conversationID == "third"); #expect(app.ownerCount == 3)
    await app.openConversation("parent")
    #expect(app.current === original); #expect(original.draftUploads == uploads); #expect(original.draftText == "Original private draft")
    #expect(next.draftText == "Second private draft"); #expect(await first.closed == false)
    #expect(await first.sent("upload_submit").isEmpty); #expect(await second.sent("upload_submit").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceResultCannotMoveForegroundAfterConversationChangesOrStartSecondChoice() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = CatalogOwnerOpenGate(), first = SessionFixture(), second = SessionFixture(id: "other")
    await first.configureWorkspace(gate: gate)
    let app = await managedFixture(root, [first, second]), original = app.current
    original.editDraft("Original")
    let token = try #require(app.beginWorkspaceChoice()), operation = Task { await app.finishWorkspaceChoice(token, path: "/tmp/chosen") }
    while !(await gate.didEnter()) { await Task.yield() }
    #expect(app.beginWorkspaceChoice() == nil)
    await app.finishWorkspaceChoice(token, path: "/tmp/duplicate-callback")
    #expect(await first.sent("native_workspace").count == 1)
    await app.newConversation(); #expect(app.current.conversationID == "other")
    app.current.editDraft("Other")
    await gate.release(); await operation.value
    #expect(app.current.conversationID == "other"); #expect(app.ownerCount == 2)
    #expect(app.workspaceIssue.contains("changed")); #expect(!app.workspaceNeedsReview); #expect(app.current.draftText == "Other"); #expect(original.draftText == "Original")
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceDisconnectBeforeConfirmationKeepsOriginalAndRequiresExplicitReconnect() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = CatalogOwnerOpenGate(), first = SessionFixture()
    await first.configureWorkspace(gate: gate)
    let app = await managedFixture(root, [first]), original = app.current
    original.editDraft("Survives disconnect")
    let token = try #require(app.beginWorkspaceChoice()), operation = Task { await app.finishWorkspaceChoice(token, path: "/tmp/chosen") }
    while !(await gate.didEnter()) { await Task.yield() }
    await first.disconnect(); try await waitUntil { !original.isConnected }; await gate.release(); await operation.value
    #expect(app.current === original); #expect(app.ownerCount == 1); #expect(!app.workspaceIssue.isEmpty)
    #expect(original.draftText == "Survives disconnect"); #expect(app.beginWorkspaceChoice() == nil)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceNewOwnerFailureRequiresReviewAndNeverReplaysTransition() async throws {
    for startupFailure in [true, false] {
        let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
        let first = SessionFixture(), broken = SessionFixture(id: "broken", failStart: startupFailure)
        if !startupFailure { await broken.configureWorkspace(failure: "Folder moved after validation") }
        let app = await managedFixture(root, [first, broken]), original = app.current
        original.editDraft("Kept on failure")
        try await chooseWorkspace(app, "/tmp/moved")
        #expect(app.current === original); #expect(app.ownerCount == 1); #expect(await broken.closed)
        #expect(app.workspaceNeedsReview); #expect(app.beginWorkspaceChoice() == nil)
        #expect(original.draftText == "Kept on failure"); #expect(!app.workspaceIssue.isEmpty)
        #expect(await first.sent("native_workspace").count == 1)
        await app.openConversation("parent")
        #expect(!app.workspaceNeedsReview); #expect(app.canChooseWorkspace)
        #expect(await app.shutdown(confirmed: true))
    }
}

@Test @MainActor func workspaceCapacityPreservesEveryRecoveryOwnerAndOriginalDraft() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixtures = (0..<4).map { SessionFixture(id: "chat-\($0)") }, app = await managedFixture(root, fixtures)
    for index in 0..<3 {
        await fixtures[index].patch(["requests_issue": .string("Retained uncertain evidence")])
        try await waitUntil { app.current.recovery?.unresolved == true }; await app.newConversation()
    }
    let original = app.current; original.editDraft("Capacity draft")
    try await chooseWorkspace(app, "/tmp/no-slot")
    #expect(app.current === original); #expect(app.ownerCount == 4); #expect(app.workspaceIssue.contains("Four chats"))
    #expect(!app.workspaceNeedsReview); #expect(app.canChooseWorkspace); #expect(original.draftText == "Capacity draft")
    for fixture in fixtures { #expect(await fixture.closed == false) }
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceWrongConversationResultIsRejectedWithoutOwnerCreation() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), app = await managedFixture(root, [first])
    let request = NativeWorkspaceRequest(path: "/tmp/folder", id: "expected")
    await first.configureWorkspace(result: .object(["request_id": .string("expected"), "conversation_id": .string("wrong"), "source": .string("/tmp/folder"), "new": .bool(true)]))
    await #expect(throws: (any Error).self) { try await app.current.resolveWorkspace(request) }
    #expect(app.ownerCount == 1); #expect(app.current.conversationID == "parent")
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspacePickerTicketCannotSurviveAnExplicitOwnerReconnect() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), replacement = SessionFixture(), app = await managedFixture(root, [first, replacement])
    let owner = app.current, generation = owner.generation, ticket = try #require(app.beginWorkspaceChoice())
    owner.editDraft("Kept across explicit reconnect")
    await first.disconnect(); try await waitUntil { !owner.isConnected }
    await app.reconnect()
    #expect(owner.generation != generation); #expect(owner.isConnected)
    await app.finishWorkspaceChoice(ticket, path: "/tmp/stale-picker")
    #expect(app.current === owner); #expect(app.ownerCount == 1); #expect(app.workspaceIssue.contains("changed"))
    #expect(owner.draftText == "Kept across explicit reconnect")
    #expect(await replacement.sent("native_workspace").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceUnexpectedNewOwnerReviewIsRetainedWithoutAuthorizingOrSelectingIt() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), unexpected = SessionFixture(id: "unexpected")
    await unexpected.patch(["pending_approval": .object(["token": .string("unexpected-exact-token"), "id": .number(1)]), "requests_issue": .string("Uncertain new owner evidence")])
    let app = await managedFixture(root, [first, unexpected]), original = app.current
    original.editDraft("Keep original on unexpected owner")
    try await chooseWorkspace(app, "/tmp/chosen")
    #expect(app.current === original); #expect(app.ownerCount == 2); #expect(app.workspaceNeedsReview)
    #expect(await unexpected.closed == false); #expect(original.draftText == "Keep original on unexpected owner")
    #expect(app.owners["unexpected"]?.snapshot?.value["pending_approval"]["token"].string == "unexpected-exact-token")
    #expect(await unexpected.sent("approval").isEmpty); #expect(await unexpected.sent("upload_submit").isEmpty)
    await app.openConversation("unexpected")
    #expect(app.current.conversationID == "unexpected"); #expect(!app.workspaceNeedsReview)
    #expect(await app.shutdown(confirmed: true))
}
