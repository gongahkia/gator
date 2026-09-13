import AppKit
import Foundation
import QuickLookThumbnailing
import SwiftUI
import UniformTypeIdentifiers

enum HCBTextMarkup {
    static func markdownSource(from raw: String) -> String {
        guard containsHTMLMarkup(raw) else { return raw }
        let converted = MarkdownHTML.calendarHTMLToMarkdown(raw)
        return stripRemainingHTML(from: converted)
    }

    private static func containsHTMLMarkup(_ value: String) -> Bool {
        value.range(
            of: #"(?i)(&lt;|<)/?[a-z][a-z0-9-]*(\s[^>]*)?(&gt;|>)"#,
            options: .regularExpression
        ) != nil
    }

    private static func stripRemainingHTML(from value: String) -> String {
        let withoutTags = replacingMatches(
            in: value,
            pattern: #"(?i)</?[a-z][a-z0-9-]*(\s[^>]*)?>"#,
            with: ""
        )
        return withoutTags
            .replacingOccurrences(of: "&nbsp;", with: " ")
            .replacingOccurrences(of: "&quot;", with: "\"")
            .replacingOccurrences(of: "&#39;", with: "'")
            .replacingOccurrences(of: "&apos;", with: "'")
            .replacingOccurrences(of: "&lt;", with: "<")
            .replacingOccurrences(of: "&gt;", with: ">")
            .replacingOccurrences(of: "&amp;", with: "&")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func replacingMatches(in value: String, pattern: String, with replacement: String) -> String {
        guard let regex = try? NSRegularExpression(pattern: pattern) else { return value }
        let range = NSRange(value.startIndex..<value.endIndex, in: value)
        return regex.stringByReplacingMatches(in: value, range: range, withTemplate: replacement)
    }
}

extension Text {
    static func markdown(_ string: String) -> Text {
        let string = HCBTextMarkup.markdownSource(from: string)
        let options = AttributedString.MarkdownParsingOptions(
            interpretedSyntax: .inlineOnlyPreservingWhitespace
        )
        if let attributed = try? AttributedString(markdown: string, options: options) {
            return Text(attributed)
        }
        return Text(string)
    }
}

// Block-aware markdown renderer. Handles bullet lists (- / *), numbered
// lists (N.), and paragraphs. Inline styling (bold/italic/links/code) still
// flows through Text.markdown per line. Blank lines preserve paragraph gaps.
// §7.01 Phase A2 — view mode only, edit mode stays plain-text source.
struct MarkdownBlock: View {
    let source: String
    var lineSpacing: CGFloat = 2

    var body: some View {
        VStack(alignment: .leading, spacing: lineSpacing) {
            ForEach(Array(parse().enumerated()), id: \.offset) { _, block in
                block
            }
        }
    }

    private func parse() -> [AnyView] {
        let lines = HCBTextMarkup.markdownSource(from: source).components(separatedBy: "\n")
        var out: [AnyView] = []
        for raw in lines {
            let line = raw
            if let attachment = LocalFileAttachment.parseMarkdownLine(line) {
                out.append(AnyView(LocalFileAttachmentView(attachment: attachment)))
            } else if let prefix = bulletPrefix(line) {
                let body = String(line.dropFirst(prefix.count)).trimmingCharacters(in: .whitespaces)
                out.append(AnyView(
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text("•").foregroundStyle(.secondary)
                        Text.markdown(body)
                    }
                ))
            } else if let (marker, rest) = numberedPrefix(line) {
                out.append(AnyView(
                    HStack(alignment: .firstTextBaseline, spacing: 6) {
                        Text(marker).foregroundStyle(.secondary).monospacedDigit()
                        Text.markdown(rest)
                    }
                ))
            } else if line.trimmingCharacters(in: .whitespaces).isEmpty {
                out.append(AnyView(Text(" ").hidden()))
            } else {
                out.append(AnyView(Text.markdown(line)))
            }
        }
        return out
    }

