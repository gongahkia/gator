import Foundation
import Testing
@testable import GatorCore

actor SessionFixture: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    private let output: AsyncStream<BridgeEvent>.Continuation
    private var state: [String: JSONValue]
    private var frames: [JSONValue] = []
    private var sequence = 0
    private(set) var closed = false
    private(set) var launch: BridgeLaunchConfiguration?
    let failStart: Bool
    let failFocus: Bool
    let silentBootstrap: Bool
    let focusDelay: Duration
    var autoAdmit = true
    init(id: String = "parent", failStart: Bool = false, failFocus: Bool = false, silentBootstrap: Bool = false, focusDelay: Duration = .zero) {
        let stream = AsyncStream<BridgeEvent>.makeStream(); events = stream.stream; output = stream.continuation
        guard case .object(var initial) = sampleState().value else { fatalError() }
        initial["conversation_id"] = .string(id); initial["title"] = .string(id); initial["foreground"] = .bool(false); initial["dialogue_active"] = .bool(true)
        self.state = initial; self.failStart = failStart; self.failFocus = failFocus; self.silentBootstrap = silentBootstrap; self.focusDelay = focusDelay
    }
    func start(_ configuration: BridgeLaunchConfiguration) throws {
        launch = configuration
        if failStart { throw BridgeFailure("Fixture startup failed") }
        if !configuration.conversationID.isEmpty { state["conversation_id"] = .string(configuration.conversationID); state["title"] = .string(configuration.conversationID) }
        publish()
    }
    private func publish() { output.yield(.state(BridgeSnapshot(.object(state)))) }
    func patch(_ values: [String: JSONValue]) { state.merge(values) { _, new in new }; publish() }
    func sent(_ type: String) -> [JSONValue] { frames.filter { $0["type"].string == type } }
    func setAutoAdmit(_ enabled: Bool) { autoAdmit = enabled }
    func disconnect() { output.yield(.disconnected("Fixture lost only this owner")) }
    func send(_ command: BridgeCommand, nonce: String) async throws {
        let frame = try JSONDecoder().decode(JSONValue.self, from: command.data(nonce: nonce)); frames.append(frame)
        switch command {
        case .host(.focus(let focused)):
            if focusDelay != .zero { try await Task.sleep(for: focusDelay) }
            if !failFocus { patch(["foreground": .bool(focused), "session_request": .null]) }
        case .newConversation:
            sequence += 1; patch(["session_request": .object(["sequence": .number(Decimal(sequence)), "new": .bool(true)])])
        case .conversation(let id, _):
            sequence += 1; patch(["session_request": .object(["sequence": .number(Decimal(sequence)), "conversation_id": .string(id)])])
        case let .host(.configure(_, _, peers)):
            patch(["dialogue_peers": .array(peers)])
        case let .host(.bootstrap(parent, turn)):
            patch(["running": .bool(true), "dialogue_task": .object(["parent_id": .string(parent), "turn_id": .string(turn), "conversation_id": state["conversation_id"]!, "state": .string("running"), "run_id": .string("exact-operation"), "can_stop": .bool(true), "can_guide": .bool(true)])])
            if !silentBootstrap { patch(["dialogue_bootstrap_result": .object(["turn_id": .string(turn), "child_id": state["conversation_id"]!])]) }
        case .host(.control(let control)):
            var result: [String: JSONValue] = ["request_id": .string(control.requestID)]
            if control.runID != "exact-operation" { result["error"] = .string("Selected operation changed") }
            else if control.action == .stop {
                result["cancelled_plan"] = .bool(true)
                var task = state["dialogue_task"]!; if case .object(var fields) = task { fields["state"] = .string("stopped"); task = .object(fields) }
                patch(["running": .bool(false), "dialogue_task": task])
            } else if control.action == .revise_plan { result["revising_plan"] = .bool(true) }
            patch(["dialogue_control_result": .object(result)])
        case let .host(.result(turn, _, error, _, _)):
            let turns = state["dialogue"]?.array.map { value -> JSONValue in
                guard value["id"].string == turn, case .object(var fields) = value else { return value }
                fields["state"] = .string(error.isEmpty ? "delegated" : "failed"); return .object(fields)
            } ?? []
            patch(["dialogue_dispatch": .null, "dialogue_busy": .bool(false), "dialogue": .array(turns)])
        case .submission(let id, _, _):
            if autoAdmit { patch(["upload_result": .object(["request_id": .string(id)])]) }
        default: break
        }
        output.yield(.acknowledged(nonce))
    }
    func shutdown() { closed = true; output.yield(.disconnected("Fixture stopped")); output.finish() }
}

