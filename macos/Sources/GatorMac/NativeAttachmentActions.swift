import AppKit
import SwiftUI
import UniformTypeIdentifiers
import GatorCore

@MainActor
enum NativeAttachmentActions {
    static func pick(owner: GatorApplicationState, sessions: ManagedSessionCoordinator) {
        guard let request = owner.beginAttachmentImport() else { return }
        let panel = NSOpenPanel()
        panel.title = "Attach Files"; panel.message = "Private copies stay with this conversation. " + NativeUploadLimits.hint
        panel.prompt = "Attach"; panel.canChooseFiles = true; panel.canChooseDirectories = false; panel.canCreateDirectories = false
        panel.allowsMultipleSelection = true; panel.resolvesAliases = false
        panel.allowedContentTypes = NativeUploadLimits.extensions.compactMap { UTType(filenameExtension: $0, conformingTo: .data) }
        let parent = NSApp.keyWindow, hides = parent?.hidesOnDeactivate
        parent?.hidesOnDeactivate = false
        let completed: (NSApplication.ModalResponse) -> Void = { response in
            Task { @MainActor in
                if response == .OK { await owner.importFiles(panel.urls, request: request) }
                else { owner.cancelAttachmentImport(request) }
                if let hides { parent?.hidesOnDeactivate = hides }
                if sessions.current === owner { parent?.makeKey(); owner.requestComposerFocus() }
            }
        }
        if let parent { panel.beginSheetModal(for: parent, completionHandler: completed) }
        else { panel.begin(completionHandler: completed) }
    }
    static func pasteImage(owner: GatorApplicationState, explicitly: Bool) -> Bool {
        let board = NSPasteboard.general
        let hasText = board.types?.contains(.string) == true
        let hasFile = NativeDropInput.hasFiles(board)
        NativeInputTrace.record("clipboard inspected text=\(hasText) file=\(hasFile)")
        let types: [NSPasteboard.PasteboardType] = [.png, .init("public.jpeg"), .init("org.webmproject.webp"), .init("public.webp"), .tiff]
        guard NativeClipboardPolicy.pasteImage(hasText: hasText, hasFileURL: hasFile, hasImage: types.contains(where: { board.types?.contains($0) == true }), explicitlyImage: explicitly) else { return false }
        guard let request = owner.beginAttachmentImport() else { return true }
        guard let type = types.first(where: { board.types?.contains($0) == true }), let bytes = board.data(forType: type), bytes.count <= NativeUploadLimits.total else { owner.failAttachmentImport(request, message: "Clipboard image is unavailable or too large. Paste a supported image within the message limits."); return true }
        Task { await owner.importImage(bytes, request: request) }
        return true
    }
    static func drop(_ providers: [NSItemProvider], owner: GatorApplicationState) -> Bool {
        guard let request = owner.beginAttachmentImport() else { return false }
        guard !providers.isEmpty, providers.count + owner.draftUploads.count <= NativeUploadLimits.count, providers.allSatisfy({ $0.hasItemConformingToTypeIdentifier(UTType.fileURL.identifier) }) else { owner.failAttachmentImport(request, message: "Drop up to 12 ordinary supported files. Text and web links are not file selections."); return false }
        Task {
            do {
                var urls: [URL] = []
                for provider in providers {
                    let data: Data = try await withCheckedThrowingContinuation { continuation in
                        provider.loadDataRepresentation(forTypeIdentifier: UTType.fileURL.identifier) { data, error in
                            if let data { continuation.resume(returning: data) }
                            else { continuation.resume(throwing: error ?? CocoaError(.fileReadUnknown)) }
                        }
                    }
                    guard data.count <= 16 * 1024, let url = URL(dataRepresentation: data, relativeTo: nil), url.isFileURL else { throw CocoaError(.fileReadUnsupportedScheme) }
                    urls.append(url)
                }
                await owner.importFiles(urls, request: request)
            } catch { owner.failAttachmentImport(request, message: "The drop could not be imported as ordinary files. Existing draft kept.") }
        }
        return true
    }
}