    private func bulletPrefix(_ line: String) -> String? {
        let trimmed = line.drop(while: { $0 == " " || $0 == "\t" })
        if trimmed.hasPrefix("- ") { return String(line.prefix(line.count - trimmed.count)) + "- " }
        if trimmed.hasPrefix("* ") { return String(line.prefix(line.count - trimmed.count)) + "* " }
        return nil
    }

    private func numberedPrefix(_ line: String) -> (String, String)? {
        let trimmed = line.drop(while: { $0 == " " || $0 == "\t" })
        var digits = ""
        var rest = Substring(trimmed)
        while let c = rest.first, c.isNumber {
            digits.append(c)
            rest = rest.dropFirst()
        }
        guard digits.isEmpty == false else { return nil }
        guard rest.hasPrefix(". ") else { return nil }
        return ("\(digits).", String(rest.dropFirst(2)))
    }
}

struct LocalFileAttachment: Equatable {
    enum Kind: String, Equatable {
        case image
        case file

        var labelPrefix: String {
            switch self {
            case .image: "Local image: "
            case .file: "Local file: "
            }
        }

        var systemImage: String {
            switch self {
            case .image: "photo"
            case .file: "paperclip"
            }
        }
    }

    let kind: Kind
    let displayName: String
    let url: URL

    static func markdownPointer(for url: URL, kind: Kind) -> String {
        let name = sanitizedLabel(url.lastPathComponent.isEmpty ? url.path : url.lastPathComponent)
        return markdownPointer(displayName: name, url: url, kind: kind)
    }

    static func markdownPointer(displayName: String, url: URL, kind: Kind) -> String {
        let name = sanitizedLabel(displayName.isEmpty ? (url.lastPathComponent.isEmpty ? url.path : url.lastPathComponent) : displayName)
        let destination = markdownDestination(for: url)
        return "[\(kind.labelPrefix)\(name)](\(destination))"
    }

    static func parseMarkdownLine(_ line: String) -> LocalFileAttachment? {
        let trimmed = line.trimmingCharacters(in: .whitespacesAndNewlines)
        guard let kind = Kind.allCases.first(where: { trimmed.hasPrefix("[\($0.labelPrefix)") }) else {
            return nil
        }
        let prefix = "[\(kind.labelPrefix)"
        guard trimmed.hasPrefix(prefix), trimmed.hasSuffix(")") else { return nil }
        guard let separator = trimmed.range(of: "](") else { return nil }

        let labelStart = trimmed.index(trimmed.startIndex, offsetBy: prefix.count)
        guard labelStart <= separator.lowerBound else { return nil }
        let rawName = String(trimmed[labelStart..<separator.lowerBound])
        let urlStart = separator.upperBound
        let urlEnd = trimmed.index(before: trimmed.endIndex)
        guard urlStart <= urlEnd else { return nil }
        let rawURL = String(trimmed[urlStart..<urlEnd])
        guard let url = URL(string: rawURL), url.isFileURL else { return nil }
        return LocalFileAttachment(kind: kind, displayName: rawName, url: url)
    }

    private static func markdownDestination(for url: URL) -> String {
        url.absoluteString
            .replacingOccurrences(of: "(", with: "%28")
            .replacingOccurrences(of: ")", with: "%29")
    }

    private static func sanitizedLabel(_ value: String) -> String {
        value
            .replacingOccurrences(of: "[", with: "(")
            .replacingOccurrences(of: "]", with: ")")
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    static func parseAll(in source: String) -> [LocalFileAttachment] {
        HCBTextMarkup.markdownSource(from: source)
            .components(separatedBy: "\n")
            .compactMap(parseMarkdownLine)
    }

    static func rewritePointers(in source: String, replacing replacements: [String: URL]) -> String {
        guard replacements.isEmpty == false else { return source }
        return source
            .components(separatedBy: "\n")
            .map { line in
                guard let attachment = parseMarkdownLine(line),
                      let replacement = replacements[attachment.url.absoluteString] else {
                    return line
                }
                return markdownPointer(
                    displayName: attachment.displayName,
                    url: replacement,
                    kind: attachment.kind
                )
            }
            .joined(separator: "\n")
    }

    var canExportOrDownload: Bool {
        health.isAvailable
    }

    var health: LocalAttachmentHealth {
        let didStartAccessing = url.startAccessingSecurityScopedResource()
        defer {
            if didStartAccessing {
                url.stopAccessingSecurityScopedResource()
            }
        }

        guard FileManager.default.fileExists(atPath: url.path),
              FileManager.default.isReadableFile(atPath: url.path) else {
            if FileManager.default.fileExists(atPath: url.path) {
                return .unreadable
            }
            return .missing
        }
        if kind == .image {
            guard NSImage(contentsOf: url) != nil else {
                return .corruptImage
            }
        }
        return .available
    }
}

extension LocalFileAttachment.Kind: CaseIterable {}

enum LocalAttachmentHealth: Equatable {
    case available
    case missing
    case unreadable
    case corruptImage

