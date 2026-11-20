import SwiftUI
import AppKit

struct NativeThinkingStatus: View {
    let text: String
    let icon: String
    let owner: String
    let pulsing: Bool
    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 6) {
            Image(systemName: icon).foregroundStyle(.secondary).accessibilityHidden(true)
            NativePulseLabel(text: text, owner: owner, active: pulsing).frame(maxWidth: .infinity, alignment: .leading)
        }
    }
}
struct NativePulseLabel: NSViewRepresentable {
    let text: String
    let owner: String
    let active: Bool
    func makeNSView(context: Context) -> PulseField { PulseField() }
    func updateNSView(_ view: PulseField, context: Context) { view.configure(text: text, owner: owner, active: active) }
    func sizeThatFits(_ proposal: ProposedViewSize, nsView: PulseField, context: Context) -> CGSize? {
        nsView.cell?.cellSize(forBounds: CGRect(x: 0, y: 0, width: proposal.width ?? 400, height: .greatestFiniteMagnitude))
    }
    static func dismantleNSView(_ view: PulseField, coordinator: ()) { view.stop() }
    @MainActor final class PulseField: NSTextField {
        private(set) var pulsing = false
        private var owner = ""
        var install: (CALayer, CABasicAnimation) -> Void = { $0.add($1, forKey: "gator.thinking") }
        init() {
            super.init(frame: .zero); isEditable = false; isSelectable = false; isBordered = false; drawsBackground = false
            maximumNumberOfLines = 0; lineBreakMode = .byWordWrapping
            font = .preferredFont(forTextStyle: .callout); textColor = .labelColor; wantsLayer = true
        }
        required init?(coder: NSCoder) { nil }
        override var acceptsFirstResponder: Bool { false }
        func configure(text: String, owner: String, active: Bool) {
            if !stringValue.utf8.elementsEqual(text.utf8) { stringValue = text; setAccessibilityLabel(text); invalidateIntrinsicContentSize() }
            if self.owner != owner { stop(); self.owner = owner }
            guard pulsing != active else { return }
            stop()
            if active, let layer {
                let animation = CABasicAnimation(keyPath: "opacity")
                animation.fromValue = 0.82; animation.toValue = 1.0; animation.duration = 1.3
                animation.autoreverses = true; animation.repeatCount = .infinity; animation.timingFunction = CAMediaTimingFunction(name: .easeInEaseOut)
                install(layer, animation); pulsing = true
            }
        }
        func stop() { layer?.removeAnimation(forKey: "gator.thinking"); layer?.opacity = 1; pulsing = false }
    }
}
