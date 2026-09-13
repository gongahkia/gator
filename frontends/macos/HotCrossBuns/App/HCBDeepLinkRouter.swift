import Foundation

// hotcrossbuns:// URL scheme router.
//
// Route shapes (decided during design):
//  - hotcrossbuns://task/<id>                       → open task in Store
//  - hotcrossbuns://event/<id>                      → open event in Calendar
//  - hotcrossbuns://new/task?title=&notes=&due=&list=&tags=
//  - hotcrossbuns://new/event?title=&start=&end=&location=&calendar=&allday=
//  - hotcrossbuns://search?q=
//  - hotcrossbuns://open                          → activate the app
//
// Safety invariants:
//  - No URL may commit a write directly. `new/*` prefills a sheet; the user must
//    still hit Enter / Create. This file returns a typed action; the caller
//    (MacSidebarShell) presents the sheet.
//  - Unknown scheme / host / path → failure result, never crash.
//  - Google OAuth redirects use a different scheme and are NOT routed here.
//  - Param values are capped at a generous size so crafted URLs can't exhaust
//    memory via a pathological query string.

enum HCBDeepLinkAction: Equatable, Sendable {
    case openApp
    case openTask(id: String)
    case openEvent(id: String)
    case newTask(DeepLinkTaskPrefill)
    case newEvent(DeepLinkEventPrefill)
    case search(String)
}

struct DeepLinkTaskPrefill: Equatable, Sendable {
    var title: String?
    var notes: String?
    var dueDate: Date?
    var listIdOrTitle: String?
    var tags: [String]

    init(title: String? = nil, notes: String? = nil, dueDate: Date? = nil, listIdOrTitle: String? = nil, tags: [String] = []) {
        self.title = title
        self.notes = notes
        self.dueDate = dueDate
        self.listIdOrTitle = listIdOrTitle
        self.tags = tags
    }
}

struct DeepLinkEventPrefill: Equatable, Sendable {
    var title: String?
    var startDate: Date?
    var endDate: Date?
    var isAllDay: Bool
    var location: String?
    var calendarIdOrSummary: String?

    init(title: String? = nil, startDate: Date? = nil, endDate: Date? = nil, isAllDay: Bool = false,
         location: String? = nil, calendarIdOrSummary: String? = nil) {
        self.title = title
        self.startDate = startDate
        self.endDate = endDate
        self.isAllDay = isAllDay
        self.location = location
        self.calendarIdOrSummary = calendarIdOrSummary
    }
}

struct HCBDeepLinkError: Error, Equatable, Sendable {
    let message: String
}

enum HCBDeepLinkRouter {
    struct HelpRoute: Identifiable, Sendable {
        let title: String
        let example: String
        let summary: String

        var id: String { example }
    }

    static let scheme = "hotcrossbuns"
    // 2 KB per param. Task titles + notes rarely exceed this in practice;
    // anything larger is almost certainly a crafted URL.
    static let maxParamLength = 2048
    // Max id length — Google task/event ids are typically < 64 chars.
    static let maxIdLength = 256
    static let helpRoutes: [HelpRoute] = [
        HelpRoute(
            title: String(localized: "Open app"),
            example: "hotcrossbuns://open",
            summary: String(localized: "Activates Hot Cross Buns without changing the current view.")
        ),
        HelpRoute(
            title: String(localized: "Open task"),
            example: "hotcrossbuns://task/abc123",
            summary: String(localized: "Opens a specific task in the Tasks surface.")
        ),
        HelpRoute(
            title: String(localized: "Open event"),
            example: "hotcrossbuns://event/evt_456",
            summary: String(localized: "Opens a specific calendar event in Calendar.")
        ),
        HelpRoute(
            title: String(localized: "Prefill task"),
            example: "hotcrossbuns://new/task?title=Email%20rent&due=tmr&list=personal&tags=bills,urgent",
            summary: String(localized: "Stages a new-task sheet with title, due date, list hint, and tags.")
        ),
        HelpRoute(
            title: String(localized: "Prefill event"),
            example: "hotcrossbuns://new/event?title=Standup&start=2026-04-25T09:00&end=2026-04-25T09:30&location=Zoom",
            summary: String(localized: "Stages a new-event sheet with start, end, calendar, all-day, and location fields.")
        ),
        HelpRoute(
            title: String(localized: "Search"),
            example: "hotcrossbuns://search?q=kind:note%20tag:deep",
            summary: String(localized: "Opens the command palette with a staged search query.")
        )
    ]

