import Foundation

// §6.13b — Event template (local-only, never written to Google). Every
// string-typed field is a template expression that passes through
// HCBTemplateExpander at instantiation. Plain literals (no `{{…}}`) pass
// through unchanged, so templates authored before the variable extension
// keep working.
//
// dateAnchor / timeAnchor are the separated pieces of the event start:
//   dateAnchor = "{{today}}" → 2026-04-20 (template-expanded YYYY-MM-DD).
//   timeAnchor = "09:30" → literal HH:mm in 24h.
// The two together resolve to a start Date in the current calendar's
// timezone. Either can be empty; a non-empty dateAnchor that fails to
// parse is flagged at instantiation rather than silently falling back.
struct EventTemplate: Identifiable, Hashable, Codable, Sendable {
    var id: UUID
    var name: String
    var summary: String             // template
    var details: String             // template
    var location: String            // template
    var dateAnchor: String          // template → YYYY-MM-DD; "" = today
    var timeAnchor: String          // "HH:mm" 24h; "" = round-up now to next 15m
    var durationMinutes: Int
    var isAllDay: Bool
    var reminderMinutes: Int?
    var colorId: String?
    var attendees: [String]         // each entry is a template
    var addGoogleMeet: Bool
    var recurrenceRule: String      // RRULE body without "RRULE:" prefix; "" = none
    var calendarIdOrTitle: String   // "" = first writable calendar
    var lastUsedAt: Date?
    var useCount: Int

    init(
        id: UUID = UUID(),
        name: String,
        summary: String,
        details: String = "",
        location: String = "",
        dateAnchor: String = "",
        timeAnchor: String = "",
        durationMinutes: Int = 60,
        isAllDay: Bool = false,
        reminderMinutes: Int? = nil,
        colorId: String? = nil,
        attendees: [String] = [],
        addGoogleMeet: Bool = false,
        recurrenceRule: String = "",
        calendarIdOrTitle: String = "",
        lastUsedAt: Date? = nil,
        useCount: Int = 0
    ) {
        self.id = id
        self.name = name
        self.summary = summary
        self.details = details
        self.location = location
        self.dateAnchor = dateAnchor
        self.timeAnchor = timeAnchor
        self.durationMinutes = durationMinutes
        self.isAllDay = isAllDay
        self.reminderMinutes = reminderMinutes
        self.colorId = colorId
        self.attendees = attendees
        self.addGoogleMeet = addGoogleMeet
        self.recurrenceRule = recurrenceRule
        self.calendarIdOrTitle = calendarIdOrTitle
        self.lastUsedAt = lastUsedAt
        self.useCount = max(0, useCount)
    }

    enum CodingKeys: String, CodingKey {
        case id, name, summary, details, location, dateAnchor, timeAnchor
        case durationMinutes, isAllDay, reminderMinutes, colorId
        case attendees, addGoogleMeet, recurrenceRule, calendarIdOrTitle
        case lastUsedAt, useCount
    }

    // decodeIfPresent on new fields keeps pre-§6.13b caches loadable.
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(UUID.self, forKey: .id)
        name = try c.decode(String.self, forKey: .name)
        summary = try c.decode(String.self, forKey: .summary)
        details = try c.decodeIfPresent(String.self, forKey: .details) ?? ""
        location = try c.decodeIfPresent(String.self, forKey: .location) ?? ""
        dateAnchor = try c.decodeIfPresent(String.self, forKey: .dateAnchor) ?? ""
        timeAnchor = try c.decodeIfPresent(String.self, forKey: .timeAnchor) ?? ""
        durationMinutes = try c.decodeIfPresent(Int.self, forKey: .durationMinutes) ?? 60
        isAllDay = try c.decodeIfPresent(Bool.self, forKey: .isAllDay) ?? false
        reminderMinutes = try c.decodeIfPresent(Int.self, forKey: .reminderMinutes)
        colorId = try c.decodeIfPresent(String.self, forKey: .colorId)
        attendees = try c.decodeIfPresent([String].self, forKey: .attendees) ?? []
        addGoogleMeet = try c.decodeIfPresent(Bool.self, forKey: .addGoogleMeet) ?? false
        recurrenceRule = try c.decodeIfPresent(String.self, forKey: .recurrenceRule) ?? ""
        calendarIdOrTitle = try c.decodeIfPresent(String.self, forKey: .calendarIdOrTitle) ?? ""
        lastUsedAt = try c.decodeIfPresent(Date.self, forKey: .lastUsedAt)
        useCount = max(0, try c.decodeIfPresent(Int.self, forKey: .useCount) ?? 0)
    }

    // Extracts every {{prompt:Label}} placeholder across every templated
    // field so the instantiation UI can collect answers before expansion.
    // Mirrors TaskTemplate.requiredPrompts.
    func requiredPrompts() -> [String] {
        var seen = Set<String>()
        var out: [String] = []
        var fields = [summary, details, location, dateAnchor, calendarIdOrTitle]
        fields.append(contentsOf: attendees)
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

    func duplicated(id: UUID = UUID()) -> EventTemplate {
        EventTemplate(
            id: id,
            name: hcbCopyName(name),
            summary: summary,
            details: details,
            location: location,
            dateAnchor: dateAnchor,
            timeAnchor: timeAnchor,
            durationMinutes: durationMinutes,
            isAllDay: isAllDay,
            reminderMinutes: reminderMinutes,
            colorId: colorId,
            attendees: attendees,
            addGoogleMeet: addGoogleMeet,
            recurrenceRule: recurrenceRule,
            calendarIdOrTitle: calendarIdOrTitle,
            lastUsedAt: nil,
            useCount: 0
        )
    }
}
