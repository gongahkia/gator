import Foundation
import Testing
import Darwin
import CoreGraphics
import ImageIO
@testable import GatorCore

/// Reproducible headless measurement, not an arbitrary timing assertion.
@Test func nativePresentationBenchmark() throws {
    var rows: [[String: Any]] = []
    for count in [50, 500, 2000] {
        let messages: [JSONValue] = (0..<count).map { i in .object(["role": .string(i % 2 == 0 ? "You" : "Gator"), "text": .string("Message \(i): " + String(repeating: "Readable assistant context and retained evidence. ", count: 8)), "created_at": .string("2026-11-16T05:00:00.123456Z")]) }
        let value: JSONValue = .object(["conversation_id": .string("benchmark"), "messages": .array(messages)])
        let bytes = try JSONEncoder().encode(value)
        var decoding: [Double] = [], projecting: [Double] = []
        for _ in 0..<7 {
            try autoreleasepool {
                var start = Date.timeIntervalSinceReferenceDate
                let decoded = try JSONDecoder().decode(JSONValue.self, from: bytes)
                decoding.append((Date.timeIntervalSinceReferenceDate - start) * 1000)
                start = Date.timeIntervalSinceReferenceDate
                let transcript = BridgeSnapshot(decoded).transcript
                projecting.append((Date.timeIntervalSinceReferenceDate - start) * 1000)
                #expect(transcript.count == count)
            }
        }
        rows.append(["messages": count, "bytes": bytes.count, "decodeMedianMS": decoding.sorted()[3], "transcriptMedianMS": projecting.sorted()[3]])
    }
    var usage = rusage(); getrusage(RUSAGE_SELF, &usage)
    let result: [String: Any] = ["kind": "native-presentation", "samples": 7, "fixtures": rows, "peakRSSBytes": usage.ru_maxrss]
    print("GATOR_BENCHMARK " + String(decoding: try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys]), as: UTF8.self))
}

final class PerformanceOwnerPool: @unchecked Sendable {
    private let lock = NSLock(); private var values: [GoBridgeConnection] = []
    func next() -> any BridgeTransport { lock.withLock { let go = GoBridgeConnection(); values.append(go); return go } }
    var connections: [GoBridgeConnection] { lock.withLock { values } }
}
@Test @MainActor func nativeOwnerStartupBenchmark() async throws {
    let fixture = try await NativeAttachmentFixture()
    do {
        #expect(await fixture.app.shutdown())
        let pool = PerformanceOwnerPool(), store = NativeDraftStore(directory: fixture.root.appendingPathComponent("performance"), profile: "performance")
        var samples: [Double] = []
        for _ in 0..<4 {
            let app = ManagedSessionCoordinator(configuration: fixture.configuration, store: store, factory: { pool.next() })
            let start = Date.timeIntervalSinceReferenceDate; await app.start(); #expect(app.current.isConnected)
            samples.append((Date.timeIntervalSinceReferenceDate - start) * 1000)
            #expect(await app.shutdown())
        }
        let app = ManagedSessionCoordinator(configuration: fixture.configuration, store: store, factory: { pool.next() })
        await app.start(); for _ in 0..<3 { await app.newConversation() }
        #expect(app.ownerCount == 4); #expect(app.workingCount == 0)
        try await Task.sleep(for: .seconds(2))
        var pids: [String] = []
        for connection in pool.connections { if let pid = await connection.processIdentifier() { pids.append(String(pid)) } }
        let ps = Process(), output = Pipe(); ps.executableURL = URL(fileURLWithPath: "/bin/ps")
        ps.arguments = ["-p", pids.joined(separator: ","), "-o", "pid=,rss=,%cpu="]; ps.standardOutput = output
        try ps.run(); let bytes = output.fileHandleForReading.readDataToEndOfFile(); ps.waitUntilExit()
        print("GATOR_OWNER_BENCHMARK", String(decoding: try JSONSerialization.data(withJSONObject: ["timeToConnectedMS": samples, "cachedOwners": app.ownerCount, "idlePSSamplePID_RSSKiB_CPUPercent": String(decoding: bytes, as: UTF8.self)], options: [.sortedKeys]), as: UTF8.self))
        #expect(await app.shutdown()); #expect(fixture.trace().isEmpty)
        await fixture.close()
    } catch { await fixture.close(); throw error }
}

@Test func nativeThumbnailBenchmark() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let width = 512, height = 512
    var pixels = [UInt8](repeating: 0, count: width * height * 4), seed: UInt32 = 17
    for i in pixels.indices { seed = 1664525 &* seed &+ 1013904223; pixels[i] = i % 4 == 3 ? 255 : UInt8(truncatingIfNeeded: seed >> 24) }
    let data = Data(pixels), provider = try #require(CGDataProvider(data: data as CFData))
    let image = try #require(CGImage(width: width, height: height, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: width * 4, space: CGColorSpaceCreateDeviceRGB(), bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.last.rawValue), provider: provider, decode: nil, shouldInterpolate: false, intent: .defaultIntent))
    let bytes = NSMutableData(), destination = try #require(CGImageDestinationCreateWithData(bytes, "public.png" as CFString, 1, nil))
    CGImageDestinationAddImage(destination, image, nil); #expect(CGImageDestinationFinalize(destination))
    let source = root.appendingPathComponent("thumbnail.png"); try (bytes as Data).write(to: source)
    let store = NativeUploadStore(root: root.appendingPathComponent("uploads"))
    let files = try await store.importFiles(Array(repeating: source, count: 12), existing: [])
    let start = Date.timeIntervalSinceReferenceDate
    var sizes: [Int] = []
    for file in files { sizes.append(try await store.thumbnail(file).count) }
    var usage = rusage(); getrusage(RUSAGE_SELF, &usage)
    print("GATOR_THUMBNAIL_BENCHMARK", "files=12 sourcePixels=512x512 stagedBytes=\(files.reduce(0) { $0 + $1.size }) thumbnailBytes=\(sizes.reduce(0,+)) elapsedMS=\((Date.timeIntervalSinceReferenceDate - start) * 1000) peakRSSBytes=\(usage.ru_maxrss)")
    #expect(sizes.count == 12)
}
