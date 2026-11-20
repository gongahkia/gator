import SwiftUI

struct NativeMascotPreferencesView: View {
    @AppStorage(NativeMascotPreference.shown) private var shown = true
    @AppStorage(NativeMascotPreference.animated) private var animated = true
    @AppStorage(NativeMascotPreference.style) private var style = NativeMascotStyle.rendered.rawValue
    @AppStorage(NativeDesktopPetPreference.shown) private var desktopPet = false
    @AppStorage(NativeDesktopPetPreference.display) private var display = ""
    @State private var displays = NativeDesktopPetScreen.current()
    var body: some View {
        Form {
            Section("Gator Mascot") {
                Toggle("Show In-Chat Mascot", isOn: $shown).accessibilityIdentifier("gator.mascot.shown")
                Picker("Mascot Style", selection: Binding(get: { NativeMascotPreference.appearance(style).rawValue }, set: { style = $0 })) {
                    ForEach(NativeMascotStyle.allCases, id: \.rawValue) { appearance in Text(appearance.label).tag(appearance.rawValue) }
                }.accessibilityIdentifier("gator.mascot.style")
                Toggle("Animate Mascot", isOn: $animated).accessibilityIdentifier("gator.mascot.animated")
                Text("Short reactions to the current conversation. macOS Reduce Motion takes precedence.").font(.callout).foregroundStyle(.secondary)
            }
            Section("Desktop Pet") {
                Toggle("Show Desktop Pet", isOn: $desktopPet).accessibilityIdentifier("gator.desktopPet.shown")
                Picker("Display", selection: $display) {
                    Text("Primary Display").tag("")
                    ForEach(displays.filter { !$0.id.isEmpty }, id: \.id) { screen in Text(screen.name).tag(screen.id) }
                    if !display.isEmpty && !displays.contains(where: { $0.id == display }) {
                        Text("Chosen display unavailable · using primary").tag(display)
                    }
                }.accessibilityIdentifier("gator.desktopPet.display")
                Button("Tuck Away Desktop Pet") { desktopPet = false }.disabled(!desktopPet)
                    .accessibilityIdentifier("gator.desktopPet.tuck")
                Text("Click-through companion for the current conversation. Uses the style and animation settings above, independently of the in-chat mascot.")
                    .font(.callout).foregroundStyle(.secondary)
                Text("May remain visible in fullscreen or shared screens. Tuck Away hides it until you explicitly turn it on again.")
                    .font(.callout).foregroundStyle(.secondary)
            }
        }.formStyle(.grouped).frame(width: 420).fixedSize(horizontal: false, vertical: true)
            .onReceive(NotificationCenter.default.publisher(for: NSApplication.didChangeScreenParametersNotification)) { _ in
                displays = NativeDesktopPetScreen.current()
            }
    }
}

struct NativeDesktopPetMenu: View {
    @AppStorage(NativeDesktopPetPreference.shown) private var shown = false
    var body: some View {
        Toggle("Show Desktop Pet", isOn: $shown)
        Button("Tuck Away Desktop Pet") { shown = false }.disabled(!shown)
    }
}
