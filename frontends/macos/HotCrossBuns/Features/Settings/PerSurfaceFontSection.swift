import SwiftUI

// Per-surface font overrides (§6.11). Sits under the main Appearance block.
// Each surface row: font family menu + size stepper. Unset fields show
// "(inherit)" and fall through to the global Appearance values above. All
// surfaces (editor, sidebar, calendar grid, task list, inspector, menu bar)
// honour overrides via the .hcbSurface(_:) environment modifier.
struct PerSurfaceFontSection: View {
    @Environment(AppModel.self) private var model
    @State private var availableFonts: [String] = []
    @State private var searchText: String = ""

    var body: some View {
        Section("Per-surface fonts") {
            Text("Override the global font + size for specific surfaces. Settings persist locally and never touch Google data. Unset surfaces inherit the global Appearance values above.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
            ForEach(HCBSurface.allCases, id: \.self) { surface in
                row(for: surface)
            }
        }
        .task {
            if availableFonts.isEmpty {
                availableFonts = HCBInstalledFonts.available()
            }
        }
    }

    @ViewBuilder
    private func row(for surface: HCBSurface) -> some View {
        let current = model.settings.perSurfaceFontOverrides[surface.rawValue] ?? .empty
        VStack(alignment: .leading, spacing: 4) {
            HStack {
                Label(surface.title, systemImage: surface.systemImage)
                    .hcbFont(.subheadline, weight: .medium)
                Spacer()
                Button("Reset") {
                    model.setPerSurfaceFont(surface, override: .empty)
                }
                .buttonStyle(.borderless)
                .hcbFont(.caption)
                .disabled(current.isEmpty)
            }
            HStack(spacing: 8) {
                Menu {
                    Button("(Inherit global)") {
                        var next = current
                        next.fontName = nil
                        model.setPerSurfaceFont(surface, override: next)
                    }
                    Divider()
                    ForEach(availableFonts.prefix(200), id: \.self) { family in
                        Button(family) {
                            var next = current
                            next.fontName = family
                            model.setPerSurfaceFont(surface, override: next)
                        }
                    }
                    if availableFonts.count > 200 {
                        Divider()
                        Text("Showing first 200 fonts — use Appearance search above to browse all")
                            .foregroundStyle(.secondary)
                    }
                } label: {
                    HStack(spacing: 4) {
                        Image(systemName: "textformat")
                        Text(current.fontName ?? "(inherit)")
                            .lineLimit(1)
                    }
                }
                .menuStyle(.borderlessButton)
                .fixedSize()
                Spacer(minLength: 12)
                HStack(spacing: 4) {
                    Text("Size")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                    Stepper(
                        "",
                        value: Binding(
                            get: { current.pointSize ?? 0 },
                            set: { newValue in
                                var next = current
                                next.pointSize = newValue <= 0 ? nil : HCBTextSize.clamp(newValue)
                                model.setPerSurfaceFont(surface, override: next)
                            }
                        ),
                        in: 0 ... HCBTextSize.maxPoints,
                        step: HCBTextSize.stepPoints
                    )
                    .labelsHidden()
                    Text(current.pointSize.map { "\(Int($0))pt" } ?? "(inherit)")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .monospacedDigit()
                        .frame(minWidth: 54, alignment: .trailing)
                }
            }
        }
    }
}
