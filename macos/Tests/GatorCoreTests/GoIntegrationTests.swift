import Foundation
import Testing
@testable import GatorCore

@Test @MainActor func realGoBridgeSubmitsStreamsRestoresAndPreservesDraft() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let provider = Process(), output = Pipe()
    provider.executableURL = URL(fileURLWithPath: "/usr/bin/env")
    provider.arguments = ["node", try #require(Bundle.module.url(forResource: "provider", withExtension: "mjs", subdirectory: "Fixtures")).path]
    provider.standardOutput = output; provider.standardError = FileHandle.standardError
    try provider.run()
    defer { provider.terminate(); provider.waitUntilExit() }
    var portLine = Data()
    while let byte = try output.fileHandleForReading.read(upToCount: 1), !byte.isEmpty { if byte == Data([10]) { break }; portLine.append(byte) }
    let port = try JSONDecoder().decode(JSONValue.self, from: portLine)["port"].int
    #expect(port > 0)
    let config = root.appendingPathComponent("config/gator"), workspace = root.appendingPathComponent("workspace")
    try FileManager.default.createDirectory(at: config, withIntermediateDirectories: true)
    let settings: JSONValue = .object(["version": .number(4), "defaults": .object(["provider": .string("openai"), "model": .string("gpt-5.6")]), "provider_endpoints": .object(["openai": .string("http://127.0.0.1:\(port)/responses")])])
    try JSONEncoder().encode(settings).write(to: config.appendingPathComponent("config.json"))
    let runtime = ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"
    let env = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "OPENAI_API_KEY": "fixture-only-not-a-real-credential", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null", "GATOR_UI_SESSION_HOST": "1"]
    let configuration = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: runtime), workspace: workspace, environment: env)
    let app = GatorApplicationState(configuration: configuration, nativeDraftDirectory: root.appendingPathComponent("native/drafts"), profile: "test")
    await app.start(); try await waitUntil { app.isConnected }
    #expect(app.readiness["state"].string == "configured")
    app.editDraft("Summarize this empty workspace")
    await app.submit()
    try await waitUntil { app.snapshot?.running == true }
    try await waitUntil { app.snapshot?.value["activity"]["entries"].array.contains(where: { $0["text"].string.contains("Native integration") }) == true }
    #expect(app.snapshot?.running == true) // Actual text delta before final completion.
    try await waitUntil { app.snapshot?.running == false }
    #expect(app.messages.contains { $0.text.contains("Native integration answer") })
    #expect(app.pendingSubmission == nil); #expect(app.draftText.isEmpty)
    let originalID = app.conversationID
    app.editDraft("Keep across native restart")
    #expect(await app.shutdown())
    let reopened = GatorApplicationState(configuration: configuration, nativeDraftDirectory: root.appendingPathComponent("native/drafts"), profile: "test")
    await reopened.start(); try await waitUntil { reopened.isConnected }
    #expect(reopened.conversationID == originalID)
    #expect(reopened.draftText == "Keep across native restart")
    #expect(reopened.messages.contains { $0.text.contains("Native integration answer") })
    #expect(reopened.snapshot?.running == false)
    #expect(await reopened.shutdown())
}

@Test func subprocessAcknowledgementExitAndCleanup() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let original = try #require(Bundle.module.url(forResource: "bridge-fixture", withExtension: "mjs", subdirectory: "Fixtures"))
    let executable = root.appendingPathComponent("fixture.mjs")
    var bytes = Data("#!/usr/bin/env node\n".utf8); bytes.append(try Data(contentsOf: original)); try bytes.write(to: executable)
    try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
    let transport = GoBridgeConnection(acknowledgementTimeout: .seconds(2))
    try await transport.start(BridgeLaunchConfiguration(runtime: executable, workspace: root))
    var iterator = transport.events.makeAsyncIterator()
    guard case .state = await iterator.next() else { Issue.record("Expected process state"); await transport.shutdown(); return }
    try await transport.send(.newConversation, nonce: "transport-only")
    #expect(await iterator.next() == .acknowledged("transport-only"))
    // The fixture exits unexpectedly on /cancel without acknowledging it.
    await #expect(throws: (any Error).self) { try await transport.send(.stop, nonce: "lost") }
    guard case .disconnected = await iterator.next() else { Issue.record("Expected disconnect"); await transport.shutdown(); return }
    await transport.shutdown()
    #expect(await iterator.next() == nil)
}

@Test func shutdownEscalatesEvenWhenAFullInputPipeBlocksWrites() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let source = try #require(Bundle.module.url(forResource: "bridge-fixture", withExtension: "mjs", subdirectory: "Fixtures"))
    let executable = root.appendingPathComponent("blocked-fixture.mjs")
    var bytes = Data("#!/usr/bin/env node\n".utf8); bytes.append(try Data(contentsOf: source)); try bytes.write(to: executable)
    try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: executable.path)
    var env = ProcessInfo.processInfo.environment; env["GATOR_FIXTURE_NO_READ"] = "1"
    let transport = GoBridgeConnection(acknowledgementTimeout: .milliseconds(80))
    try await transport.start(BridgeLaunchConfiguration(runtime: executable, workspace: root, environment: env))
    var events = transport.events.makeAsyncIterator(); _ = await events.next()
    let pid = try #require(await transport.processIdentifier())
    await #expect(throws: (any Error).self) { try await transport.send(.submission(id: "blocked", context: "conversation:fixture", text: String(repeating: "a", count: 128 * 1024)), nonce: "blocked") }
    let start = ContinuousClock.now
    await transport.shutdown()
    #expect(ContinuousClock.now - start < .seconds(7))
    #expect(kill(pid, 0) != 0)
}

final class GoTestConnections: @unchecked Sendable {
    private let lock = NSLock()
    private var owners: [GoBridgeConnection]
    init(_ owners: [GoBridgeConnection]) { self.owners = owners }
    func next() -> any BridgeTransport { lock.withLock { owners.removeFirst() } }
}

@Test @MainActor func realGoUnexpectedExitAndExplicitReconnectPreserveIdentityWithoutReplay() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = GoBridgeConnection(), replacement = GoBridgeConnection(), owners = GoTestConnections([first, replacement])
    let runtime = ProcessInfo.processInfo.environment["GATOR_TEST_BINARY"] ?? "/tmp/gator-native-runtime"
    let config = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: runtime), workspace: root.appendingPathComponent("workspace"), environment: ["PATH": "/usr/bin:/bin", "HOME": root.path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path])
    let app = GatorApplicationState(configuration: config, store: NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "reconnect"), factory: { owners.next() })
    await app.start(); try await waitUntil { app.isConnected }
    let original = app.conversationID, pid = try #require(await first.processIdentifier())
    app.editDraft("Keep through actual process failure")
    #expect(kill(pid, SIGTERM) == 0)
    try await waitUntil { app.connectionStatus == .disconnected }
    #expect(app.draftText == "Keep through actual process failure")
    await app.reconnect(); try await waitUntil { app.isConnected }
    #expect(app.conversationID == original); #expect(app.draftText == "Keep through actual process failure")
    #expect(app.snapshot?.running == false); #expect(app.snapshot?.value["queued"].int == 0)
    #expect(await app.shutdown())
}
