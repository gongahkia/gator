import Foundation
import Testing
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers
@testable import GatorCore

func attachmentPNG() throws -> Data {
    let space = CGColorSpaceCreateDeviceRGB()
    let context = try #require(CGContext(data: nil, width: 8, height: 8, bitsPerComponent: 8, bytesPerRow: 32, space: space, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue))
    context.setFillColor(CGColor(red: 0.1, green: 0.5, blue: 0.3, alpha: 1)); context.fill(CGRect(x: 0, y: 0, width: 8, height: 8))
    let image = try #require(context.makeImage()), bytes = NSMutableData()
    let destination = try #require(CGImageDestinationCreateWithData(bytes, UTType.png.identifier as CFString, 1, nil))
    CGImageDestinationAddImage(destination, image, nil); #expect(CGImageDestinationFinalize(destination)); return bytes as Data
}
func uploadFile(_ root: URL, _ name: String = "notes.md", _ content: Data = Data("Original jade heron".utf8)) throws -> URL {
    let path = root.appendingPathComponent(name); try content.write(to: path); return path
}
@Test func nativeCopiesArePrivateImmutableUniqueAndAtomic() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let storage = root.appendingPathComponent("uploads"), store = NativeUploadStore(root: storage), source = try uploadFile(root)
    let first = try await store.importFiles([source, source], existing: [])
    #expect(first.count == 2); #expect(first[0].id != first[1].id)
    #expect(first[0].name == "notes.md"); #expect(first.allSatisfy { $0.valid })
    #expect(try Data(contentsOf: source) == Data("Original jade heron".utf8))
    try Data("Changed source".utf8).write(to: source); try FileManager.default.removeItem(at: source)
    try await store.check(first)
    #expect(try Data(contentsOf: storage.appendingPathComponent(first[0].id)) == Data("Original jade heron".utf8))
    for (url, mode) in [(storage, 0o700), (storage.appendingPathComponent(String(first[0].id.prefix(32))), 0o700), (storage.appendingPathComponent(first[0].id), 0o600)] {
        #expect((try FileManager.default.attributesOfItem(atPath: url.path)[.posixPermissions] as? NSNumber)?.intValue == mode)
    }
    let valid = try uploadFile(root, "later.txt"), bad = try uploadFile(root, "program.exe")
    await #expect(throws: (any Error).self) { try await store.importFiles([valid, bad], existing: first) }
    #expect(try FileManager.default.contentsOfDirectory(atPath: storage.path).count == 2)
    try await store.check(first)
}
@Test(arguments: ["empty", "document", "image", "extension", "filename", "count", "total"])
func nativeUploadLimitsRejectInvalidSelections(_ kind: String) async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeUploadStore(root: root.appendingPathComponent("uploads"))
    let size = kind == "document" ? NativeUploadLimits.document + 1 : kind == "image" ? NativeUploadLimits.image + 1 : kind == "total" ? 6 * 1024 * 1024 : kind == "empty" ? 0 : 1
    let name = kind == "image" ? "huge.png" : kind == "filename" ? "bad\nname.txt" : kind == "extension" ? "program.app" : "file.txt"
    let file = try uploadFile(root, name, Data(repeating: 65, count: size))
    let files = Array(repeating: file, count: kind == "count" ? 13 : kind == "total" ? 3 : 1)
    await #expect(throws: (any Error).self) { try await store.importFiles(files, existing: []) }
    if FileManager.default.fileExists(atPath: root.appendingPathComponent("uploads").path) { #expect(try FileManager.default.contentsOfDirectory(atPath: root.appendingPathComponent("uploads").path).isEmpty) }
}
@Test func privateRootDirectoriesAndFilesRejectSymlinksAndChangedBytes() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let outside = root.appendingPathComponent("outside"); try FileManager.default.createDirectory(at: outside, withIntermediateDirectories: true)
    let badRoot = root.appendingPathComponent("linked"); try FileManager.default.createSymbolicLink(at: badRoot, withDestinationURL: outside)
    let file = try uploadFile(root)
    await #expect(throws: (any Error).self) { try await NativeUploadStore(root: badRoot).importFiles([file], existing: []) }
    let sourceLink = root.appendingPathComponent("link.md"); try FileManager.default.createSymbolicLink(at: sourceLink, withDestinationURL: file)
    let store = NativeUploadStore(root: root.appendingPathComponent("uploads"))
    await #expect(throws: (any Error).self) { try await store.importFiles([sourceLink], existing: []) }
    let uploaded = try await store.importFiles([file], existing: [])[0], copy = root.appendingPathComponent("uploads").appendingPathComponent(uploaded.id)
    try Data(repeating: 65, count: uploaded.size).write(to: copy)
    await #expect(throws: (any Error).self) { try await store.check([uploaded]) }
    try FileManager.default.removeItem(at: copy); try FileManager.default.createSymbolicLink(at: copy, withDestinationURL: file)
    await #expect(throws: (any Error).self) { try await store.check([uploaded]) }
    let directory = copy.deletingLastPathComponent(); try FileManager.default.removeItem(at: directory)
    try FileManager.default.createSymbolicLink(at: directory, withDestinationURL: root)
    await #expect(throws: (any Error).self) { try await store.check([uploaded]) }
    #expect(!NativeUploadLimits.validID("../notes.md")); #expect(!NativeUploadLimits.validID(String(repeating: "a", count: 32) + "/../notes.md"))
    #expect(try FileManager.default.contentsOfDirectory(atPath: outside.path).isEmpty)
}
@Test func imagesDecodeBoundedlyAndPasteNormalizesWithoutOriginalPaths() async throws {
    let root = try temporaryDirectory(); defer { try? FileManager.default.removeItem(at: root) }
    let store = NativeUploadStore(root: root.appendingPathComponent("uploads")), png = try attachmentPNG()
    let image = try uploadFile(root, "image.png", png)
    let file = try await store.importFiles([image], existing: [])[0]
    #expect(file.isImage); #expect(try await store.thumbnail(file).count > 0)
    let pasted = try await store.importPastedImage(png, existing: [file])
    #expect(pasted[0].name == "Pasted image.png"); try await store.check(pasted)
    await #expect(throws: (any Error).self) { try await store.importPastedImage(Data("invalid".utf8), existing: []) }
    let bad = try uploadFile(root, "fake.png", Data("not pixels".utf8))
    await #expect(throws: (any Error).self) { try await store.importFiles([bad], existing: []) }
    var huge = png
    for offset in [16, 20] { huge.replaceSubrange(offset..<(offset + 4), with: [UInt8(0), 0, 0x20, 0]) }
    await #expect(throws: (any Error).self) { try await store.importPastedImage(huge, existing: []) }
}
@Test func clipboardPolicyKeepsTextAndFileURLsSeparateFromImageIntent() {
    #expect(NativeClipboardPolicy.pasteImage(hasText: false, hasFileURL: false, hasImage: true, explicitlyImage: false))
    #expect(!NativeClipboardPolicy.pasteImage(hasText: true, hasFileURL: false, hasImage: true, explicitlyImage: false))
    #expect(NativeClipboardPolicy.pasteImage(hasText: true, hasFileURL: false, hasImage: true, explicitlyImage: true))
    #expect(!NativeClipboardPolicy.pasteImage(hasText: false, hasFileURL: true, hasImage: true, explicitlyImage: true))
}
