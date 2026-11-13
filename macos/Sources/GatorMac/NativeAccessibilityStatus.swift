import AppKit
import SwiftUI

/// Announces only meaningful state changes in the active presentation. It
/// neither moves keyboard/VoiceOver focus nor announces streaming transcript deltas.
struct NativeAccessibilityStatus: NSViewRepresentable {
    let status: String
    func makeCoordinator() -> Coordinator { Coordinator() }
    func makeNSView(context: Context) -> NSView { NSView() }
    func updateNSView(_ view: NSView, context: Context) {
        guard !status.isEmpty, status != context.coordinator.last, view.window?.isKeyWindow == true else { return }
        context.coordinator.last = status
        NSAccessibility.post(element: NSApplication.shared, notification: .announcementRequested, userInfo: [.announcement: status, .priority: NSAccessibilityPriorityLevel.low.rawValue])
    }
    final class Coordinator { var last = "" }
}