final class SessionFixturePool: @unchecked Sendable {
    private let lock = NSLock()
    private var pending: [SessionFixture]
    init(_ fixtures: [SessionFixture]) { pending = fixtures }
    func next() -> any BridgeTransport { lock.withLock { pending.removeFirst() } }
}
@MainActor func managedFixture(_ root: URL, _ fixtures: [SessionFixture], timeout: Duration = .milliseconds(150)) async -> ManagedSessionCoordinator {
    let pool = SessionFixturePool(fixtures)
    let app = ManagedSessionCoordinator(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed"), operationTimeout: timeout, admissionTimeout: .milliseconds(100), factory: { pool.next() })
    await app.start(); return app
}
func dispatchFixture(sequence: Int, kind: String = "task", turn: String = "original-turn", child: String = "child", run: String = "exact-operation", ownerTask: String = "") -> JSONValue {
    .object(["sequence": .number(Decimal(sequence)), "kind": .string(kind), "turn_id": .string(turn), "child_id": .string(child), "target_run": .string(run), "instruction": .string(kind == "stop" ? "" : "Use the smaller scope"), "owner_task": .string(ownerTask)])
}
func waitForFrames(_ fixture: SessionFixture, _ type: String, _ count: Int) async throws {
    let deadline = ContinuousClock.now + .seconds(3)
    while await fixture.sent(type).count < count {
        guard ContinuousClock.now < deadline else { throw BridgeFailure("Missing fixture frame: " + type) }
        try await Task.sleep(for: .milliseconds(5))
    }
}
@MainActor func startChild(_ app: ManagedSessionCoordinator, parent: SessionFixture) async throws {
    await parent.patch(["dialogue_dispatch": dispatchFixture(sequence: 1), "dialogue": .array([.object(["id": .string("original-turn"), "text": .string("Independent work"), "task_chat": .string("child")])])])
    try await waitUntil { app.owners["child"]?.snapshot?.running == true }
    await app.settled()
}

@Test @MainActor func managedSurfacesShareCoordinatorAndSwitchingReusesOriginalOwnerWithDrafts() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second])
    let compact = app, full = app, original = app.current
    original.editDraft("Parent draft")
    await first.patch(["running": .bool(true)])
    try await waitUntil { original.snapshot?.running == true }
    await compact.newConversation()
    #expect(full.current.conversationID == "second"); #expect(app.ownerCount == 2)
    #expect(original.snapshot?.running == true); #expect(await first.closed == false)
    full.current.editDraft("Second draft")
    await app.openConversation("parent")
    #expect(app.current === original); #expect(app.current.draftText == "Parent draft")
    #expect(app.owners["second"]?.draftText == "Second draft")
    #expect(await first.launch?.managed == true); #expect(await second.launch?.bootMode == "new")
    #expect(app.quitGuard != nil); #expect(await app.shutdown() == false)
    #expect(await app.shutdown(confirmed: true))
    let reopened = await managedFixture(root, [SessionFixture()])
    #expect(reopened.current.draftText == "Parent draft")
    let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed")
    #expect(try await store.load().drafts["conversation:second"]?.text == "Second draft")
    #expect(await reopened.shutdown())
}

@Test @MainActor func transitionsAreSerializedAndFailedFocusRestoresOriginalExecution() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), next = SessionFixture(id: "next", focusDelay: .milliseconds(20)), broken = SessionFixture(id: "broken", failFocus: true)
    let app = await managedFixture(root, [first, next, broken])
    await first.patch(["update_conversations": .array([.object(["id": .string("next")]), .object(["id": .string("broken")])]), "running": .bool(true)])
    try await waitUntil { app.current.snapshot?.running == true }
    let one = Task { await app.openConversation("next") }
    try await waitUntil { app.switching }
    let two = Task { await app.openConversation("broken") }
    await one.value; await two.value
    #expect(app.current.conversationID == "next"); #expect(app.issue.contains("confirm"))
    #expect(app.owners["parent"]?.snapshot?.running == true)
    #expect(await first.closed == false)
    #expect(await next.sent("focus_session").last?["foreground"].bool == true)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func hostSequencesAreDeduplicatedAndStaleNavigationCannotStealFocus() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second])
    let request = JSONValue.object(["sequence": .number(1), "new": .bool(true)])
    await first.patch(["session_request": request])
    try await waitUntil { app.current.conversationID == "second" }; await app.settled()
    await first.patch(["session_request": request])
    try await Task.sleep(for: .milliseconds(30)); await app.settled()
    #expect(app.ownerCount == 2); #expect(app.current.conversationID == "second")
    await second.patch(["session_request": .object(["sequence": .number(1), "conversation_id": .string("parent")])])
    try await waitUntil { app.current.conversationID == "parent" }; await app.settled()
    #expect(app.ownerCount == 2)
    #expect(await app.shutdown())
}

