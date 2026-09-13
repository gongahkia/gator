import Foundation

enum RecurrenceFrequency: String, CaseIterable, Hashable, Sendable {
    case daily
    case weekly
    case monthly
    case yearly

    var title: String {
        switch self {
        case .daily: "Daily"
        case .weekly: "Weekly"
        case .monthly: "Monthly"
        case .yearly: "Yearly"
        }
    }

    var rfcValue: String {
        switch self {
        case .daily: "DAILY"
        case .weekly: "WEEKLY"
        case .monthly: "MONTHLY"
        case .yearly: "YEARLY"
        }
    }

    static func parse(_ value: String) -> RecurrenceFrequency? {
        RecurrenceFrequency.allCases.first { $0.rfcValue == value.uppercased() }
    }
}

// End-condition on a recurrence. Maps to RFC 5545 COUNT / UNTIL tokens
// which Google Calendar honours directly. Default is `.never` (open-ended).
enum RecurrenceEnd: Hashable, Sendable {
    case never
    case after(Int)    // COUNT=N  (minimum 1)
    case until(Date)   // UNTIL=YYYYMMDDTHHMMSSZ (UTC, per RFC 5545)
}

struct RecurrenceRule: Equatable, Hashable, Sendable {
    var frequency: RecurrenceFrequency
    var interval: Int
    var end: RecurrenceEnd

    init(frequency: RecurrenceFrequency, interval: Int = 1, end: RecurrenceEnd = .never) {
        self.frequency = frequency
        self.interval = max(1, interval)
        self.end = end
    }

    func rruleString() -> String {
        var body = "RRULE:FREQ=\(frequency.rfcValue);INTERVAL=\(max(interval, 1))"
        switch end {
        case .never:
            break
        case .after(let count):
            body += ";COUNT=\(max(1, count))"
        case .until(let date):
            body += ";UNTIL=\(Self.untilFormatter.string(from: date))"
        }
        return body
    }

    static func parse(rrule: String) -> RecurrenceRule? {
        let trimmed = rrule.hasPrefix("RRULE:") ? String(rrule.dropFirst(6)) : rrule
        var frequency: RecurrenceFrequency?
        var interval = 1
        var end: RecurrenceEnd = .never
        for component in trimmed.split(separator: ";") {
            let parts = component.split(separator: "=", maxSplits: 1)
            guard parts.count == 2 else { continue }
            let key = parts[0].uppercased()
            let value = String(parts[1])
            switch key {
            case "FREQ":
                frequency = RecurrenceFrequency.parse(value)
            case "INTERVAL":
                interval = Int(value) ?? 1
            case "COUNT":
                if let n = Int(value), n > 0 {
                    end = .after(n)
                }
            case "UNTIL":
                if let date = Self.untilFormatter.date(from: value) ?? Self.untilDateOnlyFormatter.date(from: value) {
                    end = .until(date)
                }
            default:
                continue
            }
        }
        guard let frequency else { return nil }
        return RecurrenceRule(frequency: frequency, interval: interval, end: end)
    }

    func advance(_ date: Date, calendar: Calendar = .current) -> Date? {
        switch frequency {
        case .daily:
            return calendar.date(byAdding: .day, value: interval, to: date)
        case .weekly:
            return calendar.date(byAdding: .weekOfYear, value: interval, to: date)
        case .monthly:
            return calendar.date(byAdding: .month, value: interval, to: date)
        case .yearly:
            return calendar.date(byAdding: .year, value: interval, to: date)
        }
    }

    var summary: String {
        let base = interval == 1 ? frequency.title : "Every \(interval) \(pluralUnit)"
        switch end {
        case .never:
            return base
        case .after(let n):
            return "\(base) · \(n) time\(n == 1 ? "" : "s")"
        case .until(let date):
            return "\(base) · until \(date.formatted(.dateTime.day().month(.abbreviated).year()))"
        }
    }

    private var pluralUnit: String {
        switch frequency {
        case .daily: "days"
        case .weekly: "weeks"
        case .monthly: "months"
        case .yearly: "years"
        }
    }

    // RFC 5545: UNTIL MUST be in UTC with the literal Z suffix when the
    // DTSTART is a datetime. Google accepts both forms (date-only and
    // datetime) — we always emit the datetime form; parse falls back to
    // date-only for legacy strings.
    private static let untilFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyyMMdd'T'HHmmss'Z'"
        return f
    }()

    private static let untilDateOnlyFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(identifier: "UTC")
        f.dateFormat = "yyyyMMdd"
        return f
    }()
}
