import Foundation
import Testing
import Darwin
@testable import GatorCore

func safetyWait(_ condition: @Sendable () -> Bool, seconds: Int = 10) async throws {
    let deadline = ContinuousClock.now + .seconds(seconds)
    while !condition() {
        guard ContinuousClock.now < deadline else { throw BridgeFailure("Safety fixture condition timed out") }
        try await Task.sleep(for: .milliseconds(5))
    }
}
actor NativeArchiveHelper {
    static let shared = NativeArchiveHelper()
    private var executable: URL?
    func binary() async throws -> URL {
        if let executable { return executable }
        let directory = try temporaryDirectory(), binary = directory.appendingPathComponent("archive-writer")
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let sources = try FileManager.default.contentsOfDirectory(at: root.appendingPathComponent("Sources/GatorCore"), includingPropertiesForKeys: nil).filter { $0.pathExtension == "swift" }.sorted { $0.path < $1.path }
        let fixture = try #require(Bundle.module.url(forResource: "native-archive-writer", withExtension: "swift", subdirectory: "Fixtures"))
        let log = directory.appendingPathComponent("build.log"); try Data().write(to: log)
        let handle = try FileHandle(forWritingTo: log); defer { try? handle.close() }
        let compiler = Process(); compiler.executableURL = URL(fileURLWithPath: "/usr/bin/xcrun")
        compiler.arguments = ["swiftc", "-swift-version", "6", "-parse-as-library"] + sources.map(\.path) + [fixture.path, "-o", binary.path]
        compiler.standardOutput = handle; compiler.standardError = handle
        try compiler.run()
        do { try await safetyWait({ !compiler.isRunning }, seconds: 45) }
        catch { compiler.terminate(); throw error }
        guard compiler.terminationStatus == 0 else { throw BridgeFailure(String(decoding: try Data(contentsOf: log).prefix(8000), as: UTF8.self)) }
        executable = binary; return binary
    }
}
func archiveChild(_ root: URL, name: String, mode: String = "write") async throws -> Process {
    let child = Process(); child.executableURL = try await NativeArchiveHelper.shared.binary()
    child.arguments = [root.path, name, mode]
    child.standardOutput = FileHandle.nullDevice; child.standardError = FileHandle.nullDevice
    try child.run(); return child
}
func reapArchiveChild(_ child: Process) async throws {
    try await safetyWait { !child.isRunning }
    #expect(child.terminationStatus == 0)
    #expect(kill(child.processIdentifier, 0) != 0)
}

@Test func safetySixteenIndependentActorsPreserveEveryDistinctDraftAndSelection() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let directory = root.appendingPathComponent("native")
    try await withThrowingTaskGroup(of: Void.self) { group in
        for index in 0..<16 {
            group.addTask {
                let store = NativeDraftStore(directory: directory, profile: "safety")
                var archive = try await store.load()
                let key = "conversation:owner-\(index)"
                archive.drafts[key] = NativeDraft(text: "distinct bytes \(index)")
                try await store.saveOwner(archive, context: key)
            }
        }
        group.addTask { try await NativeDraftStore(directory: directory, profile: "safety").selectConversation("chosen") }
        try await group.waitForAll()
    }
    let store = NativeDraftStore(directory: directory, profile: "safety"), result = try await store.load()
    #expect(result.drafts.count == 16); #expect(result.selectedConversationID == "chosen")
    for index in 0..<16 { #expect(result.drafts["conversation:owner-\(index)"]?.text == "distinct bytes \(index)") }
    for path in [await store.file, URL(fileURLWithPath: await store.file.path + ".lock")] {
        #expect((try FileManager.default.attributesOfItem(atPath: path.path)[.posixPermissions] as? NSNumber)?.intValue == 0o600)
    }
}

@Test func safetyCrossProcessTransactionsPreserveSixteenLoadedBeforeWriteSnapshots() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    var children: [Process] = []
    defer { for child in children where child.isRunning { child.terminate() } }
    for index in 0..<16 { children.append(try await archiveChild(root, name: "owner-\(index)")) }
    try await safetyWait { (0..<16).allSatisfy { FileManager.default.fileExists(atPath: root.appendingPathComponent("owner-\($0).ready").path) } }
    try Data().write(to: root.appendingPathComponent("start"))
    async let selection: Void = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "safety").selectConversation("selected-after-gate")
    for child in children { try await reapArchiveChild(child) }
    try await selection
    let result = try await NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "safety").load()
    #expect(result.drafts.count == 16); #expect(result.selectedConversationID == "selected-after-gate")
    for index in 0..<16 { #expect(result.drafts["conversation:owner-\(index)"]?.text == "child owner-\(index)") }
}

