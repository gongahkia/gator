import Foundation
import Testing
@testable import GatorCore

@Test @MainActor func duplicateDocumentBasenamesPreserveDistinctBytesAndFailGoAdmissionWithoutExecution() async throws {
    let f = try await NativeAttachmentFixture()
    do {
        let a = f.root.appendingPathComponent("a"), b = f.root.appendingPathComponent("b")
        for directory in [a, b] { try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true) }
        let first = try uploadFile(a, "report.md", Data("First independent original".utf8))
        let second = try uploadFile(b, "report.md", Data("Second independent original".utf8))
        let owner = f.app.current
        try await attach(owner, [first, second])
        let files = owner.draftUploads
        #expect(files.map(\.name) == ["report.md", "report.md"])
        #expect(files[0].id != files[1].id); #expect(files[0].sha256 != files[1].sha256)
        owner.editDraft("Read both independently selected reports")
        await f.app.submit(from: owner)
        try await managedEventually("authoritative duplicate display-name rejection") { owner.submission?.phase == .failed }
        #expect(owner.issue.contains("supplied more than once"))
        #expect(owner.draftUploads == files)
        #expect(owner.draftText == "Read both independently selected reports")
        #expect(f.trace().isEmpty)
        for (file, expected) in zip(files, ["First independent original", "Second independent original"]) {
            #expect(try String(contentsOf: f.configuration.nativeUploadDirectory.appendingPathComponent(file.id), encoding: .utf8) == expected)
        }
        owner.removeAttachment(files[0].id)
        #expect(owner.draftUploads == [files[1]])
        #expect(try String(contentsOf: first, encoding: .utf8) == "First independent original")
        #expect(try String(contentsOf: second, encoding: .utf8) == "Second independent original")
        await f.close()
    } catch { await f.close(); throw error }
}

/// Production framing and application-state replacement, with no visible UI.
/// Reports measured cost; it does not invent an animation/frame-rate threshold.
@Test @MainActor func nativeStreamingSnapshotHandlingBenchmark() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let transport = FixtureTransport(), app = try await fixtureApp(root, transport: transport)
    let messages: [JSONValue] = (0..<500).map { .object(["role": .string($0 % 2 == 0 ? "You" : "Gator"), "text": .string("Retained message \($0): " + String(repeating: "context ", count: 12))]) }
    var frames = Data()
    for index in 1...30 {
        guard case .object(var fields) = sampleState().value else { throw BridgeFailure("Invalid controlled snapshot") }
        fields["messages"] = .array(messages)
        fields["activity"] = .object(["text": .string(String(repeating: "stream ", count: index))])
        fields["running"] = .bool(true)
        var bytes = try JSONEncoder().encode(JSONValue.object(["version": .number(1), "type": .string("state"), "state": .object(fields)])); bytes.append(10); frames.append(bytes)
    }
    var samples: [Double] = []
    for _ in 0..<5 {
        var framer = BridgeFramer(), count = 0
        let start = Date.timeIntervalSinceReferenceDate
        for offset in stride(from: 0, to: frames.count, by: 16384) {
            for event in try framer.append(Data(frames[offset..<min(offset + 16384, frames.count)])) {
                app.receive(event, generation: app.generation); count += 1
            }
        }
        try framer.finish()
        samples.append((Date.timeIntervalSinceReferenceDate - start) * 1000)
        #expect(count == 30); #expect(app.messages.count == 500)
        #expect(app.snapshot?.value["activity"]["text"].string == String(repeating: "stream ", count: 30))
    }
    print("GATOR_STREAM_BENCHMARK", "samples=5 snapshots=30 retainedMessages=500 wireBytes=\(frames.count) medianBatchMS=\(samples.sorted()[2]) medianPerSnapshotMS=\(samples.sorted()[2] / 30)")
    #expect(await app.shutdown(confirmed: true))
}
