import Foundation
import Testing
@testable import GatorCore

actor FixtureTransport: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    let continuation: AsyncStream<BridgeEvent>.Continuation
    private var frames: [JSONValue] = []
    private var starts = 0
    private var closes = 0
    let initial: BridgeSnapshot
    init(initial: BridgeSnapshot = sampleState()) {
        let channel = AsyncStream<BridgeEvent>.makeStream(); events = channel.stream; continuation = channel.continuation; self.initial = initial
    }
    func start(_ configuration: BridgeLaunchConfiguration) { starts += 1; continuation.yield(.state(initial)) }
    func send(_ command: BridgeCommand, nonce: String) throws {
        frames.append(try JSONDecoder().decode(JSONValue.self, from: command.data(nonce: nonce)))
        continuation.yield(.acknowledged(nonce))
    }
    func emit(_ event: BridgeEvent) { continuation.yield(event) }
    func shutdown() { closes += 1; continuation.yield(.disconnected("Fixture closed")); continuation.finish() }
    func sent() -> [JSONValue] { frames }
    func counts() -> (Int, Int) { (starts, closes) }
}

final class FixtureConnections: @unchecked Sendable {
    private let lock = NSLock()
    private var transports: [FixtureTransport]
    init(_ transports: [FixtureTransport]) { self.transports = transports }
    func next() -> any BridgeTransport { lock.withLock { transports.removeFirst() } }
}

func sampleState(_ changes: [String: JSONValue] = [:]) -> BridgeSnapshot {
    var value: [String: JSONValue] = ["conversation_id": .string("one"), "source": .string("/fixture"), "primary": .bool(true), "mode": .string("chat"), "title": .string("One"), "running": .bool(false), "cancelling": .bool(false), "queued": .number(0), "messages": .array([]), "provider": .string("fixture"), "model": .string("model"), "recent_id": .string(String(repeating: "a", count: 64)), "recent_conversations": .array([.object(["id": .string("two"), "title": .string("Two")])]), "composer": .object(["id": .string(String(repeating: "b", count: 64)), "effort": .string("standard"), "models": .array([.object(["provider": .string("fixture"), "model": .string("other")])]), "readiness": .object(["state": .string("configured")])])]
    for (key, value2) in changes { value[key] = value2 }; return BridgeSnapshot(.object(value))
}

@MainActor func waitUntil(_ check: () -> Bool) async throws {
    let deadline = ContinuousClock.now + .seconds(3)
    while !check() { if ContinuousClock.now > deadline { throw BridgeFailure("Test timed out") }; try await Task.sleep(for: .milliseconds(5)) }
}

@MainActor func fixtureApp(_ root: URL, transport: FixtureTransport, timeout: Duration = .seconds(2)) async throws -> GatorApplicationState {
    let app = GatorApplicationState(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "fixture"), responseTimeout: timeout, factory: { transport })
    await app.start(); try await waitUntil { app.isConnected }; return app
}

@Test @MainActor func ackIsNotAdmissionAndLateAcceptancePreservesNewDraft() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = FixtureTransport(), app = try await fixtureApp(root, transport: fixture)
    app.editDraft("Original draft"); await app.submit()
    try await waitUntil { app.pendingSubmission?.phase == .acknowledged }
    let request = try #require(app.pendingSubmission?.id)
    #expect(app.draftText == "Original draft")
    await fixture.emit(.state(sampleState(["upload_result": .object(["request_id": .string("wrong")])])))
    try await Task.sleep(for: .milliseconds(20)); #expect(app.pendingSubmission?.id == request)
    app.editDraft("Newer typing")
    await fixture.emit(.state(sampleState(["running": .bool(true), "upload_result": .object(["request_id": .string(request)])])))
    try await waitUntil { app.pendingSubmission == nil }
    #expect(app.draftText == "Newer typing"); #expect(app.submission?.phase == .running)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func sharedPresentationReferencesUseOneOwnerAndSnapshotsReplace() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = FixtureTransport(), app = try await fixtureApp(root, transport: fixture)
    let compact = app, full = app
    compact.editDraft("Shared draft"); #expect(full.draftText == "Shared draft")
    await full.start(); let counts = await fixture.counts(); #expect(counts.0 == 1)
    let message = JSONValue.object(["role": .string("Gator"), "text": .string("First")])
    await fixture.emit(.state(sampleState(["messages": .array([message])])))
    try await waitUntil { app.messages.count == 1 }
    let id = app.messages[0].id
    await fixture.emit(.state(sampleState(["messages": .array([.object(["role": .string("Gator"), "text": .string("First continued")])])])))
    try await waitUntil { app.messages.first?.text == "First continued" }
    #expect(app.messages.count == 1); #expect(app.messages[0].id == id)
    #expect(await app.shutdown())
}

