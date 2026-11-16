import SwiftUI
import AppKit
import GatorCore

/// NSTextView owns IME, selection, undo and standard editing shortcuts.
struct NativeComposer: NSViewRepresentable {
    var text: String
    var editable: Bool
    var focusRequest: UUID
    var onEdit: (String) -> Void
    var onDropFiles: ([URL]) -> Bool
    var onDragTarget: (Bool) -> Void
    var onPasteImage: () -> Bool
    var onSend: () -> Void

    func makeCoordinator() -> Coordinator { Coordinator(self) }
    func makeNSView(context: Context) -> NSScrollView {
        let scroll = NSScrollView(); scroll.drawsBackground = false
        scroll.hasVerticalScroller = true; scroll.autohidesScrollers = true
        let view = ComposerTextView(); view.delegate = context.coordinator
        view.isRichText = false; view.allowsUndo = true; view.drawsBackground = false
        view.font = .preferredFont(forTextStyle: .body); view.textColor = .labelColor
        view.isAutomaticQuoteSubstitutionEnabled = false; view.isAutomaticDashSubstitutionEnabled = false
        view.textContainerInset = NSSize(width: 2, height: 4)
        view.minSize = NSSize(width: 0, height: 0)
        view.maxSize = NSSize(width: CGFloat.greatestFiniteMagnitude, height: CGFloat.greatestFiniteMagnitude)
        view.isVerticallyResizable = true; view.isHorizontallyResizable = false
        view.autoresizingMask = [.width]; view.textContainer?.widthTracksTextView = true
        view.setAccessibilityLabel("Message Gator")
        view.setAccessibilityHelp("Return sends. Shift Return inserts a newline. Composing text is not submitted until composition ends.")
        view.registerForDraggedTypes([.fileURL, .init("NSFilenamesPboardType")])
        scroll.documentView = view
        return scroll
    }
    func updateNSView(_ scroll: NSScrollView, context: Context) {
        context.coordinator.parent = self
        guard let view = scroll.documentView as? ComposerTextView else { return }
        view.onDropFiles = onDropFiles; view.onDragTarget = onDragTarget
        view.onPasteImage = onPasteImage
        view.isEditable = editable
        if editable { view.requestComposerFocus(focusRequest) }
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
            if selector == #selector(NSResponder.insertNewline(_:)), NativeInteractionSemantics.sendOnReturn(hasMarkedText: textView.hasMarkedText(), shift: NSApp.currentEvent?.modifierFlags.contains(.shift) == true) {
                parent.onEdit(textView.string); parent.onSend(); return true
            }
            return false
        }
    }
}

/// Focus can be requested before SwiftUI attaches the new composer to a window.
/// Keep that request until attachment/key activation; streaming never creates one.
@MainActor
final class ComposerTextView: NSTextView {
    var onDropFiles: (([URL]) -> Bool)?
    var onDragTarget: ((Bool) -> Void)?
    override func draggingEntered(_ sender: any NSDraggingInfo) -> NSDragOperation {
        if NativeDropInput.hasFiles(sender.draggingPasteboard) { onDragTarget?(true); return .copy }
        return super.draggingEntered(sender)
    }
    override func draggingUpdated(_ sender: any NSDraggingInfo) -> NSDragOperation { draggingEntered(sender) }
    override func draggingExited(_ sender: (any NSDraggingInfo)?) { onDragTarget?(false); super.draggingExited(sender) }
    override func performDragOperation(_ sender: any NSDraggingInfo) -> Bool {
        onDragTarget?(false)
        if NativeDropInput.hasFiles(sender.draggingPasteboard) {
            guard let urls = NativeDropInput.urls(sender.draggingPasteboard), !urls.isEmpty else { return false }
            let accepted = onDropFiles?(urls) == true
            NativeInputTrace.record("drop received files=\(urls.count) accepted=\(accepted)")
            return accepted
        }
        return super.performDragOperation(sender)
    }
    var onPasteImage: (() -> Bool)?
    var readPlainText: () -> String? = { NSPasteboard.general.string(forType: .string) }
    override func cancelOperation(_ sender: Any?) {
        if !hasMarkedText(), window?.identifier?.rawValue == "gator.compact" { window?.close(); return }
        super.cancelOperation(sender)
    }
    private func isPasteShortcut(_ event: NSEvent) -> Bool {
        event.modifierFlags.intersection([.command, .control, .option, .shift]) == .command && event.charactersIgnoringModifiers?.lowercased() == "v"
    }
    override func performKeyEquivalent(with event: NSEvent) -> Bool {
        if event.keyCode == 9 { NativeInputTrace.record("paste equivalent matched=\(isPasteShortcut(event)) responder=\(window?.firstResponder === self)") }
        // Accessory/menu-bar applications may have no active Edit menu. Route
        // explicit Command-V through the text responder's native paste path.
        if window?.firstResponder === self, isPasteShortcut(event) { paste(nil); return true }
        return super.performKeyEquivalent(with: event)
    }
    override func keyDown(with event: NSEvent) {
        if event.keyCode == 9 { NativeInputTrace.record("paste key matched=\(isPasteShortcut(event))") }
        if event.keyCode == 48, !hasMarkedText(), event.modifierFlags.intersection([.command, .control, .option]).isEmpty {
            if event.modifierFlags.contains(.shift) { window?.selectPreviousKeyView(self) } else { window?.selectNextKeyView(self) }
            return
        }
        if isPasteShortcut(event) { paste(nil); return }
        super.keyDown(with: event)
    }
    override func paste(_ sender: Any?) {
        NativeInputTrace.record("paste received editable=\(isEditable) marked=\(hasMarkedText())")
        if !isEditable || hasMarkedText() { return }
        if onPasteImage?() == true { NativeInputTrace.record("image paste handled"); return }
        // Plain text wins when both text and pixels are present; HTML is never
        // rendered or interpreted, and file URLs never become image imports.
        if let text = readPlainText() { insertText(text, replacementRange: selectedRange()) }
    }
    private var requested: UUID?
    private var handled: UUID?
    private var observation: WindowFocusObservation?
    func requestComposerFocus(_ token: UUID) { requested = token; scheduleFocus() }
    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        observation = nil
        if let window {
            let token = NotificationCenter.default.addObserver(forName: NSWindow.didBecomeKeyNotification, object: window, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.applyFocus() }
            }
            observation = WindowFocusObservation(token)
        }
        scheduleFocus()
    }
    private func scheduleFocus() { DispatchQueue.main.async { [weak self] in self?.applyFocus() } }
    private func applyFocus() {
        guard let requested, requested != handled, let window, window.isKeyWindow, window.isVisible, !hasMarkedText() else { return }
        if window.makeFirstResponder(self) { handled = requested }
    }
}

/// NotificationCenter removal is thread safe; the callback itself runs on main.
private final class WindowFocusObservation: @unchecked Sendable {
    let token: NSObjectProtocol
    init(_ token: NSObjectProtocol) { self.token = token }
    deinit { NotificationCenter.default.removeObserver(token) }
}
