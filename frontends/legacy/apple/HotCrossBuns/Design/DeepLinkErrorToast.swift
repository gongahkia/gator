import SwiftUI

// Ephemeral toast shown when a hotcrossbuns:// URL can't be parsed. Normal
// routes are silent; this surfaces typos and malformed scripts so a user
// firing a deep link isn't left wondering why nothing happened.
struct DeepLinkErrorToast: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Binding var message: String?
    // Auto-dismiss after this delay. Longer than UndoToast (6s) since the
    // message is diagnostic text the user may want to read in full.
    private let dismissAfter: Duration = .seconds(7)

    var body: some View {
        VStack {
            Spacer(minLength: 0)
            if let message {
                content(message: message)
                    .transition(HCBMotion.transition(.move(edge: .bottom).combined(with: .opacity), reduceMotion: reduceMotion))
                    .task(id: message) {
                        try? await Task.sleep(for: dismissAfter)
                        // Guard — another error may have replaced this one; don't clobber.
                        if self.message == message { self.message = nil }
                    }
            }
        }
        .animation(HCBMotion.animation(.easeOut(duration: 0.16), reduceMotion: reduceMotion), value: message)
        .allowsHitTesting(message != nil)
    }

    private func content(message: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: "link.badge.plus")
                .foregroundStyle(.orange)
            VStack(alignment: .leading, spacing: 2) {
                Text("Deep link error")
                    .hcbFont(.subheadline, weight: .semibold)
                Text(message)
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 12)
            Button { self.message = nil } label: {
                Image(systemName: "xmark")
                    .hcbFont(.caption)
            }
            .buttonStyle(.borderless)
            .accessibilityLabel("Dismiss")
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 12)
        .hcbScaledFrame(maxWidth: 520)
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
