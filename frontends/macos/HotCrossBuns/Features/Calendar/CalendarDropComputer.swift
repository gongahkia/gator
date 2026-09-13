import Foundation

enum CalendarDropComputer {
    static let snapMinutes: Int = 15
    static let defaultDurationMinutes: Int = 60

    static func snappedStart(
        for dropY: CGFloat,
        hourHeight: CGFloat,
        dayStart: Date,
        calendar: Calendar = .current
    ) -> Date {
        guard hourHeight > 0 else { return dayStart }
        let rawMinutes = Int((dropY / hourHeight) * 60)
        let clamped = max(0, min(rawMinutes, 24 * 60 - defaultDurationMinutes))
        let snapped = (clamped / snapMinutes) * snapMinutes
        return calendar.date(byAdding: .minute, value: snapped, to: dayStart) ?? dayStart
    }

    static func defaultEndDate(from start: Date, calendar: Calendar = .current) -> Date {
        calendar.date(byAdding: .minute, value: defaultDurationMinutes, to: start) ?? start
    }
}

struct AvailabilitySlot: Identifiable, Hashable, Codable, Sendable {
    var startDate: Date
    var endDate: Date

    var id: String {
        "\(startDate.timeIntervalSinceReferenceDate)-\(endDate.timeIntervalSinceReferenceDate)"
    }

    var durationMinutes: Int {
        max(0, Int(endDate.timeIntervalSince(startDate) / 60))
    }
}

struct AvailabilityGridSelection {
    var slots: [AvailabilitySlot]
    var defaultDurationMinutes: Int
    var onSelect: (AvailabilitySlot) -> Void
    var onReject: (String) -> Void
    var isSlotAvailable: (AvailabilitySlot) -> Bool
}

struct AvailabilityHoldGroup: Identifiable, Equatable, Sendable {
    var id: String
    var metadata: AvailabilityHoldMetadata
    var events: [CalendarEventMirror]
}

enum AvailabilityHoldLimits {
    static let maxSlotsPerGroup = 24
}

enum AvailabilitySlotResolver {
    static func normalized(_ slots: [AvailabilitySlot]) -> [AvailabilitySlot] {
        var seen: Set<String> = []
        return slots
            .filter { $0.endDate > $0.startDate }
            .sorted { lhs, rhs in
                if lhs.startDate == rhs.startDate { return lhs.endDate < rhs.endDate }
                return lhs.startDate < rhs.startDate
            }
            .filter { slot in
                guard seen.contains(slot.id) == false else { return false }
                seen.insert(slot.id)
                return true
            }
    }

    static func overlaps(_ lhs: AvailabilitySlot, _ rhs: AvailabilitySlot) -> Bool {
        lhs.startDate < rhs.endDate && lhs.endDate > rhs.startDate
    }

    static func blockingEvents(
        for slot: AvailabilitySlot,
        events: [CalendarEventMirror],
        calendarIDs: Set<CalendarListMirror.ID>
    ) -> [CalendarEventMirror] {
        events.filter { event in
            guard calendarIDs.contains(event.calendarID),
                  event.status != .cancelled,
                  event.isAllDay == false,
                  event.transparency != .transparent
            else {
                return false
            }
            return slot.startDate < event.endDate && slot.endDate > event.startDate
        }
    }

    static func overlapsSelectedSlots(
        _ slot: AvailabilitySlot,
        selectedSlots: [AvailabilitySlot]
    ) -> Bool {
        selectedSlots.contains { overlaps(slot, $0) }
    }
}

enum AvailabilitySnippetFormatter {
    static func snippet(
        title: String,
        durationMinutes: Int,
        timeZoneID: String,
        slots: [AvailabilitySlot],
        calendar: Calendar = .current
    ) -> String {
        let resolvedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let heading = resolvedTitle.isEmpty ? "Available meeting times" : resolvedTitle
        let normalized = AvailabilitySlotResolver.normalized(slots)
        var lines = [
            heading,
            "\(max(15, durationMinutes)) minutes - \(TimezoneSupport.displayName(for: timeZoneID))"
        ]

        guard normalized.isEmpty == false else {
            lines.append("No slots selected.")
            return lines.joined(separator: "\n")
        }

        for slot in normalized {
            lines.append("- \(dateLabel(slot.startDate, timeZoneID: timeZoneID, calendar: calendar)): \(timeLabel(slot.startDate, timeZoneID: timeZoneID))-\(timeLabel(slot.endDate, timeZoneID: timeZoneID))")
        }
        return lines.joined(separator: "\n")
    }

    static func holdDetails(
        title: String,
        durationMinutes: Int,
        timeZoneID: String,
        slot: AvailabilitySlot
    ) -> String {
        """
        Availability hold for \(title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Meeting" : title.trimmingCharacters(in: .whitespacesAndNewlines)).
        \(max(15, durationMinutes)) minutes - \(TimezoneSupport.displayName(for: timeZoneID))
        \(dateLabel(slot.startDate, timeZoneID: timeZoneID)): \(timeLabel(slot.startDate, timeZoneID: timeZoneID))-\(timeLabel(slot.endDate, timeZoneID: timeZoneID))
        """
    }

    private static func dateLabel(_ date: Date, timeZoneID: String, calendar: Calendar = .current) -> String {
        let formatter = DateFormatter()
        formatter.calendar = calendar
        formatter.timeZone = TimezoneSupport.timeZone(for: timeZoneID)
        formatter.locale = .current
        formatter.dateFormat = "EEE, MMM d"
        return formatter.string(from: date)
    }

    private static func timeLabel(_ date: Date, timeZoneID: String) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.timeZone = TimezoneSupport.timeZone(for: timeZoneID)
        formatter.locale = .current
        formatter.dateStyle = .none
        formatter.timeStyle = .short
        return formatter.string(from: date)
    }
}
