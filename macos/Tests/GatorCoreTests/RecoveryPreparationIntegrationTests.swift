import Foundation
import Testing
@testable import GatorCore

/// Setup only: retain the original source selection using existing Go commands.
func seedRecoveryScope(runtime: URL, workspace: URL, environment: [String: String]) throws -> String {
    let process = Process(), input = Pipe(), output = Pipe()
    process.executableURL = runtime; process.arguments = ["ui-backend", "", ""]; process.currentDirectoryURL = workspace
    process.environment = environment.merging(["GATOR_UI_BACKEND": "1"]) { _, new in new }
    process.standardInput = input; process.standardOutput = output; process.standardError = FileHandle.standardError
    try process.run(); defer { if process.isRunning { process.terminate(); process.waitUntilExit() } }
    for text in ["/mode inspect", "/connector add reports", "Retain fixture source selection"] {
        var frame = try JSONEncoder().encode(JSONValue.object(["version": .number(1), "type": .string("submit"), "text": .string(text)])); frame.append(10)
        try input.fileHandleForWriting.write(contentsOf: frame)
    }
    var framer = BridgeFramer()
    while process.isRunning {
        let data = output.fileHandleForReading.availableData; if data.isEmpty { break }
        for event in try framer.append(data) {
            if case .state(let snapshot) = event, snapshot.messages.contains(where: { $0.text == "Recovery fixture scope retained." }) {
                try input.fileHandleForWriting.write(contentsOf: Data("{\"version\":1,\"type\":\"exit\"}\n".utf8))
                try input.fileHandleForWriting.close(); process.waitUntilExit(); return snapshot.conversationID
            }
        }
    }
    throw BridgeFailure("Could not seed disposable original source scope")
}
@MainActor final class NativePreparationFixture {
    let root: URL, provider: Process, configuration: BridgeLaunchConfiguration
    let pool = RecoveryGoPool()
    var app: ManagedSessionCoordinator
    init(access: Bool = false) async throws {
        root = try temporaryDirectory(); provider = Process(); let output = Pipe()
        let path = ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin"
        provider.executableURL = URL(fileURLWithPath: "/usr/bin/env"); provider.arguments = ["node", try #require(Bundle.module.url(forResource: "recovery-provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
        provider.environment = ["PATH": path, "GATOR_FIXTURE_CONTROL": root.path]; provider.standardOutput = output; provider.standardError = FileHandle.standardError
        try provider.run(); var line = Data()
        while let byte = try output.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
        let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
        let config = root.appendingPathComponent("config/gator"), workspace = root.appendingPathComponent("workspace")
        try FileManager.default.createDirectory(at: config, withIntermediateDirectories: true); try FileManager.default.createDirectory(at: workspace, withIntermediateDirectories: true)
        var settings: [String: JSONValue] = ["version": .number(4), "defaults": .object(["provider": .string("fixture"), "model": .string("fixture")]), "custom_providers": .array([.object(["id": .string("fixture"), "base_url": .string("http://127.0.0.1:\(port)/chat"), "models": .array([.string("fixture")]), "default_model": .string("fixture")])])]
        if access { settings["connectors"] = .array(["reports", "alpha", "beta"].map { id in .object(["version": .number(1), "id": .string(id), "name": .string(id.capitalized + " local account"), "kind": .string("http_json"), "resource": .string("http://127.0.0.1:\(port)/source/" + id), "authentication": .string("none")]) }) }
        try JSONEncoder().encode(JSONValue.object(settings)).write(to: config.appendingPathComponent("config.json"))
        let env = ["PATH": path, "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
        let runtime = URL(fileURLWithPath: ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime")
        let id: String
        do { id = access ? try await Task.detached { try seedRecoveryScope(runtime: runtime, workspace: workspace, environment: env) }.value : "" }
        catch { provider.terminate(); provider.waitUntilExit(); throw error }
        configuration = BridgeLaunchConfiguration(runtime: runtime, workspace: workspace, environment: env, conversationID: id)
        let pool = pool
        app = ManagedSessionCoordinator(configuration: configuration, store: NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "recovery"), factory: { pool.next() })
        await app.start(); try await managedEventually("preparation fixture ready") { app.current.isConnected }
        await app.current.selectWorkMode(.inspect)
    }
    func fail(access: Bool = false) async throws -> NativeRequestRecovery {
        if access { try await cli(["provider", "connector", "remove", "reports", "--yes"]) }
        app.current.editDraft("Summarize the original input"); try await managedEventually("original composer ready") { app.canSubmit }
        await app.submit(from: app.current)
        try await managedEventually("real recoverable failure") { !app.current.requestRecoveries.isEmpty && app.current.snapshot?.running == false }
        return try #require(app.current.requestRecoveries.first)
    }
    func cli(_ args: [String]) async throws {
        let config = configuration
        try await Task.detached {
            let command = Process(); command.executableURL = config.runtime; command.arguments = args
            command.environment = config.environment.merging(["GATOR_INTERNAL_COMMANDS": "1"]) { _, new in new }
            command.standardOutput = FileHandle.nullDevice; command.standardError = FileHandle.standardError
            try command.run(); command.waitUntilExit(); guard command.terminationStatus == 0 else { throw BridgeFailure("Fixture CLI setup failed") }
        }.value
    }
    func trace() -> [JSONValue] { ((try? String(contentsOf: root.appendingPathComponent("recovery-trace.jsonl"), encoding: .utf8)) ?? "").split(separator: "\n").compactMap { try? JSONDecoder().decode(JSONValue.self, from: Data($0.utf8)) } }
    func mark(_ name: String) throws { try Data().write(to: root.appendingPathComponent(name)) }
    func inspect(_ task: String) async throws -> NativePlanReview {
        let owner = app.current, summary = try #require(owner.snapshot?.value["task_plans"].array.first { $0["id"].string == task })
        let identity = try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: summary))
        await app.loadPlan(identity, owner: owner)
        try await managedEventually("fresh recovery plan review") { owner.planReviews[task]?.identity == identity && owner.operation.isEmpty }
        return try #require(owner.planReviews[task])
    }
    func close() async { try? mark("release-preparation"); _ = await app.shutdown(confirmed: true); if provider.isRunning { provider.terminate(); provider.waitUntilExit() }; try? FileManager.default.removeItem(at: root) }
}