    var isAvailable: Bool {
        self == .available
    }

    var warning: String? {
        switch self {
        case .available:
            return nil
        case .missing:
            return "Local file is missing or moved."
        case .unreadable:
            return "Local file cannot be read."
        case .corruptImage:
            return "Local image cannot be opened. It may be corrupted or unsupported."
        }
    }

    var repairLabel: String {
        switch self {
        case .available:
            return "Available"
        case .missing:
            return "Missing"
        case .unreadable:
            return "Unreadable"
        case .corruptImage:
            return "Corrupted image"
        }
    }
}

private struct LocalFileAttachmentView: View {
    let attachment: LocalFileAttachment
    @State private var downloadMessage: String?

    var body: some View {
        let status = attachmentStatus()
        VStack(alignment: .leading, spacing: 6) {
            if case .image(let image) = status {
                Image(nsImage: image)
                    .resizable()
                    .scaledToFit()
                    .frame(maxHeight: 180)
                    .clipShape(RoundedRectangle(cornerRadius: 8))
            } else if case .available = status {
                LocalFileThumbnailView(url: attachment.url)
            } else if let warning = status.warning {
                Label(warning, systemImage: "exclamationmark.triangle")
                    .hcbFont(.caption)
                    .foregroundStyle(.red)
            }
            HStack(spacing: 8) {
                Link(destination: attachment.url) {
                    Label(attachment.displayName, systemImage: attachment.kind.systemImage)
                        .hcbFont(.caption)
                        .foregroundStyle(AppColor.blue)
                        .lineLimit(1)
                }
                Spacer(minLength: 0)
                Button {
                    downloadCopy()
                } label: {
                    Image(systemName: "arrow.down.circle")
                        .hcbFont(.caption)
                }
                .buttonStyle(.borderless)
                .disabled(status.canDownload == false)
                .help("Copy to Downloads")
            }
            if let downloadMessage {
                Text(downloadMessage)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .hcbScaledPadding(8)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(AppColor.cream.opacity(0.5))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.6)
        )
    }

    private enum AttachmentStatus {
        case available
        case image(NSImage)
        case missing
        case unreadable
        case corruptImage

        var warning: String? {
            switch self {
            case .available, .image:
                return nil
            case .missing:
                return "Local file is missing or moved."
            case .unreadable:
                return "Local file cannot be read."
            case .corruptImage:
                return "Local image cannot be opened. It may be corrupted or unsupported."
            }
        }

        var canDownload: Bool {
            switch self {
            case .available, .image:
                return true
            case .missing, .unreadable, .corruptImage:
                return false
            }
        }
    }

    private func attachmentStatus() -> AttachmentStatus {
        switch attachment.health {
        case .missing:
            return .missing
        case .unreadable:
            return .unreadable
        case .corruptImage:
            return .corruptImage
        case .available:
            if attachment.kind == .image, let image = NSImage(contentsOf: attachment.url) {
                return .image(image)
            }
            return .available
        }
    }

    private func downloadCopy() {
        do {
            let copied = try LocalAttachmentStore.copyToDownloads(attachment.url)
            downloadMessage = "Copied to Downloads as \(copied.lastPathComponent)."
        } catch {
            downloadMessage = error.localizedDescription
        }
    }
}

private struct LocalFileThumbnailView: View {
    let url: URL
    @State private var thumbnail: NSImage?

