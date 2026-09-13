import XCTest
@testable import HotCrossBunsMac

final class CalendarViewFilterTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private func day(_ y: Int, _ m: Int, _ d: Int, hour: Int = 0) -> Date {
        calendar.date(from: DateComponents(year: y, month: m, day: d, hour: hour))!
    }

    private func event(
        id: String,
        calendarID: String = "work",
        summary: String = "Planning",
        details: String = "",
        location: String = "",
        colorId: String? = nil,
        status: CalendarEventStatus = .confirmed
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: summary,
            details: details,
            startDate: day(2026, 5, 11, hour: 9),
            endDate: day(2026, 5, 11, hour: 10),
            isAllDay: false,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            location: location,
            colorId: colorId
        )
    }

    func testEmptyPersistedStateDecodesToAllVisible() {
        XCTAssertEqual(CalendarViewFilterState.decoded(from: ""), .allVisible)
        XCTAssertEqual(CalendarViewFilterState.allVisible.encodedString(), "")
    }

    func testPersistedStateRoundTripsAndNormalizesTags() {
        let state = CalendarViewFilterState(
            visibleCalendarIDs: ["work"],
            visibleColorIDs: [CalendarEventColor.sage.rawValue],
            visibleTagNames: ["#Launch", " Ops "]
        )

        let decoded = CalendarViewFilterState.decoded(from: state.encodedString())

        XCTAssertEqual(decoded.visibleCalendarIDs, ["work"])
        XCTAssertEqual(decoded.visibleColorIDs, [CalendarEventColor.sage.rawValue])
        XCTAssertEqual(decoded.visibleTagNames, ["launch", "ops"])
    }

    func testCalendarFilterAllowsOnlyVisibleCalendarIDs() {
        let filter = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: ["work"], visibleColorIDs: nil, visibleTagNames: nil)
        )

        XCTAssertTrue(filter.allows(event(id: "visible", calendarID: "work")))
        XCTAssertFalse(filter.allows(event(id: "hidden", calendarID: "personal")))
    }

    func testColorFilterDistinguishesDefaultAndExplicitColors() {
        let defaultOnly = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: nil, visibleColorIDs: [CalendarEventColor.defaultColor.rawValue], visibleTagNames: nil)
        )
        let peacockOnly = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: nil, visibleColorIDs: [CalendarEventColor.peacock.rawValue], visibleTagNames: nil)
        )

        XCTAssertTrue(defaultOnly.allows(event(id: "default", colorId: nil)))
        XCTAssertFalse(defaultOnly.allows(event(id: "blue", colorId: CalendarEventColor.peacock.rawValue)))
        XCTAssertTrue(peacockOnly.allows(event(id: "blue", colorId: CalendarEventColor.peacock.rawValue)))
    }

    func testLiteralHashtagFilterMatchesSummaryDetailsAndLocation() {
        let filter = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: nil, visibleColorIDs: nil, visibleTagNames: ["launch"])
        )

        XCTAssertTrue(filter.allows(event(id: "summary", summary: "Prep #Launch")))
        XCTAssertTrue(filter.allows(event(id: "details", details: "Bring #launch notes")))
        XCTAssertTrue(filter.allows(event(id: "location", location: "Room #launch")))
        XCTAssertTrue(filter.allows(event(id: "untagged", summary: "Planning")))
        XCTAssertFalse(filter.allows(event(id: "other", summary: "Prep #ops")))
    }

    func testColorBindingTagMatchesMappedEventColor() {
        let filter = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: nil, visibleColorIDs: nil, visibleTagNames: ["marketing"]),
            colorTagBindings: [CalendarEventColor.sage.rawValue: "#Marketing"]
        )

        XCTAssertTrue(filter.allows(event(id: "mapped", colorId: CalendarEventColor.sage.rawValue)))
        XCTAssertTrue(filter.allows(event(id: "unmapped-untagged", colorId: CalendarEventColor.tomato.rawValue)))
        XCTAssertFalse(filter.allows(event(id: "unmapped-tagged", summary: "Launch #ops", colorId: CalendarEventColor.tomato.rawValue)))
    }

    func testEmptyVisibleTagSetHidesOnlyTaggedEvents() {
        let filter = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: nil, visibleColorIDs: nil, visibleTagNames: []),
            colorTagBindings: [CalendarEventColor.sage.rawValue: "#Marketing"]
        )

        XCTAssertTrue(filter.allows(event(id: "untagged", summary: "Planning", colorId: CalendarEventColor.tomato.rawValue)))
        XCTAssertFalse(filter.allows(event(id: "literal", summary: "Prep #Launch", colorId: CalendarEventColor.tomato.rawValue)))
        XCTAssertFalse(filter.allows(event(id: "mapped", summary: "Planning", colorId: CalendarEventColor.sage.rawValue)))
    }

    func testHashUnitNumberDoesNotBecomeCalendarTag() {
        let event = event(id: "unit", summary: "Viewing at #11-07", colorId: CalendarEventColor.tomato.rawValue)
        let filter = CalendarEventViewFilter(
            state: CalendarViewFilterState(visibleCalendarIDs: nil, visibleColorIDs: nil, visibleTagNames: [])
        )

        XCTAssertTrue(CalendarEventViewFilter.literalTagNames(in: event).isEmpty)
        XCTAssertTrue(filter.allows(event))
    }

    func testCalendarSnapshotCountsLiteralAndColorBoundTags() {
        let calendars = [
            CalendarListMirror(
                id: "work",
                summary: "Work",
                colorHex: "#d50000",
                isSelected: true,
                accessRole: "owner"
            ),
            CalendarListMirror(
                id: "personal",
                summary: "Personal",
                colorHex: "#039be5",
                isSelected: false,
                accessRole: "reader"
            )
        ]
        var settings = AppSettings.default
        settings.colorTagBindings = [CalendarEventColor.sage.rawValue: "#Marketing"]

        let snapshot = CalendarSnapshot.build(
            calendars: calendars,
            events: [
                event(
                    id: "literal",
                    summary: "Launch #Ops",
                    details: "Prep #Roadmap",
                    location: "Room #11-07",
                    colorId: CalendarEventColor.sage.rawValue
                ),
                event(id: "mapped", colorId: CalendarEventColor.sage.rawValue),
                event(
                    id: "cancelled",
                    summary: "Cancelled #Ops",
                    colorId: CalendarEventColor.sage.rawValue,
                    status: .cancelled
                ),
                event(
                    id: "hidden-calendar",
                    calendarID: "personal",
                    summary: "Hidden #Ops",
                    colorId: CalendarEventColor.sage.rawValue
                )
            ],
            settings: settings
        )

        XCTAssertEqual(snapshot.selectedCalendarIDs, ["work"])
        XCTAssertEqual(snapshot.eventCountsByCalendarID["work"], 2)
        XCTAssertNil(snapshot.eventCountsByCalendarID["personal"])
        XCTAssertEqual(snapshot.eventCountsByColorID[CalendarEventColor.sage.rawValue], 2)
        XCTAssertEqual(snapshot.eventCountsByTagName["ops"], 1)
        XCTAssertEqual(snapshot.eventCountsByTagName["roadmap"], 1)
        XCTAssertEqual(snapshot.eventCountsByTagName["marketing"], 2)
        XCTAssertNil(snapshot.eventCountsByTagName["11-07"])
    }

    func testCalendarSnapshotCountsUntaggedEventsAndDeduplicatesLiteralTags() {
        let calendars = [
            CalendarListMirror(
                id: "work",
                summary: "Work",
                colorHex: "#d50000",
                isSelected: true,
                accessRole: "owner"
            )
        ]

        let snapshot = CalendarSnapshot.build(
            calendars: calendars,
            events: [
                event(
                    id: "duplicate-tag",
                    summary: "Launch #Ops",
                    details: "Prep #ops",
                    location: "Room #Ops"
                ),
                event(id: "untagged", summary: "Planning")
            ],
            settings: .default
        )

        XCTAssertEqual(snapshot.eventCountsByCalendarID["work"], 2)
        XCTAssertEqual(snapshot.eventCountsByColorID[CalendarEventColor.defaultColor.rawValue], 2)
        XCTAssertEqual(snapshot.eventCountsByTagName["ops"], 1)
    }

    func testSectionsCombineWithAndSemantics() {
        let filter = CalendarEventViewFilter(
            state: CalendarViewFilterState(
                visibleCalendarIDs: ["work"],
                visibleColorIDs: [CalendarEventColor.tomato.rawValue],
                visibleTagNames: ["urgent"]
            )
        )

        XCTAssertTrue(filter.allows(event(
            id: "match",
            calendarID: "work",
            summary: "Incident #urgent",
            colorId: CalendarEventColor.tomato.rawValue
        )))
        XCTAssertFalse(filter.allows(event(
            id: "wrong-calendar",
            calendarID: "personal",
            summary: "Incident #urgent",
            colorId: CalendarEventColor.tomato.rawValue
        )))
        XCTAssertFalse(filter.allows(event(
            id: "wrong-color",
            calendarID: "work",
            summary: "Incident #urgent",
            colorId: CalendarEventColor.sage.rawValue
        )))
        XCTAssertFalse(filter.allows(event(
            id: "wrong-tag",
            calendarID: "work",
            summary: "Incident #later",
            colorId: CalendarEventColor.tomato.rawValue
        )))
    }
}
