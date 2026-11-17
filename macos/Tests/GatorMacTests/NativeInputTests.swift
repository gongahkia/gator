import AppKit
import SwiftUI
import Testing
@testable import GatorMac

@MainActor func inputKey(_ code: UInt16, _ modifiers: NSEvent.ModifierFlags = [], _ characters: String = "") throws -> NSEvent {
    try #require(NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: modifiers, timestamp: 0, windowNumber: 0, context: nil, characters: characters, charactersIgnoringModifiers: characters, isARepeat: false, keyCode: code))
}
@Test @MainActor func commandVPasteRoutesExactlyOnceAndPlainTextUsesNativeInsertion() throws {
    let view = ComposerTextView(); view.isEditable = true
    var images = 0
    view.onPasteImage = { images += 1; return true }
    view.keyDown(with: try inputKey(9, .command, "v"))
    #expect(images == 1); #expect(view.string.isEmpty)
    view.onPasteImage = { false }; view.readPlainText = { "A normal paste\nwith two lines" }
    view.paste(nil); #expect(view.string == "A normal paste\nwith two lines")
    view.isEditable = false; view.paste(nil); #expect(images == 1)
}
@Test @MainActor func markedTextDoesNotBecomeAnAttachmentOrLoseComposition() {
    let view = ComposerTextView(); view.isEditable = true
    var pasted = false; view.onPasteImage = { pasted = true; return true }
    view.setMarkedText("にほん", selectedRange: NSRange(location: 3, length: 0), replacementRange: NSRange(location: NSNotFound, length: 0))
    #expect(view.hasMarkedText()); view.paste(nil)
    #expect(!pasted); #expect(view.hasMarkedText())
    view.insertText("日本", replacementRange: NSRange(location: NSNotFound, length: 0)); #expect(!view.hasMarkedText()); #expect(view.string == "日本")
}
@Test @MainActor func filePasteboardCarriesOnlyExplicitFileURLsAndRetainsOrdering() throws {
    let board = NSPasteboard(name: .init("dev.gator.native.tests." + UUID().uuidString))
    defer { board.releaseGlobally() }
    let first = URL(fileURLWithPath: "/tmp/first.md"), second = URL(fileURLWithPath: "/tmp/second.png")
    #expect(board.writeObjects([first as NSURL, second as NSURL]))
    #expect(NativeDropInput.urls(board) == [first, second])
    board.clearContents(); board.setString("/tmp/first.md", forType: .string)
    #expect(!NativeDropInput.hasFiles(board)); #expect(NativeDropInput.urls(board) == nil)
    board.clearContents(); board.writeObjects([URL(string: "https://example.invalid/file.md")! as NSURL])
    #expect(NativeDropInput.urls(board) == nil)
}
@Test @MainActor func tabLeavesComposerAndShiftTabReturnsWithoutInsertingText() throws {
    _ = NSApplication.shared
    final class FocusButton: NSButton { override var acceptsFirstResponder: Bool { true }; override var canBecomeKeyView: Bool { true } }
    let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 300, height: 100), styleMask: [.titled], backing: .buffered, defer: false)
    window.isReleasedWhenClosed = false
    let editor = ComposerTextView(frame: NSRect(x: 0, y: 30, width: 250, height: 60)), button = FocusButton(title: "Attach", target: nil, action: nil)
    button.frame = NSRect(x: 0, y: 0, width: 80, height: 24)
    window.contentView?.addSubview(editor); window.contentView?.addSubview(button)
    editor.nextKeyView = button; button.nextKeyView = editor
    #expect(window.makeFirstResponder(editor))
    editor.keyDown(with: try inputKey(48, [], "\t"))
    #expect(window.firstResponder === button); #expect(editor.string.isEmpty)
    #expect(window.makeFirstResponder(editor)); editor.keyDown(with: try inputKey(48, .shift, "\t"))
    #expect(window.firstResponder === button); #expect(editor.string.isEmpty)
    window.close()
}
@Test @MainActor func nativeAccentMeetsReadableTextContrastInBothAppearances() throws {
    func luminance(_ color: NSColor) -> Double {
        let c = color.usingColorSpace(.deviceRGB)!
        func linear(_ x: CGFloat) -> Double { let v = Double(x); return v <= 0.04045 ? v / 12.92 : pow((v + 0.055) / 1.055, 2.4) }
        return 0.2126 * linear(c.redComponent) + 0.7152 * linear(c.greenComponent) + 0.0722 * linear(c.blueComponent)
    }
    for name in [NSAppearance.Name.aqua, .darkAqua] {
        let appearance = try #require(NSAppearance(named: name))
        appearance.performAsCurrentDrawingAppearance {
            let foreground = luminance(NSColor(GatorTheme.accent)), background = luminance(.windowBackgroundColor)
            #expect((max(foreground, background) + 0.05) / (min(foreground, background) + 0.05) >= 4.5)
        }
    }
}
@Test @MainActor func fullWindowUsesStandardApplicationMenusWhileMenuBarOnlyStaysAccessory() {
    #expect(NativeApplicationPresentation.policy(hasFullWindow: true) == .regular)
    #expect(NativeApplicationPresentation.policy(hasFullWindow: false) == .accessory)
    #expect(NativeApplicationPresentation.policy(hasFullWindow: false, current: .regular) == .regular)
}
@Test @MainActor func oversizedPlainTextPasteLeavesCurrentNativeDraftIntact() {
    let view = ComposerTextView(); view.isEditable = true; view.string = "Keep this draft"
    view.onPasteImage = { false }; view.readPlainText = { String(repeating: "x", count: 128 * 1024 + 1) }
    var rejected = false; view.onRejectedText = { _ in rejected = true }
    view.paste(nil)
    #expect(rejected); #expect(view.string == "Keep this draft")
}

