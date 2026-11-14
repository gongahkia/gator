import Foundation
import Testing
@testable import GatorCore

func receiptBundle(status: String = "unknown") -> JSONValue {
    .object(["RunID": .string("work-one"), "Status": .string("failed"), "ManifestSHA256": .string(String(repeating: "c", count: 64)), "Actions": .array([.object(["ID": .string("action-" + String(repeating: "a", count: 16)), "CorrelationID": .string(String(repeating: "b", count: 32)), "Operation": .string("gmail_send"), "Status": .string(status), "Recipient": .string("venue@example.com"), "Subject": .string("Venue update"), "Preview": .string("PRIVATE")])])])
}
func recoverySnapshot(_ changes: [String: JSONValue] = [:]) -> BridgeSnapshot {
    BridgeSnapshot(changedJSON(sampleState().value, ["conversation_id": .string("parent"), "messages": .array([.object(["role": .string("Result"), "bundle": receiptBundle()])])].merging(changes) { _, new in new }))
}
@Test(arguments: ["interrupted", "failed", "review", "stopped", "completed", "unknown"])
func recoveryStatesRemainDistinct(_ state: String) {
    let label = NativeRecoveryPresentation.status(state)
    #expect(!label.isEmpty)
    if state != "failed" { #expect(!label.contains("Failed")) }
    if state == "unknown" { #expect(label.contains("uncertain")) }
}
@Test func recoveredAttemptsAreDistinctAndNeverGainBundleAuthority() throws {
    let receipt = receiptBundle()["Actions"].array[0]
    let projection = NativeRecoveryProjection(recoverySnapshot(["messages": .array([]), "action_recoveries": .array([.object(["id": .string("attempt-1"), "run_id": .string("run"), "receipt": receipt]), .object(["id": .string("attempt-2"), "run_id": .string("run"), "receipt": receipt])]), "action_recovery_truncated": .bool(true), "action_recovery_issue": .string("Unavailable older receipts")]))
    #expect(projection.receipts.count == 2); #expect(projection.receipts.allSatisfy { $0.identity == nil })
    #expect(projection.notices.count == 2); #expect(projection.unresolved)
    #expect(projection.receipts.flatMap(\.fields).allSatisfy { !$0.text.contains("PRIVATE") })
}
@Test func exactBundleIdentityAndDuplicateProjection() throws {
    let bundle = receiptBundle()
    let p = NativeRecoveryProjection(recoverySnapshot(["work_details": .object(["latest_bundle": bundle])]))
    #expect(p.receipts.count == 1)
    let identity = try #require(p.receipts.first?.identity)
    #expect(identity.conversationID == "parent"); #expect(identity.manifestSHA256 == String(repeating: "c", count: 64))
    let data = try BridgeCommand.receipt(identity, action: .checkSent, request: "request-1").data(nonce: "nonce")
    let frame = try JSONDecoder().decode(JSONValue.self, from: data)
    #expect(frame["receipt"]["correlation_id"] == .string(identity.correlationID)); #expect(frame["receipt"]["action"].string == "check_sent")
}
@Test(arguments: ["executed", "failed", "pending", "denied"])
func ineligibleReceiptsHaveNoCheck(_ status: String) {
    #expect(NativeRecoveryProjection(recoverySnapshot(["messages": .array([.object(["bundle": receiptBundle(status: status)])])])).receipts.first?.identity == nil)
}
@Test func conflictingAndAmbiguousReceiptsFailClosed() {
    let bundle = receiptBundle(), action = bundle["Actions"].array[0]
    let ambiguous = changedJSON(bundle, ["Actions": .array([action, action])])
    #expect(NativeRecoveryProjection(recoverySnapshot(["messages": .array([.object(["bundle": ambiguous])])])).receipts.allSatisfy { $0.identity == nil })
    let p = NativeRecoveryProjection(recoverySnapshot(["work_details": .object(["latest_bundle": changedJSON(bundle, ["Actions": .array([])])])]))
    #expect(p.receipts.allSatisfy { $0.identity == nil }); #expect(!p.notices.isEmpty)
}
@Test func failedRequestGuidanceIsReadOnlyAndPartialQueueIsNotCompletion() {
    let projection = NativeRecoveryProjection(recoverySnapshot(["queue": .array([.object(["id": .string("queued"), "interrupted": .bool(true), "text": .string("Original work")])]), "request_recoveries": .array([.object(["task_id": .string("failed"), "state": .string("failed"), "text": .string("Original objective"), "recovery": .object(["kind": .string("source_access"), "retry_safe": .bool(true), "repair": .string("Repair account"), "needs": .array([.object(["connector_id": .string("google"), "operation": .string("gmail_list")])])])])])]))
    #expect(projection.records.contains { $0.id == "queue:queued" && $0.status.contains("Interrupted") })
    #expect(projection.records.contains { $0.id == "request:failed" && $0.detail.contains("not permission") && $0.fields.contains { $0.text.contains("gmail_list") } })
}

@Test @MainActor func receiptAckIsNotResultDuplicateClicksAndWrongRequestCannotComplete() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let transport = SessionFixture(), app = await managedFixture(root, [transport], admissionTimeout: .seconds(2)), owner = app.current
    await transport.patch(["messages": recoverySnapshot().value["messages"]]); try await waitUntil { owner.recovery?.receipts.count == 1 }
    let receipt = try #require(owner.recovery?.receipts.first)
    await app.checkReceipt(owner: owner, receipt: receipt, generation: owner.generation, action: .inspect)
    try await waitUntil { owner.receiptOperation?.phase == "acknowledged" }
    let request = try #require(owner.receiptOperation?.id)
    await transport.patch(["receipt_result": .object(["request_id": .string("wrong"), "check": .object(["status": .string("found")])])])
    await app.checkReceipt(owner: owner, receipt: receipt, generation: owner.generation, action: .checkSent)
    #expect(await transport.sent("receipt").count == 1); #expect(owner.receiptOperation?.unresolved == true)
    await transport.patch(["receipt_result": .object(["request_id": .string(request), "check": .object(["status": .string("unconfirmed")])])])
    try await waitUntil { owner.receiptOperation?.phase == "confirmed" }
    #expect(owner.receiptOperation?.summary.contains("does not prove") == true)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func receiptDisconnectPersistsUncertaintyNoReplayOrWrongOwnerAndDraftIsolation() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), replacement = SessionFixture(id: "parent")
    let app = await managedFixture(root, [first, second, replacement], admissionTimeout: .seconds(2)), original = app.current
    original.editDraft("original draft")
    await first.patch(["messages": recoverySnapshot().value["messages"]]); try await waitUntil { original.recovery?.receipts.count == 1 }
    let receipt = try #require(original.recovery?.receipts.first), oldGeneration = original.generation
    await app.checkReceipt(owner: original, receipt: receipt, generation: oldGeneration, action: .checkSent)
    try await waitUntil { original.receiptOperation?.phase == "acknowledged" }
    let id = original.receiptOperation?.id
    await app.newConversation(); let other = app.current; other.editDraft("other draft")
    #expect(other !== original)
    await app.checkReceipt(owner: other, receipt: receipt, generation: other.generation, action: .checkSent)
    #expect(await second.sent("receipt").isEmpty)
    await first.disconnect(); try await waitUntil { original.receiptOperation?.phase == "uncertain" }
    #expect(other.isConnected); #expect(app.current === other); #expect(app.quitGuard != nil)
    await app.openConversation(original.conversationID); await app.reconnect()
    #expect(original.generation != oldGeneration); #expect(original.receiptOperation?.id == id)
    #expect(original.receiptUnavailable(receipt, generation: oldGeneration) != nil)
    #expect(await replacement.sent("receipt").isEmpty)
    #expect(original.draftText == "original draft"); #expect(other.draftText == "other draft")
    let archive = try await NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed").load()
    #expect(archive.receiptOperationsByContext?["conversation:parent"]?.id == id)
    #expect(archive.version == 5)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func lateReceiptResultRemainsWithOriginalOwnerWhileAnotherChatIsSelected() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second], admissionTimeout: .seconds(2)), original = app.current
    await first.patch(["messages": recoverySnapshot().value["messages"]]); try await waitUntil { original.recovery?.receipts.count == 1 }
    let receipt = try #require(original.recovery?.receipts.first)
    await app.checkReceipt(owner: original, receipt: receipt, generation: original.generation, action: .checkSent)
    let id = try #require(original.receiptOperation?.id)
    await app.newConversation(); let selected = app.current
    selected.editDraft("Keep newer unrelated draft")
    await second.patch(["receipt_result": .object(["request_id": .string(id), "check": .object(["status": .string("found")])])])
    #expect(selected.receiptOperation == nil)
    await first.patch(["receipt_result": .object(["request_id": .string(id), "check": .object(["status": .string("found"), "provider_id": .string("sent-1")])])])
    try await waitUntil { original.receiptOperation?.phase == "confirmed" }
    #expect(app.current === selected); #expect(selected.draftText == "Keep newer unrelated draft")
    #expect(original.receiptOperation?.result["check"]["provider_id"].string == "sent-1")
    #expect(await app.shutdown(confirmed: true))
}
@Test @MainActor func inspectionNeverTriggersRecoveryMutationOrLookupAndUnresolvedOwnersCannotBeEvicted() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixtures = [SessionFixture(), SessionFixture(id: "second"), SessionFixture(id: "third"), SessionFixture(id: "fourth")]
    let app = await managedFixture(root, fixtures, admissionTimeout: .seconds(2))
    for (i, fixture) in fixtures.enumerated() {
        await fixture.patch(["messages": recoverySnapshot().value["messages"]]); try await waitUntil { app.current.recovery?.unresolved == true }
        app.inspectTask(app.current.conversationID); #expect(app.inspection?.ownerID == app.current.conversationID)
        #expect(await fixture.sent("receipt").isEmpty); #expect(await fixture.sent("request_recovery").isEmpty)
        #expect(app.retainedOwnerExplanation(app.current) != nil)
        if i < 3 { await app.newConversation() }
    }
    let original = app.current
    await app.newConversation()
    #expect(app.ownerCount == 4); #expect(app.current === original); #expect(app.issue.contains("Four chats"))
    for fixture in fixtures { #expect(await fixture.closed == false) }
    #expect(await app.shutdown(confirmed: true))
}
@Test func missingOrMalformedManifestCannotOfferReceiptChecks() {
    for digest in [JSONValue.null, .string(""), .string("bad"), .string(String(repeating: "G", count: 64))] {
        let bundle = changedJSON(receiptBundle(), ["ManifestSHA256": digest])
        #expect(NativeRecoveryProjection(recoverySnapshot(["messages": .array([.object(["bundle": bundle])])])).receipts.first?.identity == nil)
    }
}

@Test func exactTaskIdentityCombinesFailedRequestAndPlanWithoutTextMatching() {
    let recovery: JSONValue = .object(["task_id": .string("same"), "state": .string("failed"), "text": .string("Similar title"), "recovery": .object(["repair": .string("Inspect access")])])
    let plan: JSONValue = .object(["id": .string("same"), "state": .string("failed"), "phase": .string("interrupted"), "task": .string("Different text"), "work_id": .string("work-retained")])
    let projection = NativeRecoveryProjection(recoverySnapshot(["messages": .array([]), "request_recoveries": .array([recovery]), "task_plans": .array([plan, changedJSON(plan, ["id": .string("other"), "task": .string("Similar title")])])]))
    #expect(projection.records.count == 2)
    #expect(projection.records.first?.fields.contains { $0.text.contains("work-retained") } == true)
    #expect(Set(projection.records[0].fields.map(\.id)).count == projection.records[0].fields.count)
}
