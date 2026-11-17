import Foundation
import Testing
@testable import GatorCore

// synchronous fixture gates pause disk/I/O owners, never the main actor.
final class SafetyWriteGate: @unchecked Sendable {
    private let condition = NSCondition()
    private var armed = false, released = false, failed = false, reached = false
    var entered: Bool { condition.lock(); defer { condition.unlock() }; return reached }
    func arm() { condition.lock(); armed = true; released = false; reached = false; condition.unlock() }
    func setFailure(_ value: Bool) { condition.lock(); failed = value; condition.unlock() }
    func release() { condition.lock(); released = true; condition.broadcast(); condition.unlock() }
    func pause() throws {
        condition.lock(); defer { condition.unlock() }
        if armed {
            armed = false; reached = true; condition.broadcast()
            while !released { condition.wait() }
        }
        if failed { throw BridgeFailure("Controlled native storage failure") }
    }
}
actor ClosingSafetyTransport: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    private let output: AsyncStream<BridgeEvent>.Continuation
    private var release: CheckedContinuation<Void, Never>?
    private(set) var startedClosing = false
    var confirmed = true
    init() { let stream = AsyncStream<BridgeEvent>.makeStream(); events = stream.stream; output = stream.continuation }
    func start(_ configuration: BridgeLaunchConfiguration) { output.yield(.state(sampleState())) }
    func send(_ command: BridgeCommand, nonce: String) { output.yield(.acknowledged(nonce)) }
    func shutdown() async { _ = await shutdownConfirmed() }
    func shutdownConfirmed() async -> Bool {
        startedClosing = true
        await withCheckedContinuation { release = $0 }
        if confirmed { output.yield(.disconnected("Controlled owner exited")); output.finish() }
        return confirmed
    }
    func finish(confirmed: Bool = true) { self.confirmed = confirmed; release?.resume(); release = nil; startedClosing = false }
    func emit(_ snapshot: BridgeSnapshot) { output.yield(.state(snapshot)) }
}
func safetyAwait(_ condition: @Sendable () async -> Bool) async throws {
    let deadline = ContinuousClock.now + .seconds(5)
    while !(await condition()) {
        guard ContinuousClock.now < deadline else { throw BridgeFailure("Safety async condition timed out") }
        try await Task.sleep(for: .milliseconds(5))
    }
}
@MainActor func closingSafetyApp(_ root: URL, store: NativeDraftStore, transport: ClosingSafetyTransport, uploads: NativeUploadStore? = nil) async throws -> GatorApplicationState {
    let app = GatorApplicationState(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: store, uploads: uploads, factory: { transport })
    await app.start(); try await waitUntil { app.isConnected }
    #expect(await app.flushDrafts()); return app
}
extension NativeUploadStore {
    func holdForSafetyTest(_ gate: SafetyWriteGate) throws { try gate.pause() }
}

@Test @MainActor func safetyLateEditIsRejectedDuringOwnerShutdownAndAcceptedTextRemainsDurable() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "closing"), transport = ClosingSafetyTransport()
    let app = try await closingSafetyApp(root, store: store, transport: transport)
    app.editDraft("accepted before close")
    let closing = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }
    #expect(app.closing); #expect(!app.canEditDraft)
    app.editDraft("must not be accepted")
    app.setPlanFirst(true); app.setReplyToUpdateID("late-update")
    #expect(app.beginAttachmentImport() == nil)
    #expect(app.draftText == "accepted before close"); #expect(!app.planFirst); #expect(app.draftReplyToUpdateID == nil)
    await transport.finish()
    #expect(await closing.value)
    let final = try await store.load()
    #expect(final.drafts["conversation:one"]?.text == "accepted before close")
    #expect(app.connectionStatus == .closed)
    #expect(await app.shutdown(confirmed: true))
    #expect(!(await transport.startedClosing))
}

@Test @MainActor func safetyConcurrentAcceptedEditJoinsAnAlreadyRunningFlush() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = SafetyWriteGate(); defer { gate.release() }
    let store = NativeDraftStore(directory: root, profile: "flush", writeHook: { stage in if stage == .beforeReplace { try gate.pause() } })
    let transport = ClosingSafetyTransport(), app = try await closingSafetyApp(root, store: store, transport: transport)
    gate.arm(); app.editDraft("first accepted text")
    try await safetyWait { gate.entered }
    let flushing = Task { await app.flushDrafts() }
    app.editDraft("second accepted text during flush")
    gate.release(); #expect(await flushing.value)
    #expect(try await store.load().drafts["conversation:one"]?.text == "second accepted text during flush")
    let closing = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }; await transport.finish()
    #expect(await closing.value)
}

@Test @MainActor func safetyFailedClosingWriteRestoresEditingAndRetriedQuitSavesLatestInput() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = SafetyWriteGate(); defer { gate.release() }
    let store = NativeDraftStore(directory: root, profile: "failed-close", writeHook: { stage in if stage == .beforeReplace { try gate.pause() } })
    let transport = ClosingSafetyTransport(), app = try await closingSafetyApp(root, store: store, transport: transport)
    app.editDraft("initial durable text"); #expect(await app.flushDrafts())
    gate.arm(); gate.setFailure(true); app.editDraft("accepted but delayed input")
    try await safetyWait { gate.entered }
    let closing = Task { await app.shutdown(confirmed: true) }
    try await waitUntil { app.closing }
    #expect(!app.canEditDraft); #expect(!(await transport.startedClosing))
    gate.release(); #expect(!(await closing.value))
    #expect(app.canEditDraft); #expect(app.isConnected); #expect(!app.persistenceIssue.isEmpty)
    #expect(app.draftText == "accepted but delayed input")
    #expect(try await store.load().drafts["conversation:one"]?.text == "initial durable text")
    app.editDraft("further input after aborted quit")
    #expect(app.draftText == "further input after aborted quit")
    #expect(!(await app.flushDrafts()))
    gate.setFailure(false)
    let retry = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }; await transport.finish()
    #expect(await retry.value)
    #expect(try await store.load().drafts["conversation:one"]?.text == "further input after aborted quit")
    #expect(app.persistenceIssue.isEmpty)
}

