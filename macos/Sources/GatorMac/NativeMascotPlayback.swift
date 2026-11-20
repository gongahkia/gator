import AppKit
import ImageIO
import QuartzCore

struct NativeMascotFrames: Sendable {
    let images: [CGImage]
    let bytes: Int
    let duration: Double
}
actor NativeMascotFrameStore {
    static let maximumBytes = 1024 * 1024
    static let maximumFileBytes = 256 * 1024
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
    func load(name: String, ticket: Int) throws -> NativeMascotFrames {
        guard ticket >= latest else { throw CancellationError() }; latest = ticket
        guard ["completed", "attention"].contains(name) else { throw CocoaError(.fileReadCorruptFile) }
        if key == name, let cached { return cached }
        cached = nil; key = ""
        let url = root.appendingPathComponent(name + ".gif")
        let size = try url.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
        guard size > 0, size <= Self.maximumFileBytes else { throw CocoaError(.fileReadTooLarge) }
        let data = try Data(contentsOf: url)
        guard data.count <= Self.maximumFileBytes,
              let source = CGImageSourceCreateWithData(data as CFData, [kCGImageSourceShouldCache: false] as CFDictionary),
              CGImageSourceGetType(source) as String? == "com.compuserve.gif"
        else { throw CocoaError(.fileReadCorruptFile) }
        let count = CGImageSourceGetCount(source)
        guard count >= 2, count <= 16 else { throw CocoaError(.fileReadCorruptFile) }
        var images: [CGImage] = [], bytes = 0, duration = 0.0
        for index in 0..<count {
            try Task.checkCancellation()
            guard let properties = CGImageSourceCopyPropertiesAtIndex(source, index, nil) as? [CFString: Any],
                  let width = properties[kCGImagePropertyPixelWidth] as? Int, let height = properties[kCGImagePropertyPixelHeight] as? Int,
                  width > 0, height > 0, width <= 128, height <= 128,
                  let gif = properties[kCGImagePropertyGIFDictionary] as? [CFString: Any],
                  let delay = gif[kCGImagePropertyGIFUnclampedDelayTime] as? Double ?? gif[kCGImagePropertyGIFDelayTime] as? Double,
                  delay >= 0.03, delay <= 0.1,
                  let image = CGImageSourceCreateImageAtIndex(source, index, [kCGImageSourceShouldCacheImmediately: true] as CFDictionary)
            else { throw CocoaError(.fileReadCorruptFile) }
            bytes += image.bytesPerRow * image.height; duration += delay
            guard bytes <= Self.maximumBytes, duration <= 1.6 else { throw CocoaError(.fileReadTooLarge) }
            images.append(image); decodeCount += 1
        }
        let result = NativeMascotFrames(images: images, bytes: bytes, duration: duration)
        cached = result; key = name; return result
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
    private(set) var activeFrameBytes = 0
    private var task: Task<Void, Never>?
    private var completion: MascotAnimationCompletion?
    init(root: URL) {
        store = NativeMascotFrameStore(root: root)
        layer.contentsGravity = .resizeAspect; layer.isOpaque = false
        layer.actions = ["contents": NSNull(), "bounds": NSNull(), "position": NSNull()]
    }
    deinit { task?.cancel() }
    static func keyframes(_ frames: NativeMascotFrames) -> CAKeyframeAnimation {
        let animation = CAKeyframeAnimation(keyPath: "contents")
        animation.values = frames.images; animation.duration = frames.duration; animation.calculationMode = .discrete
        animation.keyTimes = frames.images.indices.map { NSNumber(value: Double($0) / Double(max(1, frames.images.count - 1))) }
        animation.isRemovedOnCompletion = true
        return animation
    }
    func configure(_ request: NativeMascotRequest) {
        guard current != request else { return }
        let old = current; current = request
        if old?.owner == request.owner, old?.mode == request.mode, old?.expression == request.expression,
           request.expression.reaction == nil, old?.shown == request.shown, old?.visible == request.visible,
           old?.motionAllowed == request.motionAllowed { return }
        ticket += 1; let epoch = ticket
        task?.cancel(); layer.removeAllAnimations(); layer.contents = nil; completion = nil; activeFrameBytes = 0
        // reopening, changing mode or returning from hidden must not replay a past reaction.
        let fresh = old?.owner == request.owner && old?.shown == true && old?.visible == true &&
            old?.mode == request.mode && old?.motionAllowed == true && request.revision > (old?.revision ?? request.revision)
        guard request.shown, request.visible, request.mode != .off, request.motionAllowed, fresh,
              let name = request.expression.reaction else {
            task = Task { [store] in await store.clear(ticket: epoch) }; return
        }
        task = Task { [weak self, store] in
            do {
                let frames = try await store.load(name: name, ticket: epoch)
                try Task.checkCancellation()
                guard let self, ticket == epoch else { return }
                layer.contents = frames.images.last
                let animation = Self.keyframes(frames)
                // decoding crosses an actor hop; start at delivery, not an older implicit transaction.
                animation.beginTime = layer.convertTime(CACurrentMediaTime(), from: nil)
                let callback = MascotAnimationCompletion(player: self, ticket: epoch); animation.delegate = callback; completion = callback
                activeFrameBytes = frames.bytes; animationStarts += 1; layer.add(animation, forKey: "gator.expression")
            } catch {
                guard let self, ticket == epoch else { return }
                layer.contents = nil; activeFrameBytes = 0; await store.clear(ticket: epoch)
            }
        }
    }
    fileprivate func finished(ticket: Int) {
        guard self.ticket == ticket else { return }
        layer.removeAnimation(forKey: "gator.expression"); layer.contents = nil; completion = nil; activeFrameBytes = 0
        task = Task { [store] in await store.clear(ticket: ticket) }
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
    let emoji = NSTextField(labelWithString: "")
    private var observers: MascotWindowObservers?
    var request: NativeMascotRequest? { didSet { refreshVisibility() } }
    override var acceptsFirstResponder: Bool { false }
    init(root: URL) {
        player = NativeMascotPlayer(root: root)
        super.init(frame: .zero); wantsLayer = true; layer = CALayer(); layer?.addSublayer(player.layer)
        emoji.alignment = .center; emoji.isSelectable = false; emoji.setAccessibilityElement(false)
        addSubview(emoji); setAccessibilityElement(false)
    }
    required init?(coder: NSCoder) { nil }
    override func layout() {
        super.layout(); player.layer.frame = bounds
        let size = min(bounds.width, bounds.height) * 0.72
        emoji.font = .systemFont(ofSize: size)
        let fitted = emoji.fittingSize
        emoji.frame = CGRect(x: 0, y: (bounds.height - fitted.height) / 2, width: bounds.width, height: fitted.height)
    }
    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow(); observers = nil
        if let window {
            let bag = MascotWindowObservers()
            for name in [NSWindow.didChangeOcclusionStateNotification, NSWindow.didMiniaturizeNotification, NSWindow.didDeminiaturizeNotification, NSWindow.willCloseNotification] {
                bag.tokens.append(NotificationCenter.default.addObserver(forName: name, object: window, queue: .main) { [weak self] notification in
                    let closing = notification.name == NSWindow.willCloseNotification
                    MainActor.assumeIsolated {
                        if closing { self?.player.stop(); self?.emoji.stringValue = "" }
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
        emoji.stringValue = request.visible && request.shown && request.mode != .off ? request.expression.emoji : ""
        player.configure(request); needsLayout = true
    }
    func detach() { observers = nil; player.stop(); emoji.stringValue = "" }
}
