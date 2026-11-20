import SwiftUI
import AppKit

private final class MotionWindowObservers {
    var tokens: [NSObjectProtocol] = []
    deinit { for token in tokens { NotificationCenter.default.removeObserver(token) } }
}

struct NativeMotionVisibility: NSViewRepresentable {
    @Binding var visible: Bool
    func makeNSView(context: Context) -> Probe { Probe(update: { visible = $0 }) }
    func updateNSView(_ view: Probe, context: Context) { view.update = { visible = $0 }; view.refresh() }
    static func dismantleNSView(_ view: Probe, coordinator: ()) { view.detach() }
    @MainActor final class Probe: NSView {
        var update: (Bool) -> Void
        private var observers: MotionWindowObservers?
        private var last = false
        private var detached = false
        init(update: @escaping (Bool) -> Void) { self.update = update; super.init(frame: .zero); setAccessibilityElement(false) }
        required init?(coder: NSCoder) { nil }
        override var acceptsFirstResponder: Bool { false }
        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow(); removeObservers(); detached = false
            let bag = MotionWindowObservers()
            let windows: [Notification.Name] = [NSWindow.didChangeOcclusionStateNotification, NSWindow.didBecomeKeyNotification, NSWindow.didResignKeyNotification, NSWindow.didMiniaturizeNotification, NSWindow.didDeminiaturizeNotification, NSWindow.willCloseNotification]
            let applications: [Notification.Name] = [NSApplication.didBecomeActiveNotification, NSApplication.didResignActiveNotification]
            for name in windows + applications {
                let closing = name == NSWindow.willCloseNotification
                bag.tokens.append(NotificationCenter.default.addObserver(forName: name, object: windows.contains(name) ? window : NSApp, queue: .main) { [weak self] _ in
                    MainActor.assumeIsolated {
                        if closing { self?.publish(false) } else { self?.refresh() }
                    }
                })
            }
            observers = bag
            refresh()
        }
        func refresh() { publish(!detached && window?.isVisible == true && window?.isKeyWindow == true && window?.occlusionState.contains(.visible) == true && NSApp.isActive) }
        private func publish(_ value: Bool) {
            guard value != last else { return }; last = value
            DispatchQueue.main.async { [weak self] in guard let self, !self.detached || !value, self.last == value else { return }; self.update(value) }
        }
        func detach() { detached = true; publish(false); removeObservers() }
        private func removeObservers() { observers = nil }
    }
}
