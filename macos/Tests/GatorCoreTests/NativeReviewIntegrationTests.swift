import Foundation
import Testing
@testable import GatorCore

/// Fixture setup uses the existing bridge's terminal settings commands. Native
/// product views do not receive this helper or arbitrary frame-writing authority.
func seedReviewConversation(runtime: String, root: URL, environment: [String: String]) throws -> String {
    let process = Process(), input = Pipe(), output = Pipe()
    process.executableURL = URL(fileURLWithPath: runtime); process.arguments = ["ui-backend", "", ""]
    process.currentDirectoryURL = root.appendingPathComponent("workspace")
    process.environment = environment.merging(["GATOR_UI_BACKEND": "1"]) { _, new in new }
    process.standardInput = input; process.standardOutput = output; process.standardError = FileHandle.standardError
    try process.run()
    defer { if process.isRunning { process.terminate(); process.waitUntilExit() } }
    for text in ["/mode act", "/connector add google"] {
        var data = try JSONEncoder().encode(JSONValue.object(["version": .number(1), "type": .string("submit"), "text": .string(text)])); data.append(10)
        try input.fileHandleForWriting.write(contentsOf: data)
    }
    var framer = BridgeFramer(), conversation = ""
    var seededScope = false
    while process.isRunning {
        let bytes = output.fileHandleForReading.availableData
        if bytes.isEmpty { break }
        for event in try framer.append(bytes) {
            guard case .state(let state) = event else { continue }
            if !state.conversationID.isEmpty { conversation = state.conversationID }
            if !seededScope, state.value["work_details"]["connector_ids"].array.contains(.string("google")) {
                seededScope = true
                try input.fileHandleForWriting.write(contentsOf: Data("{\"version\":1,\"type\":\"submit\",\"text\":\"Prepare controlled fixture scope\"}\n".utf8))
            }
            if seededScope, state.messages.contains(where: { $0.text == "Fixture scope retained." }) {
                try input.fileHandleForWriting.write(contentsOf: Data("{\"version\":1,\"type\":\"exit\"}\n".utf8))
                try input.fileHandleForWriting.close(); process.waitUntilExit(); return conversation
            }
        }
    }
    throw BridgeFailure("Controlled connector scope was not seeded through Go.")
}

@MainActor final class NativeReviewFixture {
    let root: URL
    let provider: Process
    let app: ManagedSessionCoordinator
    init(action: Bool) async throws {
        root = try temporaryDirectory(); provider = Process(); let pipe = Pipe()
        provider.executableURL = URL(fileURLWithPath: "/usr/bin/env")
        provider.arguments = ["node", try #require(Bundle.module.url(forResource: "review-provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
        let path = ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin"
        provider.environment = ["PATH": path, "GATOR_FIXTURE_CONTROL": root.path]
        provider.standardOutput = pipe; provider.standardError = FileHandle.standardError
        try provider.run()
        var line = Data()
        while let byte = try pipe.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
        let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
        let configRoot = root.appendingPathComponent("config/gator"), workspace = root.appendingPathComponent("workspace")
        try FileManager.default.createDirectory(at: configRoot, withIntermediateDirectories: true)
        try FileManager.default.createDirectory(at: workspace, withIntermediateDirectories: true)
        let base = "http://127.0.0.1:\(port)"
        let config: JSONValue = .object(["version": .number(4), "defaults": .object(["provider": .string("fixture"), "model": .string("fixture")]), "custom_providers": .array([.object(["id": .string("fixture"), "base_url": .string(base + "/chat"), "models": .array([.string("fixture")]), "default_model": .string("fixture")])]), "connectors": .array([.object(["version": .number(1), "id": .string("google"), "name": .string("Local controlled fixture"), "kind": .string("google_workspace"), "resource": .string(base), "authentication": .string("none")])])])
        try JSONEncoder().encode(config).write(to: configRoot.appendingPathComponent("config.json"))
        let runtime = ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"
        let env = ["PATH": path, "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
        let conversation = action ? try seedReviewConversation(runtime: runtime, root: root, environment: env) : ""
        app = ManagedSessionCoordinator(configuration: BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: runtime), workspace: workspace, environment: env, conversationID: conversation), nativeDraftDirectory: root.appendingPathComponent("native"), profile: "review")
        await app.start()
        try await managedEventually("review fixture ready") { app.current.isConnected }
        if action {
            await app.current.selectWorkMode(.act)
            #expect(app.current.snapshot?.value["work_details"]["mode"].string == "act")
        }
    }
    func send(_ text: String) async throws {
        app.current.editDraft(text)
        try await managedEventually("review composer readiness") { app.canSubmit }
        await app.submit(from: app.current)
    }
    func trace() -> [JSONValue] {
        ((try? String(contentsOf: root.appendingPathComponent("review-trace.jsonl"), encoding: .utf8)) ?? "").split(separator: "\n").compactMap { try? JSONDecoder().decode(JSONValue.self, from: Data($0.utf8)) }
    }
    func close() async {
        try? Data().write(to: root.appendingPathComponent("release-main")); try? Data().write(to: root.appendingPathComponent("release-revision"))
        _ = await app.shutdown(confirmed: true)
        provider.terminate(); provider.waitUntilExit(); try? FileManager.default.removeItem(at: root)
    }
}