@Test func safetyStaleSameContextWritersFailInsteadOfReplacingNewerDrafts() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let first = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "safety")
    let second = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "safety")
    var seed = NativeArchive(); seed.drafts["conversation:shared"] = NativeDraft(text: "original")
    try await first.save(seed)
    var a = try await first.load(), b = try await second.load()
    a.drafts["conversation:shared"] = NativeDraft(text: "newer accepted writer")
    b.drafts["conversation:shared"] = NativeDraft(text: "stale proposed writer")
    try await first.saveOwner(a, context: "conversation:shared")
    await #expect(throws: (any Error).self) { try await second.saveOwner(b, context: "conversation:shared") }
    await #expect(throws: (any Error).self) { try await second.save(b) }
    #expect(try await first.load().drafts["conversation:shared"]?.text == "newer accepted writer")
    #expect(b.drafts["conversation:shared"]?.text == "stale proposed writer")
    let child = try await archiveChild(root, name: "shared", mode: "conflict")
    defer { if child.isRunning { child.terminate() } }
    try await safetyWait { FileManager.default.fileExists(atPath: root.appendingPathComponent("shared.ready").path) }
    var next = try await first.load(); next.drafts["conversation:shared"] = NativeDraft(text: "newer than child load")
    try await first.saveOwner(next, context: "conversation:shared")
    try Data().write(to: root.appendingPathComponent("start")); try await reapArchiveChild(child)
    #expect(try String(contentsOf: root.appendingPathComponent("shared.result"), encoding: .utf8) == "conflict")
    #expect(try await first.load().drafts["conversation:shared"]?.text == "newer than child load")
}

@Test func safetyFullSnapshotSaveMergesUnrelatedContextsAndRejectsStaleSelection() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "safety")
    var old = try await store.load()
    var other = NativeArchive(); other.drafts["conversation:other"] = NativeDraft(text: "retain other")
    try await store.saveOwner(other, context: "conversation:other")
    try await store.selectConversation("other")
    old.drafts["conversation:mine"] = NativeDraft(text: "retain mine")
    try await store.save(old)
    let merged = try await store.load()
    #expect(merged.drafts.count == 2); #expect(merged.drafts["conversation:other"]?.text == "retain other")
    #expect(merged.selectedConversationID == "other")
    old.selectedConversationID = "stale-selection"
    await #expect(throws: (any Error).self) { try await store.save(old) }
    #expect(try await store.load().selectedConversationID == "other")
}

@Test func safetyUnrelatedWritesPreservePendingPlanRecoveryReceiptAndApprovalCorrectionIdentity() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "safety"), context = "conversation:owner"
    let identity = try #require(NativePlanIdentity(conversationID: "owner", summary: planSummary()))
    let token = String(repeating: "a", count: 64), generation = UUID()
    let target = NativeCorrectionTarget(kind: .action, ownerID: "owner", generation: generation, reviewToken: token, plan: nil, taskTurnID: "turn")
    var seed = NativeArchive(); seed.drafts[context] = NativeDraft(text: "kept", correction: NativeCorrectionDraft(target: target))
    let draft = try #require(seed.drafts[context])
    seed.pendingByContext = [context: PendingSubmission(id: "submission-id", context: context, text: "kept", draftRevision: draft.revision, phase: .submitted)]
    seed.planOperationsByContext = [context: NativePlanOperation(id: "plan-id", nonce: "plan-nonce", identity: identity, generation: generation, action: .approve)]
    seed.receiptOperationsByContext = [context: NativeReceiptOperation(id: "receipt-id", nonce: "receipt-nonce", identity: NativeReceiptIdentity(conversationID: "owner", runID: "run", actionID: "action-" + String(repeating: "b", count: 16), correlationID: String(repeating: "c", count: 32), manifestSHA256: token), generation: generation, action: .inspect)]
    seed.recoveryPreparationsByContext = [context: NativeRecoveryPreparation(id: "recovery-id", nonce: "recovery-nonce", conversationID: "owner", taskID: identity.taskID, token: token, generation: generation, action: .prepareRetry, connections: nil, accessToken: nil, previousRevision: 1)]
    try await store.save(seed)
    var other = NativeArchive(); other.drafts["conversation:other"] = NativeDraft(text: "other")
    try await store.saveOwner(other, context: "conversation:other"); try await store.selectConversation("other")
    let saved = try await store.load()
    #expect(saved.pendingByContext?[context]?.id == "submission-id")
    #expect(saved.planOperationsByContext?[context]?.identity == identity)
    #expect(saved.planOperationsByContext?[context]?.id == "plan-id")
    #expect(saved.receiptOperationsByContext?[context]?.id == "receipt-id")
    #expect(saved.recoveryPreparationsByContext?[context]?.token == token)
    #expect(saved.recoveryPreparationsByContext?[context]?.id == "recovery-id")
    #expect(saved.drafts[context]?.correction?.target == target)
}