    static func route(_ url: URL, now: Date = Date(), calendar: Calendar = .current) -> Result<HCBDeepLinkAction, HCBDeepLinkError> {
        guard url.scheme?.lowercased() == scheme else {
            return .failure(.init(message: "Not a hotcrossbuns:// URL."))
        }
        guard let host = url.host?.lowercased(), host.isEmpty == false else {
            return .failure(.init(message: "Missing route — expected hotcrossbuns://open, task, event, new, or search."))
        }

        // `/` path components from URL.pathComponents include "/" entries; strip them.
        let pathComponents = url.pathComponents.filter { $0 != "/" && $0.isEmpty == false }
        let params = parseParams(url: url)

        if let oversized = params.first(where: { $0.value.count > maxParamLength }) {
            return .failure(.init(message: "Parameter '\(oversized.key)' exceeds \(maxParamLength) chars."))
        }

        switch host {
        case "open", "home":
            return .success(.openApp)

        case "task":
            guard let id = pathComponents.first, id.isEmpty == false else {
                return .failure(.init(message: "Missing task id — expected hotcrossbuns://task/<id>."))
            }
            guard id.count <= maxIdLength else {
                return .failure(.init(message: "Task id too long."))
            }
            return .success(.openTask(id: id))

        case "event":
            guard let id = pathComponents.first, id.isEmpty == false else {
                return .failure(.init(message: "Missing event id — expected hotcrossbuns://event/<id>."))
            }
            guard id.count <= maxIdLength else {
                return .failure(.init(message: "Event id too long."))
            }
            return .success(.openEvent(id: id))

        case "new":
            guard let kind = pathComponents.first?.lowercased() else {
                return .failure(.init(message: "Missing resource after new/ — expected new/task or new/event."))
            }
            switch kind {
            case "task": return parseNewTask(params: params, now: now, calendar: calendar)
            case "event": return parseNewEvent(params: params, calendar: calendar)
            default: return .failure(.init(message: "Unknown new/\(kind) — expected new/task or new/event."))
            }

        case "search":
            guard let q = params["q"], q.isEmpty == false else {
                return .failure(.init(message: "search requires ?q=…"))
            }
            return .success(.search(q))

        default:
            return .failure(.init(message: "Unknown route '\(host)' — expected open, task, event, new, or search."))
        }
    }

    private static func parseParams(url: URL) -> [String: String] {
        guard let comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let items = comps.queryItems else { return [:] }
        var out: [String: String] = [:]
        for item in items {
            if let v = item.value {
                // URLComponents already percent-decodes. Last write wins for repeated keys.
                out[item.name.lowercased()] = v
            }
        }
        return out
    }

    private static func parseNewTask(params: [String: String], now: Date, calendar: Calendar) -> Result<HCBDeepLinkAction, HCBDeepLinkError> {
        var prefill = DeepLinkTaskPrefill(
            title: params["title"]?.nonEmptyTrimmed,
            notes: params["notes"]?.nonEmptyTrimmed,
            listIdOrTitle: params["list"]?.nonEmptyTrimmed
        )
        if let due = params["due"]?.nonEmptyTrimmed {
            switch parseDateParam(due, now: now, calendar: calendar) {
            case .success(let d): prefill.dueDate = d
            case .failure(let err): return .failure(err)
            }
        }
        if let rawTags = params["tags"]?.nonEmptyTrimmed {
            // Accept comma- or whitespace-separated. Strip leading '#'. Drop empties.
            let tokens = rawTags.split(whereSeparator: { $0 == "," || $0.isWhitespace })
            prefill.tags = tokens
                .map { String($0).trimmingCharacters(in: CharacterSet(charactersIn: "#")) }
                .filter { $0.isEmpty == false }
        }
        return .success(.newTask(prefill))
    }

    private static func parseNewEvent(params: [String: String], calendar: Calendar) -> Result<HCBDeepLinkAction, HCBDeepLinkError> {
        var prefill = DeepLinkEventPrefill(
            title: params["title"]?.nonEmptyTrimmed,
            location: params["location"]?.nonEmptyTrimmed,
            calendarIdOrSummary: params["calendar"]?.nonEmptyTrimmed
        )
        if let start = params["start"]?.nonEmptyTrimmed {
            switch parseDateTimeParam(start, calendar: calendar) {
            case .success(let (date, impliesAllDay)):
                prefill.startDate = date
                if impliesAllDay { prefill.isAllDay = true }
            case .failure(let err): return .failure(err)
            }
        }
        if let end = params["end"]?.nonEmptyTrimmed {
            switch parseDateTimeParam(end, calendar: calendar) {
            case .success(let (date, _)): prefill.endDate = date
            case .failure(let err): return .failure(err)
            }
        }
        if let allDayRaw = params["allday"]?.lowercased() {
            if ["1", "true", "yes", "on"].contains(allDayRaw) { prefill.isAllDay = true }
            if ["0", "false", "no", "off"].contains(allDayRaw) { prefill.isAllDay = false }
        }
        return .success(.newEvent(prefill))
    }

