import AppKit
import SwiftUI

struct HCBWindowSceneID: RawRepresentable, Hashable, Codable, Sendable, Identifiable {
    var rawValue: String
    var id: String { rawValue }

    init(rawValue: String) {
        self.rawValue = rawValue
    }

    static let main = HCBWindowSceneID(rawValue: "main")
    static let help = HCBWindowSceneID(rawValue: "help")
    static let history = HCBWindowSceneID(rawValue: "history")
    static let syncIssues = HCBWindowSceneID(rawValue: "sync-issues")
    static let diagnostics = HCBWindowSceneID(rawValue: "diagnostics")

    static let restorableAuxiliaryIDs: [HCBWindowSceneID] = [
        .help,
        .history,
        .syncIssues,
        .diagnostics
    ]
}

struct WindowFrameSnapshot: Codable, Equatable, Sendable {
    var x: Double
    var y: Double
    var width: Double
    var height: Double

    init(x: Double, y: Double, width: Double, height: Double) {
        self.x = x
        self.y = y
        self.width = max(240, width)
        self.height = max(180, height)
    }

    init(_ frame: CGRect) {
        self.init(
            x: frame.origin.x,
            y: frame.origin.y,
            width: frame.width,
            height: frame.height
        )
    }

    var cgRect: CGRect {
        CGRect(x: x, y: y, width: width, height: height)
    }
}

@MainActor
final class HCBMainWindowPresenter {
    static let shared = HCBMainWindowPresenter()

    private var openWindow: OpenWindowAction?

    private init() {}

    func configure(openWindow: OpenWindowAction) {
        self.openWindow = openWindow
    }

    func show(openWindow: OpenWindowAction? = nil) {
        if let openWindow {
            configure(openWindow: openWindow)
        }

        NSApp.activate(ignoringOtherApps: true)
        if Self.focusExistingMainWindow() {
            return
        }

        self.openWindow?(id: HCBWindowSceneID.main.rawValue)
        Task { @MainActor in
            await Task.yield()
            NSApp.activate(ignoringOtherApps: true)
            _ = Self.focusExistingMainWindow()
        }
    }

    @discardableResult
    static func focusExistingMainWindow() -> Bool {
        guard let window = firstMainWindow(in: NSApp.windows) else { return false }
        window.makeKeyAndOrderFront(nil)
        return true
    }

    static func firstMainWindow(in windows: [NSWindow]) -> NSWindow? {
        windows.first { window in
            isMainWindow(identifierRawValue: window.identifier?.rawValue, title: window.title)
        }
    }

    nonisolated static func isMainWindow(identifierRawValue: String?, title: String) -> Bool {
        identifierRawValue == HCBWindowSceneID.main.rawValue || title == "Hot Cross Buns"
    }
}

final class WindowRestorationStore {
    static let shared = WindowRestorationStore()

    private let defaults: UserDefaults
    private let frameKeyPrefix = "hcb.window.frame."
    private let openWindowsKey = "hcb.window.openRestorableIDs"
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    func frame(for windowID: HCBWindowSceneID) -> CGRect? {
        guard
            let data = defaults.data(forKey: frameKey(for: windowID)),
            let snapshot = try? decoder.decode(WindowFrameSnapshot.self, from: data)
        else {
            return nil
        }
        return snapshot.cgRect
    }

    func saveFrame(_ frame: CGRect, for windowID: HCBWindowSceneID) {
        guard frame.width >= 240, frame.height >= 180 else { return }
        let snapshot = WindowFrameSnapshot(frame)
        guard let data = try? encoder.encode(snapshot) else { return }
        defaults.set(data, forKey: frameKey(for: windowID))
    }

    func markOpen(_ windowID: HCBWindowSceneID) {
        guard HCBWindowSceneID.restorableAuxiliaryIDs.contains(windowID) else { return }
        var ids = openWindowIDs()
        ids.insert(windowID)
        saveOpenWindowIDs(ids)
    }

    func markClosed(_ windowID: HCBWindowSceneID) {
        guard HCBWindowSceneID.restorableAuxiliaryIDs.contains(windowID) else { return }
        var ids = openWindowIDs()
        ids.remove(windowID)
        saveOpenWindowIDs(ids)
    }

    func openWindowIDs() -> Set<HCBWindowSceneID> {
        guard
            let data = defaults.data(forKey: openWindowsKey),
            let rawIDs = try? decoder.decode(Set<String>.self, from: data)
        else {
            return []
        }
        let allowed = Set(HCBWindowSceneID.restorableAuxiliaryIDs)
        return Set(rawIDs.map(HCBWindowSceneID.init(rawValue:)).filter { allowed.contains($0) })
    }

    func clearOpenWindows() {
        defaults.removeObject(forKey: openWindowsKey)
    }

    private func saveOpenWindowIDs(_ ids: Set<HCBWindowSceneID>) {
        let rawIDs = Set(ids.map(\.rawValue))
        guard let data = try? encoder.encode(rawIDs) else { return }
        defaults.set(data, forKey: openWindowsKey)
    }

