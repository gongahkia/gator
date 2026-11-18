import Foundation
import Darwin
import Testing
@testable import GatorCore

func workspaceFixtureCanonical(_ path: String) throws -> String {
    let pointer = try #require(realpath(path, nil)); defer { free(pointer) }
    return String(cString: pointer)
}

@Test @MainActor func workspaceActualGoCanonicalFoldersDraftsCatalogAndRestartIdentity() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 2)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let folder = fixture.root.appendingPathComponent("Project café 日本 ; source"), alias = fixture.root.appendingPathComponent("alias")
    let another = fixture.root.appendingPathComponent("Another project")
    try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    try FileManager.default.createDirectory(at: another, withIntermediateDirectories: true)
    try FileManager.default.createSymbolicLink(at: alias, withDestinationURL: folder)
    let canonical = try workspaceFixtureCanonical(folder.path), app = fixture.application()
    do {
        await app.start(); try await managedEventually("workspace original ready") { app.current.isConnected }
        let original = app.current, originalID = original.conversationID, source = app.workspaceSource
        original.editDraft("Original workspace draft")
        try await attach(original, [try uploadFile(fixture.root)])
        let originalUploads = original.draftUploads
        #expect(originalUploads.allSatisfy { FileManager.default.fileExists(atPath: fixture.configuration.nativeUploadDirectory.appendingPathComponent($0.id).path) })
        #expect(await original.flushDrafts())
        let messages = original.transcript, scope = NativeTranscriptSearch.Scope(context: original.context, generation: original.generation)
        let index = NativeTranscriptSearch(scope: scope, messages: messages)
        let start = ContinuousClock.now
        try await chooseWorkspace(app, alias.path)
        #expect(app.workspaceIssue.isEmpty); #expect(app.current !== original); #expect(app.ownerCount == 2)
        let selected = app.current, selectedID = selected.conversationID
        print("GATOR_P03_HANDOFF requestToConfirmedOwner=\(start.duration(to: .now))")
        #expect(app.workspaceSource == canonical); #expect(selectedID != originalID)
        #expect(selected.draftText.isEmpty); #expect(selected.draftUploads.isEmpty); #expect(selected.pendingSubmission == nil)
        #expect(selected.snapshot?.running == false); #expect(selected.snapshot?.hasApproval == false)
        #expect(selected.snapshot?.value["queued"].int == 0); #expect(selected.snapshot?.value["dialogue"].array.isEmpty == true)
        #expect(selected.snapshot?.value["dialogue_task"] == .null); #expect(selected.planOperation == nil)
        #expect(try FileManager.default.contentsOfDirectory(atPath: folder.path).isEmpty)
        #expect(original.draftText == "Original workspace draft"); #expect(original.draftUploads == originalUploads)
        #expect(original.snapshot?.value["source"].string == source); #expect(original.transcript == messages)
        #expect(index.search("Public").matches.allSatisfy { NativeTranscriptSearch.revealTarget($0, scope: scope, messages: original.transcript) == $0.messageID })
        selected.editDraft("Selected workspace draft")
        try await chooseWorkspace(app, alias.path)
        #expect(app.current === selected); #expect(app.ownerCount == 2); #expect(app.workspaceStatus.contains("Already"))
        try await chooseWorkspace(app, another.path)
        let other = app.current, otherID = other.conversationID
        #expect(app.workspaceIssue.isEmpty); #expect(app.ownerCount == 3); #expect(otherID != selectedID)
        #expect(app.workspaceSource == (try workspaceFixtureCanonical(another.path)))
        await app.openConversation(originalID)
        #expect(app.current === original); #expect(app.workspaceSource == source); #expect(original.draftUploads == originalUploads)
        #expect(original.draftText == "Original workspace draft")
        var query = NativeCatalogRequest(.query); query.scope = .all
        let catalog = try await original.nativeCatalog(query)
        #expect(catalog.page?.items.first { $0.id == selectedID }?.source == canonical)
        #expect(catalog.page?.items.first { $0.id == originalID }?.source == source)
        await app.openConversation(selectedID)
        #expect(app.current === selected); #expect(selected.draftText == "Selected workspace draft")
        #expect(await app.shutdown(confirmed: true))
        // Deliberately keep the original runtime directory. The selected source
        // must come from each retained conversation, never an app-wide override.
        var restartConfig = fixture.configuration; restartConfig.conversationID = ""
        let reopened = ManagedSessionCoordinator(configuration: restartConfig, nativeDraftDirectory: fixture.root.appendingPathComponent("native"), profile: fixture.root.path); await reopened.start()
        #expect(reopened.current.conversationID == selectedID); #expect(reopened.workspaceSource == canonical)
        #expect(reopened.current.draftText == "Selected workspace draft"); #expect(reopened.ownerCount == 1)
        reopened.showCatalog(); try await managedEventually("workspace restart catalog") { !reopened.catalog.loading && reopened.catalog.page != nil }
        reopened.catalog.selection = originalID; reopened.catalog.openSelected()
        try await managedEventually("original source after restart") { !reopened.catalog.acting && reopened.current.conversationID == originalID }
        #expect(reopened.workspaceSource == source); #expect(reopened.current.draftText == "Original workspace draft")
        #expect(reopened.current.draftUploads == originalUploads)
        reopened.catalog.dismiss(); #expect(await reopened.shutdown(confirmed: true))
    } catch { _ = await app.shutdown(confirmed: true); throw error }
}

