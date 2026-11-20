import SwiftUI

struct NativeMascotPreferencesView: View {
    @AppStorage(NativeMascotPreference.shown) private var shown = true
    @AppStorage(NativeMascotPreference.animated) private var animated = true
    @AppStorage(NativeMascotPreference.mode) private var mode = NativePresenceMode.emoji.rawValue
    @AppStorage(NativeDesktopPetPreference.shown) private var desktopPet = false
    @AppStorage(NativeDesktopPetPreference.display) private var display = ""
    @State private var displays = NativeDesktopPetScreen.current()
    var body: some View {
        Form {
            Section("Agent Presence") {
                Toggle("Show In-Chat Presence", isOn: $shown).accessibilityIdentifier("gator.mascot.shown")
                Picker("Agent Presence", selection: Binding(get: { NativeMascotPreference.presence(mode).rawValue }, set: { mode = $0 })) {
                    ForEach(NativePresenceMode.allCases, id: \.rawValue) { appearance in Text(appearance.label).tag(appearance.rawValue) }
                }.accessibilityIdentifier("gator.presence.mode")
                Toggle("Animate Reactions", isOn: $animated).accessibilityIdentifier("gator.mascot.animated")
                Text("Native emoji reflect the current conversation. Optional GIF accents play once for completion or attention; macOS Reduce Motion takes precedence.").font(.callout).foregroundStyle(.secondary)
            }
            Section("Desktop Presence") {
                Toggle("Show Desktop Presence", isOn: $desktopPet).accessibilityIdentifier("gator.desktopPet.shown")
                Picker("Display", selection: $display) {
                    Text("Primary Display").tag("")
                    ForEach(displays.filter { !$0.id.isEmpty }, id: \.id) { screen in Text(screen.name).tag(screen.id) }
                    if !display.isEmpty && !displays.contains(where: { $0.id == display }) {
                        Text("Chosen display unavailable · using primary").tag(display)
                    }
                }.accessibilityIdentifier("gator.desktopPet.display")
                Button("Tuck Away Desktop Presence") { desktopPet = false }.disabled(!desktopPet)
                    .accessibilityIdentifier("gator.desktopPet.tuck")
                Text("Click-through emoji for the current conversation. Uses Agent Presence and Animate Reactions above, independently of in-chat visibility. Off hides both surfaces without changing their visibility choices.")
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
        Toggle("Show Desktop Presence", isOn: $shown)
        Button("Tuck Away Desktop Presence") { shown = false }.disabled(!shown)
    }
}