    var body: some View {
        Group {
            if let thumbnail {
                Image(nsImage: thumbnail)
                    .resizable()
                    .scaledToFit()
            } else {
                Image(nsImage: NSWorkspace.shared.icon(forFile: url.path))
                    .resizable()
                    .scaledToFit()
                    .symbolRenderingMode(.hierarchical)
            }
        }
        .frame(width: 96, height: 72)
        .hcbScaledPadding(8)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(AppColor.cream.opacity(0.55))
        )
        .task(id: url) {
            thumbnail = await quickLookThumbnail(for: url)
        }
    }

    private func quickLookThumbnail(for url: URL) async -> NSImage? {
        let didStartAccessing = url.startAccessingSecurityScopedResource()
        defer {
            if didStartAccessing {
                url.stopAccessingSecurityScopedResource()
            }
        }
        let request = QLThumbnailGenerator.Request(
            fileAt: url,
            size: CGSize(width: 192, height: 144),
            scale: NSScreen.main?.backingScaleFactor ?? 2,
            representationTypes: .thumbnail
        )
        return await withCheckedContinuation { continuation in
            QLThumbnailGenerator.shared.generateBestRepresentation(for: request) { thumbnail, _ in
                continuation.resume(returning: thumbnail?.nsImage)
            }
        }
    }
}

enum LocalAttachmentStore {
    static var attachmentsDirectoryURL: URL? {
        guard let appSupportURL = FileManager.default.urls(
            for: .applicationSupportDirectory,
            in: .userDomainMask
        ).first else {
            return nil
        }
        let appDirectoryName = Bundle.main.bundleIdentifier ?? "HotCrossBuns"
        return appSupportURL
            .appending(path: appDirectoryName, directoryHint: .isDirectory)
            .appending(path: "Attachments", directoryHint: .isDirectory)
    }

    static func pointerBlock(forFileURLs urls: [URL]) -> String {
        urls
            .filter(\.isFileURL)
            .map { url in
                let kind: LocalFileAttachment.Kind = isReadableImage(url) ? .image : .file
                return LocalFileAttachment.markdownPointer(for: url, kind: kind)
            }
            .joined(separator: "\n")
    }

    static func pointerBlockFromPasteboard(_ pasteboard: NSPasteboard = .general) throws -> String? {
        if let urls = pasteboard.readObjects(
            forClasses: [NSURL.self],
            options: [.urlReadingFileURLsOnly: true]
        ) as? [URL], urls.isEmpty == false {
            return pointerBlock(forFileURLs: urls)
        }

        if let image = NSImage(pasteboard: pasteboard) {
            let url = try saveImage(image, suggestedName: "Clipboard Image")
            return LocalFileAttachment.markdownPointer(for: url, kind: .image)
        }

        return nil
    }

    static func pointerBlock(fromProviders providers: [NSItemProvider]) async -> String {
        var pointers: [String] = []
        for provider in providers {
            if let url = await loadFileURL(from: provider) {
                pointers.append(LocalFileAttachment.markdownPointer(
                    for: url,
                    kind: isReadableImage(url) ? .image : .file
                ))
            } else if let imageURL = await loadImageToLocalAttachment(from: provider) {
                pointers.append(LocalFileAttachment.markdownPointer(for: imageURL, kind: .image))
            }
        }
        return pointers.joined(separator: "\n")
    }

    static func saveImage(_ image: NSImage, suggestedName: String) throws -> URL {
        guard let data = pngData(from: image) else {
            throw AttachmentError.couldNotEncodeImage
        }
        return try saveAttachmentData(data, preferredName: "\(suggestedName).png")
    }

    static func saveAttachmentData(_ data: Data, preferredName: String) throws -> URL {
        guard let directory = attachmentsDirectoryURL else {
            throw AttachmentError.attachmentsDirectoryUnavailable
        }
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let url = uniqueURL(in: directory, preferredName: preferredName)
        try data.write(to: url, options: [.atomic])
        return url
    }

