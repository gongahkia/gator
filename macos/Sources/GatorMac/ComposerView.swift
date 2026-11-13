import SwiftUI
import AppKit
import GatorCore

struct ComposerView: View {
    @Environment(GatorApplicationState.self) private var application
    @Environment(ManagedSessionCoordinator.self) private var sessions
    var compact: Bool
    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let target = application.correctionTarget {
                HStack {
                    Text(target.label).font(.caption.weight(.medium))
                    Spacer()
                    Button("Cancel Correction") { application.cancelCorrection() }.disabled(application.pendingSubmission != nil)
                }
                if let issue = sessions.correctionUnavailableReason { Text(verbatim: issue).font(.caption).foregroundStyle(.orange) }
                Button("Inspect Correction Target") { sessions.inspectTask(target.ownerID) }.font(.caption)
            }
            ZStack(alignment: .topLeading) {
                if application.draftText.isEmpty {
                    Text("Message Gator…").font(.system(size: 14)).foregroundStyle(.tertiary).padding(.top, 4).padding(.leading, 2).allowsHitTesting(false)
                }
                NativeComposer(text: application.draftText, editable: application.canEditDraft, focusRequest: application.focusRequest, onEdit: application.editDraft) {
                    if sessions.canSubmit { let owner = application; Task { await sessions.submit(from: owner) } }
                }
            }
            .frame(height: compact ? 68 : 88)
            HStack {
                if application.correctionTarget == nil {
                    Toggle("Plan first", isOn: Binding(get: { application.planFirst }, set: { value in application.setPlanFirst(value) })).toggleStyle(.checkbox).font(.caption)
                        .help("Request a proposed plan for review before any implementation")
                }
                Text("⇧ Return: new line")
                    .font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button {
                    let owner = application; Task { await sessions.submit(from: owner) }
                } label: { Image(systemName: "arrow.up").font(.system(size: 15, weight: .semibold)).frame(width: 30, height: 28) }
                .buttonStyle(.borderedProminent).clipShape(RoundedRectangle(cornerRadius: 9))
                .disabled(!sessions.canSubmit).help(application.sendUnavailableReason ?? "Send message")
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
    var focusRequest: UUID
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
        view.setAccessibilityHelp("Return sends. Shift Return inserts a newline. Composing text is not submitted until composition ends.")
        scroll.documentView = view
        return scroll
    }
    func updateNSView(_ scroll: NSScrollView, context: Context) {
        context.coordinator.parent = self
        guard let view = scroll.documentView as? NSTextView else { return }
        view.isEditable = editable
        if editable, context.coordinator.focusRequest != focusRequest, let window = view.window, window.isKeyWindow, !view.hasMarkedText() {
            context.coordinator.focusRequest = focusRequest
            DispatchQueue.main.async { if window.isKeyWindow && !view.hasMarkedText() { window.makeFirstResponder(view) } }
        }
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
        var focusRequest: UUID?
        init(_ parent: NativeComposer) { self.parent = parent }
        func textDidChange(_ notification: Notification) {
            guard let view = notification.object as? NSTextView else { return }
            parent.onEdit(view.string)
        }
        func textView(_ textView: NSTextView, doCommandBy selector: Selector) -> Bool {
            if selector == #selector(NSResponder.insertNewline(_:)), NativeInteractionSemantics.sendOnReturn(hasMarkedText: textView.hasMarkedText(), shift: NSApp.currentEvent?.modifierFlags.contains(.shift) == true) {
                parent.onEdit(textView.string); parent.onSend(); return true
            }
            return false
        }
    }
}
