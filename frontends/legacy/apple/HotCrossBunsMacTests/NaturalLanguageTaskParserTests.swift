import XCTest
@testable import HotCrossBunsMac

final class NaturalLanguageTaskParserTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        cal.firstWeekday = 1
        return cal
    }()

    private var now: Date {
        // saturday, 2026-04-18 10:00 UTC
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 10))!
    }

    private func parser() -> NaturalLanguageTaskParser {
        NaturalLanguageTaskParser(now: now, calendar: calendar)
    }

    private func day(_ year: Int, _ month: Int, _ day: Int) -> Date {
        calendar.date(from: DateComponents(year: year, month: month, day: day))!
    }

    func testPlainTitleHasNoMetadata() {
        let result = parser().parse("call dentist")
        XCTAssertEqual(result.title, "call dentist")
        XCTAssertNil(result.dueDate)
        XCTAssertNil(result.taskListHint)
        XCTAssertTrue(result.matchedTokens.isEmpty)
    }

    func testTomorrowToken() {
        let result = parser().parse("email rent receipt tmr")
        XCTAssertEqual(result.title, "email rent receipt")
        XCTAssertEqual(result.dueDate, day(2026, 4, 19))
    }

    func testTodayToken() {
        let result = parser().parse("ship release today")
        XCTAssertEqual(result.title, "ship release")
        XCTAssertEqual(result.dueDate, calendar.startOfDay(for: now))
    }

    func testInDaysOffset() {
        let result = parser().parse("book flight in 3 days")
        XCTAssertEqual(result.title, "book flight")
        XCTAssertEqual(result.dueDate, day(2026, 4, 21))
    }

    func testInWeeksOffset() {
        let result = parser().parse("review roadmap in 2 weeks")
        XCTAssertEqual(result.title, "review roadmap")
        XCTAssertEqual(result.dueDate, day(2026, 5, 2))
    }

    func testWeekdayResolvesForward() {
        // saturday now; "mon" should be the next Monday (4/20)
        let result = parser().parse("gym mon")
        XCTAssertEqual(result.title, "gym")
        XCTAssertEqual(result.dueDate, day(2026, 4, 20))
    }

    func testSameWeekdayResolvesNextWeek() {
        // saturday now; "sat" should go to next Saturday (4/25), not today
        let result = parser().parse("laundry sat")
        XCTAssertEqual(result.title, "laundry")
        XCTAssertEqual(result.dueDate, day(2026, 4, 25))
    }

    func testMonthDay() {
        let result = parser().parse("tax filing apr 25")
        XCTAssertEqual(result.title, "tax filing")
        XCTAssertEqual(result.dueDate, day(2026, 4, 25))
    }

    func testMonthDayWithOrdinal() {
        let result = parser().parse("book physio may 3rd")
        XCTAssertEqual(result.title, "book physio")
        XCTAssertEqual(result.dueDate, day(2026, 5, 3))
    }

    func testPastMonthRollsToNextYear() {
        // saturday is april 18 2026; "mar 10" has passed, so expect 2027
        let result = parser().parse("file report mar 10")
        XCTAssertEqual(result.dueDate, day(2027, 3, 10))
    }

    func testNumericMonthDay() {
        let result = parser().parse("renew license 5/20")
        XCTAssertEqual(result.title, "renew license")
        XCTAssertEqual(result.dueDate, day(2026, 5, 20))
    }

    func testListHintCaptures() {
        let result = parser().parse("read paper #work")
        XCTAssertEqual(result.title, "read paper")
        XCTAssertEqual(result.taskListHint, "work")
    }

    func testCombinedDueAndList() {
        let result = parser().parse("submit invoice tmr #freelance")
        XCTAssertEqual(result.title, "submit invoice")
        XCTAssertEqual(result.dueDate, day(2026, 4, 19))
        XCTAssertEqual(result.taskListHint, "freelance")
    }

    func testEmptyInputReturnsEmpty() {
        let result = parser().parse("   ")
        XCTAssertEqual(result.title, "")
        XCTAssertNil(result.dueDate)
        XCTAssertNil(result.taskListHint)
    }

    func testTitlePreservesInternalSpacing() {
        let result = parser().parse("write    blog post   tmr")
        XCTAssertEqual(result.title, "write blog post")
    }

    func testListHintAllowsHyphenAndUnderscore() {
        let result = parser().parse("refactor #side_project-v2")
        XCTAssertEqual(result.taskListHint, "side_project-v2")
    }

    func testUnitNumberHashIsNotListHintOrDueDate() {
        let result = parser().parse("view house #11-07 #home")
        XCTAssertEqual(result.title, "view house #11-07")
        XCTAssertEqual(result.taskListHint, "home")
        XCTAssertNil(result.dueDate)
    }

    // MARK: - Expanded vocabulary

    func testTodayAbbreviationTDY() {
        let result = parser().parse("ship release tdy")
        XCTAssertEqual(result.title, "ship release")
        XCTAssertEqual(result.dueDate, calendar.startOfDay(for: now))
    }

    func testTomorrowAbbreviationTMW() {
        let result = parser().parse("send followup tmw")
        XCTAssertEqual(result.title, "send followup")
        XCTAssertEqual(result.dueDate, day(2026, 4, 19))
    }

    func testTomorrow2Moro() {
        let result = parser().parse("pay bill 2moro")
        XCTAssertEqual(result.title, "pay bill")
        XCTAssertEqual(result.dueDate, day(2026, 4, 19))
    }

    func testDayAfterTomorrowDAT() {
        let result = parser().parse("vet appointment dat")
        XCTAssertEqual(result.title, "vet appointment")
        XCTAssertEqual(result.dueDate, day(2026, 4, 20))
    }

    func testEndOfDayEOD() {
        let result = parser().parse("send report eod")
        XCTAssertEqual(result.title, "send report")
        XCTAssertEqual(result.dueDate, calendar.startOfDay(for: now))
    }

    func testEndOfWeekEOW() {
        // saturday 4/18, EOW = Saturday (delta 0 → bump to 7 next Saturday)
        let result = parser().parse("draft presentation eow")
        XCTAssertEqual(result.dueDate, day(2026, 4, 25))
    }

    func testEndOfMonthEOM() {
        let result = parser().parse("quarterly review eom")
        XCTAssertEqual(result.dueDate, day(2026, 4, 30))
    }

    func testEndOfYearEOY() {
        let result = parser().parse("file taxes eoy")
        XCTAssertEqual(result.dueDate, day(2026, 12, 31))
    }

    func testNextWeekAbbreviationNW() {
        let result = parser().parse("call dad nw")
        XCTAssertEqual(result.dueDate, day(2026, 4, 25))
    }

    func testNextMonthAbbreviationNM() {
        let result = parser().parse("dentist nm")
        XCTAssertEqual(result.dueDate, day(2026, 5, 18))
    }

    func testWeekendToken() {
        // saturday now; "weekend" → upcoming Saturday (today) → delta 0 → bump to 7 → 4/25
        let result = parser().parse("plan trip this weekend")
        XCTAssertEqual(result.dueDate, day(2026, 4, 25))
    }

    func testTwoLetterWeekday() {
        // "mo" = Monday. Saturday now → Monday = 2 days later (4/20).
        let result = parser().parse("gym mo")
        XCTAssertEqual(result.dueDate, day(2026, 4, 20))
    }

    func testISODate() {
        let result = parser().parse("launch 2026-07-15")
        XCTAssertEqual(result.title, "launch")
        XCTAssertEqual(result.dueDate, day(2026, 7, 15))
    }

    func testNumericDashDate() {
        let result = parser().parse("ship 5-20")
        XCTAssertEqual(result.dueDate, day(2026, 5, 20))
    }

    func testNextWeekdayBumpsOneWeek() {
        // saturday now; "next mon" = Monday AFTER the coming one = +9 days = 4/27
        let result = parser().parse("sprint review next mon")
        XCTAssertEqual(result.dueDate, day(2026, 4, 27))
    }
}

