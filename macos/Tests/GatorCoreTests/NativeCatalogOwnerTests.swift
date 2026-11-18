import Foundation
import Testing
@testable import GatorCore

actor CatalogOwnerOpenGate {
    private var pending: CheckedContinuation<Void, Never>?
    private var entered = false
    func wait() async { entered = true; await withCheckedContinuation { pending = $0 } }
    func didEnter() -> Bool { entered }
    func release() { pending?.resume(); pending = nil }
}

@Test @MainActor func catalogManagedNonRecentOpenReusesOwnersPreservesDraftsAndNeverReconnectsDisconnectedOwner() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), older = SessionFixture(id: "older")
    let items = [catalogItemValue("parent"), catalogItemValue("older")]
    await parent.configureCatalog(items); await older.configureCatalog(items)
    let app = await managedFixture(root, [parent, older]), original = app.current
    original.editDraft("Unsent parent draft")
    #expect(original.snapshot?.value["recent_conversations"].array.contains { $0["id"].string == "older" } != true)
    app.showCatalog(); try await waitUntil { !app.catalog.loading && app.catalog.page != nil }
    app.catalog.selection = "older"; app.catalog.openSelected()
    try await waitUntil { !app.catalog.acting && app.current.conversationID == "older" }
    #expect(app.ownerCount == 2); #expect(original.draftText == "Unsent parent draft")
    #expect(await parent.sent("conversation").isEmpty)
    let retainedOlder = app.current; retainedOlder.editDraft("Older draft")
    app.catalog.selection = "parent"; app.catalog.openSelected()
    try await waitUntil { !app.catalog.acting && app.current === original }
    #expect(app.ownerCount == 2); #expect(app.current.draftText == "Unsent parent draft")
    await older.disconnect(); try await waitUntil { retainedOlder.connectionStatus == .disconnected }
    let frames = await older.sent("native_catalog").count
    app.catalog.selection = "older"; app.catalog.openSelected()
    try await waitUntil { !app.catalog.acting && app.current === retainedOlder }
    #expect(app.ownerCount == 2); #expect(app.current.draftText == "Older draft"); #expect(app.issue.contains("Reconnect explicitly"))
    #expect(await older.sent("native_catalog").count == frames)
    app.catalog.dismiss(); #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func catalogNativeAPIRejectsReusedIdentityInsteadOfReadingThePreviousResult() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let transport = SessionFixture(); await transport.configureCatalog([catalogItemValue()])
    let app = await managedFixture(root, [transport])
    var query = NativeCatalogRequest(.query)
    _ = try await app.current.nativeCatalog(query)
    query.text = "changed query"
    await #expect(throws: (any Error).self) { try await app.current.nativeCatalog(query) }
    #expect(await transport.sent("native_catalog").count == 1)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func catalogCapacityNeverRetiresOwnersHoldingRecoveryEvidence() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixtures = (0..<5).map { SessionFixture(id: "chat-\($0)") }, app = await managedFixture(root, fixtures)
    for index in 0..<3 {
        await fixtures[index].patch(["requests_issue": .string("Retained uncertain fixture evidence")])
        try await waitUntil { app.current.recovery?.unresolved == true }
        await app.newConversation()
    }
    let original = app.current
    await fixtures[3].configureCatalog([catalogItemValue("older-outside")])
    app.showCatalog(); try await waitUntil { !app.catalog.loading && app.catalog.page != nil }
    app.catalog.selection = "older-outside"; app.catalog.openSelected()
    try await waitUntil { !app.catalog.acting }
    #expect(app.current === original); #expect(app.ownerCount == 4); #expect(app.catalog.issue.contains("Four chats"))
    for fixture in fixtures.prefix(4) { #expect(await fixture.closed == false) }
    app.catalog.dismiss(); #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func catalogClosedDuringOpenValidationCannotStealForeground() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = CatalogOwnerOpenGate(), parent = SessionFixture(), other = SessionFixture(id: "other")
    await parent.configureCatalog([catalogItemValue("older")], openGate: gate)
    let app = await managedFixture(root, [parent, other])
    app.showCatalog(); try await waitUntil { !app.catalog.loading && app.catalog.page != nil }
    app.catalog.selection = "older"; app.catalog.openSelected()
    while !(await gate.didEnter()) { await Task.yield() }
    // Closing the catalog while canonical validation is in flight revokes navigation.
    app.catalog.dismiss(); await gate.release()
    try await waitUntil { !app.catalog.acting }
    #expect(app.current.conversationID == "parent"); #expect(app.ownerCount == 1)
    #expect(app.catalog.openedID == nil); #expect(app.catalog.issue.contains("changed"))
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func catalogQueryCompletesAcrossForegroundSwitchWithoutChangingDraftFocusOrOwners() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = CatalogOwnerOpenGate(), parent = SessionFixture(), other = SessionFixture(id: "other")
    await parent.configureCatalog([catalogItemValue("older")], queryGate: gate)
    let app = await managedFixture(root, [parent, other]), original = app.current
    original.editDraft("Parent unsent text"); app.showCatalog()
    let focus = app.catalog.focusRequest
    while !(await gate.didEnter()) { await Task.yield() }
    await app.newConversation(); #expect(app.current.conversationID == "other")
    app.current.editDraft("Other unsent text"); await gate.release()
    try await waitUntil { !app.catalog.loading && app.catalog.page != nil }
    #expect(app.current.conversationID == "other"); #expect(app.current.draftText == "Other unsent text")
    #expect(original.draftText == "Parent unsent text"); #expect(app.catalog.focusRequest == focus)
    #expect(app.ownerCount == 2); #expect(app.catalog.openedID == nil)
    app.catalog.dismiss(); #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func catalogDestinationVerificationRejectsChangedRecordAndReleasesOnlyTheNewIdleCandidate() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), changed = SessionFixture(id: "older")
    await parent.configureCatalog([catalogItemValue("older")])
    await changed.configureCatalog([catalogItemValue("older", version: String(repeating: "b", count: 64))])
    let app = await managedFixture(root, [parent, changed]), original = app.current
    app.showCatalog(); try await waitUntil { !app.catalog.loading && app.catalog.page != nil }
    app.catalog.selection = "older"; app.catalog.openSelected(); try await waitUntil { !app.catalog.acting }
    #expect(app.current === original); #expect(app.ownerCount == 1); #expect(await changed.closed)
    #expect(app.catalog.issue.contains("changed")); app.catalog.dismiss(); #expect(await app.shutdown(confirmed: true))
}
