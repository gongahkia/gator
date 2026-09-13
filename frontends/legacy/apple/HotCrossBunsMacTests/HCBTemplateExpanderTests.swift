import XCTest
@testable import HotCrossBunsMac

final class HCBTemplateExpanderTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        // 2026-04-18 was a Saturday (weekday 7). Picked so nextWeekday:mon is
        // +2 days and nextWeekday:sun is +1.
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18))!
    }

    private func ctx(clipboard: String? = nil, prompts: [String: String] = [:]) -> HCBTemplateContext {
        HCBTemplateContext(now: now, calendar: calendar, clipboard: clipboard, prompts: prompts)
    }

    // MARK: - literal passthrough

    func testLiteralUnchanged() {
        XCTAssertEqual(HCBTemplateExpander.expand("Hello world", context: ctx()), "Hello world")
    }

    func testUnknownVariableLeftVisible() {
        // Typo should stay visible so users see which placeholder failed.
        XCTAssertEqual(HCBTemplateExpander.expand("{{tody}}", context: ctx()), "{{tody}}")
    }

    func testUnmatchedOpenerLeftIntact() {
        XCTAssertEqual(HCBTemplateExpander.expand("Hi {{unclosed", context: ctx()), "Hi {{unclosed")
    }

    // MARK: - dates

    func testToday() {
        XCTAssertEqual(HCBTemplateExpander.expand("{{today}}", context: ctx()), "2026-04-18")
    }

    func testTomorrowYesterday() {
        XCTAssertEqual(HCBTemplateExpander.expand("{{tomorrow}}", context: ctx()), "2026-04-19")
        XCTAssertEqual(HCBTemplateExpander.expand("{{yesterday}}", context: ctx()), "2026-04-17")
    }

    func testRelativeDays() {
        XCTAssertEqual(HCBTemplateExpander.expand("{{+7d}}", context: ctx()), "2026-04-25")
        XCTAssertEqual(HCBTemplateExpander.expand("{{-3d}}", context: ctx()), "2026-04-15")
    }

    func testRelativeWeeksMonths() {
        XCTAssertEqual(HCBTemplateExpander.expand("{{+2w}}", context: ctx()), "2026-05-02")
        // Month math in the gregorian calendar — adding 1 month to April 18 lands on May 18.
        XCTAssertEqual(HCBTemplateExpander.expand("{{+1m}}", context: ctx()), "2026-05-18")
    }

    func testNextWeekdayAdvancesPastToday() {
        // now = Saturday. nextWeekday:sat should be NEXT saturday (7 days), not today.
        XCTAssertEqual(HCBTemplateExpander.expand("{{nextWeekday:sat}}", context: ctx()), "2026-04-25")
        // sun → +1, mon → +2
        XCTAssertEqual(HCBTemplateExpander.expand("{{nextWeekday:sun}}", context: ctx()), "2026-04-19")
        XCTAssertEqual(HCBTemplateExpander.expand("{{nextWeekday:mon}}", context: ctx()), "2026-04-20")
    }

    func testNextWeekdayCaseInsensitive() {
        XCTAssertEqual(HCBTemplateExpander.expand("{{nextWeekday:MON}}", context: ctx()), "2026-04-20")
    }

    // MARK: - clipboard / cursor / prompt

    func testClipboardInsertion() {
        XCTAssertEqual(HCBTemplateExpander.expand("paste:{{clipboard}}", context: ctx(clipboard: "hello")),
                       "paste:hello")
    }

    func testClipboardEmptyWhenNil() {
        XCTAssertEqual(HCBTemplateExpander.expand("paste:{{clipboard}}!", context: ctx()), "paste:!")
    }

    func testCursorSentinelInserted() {
        let out = HCBTemplateExpander.expand("before {{cursor}} after", context: ctx())
        XCTAssertTrue(out.contains(HCBTemplateExpander.cursorSentinel))
    }

    func testPromptSubstitution() {
        let out = HCBTemplateExpander.expand("Owner: {{prompt:Owner}}", context: ctx(prompts: ["Owner": "Alice"]))
        XCTAssertEqual(out, "Owner: Alice")
    }

    func testPromptMissingLeftVisible() {
        // Unanswered prompt stays as literal {{prompt:Owner}} so the user sees the gap.
        let out = HCBTemplateExpander.expand("Owner: {{prompt:Owner}}", context: ctx())
        XCTAssertEqual(out, "Owner: {{prompt:Owner}}")
    }

    // MARK: - combined

    func testMixedTemplate() {
        let out = HCBTemplateExpander.expand(
            "Due {{+7d}} — owner {{prompt:Owner}}",
            context: ctx(prompts: ["Owner": "Alice"])
        )
        XCTAssertEqual(out, "Due 2026-04-25 — owner Alice")
    }

    // MARK: - TaskTemplate.requiredPrompts

    func testRequiredPromptsAggregates() {
        let t = TaskTemplate(
            id: UUID(),
            name: "Weekly review",
            title: "Review {{prompt:Week}}",
            notes: "Owned by {{prompt:Owner}}",
            due: "{{today}}",
            listIdOrTitle: ""
        )
        XCTAssertEqual(Set(t.requiredPrompts()), ["Week", "Owner"])
    }

    func testRequiredPromptsDedupesAcrossFields() {
        let t = TaskTemplate(
            id: UUID(),
            name: "x",
            title: "{{prompt:Owner}}",
            notes: "cc {{prompt:Owner}}",
            due: "",
            listIdOrTitle: ""
        )
        XCTAssertEqual(t.requiredPrompts(), ["Owner"])
    }

    func testDuplicatedTaskTemplateGetsNewIDCopyNameAndPreservesFields() {
        let originalID = UUID()
        let copyID = UUID()
        let template = TaskTemplate(
            id: originalID,
            name: "Review",
            title: "Weekly {{prompt:Topic}}",
            notes: "Notes",
            due: "{{+7d}}",
            listIdOrTitle: "Work"
        )

        let copy = template.duplicated(id: copyID)

        XCTAssertNotEqual(copy.id, template.id)
        XCTAssertEqual(copy.id, copyID)
        XCTAssertEqual(copy.name, "Review Copy")
        XCTAssertEqual(copy.title, template.title)
        XCTAssertEqual(copy.notes, template.notes)
        XCTAssertEqual(copy.due, template.due)
        XCTAssertEqual(copy.listIdOrTitle, template.listIdOrTitle)
        XCTAssertNil(copy.lastUsedAt)
        XCTAssertEqual(copy.useCount, 0)
    }

    func testDuplicatedEventTemplateGetsNewIDCopyNameAndPreservesFields() {
        let originalID = UUID()
        let copyID = UUID()
        let template = EventTemplate(
            id: originalID,
            name: "Planning",
            summary: "Plan {{prompt:Topic}}",
            details: "Agenda",
            location: "Room 4",
            dateAnchor: "{{nextWeekday:mon}}",
            timeAnchor: "09:30",
            durationMinutes: 45,
            isAllDay: false,
            reminderMinutes: 10,
            colorId: "5",
            attendees: ["ada@example.com"],
            addGoogleMeet: true,
            recurrenceRule: "FREQ=WEEKLY",
            calendarIdOrTitle: "Team"
        )

        let copy = template.duplicated(id: copyID)

        XCTAssertNotEqual(copy.id, template.id)
        XCTAssertEqual(copy.id, copyID)
        XCTAssertEqual(copy.name, "Planning Copy")
        XCTAssertEqual(copy.summary, template.summary)
        XCTAssertEqual(copy.details, template.details)
        XCTAssertEqual(copy.location, template.location)
        XCTAssertEqual(copy.dateAnchor, template.dateAnchor)
        XCTAssertEqual(copy.timeAnchor, template.timeAnchor)
        XCTAssertEqual(copy.durationMinutes, template.durationMinutes)
        XCTAssertEqual(copy.isAllDay, template.isAllDay)
        XCTAssertEqual(copy.reminderMinutes, template.reminderMinutes)
        XCTAssertEqual(copy.colorId, template.colorId)
        XCTAssertEqual(copy.attendees, template.attendees)
        XCTAssertEqual(copy.addGoogleMeet, template.addGoogleMeet)
        XCTAssertEqual(copy.recurrenceRule, template.recurrenceRule)
        XCTAssertEqual(copy.calendarIdOrTitle, template.calendarIdOrTitle)
        XCTAssertNil(copy.lastUsedAt)
        XCTAssertEqual(copy.useCount, 0)
    }

    func testTaskTemplateDecodesMissingUsageMetadata() throws {
        let id = UUID()
        let json = """
        {
          "id": "\(id.uuidString)",
          "name": "Legacy",
          "title": "Do it"
        }
        """.data(using: .utf8)!

        let decoded = try JSONDecoder().decode(TaskTemplate.self, from: json)

        XCTAssertNil(decoded.lastUsedAt)
        XCTAssertEqual(decoded.useCount, 0)
    }

    func testEventTemplateDecodesMissingUsageMetadata() throws {
        let id = UUID()
        let json = """
        {
          "id": "\(id.uuidString)",
          "name": "Legacy",
          "summary": "Meet"
        }
        """.data(using: .utf8)!

        let decoded = try JSONDecoder().decode(EventTemplate.self, from: json)

        XCTAssertNil(decoded.lastUsedAt)
        XCTAssertEqual(decoded.useCount, 0)
    }

    @MainActor
    func testMarkTemplateUsedOnlyUpdatesTarget() {
        let taskID = UUID()
        let otherTaskID = UUID()
        let eventID = UUID()
        let otherEventID = UUID()
        let model = AppModel.bootstrap()
        model.settings.taskTemplates = [
            TaskTemplate(id: taskID, name: "Target", title: "Task"),
            TaskTemplate(id: otherTaskID, name: "Other", title: "Task")
        ]
        model.settings.eventTemplates = [
            EventTemplate(id: eventID, name: "Target", summary: "Event"),
            EventTemplate(id: otherEventID, name: "Other", summary: "Event")
        ]

        model.markTaskTemplateUsed(taskID)
        model.markEventTemplateUsed(eventID)

        let task = model.settings.taskTemplates.first { $0.id == taskID }
        let otherTask = model.settings.taskTemplates.first { $0.id == otherTaskID }
        let event = model.settings.eventTemplates.first { $0.id == eventID }
        let otherEvent = model.settings.eventTemplates.first { $0.id == otherEventID }
        XCTAssertEqual(task?.useCount, 1)
        XCTAssertNotNil(task?.lastUsedAt)
        XCTAssertEqual(otherTask?.useCount, 0)
        XCTAssertNil(otherTask?.lastUsedAt)
        XCTAssertEqual(event?.useCount, 1)
        XCTAssertNotNil(event?.lastUsedAt)
        XCTAssertEqual(otherEvent?.useCount, 0)
        XCTAssertNil(otherEvent?.lastUsedAt)
    }
}
