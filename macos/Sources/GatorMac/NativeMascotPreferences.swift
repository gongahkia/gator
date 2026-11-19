import SwiftUI

struct NativeMascotPreferencesView: View {
    @AppStorage(NativeMascotPreference.shown) private var shown = true
    @AppStorage(NativeMascotPreference.animated) private var animated = true
    @AppStorage(NativeMascotPreference.style) private var style = NativeMascotStyle.rendered.rawValue
    var body: some View {
        Form {
            Section("Gator Mascot") {
                Toggle("Show Gator Mascot", isOn: $shown).accessibilityIdentifier("gator.mascot.shown")
                Picker("Mascot Style", selection: Binding(get: { NativeMascotPreference.appearance(style).rawValue }, set: { style = $0 })) {
                    ForEach(NativeMascotStyle.allCases, id: \.rawValue) { appearance in Text(appearance.label).tag(appearance.rawValue) }
                }.accessibilityIdentifier("gator.mascot.style")
                Toggle("Animate Mascot", isOn: $animated).accessibilityIdentifier("gator.mascot.animated")
                Text("Short reactions to the current conversation. macOS Reduce Motion takes precedence.").font(.callout).foregroundStyle(.secondary)
            }
        }.formStyle(.grouped).frame(width: 420).fixedSize(horizontal: false, vertical: true)
    }
}
