import AppKit
import SwiftUI

@MainActor
struct DockBadgeModifier: ViewModifier {
    let overdueCount: Int
    let enabled: Bool

    func body(content: Content) -> some View {
        content
            .onChange(of: overdueCount) { _, _ in apply() }
            .onChange(of: enabled) { _, _ in apply() }
            .onAppear { apply() }
    }

    private func apply() {
        guard enabled, overdueCount > 0 else {
            NSApp.dockTile.badgeLabel = nil
            return
        }
        NSApp.dockTile.badgeLabel = String(overdueCount)
    }
}

extension View {
    func dockBadge(overdueCount: Int, enabled: Bool) -> some View {
        modifier(DockBadgeModifier(overdueCount: overdueCount, enabled: enabled))
    }
}
