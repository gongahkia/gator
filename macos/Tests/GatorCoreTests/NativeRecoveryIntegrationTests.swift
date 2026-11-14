import Foundation
import Testing
@testable import GatorCore

final class RecoveryGoPool: @unchecked Sendable {
    private let lock = NSLock()
    private var firstConnection: GoBridgeConnection?
    func next() -> any BridgeTransport { lock.withLock { let value = GoBridgeConnection(); if firstConnection == nil { firstConnection = value }; return value } }
    var first: GoBridgeConnection? { lock.withLock { firstConnection } }
}

@Test @MainActor func realGoInterruptedPlanRestoresOriginalIdentityWithoutReplay() async throws {
    let pool = RecoveryGoPool()
    let fixture = try await NativePlanFixture(factory: { pool.next() })
    do {
        let owner = fixture.app.current, plan = try await fixture.prepare()
        await fixture.app.authorizePlan(owner: owner, review: plan, action: .approve)
        try await managedEventually("execution held before interruption") { owner.planOperation?.phase == .working && fixture.trace().contains { $0["kind"].string == "execution" } }
        owner.editDraft("Preserve after interruption")
        let connection = try #require(pool.first), pid = try #require(await connection.processIdentifier())
        #expect(kill(pid, SIGKILL) == 0)
        try await managedEventually("killed Go owner reported disconnected") { owner.connectionStatus == .disconnected }
        let identity = owner.conversationID
        await fixture.app.reconnect()
        try await managedEventually("canonical interrupted task") { owner.snapshot?.value["task_plans"].array.contains { $0["id"].string == plan.identity.taskID && $0["phase"].string == "interrupted" } == true }
        #expect(owner.conversationID == identity); #expect(owner.snapshot?.running == false)
        #expect(owner.recovery?.records.contains { $0.id == "plan:" + plan.identity.taskID && $0.status.contains("Interrupted") } == true)
        #expect(owner.snapshot?.value["request_recoveries"].array.isEmpty == true) // A universal request-recovery list would lose this work.
        #expect(fixture.trace().filter { $0["kind"].string == "execution" }.count == 1)
        #expect(owner.draftText == "Preserve after interruption")
        fixture.app.inspectTask(identity); #expect(fixture.app.inspectedOwner === owner)
        await fixture.app.newConversation(); let other = fixture.app.current
        #expect(other !== owner); #expect(other.isConnected)
        try await fixture.send("A normal unrelated conversation")
        try await managedEventually("unrelated owner still useful") { other.pendingSubmission == nil }
        #expect(owner.snapshot?.running == false)
        await fixture.close()
    } catch { print("Interrupted recovery fixture", fixture.app.issue, fixture.app.current.issue, fixture.app.current.snapshot?.value.pretty ?? ""); await fixture.close(); throw error }
}

