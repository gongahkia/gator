import Foundation

// Variable expander for user-defined templates (§6.13). Pure, deterministic,
// context-driven — takes a template string and a `HCBTemplateContext`
// (today, clipboard, prompt answers) and returns the expanded string.
// Tests exercise every supported variable without any SwiftUI/clipboard
// dependency.
//
// Supported variables (case-sensitive `{{…}}`):
//   {{today}}                  — YYYY-MM-DD
//   {{tomorrow}}               — YYYY-MM-DD
//   {{yesterday}}              — YYYY-MM-DD
//   {{+Nd}} / {{-Nd}}          — relative YYYY-MM-DD (Nd days; also w/m/y)
//   {{nextWeekday:mon}}        — next matching weekday (sun…sat, 3-letter)
//   {{clipboard}}              — pasteboard contents (provided via context)
//   {{cursor}}                 — ⟦cursor⟧ sentinel (stripped by caller; used
//                                by editor UIs to place the insertion point)
//   {{prompt:Label}}           — answers[Label] provided by caller
//
// Unknown variables are LEFT INTACT rather than silently dropped — a typo'd
// `{{tody}}` stays visible so the user sees what failed instead of wondering
// why their date didn't render.
struct HCBTemplateContext: Sendable {
    let now: Date
    let calendar: Calendar
    let clipboard: String?
    let prompts: [String: String]

    init(
        now: Date = Date(),
        calendar: Calendar = .current,
        clipboard: String? = nil,
        prompts: [String: String] = [:]
    ) {
        self.now = now
        self.calendar = calendar
        self.clipboard = clipboard
        self.prompts = prompts
    }
}

enum HCBTemplateExpander {
    static let cursorSentinel = "\u{2045}cursor\u{2046}" // ⟦cursor⟧

    static func expand(_ template: String, context: HCBTemplateContext) -> String {
        guard template.contains("{{") else { return template }
        var out = ""
        var idx = template.startIndex
        while idx < template.endIndex {
            if let openRange = template.range(of: "{{", range: idx ..< template.endIndex) {
                out += template[idx ..< openRange.lowerBound]
                if let closeRange = template.range(of: "}}", range: openRange.upperBound ..< template.endIndex) {
                    let body = String(template[openRange.upperBound ..< closeRange.lowerBound])
                    out += resolve(body, context: context) ?? "{{\(body)}}"
                    idx = closeRange.upperBound
                } else {
                    // Unmatched opener — emit the rest verbatim.
                    out += template[openRange.lowerBound ..< template.endIndex]
                    idx = template.endIndex
                }
            } else {
                out += template[idx ..< template.endIndex]
                break
            }
        }
        return out
    }

    // Returns nil for unknown variables so the caller can leave the `{{…}}`
    // raw in the output (see above). Keeps the failure visible.
    private static func resolve(_ body: String, context: HCBTemplateContext) -> String? {
        let trimmed = body.trimmingCharacters(in: .whitespacesAndNewlines)
        let lower = trimmed.lowercased()
        if lower == "today" { return formatDate(context.now, context: context) }
        if lower == "tomorrow" {
            let d = context.calendar.date(byAdding: .day, value: 1, to: context.calendar.startOfDay(for: context.now))
            return formatDate(d ?? context.now, context: context)
        }
        if lower == "yesterday" {
            let d = context.calendar.date(byAdding: .day, value: -1, to: context.calendar.startOfDay(for: context.now))
            return formatDate(d ?? context.now, context: context)
        }
        if lower == "clipboard" { return context.clipboard ?? "" }
        if lower == "cursor" { return cursorSentinel }
        // {{+Nd}}, {{-Nw}}, etc.
        if let d = parseRelativeDate(trimmed, context: context) {
            return formatDate(d, context: context)
        }
        // {{nextWeekday:mon}}
        if lower.hasPrefix("nextweekday:") {
            let raw = String(trimmed.dropFirst("nextWeekday:".count)).lowercased()
            if let weekday = weekdayFromAbbrev(raw),
               let d = nextWeekday(from: context.now, target: weekday, calendar: context.calendar) {
                return formatDate(d, context: context)
            }
        }
        // {{prompt:Label}}
        if trimmed.hasPrefix("prompt:") {
            let label = String(trimmed.dropFirst("prompt:".count)).trimmingCharacters(in: .whitespacesAndNewlines)
            return context.prompts[label] ?? "{{prompt:\(label)}}"
        }
        return nil
    }

    private static func formatDate(_ date: Date, context: HCBTemplateContext) -> String {
        let f = DateFormatter()
        f.calendar = context.calendar
        f.timeZone = context.calendar.timeZone
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f.string(from: date)
    }

