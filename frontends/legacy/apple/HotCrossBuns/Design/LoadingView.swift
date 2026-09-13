import SwiftUI
import AppKit
import QuartzCore

struct LoadingView: View {
    let message: String
    @Environment(\.hcbReduceMotion) private var reduceMotion

    var body: some View {
        ZStack {
            Rectangle()
                .fill(.regularMaterial)
                .ignoresSafeArea()

            VStack(spacing: 16) {
                LoadingBunsIcon(reduceMotion: reduceMotion)

                Text(message)
                    .hcbFont(.headline)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(AppColor.ink)
            }
            .padding(28)
        }
    }
}

struct LoadingBunsIcon: View {
    let reduceMotion: Bool
    var size: CGFloat = 104

    var body: some View {
        Group {
            if reduceMotion {
                Image("LoadingBunsWelcome")
                    .resizable()
                    .scaledToFit()
            } else {
                CoreAnimationLoadingBunsIcon()
            }
        }
        .frame(width: size, height: size)
        .accessibilityHidden(true)
    }
}

private struct CoreAnimationLoadingBunsIcon: NSViewRepresentable {
    func makeNSView(context: Context) -> RotatingLoadingBunsView {
        let view = RotatingLoadingBunsView()
        view.startAnimating()
        return view
    }

    func updateNSView(_ nsView: RotatingLoadingBunsView, context: Context) {
        nsView.startAnimating()
    }

    static func dismantleNSView(_ nsView: RotatingLoadingBunsView, coordinator: ()) {
        // Let Core Animation own the final frames if SwiftUI keeps the view
        // alive briefly for an exit transition. The layer is torn down with
        // the NSView, so there is no persistent animation to cancel here.
    }
}

private final class RotatingLoadingBunsView: NSView {
    private static let animationKey = "hcb.loadingBuns.rotation"
    private let imageLayer = CALayer()

    override init(frame frameRect: NSRect) {
        super.init(frame: frameRect)
        wantsLayer = true
        setAccessibilityElement(false)
        imageLayer.contentsGravity = .resizeAspect
        imageLayer.anchorPoint = CGPoint(x: 0.5, y: 0.5)
        imageLayer.contents = Self.loadingImage()
        layer?.addSublayer(imageLayer)
        updateContentsScale()
    }

    @available(*, unavailable)
    required init?(coder: NSCoder) {
        nil
    }

    override var intrinsicContentSize: NSSize {
        NSSize(width: 104, height: 104)
    }

    override func layout() {
        super.layout()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        let side = min(bounds.width, bounds.height)
        imageLayer.bounds = CGRect(x: 0, y: 0, width: side, height: side)
        imageLayer.position = CGPoint(x: bounds.midX, y: bounds.midY)
        CATransaction.commit()
    }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        guard window != nil else { return }
        updateContentsScale()
        startAnimating()
    }

    override func viewDidChangeBackingProperties() {
        super.viewDidChangeBackingProperties()
        updateContentsScale()
    }

    func startAnimating() {
        guard imageLayer.animation(forKey: Self.animationKey) == nil else { return }
        let animation = CABasicAnimation(keyPath: "transform.rotation.z")
        animation.fromValue = 0
        animation.toValue = CGFloat.pi * 2
        animation.duration = 1.25
        animation.repeatCount = .infinity
        animation.timingFunction = CAMediaTimingFunction(name: .linear)
        animation.isRemovedOnCompletion = false
        imageLayer.add(animation, forKey: Self.animationKey)
    }

    func stopAnimating() {
        imageLayer.removeAnimation(forKey: Self.animationKey)
    }

    deinit {
        stopAnimating()
    }

    private func updateContentsScale() {
        imageLayer.contentsScale = window?.backingScaleFactor ?? NSScreen.main?.backingScaleFactor ?? 2
    }

    private static func loadingImage() -> CGImage? {
        NSImage(named: "LoadingBunsWelcome")?.cgImage(forProposedRect: nil, context: nil, hints: nil)
    }
}

struct LoadingOverlayModifier: ViewModifier {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let state: LoadingOverlayState?

    func body(content: Content) -> some View {
        content
            .overlay {
                if let state {
                    LoadingView(message: state.message)
                        .hcbMotionTransition(.opacity)
                }
            }
            .animation(HCBMotion.animation(.easeInOut(duration: 0.18), reduceMotion: reduceMotion), value: state)
    }
}

extension View {
    func loadingOverlay(_ state: LoadingOverlayState?) -> some View {
        modifier(LoadingOverlayModifier(state: state))
    }
}
