import Foundation
import Observation
import Synchronization
import Testing
@testable import GatorCore

@Test @MainActor func terminalSpinnerFramesDoNotInvalidateNativeStateOrOwnerCallbacks() async throws {
    let root = try temporaryDirectory()
    defer { try? FileManager.default.removeItem(at: root) }
    let transport = FixtureTransport(initial: sampleState(["loading_frame": .number(0)]))
    let application = try await fixtureApp(root, transport: transport)
    let initial = try #require(application.snapshot)
    let invalidations = Mutex(0)
    var callbacks = 0
    application.changed = { _, _ in callbacks += 1 }
    withObservationTracking { _ = application.snapshot } onChange: { invalidations.withLock { $0 += 1 } }
    for index in 1...100 {
        var fields = try #require(initial.value.objectFields)
        fields["loading_frame"] = .number(Decimal(index))
        application.receive(.state(BridgeSnapshot(.object(fields))), generation: application.generation)
    }
    #expect(invalidations.withLock { $0 } == 0)
    #expect(callbacks == 0)
    #expect(application.snapshot == initial)
    _ = await application.shutdown(confirmed: true)
}

@Test func nativeSnapshotComparisonRetainsEveryNonSpinnerField() {
    let baseline: [String: JSONValue] = ["loading_frame": .number(1), "elapsed": .string("1s"), "pending_approval": .object(["id": .number(1), "token": .string("exact")])]
    let before = BridgeSnapshot(.object(baseline))
    var spinner = baseline; spinner["loading_frame"] = .number(2)
    #expect(before.hasSameNativeContent(as: BridgeSnapshot(.object(spinner))))
    let composed = BridgeSnapshot(.object(["source": .string("/fixture/caf\u{e9}")]))
    let decomposed = BridgeSnapshot(.object(["source": .string("/fixture/cafe\u{301}")]))
    #expect(!composed.hasSameNativeContent(as: decomposed))
    let differentKeyBytes = BridgeSnapshot(.object(["K": .bool(true)]))
    #expect(!differentKeyBytes.hasSameNativeContent(as: BridgeSnapshot(.object(["\u{212a}": .bool(true)]))))
    for (key, value) in ["elapsed": JSONValue.string("2s"), "pending_approval": .null, "messages": .array([.object(["text": .string("Public update")])]), "session_request": .object(["new": .bool(true)]), "native_provider_result": .object(["request_id": .string("new")]), "unknown_future_field": .bool(true)] {
        var changed = spinner; changed[key] = value
        #expect(!before.hasSameNativeContent(as: BridgeSnapshot(.object(changed))))
    }
}

private extension JSONValue {
    var objectFields: [String: JSONValue]? { if case .object(let fields) = self { return fields }; return nil }
}

@Test @MainActor func meaningfulSnapshotsStillPublishAndExpireExactReviewsAfterSpinnerTicks() async throws {
    let root = try temporaryDirectory()
    defer { try? FileManager.default.removeItem(at: root) }
    let transport = FixtureTransport(initial: sampleState(["loading_frame": .number(0)]))
    let application = try await fixtureApp(root, transport: transport)
    var fields = try #require(application.snapshot?.value.objectFields)
    fields["loading_frame"] = .number(1); fields["elapsed"] = .string("2s")
    application.receive(.state(BridgeSnapshot(.object(fields))), generation: application.generation)
    #expect(application.snapshot?.value["elapsed"].string == "2s")
    let token = String(repeating: "a", count: 64)
    fields["pending_approval"] = .object(["id": .number(7), "token": .string(token), "message_index": .number(0)])
    fields["messages"] = .array([.object(["role": .string("Approval"), "result": .object(["kind": .string("approval"), "summary": .string("Exact fixture review")])])])
    application.receive(.state(BridgeSnapshot(.object(fields))), generation: application.generation)
    let old = try #require(application.currentApprovalReview)
    fields["pending_approval"] = .object(["id": .number(7), "token": .string(String(repeating: "b", count: 64)), "message_index": .number(0)])
    application.receive(.state(BridgeSnapshot(.object(fields))), generation: application.generation)
    #expect(!application.isCurrentReview(old))
    await application.respondToApproval(old, approved: true)
    #expect(await transport.sent().filter { $0["type"].string == "approval" }.isEmpty)
    fields["unknown_future_result"] = .object(["request_id": .string("exact-new-result")])
    application.receive(.state(BridgeSnapshot(.object(fields))), generation: application.generation)
    #expect(application.snapshot?.value["unknown_future_result"]["request_id"].string == "exact-new-result")
    _ = await application.shutdown(confirmed: true)
}

