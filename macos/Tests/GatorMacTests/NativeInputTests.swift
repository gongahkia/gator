import AppKit
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
}
