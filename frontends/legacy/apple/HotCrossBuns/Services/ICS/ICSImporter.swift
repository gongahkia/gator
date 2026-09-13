import Foundation

// Minimal RFC5545 importer for drag-dropped .ics files. Handles the
// common shape emitted by Google Calendar and Apple Calendar exports:
//
//   BEGIN:VEVENT
//   SUMMARY:...
//   DTSTART[;TZID=...][;VALUE=DATE]:20260419T140000Z
//   DTEND[;TZID=...][;VALUE=DATE]:20260419T150000Z
//   DESCRIPTION:...
//   LOCATION:...
//   RRULE:FREQ=WEEKLY;INTERVAL=1
//   END:VEVENT
//
// Not supported in v1: VTIMEZONE blocks (we rely on TZID name resolving
// via TimeZone(identifier:)), per-instance exception overrides
// (EXDATE / RECURRENCE-ID), alarms (VALARM), attendees (ATTENDEE),
// and escape sequences inside text values. The importer hands each
// draft to AppModel.createEvent with the default reminder / guests
// settings, matching what the Add Event sheet does.
struct ICSEventDraft: Equatable, Sendable {
    var summary: String
    var description: String
    var location: String
    var startDate: Date
    var endDate: Date
    var isAllDay: Bool
    var recurrence: [String]
    var startTimeZoneID: String?
    var endTimeZoneID: String?
}

enum ICSImporter {
    static func parse(_ contents: String) -> [ICSEventDraft] {
        let unfolded = unfold(lines: contents.components(separatedBy: .newlines))
        var drafts: [ICSEventDraft] = []
        var current: ICSBuilder?
        for rawLine in unfolded {
            let line = rawLine.trimmingCharacters(in: .whitespaces)
            if line == "BEGIN:VEVENT" {
                current = ICSBuilder()
            } else if line == "END:VEVENT" {
                if let draft = current?.build() {
                    drafts.append(draft)
                }
                current = nil
            } else if current != nil, let (key, params, value) = parseProperty(line) {
                current?.absorb(key: key, params: params, value: value)
            }
        }
        return drafts
    }

    // Line-folding: a line that begins with a space or tab is a
    // continuation of the previous one. Merge before property parsing.
    private static func unfold(lines: [String]) -> [String] {
        var result: [String] = []
        for line in lines {
            if let first = line.first, (first == " " || first == "\t"), var last = result.last {
                last.append(contentsOf: line.dropFirst())
                result[result.count - 1] = last
            } else {
                result.append(line)
            }
        }
        return result
    }

    private static func parseProperty(_ line: String) -> (String, [String: String], String)? {
        guard let colon = line.firstIndex(of: ":") else { return nil }
        let lhs = String(line[..<colon])
        let value = String(line[line.index(after: colon)...])
        let parts = lhs.split(separator: ";").map(String.init)
        guard let key = parts.first?.uppercased() else { return nil }
        var params: [String: String] = [:]
        for raw in parts.dropFirst() {
            let kv = raw.split(separator: "=", maxSplits: 1).map(String.init)
            if kv.count == 2 {
                params[kv[0].uppercased()] = kv[1]
            }
        }
        return (key, params, value)
    }
}

private struct ICSBuilder {
    var summary: String = ""
    var description: String = ""
    var location: String = ""
    var startDate: Date?
    var endDate: Date?
    var isAllDay: Bool = false
    var recurrence: [String] = []
    var startTimeZoneID: String?
    var endTimeZoneID: String?

    mutating func absorb(key: String, params: [String: String], value: String) {
        switch key {
        case "SUMMARY":
            summary = unescape(value)
        case "DESCRIPTION":
            description = unescape(value)
        case "LOCATION":
            location = unescape(value)
        case "DTSTART":
            let (date, allDay, timeZoneID) = ICSDateParser.parse(value: value, params: params)
            if let date {
                startDate = date
                startTimeZoneID = timeZoneID
                if allDay { isAllDay = true }
            }
        case "DTEND":
            let (date, _, timeZoneID) = ICSDateParser.parse(value: value, params: params)
            endDate = date
            endTimeZoneID = timeZoneID
        case "RRULE":
            recurrence.append("RRULE:\(value)")
        default:
            break
        }
    }