@Test @MainActor func realGoPrepareRetryAdmitsOnlyProposalAndSeparateApprovalExecutesExactlyOnce() async throws {
    let f = try await NativePreparationFixture()
    do {
        let owner = f.app.current, recovery = try await f.fail()
        #expect(recovery.retrySafe); #expect(recovery.needs.isEmpty)
        owner.editDraft("Retain unrelated draft")
        try f.mark("repaired"); try f.mark("hold-preparation")
        await f.app.prepareRecovery(owner: owner, recovery: recovery, action: .prepareRetry)
        try await managedEventually("Go admission and held preparation") { ["accepted", "preparing"].contains(owner.recoveryPreparation?.phase ?? "") && f.trace().contains { $0["kind"].string == "preparation" } }
        #expect(f.trace().filter { $0["kind"].string == "execution" }.isEmpty)
        #expect(owner.planReviews.isEmpty)
        try f.mark("release-preparation")
        try await managedEventually("actual revised proposal") { owner.recoveryPreparation?.phase == "plan_ready" && owner.snapshot?.running == false }
        let review = try await f.inspect(recovery.id)
        #expect(f.trace().filter { $0["kind"].string == "execution" }.isEmpty)
        await f.app.authorizePlan(owner: owner, review: review, action: .approve)
        try await managedEventually("separately approved recovery completes") { owner.planOperation?.phase == .completed }
        #expect(f.trace().filter { $0["kind"].string == "execution" }.count == 1)
        #expect(owner.draftText == "Retain unrelated draft"); #expect(!owner.isCurrentRecovery(recovery))
        await f.close()
    } catch { print("Retry preparation fixture", f.app.issue, f.app.current.issue, f.app.current.recoveryPreparation as Any); await f.close(); throw error }
}
@Test @MainActor func realGoAccessSelectionRevalidatesStaleScopeAndPreparesWithoutWrites() async throws {
    let f = try await NativePreparationFixture(access: true)
    do {
        let owner = f.app.current, recovery = try await f.fail(access: true)
        #expect(!recovery.needs.isEmpty); #expect(f.trace().filter { $0["kind"].string == "failure" || $0["kind"].string == "execution" }.isEmpty)
        await f.app.reviewRecoveryAccess(owner: owner, recovery: recovery)
        try await managedEventually("actual access requirements") { owner.recoveryAccess[recovery.id]?.review != nil && owner.operation.isEmpty }
        #expect(owner.recoveryAccess[recovery.id]?.ready == false)
        #expect(owner.proposeRecoveryAccount(recovery, index: 0, account: "beta"))
        #expect(owner.recoveryAccess[recovery.id]?.review == nil)
        await f.app.reviewRecoveryAccess(owner: owner, recovery: recovery, proposed: true)
        try await managedEventually("replacement account ready from Go") { owner.recoveryAccess[recovery.id]?.ready == true && owner.operation.isEmpty }
        let configURL = f.root.appendingPathComponent("config/gator/config.json"), original = try Data(contentsOf: configURL)
        var config = try JSONDecoder().decode(JSONValue.self, from: original)
        let connections = config["connectors"].array.map { d in d["id"].string == "beta" ? changedJSON(d, ["resource": .string(d["resource"].string + "-changed")]) : d }
        config = changedJSON(config, ["connectors": .array(connections)]); try JSONEncoder().encode(config).write(to: configURL)
        await f.app.prepareRecovery(owner: owner, recovery: recovery, action: .prepareAccess)
        try await managedEventually("Go rejects old access token") { owner.recoveryPreparation?.phase == "rejected" }
        #expect(owner.recoveryPreparation?.error.contains("access changed") == true)
        #expect(owner.recoveryAccess[recovery.id]?.review == nil); #expect(f.trace().filter { $0["kind"].string == "preparation" }.isEmpty)
        try original.write(to: configURL)
        await f.app.reviewRecoveryAccess(owner: owner, recovery: recovery, proposed: true)
        try await managedEventually("fresh reviewed configured access") { owner.recoveryAccess[recovery.id]?.ready == true && owner.operation.isEmpty }
        try f.mark("repaired")
        await f.app.prepareRecovery(owner: owner, recovery: recovery, action: .prepareAccess)
        try await managedEventually("same original task revised with reviewed access") { owner.recoveryPreparation?.phase == "plan_ready" }
        #expect(owner.recoveryPreparation?.connections == ["beta"])
        let review = try await f.inspect(recovery.id)
        #expect(review.value["access_changes"].array.count > 0)
        #expect(f.trace().filter { $0["kind"].string == "execution" || $0["kind"].string == "source" }.isEmpty)
        await f.close()
    } catch { print("Access preparation fixture", f.app.issue, f.app.current.issue, f.app.current.recoveryPreparation as Any, f.app.current.snapshot?.value["request_recoveries"].pretty ?? ""); await f.close(); throw error }
}
@Test @MainActor func realGoPreparationInterruptionRetainsPendingIdentityAndNeverReplays() async throws {
    let f = try await NativePreparationFixture()
    do {
        let owner = f.app.current, recovery = try await f.fail()
        try f.mark("repaired"); try f.mark("hold-preparation")
        owner.editDraft("Keep after uncertain recovery")
        await f.app.prepareRecovery(owner: owner, recovery: recovery, action: .prepareRetry)
        try await managedEventually("held recovery preparation") { f.trace().contains { $0["kind"].string == "preparation" } }
        let pending = try #require(owner.recoveryPreparation), bridge = try #require(f.pool.first), pid = try #require(await bridge.processIdentifier())
        #expect(kill(pid, SIGKILL) == 0)
        try await managedEventually("recovery owner disconnected") { owner.connectionStatus == .disconnected }
        #expect(owner.recoveryPreparation?.phase == "uncertain")
        #expect(await f.app.shutdown(confirmed: true))
        var config = f.configuration; config.conversationID = owner.conversationID
        f.app = ManagedSessionCoordinator(configuration: config, nativeDraftDirectory: f.root.appendingPathComponent("native"), profile: "recovery")
        await f.app.start(); let restored = f.app.current
        try await managedEventually("restored original recovery owner") { restored.isConnected }
        #expect(restored.recoveryPreparation?.id == pending.id); #expect(restored.recoveryPreparation?.phase == "uncertain")
        #expect(restored.recoveryPreparation?.accepted == true); #expect(restored.recoveryPreparation?.resultConfirmed == true)
        #expect(f.trace().filter { $0["kind"].string == "preparation" }.count == 1)
        #expect(f.trace().filter { $0["kind"].string == "execution" }.isEmpty)
        #expect(restored.draftText == "Keep after uncertain recovery")
        await f.close()
    } catch { await f.close(); throw error }
}