    private func frameKey(for windowID: HCBWindowSceneID) -> String {
        frameKeyPrefix + windowID.rawValue
    }
}

struct WindowSessionRestorer: View {
    @Environment(\.openWindow) private var openWindow
    let settings: AppSettings
    var store: WindowRestorationStore = .shared
    @State private var didRestore = false

    var body: some View {
        Color.clear
            .frame(width: 0, height: 0)
            .onAppear(perform: restoreOnce)
            .onChange(of: settings.restoreWindowStateEnabled) { _, enabled in
                if enabled == false {
                    store.clearOpenWindows()
                }
            }
    }

    private func restoreOnce() {
        guard didRestore == false else { return }
        didRestore = true
        guard settings.restoreWindowStateEnabled else { return }

        for id in HCBWindowSceneID.restorableAuxiliaryIDs where store.openWindowIDs().contains(id) {
            openWindow(id: id.rawValue)
        }
    }
}

struct WindowRestorationModifier: ViewModifier {
    let windowID: HCBWindowSceneID
    let settings: AppSettings
    var store: WindowRestorationStore = .shared

    func body(content: Content) -> some View {
        content.background {
            WindowRestorationAccessor(
                windowID: windowID,
                isEnabled: settings.restoreWindowStateEnabled,
                store: store
            )
            .frame(width: 0, height: 0)
        }
    }
}

extension View {
    func hcbWindowRestoration(
        _ windowID: HCBWindowSceneID,
        settings: AppSettings,
        store: WindowRestorationStore = .shared
    ) -> some View {
        modifier(WindowRestorationModifier(windowID: windowID, settings: settings, store: store))
    }
}

private struct WindowRestorationAccessor: NSViewRepresentable {
    let windowID: HCBWindowSceneID
    let isEnabled: Bool
    let store: WindowRestorationStore

    func makeNSView(context: Context) -> NSView {
        let view = NSView(frame: .zero)
        context.coordinator.update(windowID: windowID, isEnabled: isEnabled, store: store)
        return view
    }

    func updateNSView(_ nsView: NSView, context: Context) {
        context.coordinator.update(windowID: windowID, isEnabled: isEnabled, store: store)
        DispatchQueue.main.async {
            context.coordinator.attach(to: nsView.window)
        }
    }

    func makeCoordinator() -> Coordinator {
        Coordinator(windowID: windowID, isEnabled: isEnabled, store: store)
    }

    final class Coordinator {
        private var windowID: HCBWindowSceneID
        private var isEnabled: Bool
        private var store: WindowRestorationStore
        private weak var window: NSWindow?
        private var observers: [NSObjectProtocol] = []
        private var didApplyFrame = false

        init(windowID: HCBWindowSceneID, isEnabled: Bool, store: WindowRestorationStore) {
            self.windowID = windowID
            self.isEnabled = isEnabled
            self.store = store
        }

        deinit {
            removeObservers()
        }

        func update(windowID: HCBWindowSceneID, isEnabled: Bool, store: WindowRestorationStore) {
            self.windowID = windowID
            self.isEnabled = isEnabled
            self.store = store
            if isEnabled == false {
                didApplyFrame = true
            }
        }

        func attach(to nextWindow: NSWindow?) {
            guard let nextWindow else { return }
            guard window !== nextWindow else { return }

            removeObservers()
            window = nextWindow
            didApplyFrame = false
            nextWindow.identifier = NSUserInterfaceItemIdentifier(windowID.rawValue)

            if isEnabled {
                restoreFrameIfAvailable(on: nextWindow)
                store.markOpen(windowID)
            }

            let center = NotificationCenter.default
            observers = [
                center.addObserver(
                    forName: NSWindow.didMoveNotification,
                    object: nextWindow,
                    queue: .main
                ) { [weak self] note in
                    self?.saveFrame(from: note.object as? NSWindow)
                },
                center.addObserver(
                    forName: NSWindow.didEndLiveResizeNotification,
                    object: nextWindow,
                    queue: .main
                ) { [weak self] note in
                    self?.saveFrame(from: note.object as? NSWindow)
                },
                center.addObserver(
                    forName: NSWindow.willCloseNotification,
                    object: nextWindow,
                    queue: .main
                ) { [weak self] note in
                    self?.handleClose(note.object as? NSWindow)
                }
            ]
        }

        private func restoreFrameIfAvailable(on window: NSWindow) {
            guard didApplyFrame == false else { return }
            didApplyFrame = true
            guard let frame = store.frame(for: windowID) else { return }
            window.setFrame(frame, display: true)
        }

        private func saveFrame(from window: NSWindow?) {
            guard isEnabled, let window else { return }
            store.saveFrame(window.frame, for: windowID)
        }

        private func handleClose(_ window: NSWindow?) {
            saveFrame(from: window)
            if isEnabled {
                store.markClosed(windowID)
            }
        }

        private func removeObservers() {
            for observer in observers {
                NotificationCenter.default.removeObserver(observer)
            }
            observers.removeAll()
        }
    }
}