@Test @MainActor func ownerSwitchReplacesNativeCompositionContextAndKeepsCallbacksWithOriginalOwner() async throws {
    _ = NSApplication.shared
    var editsA: [String] = [], editsB: [String] = []
    func root(_ key: String, _ text: String, _ edit: @escaping (String) -> Void) -> some View {
        NativeComposer(text: text, editable: true, focusRequest: UUID(), onEdit: edit, onDropFiles: { _ in false }, onDragTarget: { _ in }, onPasteImage: { false }, onSend: {}).id(key).frame(width: 300, height: 80)
    }
    let host = NSHostingView(rootView: root("owner-A", "A draft", { editsA.append($0) }))
    host.frame = NSRect(x: 0, y: 0, width: 300, height: 80)
    func editor(_ view: NSView) -> ComposerTextView? { if let text = view as? ComposerTextView { return text }; return view.subviews.lazy.compactMap(editor).first }
    host.layoutSubtreeIfNeeded(); try await Task.sleep(for: .milliseconds(30))
    let original = try #require(editor(host))
    original.setMarkedText("にほん", selectedRange: NSRange(location: 3, length: 0), replacementRange: NSRange(location: NSNotFound, length: 0))
    host.rootView = root("owner-B", "B draft", { editsB.append($0) })
    host.layoutSubtreeIfNeeded(); try await Task.sleep(for: .milliseconds(30))
    let replacement = try #require(editor(host))
    #expect(replacement !== original); #expect(!replacement.hasMarkedText()); #expect(replacement.string == "B draft")
    original.insertText("日本", replacementRange: NSRange(location: NSNotFound, length: 0))
    #expect(editsB.isEmpty); #expect(replacement.string == "B draft")
}

@Test @MainActor func windowPresentationKeepsSwiftUISceneIdentityAndMapsBothSurfaces() {
    let full = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 540, height: 480), styleMask: [.titled], backing: .buffered, defer: true)
    full.isReleasedWhenClosed = false; full.identifier = .init("conversation-scene")
    NativeApplicationPresentation.register(full, compact: false)
    #expect(full.identifier?.rawValue == "conversation-scene")
    #expect(NativeApplicationPresentation.fullWindow === full)
    #expect(!NativeApplicationPresentation.isCompact(full))
    var compact: NSWindow? = NSWindow(contentRect: .zero, styleMask: [], backing: .buffered, defer: true)
    compact?.isReleasedWhenClosed = false
    NativeApplicationPresentation.register(compact!, compact: true)
    #expect(NativeApplicationPresentation.isCompact(compact))
    compact?.close(); compact = nil
    // AppKit may retain a closed window until its event loop drains; the
    // presentation registry deliberately does not own that lifetime.
    #expect(NativeApplicationPresentation.fullWindow === full)
    #expect(full.identifier?.rawValue == "conversation-scene")
    full.close()
}