@Test @MainActor func navigationAndStopRespectBusyOwner() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = FixtureTransport(initial: sampleState(["running": .bool(true)])), app = try await fixtureApp(root, transport: fixture)
    app.editDraft("Keep drafting")
    await app.newConversation(); await app.openConversation("two"); await app.submit()
    #expect(await fixture.sent().isEmpty)
    await app.stopWork()
    let sent = await fixture.sent(); #expect(sent.count == 1); #expect(sent[0]["text"].string == "/cancel")
    #expect(app.snapshot?.running == true); #expect(app.stopping)
    #expect(app.draftText == "Keep drafting")
    #expect(await app.shutdown() == false)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func settingsRequireMatchingActualResultAndReadinessIsConfigurationOnly() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = FixtureTransport(), app = try await fixtureApp(root, transport: fixture)
    app.editDraft("Keep settings draft"); await app.selectModel(provider: "fixture", model: "other")
    let sent = await fixture.sent(), request = try #require(sent.last?["composer"]["request_id"].string)
    #expect(!app.operation.isEmpty); #expect(app.modelLabel.contains("model"))
    await fixture.emit(.state(sampleState(["composer_result": .object(["request_id": .string("wrong")])])))
    try await Task.sleep(for: .milliseconds(20)); #expect(!app.operation.isEmpty)
    await fixture.emit(.state(sampleState(["composer_result": .object(["request_id": .string(request)]), "model": .string("other")])))
    try await waitUntil { app.operation.isEmpty }
    #expect(app.modelLabel == "fixture / other"); #expect(app.draftText == "Keep settings draft")
    await fixture.emit(.state(sampleState(["composer": .object(["readiness": .object(["state": .string("needs_setup"), "issue": .string("Missing configuration")])])])))
    try await waitUntil { !app.canSend }
    #expect(app.sendUnavailableReason == "Missing configuration")
    #expect(await app.shutdown())
}

@Test @MainActor func approvalGuardsExactIdentityAndNeverReplaysOnDisconnect() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let token = String(repeating: "a", count: 64), result = JSONValue.object(["kind": .string("approval"), "preview": .object(["account": .string("fixture"), "body": .string("Exact content")])])
    let pending = JSONValue.object(["id": .number(7), "token": .string(token), "message_index": .number(0)])
    let fixture = FixtureTransport(initial: sampleState(["pending_approval": pending, "messages": .array([.object(["role": .string("Gator"), "result": result])])])), app = try await fixtureApp(root, transport: fixture)
    app.openApprovalReview(); let review = try #require(app.review)
    #expect(app.isCurrentReview(review)); #expect(review.result == result)
    await fixture.emit(.state(sampleState(["pending_approval": .object(["id": .number(7), "token": .string(String(repeating: "b", count: 64)), "message_index": .number(0)])])))
    try await waitUntil { !app.isCurrentReview(review) }
    await app.respondToApproval(review, approved: true); #expect(await fixture.sent().isEmpty)
    await fixture.emit(.disconnected("Lost connection")); try await waitUntil { !app.isConnected }
    await app.respondToApproval(review, approved: false); #expect(await fixture.sent().isEmpty)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func disconnectPersistsUncertainIdentityAndExplicitRetryUsesOriginal() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = FixtureTransport(), app = try await fixtureApp(root, transport: fixture, timeout: .milliseconds(80))
    app.editDraft("Original uncertain draft"); await app.submit()
    let original = try #require(app.pendingSubmission)
    try await waitUntil { app.pendingSubmission?.phase == .uncertain }
    #expect(app.draftText == original.text)
    await app.retryOriginalAdmission()
    let sent = await fixture.sent(); #expect(sent.count == 2)
    #expect(sent.allSatisfy { $0["upload"]["request_id"].string == original.id })
    #expect(await app.shutdown(confirmed: true))
    let replacement = FixtureTransport(), reopened = try await fixtureApp(root, transport: replacement)
    #expect(reopened.pendingSubmission?.id == original.id)
    #expect(reopened.draftText == original.text)
    #expect(await replacement.sent().isEmpty)
    #expect(await reopened.shutdown(confirmed: true))
}

