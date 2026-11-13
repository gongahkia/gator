import AppKit
import SwiftUI
import GatorCore

/// Approve/decline require a click, explicit accessibility press, or Space.
/// Return from a review or an IME composition can never activate a decision.
struct ExactDecisionButton: NSViewRepresentable {
    let title: String
    let enabled: Bool
    let action: () -> Void
    func makeCoordinator() -> Coordinator { Coordinator(action) }
    func makeNSView(context: Context) -> DecisionButton {
        let button = DecisionButton(title: title, target: context.coordinator, action: #selector(Coordinator.invokeDecision))
        button.bezelStyle = .rounded; button.keyEquivalent = ""; button.setAccessibilityLabel(title)
        button.setAccessibilityHelp("Click or press Space to choose this exact action. Return does not make a decision.")
        return button
    }
    func updateNSView(_ button: DecisionButton, context: Context) {
        button.title = title; button.isEnabled = enabled; context.coordinator.action = action
    }
    @MainActor final class Coordinator: NSObject {
        var action: () -> Void
        init(_ action: @escaping () -> Void) { self.action = action }
        @objc func invokeDecision() { action() }
    }
    final class DecisionButton: NSButton {
        override func keyDown(with event: NSEvent) {
            guard NativeInteractionSemantics.decisionKeyAllowed(event.keyCode) else { return }
            super.keyDown(with: event)
        }
        override func performKeyEquivalent(with event: NSEvent) -> Bool {
            guard NativeInteractionSemantics.decisionKeyAllowed(event.keyCode) else { return false }
            return super.performKeyEquivalent(with: event)
        }
    }
}
