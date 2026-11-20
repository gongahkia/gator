import SwiftUI
import AppKit
import GatorCore

enum NativeMascotProjection {
    @MainActor static func signal(_ application: GatorApplicationState) -> NativeMascotSignal {
        signal(snapshot: application.snapshot, messages: application.messages,
               owner: application.context + ":" + application.generation.uuidString,
               connected: application.isConnected, reviewing: application.review != nil)
    }
    static func signal(snapshot: BridgeSnapshot?, messages: [BridgeMessage], owner: String, connected: Bool, reviewing: Bool) -> NativeMascotSignal {
        let publicBytes = snapshot?.value["activity"]["entries"].array.filter { $0["kind"].string == "text" }.reduce(0) { $0 + $1["text"].string.utf8.count } ?? 0
        let outcome = messages.last { ["completed", "failed", "stopped"].contains($0.value["activity"]["state"].string) }
        let identity = outcome.map { $0.value["run_id"].string + ":" + $0.value["activity"]["finished_at"].string } ?? ""
        let confirmed = outcome?.value["activity"]["finished_at"].string.isEmpty == false && outcome?.value["run_id"].string.isEmpty == false
        let completed = confirmed && outcome?.value["activity"]["state"].string == "completed" ? identity : ""
        let failed = confirmed && outcome?.value["activity"]["state"].string == "failed" ? identity : ""
        let attention = snapshot?.hasApproval == true || reviewing || snapshot?.value["task_plans"].array.contains { $0["phase"].string == "review" } == true
        let publicTextActive = snapshot?.value["activity"]["entries"].array.contains { $0["kind"].string == "text" && $0["state"].string == "running" } == true
        return NativeMascotSignal(owner: owner,
                                 connected: connected, working: snapshot?.running == true || snapshot?.value["dialogue_busy"].bool == true,
                                 attention: attention, publicTextRevision: publicBytes, completedRun: completed, failedRun: failed, publicTextActive: publicTextActive)
    }
}

struct NativeMascotReadingKey: PreferenceKey {
    static let defaultValue = false
    static func reduce(value: inout Bool, nextValue: () -> Bool) { value = value || nextValue() }
}

struct NativeMascotHeader: View {
    let application: GatorApplicationState
    let compact: Bool
    let reading: Bool
    let reviewing: Bool
    let height: CGFloat
    @AppStorage(NativeMascotPreference.shown) private var shown = true
    @AppStorage(NativeMascotPreference.animated) private var animated = true
    @AppStorage(NativeMascotPreference.mode) private var mode = NativePresenceMode.emoji.rawValue
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var state = NativeMascotStateMachine()
    private var signal: NativeMascotSignal { NativeMascotProjection.signal(application) }
    private var collapsed: Bool { NativeMascotStateMachine.collapsed(reading: reading, reviewing: reviewing, compact: compact, height: height) }
    private var layout: NativePresenceLayout { .init(compact: compact, collapsed: collapsed) }
    var body: some View {
        Group {
            if shown && NativeMascotPreference.presence(mode) != .off {
                HStack {
                    Spacer(minLength: 0)
                    NativeMascotRepresentation(request: .init(owner: signal.owner, mode: NativeMascotPreference.presence(mode),
                        expression: state.previous?.owner == signal.owner ? state.expression : (signal.connected ? .neutral : .disconnected),
                        revision: state.revision, animate: animated, shown: !application.closing, reduceMotion: reduceMotion))
                        .frame(width: layout.size, height: layout.size).offset(x: compact ? 12 : 24)
                    Spacer(minLength: 0)
                }
                .frame(height: layout.slot)
                .allowsHitTesting(false).accessibilityHidden(true)
            }
        }
        .onChange(of: signal, initial: true) { _, value in state.receive(value) }
    }
}

struct NativeMascotRepresentation: NSViewRepresentable {
    let request: NativeMascotRequest
    func makeNSView(context: Context) -> NativeMascotCanvas {
        NativeMascotCanvas(root: (Bundle.main.resourceURL ?? Bundle.main.bundleURL).appendingPathComponent("Presence"))
    }
    func updateNSView(_ view: NativeMascotCanvas, context: Context) { view.request = request }
    static func dismantleNSView(_ view: NativeMascotCanvas, coordinator: ()) { view.detach() }
}