@Test @MainActor func workspaceActualGoRejectsFilesMissingControlCharactersAndRevokedAccessWithoutArchiveChanges() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 1)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let app = fixture.application(), file = fixture.root.appendingPathComponent("ordinary.txt"), denied = fixture.root.appendingPathComponent("denied")
    try Data("private test data".utf8).write(to: file)
    try FileManager.default.createDirectory(at: denied, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o000])
    let listOnly = fixture.root.appendingPathComponent("list-only")
    try FileManager.default.createDirectory(at: listOnly, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o400])
    defer {
        try? FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: denied.path)
        try? FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: listOnly.path)
    }
    await app.start(); let original = app.current
    original.editDraft("Keep rejected input"); #expect(await original.flushDrafts())
    let directory = fixture.root.appendingPathComponent("native")
    let path = try #require(FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil).first { $0.pathExtension == "json" })
    let archive = try Data(contentsOf: path)
    for selection in [file.path, fixture.root.appendingPathComponent("missing").path, denied.path, listOnly.path, fixture.root.path + "\n/source"] {
        try await chooseWorkspace(app, selection)
        #expect(!app.workspaceIssue.isEmpty); #expect(app.current === original); #expect(app.ownerCount == 1)
        #expect(original.draftText == "Keep rejected input"); #expect(try Data(contentsOf: path) == archive)
        #expect(!app.workspaceNeedsReview)
    }
    #expect(await app.shutdown(confirmed: true))
}

final class WorkspaceLaunchMutation: @unchecked Sendable {
    private let lock = NSLock()
    private var count = 0
    private var problem = ""
    let original, moved: URL
    init(_ original: URL) { self.original = original; moved = original.deletingLastPathComponent().appendingPathComponent("moved-before-startup") }
    func next() -> any BridgeTransport {
        lock.withLock {
            count += 1
            if count == 2 {
                do { try FileManager.default.moveItem(at: original, to: moved) }
                catch { problem = error.localizedDescription }
            }
        }
        return GoBridgeConnection()
    }
    var failure: String { lock.withLock { problem } }
}