    // Date-only parser: YYYY-MM-DD, today, tomorrow, yesterday, [+-]N[dwmy].
    // Always returns startOfDay in the caller's calendar.
    static func parseDateParam(_ s: String, now: Date = Date(), calendar: Calendar = .current) -> Result<Date, HCBDeepLinkError> {
        let lower = s.lowercased()
        let startOfToday = calendar.startOfDay(for: now)
        switch lower {
        case "today": return .success(startOfToday)
        case "tomorrow":
            if let d = calendar.date(byAdding: .day, value: 1, to: startOfToday) { return .success(d) }
            return .failure(.init(message: "Could not compute 'tomorrow'."))
        case "yesterday":
            if let d = calendar.date(byAdding: .day, value: -1, to: startOfToday) { return .success(d) }
            return .failure(.init(message: "Could not compute 'yesterday'."))
        default: break
        }

        if lower.range(of: "^[+-]\\d+[dwmy]$", options: .regularExpression) != nil {
            let sign = lower.first! == "+" ? 1 : -1
            let unit = lower.last!
            let digits = lower.dropFirst().dropLast()
            guard let n = Int(digits) else {
                return .failure(.init(message: "Invalid relative date '\(s)'."))
            }
            let component: Calendar.Component
            switch unit {
            case "d": component = .day
            case "w": component = .weekOfYear
            case "m": component = .month
            case "y": component = .year
            default: return .failure(.init(message: "Invalid relative-date unit '\(unit)'."))
            }
            guard let result = calendar.date(byAdding: component, value: sign * n, to: startOfToday) else {
                return .failure(.init(message: "Could not compute relative date '\(s)'."))
            }
            return .success(result)
        }

        let absFormatter = DateFormatter()
        absFormatter.calendar = calendar
        absFormatter.timeZone = calendar.timeZone
        absFormatter.locale = Locale(identifier: "en_US_POSIX")
        absFormatter.dateFormat = "yyyy-MM-dd"
        if let d = absFormatter.date(from: s) {
            return .success(calendar.startOfDay(for: d))
        }
        return .failure(.init(message: "Unrecognized date '\(s)'. Use today / tomorrow / yesterday, +Nd/-Nd, or YYYY-MM-DD."))
    }

    // Datetime parser for event start/end. Returns (date, impliesAllDay).
    // Accepts: YYYY-MM-DD (all-day), ISO-8601, 'YYYY-MM-DDTHH:MM[:SS]' local.
    static func parseDateTimeParam(_ s: String, calendar: Calendar = .current) -> Result<(Date, Bool), HCBDeepLinkError> {
        let dateOnly = DateFormatter()
        dateOnly.calendar = calendar
        dateOnly.timeZone = calendar.timeZone
        dateOnly.locale = Locale(identifier: "en_US_POSIX")
        dateOnly.dateFormat = "yyyy-MM-dd"
        if let d = dateOnly.date(from: s) {
            return .success((calendar.startOfDay(for: d), true))
        }

        let iso = ISO8601DateFormatter()
        iso.formatOptions = [.withInternetDateTime]
        if let d = iso.date(from: s) {
            return .success((d, false))
        }
        iso.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        if let d = iso.date(from: s) {
            return .success((d, false))
        }

        let local = DateFormatter()
        local.calendar = calendar
        local.timeZone = calendar.timeZone
        local.locale = Locale(identifier: "en_US_POSIX")
        for fmt in ["yyyy-MM-dd'T'HH:mm:ss", "yyyy-MM-dd'T'HH:mm"] {
            local.dateFormat = fmt
            if let d = local.date(from: s) { return .success((d, false)) }
        }

        // Convenience: today/tomorrow/yesterday → all-day start on that date
        if ["today", "tomorrow", "yesterday"].contains(s.lowercased()) {
            switch parseDateParam(s, calendar: calendar) {
            case .success(let d): return .success((d, true))
            case .failure(let e): return .failure(e)
            }
        }

        return .failure(.init(message: "Unrecognized datetime '\(s)'. Use YYYY-MM-DD, ISO-8601 (2026-04-22T10:00:00Z), or 'YYYY-MM-DDTHH:MM' local."))
    }
}

private extension String {
    // Non-empty after trimming whitespace. Used to map empty query values to nil.
    var nonEmptyTrimmed: String? {
        let trimmed = self.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
