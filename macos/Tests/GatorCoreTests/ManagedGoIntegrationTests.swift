import Foundation
import Testing
@testable import GatorCore

@MainActor func managedEventually(_ label: String, _ condition: () -> Bool) async throws {
    let deadline = ContinuousClock.now + .seconds(15)
    while !condition() {
        guard ContinuousClock.now < deadline else { throw BridgeFailure("Managed Go integration timed out: " + label) }
        try await Task.sleep(for: .milliseconds(15))
    }
}

@Test @MainActor func actualGoManagedDialogueKeepsMainWorkIndependentAndRetainsChildResult() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let provider = Process(), pipe = Pipe()
    provider.executableURL = URL(fileURLWithPath: "/usr/bin/env")
    provider.arguments = ["node", try #require(Bundle.module.url(forResource: "managed-provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
    provider.environment = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "GATOR_FIXTURE_CONTROL": root.path]
    provider.standardOutput = pipe; provider.standardError = FileHandle.standardError
    try provider.run(); defer { provider.terminate(); provider.waitUntilExit() }
    var line = Data()
    while let byte = try pipe.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
    let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
    #expect(port > 0)
    let runtime = ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"
    let env = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
    func cli(_ arguments: [String]) throws {
        let command = Process(); command.executableURL = URL(fileURLWithPath: runtime); command.arguments = arguments
        command.environment = env.merging(["GATOR_INTERNAL_COMMANDS": "1"]) { _, new in new }
        command.standardOutput = FileHandle.nullDevice; command.standardError = FileHandle.standardError
        try command.run(); command.waitUntilExit(); #expect(command.terminationStatus == 0)
    }
    try cli(["provider", "add", "managed-fixture", "--base-url", "http://127.0.0.1:\(port)/v1/chat/completions", "--model", "fixture"])
    try cli(["config", "set", "default-provider", "managed-fixture"])
    let config = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: runtime), workspace: root.appendingPathComponent("workspace"), environment: env)
    let app = ManagedSessionCoordinator(configuration: config, nativeDraftDirectory: root.appendingPathComponent("native"), profile: "managed-go")
    do {
        await app.start(); try await managedEventually("first owner") { app.current.isConnected }
        #expect(app.issue.isEmpty)
        let parent = app.current, parentID = parent.conversationID
        func send(_ text: String) async throws {
            parent.editDraft(text)
            try await managedEventually("composer ready for " + text) { app.canSubmit }
            await app.submit(from: parent)
            try await managedEventually("admitted " + text) { parent.pendingSubmission == nil }
        }
        try await send("Compare venues for the trip")
        try await managedEventually("main Work and conversation readiness") { parent.snapshot?.running == true && parent.snapshot?.value["dialogue_active"].bool == true }
        try await send("Hello while working")
        try await managedEventually("ordinary reply before main completes") { parent.transcript.contains { $0.text.contains("I am here") } }
        #expect(parent.submission?.phase == .completed) // The reply is done; the original Work is not.
        #expect(parent.snapshot?.running == true); #expect(parent.snapshot?.value["queued"].int == 0)
        #expect(app.current === parent)

        try await send("Read separate brief")
        try await managedEventually("background child owner admission") { app.ownerCount == 2 && app.owners.values.contains { $0 !== parent && $0.snapshot?.running == true } && parent.snapshot?.value["dialogue_busy"].bool == false }
        let child = try #require(app.owners.values.first(where: { $0 !== parent })), childID = child.conversationID
        #expect(child.snapshot?.value["dialogue_task"]["parent_id"].string == parentID)
        #expect(app.current === parent); #expect(app.workingCount == 2)
        parent.editDraft("Parent draft while visiting")
        await app.openConversation(childID)
        #expect(app.current === child); #expect(parent.snapshot?.running == true)
        child.editDraft("Child draft")
        await app.openConversation(parentID)
        #expect(app.current === parent); #expect(parent.draftText == "Parent draft while visiting")
        #expect(app.ownerCount == 2)
        try await send("Keep the venues under $2,000")
        try await managedEventually("guide to exact original Work") { parent.snapshot?.value["dialogue"].array.contains { $0["text"].string == "Keep the venues under $2,000" && $0["state"].string == "guided" } == true }
        #expect(child.snapshot?.running == true)

        try Data().write(to: root.appendingPathComponent("release-child"))
        try await managedEventually("retained child completion") { child.snapshot?.running == false && parent.snapshot?.value["dialogue_peers"].array.contains { $0["conversation_id"].string == childID && $0["state"].string == "completed" } == true }
        #expect(app.current === parent); #expect(parent.snapshot?.running == true)
        #expect(app.activity.contains { $0.id == childID && $0.needsAttention })
        try await send("What did the separate brief say?")
        try await managedEventually("result used in conversation") { parent.transcript.contains { $0.text == "The separate brief says jade heron." } }
        #expect(parent.snapshot?.running == true)
        try await send("Research a stop target")
        try await managedEventually("another child with exact controls") { parent.snapshot?.value["dialogue_peers"].array.contains { $0["conversation_id"].string != childID && $0["can_stop"].bool && $0["can_guide"].bool } == true && parent.snapshot?.value["dialogue_busy"].bool == false }
        try await send("Guide research locally")
        try await managedEventually("host guide accepted by original child") { parent.snapshot?.value["dialogue"].array.contains { $0["text"].string == "Guide research locally" && $0["state"].string == "guided" } == true }
        try await send("Stop research target")
        try await managedEventually("host stop accepted by original child") { parent.snapshot?.value["dialogue"].array.contains { $0["text"].string == "Stop research target" && $0["state"].string == "stopped" } == true }
        #expect(parent.snapshot?.running == true)
        #expect(app.ownerCount == 3)
        await app.openConversation(childID)
        #expect(child.messages.contains { $0.text.contains("jade heron") }); #expect(child.draftText == "Child draft")
        try Data().write(to: root.appendingPathComponent("release-main"))
        try await managedEventually("main completion in background") { parent.snapshot?.running == false }
        #expect(app.current === child)
        #expect(parent.messages.contains { $0.text.contains("$2,000") })
        #expect(app.activity.contains { $0.id == parentID && $0.needsAttention })
        #expect(await app.shutdown())

        let restored = ManagedSessionCoordinator(configuration: config, nativeDraftDirectory: root.appendingPathComponent("native"), profile: "managed-go")
        await restored.start()
        #expect(restored.current.conversationID == childID); #expect(restored.current.draftText == "Child draft")
        #expect(restored.ownerCount == 1) // Inspection never recreates old working owners.
        #expect(restored.current.messages.contains { $0.text.contains("jade heron") })
        #expect(await restored.shutdown())
    } catch {
        print("Managed fixture issue:", app.issue)
        print("Managed fixture dialogue:", app.current.snapshot?.value["dialogue"].pretty ?? "")
        print("Managed fixture peers:", app.current.snapshot?.value["dialogue_peers"].pretty ?? "")
        print("Managed fixture trace:", (try? String(contentsOf: root.appendingPathComponent("trace.jsonl"), encoding: .utf8)) ?? "")
        _ = await app.shutdown(confirmed: true)
        throw error
    }
}
