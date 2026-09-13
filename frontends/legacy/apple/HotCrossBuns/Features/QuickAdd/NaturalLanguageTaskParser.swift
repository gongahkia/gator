import Foundation

struct ParsedQuickAddTask: Equatable, Sendable {
    var title: String
    var dueDate: Date?
    var taskListHint: String?
    var matchedTokens: [MatchedToken]

    struct MatchedToken: Equatable, Sendable {
        let kind: Kind
        let display: String

        enum Kind: Equatable, Sendable {
            case dueDate
            case list
        }
    }

    var hasParsedMetadata: Bool { matchedTokens.isEmpty == false }
}

struct NaturalLanguageTaskParser: Sendable {
    struct HelpEntry: Identifiable, Sendable {
        let title: String
        let examples: [String]
        let summary: String

        var id: String { title }
    }

    static let helpEntries: [HelpEntry] = [
        HelpEntry(
            title: String(localized: "Relative dates"),
            examples: ["tdy", "tnt", "tmr", "next monday", "in 3 days", "eom"],
            summary: String(localized: "Accepts today / tonight / tomorrow aliases, next-weekday phrases, relative offsets, and end-of-period shortcuts.")
        ),
        HelpEntry(
            title: String(localized: "Absolute dates"),
            examples: ["2026-04-25", "4/25", "Apr 25", "25 Apr"],
            summary: String(localized: "Parses ISO, numeric month-day, and month-name dates.")
        ),
        HelpEntry(
            title: String(localized: "List hints"),
            examples: ["#personal", "#deep-work"],
            summary: String(localized: "Maps a task to a task list hint without opening the full editor.")
        )
    ]

    let calendar: Calendar
    let now: Date

    init(now: Date = Date(), calendar: Calendar = .current) {
        self.calendar = calendar
        self.now = now
    }

    func parse(_ input: String) -> ParsedQuickAddTask {
        let trimmed = input.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            return ParsedQuickAddTask(title: "", dueDate: nil, taskListHint: nil, matchedTokens: [])
        }

        var working = " \(trimmed) "
        var tokens: [ParsedQuickAddTask.MatchedToken] = []

        var taskListHint: String?
        if let (hint, replacement) = extractListHint(in: working) {
            taskListHint = hint
            working = replacement
            tokens.append(.init(kind: .list, display: "#\(hint)"))
        }

        var dueDate: Date?
        if let (date, replacement, display) = extractDueDate(in: working) {
            dueDate = date
            working = replacement
            tokens.append(.init(kind: .dueDate, display: display))
        }