@Test @MainActor func identicalStateCanReconnectWithoutLosingTheOwnerGenerationGuard() async throws {
    let root = try temporaryDirectory()
    defer { try? FileManager.default.removeItem(at: root) }
    let application = try await fixtureApp(root, transport: FixtureTransport(initial: sampleState()))
    let snapshot = try #require(application.snapshot), generation = application.generation
    application.receive(.disconnected("Fixture disconnect"), generation: generation)
    #expect(!application.isConnected)
    application.receive(.state(snapshot), generation: UUID())
    #expect(!application.isConnected)
    application.receive(.state(snapshot), generation: generation)
    #expect(application.isConnected)
    _ = await application.shutdown(confirmed: true)
}

@Test @MainActor func realGoSpinnerOnlySnapshotsKeepExactPendingReviewUndecided() async throws {
    let fixture = try await NativeReviewFixture(action: false, providerFixture: "dev-acceptance-provider")
    do {
        let parent = fixture.app.current
        try await fixture.send("Hold unrelated work")
        try await managedEventually("original task held") { parent.snapshot?.running == true && parent.snapshot?.value["dialogue_active"].bool == true }
        try await fixture.send("Begin inert pending approval")
        try await managedEventually("Go pending review") { fixture.app.owners.values.contains { $0 !== parent && $0.currentApprovalReview != nil } }
        let owner = try #require(fixture.app.owners.values.first { $0 !== parent && $0.currentApprovalReview != nil })
        await fixture.app.openConversation(owner.conversationID)
        owner.openApprovalReview()
        let review = try #require(owner.review), snapshot = try #require(owner.snapshot)
        let directory = fixture.root.appendingPathComponent("state/gator/interactions")
        let records = try #require(FileManager.default.enumerator(at: directory, includingPropertiesForKeys: nil)).allObjects.compactMap { $0 as? URL }.filter { $0.pathExtension == "json" }
        let recordURL = try #require(records.first), before = try Data(contentsOf: recordURL)
        let record = try JSONDecoder().decode(JSONValue.self, from: before)
        #expect(record["status"].string == "pending"); #expect(record["approved"] == .null)
        let previousCallback = owner.changed
        var callbacks = 0
        owner.changed = { app, prior in callbacks += 1; previousCallback?(app, prior) }
        for index in 1...100 {
            var fields = try #require(snapshot.value.objectFields)
            fields["loading_frame"] = .number(Decimal(index))
            owner.receive(.state(BridgeSnapshot(.object(fields))), generation: review.generation)
        }
        owner.changed = previousCallback
        #expect(callbacks == 0)
        #expect(owner.review == review); #expect(owner.isCurrentReview(review))
        #expect(try Data(contentsOf: recordURL) == before)
        #expect(fixture.app.ownerCount == 2)
        let shutDown = await fixture.app.shutdown(confirmed: true)
        #expect(shutDown)
        #expect(try Data(contentsOf: recordURL) == before)
        print("P04E1R2_HEADLESS_PENDING owner=\(review.conversationID) generation=\(review.generation) interaction=\(review.interactionID) callbacks=\(callbacks) durablePendingUnchanged=true shutdown=\(shutDown) nativeDesktopCycle=false")
        await fixture.close()
    } catch {
        await fixture.close()
        throw error
    }
}
