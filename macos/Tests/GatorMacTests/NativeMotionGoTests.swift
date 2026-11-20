import Foundation
import Testing
@testable import GatorCore
@testable import GatorMac

@Test @MainActor func streamingRealGoPublicDeltasRetainExactIdentityAndSettleWithoutAdmission() async throws {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent("gator-motion-" + UUID().uuidString)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: root) }
    let provider = Process(), output = Pipe()
    provider.executableURL = URL(fileURLWithPath: "/usr/bin/env")
    let fixture = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("GatorCoreTests/Fixtures/provider.mjs")
    provider.arguments = ["node", fixture.path]; provider.standardOutput = output; provider.standardError = FileHandle.standardError
    provider.environment = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path]
    try provider.run(); defer { provider.terminate(); provider.waitUntilExit() }
    var line = Data()
    while let byte = try output.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; line.append(byte) }
    let port = try JSONDecoder().decode(JSONValue.self, from: line)["port"].int
    #expect(port > 0)
    let config = root.appendingPathComponent("config/gator")
    try FileManager.default.createDirectory(at: config, withIntermediateDirectories: true)
    let settings: JSONValue = .object(["version": .number(4), "defaults": .object(["provider": .string("openai"), "model": .string("gpt-5.6")]), "provider_endpoints": .object(["openai": .string("http://127.0.0.1:\(port)/responses")])])
    try JSONEncoder().encode(settings).write(to: config.appendingPathComponent("config.json"))
    let runtime = try #require(ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"])
    let env = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "OPENAI_API_KEY": "inert-motion-fixture-key", "GATOR_UI_SESSION_HOST": "1", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
    let app = GatorApplicationState(configuration: .init(runtime: URL(fileURLWithPath: runtime), workspace: root.appendingPathComponent("workspace"), environment: env), nativeDraftDirectory: root.appendingPathComponent("native"), profile: "motion")
    func wait(_ predicate: () -> Bool) async throws {
        let deadline = ContinuousClock.now + .seconds(15)
        while !predicate() { if ContinuousClock.now >= deadline { throw NSError(domain: "motion-fixture-timeout", code: 1) }; try await Task.sleep(for: .milliseconds(5)) }
    }
    await app.start(); try await wait { app.isConnected }
    let generation = app.generation, context = app.context
    app.editDraft("Summarize this empty workspace"); await app.submit()
    try await wait { app.snapshot?.running == true }
    let scope = context + generation.uuidString
    try await wait { NativeMotionPresentation.publicText(snapshot: app.snapshot, owner: scope).contains { $0.active && !$0.text.isEmpty } }
    let entry = try #require(NativeMotionPresentation.publicText(snapshot: app.snapshot, owner: scope).first(where: { $0.active }))
    #expect(entry.hasExactIdentity && entry.text.contains("Native integration"))
    let presentation = NativeStreamingPresentation()
    presentation.receive(entry, policy: .init())
    app.editDraft("Unsent draft stays untouched")
    try await wait { app.snapshot?.running == false }
    #expect(app.messages.contains { $0.text.contains("Native integration answer 🐊") })
    #expect(app.context == context && app.generation == generation && app.draftText == "Unsent draft stays untouched")
    #expect(app.snapshot?.value["queued"].int == 0 && app.snapshot?.hasApproval == false)
    presentation.hide(); #expect(!presentation.scheduled)
    #expect(await app.shutdown())
}
