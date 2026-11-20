import Foundation
import Testing
@testable import GatorCore
@testable import GatorMac

@Test @MainActor func messageReactionRealGoPersistsWithoutInferenceAndRestoresDraft() async throws {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent("gator-reaction-" + UUID().uuidString)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: root) }
    let countFile = root.appendingPathComponent("requests.json"), provider = Process(), output = Pipe()
    let fixture = "const http=require('http'),fs=require('fs');let count=0;fs.writeFileSync(process.argv[1],'0');const s=http.createServer((q,r)=>{fs.writeFileSync(process.argv[1],String(++count));r.writeHead(503);r.end('inert fixture: inference must not be requested')});s.listen(0,'127.0.0.1',()=>console.log(JSON.stringify({port:s.address().port})));"
    provider.executableURL = URL(fileURLWithPath: "/usr/bin/env"); provider.arguments = ["node", "-e", fixture, countFile.path]
    provider.environment = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path]
    provider.standardOutput = output; provider.standardError = FileHandle.standardError
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
    let env = ["PATH": ProcessInfo.processInfo.environment["PATH"] ?? "/usr/bin:/bin", "HOME": root.path, "GATOR_CONFIG_DIR": root.appendingPathComponent("config").path, "GATOR_STATE_DIR": root.appendingPathComponent("state").path, "OPENAI_API_KEY": "inert-reaction-fixture-key", "GATOR_UI_SESSION_HOST": "1", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": "/dev/null"]
    let configuration = BridgeLaunchConfiguration(runtime: URL(fileURLWithPath: runtime), workspace: root.appendingPathComponent("workspace"), environment: env)
    let native = root.appendingPathComponent("native")
    let sessions = ManagedSessionCoordinator(configuration: configuration, nativeDraftDirectory: native, profile: "reaction")
    var reopened: ManagedSessionCoordinator?
    func wait(_ predicate: () -> Bool) async throws {
        let deadline = ContinuousClock.now + .seconds(15)
        while !predicate() { if ContinuousClock.now >= deadline { throw NSError(domain: "reaction-fixture-timeout", code: 1) }; try await Task.sleep(for: .milliseconds(5)) }
    }
    do {
        await sessions.start(); try await wait { sessions.current.isConnected }
        let owner = sessions.current, conversation = owner.conversationID, generation = owner.generation
        try await owner.host(.configure(canStart: true, otherBusy: false, peers: []))
        let began = ContinuousClock.now
        owner.editDraft("Thanks Gator, that was helpful!"); await sessions.submit(from: owner)
        try await wait { owner.transcript.first?.messageReaction != nil }
        let latency = ContinuousClock.now - began
        let reaction = try #require(owner.transcript.first?.messageReaction)
        #expect(reaction.conversation == conversation && reaction.kind == "appreciation")
        #expect(sessions.ownerCount == 1 && owner.generation == generation && owner.snapshot?.running == false)
        #expect(NativeQuickReactionMode.transcript(owner.transcript, mode: .subtle).count == 1)
        #expect(NativeQuickReactionMode.transcript(owner.transcript, mode: .off).count == 2)
        #expect(try String(contentsOf: countFile, encoding: .utf8) == "0")
        let enumeration = try #require(FileManager.default.enumerator(at: root.appendingPathComponent("state"), includingPropertiesForKeys: nil))
        let file = try #require(enumeration.allObjects.compactMap { $0 as? URL }.first { $0.deletingLastPathComponent().lastPathComponent == "dialogue" && $0.pathExtension == "json" })
        let retained = try JSONDecoder().decode(JSONValue.self, from: Data(contentsOf: file))
        #expect(retained["version"] == .number(1) && retained["state"].string == "answered" && retained["decision"]["reaction"].string == "appreciation")
        #expect(retained["response"].string == "You're welcome." && retained["id"].string == reaction.turn)
        owner.editDraft("Keep this unsent draft")
        #expect(await sessions.shutdown(confirmed: true))
        let restored = ManagedSessionCoordinator(configuration: configuration, nativeDraftDirectory: native, profile: "reaction"); reopened = restored
        await restored.start(); try await wait { restored.current.isConnected }
        #expect(restored.current.conversationID == conversation && restored.current.draftText == "Keep this unsent draft")
        #expect(restored.current.transcript.first?.messageReaction == reaction)
        #expect(restored.current.generation != generation && restored.ownerCount == 1)
        #expect(try String(contentsOf: countFile, encoding: .utf8) == "0")
        #expect(await restored.shutdown(confirmed: true))
        if ProcessInfo.processInfo.environment["GATOR_REACTION_CAPTURE"] == "1" {
            let destination = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("reactions/evidence/p04e-2/real-go-reaction.json")
            let evidence: [String: Any] = ["conversation": conversation, "turn": reaction.turn, "revision": reaction.revision, "modelRequests": 0, "ownerCount": 1, "retainedVersion": 1, "restoredReaction": true, "restoredDraft": true, "admissionToReactionMilliseconds": Double(latency.components.seconds) * 1000 + Double(latency.components.attoseconds) / 1e15]
            try JSONSerialization.data(withJSONObject: evidence, options: [.prettyPrinted, .sortedKeys]).write(to: destination)
        }
    } catch {
        _ = await sessions.shutdown(confirmed: true)
        if let reopened { _ = await reopened.shutdown(confirmed: true) }
        throw error
    }
}
