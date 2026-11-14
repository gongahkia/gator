import AppKit
import SwiftUI
import GatorCore

/// Approve/decline require a click, explicit accessibility press, or Space.
/// Return from a review or an IME composition can never activate a decision.
struct ExactDecisionButton: View {
    let title: String
    let enabled: Bool
    let action: () -> Void
    var body: some View {
        if enabled {
            NativeDecisionControl(title: title, enabled: true, action: action)
                .focusable()
                .accessibilityElement(children: .ignore)
                .accessibilityLabel(title)
                .accessibilityAddTraits(.isButton)
                .accessibilityHint("Space or an explicit press chooses this exact decision. Return never authorizes.")
                .accessibilityAction { action() }
                .onKeyPress(.return) { .handled }
                .onKeyPress(.space, phases: .down) { press in
                    guard press.modifiers.isEmpty else { return .ignored }
                    action(); return .handled
                }
        } else {
            // A standard disabled native control exposes AXEnabled correctly.
            // A trait-only custom AX proxy can otherwise appear enabled even
            // when its callback is guarded. There is no decision action here.
            Button(title, action: {}).disabled(true)
                .accessibilityLabel(title)
                .accessibilityHint("Unavailable. Inspect the current owner and exact review before deciding.")
        }
    }
}

private struct NativeDecisionControl: NSViewRepresentable {
    let title: String
    let enabled: Bool
    let action: () -> Void
    func makeCoordinator() -> Coordinator { Coordinator(action) }
    func makeNSView(context: Context) -> DecisionButton {
        let button = DecisionButton(title: title, target: context.coordinator, action: #selector(Coordinator.invokeDecision))
        button.bezelStyle = .rounded; button.keyEquivalent = ""; button.setAccessibilityLabel(title)
        button.focusRingType = .exterior
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
        override var acceptsFirstResponder: Bool { isEnabled }
        override func becomeFirstResponder() -> Bool {
            let accepted = super.becomeFirstResponder()
            if accepted { scrollToVisible(bounds) }
            return accepted
        }
        override func keyDown(with event: NSEvent) {
            if event.keyCode == 48 {
                if event.modifierFlags.contains(.shift) { window?.selectPreviousKeyView(self) }
                else { window?.selectNextKeyView(self) }
                return
            }
            guard NativeInteractionSemantics.decisionKeyAllowed(event.keyCode) else { return }
            if event.keyCode == 49 {
                let modified = !event.modifierFlags.intersection([.command, .control, .option, .shift]).isEmpty
                if isEnabled && NativeInteractionSemantics.activateDecisionOnSpace(code: event.keyCode, repeating: event.isARepeat, modified: modified) { performClick(nil) }
                return
            }
            super.keyDown(with: event)
        }
        override func keyUp(with event: NSEvent) {
            if event.keyCode == 49 || !NativeInteractionSemantics.decisionKeyAllowed(event.keyCode) { return }
            super.keyUp(with: event)
        }
        override func performKeyEquivalent(with event: NSEvent) -> Bool {
            guard NativeInteractionSemantics.decisionKeyAllowed(event.keyCode) else { return false }
            return super.performKeyEquivalent(with: event)
        }
    }
}
