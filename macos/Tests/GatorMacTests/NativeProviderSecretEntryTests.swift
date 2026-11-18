import AppKit
import SwiftUI
import Testing
@testable import GatorMac
@testable import GatorCore

@Test @MainActor func providerSecretEditorUsesNativeSecureMaskAndClearsOnDismantle() {
    let entry = NativeProviderSecretEntry(text: .constant("P04_INERT_SECURE_FIELD"))
    let coordinator = entry.makeCoordinator(), field = NSSecureTextField()
    field.stringValue = "P04_INERT_SECURE_FIELD"; field.delegate = coordinator
    #expect(field.cell is NSSecureTextFieldCell)
    #expect((field.cell as? NSSecureTextFieldCell)?.echosBullets == true)
    NativeProviderSecretEntry.dismantleNSView(field, coordinator: coordinator)
    #expect(field.stringValue.isEmpty); #expect(field.delegate == nil)
}

// A hidden host exercises the native Escape equivalent after the secure editor
// is replaced by review, without launching or activating a desktop application.
@Test @MainActor func providerReviewEscapeCancelsAfterSecureEditorRemoval() async throws {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent("gator-native-test-provider-keyboard-" + UUID().uuidString)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
    defer { try? FileManager.default.removeItem(at: root) }
    let sessions = ManagedSessionCoordinator(configuration: BridgeLaunchConfiguration(runtime: root.appendingPathComponent("unused"), workspace: root), nativeDraftDirectory: root.appendingPathComponent("native"), profile: root.path)
    let setup = sessions.providers
    setup.present { request in
        let row: JSONValue = .object(["id": .string("openai"), "model": .string("fixture"), "kind": .string(""), "issue": .string(""), "destination": .string("127.0.0.1:9"), "api_version": .string("v1"), "stored": .bool(false), "ambient": .bool(false), "expired": .bool(false), "configured": .bool(false), "ready": .bool(false), "fields": .array([])])
        var value: [String: JSONValue] = ["request_id": .string(request.id), "action": .string(request.action.rawValue), "provider": .string(request.provider), "confirmed": .bool(false), "changed": .bool(false), "conflict": .bool(false)]
        if request.action == .status { value["rows"] = .array([row]) }
        if request.action == .review { value["review"] = .object(["id": .string("opaque"), "provider": .string("openai"), "operation": .string("save"), "model": .string("fixture"), "destination": .string("127.0.0.1:9"), "endpoint_mode": .string("retain"), "credential_action": .string("add"), "sets_defaults": .bool(true)]) }
        return try NativeProviderResult(.object(value))
    }
    let deadline = ContinuousClock.now + .seconds(3)
    while setup.selected == nil || setup.busy {
        guard ContinuousClock.now < deadline else { throw BridgeFailure("Provider fixture did not load") }
        await Task.yield()
    }
    let host = NSHostingView(rootView: ProviderSetupView(sessions: sessions))
    let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 820, height: 720), styleMask: [.titled], backing: .buffered, defer: false)
    window.isReleasedWhenClosed = false; window.contentView = host
    defer { window.contentView = nil; window.close(); setup.dismiss() }
    #expect(await setup.prepare(replacing: true))
    host.layoutSubtreeIfNeeded(); await Task.yield(); host.layoutSubtreeIfNeeded()
    let event = try #require(NSEvent.keyEvent(with: .keyDown, location: .zero, modifierFlags: [], timestamp: 0, windowNumber: window.windowNumber, context: nil, characters: "\u{1b}", charactersIgnoringModifiers: "\u{1b}", isARepeat: false, keyCode: 53))
    #expect(window.performKeyEquivalent(with: event))
    #expect(setup.review == nil)
    #expect(setup.isPresented)
}

@Test @MainActor func providerSecureEditorEscapeClearsTransientBindingBeforeCancellation() {
    final class Entry { var value = "P04_INERT_SECURE_CANCEL"; var cancelled = false }
    let state = Entry()
    let entry = NativeProviderSecretEntry(text: Binding(get: { state.value }, set: { state.value = $0 }), onCancel: {
        #expect(state.value.isEmpty)
        state.cancelled = true
    })
    let coordinator = entry.makeCoordinator(), field = NSSecureTextField()
    field.stringValue = state.value
    #expect(!coordinator.control(field, textView: NSTextView(), doCommandBy: #selector(NSResponder.insertNewline(_:))))
    #expect(!state.cancelled)
    #expect(coordinator.control(field, textView: NSTextView(), doCommandBy: #selector(NSResponder.cancelOperation(_:))))
    #expect(state.cancelled); #expect(state.value.isEmpty); #expect(field.stringValue.isEmpty)
}