    func build() -> ICSEventDraft? {
        guard let startDate else { return nil }
        let resolvedEnd: Date
        if let endDate {
            resolvedEnd = endDate
        } else if isAllDay {
            // RFC5545 all-day without DTEND: ends at start + 1 day (exclusive).
            resolvedEnd = Calendar.current.date(byAdding: .day, value: 1, to: startDate) ?? startDate
        } else {
            resolvedEnd = startDate.addingTimeInterval(3600)
        }
        // Our app's event model stores the *inclusive* end date for all-day
        // (after the H2 fix) and the exclusive wall-clock end for timed.
        // ICS uses exclusive end for all-day; translate to inclusive.
        let normalizedEnd: Date
        if isAllDay {
            normalizedEnd = Calendar.current.date(byAdding: .day, value: -1, to: resolvedEnd) ?? resolvedEnd
        } else {
            normalizedEnd = resolvedEnd
        }
        return ICSEventDraft(
            summary: summary.isEmpty ? "Imported event" : summary,
            description: description,
            location: location,
            startDate: startDate,
            endDate: normalizedEnd,
            isAllDay: isAllDay,
            recurrence: recurrence,
            startTimeZoneID: startTimeZoneID,
            endTimeZoneID: endTimeZoneID ?? startTimeZoneID
        )
    }

    private func unescape(_ value: String) -> String {
        value
            .replacingOccurrences(of: "\\n", with: "\n")
            .replacingOccurrences(of: "\\N", with: "\n")
            .replacingOccurrences(of: "\\,", with: ",")
            .replacingOccurrences(of: "\\;", with: ";")
            .replacingOccurrences(of: "\\\\", with: "\\")
    }
}

enum ICSDateParser {
    // Returns (parsed Date, isAllDayHint). Honours VALUE=DATE for all-day,
    // UTC suffix Z, TZID parameter, and floating local time.
    //
    // Each call builds its own DateFormatter. Previous implementation shared
    // static formatters and mutated timeZone per call — fine for serial ICS
    // import (the only caller today), but a latent data race if any future
    // caller parsed concurrently. Formatter construction is cheap relative
    // to network-bound ICS import, so per-call is the safe default.
    static func parse(value: String, params: [String: String]) -> (Date?, Bool, String?) {
        if params["VALUE"]?.uppercased() == "DATE" || (value.count == 8 && value.allSatisfy(\.isNumber)) {
            guard let date = makeDateOnlyFormatter().date(from: value) else { return (nil, true, nil) }
            return (localMidnight(matching: date), true, nil)
        }
        let tzIdentifier = params["TZID"]
        let resolvedTimeZoneID = resolvedTimeZoneID(forIdentifier: tzIdentifier, fallback: value.hasSuffix("Z"))
        let formatter = makeDateTimeFormatter(timeZone: TimezoneSupport.timeZone(for: resolvedTimeZoneID))
        let trimmed = value.hasSuffix("Z") ? String(value.dropLast()) : value
        return (formatter.date(from: trimmed), false, resolvedTimeZoneID)
    }

    private static func makeDateOnlyFormatter() -> DateFormatter {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.calendar = Calendar(identifier: .gregorian)
        f.timeZone = TimeZone(secondsFromGMT: 0)
        f.dateFormat = "yyyyMMdd"
        return f
    }

    private static func makeDateTimeFormatter(timeZone: TimeZone) -> DateFormatter {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.calendar = Calendar(identifier: .gregorian)
        f.dateFormat = "yyyyMMdd'T'HHmmss"
        f.timeZone = timeZone
        return f
    }

    private static func resolvedTimeZoneID(forIdentifier id: String?, fallback utc: Bool) -> String {
        if let valid = TimezoneSupport.validatedIdentifier(id) {
            return valid
        }
        return utc ? "UTC" : TimezoneSupport.currentIdentifier
    }

    private static func localMidnight(matching utcMidnight: Date) -> Date {
        var utcCal = Calendar(identifier: .gregorian)
        utcCal.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        let comps = utcCal.dateComponents([.year, .month, .day], from: utcMidnight)
        return Calendar.current.date(from: DateComponents(
            year: comps.year, month: comps.month, day: comps.day
        )) ?? utcMidnight
    }
}
