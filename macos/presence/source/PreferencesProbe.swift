import SwiftUI
import AppKit

struct NativeDesktopPetScreen {
    var id: String
    var name: String
    static func current() -> [Self] { [] }
}
enum NativeDesktopPetPreference {
    static let shown = "gator.desktopPet.shown"
    static let display = "gator.desktopPet.display"
}
@main struct PreferencesProbe: App {
    var body: some Scene {
        Window("Presence Preferences Probe", id: "probe") { NativeMascotPreferencesView() }
    }
}
