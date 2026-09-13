import Testing
import Foundation
@testable import HotCrossBunsMac

// Migrated from XCTest to Swift Testing. The snapping table collapses the
// four individual "drop at Y → snaps to H:M" tests into a single
// parameterized row. New snap cases (e.g. 22:30, 23:45 edge) are now just
// another tuple.

struct CalendarDropComputerTests {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var dayStart: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18))!
    }

    // (description, drop-y pixels, expected hour, expected minute)
    // hourHeight is a constant 44 across these rows; if a future case needs
    // a different row height, add a fifth tuple element.
    @Test(arguments: [
        ("y=22 → 30 raw minutes → snaps to 0:30", 22.0, 0, 30),
        ("y=80 → ~109 raw minutes → snaps to 1:45", 80.0, 1, 45),
    ])
    func snapsTo15MinIntervals(_ description: String, y: CGFloat, expectedHour: Int, expectedMinute: Int) {
        let start = CalendarDropComputer.snappedStart(for: y, hourHeight: 44, dayStart: dayStart, calendar: calendar)
        #expect(calendar.component(.hour, from: start) == expectedHour, Comment(rawValue: description))
        #expect(calendar.component(.minute, from: start) == expectedMinute, Comment(rawValue: description))
    }

    @Test func clampsBelowZero() {
        let start = CalendarDropComputer.snappedStart(for: -200, hourHeight: 44, dayStart: dayStart, calendar: calendar)
        #expect(start == dayStart)
    }

    @Test func clampsAboveEndOfDay() {
        let start = CalendarDropComputer.snappedStart(for: 100_000, hourHeight: 44, dayStart: dayStart, calendar: calendar)
        let minutes = calendar.component(.hour, from: start) * 60 + calendar.component(.minute, from: start)
        #expect(minutes <= 23 * 60)
    }

    @Test func defaultDurationIs60Minutes() {
        let start = CalendarDropComputer.snappedStart(for: 44 * 9, hourHeight: 44, dayStart: dayStart, calendar: calendar)
        let end = CalendarDropComputer.defaultEndDate(from: start, calendar: calendar)
        #expect(end.timeIntervalSince(start) == 60 * 60)
    }

    @Test func hourHeightZeroReturnsDayStart() {
        let start = CalendarDropComputer.snappedStart(for: 200, hourHeight: 0, dayStart: dayStart, calendar: calendar)
        #expect(start == dayStart)
    }

    @Test func normalizesAvailabilitySlotsBySortingAndDroppingDuplicatesOrInvalidRanges() {
        let early = AvailabilitySlot(startDate: date(hour: 9), endDate: date(hour: 9, minute: 30))
        let late = AvailabilitySlot(startDate: date(hour: 11), endDate: date(hour: 11, minute: 30))
        let invalid = AvailabilitySlot(startDate: date(hour: 14), endDate: date(hour: 13))

        let normalized = AvailabilitySlotResolver.normalized([late, invalid, early, early])

        #expect(normalized == [early, late])
    }

    @Test func availabilityBlockingIgnoresTransparentCancelledAllDayAndUnselectedCalendarEvents() {
        let slot = AvailabilitySlot(startDate: date(hour: 9), endDate: date(hour: 10))
        let events = [
            event(
                id: "transparent",
                startDate: date(hour: 9, minute: 15),
                endDate: date(hour: 9, minute: 45),
                transparency: .transparent
            ),
            event(
                id: "cancelled",
                startDate: date(hour: 9, minute: 15),
                endDate: date(hour: 9, minute: 45),
                status: .cancelled
            ),
            event(
                id: "all-day",
                startDate: dayStart,
                endDate: calendar.date(byAdding: .day, value: 1, to: dayStart)!,
                isAllDay: true
            ),
            event(
                id: "other-calendar",
                calendarID: "secondary",
                startDate: date(hour: 9, minute: 15),
                endDate: date(hour: 9, minute: 45)
            ),
            event(
                id: "busy",
                startDate: date(hour: 9, minute: 30),
                endDate: date(hour: 10, minute: 30)
            )
        ]

        let blocking = AvailabilitySlotResolver.blockingEvents(
            for: slot,
            events: events,
            calendarIDs: ["primary"]
        )

        #expect(blocking.map(\.id) == ["busy"])
    }

    @Test func availabilitySlotOverlapTreatsTouchingBoundariesAsAvailable() {
        let selected = [
            AvailabilitySlot(startDate: date(hour: 9), endDate: date(hour: 10))
        ]

        #expect(AvailabilitySlotResolver.overlapsSelectedSlots(
            AvailabilitySlot(startDate: date(hour: 9, minute: 30), endDate: date(hour: 10, minute: 30)),
            selectedSlots: selected
        ))
        #expect(AvailabilitySlotResolver.overlapsSelectedSlots(
            AvailabilitySlot(startDate: date(hour: 10), endDate: date(hour: 10, minute: 30)),
            selectedSlots: selected
        ) == false)
    }

    private func date(hour: Int, minute: Int = 0) -> Date {
        calendar.date(byAdding: .minute, value: hour * 60 + minute, to: dayStart)!
    }

    private func event(
        id: String,
        calendarID: String = "primary",
        startDate: Date,
        endDate: Date,
        isAllDay: Bool = false,
        status: CalendarEventStatus = .confirmed,
        transparency: CalendarEventTransparency = .opaque
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: id,
            details: "",
            startDate: startDate,
            endDate: endDate,
            isAllDay: isAllDay,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            transparency: transparency
        )
    }
}