@Test @MainActor func workspaceActualGoFolderMovedBetweenValidationAndStartupKeepsPreviousOwner() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 1)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let selected = fixture.root.appendingPathComponent("move-before-launch")
    try FileManager.default.createDirectory(at: selected, withIntermediateDirectories: true)
    let mutation = WorkspaceLaunchMutation(selected)
    let app = ManagedSessionCoordinator(configuration: fixture.configuration, store: NativeDraftStore(directory: fixture.root.appendingPathComponent("native"), profile: fixture.root.path), factory: { mutation.next() })
    await app.start(); let original = app.current
    original.editDraft("Do not lose this draft")
    try await chooseWorkspace(app, selected.path)
    #expect(mutation.failure.isEmpty); #expect(FileManager.default.fileExists(atPath: mutation.moved.path))
    #expect(app.current === original); #expect(app.ownerCount == 1); #expect(app.workspaceNeedsReview)
    #expect(!app.workspaceIssue.isEmpty); #expect(original.draftText == "Do not lose this draft")
    #expect(original.snapshot?.running == false); #expect(app.beginWorkspaceChoice() == nil)
    await app.openConversation(original.conversationID)
    #expect(!app.workspaceNeedsReview)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func workspaceActualGoIndependentInstancesKeepDistinctConversationSourcesAndDrafts() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 2)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let folder = fixture.root.appendingPathComponent("shared chosen workspace")
    try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    let first = fixture.application()
    var config = fixture.configuration; config.conversationID = fixture.ids[0]
    let second = ManagedSessionCoordinator(configuration: config, nativeDraftDirectory: fixture.root.appendingPathComponent("native"), profile: fixture.root.path)
    await first.start(); await second.start()
    async let a: Void = chooseWorkspace(first, folder.path)
    async let b: Void = chooseWorkspace(second, folder.path)
    _ = try await (a, b)
    #expect(first.workspaceIssue.isEmpty); #expect(second.workspaceIssue.isEmpty)
    #expect(first.current.conversationID != second.current.conversationID)
    #expect(first.workspaceSource == (try workspaceFixtureCanonical(folder.path)))
    #expect(second.workspaceSource == first.workspaceSource)
    first.current.editDraft("Independent process one"); second.current.editDraft("Independent process two")
    #expect(await first.current.flushDrafts()); #expect(await second.current.flushDrafts())
    let archive = try await NativeDraftStore(directory: fixture.root.appendingPathComponent("native"), profile: fixture.root.path).load()
    #expect(archive.drafts[first.current.context]?.text == "Independent process one")
    #expect(archive.drafts[second.current.context]?.text == "Independent process two")
    #expect(first.current.snapshot?.running == false); #expect(second.current.snapshot?.running == false)
    #expect(await first.shutdown(confirmed: true)); #expect(await second.shutdown(confirmed: true))
}

@Test @MainActor func workspaceActualGoUnicodeNormalizationDoesNotRejectTheConfirmedDestination() async throws {
    let fixture = try CatalogIntegrationFixture.prepare(count: 1)
    defer { try? FileManager.default.removeItem(at: fixture.root) }
    let folder = fixture.root.appendingPathComponent("Folder café 日本").path
    try FileManager.default.createDirectory(atPath: folder, withIntermediateDirectories: true)
    let app = fixture.application(); await app.start()
    let original = app.current
    // Panel strings and filesystem-backed URLs can use different Unicode forms.
    try await chooseWorkspace(app, folder.precomposedStringWithCanonicalMapping)
    #expect(app.workspaceIssue.isEmpty)
    #expect(app.current !== original)
    #expect(app.workspaceSource == (try workspaceFixtureCanonical(folder)))
    let selected = app.current, count = app.ownerCount
    try await chooseWorkspace(app, folder.decomposedStringWithCanonicalMapping)
    #expect(app.workspaceIssue.isEmpty)
    #expect(app.current === selected); #expect(app.ownerCount == count)
    try await chooseWorkspace(app, folder.precomposedStringWithCanonicalMapping)
    #expect(app.workspaceIssue.isEmpty); #expect(app.current === selected); #expect(app.ownerCount == count)
    #expect(await app.shutdown(confirmed: true))
}
