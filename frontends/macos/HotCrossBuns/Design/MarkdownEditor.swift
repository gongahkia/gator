import AppKit
import SwiftUI
import UniformTypeIdentifiers

// Edit surface for task notes and event descriptions. The underlying text
// view is MarkdownLiveEditor, which renders markdown formatting live while
// keeping syntax visible but dimmed (Obsidian-style live preview). The
// toolbar applies markdown commands to the current text selection.
struct MarkdownEditor: View {
    @Environment(AppModel.self) private var model
    @Environment(\.colorScheme) private var colorScheme
    @Binding var text: String
    var placeholder: String = ""
    var minHeight: CGFloat = 120
    var maxHeight: CGFloat = 240
    // Kept for backwards compatibility with existing call sites; the live
    // editor makes the below-editor preview redundant.
    var showInlinePreview: Bool = true

    @State private var isFocused: Bool = false
    @State private var isImportingLocalAttachment = false
    @State private var attachmentImportMessage: String?
    @State private var isDropTargeted = false
    @State private var selectedRange = NSRange(location: 0, length: 0)

    private var editorFontSize: CGFloat {
        let base = CGFloat(HCBTextSize.clamp(model.settings.uiTextSizePoints))
        let override = model.settings.perSurfaceFontOverrides[HCBSurface.editor.rawValue] ?? .empty
        if let pt = override.pointSize {
            return CGFloat(HCBTextSize.clamp(pt))
        }
        return base
    }

    private var editorFontName: String? {
        let override = model.settings.perSurfaceFontOverrides[HCBSurface.editor.rawValue] ?? .empty
        if let name = override.fontName, name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            return name
        }
        if let global = model.settings.uiFontName, global.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            return global
        }
        return nil
    }

    private var baseNSFont: NSFont {
        if let name = editorFontName, let f = NSFont(name: name, size: editorFontSize) {
            return f
        }
        return NSFont.systemFont(ofSize: editorFontSize)
    }

    private var theme: MarkdownHighlightTheme {
        .current(baseFont: baseNSFont)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            toolbar
            if let attachmentImportMessage {
                Label(attachmentImportMessage, systemImage: "exclamationmark.triangle")
                    .hcbFont(.caption2)
                    .foregroundStyle(.red)
            }
            editor
        }
        .fileImporter(
            isPresented: $isImportingLocalAttachment,
            allowedContentTypes: [.data, .image],
            allowsMultipleSelection: true,
            onCompletion: handleLocalAttachmentImport
        )
        .onDrop(
            of: [UTType.fileURL.identifier, UTType.image.identifier],
            isTargeted: $isDropTargeted,
            perform: handleAttachmentDrop
        )
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(AppColor.blue.opacity(isDropTargeted ? 0.75 : 0), lineWidth: 1.2)
        )
        // colorScheme is read so the view re-renders when the user flips
        // light/dark or swaps palette — NSTextView colors are pulled from
        // the scheme store on each update.
        .id(colorScheme)
    }

    private var trimmed: String {
        text.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var toolbar: some View {
        HStack(spacing: 4) {
            toolbarButton(title: "B", systemImage: "bold", help: "Bold (**text**)") {
                wrapAtEnd(prefix: "**", suffix: "**", placeholder: "bold")
            }
            .hcbFont(.caption, weight: .bold)
            toolbarButton(title: "I", systemImage: "italic", help: "Italic (*text*)") {
                wrapAtEnd(prefix: "*", suffix: "*", placeholder: "italic")
            }
            .font(.caption.italic())
            toolbarButton(title: "U", systemImage: "underline", help: "Underline (__text__, Calendar only)") {
                wrapAtEnd(prefix: "__", suffix: "__", placeholder: "underline")
            }
            .hcbFont(.caption, weight: .semibold)
            toolbarButton(title: "•", systemImage: "list.bullet", help: "Bulleted list") {
                insertLinePrefix("- ")
            }
            toolbarButton(title: "1.", systemImage: "list.number", help: "Numbered list") {
                insertLinePrefix("1. ")
            }
            toolbarButton(title: "🔗", systemImage: "link", help: "Link selected text") {
                applyTextMutation(MarkdownEditorTextMutation.link(text: text, selection: selectedRange))
            }
            toolbarButton(title: "File", systemImage: "paperclip", help: "Attach local file or image pointer") {
                attachmentImportMessage = nil
                isImportingLocalAttachment = true
            }
            Spacer(minLength: 8)
            if trimmed.isEmpty == false {
                Text("markdown")
                    .font(.caption2.weight(.medium).monospaced())
                    .foregroundStyle(.secondary)
            }
        }
        .hcbScaledPadding(.horizontal, 2)
    }

    private func toolbarButton(title: String, systemImage: String, help: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: systemImage)
                .hcbFont(.caption)
                .hcbScaledFrame(width: 22, height: 20)
        }
        .buttonStyle(.borderless)
        .help(help)
    }

    private var editor: some View {
        MarkdownLiveEditor(
            text: $text,
            placeholder: placeholder,
            minHeight: minHeight,
            maxHeight: maxHeight,
            baseFont: baseNSFont,
            theme: theme,
            selectedRange: $selectedRange,
            onFocusChange: { focused in isFocused = focused },
            onPasteAttachments: handleAttachmentPaste
        )
        .frame(minHeight: minHeight, maxHeight: maxHeight)
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 6)
                .fill(AppColor.cream.opacity(0.6))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 6)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
    }

    private func wrapAtEnd(prefix: String, suffix: String, placeholder: String) {
        applyTextMutation(
            MarkdownEditorTextMutation.wrap(
                text: text,
                selection: selectedRange,
                prefix: prefix,
                suffix: suffix,
                placeholder: placeholder
            )
        )
    }

    private func insertLinePrefix(_ prefix: String) {
        if text.isEmpty {
            text = prefix
        } else if text.hasSuffix("\n") {
            text.append(prefix)
        } else {
            text.append("\n\(prefix)")
        }
    }

    private func handleLocalAttachmentImport(_ result: Result<[URL], Error>) {
        switch result {
        case .success(let urls):
            attachmentImportMessage = nil
            var skipped = 0
            let pointers = urls.compactMap { url -> String? in
                guard url.isFileURL else {
                    skipped += 1
                    return nil
                }
                if LocalAttachmentStore.isReadableImage(url) {
                    return LocalFileAttachment.markdownPointer(for: url, kind: .image)
                }
                return LocalFileAttachment.markdownPointer(for: url, kind: .file)
            }
            guard pointers.isEmpty == false else {
                attachmentImportMessage = "Choose a readable local file or image."
                return
            }
            appendBlock(pointers.joined(separator: "\n"))
            if skipped > 0 {
                attachmentImportMessage = "Skipped \(skipped) attachment\(skipped == 1 ? "" : "s") that could not be opened."
            }
        case .failure(let error):
            attachmentImportMessage = error.localizedDescription
        }
    }

    private func handleAttachmentDrop(_ providers: [NSItemProvider]) -> Bool {
        Task {
            let block = await LocalAttachmentStore.pointerBlock(fromProviders: providers)
            await MainActor.run {
                guard block.isEmpty == false else {
                    attachmentImportMessage = "Drop a local file or image."
                    return
                }
                attachmentImportMessage = nil
                appendBlock(block)
            }
        }
        return true
    }

    private func handleAttachmentPaste() -> Bool {
        do {
            guard let block = try LocalAttachmentStore.pointerBlockFromPasteboard(), block.isEmpty == false else {
                return false
            }
            attachmentImportMessage = nil
            appendBlock(block)
            return true
        } catch {
            attachmentImportMessage = error.localizedDescription
            return true
        }
    }

    private func appendBlock(_ block: String) {
        if text.isEmpty {
            text = block
        } else if text.hasSuffix("\n\n") {
            text.append(block)
        } else if text.hasSuffix("\n") {
            text.append("\n\(block)")
        } else {
            text.append("\n\n\(block)")
        }
    }

    private func applyTextMutation(_ mutation: MarkdownEditorTextMutation.Result) {
        text = mutation.text
        selectedRange = mutation.selection
    }
}