@Test @MainActor func dispatchBootstrapsExactChildOnceAndRoutesAllControlsWithoutMovingFocus() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), child = SessionFixture(id: "child"), app = await managedFixture(root, [parent, child])
    try await startChild(app, parent: parent)
    #expect(app.current.conversationID == "parent")
    let bootstrap = try #require(await child.sent("dialogue_bootstrap").first)
    #expect(bootstrap["dialogue_parent"].string == "parent"); #expect(bootstrap["dialogue_turn"].string == "original-turn")
    await parent.patch(["dialogue_dispatch": dispatchFixture(sequence: 1)])
    try await Task.sleep(for: .milliseconds(20)); await app.settled()
    #expect(await child.sent("dialogue_bootstrap").count == 1)
    for (offset, kind) in ["guide", "revise_plan", "revise_action", "stop"].enumerated() {
        await parent.patch(["dialogue_dispatch": dispatchFixture(sequence: offset + 2, kind: kind, turn: "control-\(offset)")])
        try await waitForFrames(parent, "dialogue_result", offset + 2); await app.settled()
        let frame = try #require(await child.sent("dialogue_control").last)
        #expect(frame["dialogue_control"]["parent_id"].string == "parent")
        #expect(frame["dialogue_control"]["turn_id"].string == "original-turn")
        #expect(frame["dialogue_control"]["run_id"].string == "exact-operation")
        #expect(app.current.conversationID == "parent")
    }
    #expect(await parent.sent("dialogue_result").last?["dialogue_result"]["cancelled_plan"].bool == true)
    #expect(await app.shutdown())
}

@Test @MainActor func uncertainBootstrapIsReportedAndNeverReplayedOrEvicted() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), child = SessionFixture(id: "child", silentBootstrap: true), app = await managedFixture(root, [parent, child], timeout: .milliseconds(50))
    try await startChild(app, parent: parent)
    #expect(app.issue.contains("not confirm"))
    #expect(await parent.sent("dialogue_result").last?["dialogue_result"]["error"].string.isEmpty == false)
    await parent.patch(["dialogue_dispatch": dispatchFixture(sequence: 1)])
    try await Task.sleep(for: .milliseconds(30)); await app.settled()
    #expect(await child.sent("dialogue_bootstrap").count == 1)
    #expect(app.quitGuard != nil)
    #expect(app.activity.contains { $0.id == "child" && $0.status.contains("unconfirmed") })
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func staleControlsAndUnknownDispatchReturnErrorsWithoutAlternativeTargets() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), child = SessionFixture(id: "child"), app = await managedFixture(root, [parent, child])
    try await startChild(app, parent: parent)
    await parent.patch(["dialogue_dispatch": dispatchFixture(sequence: 2, kind: "stop", turn: "stale-stop", run: "stale-epoch")])
    try await waitForFrames(parent, "dialogue_result", 2); await app.settled()
    #expect(await parent.sent("dialogue_result").last?["dialogue_result"]["error"].string.contains("changed") == true)
    #expect(app.owners["child"]?.snapshot?.running == true)
    await parent.patch(["dialogue_dispatch": dispatchFixture(sequence: 3, kind: "invented", turn: "unknown")])
    try await waitForFrames(parent, "dialogue_result", 3); await app.settled()
    #expect(await parent.sent("dialogue_result").last?["dialogue_result"]["error"].string.contains("Unknown") == true)
    #expect(await child.sent("dialogue_control").count == 1)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func lateAdmissionStaysWithOriginalDraftAndDisconnectDoesNotAffectOthers() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), replacement = SessionFixture(), app = await managedFixture(root, [first, second, replacement])
    await first.setAutoAdmit(false)
    let original = app.current; original.editDraft("Submitted original"); await app.submit(from: original)
    let id = try #require(original.pendingSubmission?.id)
    await app.newConversation(); app.current.editDraft("Other chat draft")
    original.editDraft("Newer original draft")
    await first.patch(["upload_result": .object(["request_id": .string(id)])])
    try await waitUntil { original.pendingSubmission == nil }
    #expect(app.current.draftText == "Other chat draft"); #expect(original.draftText == "Newer original draft")
    await first.disconnect(); try await waitUntil { original.connectionStatus == .disconnected }
    #expect(app.current.isConnected)
    await app.openConversation("parent"); #expect(app.current === original)
    await app.reconnect()
    #expect(app.current.isConnected); #expect(app.owners["second"]?.isConnected == true)
    #expect(await replacement.sent("upload_submit").isEmpty == true)
    #expect(await replacement.sent("approval").isEmpty == true)
    #expect(app.current.draftText == "Newer original draft")
    #expect(await app.shutdown())
}

