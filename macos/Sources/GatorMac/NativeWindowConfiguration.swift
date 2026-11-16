import SwiftUI
import AppKit

@MainActor enum GatorTheme {
    static let accent = Color(nsColor: NSColor(name: "GatorAccent") { appearance in
        appearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
            ? NSColor(red: 0.42, green: 0.78, blue: 0.61, alpha: 1)
            : NSColor(red: 0.16, green: 0.43, blue: 0.31, alpha: 1)
    })
}
/// Standard scenes retain ownership; this configures only their native window.
struct NativeWindowConfiguration: NSViewRepresentable {
    let compact: Bool
    func makeNSView(context: Context) -> WindowView { let view = WindowView(); view.compact = compact; return view }
    func updateNSView(_ view: WindowView, context: Context) { view.compact = compact; view.configure() }
    final class WindowView: NSView {
        var compact = false
        override func viewDidMoveToWindow() { super.viewDidMoveToWindow(); configure() }
        func configure() {
            guard let window else { return }
            // SwiftUI owns its scene/window identifier. Keep weak presentation-only
            // references instead of overwriting the scene's restoration identity.
            NativeApplicationPresentation.register(window, compact: compact)
            if !compact { window.hidesOnDeactivate = false }
            DispatchQueue.main.async { NativeApplicationPresentation.update() }
            #if DEBUG
            // Process-scoped fixture appearance only; never writes OS defaults.
            if let mode = ProcessInfo.processInfo.environment["GATOR_NATIVE_TEST_APPEARANCE"], ["light", "dark"].contains(mode) {
                let name: NSAppearance.Name = mode == "dark" ? .darkAqua : .aqua
                if window.appearance?.name != name { window.appearance = NSAppearance(named: name) }
                if NSApp.appearance?.name != name { NSApp.appearance = NSAppearance(named: name) }
            }
            #endif
        }
    }
}

@MainActor enum NativeApplicationPresentation {
    private(set) static weak var fullWindow: NSWindow?
    private(set) static weak var compactWindow: NSWindow?
    static func register(_ window: NSWindow, compact: Bool) {
        if compact { compactWindow = window } else { fullWindow = window }
    }
    static func isCompact(_ window: NSWindow?) -> Bool { window != nil && window === compactWindow }
    static func policy(hasFullWindow: Bool) -> NSApplication.ActivationPolicy { hasFullWindow ? .regular : .accessory }
    static func update() {
        let full = fullWindow?.isVisible == true
        let desired = policy(hasFullWindow: full)
        if NSApp.activationPolicy() != desired { NSApp.setActivationPolicy(desired) }
    }
}