struct MarkdownEditorTextMutation {
    struct Result {
        var text: String
        var selection: NSRange
    }

    static func link(text: String, selection: NSRange) -> Result {
        let nsText = text as NSString
        let range = clamped(selection, textLength: nsText.length)
        let label = range.length > 0 ? nsText.substring(with: range) : "text"
        let replacement = "[\(label)](https://)"
        let output = nsText.replacingCharacters(in: range, with: replacement)
        let urlLocation = range.location + 1 + (label as NSString).length + 2
        return Result(
            text: output,
            selection: NSRange(location: urlLocation, length: ("https://" as NSString).length)
        )
    }

    static func wrap(text: String, selection: NSRange, prefix: String, suffix: String, placeholder: String) -> Result {
        let nsText = text as NSString
        let range = clamped(selection, textLength: nsText.length)
        let selected = range.length > 0 ? nsText.substring(with: range) : placeholder
        let replacement = "\(prefix)\(selected)\(suffix)"
        let output = nsText.replacingCharacters(in: range, with: replacement)
        return Result(
            text: output,
            selection: NSRange(location: range.location + (prefix as NSString).length, length: (selected as NSString).length)
        )
    }

    static func clamped(_ range: NSRange, textLength: Int) -> NSRange {
        guard range.location != NSNotFound, range.location >= 0 else {
            return NSRange(location: textLength, length: 0)
        }
        let location = min(range.location, textLength)
        let length = max(0, min(range.length, textLength - location))
        return NSRange(location: location, length: length)
    }
}
