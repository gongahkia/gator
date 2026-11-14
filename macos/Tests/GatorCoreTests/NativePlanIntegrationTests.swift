import Foundation
import Testing
@testable import GatorCore

@MainActor final class NativePlanFixture {
    let root: URL
    let provider: Process
    let configuration: BridgeLaunchConfiguration
    var app: ManagedSessionCoordinator
    init() async throws {
        root = try temporaryDirectory(); provider = Process(); let pipe = Pipe()
        provider.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        provider.arguments = ["node", try #require(Bundle.module.url(forResource: "plan-provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
        let path = ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin"
        provider.environment = ["PATH": path, "GATOR_FIXTURE_CONTROL": root.path]
        provider.standardOutput = pipe; provider.standardError = FileHandle.standardError
        try provider.run()
        var line = Data()
        while let byte = try pipe.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
        let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
        let config = root.appendingPathComponent("config/gator"), workspace = root.appendingPathComponent("workspace")
        try FileManager.default.createDirectory(at: config, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: workspace, withIntermediateDirectories: true)
        let settings: JSONValue = .object(["version": .number(4), "defaults": .object(["provider": .string("fixture"), "model": .string("fixture")]), "custom_providers": .array([.object(["id": .string("fixture"), "base_url": .string("http://127.0.0.1:\(port)/chat"), "models": .array([.string("fixture")]), "default_model": .string("fixture")])])])
        try JSONEncoder().encode(settings).write(to: config.appendingPathComponent("config.json"))
        try Data().write(to: root.appendingPathComponent("hold-execution"))
        configuration = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"), workspace: workspace, environment: ["PATH": path, "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"])
        app = ManagedSessionCoordinator(configuration: configuration, nativeDraftDirectory: root.appendingPathComponent("native"), profile: "plan")
        await app.start(); try await managedEventually("plan fixture connected") { app.current.isConnected }
    }
    func send(_ text: String) async throws {
        app.current.editDraft(text); try await managedEventually("plan composer ready") { app.canSubmit }; await app.submit(from: app.current)
    }
    func prepare() async throws -> NativePlanReview {
        app.current.setPlanFirst(true); try await send("Compare the selected options")
        try await managedEventually("real proposed plan") { app.current.snapshot?.value["task_plans"].array.first?["phase"].string == "review" && app.current.snapshot?.running == false }
        #expect(app.current.submission?.phase == .accepted) // Preparation must not be labeled execution.
        app.current.setPlanFirst(false)
        return try await inspect()
    }
    func inspect() async throws -> NativePlanReview {
        let owner = app.current
        let summary = try #require(owner.snapshot?.value["task_plans"].array.first)
        let identity = try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: summary))
        await app.loadPlan(identity, owner: owner)
        try await managedEventually("matching exact plan inspection") { owner.planReviews[identity.taskID]?.identity == identity && owner.operation.isEmpty }
        return try #require(owner.planReviews[identity.taskID])
    }
    func trace() -> [JSONValue] { ((try? String(contentsOf: root.appendingPathComponent("plan-trace.jsonl"), encoding: .utf8)) ?? "").split(separator: "\n").compactMap { try? JSONDecoder().decode(JSONValue.self, from: Data($0.utf8)) } }
    func release() throws { try Data().write(to: root.appendingPathComponent("release-execution")) }
    func restart() async throws {
        let id = app.current.conversationID
        #expect(await app.shutdown(confirmed: true))
        var config = configuration; config.conversationID = id
        app = ManagedSessionCoordinator(configuration: config, nativeDraftDirectory: root.appendingPathComponent("native"), profile: "plan")
        await app.start(); try await managedEventually("restarted original plan owner") { app.current.isConnected }
    }
    func retainApprovalWithoutDispatch(_ review: NativePlanReview) throws {
        // Fixture setup deliberately runs only after the native owner exited.
        let repo = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let command = Process(); command.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        command.arguments = ["go", "run", "./macos/Tests/GatorCoreTests/Fixtures/retained-plan", root.appendingPathComponent("state").path, review.identity.conversationID, review.identity.taskID]
        command.currentDirectoryURL = repo; command.environment = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": ProcessInfo.processInfo.environment["HOME"] ?? root.path]
        command.standardOutput = FileHandle.standardOutput; command.standardError = FileHandle.standardError
        try command.run(); command.waitUntilExit(); #expect(command.terminationStatus == 0)
    }
    func close() async {
        try? release(); _ = await app.shutdown(confirmed: true)
        provider.terminate(); provider.waitUntilExit(); try? FileManager.default.removeItem(at: root)
    }
}