@Test @MainActor func safetyAllOwnersCloseAtOneBarrierAndFailedPreflightStopsNoOwner() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = SafetyWriteGate(); defer { gate.release() }
    let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "owners", writeHook: { stage in if stage == .beforeReplace { try gate.pause() } })
    let first = SessionFixture(), second = SessionFixture(id: "second"), pool = SessionFixturePool([first, second])
    let app = ManagedSessionCoordinator(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: "/unused"), workspace: root), store: store, factory: { pool.next() })
    await app.start(); let parent = app.current
    parent.editDraft("parent durable"); #expect(await parent.flushDrafts())
    await app.newConversation(); let other = app.current
    other.editDraft("other accepted"); #expect(await other.flushDrafts())
    gate.arm(); gate.setFailure(true); parent.editDraft("parent pending save")
    try await safetyWait { gate.entered }
    let closing = Task { await app.shutdown(confirmed: true) }
    try await waitUntil { app.stopping }
    #expect(parent.closing && other.closing)
    other.editDraft("late edit on second owner")
    #expect(other.draftText == "other accepted")
    gate.release(); #expect(!(await closing.value))
    #expect(!app.stopping); #expect(parent.canEditDraft && other.canEditDraft)
    #expect(!(await first.closed)); #expect(!(await second.closed))
    parent.editDraft("parent after aborted quit")
    gate.setFailure(false)
    #expect(await app.shutdown(confirmed: true))
    let final = try await store.load()
    #expect(final.drafts["conversation:parent"]?.text == "parent after aborted quit")
    #expect(final.drafts["conversation:second"]?.text == "other accepted")
    #expect(await app.shutdown(confirmed: true))
}

@Test @MainActor func safetyQueuedAttachmentImportCannotAcceptFilesAfterClosingBarrier() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = SafetyWriteGate(); defer { gate.release() }
    let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "import-close")
    let uploads = NativeUploadStore(root: root.appendingPathComponent("uploads")), transport = ClosingSafetyTransport()
    let app = try await closingSafetyApp(root, store: store, transport: transport, uploads: uploads)
    app.editDraft("keep accepted text")
    let existing = try uploadFile(root, "existing.md"), incoming = try uploadFile(root, "incoming.md")
    let first = try #require(app.beginAttachmentImport()); await app.importFiles([existing], request: first)
    let accepted = app.draftUploads; #expect(accepted.count == 1); #expect(await app.flushDrafts())
    gate.arm(); let hold = Task { try await uploads.holdForSafetyTest(gate) }
    try await safetyWait { gate.entered }
    let request = try #require(app.beginAttachmentImport())
    let importing = Task { await app.importFiles([incoming], request: request) }
    try await waitUntil { app.attachmentStatus == "Importing private copies…" }
    let closing = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }
    gate.release(); try await hold.value; await importing.value
    #expect(app.draftUploads == accepted); #expect(app.attachmentImport == nil)
    #expect(try FileManager.default.contentsOfDirectory(atPath: root.appendingPathComponent("uploads").path).count == 2)
    await transport.finish(); #expect(await closing.value)
    let final = try await store.load()
    #expect(final.drafts["conversation:one"]?.uploads == accepted)
    #expect(final.drafts["conversation:one"]?.text == "keep accepted text")
}

@Test @MainActor func safetyFinalDisconnectChangesMustPersistBeforeQuitCanSucceed() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let gate = SafetyWriteGate()
    let store = NativeDraftStore(directory: root, profile: "final-event", writeHook: { stage in if stage == .beforeReplace { try gate.pause() } })
    let transport = ClosingSafetyTransport(), app = try await closingSafetyApp(root, store: store, transport: transport)
    app.editDraft("request awaiting admission"); await app.submit()
    #expect(app.pendingSubmission != nil)
    let closing = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }
    gate.setFailure(true); await transport.finish()
    #expect(!(await closing.value)); #expect(!app.persistenceIssue.isEmpty)
    #expect(app.connectionStatus == .disconnected); #expect(app.canEditDraft)
    gate.setFailure(false)
    #expect(await app.shutdown(confirmed: true))
    #expect(try await store.load().pending?.id == app.pendingSubmission?.id)
}

@Test @MainActor func safetyUnconfirmedTransportExitBlocksQuitAndReplacementUntilConfirmed() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "unconfirmed"), transport = ClosingSafetyTransport()
    let app = try await closingSafetyApp(root, store: store, transport: transport)
    app.editDraft("keep through unconfirmed exit")
    let closing = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }; await transport.finish(confirmed: false)
    #expect(!(await closing.value)); #expect(app.connectionStatus == .disconnected)
    #expect(app.canEditDraft); #expect(app.issue.contains("shutdown could not be confirmed"))
    let generation = app.generation
    let reconnecting = Task { await app.reconnect() }
    try await safetyAwait { await transport.startedClosing }; await transport.finish(confirmed: false)
    await reconnecting.value
    #expect(app.generation == generation); #expect(app.connectionStatus == .disconnected)
    app.editDraft("accepted after unsuccessful quit")
    let retry = Task { await app.shutdown(confirmed: true) }
    try await safetyAwait { await transport.startedClosing }; await transport.finish()
    #expect(await retry.value)
    #expect(try await store.load().drafts["conversation:one"]?.text == "accepted after unsuccessful quit")
}
