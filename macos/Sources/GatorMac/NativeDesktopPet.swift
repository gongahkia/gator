import AppKit
import Observation

enum NativeDesktopPetPreference {
    static let shown = "gator.desktopPet.shown"
    static let display = "gator.desktopPet.display"
}

struct NativeDesktopPetScreen: Equatable {
    let id: String
    let name: String
    let frame: CGRect
    let visible: CGRect
    var safeInsets = NSEdgeInsetsZero
    var notch: NotchGeometry?

    var petFrame: CGRect? {
        let safe = CGRect(x: frame.minX + safeInsets.left, y: frame.minY + safeInsets.bottom,
                          width: max(0, frame.width - safeInsets.left - safeInsets.right),
                          height: max(0, frame.height - safeInsets.top - safeInsets.bottom))
        let usable = visible.intersection(safe)
        let size: CGFloat = 72, margin: CGFloat = 12
        guard !usable.isNull, usable.width >= size + 2 * margin else { return nil }
        let top = min(usable.maxY, notch.map { $0.panelFrame(expanded: true).minY } ?? usable.maxY) - margin
        let result = CGRect(x: usable.midX - size / 2, y: top - size, width: size, height: size)
        return usable.insetBy(dx: margin, dy: margin).contains(result) ? result : nil
    }

    static func selected(_ screens: [Self], preferred: String) -> Self? {
        screens.first { !preferred.isEmpty && $0.id == preferred } ?? screens.first
    }

    @MainActor static func current() -> [Self] {
        NSScreen.screens.map { screen in
            let number = (screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber)?.uint32Value ?? 0
            let id = CGDisplayCreateUUIDFromDisplayID(number).map { CFUUIDCreateString(nil, $0.takeRetainedValue()) as String } ?? ""
            return Self(id: id, name: screen.localizedName, frame: screen.frame, visible: screen.visibleFrame,
                        safeInsets: screen.safeAreaInsets,
                        notch: NotchGeometry.detect(frame: screen.frame, safeTop: screen.safeAreaInsets.top,
                                                   left: screen.auxiliaryTopLeftArea, right: screen.auxiliaryTopRightArea))
        }
    }
}

// the controller receives presentation facts only; it has no execution methods.
@MainActor protocol NativeDesktopPetSurface: AnyObject {
    func show(_ request: NativeMascotRequest, frame: CGRect)
    func close()
}

@MainActor final class NativeDesktopPet {
    private let defaults: UserDefaults
    private let readSignal: () -> NativeMascotSignal?
    private let isClosing: () -> Bool
    private let screens: () -> [NativeDesktopPetScreen]
    private let makeSurface: () -> any NativeDesktopPetSurface
    private var surface: (any NativeDesktopPetSurface)?
    private var state = NativeMascotStateMachine()
    private var observers: [(NotificationCenter, NSObjectProtocol)] = []
    private var refresh: Task<Void, Never>?
    private var stopped = false
    private var sleeping = false

    init(defaults: UserDefaults = .standard, readSignal: @escaping () -> NativeMascotSignal?,
         isClosing: @escaping () -> Bool, screens: @escaping () -> [NativeDesktopPetScreen] = NativeDesktopPetScreen.current,
         makeSurface: @escaping () -> any NativeDesktopPetSurface = { NativeDesktopPetPanel() }) {
        self.defaults = defaults; self.readSignal = readSignal; self.isClosing = isClosing
        self.screens = screens; self.makeSurface = makeSurface
        observe(.default, UserDefaults.didChangeNotification)
        observe(.default, NSApplication.didChangeScreenParametersNotification)
        observe(NSWorkspace.shared.notificationCenter, NSWorkspace.accessibilityDisplayOptionsDidChangeNotification)
        observe(NSWorkspace.shared.notificationCenter, NSWorkspace.screensDidSleepNotification)
        observe(NSWorkspace.shared.notificationCenter, NSWorkspace.screensDidWakeNotification)
        reconcile()
    }

    private func observe(_ center: NotificationCenter, _ name: Notification.Name) {
        let token = center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated {
                if name == NSWorkspace.screensDidSleepNotification { self?.sleeping = true }
                if name == NSWorkspace.screensDidWakeNotification { self?.sleeping = false }
                self?.scheduleRefresh()
            }
        }
        observers.append((center, token))
    }

    private func scheduleRefresh() {
        guard !stopped else { return }
        refresh?.cancel()
        refresh = Task { [weak self] in
            guard !Task.isCancelled else { return }
            self?.reconcile()
        }
    }

    func reconcile() {
        guard !stopped else { return }
        withObservationTracking {
            guard defaults.bool(forKey: NativeDesktopPetPreference.shown), !sleeping, !isClosing(),
                  let frame = NativeDesktopPetScreen.selected(screens(), preferred: defaults.string(forKey: NativeDesktopPetPreference.display) ?? "")?.petFrame,
                  let signal = readSignal() else {
                releaseSurface(); state = NativeMascotStateMachine(); return
            }
            state.receive(signal)
            if surface == nil { surface = makeSurface() }
            surface?.show(.init(owner: signal.owner,
                                style: NativeMascotPreference.appearance(defaults.string(forKey: NativeMascotPreference.style) ?? ""),
                                expression: state.expression, revision: state.revision,
                                animate: defaults.object(forKey: NativeMascotPreference.animated) as? Bool ?? true,
                                reduceMotion: NSWorkspace.shared.accessibilityDisplayShouldReduceMotion), frame: frame)
        } onChange: { [weak self] in
            Task { @MainActor [weak self] in self?.scheduleRefresh() }
        }
    }

    private func releaseSurface() { surface?.close(); surface = nil }

    func stop() {
        stopped = true; refresh?.cancel(); refresh = nil
        for (center, token) in observers { center.removeObserver(token) }
        observers.removeAll(); releaseSurface()
    }
}

@MainActor final class NativeDesktopPetPanel: NSPanel, NativeDesktopPetSurface {
    private let canvas: NativeMascotCanvas
    override var canBecomeKey: Bool { false }
    override var canBecomeMain: Bool { false }

    init(root: URL = (Bundle.main.resourceURL ?? Bundle.main.bundleURL).appendingPathComponent("Mascot")) {
        canvas = NativeMascotCanvas(root: root)
        super.init(contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: true)
        title = "Gator Desktop Pet"; identifier = NSUserInterfaceItemIdentifier("gator.desktopPet")
        backgroundColor = .clear; isOpaque = false; hasShadow = false; isReleasedWhenClosed = false
        isRestorable = false; isMovable = false; hidesOnDeactivate = false; ignoresMouseEvents = true
        isFloatingPanel = true; becomesKeyOnlyIfNeeded = true; worksWhenModal = false
        level = .floating; animationBehavior = .none
        collectionBehavior = [.canJoinAllSpaces, .transient, .ignoresCycle, .fullScreenNone]
        setAccessibilityElement(false)
        contentView = canvas
    }

    func show(_ request: NativeMascotRequest, frame: CGRect) {
        if self.frame != frame { setFrame(frame, display: true) }
        canvas.request = request
        if !isVisible { orderFrontRegardless() }
        canvas.refreshVisibility()
    }

    override func close() { canvas.detach(); orderOut(nil); contentView = nil; super.close() }
}

struct NativeDesktopPetPolicy {
    static func tuckAway(_ defaults: UserDefaults = .standard) { defaults.set(false, forKey: NativeDesktopPetPreference.shown) }
}