@Test @MainActor func capacityNeverEvictsBusyOrReviewOwnersAndWorkingLimitIsAuthoritative() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let fixtures = (0..<5).map { SessionFixture(id: "chat-\($0)") }, app = await managedFixture(root, fixtures)
    for index in 0..<3 {
        await fixtures[index].patch(["running": .bool(true)])
        try await waitUntil { app.current.snapshot?.running == true }
        await app.newConversation()
    }
    #expect(app.ownerCount == 4); #expect(app.workingCount == 3)
    await fixtures[3].patch(["dialogue_active": .bool(false)])
    app.current.editDraft("Cannot start fourth work")
    try await waitUntil { !app.canSubmit }
    await app.newConversation(); #expect(app.ownerCount == 4); #expect(app.issue.contains("Four chats"))
    #expect(await fixtures[0].closed == false)
    await fixtures[0].patch(["running": .bool(false), "pending_approval": .object(["id": .number(1)])])
    try await waitUntil { app.workingCount == 2 }
    await app.newConversation(); #expect(app.ownerCount == 4); #expect(await fixtures[0].closed == false)
    await fixtures[0].patch(["pending_approval": .null, "queued": .number(1)])
    await app.newConversation(); #expect(app.ownerCount == 4); #expect(await fixtures[0].closed == false)
    await fixtures[0].patch(["queued": .number(0)])
    try await Task.sleep(for: .milliseconds(20))
    await app.newConversation(); #expect(app.ownerCount == 4); #expect(await fixtures[0].closed == true)
    #expect(app.current.conversationID == "chat-4")
    #expect(app.activity.contains { $0.id == "chat-0" && $0.needsAttention }) // Retiring a cache does not hide its unread result.
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func backgroundCompletionAndApprovalAttentionNeverStealSelectionOrTokens() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let parent = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [parent, second])
    let owner = app.current
    await parent.patch(["running": .bool(true)])
    try await waitUntil { owner.snapshot?.running == true }
    await app.newConversation()
    let token = String(repeating: "a", count: 64), pending = JSONValue.object(["id": .number(4), "token": .string(token), "message_index": .number(0)])
    await parent.patch(["pending_approval": pending, "messages": .array([.object(["role": .string("Gator"), "result": .object(["kind": .string("approval"), "summary": .string("Original exact action")])])])])
    try await waitUntil { app.activity.contains { $0.id == "parent" && $0.status == "Needs review" } }
    #expect(app.current.conversationID == "second"); #expect(app.quitGuard != nil)
    await app.openConversation("parent"); owner.openApprovalReview()
    let review = try #require(owner.review)
    await app.openConversation("second")
    await app.current.respondToApproval(review, approved: true)
    #expect(await second.sent("approval").isEmpty == true)
    await app.openConversation("parent")
    #expect(owner.review == review); #expect(owner.isCurrentReview(review))
    await owner.respondToApproval(review, approved: true)
    #expect(await parent.sent("approval").last?["approval_token"].string == token)
    await parent.patch(["pending_approval": .null])
    try await waitUntil { !owner.approvalSending }
    await app.openConversation("second")
    await parent.patch(["running": .bool(false), "messages": .array([.object(["activity": .object(["state": .string("completed")])])])])
    try await waitUntil { app.activity.contains { $0.id == "parent" && $0.status == "Completed" } }
    #expect(app.current.conversationID == "second")
    #expect(await app.shutdown())
}

