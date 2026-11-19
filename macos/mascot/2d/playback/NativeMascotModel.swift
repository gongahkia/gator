import Foundation

enum NativeMascotStyle: String, CaseIterable, Sendable {
    case rendered, classic
    var label: String { self == .rendered ? "3D-rendered" : "Classic 2D" }
}
enum NativeMascotExpression: String, Sendable {
    case neutral, working, responding, completed, attention, disconnected
}
struct NativeMascotRequest: Equatable, Sendable {
    var owner: String
    var style: NativeMascotStyle
    var expression: NativeMascotExpression
    var revision: Int = 0
    var animate = true
    var shown = true
    var reduceMotion = false
    var visible = true
    var motionAllowed: Bool { animate && !reduceMotion }
}
enum NativeMascotPreference {
    static let shown = "gator.mascot.shown"
    static let animated = "gator.mascot.animated"
    static let style = "gator.mascot.style"
    static func appearance(_ value: String) -> NativeMascotStyle { NativeMascotStyle(rawValue: value) ?? .rendered }
}
struct NativeMascotClip: Sendable {
    let style: NativeMascotStyle
    let name: String
    let duration: Double
    let count: Int
    static func spec(style: NativeMascotStyle, name: String) -> Self? {
        let values: [String: (Double, Int)] = ["neutral": (0,1), "blink": (0.30,8), "working": (0.80,12), "responding": (0.55,9), "completed": (0.65,11), "attention": (0.55,9), "disconnected": (0.60,9)]
        guard let (duration, count) = values[name] else { return nil }
        return Self(style: style, name: name, duration: duration, count: count)
    }
}

/// Only selected-owner public presentation facts; no protocol action capability.
struct NativeMascotSignal: Equatable, Sendable {
    var owner: String
    var connected: Bool
    var working = false
    var attention = false
    var publicTextRevision = 0
    var completedRun = ""
}
struct NativeMascotStateMachine {
    private(set) var previous: NativeMascotSignal?
    private(set) var expression: NativeMascotExpression = .neutral
    private(set) var revision = 0
    mutating func receive(_ next: NativeMascotSignal) {
        let old = previous; previous = next
        let sameOwner = old?.owner == next.owner
        if !sameOwner { expression = .neutral; revision = 0 }
        if !next.connected { expression = .disconnected }
        else if next.attention { expression = .attention }
        else if sameOwner, next.working, next.publicTextRevision > (old?.publicTextRevision ?? 0) {
            expression = .responding; revision += 1
        } else if next.working {
            // returning from a text pulse without more text must not loop its animation
            if expression != .responding { expression = .working }
        } else if sameOwner, !next.completedRun.isEmpty, next.completedRun != old?.completedRun {
            expression = .completed; revision += 1
        } else if expression != .completed || !sameOwner { expression = .neutral }
    }
    static func collapsed(reading: Bool, reviewing: Bool, compact: Bool, height: Double) -> Bool {
        reading || reviewing || height < (compact ? 580 : 620)
    }
}
