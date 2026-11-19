import AppKit
import ImageIO
import QuartzCore

struct NativeMascotFrames: Sendable {
    let images: [CGImage]
    let bytes: Int
    let duration: Double
}
actor NativeMascotFrameStore {
    static let maximumBytes = 3 * 1024 * 1024
    private let root: URL
    private var latest = 0
    private var cached: NativeMascotFrames?
    private var key = ""
    private(set) var decodeCount = 0
    var bytes: Int { cached?.bytes ?? 0 }
    init(root: URL) { self.root = root }
    func clear(ticket: Int) {
        guard ticket >= latest else { return }
        latest = ticket; cached = nil; key = ""
    }
    func load(style: NativeMascotStyle, name: String, animate: Bool, ticket: Int) throws -> NativeMascotFrames {
        guard ticket >= latest else { throw CancellationError() }; latest = ticket
        guard let clip = NativeMascotClip.spec(style: style, name: name) else { throw CocoaError(.fileReadCorruptFile) }
        let requested = style.rawValue + "/" + name + (animate ? "/motion" : "/static")
        if key == requested, let cached { return cached }
        cached = nil; key = ""
        var images: [CGImage] = [], bytes = 0
        let indexes = animate && clip.count > 1 ? Array(0..<clip.count) : [clip.count - 1]
        for index in indexes {
            try Task.checkCancellation()
            let url = root.appendingPathComponent(style.rawValue).appendingPathComponent(name).appendingPathComponent(String(format: "%02d.png", index))
            guard let source = CGImageSourceCreateWithURL(url as CFURL, [kCGImageSourceShouldCache: false] as CFDictionary),
                  let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil) as? [CFString: Any],
                  let width = properties[kCGImagePropertyPixelWidth] as? Int, let height = properties[kCGImagePropertyPixelHeight] as? Int,
                  width > 0, height > 0, width <= 224, height <= 224,
                  let image = CGImageSourceCreateImageAtIndex(source, 0, [kCGImageSourceShouldCacheImmediately: true] as CFDictionary)
            else { throw CocoaError(.fileReadCorruptFile) }
            bytes += image.bytesPerRow * image.height
            guard bytes <= Self.maximumBytes else { throw CocoaError(.fileReadTooLarge) }
            images.append(image); decodeCount += 1
        }
        let result = NativeMascotFrames(images: images, bytes: bytes, duration: clip.duration)
        cached = result; key = requested; return result
    }
}

@MainActor private final class MascotAnimationCompletion: NSObject, CAAnimationDelegate {
    weak var player: NativeMascotPlayer?
    let ticket: Int
    init(player: NativeMascotPlayer, ticket: Int) { self.player = player; self.ticket = ticket }
    nonisolated func animationDidStop(_ anim: CAAnimation, finished flag: Bool) {
        Task { @MainActor [weak self] in guard let self else { return }; player?.finished(ticket: ticket) }
    }
}

