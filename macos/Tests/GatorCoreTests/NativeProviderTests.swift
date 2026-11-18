import Foundation
import Testing
@testable import GatorCore

func providerRowValue(_ id: String = "openai", stored: Bool = true) -> JSONValue {
    .object(["id": .string(id), "model": .string("fixture-model"), "kind": .string("API key"), "issue": .string(""), "destination": .string("127.0.0.1:9"), "api_version": .string("v1"), "stored": .bool(stored), "ambient": .bool(false), "expired": .bool(false), "configured": .bool(true), "ready": .bool(true), "fields": .array([])])
}
func providerReply(_ request: NativeProviderRequest, error: String = "", confirmed: Bool = true, wrongID: Bool = false) throws -> NativeProviderResult {
    var value: [String: JSONValue] = ["request_id": .string(wrongID ? "stale-request" : request.id), "action": .string(request.action.rawValue), "provider": .string(request.provider), "confirmed": .bool(confirmed), "changed": .bool(confirmed), "conflict": .bool(!error.isEmpty), "error": .string(error)]
    if request.action == .status || request.action == .save || request.action == .remove { value["rows"] = .array([providerRowValue(), providerRowValue("anthropic")]) }
    if request.action == .review || request.action == .review_remove {
        value["review"] = .object(["id": .string("opaque-review"), "provider": .string(request.provider), "operation": .string(request.action == .review_remove ? "remove" : "save"), "model": .string("fixture-model"), "destination": .string("127.0.0.1:9"), "endpoint_mode": .string("retain"), "credential_action": .string(request.replaceKey ? "replace" : "retain"), "sets_defaults": .bool(request.action != .review_remove)])
    }
    return try NativeProviderResult(.object(value))
}

@Test func providerProtocolCarriesSecretOnlyInExplicitSaveAndNeverChat() throws {
    let secret = "P04_NATIVE_INERT_49c3_NEVER_REAL"
    for action in [NativeProviderRequest.Action.status, .review, .review_remove, .cancel, .save, .remove] {
        var request = NativeProviderRequest(action); request.provider = "openai"; request.apiKey = secret; request.form = NativeProviderForm()
        let data = try BridgeCommand.host(.provider(request)).data(nonce: "local-nonce")
        let frame = try JSONDecoder().decode(JSONValue.self, from: data)
        #expect(frame["type"].string == "native_provider"); #expect(frame["text"] == .null); #expect(frame["upload"] == .null)
        #expect((frame["native_provider"]["api_key"].string == secret) == (action == .save))
        #expect(!String(describing: request).contains(secret))
    }
}

@MainActor private final class ProviderTestControl {
    var requests: [NativeProviderRequest.Action] = []
    var fail = false
}

@Test @MainActor func providerReviewSaveRemoveCancelAndFailureRequireFreshExplicitIntent() async throws {
    let setup = NativeProviderSetup()
    let control = ProviderTestControl()
    setup.present { request in control.requests.append(request.action); return try providerReply(request, error: control.fail ? "Configuration changed. Refresh and review." : "") }
    try await waitUntil { !setup.busy && setup.selected != nil }
    #expect(setup.selection == "openai"); #expect(setup.canReview)
    setup.forms["openai"]?.model = "Non-secret form survives cancel"
    #expect(await setup.prepare(replacing: true)); #expect(setup.review?.credentialAction == "replace")
    setup.cancelReview(); #expect(setup.review == nil); #expect(!control.requests.contains(.save))
    #expect(setup.forms["openai"]?.model == "Non-secret form survives cancel")
    #expect(await setup.prepare(replacing: false)); #expect(await setup.save(apiKey: ""))
    #expect(setup.review == nil); #expect(!setup.notice.isEmpty)
    #expect(await setup.prepare(replacing: false, removal: true)); #expect(setup.review?.operation == "remove")
    #expect(await setup.save(apiKey: "")); #expect(control.requests.last == .remove)
    control.fail = true
    let epoch = setup.editorEpoch
    #expect(!(await setup.prepare(replacing: true))); #expect(setup.needsRefresh); #expect(!setup.canReview); #expect(setup.editorEpoch != epoch)
    #expect(!(await setup.save(apiKey: "ignored")))
    control.fail = false; await setup.refresh(); #expect(setup.canReview)
    setup.dismiss(); #expect(!setup.isPresented); #expect(setup.review == nil)
}

@Test @MainActor func providerStaleCompletionDoesNotInstallReviewOrClaimSave() async throws {
    let setup = NativeProviderSetup(), gate = CatalogOwnerOpenGate()
    setup.present { request in
        if request.action == .review { await gate.wait() }
        return try providerReply(request)
    }
    try await waitUntil { setup.selected != nil && !setup.busy }
    let task = Task { await setup.prepare(replacing: true) }
    try await providerWaitForGate(gate)
    setup.selection = "anthropic"; setup.dismiss(); await gate.release()
    _ = await task.value
    #expect(setup.review == nil); #expect(setup.selection == "anthropic")
    setup.present { request in try providerReply(request, wrongID: true) }
    try await waitUntil { setup.needsRefresh && !setup.busy }
    #expect(setup.review == nil); #expect(!setup.issue.isEmpty)
}

