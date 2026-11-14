import Foundation
import Testing
@testable import GatorCore

func failedRecovery(_ changes: [String: JSONValue] = [:], needs: [JSONValue] = []) -> JSONValue {
    changedJSON(.object(["task_id": .string(String(repeating: "a", count: 32)), "token": .string(String(repeating: "b", count: 64)), "state": .string("failed"), "text": .string("Original failed request"), "provider": .string("fixture"), "model": .string("model"), "recovery": .object(["kind": .string(needs.isEmpty ? "credentials" : "source_access"), "retry_safe": .bool(true), "repair": .string("Repair original configuration"), "needs": .array(needs)])]), changes)
}
func accessNeeds() -> [JSONValue] { ["first", "second"].map { .object(["connector_id": .string($0), "kind": .string("http_json"), "operation": .string("fetch")]) } }
func accessReview(_ needs: [JSONValue], picks: [String] = ["alpha", "beta"], ready: Bool = true) -> JSONValue {
    .object(["token": .string(String(repeating: "d", count: 64)), "ready": .bool(ready), "summary": .string("Task only. Local configuration; not a connectivity test."), "connections": .array([.string("alpha"), .string("beta")]), "requirements": .array(needs.enumerated().map { i, need in .object(["need": need, "selected": .string(picks[i]), "choices": .array(["alpha", "beta"].map { id in .object(["id": .string(id), "label": .string(id), "destination": .string("http://127.0.0.1/" + id), "permission": .string("allow"), "operation": .string("fetch")]) })]) })])
}
@MainActor func patchRecovery(_ transport: SessionFixture, owner: GatorApplicationState, value: JSONValue) async throws -> NativeRequestRecovery {
    await transport.patch(["request_recoveries": .array([value])]); try await waitUntil { owner.requestRecoveries.first?.value == value }
    return try #require(owner.requestRecoveries.first)
}
@MainActor func readAccess(_ app: ManagedSessionCoordinator, fixture: SessionFixture, recovery: NativeRequestRecovery, value: JSONValue) async throws {
    await app.reviewRecoveryAccess(owner: app.current, recovery: recovery)
    let request = try #require(await fixture.sent("request_recovery").last?["request_recovery"]["request_id"].string)
    await fixture.patch(["request_recovery_result": .object(["request_id": .string(request), "access_review": value])])
    try await waitUntil { app.current.operation.isEmpty }
}
@Test(arguments: ["token", "interrupted", "unsafe", "completed", "provider"])
@MainActor func recoveryEligibilityFailsClosed(_ variant: String) async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    var value = failedRecovery()
    switch variant {
    case "token": value = changedJSON(value, ["token": .string("")])
    case "unsafe": value = changedJSON(value, ["recovery": .object(["retry_safe": .bool(false)])])
    case "provider": value = changedJSON(value, ["provider": .string("different")])
    default: value = changedJSON(value, ["state": .string(variant)])
    }
    let candidate = try await patchRecovery(fixture, owner: owner, value: value)
    #expect(owner.recoveryUnavailable(candidate, action: .prepareRetry) != nil)
    await app.prepareRecovery(owner: owner, recovery: candidate, action: .prepareRetry)
    #expect(await fixture.sent("request_recovery").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func orderedAccountSelectionInvalidatesOldTokenAndRequiresFreshReadyReview() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let candidate = try await patchRecovery(fixture, owner: owner, value: failedRecovery(needs: accessNeeds()))
    try await readAccess(app, fixture: fixture, recovery: candidate, value: accessReview(accessNeeds()))
    #expect(owner.recoveryAccess[candidate.id]?.ready == true)
    #expect(!owner.proposeRecoveryAccount(candidate, index: 0, account: "invented"))
    #expect(owner.proposeRecoveryAccount(candidate, index: 0, account: "beta"))
    #expect(owner.recoveryAccess[candidate.id]?.review == nil)
    #expect(owner.recoveryUnavailable(candidate, action: .prepareAccess) != nil)
    await app.reviewRecoveryAccess(owner: owner, recovery: candidate, proposed: true)
    let frame = try #require(await fixture.sent("request_recovery").last?["request_recovery"])
    #expect(frame["connections"].array.map(\.string) == ["beta", "beta"]) // positional duplicates must survive
    await fixture.patch(["request_recovery_result": .object(["request_id": frame["request_id"], "access_review": accessReview(accessNeeds(), picks: ["beta", "beta"], ready: false)])])
    try await waitUntil { owner.operation.isEmpty }
    #expect(owner.recoveryUnavailable(candidate, action: .prepareAccess) != nil)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func recoveryAdmissionNeedsExactResultAndPreparedPlanNeverImpliesExecution() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let candidate = try await patchRecovery(fixture, owner: owner, value: failedRecovery())
    let first = Task { await app.prepareRecovery(owner: owner, recovery: candidate, action: .prepareRetry) }
    try await waitUntil { owner.recoveryPreparation?.phase == "acknowledged" }
    let pending = try #require(owner.recoveryPreparation)
    let duplicate = Task { await app.prepareRecovery(owner: owner, recovery: candidate, action: .prepareRetry) }
    await fixture.patch(["request_recovery_result": .object(["request_id": .string("wrong"), "accepted": .bool(true)])])
    #expect(owner.recoveryPreparation?.phase == "acknowledged")
    await fixture.patch(["request_recovery_result": .object(["request_id": .string(pending.id), "accepted": .bool(true)])])
    await first.value; await duplicate.value
    #expect(owner.recoveryPreparation?.phase == "accepted"); #expect(await fixture.sent("request_recovery").count == 1)
    let plan = changedJSON(planSummary(), ["id": .string(candidate.id), "revision": .number(1)])
    await fixture.patch(["task_plans": .array([plan]), "running": .bool(false)])
    try await waitUntil { owner.recoveryPreparation?.phase == "plan_ready" }
    #expect(await fixture.sent("task_plan").isEmpty)
    #expect(owner.snapshot?.running == false); #expect(owner.recoveryPreparation?.status.contains("approve separately") == true)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func accessPreparationCarriesReviewedPositionalChoicesAndInvalidatesPlanReads() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let candidate = try await patchRecovery(fixture, owner: owner, value: failedRecovery(needs: accessNeeds()))
    try await readAccess(app, fixture: fixture, recovery: candidate, value: accessReview(accessNeeds(), picks: ["beta", "alpha"]))
    let sending = Task { await app.prepareRecovery(owner: owner, recovery: candidate, action: .prepareAccess) }
    try await waitUntil { owner.recoveryPreparation?.phase == "acknowledged" }
    let frame = try #require(await fixture.sent("request_recovery").last?["request_recovery"])
    #expect(frame["connections"].array.map(\.string) == ["beta", "alpha"])
    #expect(frame["access_token"].string == String(repeating: "d", count: 64))
    await fixture.patch(["request_recovery_result": .object(["request_id": frame["request_id"], "error": .string("account access changed; review again")])])
    await sending.value
    #expect(owner.recoveryPreparation?.phase == "rejected"); #expect(owner.recoveryAccess[candidate.id]?.review == nil)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func changedRecoveryAndOwnerGenerationInvalidateAccessAndPendingNeverReplays() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), replacement = SessionFixture(id: "parent")
    let app = await managedFixture(root, [first, second, replacement], admissionTimeout: .seconds(2)), owner = app.current
    let original = try await patchRecovery(first, owner: owner, value: failedRecovery())
    let changed = try await patchRecovery(first, owner: owner, value: failedRecovery(["token": .string(String(repeating: "e", count: 64))]))
    #expect(!owner.isCurrentRecovery(original))
    owner.editDraft("Original draft")
    let sending = Task { await app.prepareRecovery(owner: owner, recovery: changed, action: .prepareRetry) }
    try await waitUntil { owner.recoveryPreparation?.phase == "acknowledged" }
    let id = owner.recoveryPreparation?.id
    await first.disconnect(); await sending.value
    #expect(owner.recoveryPreparation?.phase == "uncertain"); #expect(app.quitGuard != nil)
    await app.reconnect()
    #expect(owner.recoveryPreparation?.id == id); #expect(!owner.isCurrentRecovery(changed))
    #expect(await replacement.sent("request_recovery").isEmpty)
    let archive = try await NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed").load()
    #expect(archive.version == 6); #expect(archive.recoveryPreparationsByContext?["conversation:parent"]?.id == id)
    #expect(owner.draftText == "Original draft")
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func reorderedOrStaleAccessResultCannotSupplyPreparationAuthority() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture], admissionTimeout: .seconds(2)), owner = app.current
    let recovery = try await patchRecovery(fixture, owner: owner, value: failedRecovery(needs: accessNeeds()))
    try await readAccess(app, fixture: fixture, recovery: recovery, value: accessReview(Array(accessNeeds().reversed())))
    #expect(owner.recoveryAccess[recovery.id]?.review == nil)
    try await readAccess(app, fixture: fixture, recovery: recovery, value: accessReview(accessNeeds()))
    let access = try #require(owner.recoveryAccess[recovery.id]); #expect(owner.isCurrentRecoveryAccess(access))
    await fixture.patch(["preferences_revision": .number(2)])
    try await waitUntil { owner.snapshot?.value["preferences_revision"].int == 2 }
    #expect(!owner.isCurrentRecoveryAccess(access)); #expect(owner.recoveryUnavailable(recovery, action: .prepareAccess) != nil)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func recoveryFinalAdmissionRechecksCapacityAndLeavesNoSendOnFailure() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = SessionFixture(), app = await managedFixture(root, [fixture]), owner = app.current
    let recovery = try await patchRecovery(fixture, owner: owner, value: failedRecovery())
    await owner.prepareRecovery(recovery, action: .prepareRetry, finalAdmissionAllowed: { false })
    #expect(owner.recoveryPreparation?.phase == "not_submitted"); #expect(await fixture.sent("request_recovery").isEmpty)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func lateRecoveryResultStaysWithOriginalOwnerAndCannotClearAnotherDraft() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second], admissionTimeout: .seconds(1)), original = app.current
    let recovery = try await patchRecovery(first, owner: original, value: failedRecovery())
    original.editDraft("Original draft")
    let sending = Task { await app.prepareRecovery(owner: original, recovery: recovery, action: .prepareRetry) }
    try await waitUntil { original.recoveryPreparation?.phase == "acknowledged" }
    let id = try #require(original.recoveryPreparation?.id)
    await sending.value
    #expect(original.recoveryPreparation?.phase == "uncertain")
    await app.newConversation(); let selected = app.current
    selected.editDraft("Independent newer draft")
    await second.patch(["request_recovery_result": .object(["request_id": .string(id), "accepted": .bool(true)])])
    #expect(selected.recoveryPreparation == nil); #expect(original.recoveryPreparation?.phase == "uncertain")
    await app.prepareRecovery(owner: selected, recovery: recovery, action: .prepareRetry)
    #expect(await second.sent("request_recovery").isEmpty)
    await first.patch(["request_recovery_result": .object(["request_id": .string(id), "accepted": .bool(true)])])
    try await waitUntil { original.recoveryPreparation?.phase == "accepted" }
    #expect(app.current === selected); #expect(selected.draftText == "Independent newer draft"); #expect(original.draftText == "Original draft")
    #expect(app.quitGuard != nil)
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func recoveryPreparationCannotBypassThreeWorkingOwners() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixtures = [SessionFixture(), SessionFixture(id: "second"), SessionFixture(id: "third"), SessionFixture(id: "fourth")]
    let app = await managedFixture(root, fixtures, admissionTimeout: .seconds(2)), original = app.current
    let recovery = try await patchRecovery(fixtures[0], owner: original, value: failedRecovery())
    for fixture in fixtures.dropFirst() {
        await app.newConversation(); await fixture.patch(["running": .bool(true)])
        try await waitUntil { app.current.snapshot?.running == true }
    }
    await app.openConversation(original.conversationID)
    #expect(app.current === original); #expect(app.workingCount == 3)
    #expect(app.recoveryUnavailable(owner: original, recovery: recovery, action: .prepareRetry)?.contains("Three tasks") == true)
    await app.prepareRecovery(owner: original, recovery: recovery, action: .prepareRetry)
    #expect(await fixtures[0].sent("request_recovery").isEmpty)
    #expect(app.workingCount == 3); #expect(await app.shutdown(confirmed: true))
}