@MainActor final class NativeMascotPlayer {
    let layer = CALayer()
    let store: NativeMascotFrameStore
    private(set) var ticket = 0
    private(set) var animationStarts = 0
    private(set) var current: NativeMascotRequest?
    private(set) var displayedStyle: NativeMascotStyle?
    private(set) var activeFrameBytes = 0
    private var task: Task<Void, Never>?
    private var blinkTask: Task<Void, Never>?
    private var completion: MascotAnimationCompletion?
    private var lastGesture: ContinuousClock.Instant?
    private let now: () -> ContinuousClock.Instant
    private let blinkDelay: Duration
    init(root: URL, blinkDelay: Duration = .seconds(11), now: @escaping () -> ContinuousClock.Instant = { .now }) {
        store = NativeMascotFrameStore(root: root); self.blinkDelay = blinkDelay; self.now = now
        layer.contentsGravity = .resizeAspect; layer.isOpaque = false
        layer.actions = ["contents": NSNull(), "bounds": NSNull(), "position": NSNull()]
    }
    deinit { task?.cancel(); blinkTask?.cancel() }
    func configure(_ request: NativeMascotRequest) {
        guard current != request else { return }
        let old = current; current = request
        if request.expression == .responding, old?.expression == .responding, old?.owner == request.owner, old?.style == request.style,
           old?.shown == request.shown, old?.visible == request.visible, old?.motionAllowed == request.motionAllowed,
           let lastGesture, lastGesture.duration(to: now()) < .milliseconds(650) { return }
        begin(name: request.expression.rawValue)
    }
    private func begin(name: String) {
        guard let request = current else { return }
        ticket += 1; let epoch = ticket
        task?.cancel(); blinkTask?.cancel(); blinkTask = nil
        layer.removeAllAnimations(); completion = nil; activeFrameBytes = 0
        if !request.shown || !request.visible {
            layer.contents = nil; displayedStyle = nil
            task = Task { [store] in await store.clear(ticket: epoch) }; return
        }
        let animate = request.motionAllowed && name != "neutral"
        if animate { lastGesture = now() }
        task = Task { [weak self, store] in
            do {
                let frames: NativeMascotFrames
                do { frames = try await store.load(style: request.style, name: name, animate: animate, ticket: epoch) }
                catch is CancellationError { return }
                catch { frames = try await store.load(style: request.style, name: "neutral", animate: false, ticket: epoch) }
                try Task.checkCancellation()
                guard let self, ticket == epoch, let final = frames.images.last else { return }
                layer.contents = final; displayedStyle = request.style
                if animate && frames.images.count > 1 {
                    let animation = CAKeyframeAnimation(keyPath: "contents")
                    animation.values = frames.images; animation.duration = frames.duration; animation.calculationMode = .discrete
                    animation.keyTimes = frames.images.indices.map { NSNumber(value: Double($0) / Double(frames.images.count - 1)) }
                    animation.isRemovedOnCompletion = true
                    let callback = MascotAnimationCompletion(player: self, ticket: epoch); animation.delegate = callback; completion = callback
                    activeFrameBytes = frames.bytes; animationStarts += 1; layer.add(animation, forKey: "gator.expression")
                } else {
                    await store.clear(ticket: epoch)
                    scheduleBlink()
                }
            } catch {
                guard let self, ticket == epoch else { return }
                layer.contents = nil; displayedStyle = nil; await store.clear(ticket: epoch)
            }
        }
    }
    fileprivate func finished(ticket: Int) {
        guard self.ticket == ticket else { return }
        layer.removeAnimation(forKey: "gator.expression"); completion = nil; activeFrameBytes = 0
        task = Task { [weak self, store] in
            await store.clear(ticket: ticket)
            guard let self, self.ticket == ticket else { return }; scheduleBlink()
        }
    }
    private func scheduleBlink() {
        guard let current, current.expression == .neutral, current.shown, current.visible, current.motionAllowed, layer.contents != nil else { return }
        let epoch = ticket
        blinkTask?.cancel()
        blinkTask = Task { [weak self, blinkDelay] in
            do { try await Task.sleep(for: blinkDelay) } catch { return }
            guard let self, ticket == epoch else { return }; begin(name: "blink")
        }
    }
    func settled() async { await task?.value }
    func stop() {
        guard var request = current else { return }; request.visible = false; configure(request)
    }
}

private final class MascotWindowObservers {
    var tokens: [NSObjectProtocol] = []
    deinit { for token in tokens { NotificationCenter.default.removeObserver(token) } }
}

@MainActor final class NativeMascotCanvas: NSView {
    let player: NativeMascotPlayer
    private var observers: MascotWindowObservers?
    var request: NativeMascotRequest? { didSet { refreshVisibility() } }
    override var acceptsFirstResponder: Bool { false }
    init(root: URL) {
        player = NativeMascotPlayer(root: root)
        super.init(frame: .zero); wantsLayer = true; layer = CALayer(); layer?.addSublayer(player.layer)
        setAccessibilityElement(false)
    }
    required init?(coder: NSCoder) { nil }
    override func layout() { super.layout(); player.layer.frame = bounds }
    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow(); observers = nil
        if let window {
            let bag = MascotWindowObservers()
            for name in [NSWindow.didChangeOcclusionStateNotification, NSWindow.didMiniaturizeNotification, NSWindow.didDeminiaturizeNotification, NSWindow.willCloseNotification] {
                bag.tokens.append(NotificationCenter.default.addObserver(forName: name, object: window, queue: .main) { [weak self] notification in
                    let closing = notification.name == NSWindow.willCloseNotification
                    MainActor.assumeIsolated {
                        if closing { self?.player.stop() }
                        else { self?.refreshVisibility() }
                    }
                })
            }
            observers = bag
        }
        refreshVisibility()
    }
    func refreshVisibility() {
        guard var request else { return }
        request.visible = request.visible && window?.isVisible == true && window?.occlusionState.contains(.visible) == true && !isHiddenOrHasHiddenAncestor
        player.configure(request)
    }
    func detach() { observers = nil; player.stop() }
}
