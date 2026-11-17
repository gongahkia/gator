import Foundation
import Testing
import Darwin
@testable import GatorCore

struct SafetyBridgeFixture {
    let root: URL
    let transport: GoBridgeConnection
    let pid: Int32
    static func start(_ mode: String) async throws -> Self {
        let root = try temporaryDirectory()
        let original = try #require(Bundle.module.url(forResource: "pipe-lifecycle", withExtension: "py", subdirectory: "Fixtures"))
        let script = root.appendingPathComponent("fixture.py"); try Data(contentsOf: original).write(to: script)
        try FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: script.path)
        let transport = GoBridgeConnection(acknowledgementTimeout: .seconds(2))
        let configuration = BridgeLaunchConfiguration(runtime: script, workspace: root, environment: ["PATH": "/usr/bin:/bin", "HOME": root.path, "GATOR_SAFETY_ROOT": root.path, "GATOR_SAFETY_MODE": mode])
        try await transport.start(configuration)
        return Self(root: root, transport: transport, pid: try #require(await transport.processIdentifier()))
    }
    func close() async {
        try? Data().write(to: root.appendingPathComponent("release"))
        await transport.shutdown()
        if FileManager.default.fileExists(atPath: root.appendingPathComponent("descendant.pid").path) {
            do {
                let child = Int32(try String(contentsOf: root.appendingPathComponent("descendant.pid"), encoding: .utf8)) ?? 0
                if child > 0 { try await safetyWait({ kill(child, 0) != 0 }) }
            } catch { Issue.record("Controlled descendant did not finish after release: \(error)") }
        }
        try? FileManager.default.removeItem(at: root)
    }
}

@Test func safetyCleanExitReapsDirectOwnerAndFinishesAllIO() async throws {
    let fixture = try await SafetyBridgeFixture.start("clean")
    do {
        var events = fixture.transport.events.makeAsyncIterator()
        guard case .state = await events.next() else { throw BridgeFailure("No fixture state") }
        try await fixture.transport.send(.newConversation, nonce: "clean-ack")
        #expect(await events.next() == .acknowledged("clean-ack"))
        #expect(await fixture.transport.shutdownConfirmed())
        #expect(kill(fixture.pid, 0) != 0)
        #expect(await fixture.transport.cleanupIsComplete())
        #expect(FileManager.default.fileExists(atPath: fixture.root.appendingPathComponent("exit.received").path))
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test(arguments: ["stdout", "stderr", "both"])
func safetyInheritedDescriptorsCannotHoldShutdownOrKillDescendants(_ mode: String) async throws {
    let fixture = try await SafetyBridgeFixture.start(mode)
    do {
        var events = fixture.transport.events.makeAsyncIterator()
        guard case .state = await events.next() else { throw BridgeFailure("No fixture state") }
        let child = try #require(Int32(try String(contentsOf: fixture.root.appendingPathComponent("descendant.pid"), encoding: .utf8)))
        #expect(kill(child, 0) == 0)
        let start = ContinuousClock.now
        #expect(await fixture.transport.shutdownConfirmed())
        let elapsed = ContinuousClock.now - start
        print("GATOR_SAFETY_SHUTDOWN mode=\(mode) elapsed=\(elapsed)")
        #expect(elapsed < .seconds(2))
        #expect(kill(fixture.pid, 0) != 0)
        #expect(kill(child, 0) == 0) // releasing our descriptors does not kill a descendant.
        #expect(await fixture.transport.cleanupIsComplete())
        #expect(await fixture.transport.diagnosticByteCount() <= 4096)
        if mode == "stderr" { #expect(await fixture.transport.diagnosticByteCount() == 4096) }
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test func safetyUnexpectedDirectExitIsDetectedDespiteInheritedStdoutAndStderr() async throws {
    let fixture = try await SafetyBridgeFixture.start("both")
    do {
        var events = fixture.transport.events.makeAsyncIterator()
        guard case .state = await events.next() else { throw BridgeFailure("No fixture state") }
        #expect(kill(fixture.pid, SIGKILL) == 0)
        let start = ContinuousClock.now
        guard case .disconnected(let reason) = await events.next() else { throw BridgeFailure("Missing direct-owner disconnect") }
        #expect(ContinuousClock.now - start < .seconds(2)); #expect(!reason.isEmpty)
        #expect(kill(fixture.pid, 0) != 0)
        #expect(await fixture.transport.shutdownConfirmed()); #expect(await fixture.transport.cleanupIsComplete())
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test func safetyPartialFrameAtExitRemainsUnconfirmedEvenThoughProcessIsReaped() async throws {
    let fixture = try await SafetyBridgeFixture.start("partial")
    do {
        var events = fixture.transport.events.makeAsyncIterator()
        guard case .state = await events.next() else { throw BridgeFailure("No fixture state") }
        #expect(await fixture.transport.shutdownConfirmed())
        guard case .disconnected(let reason) = await events.next() else { throw BridgeFailure("Missing partial-frame disconnect") }
        #expect(reason.contains("during a state frame")); #expect(reason.contains("unconfirmed"))
        #expect(kill(fixture.pid, 0) != 0); #expect(await fixture.transport.cleanupIsComplete())
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test func safetyConcurrentShutdownSharesOneExitAndResourceCleanup() async throws {
    let fixture = try await SafetyBridgeFixture.start("clean")
    do {
        var events = fixture.transport.events.makeAsyncIterator()
        guard case .state = await events.next() else { throw BridgeFailure("No fixture state") }
        async let first = fixture.transport.shutdownConfirmed()
        async let second = fixture.transport.shutdownConfirmed()
        #expect(await first); #expect(await second)
        #expect(kill(fixture.pid, 0) != 0); #expect(await fixture.transport.cleanupIsComplete())
        #expect(await fixture.transport.processIdentifier() == nil)
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test func safetyOwnerTerminationWhileStreamingReapsAndReleasesReaderTasks() async throws {
    let fixture = try await SafetyBridgeFixture.start("stream")
    do {
        var events = fixture.transport.events.makeAsyncIterator()
        var states = 0
        while states < 5 {
            guard case .state = await events.next() else { throw BridgeFailure("Streaming fixture ended early") }
            states += 1
        }
        #expect(kill(fixture.pid, SIGTERM) == 0)
        let start = ContinuousClock.now
        while let event = await events.next() { if case .disconnected = event { break } }
        #expect(ContinuousClock.now - start < .seconds(2))
        #expect(await fixture.transport.shutdownConfirmed()); #expect(kill(fixture.pid, 0) != 0)
        #expect(await fixture.transport.cleanupIsComplete())
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test func safetyIndependentOwnersCanShutDownConcurrently() async throws {
    var fixtures: [SafetyBridgeFixture] = []
    do {
        for mode in ["clean", "stdout", "stderr"] {
            let fixture = try await SafetyBridgeFixture.start(mode)
            fixtures.append(fixture)
            var events = fixture.transport.events.makeAsyncIterator()
            guard case .state = await events.next() else { throw BridgeFailure("No fixture state") }
        }
        let start = ContinuousClock.now
        await withTaskGroup(of: Void.self) { group in
            for fixture in fixtures {
                group.addTask {
                    #expect(await fixture.transport.shutdownConfirmed())
                    #expect(kill(fixture.pid, 0) != 0)
                    #expect(await fixture.transport.cleanupIsComplete())
                }
            }
        }
        #expect(ContinuousClock.now - start < .seconds(2))
        for fixture in fixtures { await fixture.close() }
    } catch {
        for fixture in fixtures { await fixture.close() }
        throw error
    }
}