@Test @MainActor func realGoRecoveryRejectsWrongIdentityAndChangedProcessingScopeBeforePreparation() async throws {
    let f = try await NativePreparationFixture()
    do {
        let owner = f.app.current, recovery = try await f.fail(), bridge = try #require(f.pool.first)
        try f.mark("repaired")
        let original = NativeRecoveryPreparation(id: "placeholder", nonce: "placeholder", conversationID: owner.conversationID, taskID: recovery.id, token: recovery.token, generation: owner.generation, action: .prepareRetry, connections: nil, accessToken: nil, previousRevision: 0)
        for changes: [String: JSONValue] in [["conversation_id": .string("wrong")], ["task_id": .string(String(repeating: "f", count: 32))], ["token": .string(String(repeating: "0", count: 64))]] {
            let request = UUID().uuidString
            let fields = original.fields.merging(changes) { _, new in new }.merging(["request_id": .string(request)]) { _, new in new }
            try await bridge.send(.recovery(fields: fields), nonce: UUID().uuidString)
            try await managedEventually("Go rejects incorrect recovery identity") { owner.snapshot?.value["request_recovery_result"]["request_id"].string == request }
            #expect(owner.snapshot?.value["request_recovery_result"]["error"].string.isEmpty == false)
        }
        let file = f.root.appendingPathComponent("config/gator/config.json"), before = try Data(contentsOf: file)
        var config = try JSONDecoder().decode(JSONValue.self, from: before)
        let providers = config["custom_providers"].array.map { changedJSON($0, ["models": .array([.string("fixture"), .string("different-processing")])]) }
        config = changedJSON(config, ["custom_providers": .array(providers)]); try JSONEncoder().encode(config).write(to: file)
        await f.app.prepareRecovery(owner: owner, recovery: recovery, action: .prepareRetry)
        try await managedEventually("Go rejects changed original processing scope") { owner.recoveryPreparation?.phase == "rejected" }
        #expect(owner.recoveryPreparation?.error.contains("configuration changed") == true)
        #expect(f.trace().filter { $0["kind"].string == "preparation" || $0["kind"].string == "execution" }.isEmpty)
        await f.close()
    } catch { await f.close(); throw error }
}
