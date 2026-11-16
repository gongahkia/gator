import Foundation
import CryptoKit
import Security
import ImageIO
import UniformTypeIdentifiers
import Darwin

public struct NativeUpload: Codable, Equatable, Hashable, Sendable, Identifiable {
    public let id, name: String
    public let size: Int
    public let sha256: String
    public var isImage: Bool { NativeUploadLimits.images.contains((name as NSString).pathExtension.lowercased()) }
    public var typeLabel: String { (name as NSString).pathExtension.uppercased() }
    public var valid: Bool {
        let parts = id.split(separator: "/", omittingEmptySubsequences: false)
        return parts.count == 2 && String(parts[0]).range(of: "^[a-f0-9]{32}$", options: .regularExpression) != nil && String(parts[1]) == name && NativeUploadLimits.validName(name) && size > 0 && size <= NativeUploadLimits.limit(name) && sha256.range(of: "^[a-f0-9]{64}$", options: .regularExpression) != nil
    }
}
public enum NativeUploadLimits {
    public static let count = 12, document = 8 * 1024 * 1024, image = 4 * 1024 * 1024, total = 16 * 1024 * 1024
    public static let extensions = "png jpg jpeg webp pdf docx odt xlsx pptx txt md markdown rst csv tsv json jsonl yaml yml toml xml html htm log ini cfg conf js jsx ts tsx py go rs java c h cpp hpp css scss sh bash zsh sql rb swift kt vue svelte tex r".split(separator: " ").map(String.init)
    public static let images: Set<String> = ["png", "jpg", "jpeg", "webp"]
    public static let hint = "12 files · documents 8 MiB · images 4 MiB · 16 MiB total"
    public static func validID(_ id: String) -> Bool {
        let parts = id.split(separator: "/", omittingEmptySubsequences: false)
        return parts.count == 2 && String(parts[0]).range(of: "^[a-f0-9]{32}$", options: .regularExpression) != nil && validName(String(parts[1]))
    }
    public static func limit(_ name: String) -> Int { images.contains((name as NSString).pathExtension.lowercased()) ? image : document }
    public static func validName(_ name: String) -> Bool {
        !name.isEmpty && name.utf16.count <= 200 && name != "." && name != ".." && !name.unicodeScalars.contains { $0.value < 32 || $0.value == 127 || $0 == "/" || $0 == "\\" } && extensions.contains((name as NSString).pathExtension.lowercased())
    }
    public static func validSelection(_ files: [NativeUpload]) -> Bool {
        files.count <= count && files.allSatisfy(\.valid) && Set(files.map(\.id)).count == files.count && files.reduce(0, { $0 + $1.size }) <= total
    }
}