@Test(arguments: [NativeArchiveWriteStage.beforeReplace, .afterReplace])
func safetyInterruptedReplacementKeepsCompleteArchiveAndReleasesProcessLock(_ stage: NativeArchiveWriteStage) async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root.appendingPathComponent("native"), profile: "safety")
    var seed = NativeArchive(); seed.drafts["conversation:original"] = NativeDraft(text: "survives interruption")
    try await store.save(seed)
    try Data().write(to: root.appendingPathComponent("start"))
    let child = try await archiveChild(root, name: "interrupted", mode: stage == .beforeReplace ? "beforeReplace" : "afterReplace")
    defer { if child.isRunning { kill(child.processIdentifier, SIGKILL) } }
    try await safetyWait { FileManager.default.fileExists(atPath: root.appendingPathComponent("interrupted.stage").path) }
    #expect(kill(child.processIdentifier, SIGKILL) == 0)
    try await safetyWait { !child.isRunning }
    #expect(kill(child.processIdentifier, 0) != 0)
    let result = try await store.load()
    #expect(result.drafts["conversation:original"]?.text == "survives interruption")
    #expect(result.drafts["conversation:interrupted"]?.text == (stage == .afterReplace ? "child interrupted" : nil))
    try await store.selectConversation("lock-released")
    #expect(try await store.load().selectedConversationID == "lock-released")
}

@Test func safetyFailedEncodingOrWriteCannotReplaceTheExistingArchive() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "safety")
    var seed = NativeArchive(); seed.drafts["conversation:kept"] = NativeDraft(text: "original bytes")
    try await store.save(seed)
    let path = await store.file, before = try Data(contentsOf: path)
    var invalid = try await store.load()
    invalid.drafts["conversation:bad"] = NativeDraft(text: String(repeating: "x", count: 128 * 1024 + 1))
    await #expect(throws: (any Error).self) { try await store.save(invalid) }
    #expect(try Data(contentsOf: path) == before)
    var nonfinite = try await store.load()
    nonfinite.drafts["conversation:encoding"] = NativeDraft(text: "not encodable", updated: Date(timeIntervalSinceReferenceDate: .infinity))
    await #expect(throws: (any Error).self) { try await store.save(nonfinite) }
    #expect(try Data(contentsOf: path) == before)
    let failing = NativeDraftStore(directory: root, profile: "safety", writeHook: { stage in if stage == .beforeReplace { throw BridgeFailure("controlled replacement failure") } })
    var update = try await failing.load(); update.drafts["conversation:new"] = NativeDraft(text: "not published")
    await #expect(throws: (any Error).self) { try await failing.save(update) }
    #expect(try Data(contentsOf: path) == before)
    #expect(try FileManager.default.contentsOfDirectory(atPath: root.path).allSatisfy { !$0.hasSuffix(".tmp") })
    try FileManager.default.setAttributes([.posixPermissions: 0o500], ofItemAtPath: root.path)
    defer { try? FileManager.default.setAttributes([.posixPermissions: 0o700], ofItemAtPath: root.path) }
    await #expect(throws: (any Error).self) { try await store.save(update) }
    #expect(try Data(contentsOf: path) == before)
}

@Test(arguments: Array(1...7)) func safetyEveryExistingArchiveVersionMigratesWithoutLosingLegacyPendingIdentity(_ version: Int) async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "safety")
    var seed = NativeArchive(); seed.version = version; seed.drafts["conversation:old"] = NativeDraft(text: "legacy")
    seed.pending = PendingSubmission(id: "legacy-request", context: "conversation:old", text: "legacy", draftRevision: seed.drafts["conversation:old"]!.revision, phase: .submitted)
    try JSONEncoder().encode(seed).write(to: await store.file)
    var loaded = try await store.load(); loaded.drafts["conversation:new"] = NativeDraft(text: "new bytes")
    try await store.saveOwner(loaded, context: "conversation:new")
    let final = try await store.load()
    #expect(final.pending?.id == "legacy-request"); #expect(final.pending?.phase == .uncertain)
    #expect(final.drafts["conversation:old"]?.text == "legacy"); #expect(final.drafts["conversation:new"]?.text == "new bytes")
}

@Test(arguments: ["{broken", "{\"version\":99,\"selectedConversationID\":\"\",\"drafts\":{}}"])
func safetyEveryEntryPointProtectsCorruptAndFutureArchives(_ text: String) async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeDraftStore(directory: root, profile: "safety"), bytes = Data(text.utf8)
    let path = await store.file; try bytes.write(to: path)
    var update = NativeArchive(); update.drafts["conversation:new"] = NativeDraft(text: "not written")
    await #expect(throws: (any Error).self) { try await store.load() }
    await #expect(throws: (any Error).self) { try await store.save(update) }
    await #expect(throws: (any Error).self) { try await store.saveOwner(update, context: "conversation:new") }
    await #expect(throws: (any Error).self) { try await store.selectConversation("new") }
    #expect(try Data(contentsOf: path) == bytes)
}
