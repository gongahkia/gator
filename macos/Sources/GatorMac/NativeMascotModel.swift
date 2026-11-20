import Foundation

enum NativePresenceMode: String, CaseIterable, Sendable {
    case emoji, reactions, off
    var label: String {
        switch self {
        case .emoji: "Emoji only"
        case .reactions: "Emoji + GIF reactions"
        case .off: "Off"
        }
    }
}

enum NativeMascotExpression: String, Sendable {
    case neutral, working, responding, completed, attention, failed, disconnected
    var emoji: String {
        switch self {
        case .neutral: "🐊"
        case .working: "⚙️"
        case .responding: "📝"
        case .completed: "✅"
        case .attention: "👀"
        case .failed: "⚠️"
        case .disconnected: "🔌"
        }
    }
    var reaction: String? {
        switch self {
        case .completed: "completed"
        case .attention: "attention"
        default: nil
        }
    }
}

struct NativeMascotRequest: Equatable, Sendable {
    var owner: String
    var mode: NativePresenceMode = .emoji
    var expression: NativeMascotExpression
    var revision: Int = 0
    var animate = true
    var shown = true
    var reduceMotion = false
    var visible = true
    var motionAllowed: Bool { mode == .reactions && animate && !reduceMotion }
}

enum NativeMascotPreference {
    // retain these keys so explicit off and Tuck Away choices survive migration.
    static let shown = "gator.mascot.shown"
    static let animated = "gator.mascot.animated"
    static let mode = "gator.presence.mode"
    static func presence(_ value: String?) -> NativePresenceMode { value.flatMap(NativePresenceMode.init(rawValue:)) ?? .emoji }
}

struct NativePresenceLayout {
    let compact: Bool
    let collapsed: Bool
    var slot: Double { compact ? 32 : 48 }
    var size: Double { compact ? (collapsed ? 20 : 28) : (collapsed ? 26 : 42) }
}

/// Only selected-owner public presentation facts; no protocol action capability.
struct NativeMascotSignal: Equatable, Sendable {
    var owner: String
    var connected: Bool
    var working = false
    var attention = false
    var publicTextRevision = 0
    var completedRun = ""
    var failedRun = ""
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
        else if next.attention {
            if sameOwner && old?.attention != true { revision += 1 }
            expression = .attention
        } else if sameOwner, next.working, next.publicTextRevision > 0, next.publicTextRevision != old?.publicTextRevision {
            expression = .responding; revision += 1
        } else if next.working {
            if expression != .responding { expression = .working }
        } else if !next.failedRun.isEmpty { expression = .failed }
        else if sameOwner, !next.completedRun.isEmpty, next.completedRun != old?.completedRun {
            expression = .completed; revision += 1
        } else if expression != .completed || !sameOwner || next.completedRun.isEmpty { expression = .neutral }
    }
    static func collapsed(reading: Bool, reviewing: Bool, compact: Bool, height: Double) -> Bool {
        reading || reviewing || height < (compact ? 580 : 620)
    }
}
