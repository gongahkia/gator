import SwiftUI
import AppKit
import GatorCore

struct ComposerView: View {
    @Environment(GatorApplicationState.self) private var application
    var compact: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            ZStack(alignment: .topLeading) {
                if application.draftText.isEmpty {
                    Text("Message Gator…").font(.system(size: 14)).foregroundStyle(.tertiary).padding(.top, 4).padding(.leading, 2).allowsHitTesting(false)
                }
                NativeComposer(text: application.draftText, editable: application.canEditDraft, onEdit: application.editDraft) {
                    if application.canSend { Task { await application.submit() } }
                }
            }
            .frame(height: compact ? 68 : 88)
            HStack {
                Text("Text only · ⇧ Return for a new line")
                    .font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button {
                    Task { await application.submit() }
                } label: { Image(systemName: "arrow.up").font(.system(size: 15, weight: .semibold)).frame(width: 30, height: 28) }
                .buttonStyle(.borderedProminent).clipShape(RoundedRectangle(cornerRadius: 9))
                .disabled(!application.canSend).help(application.sendUnavailableReason ?? "Send message")
                .accessibilityLabel("Send message")
            }
        }
        .padding(12)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 18))
        .overlay(RoundedRectangle(cornerRadius: 18).stroke(.primary.opacity(0.10), lineWidth: 0.5))
    }
}

/// NSTextView owns IME, selection, undo and standard editing shortcuts.
private struct NativeComposer: NSViewRepresentable {
    var text: String
    var editable: Bool
    var onEdit: (String) -> Void
    var onSend: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }
    func makeNSView(context: Context) -> NSScrollView {
        let scroll = NSScrollView(); scroll.drawsBackground = false
        scroll.hasVerticalScroller = true; scroll.autohidesScrollers = true
        let view = NSTextView(); view.delegate = context.coordinator
        view.isRichText = false; view.allowsUndo = true; view.drawsBackground = false
        view.font = .systemFont(ofSize: 14); view.textColor = .labelColor
        view.isAutomaticQuoteSubstitutionEnabled = false; view.isAutomaticDashSubstitutionEnabled = false
        view.textContainerInset = NSSize(width: 2, height: 4)
        view.minSize = NSSize(width: 0, height: 0)
        view.maxSize = NSSize(width: CGFloat.greatestFiniteMagnitude, height: CGFloat.greatestFiniteMagnitude)
        view.isVerticallyResizable = true; view.isHorizontallyResizable = false
        view.autoresizingMask = [.width]; view.textContainer?.widthTracksTextView = true
        view.setAccessibilityLabel("Message Gator")
        scroll.documentView = view
        return scroll
    }
    func updateNSView(_ scroll: NSScrollView, context: Context) {
        context.coordinator.parent = self
        guard let view = scroll.documentView as? NSTextView else { return }
        view.isEditable = editable
        if !view.hasMarkedText(), view.string != text {
            let selection = view.selectedRange()
            view.string = text
            let length = (text as NSString).length
            view.setSelectedRange(NSRange(location: min(selection.location, length), length: 0))
        }
    }
    @MainActor
    final class Coordinator: NSObject, NSTextViewDelegate {
        var parent: NativeComposer
        init(_ parent: NativeComposer) { self.parent = parent }
        func textDidChange(_ notification: Notification) {
            guard let view = notification.object as? NSTextView else { return }
            parent.onEdit(view.string)
        }
        func textView(_ textView: NSTextView, doCommandBy selector: Selector) -> Bool {
            if selector == #selector(NSResponder.insertNewline(_:)), !textView.hasMarkedText(), NSApp.currentEvent?.modifierFlags.contains(.shift) != true {
                parent.onEdit(textView.string); parent.onSend(); return true
            }
            return false
        }
    }
}
