import Foundation
import SwiftUI

struct CalendarViewFilterState: Codable, Equatable, Sendable {
    static let storageKey = "calendar.viewFilters.state.v1"

    var visibleCalendarIDs: Set<CalendarListMirror.ID>?
    var visibleColorIDs: Set<String>?
    var visibleTagNames: Set<String>?

    static let allVisible = CalendarViewFilterState(
        visibleCalendarIDs: nil,
        visibleColorIDs: nil,
        visibleTagNames: nil
    )

    static func decoded(from rawValue: String) -> CalendarViewFilterState {
        guard rawValue.isEmpty == false,
              let data = rawValue.data(using: .utf8),
              let decoded = try? JSONDecoder().decode(CalendarViewFilterState.self, from: data)
        else {
            return .allVisible
        }
        return decoded.normalized()
    }

    func encodedString() -> String {
        let normalized = normalized()
        guard normalized != .allVisible,
              let data = try? JSONEncoder().encode(normalized),
              let encoded = String(data: data, encoding: .utf8)
        else {
            return ""
        }
        return encoded
    }

    func normalized() -> CalendarViewFilterState {
        CalendarViewFilterState(
            visibleCalendarIDs: visibleCalendarIDs,
            visibleColorIDs: visibleColorIDs.map { Set($0.map(CalendarEventViewFilter.normalizedColorID)) },
            visibleTagNames: visibleTagNames.map { Set($0.map(CalendarEventViewFilter.normalizedTagName).filter { $0.isEmpty == false }) }
        )
    }

    var hasActiveFilters: Bool {
        visibleCalendarIDs != nil || visibleColorIDs != nil || visibleTagNames != nil
    }
}

struct CalendarEventViewFilter: Equatable, Sendable {
    var visibleCalendarIDs: Set<CalendarListMirror.ID>?
    var visibleColorIDs: Set<String>?
    var visibleTagNames: Set<String>?
    var colorTagIndex: [String: String]

    init(
        state: CalendarViewFilterState = .allVisible,
        colorTagBindings: [String: String] = [:]
    ) {
        let normalizedState = state.normalized()
        self.visibleCalendarIDs = normalizedState.visibleCalendarIDs
        self.visibleColorIDs = normalizedState.visibleColorIDs.map { Set($0.map(Self.normalizedColorID)) }
        self.visibleTagNames = normalizedState.visibleTagNames
        self.colorTagIndex = Self.colorTagIndex(from: colorTagBindings)
    }

    var cacheKey: String {
        [
            "cal=\(Self.cachePart(visibleCalendarIDs))",
            "color=\(Self.cachePart(visibleColorIDs))",
            "tag=\(Self.cachePart(visibleTagNames))",
            "bindings=\(colorTagIndex.map { "\($0.key):\($0.value)" }.sorted().joined(separator: ","))"
        ].joined(separator: "|")
    }

    func allows(_ event: CalendarEventMirror) -> Bool {
        if let visibleCalendarIDs, visibleCalendarIDs.contains(event.calendarID) == false {
            return false
        }

        let eventColorID = Self.eventColorID(for: event)
        if let visibleColorIDs, visibleColorIDs.contains(eventColorID) == false {
            return false
        }

        if let visibleTagNames {
            let eventTagNames = Self.tagNames(in: event, eventColorID: eventColorID, colorTagIndex: colorTagIndex)
            guard eventTagNames.isEmpty == false else { return true }
            return eventTagNames.isDisjoint(with: visibleTagNames) == false
        }

        return true
    }

    static func eventColorID(for event: CalendarEventMirror) -> String {
        CalendarEventColor.from(colorId: event.colorId).rawValue
    }

    static func literalTagNames(in event: CalendarEventMirror) -> Set<String> {
        let fields = [event.summary, event.details, event.location]
        return Set(fields.flatMap(TagExtractor.tags).map(normalizedTagName).filter { $0.isEmpty == false })
    }

    static func tagNames(
        in event: CalendarEventMirror,
        eventColorID: String? = nil,
        colorTagIndex: [String: String]
    ) -> Set<String> {
        let resolvedColorID = eventColorID.map(normalizedColorID) ?? Self.eventColorID(for: event)
        let colorTags = colorTagIndex.compactMap { tagName, colorID in
            normalizedColorID(colorID) == resolvedColorID ? tagName : nil
        }
        return literalTagNames(in: event).union(colorTags)
    }

    static func colorTagIndex(from bindings: [String: String]) -> [String: String] {
        bindings.reduce(into: [String: String]()) { result, entry in
            let colorID = normalizedColorID(entry.key)
            let tagName = normalizedTagName(entry.value)
            guard colorID.isEmpty == false, tagName.isEmpty == false else { return }
            result[tagName] = colorID
        }
    }

    static func normalizedTagName(_ value: String) -> String {
        var trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        while trimmed.first == "#" {
            trimmed.removeFirst()
        }
        return trimmed
            .folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
            .lowercased()
    }

    static func normalizedColorID(_ value: String) -> String {
        CalendarEventColor(rawValue: value)?.rawValue ?? CalendarEventColor.defaultColor.rawValue
    }

    private static func cachePart(_ values: Set<String>?) -> String {
        guard let values else { return "*" }
        return values.sorted().joined(separator: ",")
    }
}

private struct CalendarEventViewFilterKey: EnvironmentKey {
    static let defaultValue = CalendarEventViewFilter()
}

extension EnvironmentValues {
    var calendarEventViewFilter: CalendarEventViewFilter {
        get { self[CalendarEventViewFilterKey.self] }
        set { self[CalendarEventViewFilterKey.self] = newValue }
    }
}
