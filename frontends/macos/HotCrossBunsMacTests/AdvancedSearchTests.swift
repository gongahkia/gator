import Testing
import Foundation
@testable import HotCrossBunsMac

// Migrated from XCTest to Swift Testing. Parser-side field-token coverage
// collapsed to a parameterized table; matcher-side kept 1:1 because each
// case pairs a specific entity shape with a specific filter — no payoff
// from flattening.

struct AdvancedSearchParserTests {

    @Test(arguments: ["", "   "])
    func emptyIsEmpty(input: String) {
        #expect(AdvancedSearchParser.parse(input).isEmpty)
    }

    @Test func bareKeywords() {
        let q = AdvancedSearchParser.parse("overdue completed")
        #expect(q.requireOverdue)
        #expect(q.requireCompleted)
        #expect(q.freeText.isEmpty)
    }

    @Test func doneAliasForCompleted() {
        #expect(AdvancedSearchParser.parse("done").requireCompleted)
    }

    @Test func fieldTokens() {
        let q = AdvancedSearchParser.parse("title:bug tag:work list:home calendar:personal attendee:alice")
        #expect(q.titleContains == ["bug"])
        #expect(q.tagsAll == ["work"])
        #expect(q.listMatch == "home")
        #expect(q.calendarMatch == "personal")
        #expect(q.attendeeMatch == "alice")
    }

    // Three single-keyword `has:` filters. One row per keyword: cheap to add
    // a new `has:foo` case by appending to the table.
    @Test(arguments: [
        ("has:notes", \AdvancedSearchQuery.requireNotes),
        ("has:location", \AdvancedSearchQuery.requireLocation),
        ("has:due", \AdvancedSearchQuery.requireDue),
    ])
    func singleHasKeyword(input: String, flag: KeyPath<AdvancedSearchQuery, Bool>) {
        #expect(AdvancedSearchParser.parse(input)[keyPath: flag])
    }

    @Test func quotedValuesPreserveSpaces() {
        #expect(AdvancedSearchParser.parse("list:\"Work Email\"").listMatch == "Work Email")
    }

    @Test func freeTextResidue() {
        let q = AdvancedSearchParser.parse("tag:deep focus block")
        #expect(q.tagsAll == ["deep"])
        #expect(q.freeText == "focus block")
    }

    @Test func regexMode() {
        let q = AdvancedSearchParser.parse("/^standup/")
        #expect(q.regex == "^standup")
        #expect(q.freeText.isEmpty)
        #expect(q.titleContains.isEmpty)
    }

    @Test func unknownFieldFallsBackToFreeText() {
        #expect(AdvancedSearchParser.parse("foo:bar").freeText == "foo:bar")
    }

    @Test func multipleTitleTokensAccumulate() {
        #expect(AdvancedSearchParser.parse("title:foo title:bar").titleContains == ["foo", "bar"])
    }

    @Test func tagsLowercased() {
        #expect(AdvancedSearchParser.parse("tag:WORK").tagsAll == ["work"])
    }
}