@Test @MainActor func realGoExactPlanApprovalExecutesOnceAndConversationContinues() async throws {
    let fixture = try await NativePlanFixture()
    do {
        let app = fixture.app, owner = app.current
        let review = try await fixture.prepare()
        #expect(app.planAuthorizationUnavailable(owner: owner, review: review, action: .approve) == nil)
        #expect(fixture.trace().allSatisfy { $0["kind"].string != "execution" })
        await app.authorizePlan(owner: owner, review: review, action: .approve)
        try await managedEventually("real execution admitted and running") { owner.planOperation?.resultConfirmed == true && owner.planOperation?.phase == .working && fixture.trace().contains { $0["kind"].string == "execution" } }
        await app.authorizePlan(owner: owner, review: review, action: .approve)
        #expect(fixture.trace().filter { $0["kind"].string == "execution" }.count == 1)
        try await managedEventually("dialogue readiness while exact work runs") { owner.snapshot?.value["dialogue_active"].bool == true }
        try await fixture.send("Hello during execution")
        try await managedEventually("independent conversational reply") { owner.transcript.contains { $0.text.contains("An independent conversation reply") } }
        #expect(owner.snapshot?.running == true)
        owner.editDraft("Retain after reviewed execution")
        try fixture.release()
        try await managedEventually("retained exact task completion") { owner.planOperation?.phase == .completed && owner.snapshot?.running == false }
        #expect(owner.messages.contains { $0.text.contains("jade heron") })
        #expect(owner.draftText == "Retain after reviewed execution")
        #expect(fixture.trace().filter { $0["kind"].string == "execution" }.count == 1)
        await fixture.close()
    } catch { print("Plan execution fixture failed", fixture.app.issue, fixture.app.current.issue, fixture.app.current.snapshot?.value.pretty ?? "", fixture.trace()); await fixture.close(); throw error }
}

@Test @MainActor func realGoRevisedPlanRequiresFreshInspectionBeforeSingleExecution() async throws {
    let fixture = try await NativePlanFixture()
    do {
        let app = fixture.app, owner = app.current, original = try await fixture.prepare()
        await app.requestPlanRevision(owner: owner, review: original)
        try await fixture.send("Keep the comparison under $2,000")
        try await managedEventually("actual replacement plan") { owner.snapshot?.value["task_plans"].array.first?["phase"].string == "review" && (owner.snapshot?.value["task_plans"].array.first?["revision"].int ?? 0) > original.identity.revision && owner.snapshot?.running == false }
        await app.authorizePlan(owner: owner, review: original, action: .approve)
        #expect(fixture.trace().allSatisfy { $0["kind"].string != "execution" })
        let replacement = try await fixture.inspect()
        #expect(replacement.identity.taskID == original.identity.taskID)
        #expect(replacement.identity.digest != original.identity.digest)
        await app.authorizePlan(owner: owner, review: replacement, action: .approve)
        try await managedEventually("replacement executes") { owner.planOperation?.phase == .working }
        try fixture.release(); try await managedEventually("replacement completes") { owner.planOperation?.phase == .completed }
        #expect(fixture.trace().filter { $0["kind"].string == "execution" }.count == 1)
        await fixture.close()
    } catch { print("Plan revision execution fixture failed", fixture.app.issue, fixture.app.current.issue, fixture.trace()); await fixture.close(); throw error }
}

@Test @MainActor func realGoRestoredApprovedPlanDoesNotAutoStartAndExplicitStartExecutesOnce() async throws {
    let fixture = try await NativePlanFixture()
    do {
        let original = try await fixture.prepare(), oldID = fixture.app.current.conversationID
        #expect(await fixture.app.shutdown(confirmed: true))
        try fixture.retainApprovalWithoutDispatch(original)
        try await fixture.restart()
        let app = fixture.app, owner = app.current
        #expect(owner.conversationID == oldID); #expect(owner.generation != original.generation)
        #expect(fixture.trace().allSatisfy { $0["kind"].string != "execution" })
        #expect(owner.snapshot?.running == false)
        #expect(owner.snapshot?.value["task_plans"].array.first?["phase"].string == "approved")
        #expect(!owner.isCurrentPlanReview(original))
        let retained = try await fixture.inspect()
        #expect(app.planAuthorizationUnavailable(owner: owner, review: retained, action: .approve) != nil)
        #expect(app.planAuthorizationUnavailable(owner: owner, review: retained, action: .start) == nil)
        await app.authorizePlan(owner: owner, review: retained, action: .start)
        try await managedEventually("retained approval explicitly starts") { owner.planOperation?.phase == .working }
        await app.authorizePlan(owner: owner, review: retained, action: .start)
        try fixture.release(); try await managedEventually("retained plan completes") { owner.planOperation?.phase == .completed }
        #expect(fixture.trace().filter { $0["kind"].string == "execution" }.count == 1)
        await fixture.close()
    } catch { print("Plan restored approval fixture failed", fixture.app.issue, fixture.app.current.issue, fixture.trace()); await fixture.close(); throw error }
}