    static func copyToDownloads(_ sourceURL: URL) throws -> URL {
        guard let downloads = FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask).first else {
            throw AttachmentError.downloadsDirectoryUnavailable
        }
        let didStartAccessing = sourceURL.startAccessingSecurityScopedResource()
        defer {
            if didStartAccessing {
                sourceURL.stopAccessingSecurityScopedResource()
            }
        }
        guard FileManager.default.fileExists(atPath: sourceURL.path),
              FileManager.default.isReadableFile(atPath: sourceURL.path) else {
            throw AttachmentError.sourceUnavailable
        }
        let destination = uniqueURL(in: downloads, preferredName: sourceURL.lastPathComponent)
        try FileManager.default.copyItem(at: sourceURL, to: destination)
        return destination
    }

    static func isReadableImage(_ url: URL) -> Bool {
        let didStartAccessing = url.startAccessingSecurityScopedResource()
        defer {
            if didStartAccessing {
                url.stopAccessingSecurityScopedResource()
            }
        }
        return FileManager.default.isReadableFile(atPath: url.path) && NSImage(contentsOf: url) != nil
    }

    static func uniqueURL(in directory: URL, preferredName: String) -> URL {
        let fallback = preferredName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            ? "Attachment"
            : preferredName
        let base = URL(fileURLWithPath: fallback).deletingPathExtension().lastPathComponent
        let ext = URL(fileURLWithPath: fallback).pathExtension
        var candidate = directory.appending(path: fallback)
        var index = 2
        while FileManager.default.fileExists(atPath: candidate.path) {
            let name = ext.isEmpty ? "\(base) \(index)" : "\(base) \(index).\(ext)"
            candidate = directory.appending(path: name)
            index += 1
        }
        return candidate
    }

    private static func loadFileURL(from provider: NSItemProvider) async -> URL? {
        guard provider.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) else { return nil }
        return await withCheckedContinuation { continuation in
            provider.loadItem(forTypeIdentifier: UTType.fileURL.identifier, options: nil) { item, _ in
                if let url = item as? URL {
                    continuation.resume(returning: url)
                } else if let data = item as? Data,
                          let string = String(data: data, encoding: .utf8),
                          let url = URL(string: string) {
                    continuation.resume(returning: url)
                } else if let string = item as? String,
                          let url = URL(string: string) {
                    continuation.resume(returning: url)
                } else {
                    continuation.resume(returning: nil)
                }
            }
        }
    }

    private static func loadImageToLocalAttachment(from provider: NSItemProvider) async -> URL? {
        guard provider.hasItemConformingToTypeIdentifier(UTType.image.identifier) else { return nil }
        let suggestedName = provider.suggestedName
        return await withCheckedContinuation { continuation in
            provider.loadDataRepresentation(forTypeIdentifier: UTType.image.identifier) { data, _ in
                guard let data, NSImage(data: data) != nil else {
                    continuation.resume(returning: nil)
                    return
                }
                let name = suggestedName.map { "\($0).png" } ?? "Dropped Image.png"
                let url = try? saveAttachmentData(data, preferredName: name)
                continuation.resume(returning: url)
            }
        }
    }

    private static func pngData(from image: NSImage) -> Data? {
        guard let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
            return nil
        }
        let bitmap = NSBitmapImageRep(cgImage: cgImage)
        return bitmap.representation(using: .png, properties: [:])
    }
}

enum AttachmentError: LocalizedError {
    case attachmentsDirectoryUnavailable
    case couldNotEncodeImage
    case downloadsDirectoryUnavailable
    case sourceUnavailable

    var errorDescription: String? {
        switch self {
        case .attachmentsDirectoryUnavailable:
            return "Could not create the local attachments folder."
        case .couldNotEncodeImage:
            return "Could not encode the pasted image."
        case .downloadsDirectoryUnavailable:
            return "Could not find the Downloads folder."
        case .sourceUnavailable:
            return "The local attachment is missing or unreadable."
        }
    }
}
