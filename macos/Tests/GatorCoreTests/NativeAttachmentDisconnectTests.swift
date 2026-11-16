import Foundation
import Testing
@testable import GatorCore

/// Controlled loss of the upload result after real Go admission. Nonces and
/// other snapshots still flow; this is not a fake provider or admission result.
actor LostUploadResultBridge: BridgeTransport {
    nonisolated let events: AsyncStream<BridgeEvent>
    private let output: AsyncStream<BridgeEvent>.Continuation
    let go = GoBridgeConnection()
    private var task: Task<Void, Never>?
    private var request = ""
    init() { let stream = AsyncStream<BridgeEvent>.makeStream(); events = stream.stream; output = stream.continuation }
    func start(_ config: BridgeLaunchConfiguration) async throws {
        task = Task { for await event in go.events { self.forward(event) } }
        try await go.start(config)
    }
    private func forward(_ event: BridgeEvent) {
        if case .state(let state) = event, !request.isEmpty, state.value["upload_result"]["request_id"].string == request, case .object(var fields) = state.value {
            fields.removeValue(forKey: "upload_result"); output.yield(.state(BridgeSnapshot(.object(fields)))); return
        }
        output.yield(event)
    }
    func send(_ command: BridgeCommand, nonce: String) async throws {
        if case .submission(let id, _, _, _, _, _) = command { request = id }
        try await go.send(command, nonce: nonce)
    }
    func interrupt() async throws { let pid = try #require(await go.processIdentifier()); #expect(kill(pid, SIGKILL) == 0) }
    func shutdown() async { await go.shutdown(); task?.cancel(); output.finish() }
}
@Test @MainActor func realGoLostAttachmentAdmissionRetainsOriginalIdentityAndNeverReplaysAfterRestart() async throws {
    let f = try await NativeAttachmentFixture(), lost = LostUploadResultBridge()
    do {
        #expect(await f.app.shutdown())
        f.app = ManagedSessionCoordinator(configuration: f.configuration, store: NativeDraftStore(directory: f.root.appendingPathComponent("native"), profile: "attachment-go"), admissionTimeout: .milliseconds(300), factory: { lost })
        await f.app.start(); await f.app.current.selectWorkMode(.inspect)
        let owner = f.app.current
        try await attach(owner, [try uploadFile(f.root)]); let files = owner.draftUploads
        owner.editDraft("Hold original task")
        await f.app.submit(from: owner)
        let pending = try #require(owner.pendingSubmission)
        #expect(pending.phase == .uncertain)
        try await managedEventually("actual Go admitted copied input") { f.trace().contains { $0["kind"].string == "received" && $0.pretty.contains("Original jade heron") } }
        let id = owner.conversationID; #expect(!id.isEmpty)
        try await lost.interrupt(); try await managedEventually("original process disconnect") { owner.connectionStatus == .disconnected }
        #expect(owner.pendingSubmission?.id == pending.id); #expect(owner.draftUploads == files)
        #expect(await f.app.shutdown(confirmed: true))
        let calls = f.trace().filter { $0["kind"].string == "received" }.count
        let pool = f.pool
        f.app = ManagedSessionCoordinator(configuration: f.configuration, store: NativeDraftStore(directory: f.root.appendingPathComponent("native"), profile: "attachment-go"), factory: { pool.next() })
        await f.app.start(); #expect(f.app.current.conversationID == id)
        #expect(f.app.current.pendingSubmission?.id == pending.id); #expect(f.app.current.pendingSubmission?.context == pending.context)
        #expect(f.app.current.draftUploads == files)
        try await Task.sleep(for: .milliseconds(200))
        #expect(f.trace().filter { $0["kind"].string == "received" }.count == calls)
        let journal = try String(contentsOf: f.root.appendingPathComponent("state/gator/conversations/" + id + "/requests.json"), encoding: .utf8)
        #expect(journal.contains(pending.id)); #expect(journal.contains(files[0].id))
        #expect(f.app.current.quitGuard != nil)
        await f.close()
    } catch { print("Lost admission fixture:", f.app.issue, f.app.current.issue); await f.close(); throw error }
}
