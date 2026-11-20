import Foundation
import GatorCore

enum NativeMotionPreference {
    static let streaming = "gator.motion.streaming"
    static func enabled(in defaults: UserDefaults) -> Bool { defaults.object(forKey: streaming) as? Bool ?? true }
}
struct NativeMotionPolicy: Equatable, Sendable {
    var enabled = true
    var reduceMotion = false
    var visible = true
    var reading = false
    var reviewing = false
    var closing = false
    var connected = true
    var allows: Bool { enabled && !reduceMotion && visible && !reading && !reviewing && !closing && connected }
    static func followsLayout(following: Bool, scrolling: Bool, searching: Bool) -> Bool { following && !scrolling && !searching }
}
struct NativePublicText: Identifiable, Equatable {
    let id: String
    let text: String
    let active: Bool
    let hasExactIdentity: Bool
    static func == (lhs: Self, rhs: Self) -> Bool {
        lhs.id.utf8.elementsEqual(rhs.id.utf8) && lhs.text.utf8.elementsEqual(rhs.text.utf8) && lhs.active == rhs.active && lhs.hasExactIdentity == rhs.hasExactIdentity
    }
}
enum NativeMotionPresentation {
    static func publicText(snapshot: BridgeSnapshot?, owner: String) -> [NativePublicText] {
        guard let snapshot else { return [] }
        let trace = snapshot.value["activity"], entries = trace["entries"].array
        let running = entries.filter { $0["kind"].string == "text" && $0["state"].string == "running" }
        let ids = entries.map { $0["id"].string }
        return entries.enumerated().compactMap { index, entry in
            guard entry["kind"].string == "text" else { return nil }
            let id = entry["id"].string, start = entry["started_at"].string, run = trace["started_at"].string
            let exact = !id.isEmpty && !start.isEmpty && !run.isEmpty && ids.filter { $0 == id }.count == 1
            let identity = owner + ":" + run + ":" + (exact ? id + ":" + start : "unidentified-\(index)")
            return NativePublicText(id: identity, text: entry["text"].string,
                active: exact && running.count == 1 && entry["state"].string == "running" && snapshot.running && !snapshot.cancelling && !snapshot.hasApproval,
                hasExactIdentity: exact)
        }
    }
    static func plainProse(_ text: String) -> Bool {
        !text.contains(where: { "*_`[]<>#~\\".contains($0) })
    }
    static func hasPublicTools(_ snapshot: BridgeSnapshot?) -> Bool {
        snapshot?.value["activity"]["entries"].array.contains(where: { $0["kind"].string == "tool" }) == true
    }
    static func showsWelcome(empty: Bool, running: Bool, considering: Bool) -> Bool { empty && !running && !considering }
    static func readinessMessage(_ readiness: JSONValue) -> String {
        let issue = readiness["issue"].string
        return (issue.isEmpty ? "Model configuration needs attention." : issue) + " Open Models and Providers to inspect configuration. Local readiness does not verify a live provider request."
    }
    static func status(snapshot: BridgeSnapshot?) -> (text: String, active: Bool, icon: String)? {
        guard let snapshot else { return nil }
        if snapshot.hasApproval { return ("Waiting for your review", false, "hand.raised") }
        if snapshot.cancelling { return ("Stopping…", false, "stop.circle") }
        if snapshot.value["dialogue_busy"].bool { return ("Considering your message…", true, "clock") }
        if snapshot.running {
            let text = snapshot.value["status"].string
            return (text.isEmpty ? "Working…" : text, true, "clock")
        }
        return nil
    }
}