/// One shared actor per application. This namespace is never pruned by the JS
/// terminal upload service. No selected or potentially admitted copy is deleted.
public actor NativeUploadStore {
    public let root: URL
    public init(root: URL) { self.root = root.standardizedFileURL }
    private func failure(_ message: String) -> BridgeFailure { BridgeFailure(message) }
    private func openRoot() throws -> Int32 {
        // Canonicalize the parent for system aliases such as macOS /var. The
        // private root itself and every child are opened with O_NOFOLLOW.
        let parent = root.deletingLastPathComponent().resolvingSymlinksInPath()
        try FileManager.default.createDirectory(at: parent, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        let fd = open(parent.path, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard fd >= 0 else { throw failure("Private attachment storage is unavailable.") }; defer { close(fd) }
        guard mkdirat(fd, root.lastPathComponent, 0o700) == 0 || errno == EEXIST else { throw failure("Could not create private attachment storage.") }
        let owned = openat(fd, root.lastPathComponent, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard owned >= 0 else { throw failure("Attachment storage must be an ordinary private directory, not a symlink.") }
        var info = stat()
        guard fstat(owned, &info) == 0, info.st_uid == geteuid(), fchmod(owned, 0o700) == 0 else { close(owned); throw failure("Attachment storage ownership is unavailable.") }
        return owned
    }
    private func randomID() throws -> String {
        var bytes = [UInt8](repeating: 0, count: 16)
        guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else { throw failure("Could not create an attachment identity.") }
        return bytes.map { String(format: "%02x", $0) }.joined()
    }
    private func create(_ rootFD: Int32, name: String) throws -> (String, Int32, Int32) {
        guard NativeUploadLimits.validName(name) else { throw failure("Choose a supported document, source/config file, PNG, JPEG or WebP with a safe filename.") }
        let directory = try randomID()
        guard mkdirat(rootFD, directory, 0o700) == 0 else { throw failure("Could not reserve a unique attachment identity.") }
        let folder = openat(rootFD, directory, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard folder >= 0 else { _ = unlinkat(rootFD, directory, AT_REMOVEDIR); throw failure("Could not open private attachment storage.") }
        let file = openat(folder, name, O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW | O_CLOEXEC, 0o600)
        guard file >= 0 else { close(folder); _ = unlinkat(rootFD, directory, AT_REMOVEDIR); throw failure("Could not create a private attachment copy.") }
        return (directory, folder, file)
    }
    private func rollback(_ rootFD: Int32, _ file: NativeUpload) {
        let dir = String(file.id.prefix(32)), fd = openat(rootFD, dir, O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        if fd >= 0 { _ = unlinkat(fd, file.name, 0); close(fd); _ = unlinkat(rootFD, dir, AT_REMOVEDIR) }
    }
    public func importFiles(_ urls: [URL], existing: [NativeUpload]) throws -> [NativeUpload] {
        guard !urls.isEmpty, urls.count + existing.count <= NativeUploadLimits.count, NativeUploadLimits.validSelection(existing) else { throw failure("Choose up to 12 supported files per message.") }
        let rootFD = try openRoot(); defer { close(rootFD) }
        var imported: [NativeUpload] = []
        do {
            var total = existing.reduce(0) { $0 + $1.size }
            for url in urls {
                guard url.isFileURL else { throw failure("Drop or choose ordinary files, not web addresses or dragged text.") }
                let scoped = url.startAccessingSecurityScopedResource(); defer { if scoped { url.stopAccessingSecurityScopedResource() } }
                let name = url.lastPathComponent
                guard NativeUploadLimits.validName(name) else { throw failure("\(name): this file type or filename is not supported.") }
                let input = open(url.path, O_RDONLY | O_NOFOLLOW | O_NONBLOCK | O_CLOEXEC)
                guard input >= 0 else { throw failure("\(name): choose an accessible regular file, not a symlink.") }; defer { close(input) }
                var before = stat()
                guard fstat(input, &before) == 0, before.st_mode & S_IFMT == S_IFREG, before.st_size > 0, before.st_size <= NativeUploadLimits.limit(name) else { throw failure("\(name): file must be regular, nonempty and within its attachment size limit.") }
                total += Int(before.st_size)
                guard total <= NativeUploadLimits.total else { throw failure("Attachments must total no more than 16 MiB.") }
                let (directory, folder, output) = try create(rootFD, name: name)
                var completed = false
                defer { close(output); if !completed { _ = unlinkat(folder, name, 0); _ = unlinkat(rootFD, directory, AT_REMOVEDIR) }; close(folder) }
                var buffer = [UInt8](repeating: 0, count: 64 * 1024), hash = SHA256(), size = 0
                while true {
                    let count = Darwin.read(input, &buffer, buffer.count)
                    if count < 0 && errno == EINTR { continue }
                    guard count >= 0 else { throw failure("\(name): could not copy this file.") }
                    if count == 0 { break }
                    size += count
                    guard size <= before.st_size else { throw failure("\(name): changed during copying. Choose it again.") }
                    hash.update(data: Data(buffer.prefix(count)))
                    try buffer.withUnsafeBytes { bytes in
                        var offset = 0
                        while offset < count {
                            let n = Darwin.write(output, bytes.baseAddress!.advanced(by: offset), count - offset)
                            if n < 0 && errno == EINTR { continue }
                            guard n > 0 else { throw failure("Could not save the private attachment copy.") }; offset += n
                        }
                    }
                }
                var after = stat()
                guard fstat(input, &after) == 0, size == before.st_size, before.st_size == after.st_size, before.st_mtimespec.tv_sec == after.st_mtimespec.tv_sec, before.st_mtimespec.tv_nsec == after.st_mtimespec.tv_nsec, fsync(output) == 0 else { throw failure("\(name): changed or could not be saved. Choose it again.") }
                let file = NativeUpload(id: directory + "/" + name, name: name, size: size, sha256: hash.finalize().map { String(format: "%02x", $0) }.joined())
                if file.isImage { _ = try imageThumbnail(readFile(rootFD, file), normalize: false) }
                imported.append(file); completed = true
            }
            return imported
        } catch { for file in imported { rollback(rootFD, file) }; throw error }
    }
    private func readFile(_ rootFD: Int32, _ file: NativeUpload) throws -> Data {
        guard file.valid else { throw failure("Invalid attachment reference.") }
        let folder = openat(rootFD, String(file.id.prefix(32)), O_RDONLY | O_DIRECTORY | O_NOFOLLOW | O_CLOEXEC)
        guard folder >= 0 else { throw failure("\(file.name) is unavailable. Remove the selection and attach it again.") }; defer { close(folder) }
        let input = openat(folder, file.name, O_RDONLY | O_NOFOLLOW | O_NONBLOCK | O_CLOEXEC)
        guard input >= 0 else { throw failure("\(file.name) is unavailable. Remove the selection and attach it again.") }; defer { close(input) }
        var info = stat()
        guard fstat(input, &info) == 0, info.st_mode & S_IFMT == S_IFREG, info.st_size == file.size else { throw failure("\(file.name) changed or is unavailable. Attach it again.") }
        var data = Data(), buffer = [UInt8](repeating: 0, count: 64 * 1024)
        while data.count <= file.size {
            let n = Darwin.read(input, &buffer, min(buffer.count, file.size + 1 - data.count))
            if n < 0 && errno == EINTR { continue }
            guard n >= 0 else { throw failure("\(file.name) could not be read.") }
            if n == 0 { break }; data.append(contentsOf: buffer.prefix(n))
        }
        guard data.count == file.size, SHA256.hash(data: data).map({ String(format: "%02x", $0) }).joined() == file.sha256 else { throw failure("\(file.name) changed. The captured attachment cannot be sent with different bytes.") }
        return data
    }
    public func check(_ files: [NativeUpload]) throws {
        guard NativeUploadLimits.validSelection(files) else { throw failure("Invalid, duplicate or oversized attachment selection.") }
        if files.isEmpty { return }
        let fd = try openRoot(); defer { close(fd) }
        for file in files { _ = try readFile(fd, file) }
    }
    public func thumbnail(_ file: NativeUpload) throws -> Data {
        guard file.isImage else { throw failure("This attachment is not an image.") }
        let fd = try openRoot(); defer { close(fd) }
        return try imageThumbnail(readFile(fd, file), normalize: false)
    }
    /// Clipboard decoding is bounded before raster allocation. TIFF is accepted
    /// only at the native paste boundary and normalized to a supported PNG.
    private func imageThumbnail(_ data: Data, normalize: Bool) throws -> Data {
        guard !data.isEmpty, data.count <= (normalize ? NativeUploadLimits.total : NativeUploadLimits.image), let source = CGImageSourceCreateWithData(data as CFData, [kCGImageSourceShouldCache: false] as CFDictionary), CGImageSourceGetStatus(source) == .statusComplete, let type = CGImageSourceGetType(source) as String?, ["public.png", "public.jpeg", "org.webmproject.webp", "public.webp", "public.tiff"].contains(type), normalize || type != "public.tiff", let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any], let width = properties[kCGImagePropertyPixelWidth] as? Int, let height = properties[kCGImagePropertyPixelHeight] as? Int, width > 0, height > 0, width <= 16384, height <= 16384, width <= 16_777_216 / height else { throw failure("Choose a valid PNG, JPEG or WebP image within 16 megapixels and the staged-byte limit.") }
        let options: [CFString: Any] = [kCGImageSourceCreateThumbnailFromImageAlways: true, kCGImageSourceThumbnailMaxPixelSize: normalize ? max(width, height) : 160, kCGImageSourceShouldCacheImmediately: true, kCGImageSourceCreateThumbnailWithTransform: true]
        guard let image = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else { throw failure("This image could not be decoded.") }
        let result = NSMutableData()
        guard let destination = CGImageDestinationCreateWithData(result, UTType.png.identifier as CFString, 1, nil) else { throw failure("Could not prepare this image.") }
        CGImageDestinationAddImage(destination, image, nil)
        guard CGImageDestinationFinalize(destination) else { throw failure("Could not prepare this image.") }
        return result as Data
    }
    public func importPastedImage(_ bytes: Data, existing: [NativeUpload]) throws -> [NativeUpload] {
        let normalized = try imageThumbnail(bytes, normalize: true)
        guard normalized.count <= NativeUploadLimits.image, existing.count < NativeUploadLimits.count, NativeUploadLimits.validSelection(existing), existing.reduce(normalized.count, { $0 + $1.size }) <= NativeUploadLimits.total else { throw failure("Pasted image exceeds the 4 MiB image or 16 MiB message limit.") }
        let fd = try openRoot(); defer { close(fd) }
        // Go rejects duplicate input display names even with distinct upload IDs.
        // Repeated screenshot pastes therefore need distinct staged names too.
        let names = Set(existing.map(\.name))
        var name = "Pasted image.png", suffix = 2
        while names.contains(name) { name = "Pasted image \(suffix).png"; suffix += 1 }
        let (directory, folder, output) = try create(fd, name: name)
        defer { close(output); close(folder) }
        do {
            try normalized.withUnsafeBytes { bytes in
                var offset = 0
                while offset < bytes.count {
                    let n = Darwin.write(output, bytes.baseAddress!.advanced(by: offset), bytes.count - offset)
                    if n < 0 && errno == EINTR { continue }
                    guard n > 0 else { throw failure("Could not save the pasted image.") }; offset += n
                }
            }
            guard fsync(output) == 0 else { throw failure("Could not save the pasted image.") }
            return [NativeUpload(id: directory + "/" + name, name: name, size: normalized.count, sha256: SHA256.hash(data: normalized).map { String(format: "%02x", $0) }.joined())]
        } catch { _ = unlinkat(folder, name, 0); _ = unlinkat(fd, directory, AT_REMOVEDIR); throw error }
    }
}
