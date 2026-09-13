import Foundation

enum CalendarEventInstance {
    /// Matches Google Calendar instance-id suffix like `_20260420T090000Z` or `_20260420`.
    private static let suffixPattern = "_\\d{8}(T\\d{6}Z)?$"

    static func isInstanceID(_ eventID: String) -> Bool {
        guard let regex = try? NSRegularExpression(pattern: suffixPattern) else { return false }
        let range = NSRange(eventID.startIndex..., in: eventID)
        return regex.firstMatch(in: eventID, range: range) != nil
    }

    static func seriesID(from eventID: String) -> String {
        guard let regex = try? NSRegularExpression(pattern: suffixPattern) else { return eventID }
        let range = NSRange(eventID.startIndex..., in: eventID)
        return regex.stringByReplacingMatches(in: eventID, range: range, withTemplate: "")
    }

    static func isRecurring(_ event: CalendarEventMirror) -> Bool {
        event.recurrence.isEmpty == false || isInstanceID(event.id)
    }
}