struct AdvancedSearchMatcherTests {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18))!
    }

    private func day(_ offset: Int) -> Date {
        calendar.date(byAdding: .day, value: offset, to: calendar.startOfDay(for: now))!
    }

    private let taskLists = [
        TaskListMirror(id: "L1", title: "Work"),
        TaskListMirror(id: "L2", title: "Home")
    ]

    private let calendars = [
        CalendarListMirror(id: "cal1", summary: "Primary", colorHex: "#aaa", isSelected: true, accessRole: "owner"),
        CalendarListMirror(id: "cal2", summary: "Work Calendar", colorHex: "#bbb", isSelected: true, accessRole: "owner")
    ]

    private func task(id: String, title: String = "task", list: String = "L1", notes: String = "", due: Date? = nil, completed: Bool = false) -> TaskMirror {
        TaskMirror(
            id: id, taskListID: list, parentID: nil,
            title: title, notes: notes,
            status: completed ? .completed : .needsAction,
            dueDate: due, completedAt: nil,
            isDeleted: false, isHidden: false,
            position: nil, etag: nil, updatedAt: nil
        )
    }

    private func event(id: String, summary: String = "e", cal: String = "cal1", location: String = "", attendees: [String] = []) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id, calendarID: cal,
            summary: summary, details: "",
            startDate: Date(), endDate: Date().addingTimeInterval(3600),
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil, updatedAt: nil,
            location: location,
            attendeeEmails: attendees
        )
    }

    private func matches(_ entity: QuickSwitcherEntity, _ raw: String) -> Bool {
        let q = AdvancedSearchParser.parse(raw)
        return AdvancedSearchMatcher.matches(entity, query: q, calendars: calendars, taskLists: taskLists, now: now, calendar: calendar)
    }

    // MARK: - task filters

    @Test func overdueFiltersTasks() {
        #expect(matches(.task(task(id: "a", due: day(-1))), "overdue"))
        #expect(matches(.task(task(id: "b", due: day(1))), "overdue") == false)
    }

    @Test func listByTitle() {
        let t = task(id: "a", list: "L1")
        #expect(matches(.task(t), "list:Work"))
        #expect(matches(.task(t), "list:Home") == false)
    }

    @Test func tagFilter() {
        let t = task(id: "a", title: "fix #work #urgent")
        #expect(matches(.task(t), "tag:work"))
        #expect(matches(.task(t), "tag:work tag:urgent"))
        #expect(matches(.task(t), "tag:personal") == false)
    }

    @Test func hasNotesFilter() {
        #expect(matches(.task(task(id: "a", notes: "notes here")), "has:notes"))
        #expect(matches(.task(task(id: "b", notes: "")), "has:notes") == false)
    }

    @Test func kindNoteMatchesStandaloneAndDatedNoteBodies() {
        #expect(matches(.task(task(id: "standalone")), "kind:note"))
        #expect(matches(.task(task(id: "dated-note", notes: "details", due: day(1))), "kind:note"))
        #expect(matches(.task(task(id: "plain-task", due: day(1))), "kind:note") == false)
    }

    // MARK: - event filters

    @Test func calendarFilter() {
        let e = event(id: "a", cal: "cal2")
        #expect(matches(.event(e), "calendar:\"Work Calendar\""))
        #expect(matches(.event(e), "calendar:Primary") == false)
    }

    @Test func attendeeFilter() {
        let e = event(id: "a", attendees: ["alice@example.com", "bob@example.com"])
        #expect(matches(.event(e), "attendee:alice"))
        #expect(matches(.event(e), "attendee:charlie") == false)
    }

    @Test func hasLocationFilter() {
        #expect(matches(.event(event(id: "a", location: "Zoom")), "has:location"))
        #expect(matches(.event(event(id: "b", location: "")), "has:location") == false)
    }

    // Cross-kind rejection table: each row asserts that a task does NOT
    // match event-only filters, or an event does NOT match task-only filters.
    // Adding a new cross-kind rule = adding a row.
    @Test(arguments: [
        (true,  "attendee:alice"),    // true = apply to .task, assert no match
        (true,  "calendar:Work"),
        (true,  "has:location"),
        (false, "overdue"),            // false = apply to .event, assert no match
        (false, "completed"),
        (false, "list:Work"),
        (false, "tag:deep"),
    ])
    func crossKindRejection(applyToTask: Bool, query: String) {
        let entity: QuickSwitcherEntity = applyToTask
            ? .task(task(id: "a"))
            : .event(event(id: "a"))
        #expect(matches(entity, query) == false, Comment(rawValue: "\(applyToTask ? "task" : "event") should not match '\(query)'"))
    }

    // MARK: - title containment

    @Test func titleContainsIsAND() {
        let t = task(id: "a", title: "fix launch bug")
        #expect(matches(.task(t), "title:fix title:bug"))
        #expect(matches(.task(t), "title:fix title:deploy") == false)
    }

    // MARK: - regex

    @Test func regexMatch() {
        let t = task(id: "a", title: "Standup Wednesday")
        let q = AdvancedSearchParser.parse("/^Standup/")
        #expect(q.regex == "^Standup")
        #expect(AdvancedSearchMatcher.regexMatches(.task(t), regexPattern: "^Standup"))
        #expect(AdvancedSearchMatcher.regexMatches(.task(t), regexPattern: "^Launch") == false)
    }

    @Test func regexInvalidPatternReturnsFalse() {
        let t = task(id: "a", title: "x")
        // Unterminated group — should not crash, just return false.
        #expect(AdvancedSearchMatcher.regexMatches(.task(t), regexPattern: "(unterminated") == false)
    }
}