@Test @MainActor func providerAcknowledgementWithoutConfirmedStorageIsNotSuccessAndQuitWaitsOperation() async throws {
    let setup = NativeProviderSetup(), gate = CatalogOwnerOpenGate()
    setup.present { request in
        if request.action == .save { await gate.wait() }
        return try providerReply(request, confirmed: request.action != .save)
    }
    try await waitUntil { setup.selected != nil && !setup.busy }
    #expect(await setup.prepare(replacing: true))
    let save = Task { await setup.save(apiKey: "P04_INERT_NOT_REAL") }
    try await providerWaitForGate(gate)
    var finished = false
    let finish = Task { await setup.finishOperation(); finished = true }
    await Task.yield(); #expect(!finished); setup.dismiss()
    #expect(setup.retainsHost)
    await gate.release(); #expect(!(await save.value)); await finish.value
    #expect(finished); #expect(setup.notice.isEmpty); #expect(setup.needsRefresh)
}

private func providerWaitForGate(_ gate: CatalogOwnerOpenGate) async throws {
    let deadline = ContinuousClock.now + .seconds(3)
    while !(await gate.didEnter()) {
        guard ContinuousClock.now < deadline else { throw BridgeFailure("Provider gate was not entered") }
        try await Task.sleep(for: .milliseconds(5))
    }
}

@Test @MainActor func providerManagedOwnerPinNavigationPendingApprovalAndQuitBoundary() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), next = SessionFixture(id: "next"), gate = CatalogOwnerOpenGate()
    let app = await managedFixture(root, [parent, next])
    let original = app.current
    original.editDraft("Provider setup keeps my draft")
    let reviewToken = "exact-owner-approval-token"
    await parent.patch(["running": .bool(true), "approval": .object(["id": .number(77), "token": .string(reviewToken), "description": .string("Inert approval fixture")])])
    try await waitUntil { original.snapshot?.running == true }
    app.showProviders(); try await waitUntil { !app.providers.busy && app.providers.selected != nil }
    #expect(app.ownerCount == 1); #expect(await app.providers.prepare(replacing: true))
    await parent.configureProvider(gate: gate)
    let saving = Task { await app.providers.save(apiKey: "P04_INERT_OWNER_BOUND_KEY") }
    try await providerWaitForGate(gate)
    #expect(app.quitGuard?.contains("provider") == true)
    await app.newConversation()
    #expect(app.current !== original); #expect(app.ownerCount == 2)
    #expect(original.draftText == "Provider setup keeps my draft")
    let quit = Task { await app.shutdown(confirmed: true) }
    try await waitUntil { app.stopping }
    #expect(original.closing); #expect(app.current.closing)
    original.editDraft("Rejected late edit")
    #expect(original.draftText == "Provider setup keeps my draft")
    await gate.release(); #expect(await saving.value); #expect(await quit.value)
    #expect(await parent.sent("approval").isEmpty); #expect(await next.sent("approval").isEmpty)
    #expect(await parent.sent("upload_submit").isEmpty); #expect(await next.sent("native_provider").isEmpty)
    #expect(await parent.sent("native_provider").filter { $0["native_provider"]["action"].string == "save" }.count == 1)
}

@Test @MainActor func providerCompletionAfterSelectionChangeUpdatesStatusWithoutChangingSelection() async throws {
    let setup = NativeProviderSetup(), gate = CatalogOwnerOpenGate()
    setup.present { request in
        if request.action == .save { await gate.wait() }
        return try providerReply(request)
    }
    try await waitUntil { setup.selected != nil && !setup.busy }
    #expect(await setup.prepare(replacing: false))
    let save = Task { await setup.save(apiKey: "") }
    try await providerWaitForGate(gate)
    setup.selection = "anthropic"
    await gate.release(); #expect(await save.value)
    #expect(setup.selection == "anthropic"); #expect(setup.review == nil)
    #expect(setup.notice.hasPrefix("openai:"))
}

@Test @MainActor func providerDisconnectedOriginRequiresReconciliationAndFreshExplicitHostSelection() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let original = SessionFixture(), replacement = SessionFixture(id: "replacement")
    let app = await managedFixture(root, [original, replacement])
    let old = app.current
    old.editDraft("Keep draft after provider endpoint disconnect")
    app.showProviders(); try await waitUntil { !app.providers.busy && app.providers.selected != nil }
    await original.disconnect(); try await waitUntil { old.connectionStatus == .disconnected }
    await app.providers.refresh()
    #expect(app.providers.needsRefresh); #expect(app.quitGuard?.contains("Provider setup needs reconciliation") == true)
    #expect(!app.providers.canReview); #expect(old.draftText == "Keep draft after provider endpoint disconnect")
    let generation = old.generation
    await app.reconnect(); #expect(app.current === old); #expect(old.generation != generation)
    #expect(old.draftText == "Keep draft after provider endpoint disconnect")
    await app.providers.refresh(); #expect(app.providers.needsRefresh)
    app.providers.dismiss(); app.showProviders()
    try await waitUntil { !app.providers.busy && !app.providers.needsRefresh }
    #expect(app.providers.canReview)
    #expect(await original.sent("native_provider").filter { $0["native_provider"]["action"].string == "save" }.isEmpty)
    #expect(await replacement.sent("native_provider").filter { $0["native_provider"]["action"].string == "save" }.isEmpty)
    app.providers.dismiss(); #expect(await app.shutdown(confirmed: true))
}