final class CalendarTagResolverTests: XCTestCase {
    func testResolvesCalendarByHashtagSummary() {
        let calendars = [
            CalendarListMirror(id: "personal", summary: "Personal", colorHex: "#33b679", isSelected: true, accessRole: "owner"),
            CalendarListMirror(id: "work", summary: "Work", colorHex: "#039be5", isSelected: true, accessRole: "owner")
        ]

        let resolution = CalendarTagResolver.resolve(title: "Planning sync #work tomorrow", calendars: calendars)

        XCTAssertEqual(resolution?.calendar.id, "work")
        XCTAssertEqual(resolution?.matchedTag, "work")
    }

    func testCalendarTagKeyIgnoresSpacesCaseAndPunctuation() {
        let calendars = [
            CalendarListMirror(id: "school", summary: "School Runs", colorHex: "#f6bf26", isSelected: true, accessRole: "owner")
        ]

        let resolution = CalendarTagResolver.resolve(title: "Pickup #school-runs", calendars: calendars)

        XCTAssertEqual(resolution?.calendar.id, "school")
        XCTAssertEqual(CalendarTagResolver.tagKey(for: "School Runs"), "schoolruns")
    }

    func testCalendarTagResolverPreservesUnmatchedTags() {
        let calendars = [
            CalendarListMirror(id: "work", summary: "Work", colorHex: "#039be5", isSelected: true, accessRole: "owner")
        ]

        XCTAssertNil(CalendarTagResolver.resolve(title: "Planning #personal", calendars: calendars))
    }
}
