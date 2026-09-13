import Foundation

// Lightweight DSL for the quick switcher's entity search. Complements the
// richer QueryDSL used by custom sidebar filters (which is task-only and
// boolean-tree structured). This one:
//  - Works across tasks, events, lists, calendars, custom filters.
//  - Accepts field prefixes (title:, tag:, list:, calendar:, attendee:,
//    kind:, has:notes|location|due) and bare keywords (overdue, completed).
//    `kind:` accepts task | note | event | list | calendar | filter and
//    restricts the result to that entity kind. Notes are standalone undated
//    tasks plus tasks with note/details text.
//  - Supports regex mode via leading + trailing slashes: /pattern/
//  - Falls back to free-text fuzzy matching on any residue.
//
// Not supported here (deferred): duration/date comparators. Users who need
// those use the custom-filter DSL which already has them.
struct AdvancedSearchQuery: Equatable {
    var regex: String? // pattern between /…/ if regex mode; nil otherwise
    var titleContains: [String] // substring filters from title:X tokens
    var tagsAll: [String] // tag:X tokens — every tag must match
    var listMatch: String? // list:X — matches a task's list (id or title)
    var calendarMatch: String? // calendar:X — matches an event's calendar
    var attendeeMatch: String? // attendee:X — event attendee email/name
    var kind: EntityKind? // kind:task|note|event|list|calendar|filter
    var requireNotes: Bool
    var requireLocation: Bool
    var requireDue: Bool
    var requireCompleted: Bool
    var requireOverdue: Bool
    var freeText: String // residue → handed to FuzzySearcher for ranking

    static let empty = AdvancedSearchQuery(
        regex: nil,
        titleContains: [],
        tagsAll: [],
        listMatch: nil,
        calendarMatch: nil,
        attendeeMatch: nil,
        kind: nil,
        requireNotes: false,
        requireLocation: false,
        requireDue: false,
        requireCompleted: false,
        requireOverdue: false,
        freeText: ""
    )

    var isEmpty: Bool {
        self == .empty
    }
}

// Discriminator for `kind:` in the DSL. Task and note both resolve to
// the TaskMirror case. Notes are a client-side lens over standalone undated
// tasks and tasks with note/details text.
enum EntityKind: String, Equatable, Sendable {
    case task
    case note
    case event
    case list
    case calendar
    case filter
}

enum AdvancedSearchParser {
    // Parses a raw search string into a structured query. Never throws —
    // malformed regex is silently ignored (falls back to free-text), because
    // the switcher shouldn't blow up on a typo'd pattern.
    static func parse(_ raw: String) -> AdvancedSearchQuery {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return .empty }

        // Regex mode: /pattern/  — takes over the whole query string.
        if trimmed.count >= 2, trimmed.hasPrefix("/"), trimmed.hasSuffix("/") {
            let body = String(trimmed.dropFirst().dropLast())
            if body.isEmpty == false {
                return AdvancedSearchQuery(
                    regex: body,
                    titleContains: [],
                    tagsAll: [],
                    listMatch: nil,
                    calendarMatch: nil,
                    attendeeMatch: nil,
                    kind: nil,
                    requireNotes: false,
                    requireLocation: false,
                    requireDue: false,
                    requireCompleted: false,
                    requireOverdue: false,
                    freeText: ""
                )
            }
        }

        var q = AdvancedSearchQuery.empty
        var freeParts: [String] = []
        for token in tokenize(trimmed) {
            if apply(token: token, to: &q) == false {
                freeParts.append(token)
            }
        }
        q.freeText = freeParts.joined(separator: " ")
        return q
    }

    // Tokenizer: splits on whitespace, but preserves quoted spans so
    // `list:"My Work"` stays one token.
    static func tokenize(_ raw: String) -> [String] {
        var tokens: [String] = []
        var current = ""
        var inQuote = false
        for char in raw {
            if char == "\"" {
                inQuote.toggle()
                current.append(char)
                continue
            }
            if char.isWhitespace && inQuote == false {
                if current.isEmpty == false {
                    tokens.append(current)
                    current.removeAll()
                }
                continue
            }
            current.append(char)
        }
        if current.isEmpty == false { tokens.append(current) }
        return tokens
    }

    // Applies a single token to the query. Returns true if the token was a
    // recognised field/keyword; false means "treat as free text".
    private static func apply(token: String, to query: inout AdvancedSearchQuery) -> Bool {
        let lower = token.lowercased()
        // Bare keywords
        switch lower {
        case "overdue": query.requireOverdue = true; return true
        case "completed", "done": query.requireCompleted = true; return true
        default: break
        }

        guard let colon = token.firstIndex(of: ":") else { return false }
        let field = token[..<colon].lowercased()
        let rawValue = String(token[token.index(after: colon)...])
        let value = unquote(rawValue)
        guard value.isEmpty == false else { return false }

        switch field {
        case "title":
            query.titleContains.append(value)
            return true
        case "tag":
            query.tagsAll.append(value.lowercased())
            return true
        case "list":
            query.listMatch = value
            return true
        case "calendar":
            query.calendarMatch = value
            return true
        case "attendee":
            query.attendeeMatch = value.lowercased()
            return true
        case "kind":
            if let k = EntityKind(rawValue: value.lowercased()) {
                query.kind = k
                return true
            }
            return false
        case "has":
            switch value.lowercased() {
            case "notes": query.requireNotes = true; return true
            case "location": query.requireLocation = true; return true
            case "due": query.requireDue = true; return true
            default: return false
            }
        default:
            return false
        }
    }

    private static func unquote(_ s: String) -> String {
        var t = s
        if t.hasPrefix("\"") && t.hasSuffix("\"") && t.count >= 2 {
            t.removeFirst()
            t.removeLast()
        }
        return t
    }
}