@Test @MainActor func reconnectReplacesOwnerWithoutResendingAdmissionOrApproval() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = FixtureTransport(), second = FixtureTransport(), connections = FixtureConnections([first, second])
    let app = GatorApplicationState(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: NativeDraftStore(directory: root, profile: "reconnect"), factory: { connections.next() })
    await app.start(); try await waitUntil { app.isConnected }
    app.editDraft("Never replay this request"); await app.submit()
    let original = try #require(app.pendingSubmission), generation = app.generation
    await first.emit(.disconnected("Unexpected exit")); try await waitUntil { app.connectionStatus == .disconnected }
    await app.reconnect(); try await waitUntil { app.isConnected }
    #expect(app.generation != generation); #expect(app.pendingSubmission?.id == original.id)
    #expect(app.pendingSubmission?.phase == .uncertain); #expect(await second.sent().isEmpty)
    app.receive(.state(sampleState(["conversation_id": .string("stale-owner")])), generation: generation)
    #expect(app.conversationID == "one")
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func exactApprovalDispatchWaitsForGoAndMissingDetailsCannotApprove() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let token = String(repeating: "c", count: 64), pending = JSONValue.object(["id": .number(8), "token": .string(token), "message_index": .number(0)])
    let fixture = FixtureTransport(initial: sampleState(["pending_approval": pending])), app = try await fixtureApp(root, transport: fixture)
    app.openApprovalReview(); let missing = try #require(app.review)
    await app.respondToApproval(missing, approved: true); #expect(await fixture.sent().isEmpty)
    let result = JSONValue.object(["kind": .string("approval"), "preview": .object(["destination": .string("fixture destination"), "body": .string("Exact reviewed body")])])
    await fixture.emit(.state(sampleState(["pending_approval": pending, "messages": .array([.object(["role": .string("Gator"), "result": result])])])))
    try await waitUntil { app.messages.count == 1 }
    app.openApprovalReview(); let review = try #require(app.review)
    await app.respondToApproval(review, approved: true)
    let sent = await fixture.sent(); #expect(sent.count == 1)
    #expect(sent[0]["approval_token"].string == token); #expect(sent[0]["interaction_id"].int == 8)
    #expect(app.approvalSending) // A nonce acknowledgement alone is not success.
    await fixture.emit(.state(sampleState()))
    try await waitUntil { !app.approvalSending }
    #expect(app.review == nil); #expect(await app.shutdown())
}

@Test @MainActor func rejectedAdmissionAndSettingsKeepDraftAndActualValues() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixture = FixtureTransport(), app = try await fixtureApp(root, transport: fixture)
    app.editDraft("Keep on rejection"); await app.submit()
    let request = try #require(app.pendingSubmission?.id)
    await fixture.emit(.state(sampleState(["upload_result": .object(["request_id": .string(request), "error": .string("Rejected original scope")])])))
    try await waitUntil { app.pendingSubmission == nil }
    #expect(app.draftText == "Keep on rejection"); #expect(app.submission?.phase == .failed)
    await app.selectEffort("high")
    let frames = await fixture.sent(), settingsID = try #require(frames.last?["composer"]["request_id"].string)
    await fixture.emit(.state(sampleState(["composer_result": .object(["request_id": .string(settingsID), "error": .string("Stale model settings")])])))
    try await waitUntil { app.operation.isEmpty }
    #expect(app.issue == "Stale model settings"); #expect(app.snapshot?.composer["effort"].string == "standard")
    #expect(app.draftText == "Keep on rejection"); #expect(await app.shutdown())
}

@Test @MainActor func unreadableDraftStorageDisablesEditingButStillAllowsSafeQuit() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "broken"), original = Data("original malformed private file".utf8)
    try original.write(to: await store.file)
    let fixture = FixtureTransport()
    let app = GatorApplicationState(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: store, factory: { fixture })
    await app.start(); try await waitUntil { app.isConnected }
    #expect(!app.canEditDraft); #expect(!app.canSend); #expect(!app.persistenceIssue.isEmpty)
    #expect(await app.shutdown())
    #expect(try Data(contentsOf: await store.file) == original)
}