@MainActor final class NativeReceiptFixture {
    let root: URL, provider: Process, configuration: BridgeLaunchConfiguration
    var app: ManagedSessionCoordinator
    init() async throws {
        root = try temporaryDirectory(); provider = Process(); let output = Pipe()
        let path = ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin"
        provider.executableURL = URL(fileURLWithPath: "/usr/bin/env"); provider.arguments = ["node", try #require(Bundle.module.url(forResource: "receipt-provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
        provider.environment = ["PATH": path, "GATOR_FIXTURE_CONTROL": root.path]; provider.standardOutput = output; provider.standardError = FileHandle.standardError
        try provider.run(); var line = Data()
        while let byte = try output.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
        let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
        let env = ["PATH": path, "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
        let workspace = root.appendingPathComponent("workspace"); try FileManager.default.createDirectory(at: workspace, withIntermediateDirectories: true)
        let fixtureRoot = root
        let id: String
        do { id = try await Task.detached {
            let setup = Process(), seeded = Pipe()
            setup.executableURL = URL(fileURLWithPath: "/usr/bin/env"); setup.arguments = ["go", "run", "./macos/Tests/GatorCoreTests/Fixtures/recovery-seed", workspace.path, "http://127.0.0.1:\(port)"]
            setup.currentDirectoryURL = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
            setup.environment = env.merging(["HOME": ProcessInfo.processInfo.environment["HOME"] ?? fixtureRoot.path]) { _, new in new }
            setup.standardOutput = seeded; setup.standardError = FileHandle.standardError
            try setup.run(); let data = seeded.fileHandleForReading.readDataToEndOfFile(); setup.waitUntilExit()
            let id = String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines)
            guard setup.terminationStatus == 0, !id.isEmpty else { throw BridgeFailure("Disposable Go receipt seeding failed") }
            return id
        }.value } catch { provider.terminate(); provider.waitUntilExit(); throw error }
        configuration = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"), workspace: workspace, environment: env, conversationID: id)
        app = ManagedSessionCoordinator(configuration: configuration, nativeDraftDirectory: root.appendingPathComponent("native"), profile: "receipt")
        await app.start(); try await managedEventually("real retained receipt") { app.current.isConnected && app.current.recovery?.receipts.first?.identity != nil }
    }
    func trace() -> [JSONValue] { ((try? String(contentsOf: root.appendingPathComponent("receipt-trace.jsonl"), encoding: .utf8)) ?? "").split(separator: "\n").compactMap { try? JSONDecoder().decode(JSONValue.self, from: Data($0.utf8)) } }
    func close() async { _ = await app.shutdown(confirmed: true); if provider.isRunning { provider.terminate(); provider.waitUntilExit() }; try? FileManager.default.removeItem(at: root) }
}

@Test @MainActor func realGoReceiptInspectAndSentCheckNeverSendOrRewriteOriginalEvidence() async throws {
    let fixture = try await NativeReceiptFixture()
    do {
        let app = fixture.app, owner = app.current, receipt = try #require(fixture.app.current.recovery?.receipts.first)
        #expect(fixture.trace().isEmpty)
        #expect(receipt.fields.allSatisfy { !$0.text.contains("PRIVATE_BODY") })
        await app.checkReceipt(owner: owner, receipt: receipt, generation: owner.generation, action: .inspect)
        try await managedEventually("saved receipt result") { owner.receiptOperation?.phase == "confirmed" }
        #expect(owner.receiptOperation?.result["check"]["status"].string == "not_checked"); #expect(fixture.trace().isEmpty)
        await app.checkReceipt(owner: owner, receipt: receipt, generation: owner.generation, action: .checkSent)
        try await managedEventually("real read-only Sent check") { owner.receiptOperation?.phase == "confirmed" && owner.receiptOperation?.result["check"]["status"].string == "found" }
        #expect(fixture.trace().count == 2); #expect(fixture.trace().allSatisfy { $0["method"].string == "GET" })
        #expect(owner.recovery?.receipts.first?.value["Status"].string == "unknown")
        #expect(owner.receiptOperation?.summary.contains("does not establish delivery") == true)
        try Data().write(to: fixture.root.appendingPathComponent("unconfirmed"))
        await app.checkReceipt(owner: owner, receipt: receipt, generation: owner.generation, action: .checkSent)
        try await managedEventually("real unconfirmed lookup distinguished") { owner.receiptOperation?.result["check"]["status"].string == "unconfirmed" }
        #expect(fixture.trace().allSatisfy { $0["method"].string == "GET" })
        await fixture.close()
    } catch { print("Receipt fixture", fixture.app.issue, fixture.app.current.issue, fixture.app.current.receiptOperation as Any); await fixture.close(); throw error }
}

@Test @MainActor func realGoReceiptRejectsForeignRunManifestConversationAndCorrelation() async throws {
    let fixture = try await NativeReceiptFixture()
    do {
        let receipt = try #require(fixture.app.current.recovery?.receipts.first), identity = try #require(receipt.identity)
        #expect(await fixture.app.shutdown(confirmed: true))
        let bridge = GoBridgeConnection()
        fixture.app = ManagedSessionCoordinator(configuration: fixture.configuration, store: NativeDraftStore(directory: fixture.root.appendingPathComponent("native"), profile: "receipt"), factory: { bridge })
        await fixture.app.start(); let owner = fixture.app.current
        let variants = [
            NativeReceiptIdentity(conversationID: "wrong", runID: identity.runID, actionID: identity.actionID, correlationID: identity.correlationID, manifestSHA256: identity.manifestSHA256),
            NativeReceiptIdentity(conversationID: identity.conversationID, runID: "wrong", actionID: identity.actionID, correlationID: identity.correlationID, manifestSHA256: identity.manifestSHA256),
            NativeReceiptIdentity(conversationID: identity.conversationID, runID: identity.runID, actionID: identity.actionID, correlationID: identity.correlationID, manifestSHA256: String(repeating: "d", count: 64)),
            NativeReceiptIdentity(conversationID: identity.conversationID, runID: identity.runID, actionID: identity.actionID, correlationID: String(repeating: "e", count: 32), manifestSHA256: identity.manifestSHA256)
        ]
        for variant in variants {
            let id = UUID().uuidString
            try await bridge.send(.receipt(variant, action: .checkSent, request: id), nonce: UUID().uuidString)
            try await managedEventually("Go rejects foreign receipt identity") { owner.snapshot?.value["receipt_result"]["request_id"].string == id }
            #expect(owner.snapshot?.value["receipt_result"]["error"].string.isEmpty == false)
        }
        #expect(fixture.trace().isEmpty)
        await fixture.close()
    } catch { await fixture.close(); throw error }
}