@Test @MainActor func failedOwnerStartupAndBootstrapCreationDoNotCancelOriginalOrReplay() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), broken = SessionFixture(failStart: true), childBroken = SessionFixture(failStart: true)
    let app = await managedFixture(root, [first, broken, childBroken])
    let original = app.current
    await first.patch(["running": .bool(true)])
    try await waitUntil { original.snapshot?.running == true }
    await app.newConversation()
    #expect(app.current === original); #expect(app.ownerCount == 1)
    #expect(original.snapshot?.value["foreground"].bool == true)
    #expect(await first.closed == false); #expect(await broken.closed == true)
    await first.patch(["dialogue_dispatch": dispatchFixture(sequence: 1)])
    try await waitForFrames(first, "dialogue_result", 1); await app.settled()
    #expect(await first.sent("dialogue_result").first?["dialogue_result"]["error"].string.contains("startup") == true)
    await first.patch(["dialogue_dispatch": dispatchFixture(sequence: 1)])
    try await Task.sleep(for: .milliseconds(30)); await app.settled()
    #expect(app.ownerCount == 1); #expect(original.snapshot?.running == true)
    #expect(await first.sent("dialogue_result").count == 1)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func localRevisionUsesOriginalOwnerTaskAndForegroundStopNeverTargetsBackground() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second])
    for (index, kind) in ["revise_plan", "revise_action"].enumerated() {
        await first.patch(["dialogue_dispatch": dispatchFixture(sequence: index + 1, kind: kind, turn: "revision-\(index)", child: "parent", ownerTask: "local-original-request")])
        try await waitForFrames(first, "dialogue_result", index + 1); await app.settled()
        let control = try #require(await first.sent("dialogue_control").last?["dialogue_control"])
        #expect(control["turn_id"].string == "local-original-request")
        #expect(control["request_id"].string == "revision-\(index)")
        #expect(control["run_id"].string == "exact-operation")
        #expect(app.ownerCount == 1)
    }
    await first.patch(["running": .bool(true)])
    await app.newConversation()
    await second.patch(["running": .bool(true)])
    try await waitUntil { app.current.canStop }
    await app.current.stopWork()
    #expect(await second.sent("submit").last?["text"].string == "/cancel")
    #expect(await first.sent("submit").isEmpty == true)
    #expect(app.owners["parent"]?.snapshot?.running == true)
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func independentUncertainAdmissionsPersistTogetherAndRestartNeverReplaysEither() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = SessionFixture(), second = SessionFixture(id: "second"), app = await managedFixture(root, [first, second])
    await first.setAutoAdmit(false); await second.setAutoAdmit(false)
    let original = app.current
    original.editDraft("Original pending text"); await app.submit(from: original)
    let firstID = try #require(original.pendingSubmission?.id)
    await app.newConversation(); app.current.editDraft("Second pending text"); await app.submit(from: app.current)
    let secondID = try #require(app.current.pendingSubmission?.id)
    #expect(await app.shutdown(confirmed: true))
    let savedStore = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "managed")
    let saved = try await savedStore.load()
    #expect(saved.version == 2) // A Tranche 01 binary rejects rather than ignores the new pending identities.
    #expect(saved.pendingByContext?["conversation:parent"]?.id == firstID)
    #expect(saved.pendingByContext?["conversation:second"]?.id == secondID)
    let replacement = SessionFixture(id: "second"), restoredParent = SessionFixture()
    let reopened = await managedFixture(root, [replacement, restoredParent])
    #expect(reopened.current.conversationID == "second")
    #expect(reopened.current.pendingSubmission?.id == secondID)
    #expect(reopened.current.draftText == "Second pending text")
    await replacement.patch(["recent_conversations": .array([.object(["id": .string("parent")])])])
    try await waitUntil { reopened.current.snapshot?.value["recent_conversations"].array.first?["id"].string == "parent" }
    await reopened.openConversation("parent")
    #expect(reopened.current.conversationID == "parent")
    #expect(reopened.current.pendingSubmission?.id == firstID)
    #expect(reopened.current.draftText == "Original pending text")
    #expect(await replacement.sent("upload_submit").isEmpty == true)
    #expect(await restoredParent.sent("upload_submit").isEmpty == true)
    #expect(await reopened.shutdown(confirmed: true))
}
