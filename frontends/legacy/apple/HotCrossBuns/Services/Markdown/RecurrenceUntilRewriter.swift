import Foundation

// Rewrites an RRULE string with a new UNTIL clause, used to truncate a
// recurring series at a cutoff date ("this and following" delete).
//
// Google Calendar accepts UNTIL in RFC5545 form: yyyyMMdd (date-only,
// for all-day masters) or yyyyMMddTHHmmssZ (UTC, for timed masters).
// A series with COUNT cannot also have UNTIL, so COUNT is dropped when
// UNTIL is applied.
enum RecurrenceUntilRewriter {
    static func untilString(fromCutoff cutoff: Date, isAllDay: Bool) -> String {
        var utcCalendar = Calendar(identifier: .gregorian)
        utcCalendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        let comps = utcCalendar.dateComponents([.year, .month, .day, .hour, .minute, .second], from: cutoff)
        let y = comps.year ?? 1970
        let m = comps.month ?? 1
        let d = comps.day ?? 1
        if isAllDay {
            return String(format: "%04d%02d%02d", y, m, d)
        }
        let hh = comps.hour ?? 0
        let mm = comps.minute ?? 0
        let ss = comps.second ?? 0
        return String(format: "%04d%02d%02dT%02d%02d%02dZ", y, m, d, hh, mm, ss)
    }

    static func rewrite(rrule: String, until: String) -> String {
        let head: String
        let body: String
        if rrule.hasPrefix("RRULE:") {
            head = "RRULE:"
            body = String(rrule.dropFirst(6))
        } else {
            head = ""
            body = rrule
        }
        let components = body.split(separator: ";").map(String.init)
        var result: [String] = []
        var appliedUntil = false
        for comp in components {
            let upper = comp.uppercased()
            if upper.hasPrefix("UNTIL=") {
                result.append("UNTIL=\(until)")
                appliedUntil = true
            } else if upper.hasPrefix("COUNT=") {
                // COUNT and UNTIL are mutually exclusive; drop COUNT when
                // applying an UNTIL cutoff.
                continue
            } else {
                result.append(comp)
            }
        }
        if appliedUntil == false {
            result.append("UNTIL=\(until)")
        }
        return head + result.joined(separator: ";")
    }
}