@Test @MainActor func realGoRejectsWrongPlanIdentitiesAndUnapprovedStartIndependentlyOfNativeGuards() async throws {
    let fixture = try await NativePlanFixture()
    do {
        let original = try await fixture.prepare()
        #expect(await fixture.app.shutdown(confirmed: true))
        var config = fixture.configuration; config.conversationID = original.identity.conversationID
        let bridge = GoBridgeConnection()
        // One actual owner. Direct test-only transport access deliberately tests
        // Go's boundary independently of the presentation guards.
        fixture.app = ManagedSessionCoordinator(configuration: config, store: NativeDraftStore(directory: fixture.root.appendingPathComponent("native"), profile: "plan"), factory: { bridge })
        let app = fixture.app
        await app.start(); let owner = app.current
        try await managedEventually("reopened plan identity boundary") { owner.isConnected }
        let current = try await fixture.inspect()
        let variants = [
            try #require(NativePlanIdentity(conversationID: "another-owner", summary: current.value)),
            try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: changedJSON(current.value, ["id": .string(String(repeating: "d", count: 32))]))),
            try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: changedJSON(current.value, ["revision": .number(Decimal(current.identity.revision + 1))]))),
            try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: changedJSON(current.value, ["digest": .string(String(repeating: "e", count: 64))])))
        ]
        for identity in variants {
            let request = UUID().uuidString
            try await bridge.send(.authorizePlan(identity, action: .approve, request: request), nonce: UUID().uuidString)
            try await managedEventually("Go rejects mismatched exact plan identity") { owner.snapshot?.value["task_plan_result"]["request_id"].string == request }
            #expect(owner.snapshot?.value["task_plan_result"]["error"].string.isEmpty == false)
            #expect(fixture.trace().allSatisfy { $0["kind"].string != "execution" })
        }
        let start = UUID().uuidString
        try await bridge.send(.authorizePlan(current.identity, action: .start, request: start), nonce: UUID().uuidString)
        try await managedEventually("Go rejects Start without retained approval") { owner.snapshot?.value["task_plan_result"]["request_id"].string == start }
        #expect(owner.snapshot?.value["task_plan_result"]["error"].string.isEmpty == false)
        try await bridge.send(.host(.focus(false)), nonce: UUID().uuidString)
        try await managedEventually("owner actually backgrounded") { owner.snapshot?.value["foreground"].bool == false }
        let background = UUID().uuidString
        try await bridge.send(.authorizePlan(current.identity, action: .approve, request: background), nonce: UUID().uuidString)
        try await managedEventually("Go rejects mutation without foreground owner") { owner.snapshot?.value["task_plan_result"]["request_id"].string == background }
        #expect(owner.snapshot?.value["task_plan_result"]["error"].string.isEmpty == false)
        #expect(fixture.trace().allSatisfy { $0["kind"].string != "execution" })
        try await bridge.send(.host(.focus(true)), nonce: UUID().uuidString)
        try await managedEventually("original owner refocused") { owner.snapshot?.value["foreground"].bool == true }
        let inspected = try await fixture.inspect()
        await app.authorizePlan(owner: owner, review: inspected, action: .approve)
        try await managedEventually("only correct exact approval executes") { owner.planOperation?.phase == .working }
        try fixture.release(); try await managedEventually("correct reviewed execution retained") { owner.planOperation?.phase == .completed }
        #expect(fixture.trace().filter { $0["kind"].string == "execution" }.count == 1)
        await fixture.close()
    } catch { print("Plan identity boundary failed", fixture.app.issue, fixture.app.current.issue); await fixture.close(); throw error }
}