        let cleanedTitle = working
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)

        return ParsedQuickAddTask(
            title: cleanedTitle,
            dueDate: dueDate,
            taskListHint: taskListHint,
            matchedTokens: tokens
        )
    }

    private func extractListHint(in text: String) -> (String, String)? {
        guard let match = TagExtractor.firstTag(in: text) else {
            return nil
        }
        var replacement = text
        replacement.removeSubrange(match.range)
        return (match.tag, replacement)
    }

    private func extractDueDate(in text: String) -> (Date, String, String)? {
        let lower = text.lowercased()

        // Ordered by specificity — ISO YYYY-MM-DD comes before numeric
        // M-D so "2026-04-25" doesn't get picked up as year=20, month=26.
        let patterns: [(pattern: String, resolver: (NSTextCheckingResult, String) -> (Date, String)?)] = [
            // End-of-X shortcuts.
            ("\\beod\\b", { _, _ in self.resolveRelativeDay(offsetDays: 0, display: "End of day") }),
            ("\\beow\\b", { _, _ in self.resolveEndOfWeek() }),
            ("\\beom\\b", { _, _ in self.resolveEndOfMonth() }),
            ("\\beoy\\b", { _, _ in self.resolveEndOfYear() }),
            // today / tonight + abbreviations.
            ("\\b(today|tdy|tnt|tonight|td)\\b", { _, _ in
                self.resolveRelativeDay(offsetDays: 0, display: "Today")
            }),
            // tomorrow + many spellings.
            ("\\b(tomorrow|tmrw|tmr|tmw|tomo|2mrw|2moro|2mro)\\b", { _, _ in
                self.resolveRelativeDay(offsetDays: 1, display: "Tomorrow")
            }),
            // yesterday — kept for completeness (logging past-dated tasks).
            ("\\b(yesterday|ytd|yday)\\b", { _, _ in
                self.resolveRelativeDay(offsetDays: -1, display: "Yesterday")
            }),
            // Day after tomorrow.
            ("\\b(day\\s+after\\s+tomorrow|dat)\\b", { _, _ in
                self.resolveRelativeDay(offsetDays: 2, display: "Day after tomorrow")
            }),
            // "in N hours|days|weeks|months".
            ("\\bin\\s+(\\d{1,3})\\s+(hour|hours|hr|hrs|day|days|d|week|weeks|wk|wks|month|months|mo)\\b", { match, text in
                guard let nRange = Range(match.range(at: 1), in: text),
                      let unitRange = Range(match.range(at: 2), in: text),
                      let n = Int(text[nRange]) else { return nil }
                let unit = text[unitRange].lowercased()
                let (days, display): (Int, String) = {
                    if ["hour", "hours", "hr", "hrs"].contains(unit) {
                        // Tasks are date-only — coerce hours to "today".
                        return (0, "Today")
                    }
                    if ["week", "weeks", "wk", "wks"].contains(unit) {
                        return (n * 7, "In \(n) week\(n == 1 ? "" : "s")")
                    }
                    if ["month", "months", "mo"].contains(unit) {
                        return (n * 30, "In \(n) month\(n == 1 ? "" : "s")")
                    }
                    return (n, "In \(n) day\(n == 1 ? "" : "s")")
                }()
                return self.resolveRelativeDay(offsetDays: days, display: display)
            }),
            // next week / month / year + abbreviations.
            ("\\b(next\\s+week|nw)\\b", { _, _ in
                self.resolveRelativeDay(offsetDays: 7, display: "Next week")
            }),
            ("\\b(next\\s+month|nm)\\b", { _, _ in self.resolveNextMonth() }),
            ("\\b(next\\s+year|ny)\\b", { _, _ in self.resolveNextYear() }),
            // this weekend — upcoming Saturday.
            ("\\b(this\\s+)?weekend\\b", { _, _ in
                self.resolveWeekday(weekdayText: "sat", forceNext: false)
            }),
            // Weekdays full + 3-letter + 2-letter.
            ("\\b(next\\s+)?(monday|tuesday|wednesday|thursday|friday|saturday|sunday|mon|tue|tues|wed|thu|thur|thurs|fri|sat|sun|mo|tu|we|th|fr|sa|su)\\b", { match, text in
                guard let weekdayRange = Range(match.range(at: 2), in: text) else { return nil }
                let weekdayText = String(text[weekdayRange]).lowercased()
                let forceNext = match.range(at: 1).location != NSNotFound
                return self.resolveWeekday(weekdayText: weekdayText, forceNext: forceNext)
            }),
            // Text month + day.
            ("\\b(jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec|january|february|march|april|may|june|july|august|september|october|november|december)\\s+(\\d{1,2})(?:st|nd|rd|th)?\\b", { match, text in
                guard let monthRange = Range(match.range(at: 1), in: text),
                      let dayRange = Range(match.range(at: 2), in: text),
                      let day = Int(text[dayRange]) else { return nil }
                return self.resolveMonthDay(monthText: String(text[monthRange]), day: day)
            }),
            // Day + text month.
            ("\\b(\\d{1,2})(?:st|nd|rd|th)?\\s+(jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec|january|february|march|april|may|june|july|august|september|october|november|december)\\b", { match, text in
                guard let dayRange = Range(match.range(at: 1), in: text),
                      let monthRange = Range(match.range(at: 2), in: text),
                      let day = Int(text[dayRange]) else { return nil }
                return self.resolveMonthDay(monthText: String(text[monthRange]), day: day)
            }),
            // ISO 2026-04-25.
            ("\\b(\\d{4})-(\\d{1,2})-(\\d{1,2})\\b", { match, text in
                guard let yRange = Range(match.range(at: 1), in: text),
                      let mRange = Range(match.range(at: 2), in: text),
                      let dRange = Range(match.range(at: 3), in: text),
                      let year = Int(text[yRange]),
                      let month = Int(text[mRange]),
                      let day = Int(text[dRange]) else { return nil }
                return self.resolveISODate(year: year, month: month, day: day)
            }),
            // Numeric M/D, M-D, M.D.
            ("\\b(\\d{1,2})[/.\\-](\\d{1,2})\\b", { match, text in
                guard let mRange = Range(match.range(at: 1), in: text),
                      let dRange = Range(match.range(at: 2), in: text),
                      let monthNumber = Int(text[mRange]),
                      let dayNumber = Int(text[dRange]) else { return nil }
                return self.resolveMonthDayNumeric(month: monthNumber, day: dayNumber)
            })
        ]

        for (pattern, resolver) in patterns {
            guard let regex = try? NSRegularExpression(pattern: pattern) else { continue }
            let range = NSRange(lower.startIndex..., in: lower)
            for match in regex.matches(in: lower, range: range) {
                guard let fullRange = Range(match.range, in: text),
                      isHashPrefixed(fullRange, in: text) == false,
                      let (date, display) = resolver(match, lower) else {
                    continue
                }
                var replacement = text
                replacement.removeSubrange(fullRange)
                return (date, replacement, display)
            }
        }
        return nil
    }

    private func isHashPrefixed(_ range: Range<String.Index>, in text: String) -> Bool {
        guard range.lowerBound > text.startIndex else { return false }
        return text[text.index(before: range.lowerBound)] == "#"
    }

    private func resolveEndOfWeek() -> (Date, String)? {
        let today = calendar.startOfDay(for: now)
        let weekday = calendar.component(.weekday, from: today)
        let rawDelta = 7 - weekday // Sunday=1..Saturday=7 → Saturday
        let delta = rawDelta == 0 ? 7 : rawDelta
        return calendar.date(byAdding: .day, value: delta, to: today).map { ($0, "End of week") }
    }

    private func resolveEndOfMonth() -> (Date, String)? {
        let today = calendar.startOfDay(for: now)
        var comps = calendar.dateComponents([.year, .month], from: today)
        comps.month = (comps.month ?? 1) + 1
        comps.day = 0 // last day of previous month = last day of current month
        return calendar.date(from: comps).map { ($0, "End of month") }
    }

    private func resolveEndOfYear() -> (Date, String)? {
        var comps = calendar.dateComponents([.year], from: calendar.startOfDay(for: now))
        comps.month = 12
        comps.day = 31
        return calendar.date(from: comps).map { ($0, "End of year") }
    }

    private func resolveNextMonth() -> (Date, String)? {
        let today = calendar.startOfDay(for: now)
        return calendar.date(byAdding: .month, value: 1, to: today).map { ($0, "Next month") }
    }

    private func resolveNextYear() -> (Date, String)? {
        let today = calendar.startOfDay(for: now)
        return calendar.date(byAdding: .year, value: 1, to: today).map { ($0, "Next year") }
    }

    private func resolveISODate(year: Int, month: Int, day: Int) -> (Date, String)? {
        guard (1...12).contains(month), (1...31).contains(day), year >= 1970 else { return nil }
        var comps = DateComponents(year: year, month: month, day: day)
        comps.hour = 0; comps.minute = 0; comps.second = 0
        guard let date = calendar.date(from: comps) else { return nil }
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.dateFormat = "MMM d, yyyy"
        return (date, formatter.string(from: date))
    }

    private func resolveRelativeDay(offsetDays: Int, display: String) -> (Date, String)? {
        let today = calendar.startOfDay(for: now)
        guard let date = calendar.date(byAdding: .day, value: offsetDays, to: today) else { return nil }
        return (date, display)
    }

    private func resolveWeekday(weekdayText: String, forceNext: Bool) -> (Date, String)? {
        let weekdayIndex: Int = {
            switch weekdayText {
            case "sun", "sunday", "su": return 1
            case "mon", "monday", "mo": return 2
            case "tue", "tues", "tuesday", "tu": return 3
            case "wed", "wednesday", "we": return 4
            case "thu", "thur", "thurs", "thursday", "th": return 5
            case "fri", "friday", "fr": return 6
            case "sat", "saturday", "sa": return 7
            default: return 0
            }
        }()
        guard weekdayIndex > 0 else { return nil }

        let today = calendar.startOfDay(for: now)
        let todayWeekday = calendar.component(.weekday, from: today)
        var delta = weekdayIndex - todayWeekday
        if delta <= 0 { delta += 7 }
        // "next <weekday>" means the instance in the following week —
        // bump by 7 when today-to-target is still inside the current week.
        if forceNext, delta < 7 { delta += 7 }
        guard let target = calendar.date(byAdding: .day, value: delta, to: today) else { return nil }
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.dateFormat = "EEEE"
        let label = (forceNext ? "Next " : "") + formatter.string(from: target)
        return (target, label.capitalized)
    }

    private func resolveMonthDay(monthText: String, day: Int) -> (Date, String)? {
        let monthNumber = monthFrom(monthText: monthText)
        guard monthNumber > 0 else { return nil }
        return resolveMonthDayNumeric(month: monthNumber, day: day)
    }

    private func resolveMonthDayNumeric(month: Int, day: Int) -> (Date, String)? {
        guard (1...12).contains(month), (1...31).contains(day) else { return nil }
        let year = calendar.component(.year, from: now)
        var components = DateComponents(year: year, month: month, day: day)
        guard let candidate = calendar.date(from: components) else { return nil }
        let today = calendar.startOfDay(for: now)
        let finalDate: Date
        if candidate < today {
            components.year = year + 1
            guard let bumped = calendar.date(from: components) else { return nil }
            finalDate = bumped
        } else {
            finalDate = candidate
        }
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.dateFormat = "MMM d"
        return (finalDate, formatter.string(from: finalDate))
    }

    private func monthFrom(monthText: String) -> Int {
        switch monthText.lowercased() {
        case "jan", "january": 1
        case "feb", "february": 2
        case "mar", "march": 3
        case "apr", "april": 4
        case "may": 5
        case "jun", "june": 6
        case "jul", "july": 7
        case "aug", "august": 8
        case "sep", "sept", "september": 9
        case "oct", "october": 10
        case "nov", "november": 11
        case "dec", "december": 12
        default: 0
        }
    }
}
