import AppKit
import SwiftUI

/// AppKit's secure editor supplies masking and secure accessibility behavior.
/// The binding is transient view state, never an observable application model.
struct NativeProviderSecretEntry: NSViewRepresentable {
    @Binding var text: String
    var enabled = true
    func makeCoordinator() -> Coordinator { Coordinator(self) }
    func makeNSView(context: Context) -> NSSecureTextField {
        let field = NSSecureTextField()
        field.placeholderString = "Leave empty to retain the existing credential"
        field.setAccessibilityLabel("API key, secure input")
        field.identifier = NSUserInterfaceItemIdentifier("gator.providers.secret")
        field.delegate = context.coordinator
        return field
    }
    func updateNSView(_ field: NSSecureTextField, context: Context) {
        context.coordinator.parent = self
        if field.stringValue != text { field.stringValue = text }
        field.isEnabled = enabled
    }
    static func dismantleNSView(_ field: NSSecureTextField, coordinator: Coordinator) {
        field.delegate = nil
        field.stringValue = ""
    }
    final class Coordinator: NSObject, NSTextFieldDelegate {
        var parent: NativeProviderSecretEntry
        init(_ parent: NativeProviderSecretEntry) { self.parent = parent }
        func controlTextDidChange(_ notification: Notification) {
            guard let field = notification.object as? NSSecureTextField else { return }
            parent.text = field.stringValue
        }
    }
}
