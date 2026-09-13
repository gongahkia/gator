import SwiftUI

struct PreparedSnapshotOverlay: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    var title: String = "Preparing view..."
    var message: String

    var body: some View {
        LoadingBunsIcon(reduceMotion: reduceMotion, size: 72)
            .padding(18)
            .background(.regularMaterial, in: Circle())
            .shadow(color: .black.opacity(0.08), radius: 18, x: 0, y: 10)
            .accessibilityLabel("\(title) \(message)")
            .frame(maxWidth: .infinity, maxHeight: .infinity)
            .background(.regularMaterial.opacity(0.18))
    }
}
