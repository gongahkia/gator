import Foundation

enum HCBTransitionProfileScenario: String {
    case sidebar
    case calendarModes
    case sheets
    case commandPalette
    case settingsDiagnostics
    case all

    static var current: HCBTransitionProfileScenario? {
        guard let raw = ProcessInfo.processInfo.environment["HCB_TRANSITION_PROFILE_SCENARIO"],
              raw.isEmpty == false
        else { return nil }
        return HCBTransitionProfileScenario(rawValue: raw)
    }

    static var iterations: Int {
        guard let raw = ProcessInfo.processInfo.environment["HCB_TRANSITION_PROFILE_ITERATIONS"],
              let parsed = Int(raw)
        else { return 10 }
        return max(1, parsed)
    }

    static var stepDelay: Duration {
        guard let raw = ProcessInfo.processInfo.environment["HCB_TRANSITION_PROFILE_STEP_MS"],
              let parsed = Int(raw)
        else { return .milliseconds(420) }
        return .milliseconds(max(120, parsed))
    }

    static var startDelay: Duration {
        .milliseconds(startDelayMilliseconds)
    }

    static var startDelayMilliseconds: Int {
        guard let raw = ProcessInfo.processInfo.environment["HCB_TRANSITION_PROFILE_START_DELAY_MS"],
              let parsed = Int(raw)
        else { return 1800 }
        return max(0, parsed)
    }
}

extension Notification.Name {
    static let hcbProfileNextCalendarMode = Notification.Name("hcb.profile.next.calendar.mode")
}