    private static func parseRelativeDate(_ token: String, context: HCBTemplateContext) -> Date? {
        guard token.range(of: "^[+-]\\d+[dwmy]$", options: .regularExpression) != nil else { return nil }
        let sign = token.first! == "+" ? 1 : -1
        let unit = token.last!
        let digits = token.dropFirst().dropLast()
        guard let n = Int(digits) else { return nil }
        let component: Calendar.Component
        switch unit {
        case "d": component = .day
        case "w": component = .weekOfYear
        case "m": component = .month
        case "y": component = .year
        default: return nil
        }
        return context.calendar.date(byAdding: component, value: sign * n, to: context.calendar.startOfDay(for: context.now))
    }

    private static func weekdayFromAbbrev(_ s: String) -> Int? {
        // Calendar weekday: 1 = Sunday ... 7 = Saturday.
        switch s {
        case "sun", "sunday": return 1
        case "mon", "monday": return 2
        case "tue", "tues", "tuesday": return 3
        case "wed", "wednesday": return 4
        case "thu", "thurs", "thursday": return 5
        case "fri", "friday": return 6
        case "sat", "saturday": return 7
        default: return nil
        }
    }

    private static func nextWeekday(from date: Date, target: Int, calendar: Calendar) -> Date? {
        let startOfDay = calendar.startOfDay(for: date)
        let current = calendar.component(.weekday, from: startOfDay)
        var delta = (target - current + 7) % 7
        if delta == 0 { delta = 7 }
        return calendar.date(byAdding: .day, value: delta, to: startOfDay)
    }
}

// Task template (§6.13). Stored locally in AppSettings; never written to
// Google. Empty variables are left unexpanded so the user can see which
// placeholder failed.
struct TaskTemplate: Codable, Hashable, Identifiable, Sendable {
    let id: UUID
    var name: String
    var title: String
    var notes: String
    var due: String // raw template string — may be "{{today}}", "{{+7d}}", literal "YYYY-MM-DD", or ""
    var listIdOrTitle: String // "" = inherit default list at instantiation
    var lastUsedAt: Date?
    var useCount: Int

    init(
        id: UUID = UUID(),
        name: String,
        title: String,
        notes: String = "",
        due: String = "",
        listIdOrTitle: String = "",
        lastUsedAt: Date? = nil,
        useCount: Int = 0
    ) {
        self.id = id
        self.name = name
        self.title = title
        self.notes = notes
        self.due = due
        self.listIdOrTitle = listIdOrTitle
        self.lastUsedAt = lastUsedAt
        self.useCount = max(0, useCount)
    }

    enum CodingKeys: String, CodingKey {
        case id, name, title, notes, due, listIdOrTitle, lastUsedAt, useCount
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(UUID.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        title = try c.decode(String.self, forKey: .title)
        notes = try c.decodeIfPresent(String.self, forKey: .notes) ?? ""
        due = try c.decodeIfPresent(String.self, forKey: .due) ?? ""
        listIdOrTitle = try c.decodeIfPresent(String.self, forKey: .listIdOrTitle) ?? ""
        lastUsedAt = try c.decodeIfPresent(Date.self, forKey: .lastUsedAt)
        useCount = max(0, try c.decodeIfPresent(Int.self, forKey: .useCount) ?? 0)
    }

    // Extracts every `{{prompt:Label}}` variable across all fields so the
    // instantiation UI can ask the user for values before expansion.
    func requiredPrompts() -> [String] {
        var seen = Set<String>()
        var out: [String] = []
        let fields = [title, notes, due, listIdOrTitle]
        let regex = try? NSRegularExpression(pattern: "\\{\\{prompt:([^}]+)\\}\\}")
        for field in fields {
            let range = NSRange(field.startIndex..., in: field)
            regex?.enumerateMatches(in: field, range: range) { match, _, _ in
                guard let match, match.numberOfRanges >= 2,
                      let labelRange = Range(match.range(at: 1), in: field) else { return }
                let label = String(field[labelRange]).trimmingCharacters(in: .whitespacesAndNewlines)
                if seen.insert(label).inserted { out.append(label) }
            }
        }
        return out
    }

    func duplicated(id: UUID = UUID()) -> TaskTemplate {
        TaskTemplate(
            id: id,
            name: hcbCopyName(name),
            title: title,
            notes: notes,
            due: due,
            listIdOrTitle: listIdOrTitle,
            lastUsedAt: nil,
            useCount: 0
        )
    }
}
