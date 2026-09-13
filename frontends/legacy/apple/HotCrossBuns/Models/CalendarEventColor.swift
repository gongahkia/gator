import SwiftUI

// Google Calendar's event `colorId` palette. Values 1–11 plus "default"
// which tells Google to use the calendar's color. Hex values mirror what
// Google Calendar's web UI surfaces.
enum CalendarEventColor: String, CaseIterable, Identifiable, Sendable {
    case defaultColor = ""
    case lavender = "1"
    case sage = "2"
    case grape = "3"
    case flamingo = "4"
    case banana = "5"
    case tangerine = "6"
    case peacock = "7"
    case graphite = "8"
    case blueberry = "9"
    case basil = "10"
    case tomato = "11"

    var id: String { rawValue }

    var title: String {
        switch self {
        case .defaultColor: "Calendar default"
        case .lavender: "Lavender"
        case .sage: "Sage"
        case .grape: "Grape"
        case .flamingo: "Flamingo"
        case .banana: "Banana"
        case .tangerine: "Tangerine"
        case .peacock: "Peacock"
        case .graphite: "Graphite"
        case .blueberry: "Blueberry"
        case .basil: "Basil"
        case .tomato: "Tomato"
        }
    }

    var hex: String? {
        switch self {
        case .defaultColor: nil
        case .lavender: "#7986cb"
        case .sage: "#33b679"
        case .grape: "#8e24aa"
        case .flamingo: "#e67c73"
        case .banana: "#f6bf26"
        case .tangerine: "#f4511e"
        case .peacock: "#039be5"
        case .graphite: "#616161"
        case .blueberry: "#3f51b5"
        case .basil: "#0b8043"
        case .tomato: "#d50000"
        }
    }

    var wireValue: String? {
        rawValue.isEmpty ? nil : rawValue
    }

    static func from(colorId: String?) -> CalendarEventColor {
        guard let colorId, colorId.isEmpty == false else { return .defaultColor }
        return CalendarEventColor(rawValue: colorId) ?? .defaultColor
    }
}