@Test @MainActor func realGoActionCorrectionKeepsChildOwnerAndRequiresFreshExactApproval() async throws {
    let fixture = try await NativeReviewFixture(action: true), app = fixture.app
    do {
        let parent = app.current
        try await fixture.send("Hold another task")
        try await managedEventually("held parent") { parent.snapshot?.running == true && parent.snapshot?.value["dialogue_active"].bool == true && fixture.trace().contains { $0["kind"].string == "held-work" } }
        try await fixture.send("Send the venue update")
        try await managedEventually("delegated action review") { app.owners.values.contains { $0 !== parent && $0.currentApprovalReview != nil } && parent.snapshot?.value["dialogue_busy"].bool == false }
        let child = try #require(app.owners.values.first(where: { $0 !== parent }))
        await app.openConversation(child.conversationID); child.openApprovalReview()
        let original = try #require(child.review)
        #expect(original.result.pretty.contains("Original subject"))
        #expect(original.result.pretty.contains("FINAL REVIEW LINE"))
        child.editDraft("Keep the child's unrelated draft")
        await app.requestActionCorrection(owner: child, review: original)
        #expect(app.current === parent); #expect(app.current.correctionTarget?.ownerID == child.conversationID)
        parent.editDraft("Change the subject to Project Update. Keep the recipients and body unchanged.")
        await app.submit(from: parent)
        try await managedEventually("replacement exact review") { child.currentApprovalReview?.token != original.token && child.currentApprovalReview != nil && parent.snapshot?.value["dialogue_busy"].bool == false }
        let replacement = try #require(child.currentApprovalReview)
        #expect(app.ownerCount == 2); #expect(parent.snapshot?.running == true)
        #expect(replacement.result.pretty.contains("Project Update")); #expect(replacement.result.pretty.contains("Preserved body"))
        #expect(fixture.trace().filter { $0["kind"].string == "effect" }.isEmpty)
        #expect(!child.isCurrentReview(original)); #expect(app.correctionStatus?.contains("fresh exact review") == true)
        // Neither a stale token nor a decision from the parent may execute it.
        await app.respondToApproval(owner: child, review: replacement, approved: true)
        #expect(fixture.trace().filter { $0["kind"].string == "effect" }.isEmpty)
        await app.openConversation(child.conversationID)
        await app.respondToApproval(owner: child, review: original, approved: true)
        #expect(fixture.trace().filter { $0["kind"].string == "effect" }.isEmpty)
        child.openApprovalReview()
        await app.respondToApproval(owner: child, review: replacement, approved: true)
        try await managedEventually("controlled exact corrected action completed") { child.snapshot?.running == false }
        let effects = fixture.trace().filter { $0["kind"].string == "effect" }
        #expect(effects.count == 1)
        let encoded = try #require(effects.first?["body"]["raw"].string).replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        let padded = encoded + String(repeating: "=", count: (4 - encoded.count % 4) % 4)
        let sent = String(decoding: try #require(Data(base64Encoded: padded)), as: UTF8.self)
        #expect(sent.contains("Subject: Project Update")); #expect(sent.contains("To: venue@example.com"))
        let mimeBody = try #require(sent.components(separatedBy: "\r\n\r\n").last)
        let body = String(decoding: try #require(Data(base64Encoded: mimeBody, options: .ignoreUnknownCharacters)), as: UTF8.self)
        #expect(body.contains("Preserved body")); #expect(body.contains("FINAL REVIEW LINE"))
        #expect(child.draftText == "Keep the child's unrelated draft")
        #expect(!child.hasReviewableApproval)
        #expect(fixture.trace().filter { $0["kind"].string == "route" }.allSatisfy { !$0.pretty.contains(original.token) })
        await fixture.close()
    } catch { print("Review fixture failure", app.issue, app.current.issue, fixture.trace()); await fixture.close(); throw error }
}

@Test @MainActor func realGoPlanRevisionDistinguishesAdmissionFromReplacementAndRejectsOldReview() async throws {
    let fixture = try await NativeReviewFixture(action: false), app = fixture.app
    do {
        let owner = app.current
        owner.setPlanFirst(true)
        try await fixture.send("Compare venues")
        try await managedEventually("initial proposed plan") { owner.snapshot?.value["task_plans"].array.first?["phase"].string == "review" && owner.snapshot?.running == false }
        let summary = try #require(owner.snapshot?.value["task_plans"].array.first)
        let identity = try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: summary))
        app.inspectTask(owner.conversationID); await app.loadPlan(identity, owner: owner)
        try await managedEventually("full exact proposed plan read") { owner.planReviews[identity.taskID] != nil }
        let original = try #require(owner.planReviews[identity.taskID])
        #expect(original.value["content"]["steps"].array.first?["text"].string == "Compare venues in the selected scope")
        owner.editDraft("Unrelated draft retained after plan revision")
        try Data().write(to: fixture.root.appendingPathComponent("hold-revision"))
        await app.requestPlanRevision(owner: owner, review: original)
        owner.editDraft("Keep the venue comparison under $2,000")
        await app.submit(from: owner)
        try await managedEventually("revision admission before actual proposal") { owner.snapshot?.value["task_plans"].array.first?["phase"].string == "revising" && owner.snapshot?.value["dialogue_busy"].bool == false }
        #expect(app.correctionStatus == "Revision requested · replacement is being prepared")
        #expect(!owner.isCurrentPlanReview(original)); #expect(app.ownerCount == 1)
        #expect(owner.draftText == "Unrelated draft retained after plan revision")
        try Data().write(to: fixture.root.appendingPathComponent("release-revision"))
        try await managedEventually("actual revised proposal requires review") { owner.snapshot?.value["task_plans"].array.first?["phase"].string == "review" && owner.snapshot?.running == false }
        let updated = try #require(owner.snapshot?.value["task_plans"].array.first)
        let fresh = try #require(NativePlanIdentity(conversationID: owner.conversationID, summary: updated))
        #expect(fresh.taskID == identity.taskID); #expect(fresh.revision > identity.revision); #expect(fresh.digest != identity.digest)
        #expect(!owner.isCurrentPlan(identity))
        await app.loadPlan(identity, owner: owner); #expect(!owner.isCurrentPlanReview(original))
        await app.loadPlan(fresh, owner: owner)
        try await managedEventually("fresh revision-bound read") { owner.planReviews[identity.taskID]?.identity == fresh }
        #expect(owner.planReviews[identity.taskID]?.value["content"]["outcome"].string == "Venue comparison under $2,000")
        #expect(app.correctionStatus?.contains("Replacement plan available") == true)
        #expect(fixture.trace().allSatisfy { $0["kind"].string != "action-model" && $0["kind"].string != "effect" })
        await fixture.close()
    } catch { print("Plan fixture failure", app.issue, app.current.issue, fixture.trace()); await fixture.close(); throw error }
}
