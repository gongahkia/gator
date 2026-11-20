import SwiftUI
import AppKit
@MainActor final class ProbeDelegate: NSObject, NSApplicationDelegate {
    var pet: NativeDesktopPet?
    func applicationDidFinishLaunching(_ notification: Notification) {
        pet = NativeDesktopPet(readSignal: { .init(owner: "probe", connected: true, attention: true) }, isClosing: { false })
    }
    func applicationWillTerminate(_ notification: Notification) { pet?.stop() }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { false }
}
struct ProbePresence: NSViewRepresentable {
    let request: NativeMascotRequest
    func makeNSView(context: Context) -> NativeMascotCanvas { NativeMascotCanvas(root: Bundle.main.resourceURL!.appendingPathComponent("Presence")) }
    func updateNSView(_ view: NativeMascotCanvas, context: Context) { view.request = request }
    static func dismantleNSView(_ view: NativeMascotCanvas, coordinator: ()) { view.detach() }
}
struct ProbeContent: View {
    @AppStorage(NativeMascotPreference.shown) var shown = true
    @AppStorage(NativeMascotPreference.mode) var mode = "emoji"
    var body: some View {
        VStack {
            Text("Isolated presentation lifecycle; no Go process or transport")
            if shown && NativeMascotPreference.presence(mode) != .off {
                ProbePresence(request: .init(owner: "probe", mode: NativeMascotPreference.presence(mode), expression: .attention)).frame(width: 42, height: 42)
            }
            NativeMascotPreferencesView()
        }.padding()
    }
}
@main struct LifecycleProbe: App {
    @NSApplicationDelegateAdaptor(ProbeDelegate.self) var delegate
    var body: some Scene { Window("Lifecycle Probe", id: "probe") { ProbeContent() } }
}
