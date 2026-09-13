import SwiftUI

// Which-key-style HUD shown while the leader chord is active. Lists the
// set of next-key possibilities derived from the current prefix, so users
// don't have to memorise the bindings before the muscle memory kicks in.
struct ChordHUD: View {
    let currentKeys: [String] // keys pressed so far after the leader
    let hints: [ChordHudHint]

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            header
            Divider().hcbScaledFrame(height: 44)
            hintsGrid
        }
        .hcbScaledPadding(.horizontal, 14)
        .hcbScaledPadding(.vertical, 10)
        .background(
            RoundedRectangle(cornerRadius: 10)
                .fill(.regularMaterial)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
        .shadow(color: .black.opacity(0.15), radius: 10, x: 0, y: 3)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text("Leader")
                .hcbFont(.caption2, weight: .semibold)
                .foregroundStyle(.secondary)
            Text("⌘K \(currentKeys.joined(separator: " "))")
                .font(.system(.body, design: .monospaced, weight: .semibold))
                .foregroundStyle(AppColor.ember)
            Text("Esc cancels · 3s timeout")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private var hintsGrid: some View {
        if hints.isEmpty {
            Text("No bindings start with this prefix.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
        } else {
            // Fixed 2-column grid — readable at a glance without consuming
            // a lot of vertical space while the user is mid-chord.
            let columns = [GridItem(.flexible(), alignment: .leading),
                           GridItem(.flexible(), alignment: .leading)]
            LazyVGrid(columns: columns, alignment: .leading, spacing: 4) {
                ForEach(hints, id: \.key) { hint in
                    HStack(spacing: 8) {
                        Text(hint.key.uppercased())
                            .font(.system(.caption, design: .monospaced, weight: .semibold))
                            .foregroundStyle(AppColor.ink)
                            .frame(width: 18, alignment: .center)
                            .hcbScaledPadding(.horizontal, 4)
                            .hcbScaledPadding(.vertical, 1)
                            .background(
                                RoundedRectangle(cornerRadius: 4)
                                    .fill(.quaternary.opacity(0.6))
                            )
                        Text(hint.label)
                            .hcbFont(.caption)
                            .foregroundStyle(AppColor.ink)
                            .lineLimit(1)
                    }
                }
            }
        }
    }
}
