import SwiftUI

// Bottom toast surfaced after a bulk task batch completes. Shows the success /
// failure / dropped-no-op counts so the user knows a batch landed (success is
// otherwise silent, which is scary for destructive ops like Delete N tasks).
//
// Auto-dismisses after 7s; tap the × to dismiss early. Non-blocking — the
// toast is informational only and doesn't trap keyboard focus.
struct BulkResultToast: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Binding var message: String?
    var isWarning: Bool = false
    var successTitle: String = "Bulk action complete"
    var warningTitle: String = "Bulk action partially applied"
    var successSymbol: String = "checkmark.circle.fill"
    var warningSymbol: String = "exclamationmark.triangle.fill"
    private let dismissAfter: Duration = .seconds(7)

    var body: some View {
        VStack {
            Spacer(minLength: 0)
            if let message {
                content(message: message)
                    .transition(HCBMotion.transition(.move(edge: .bottom).combined(with: .opacity), reduceMotion: reduceMotion))
                    .task(id: message) {
                        try? await Task.sleep(for: dismissAfter)
                        if self.message == message { self.message = nil }
                    }
            }
        }
        .animation(HCBMotion.animation(.easeOut(duration: 0.16), reduceMotion: reduceMotion), value: message)
        .allowsHitTesting(message != nil)
    }

    private func content(message: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: isWarning ? warningSymbol : successSymbol)
                .foregroundStyle(isWarning ? .orange : AppColor.moss)
            VStack(alignment: .leading, spacing: 2) {
                Text(isWarning ? warningTitle : successTitle)
                    .hcbFont(.subheadline, weight: .semibold)
                Text(message)
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 12)
            Button { self.message = nil } label: {
                Image(systemName: "xmark").hcbFont(.caption)
            }
            .buttonStyle(.borderless)
            .accessibilityLabel("Dismiss")
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 12)
        .hcbScaledFrame(maxWidth: 560)
        .background(
            RoundedRectangle(cornerRadius: 10)
                .fill(.regularMaterial)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
        .shadow(color: .black.opacity(0.15), radius: 10, x: 0, y: 3)
        .hcbScaledPadding(18)
    }
}
