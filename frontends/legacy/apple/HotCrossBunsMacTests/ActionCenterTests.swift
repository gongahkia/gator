import XCTest
@testable import HotCrossBunsMac

final class ActionCenterTests: XCTestCase {
    private let calendar: Calendar = {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 9))!
    }

    private func day(_ offset: Int, hour: Int = 9) -> Date {
        let start = calendar.date(byAdding: .day, value: offset, to: calendar.startOfDay(for: now))!
        return calendar.date(byAdding: .hour, value: hour, to: start)!
    }

    func testBuildGroupsAvailabilityHoldsAndCapsOverdueTasks() {
        let tasks = [
            task(id: "older", due: day(-4)),
            task(id: "late", due: day(-1)),
            task(id: "today", due: day(0)),
            task(id: "done", due: day(-2), completed: true),
            task(id: "hidden", due: day(-2), hidden: true),
            task(id: "deleted", due: day(-2), deleted: true)
        ]
        let events = [
            hold(id: "hold-b", groupID: "group-b", title: "Design review", start: day(2), end: day(2, hour: 10)),
            hold(id: "hold-a", groupID: "group-a", title: "Planning", start: day(1), end: day(1, hour: 10)),
            hold(id: "hold-cancelled", groupID: "group-c", title: "Ignore", start: day(0), end: day(0, hour: 10), status: .cancelled),
            event(id: "normal", start: day(1), end: day(1, hour: 10))
        ]

        let snapshot = ActionCenterBuilder.build(
            tasks: tasks,
            events: events,
            pendingMutations: [],
            notificationSummary: nil,
            authState: .signedOut,
            syncState: .idle,
            isSyncPaused: false,
            mutationError: nil,
            syncFailureKind: nil,
            networkReachability: .online,
            referenceDate: now,
            calendar: calendar,
            overdueTaskDisplayLimit: 1
        )

        XCTAssertEqual(snapshot.holdGroups.map(\.id), ["group-a", "group-b"])
        XCTAssertEqual(snapshot.overdueTaskCount, 2)
        XCTAssertEqual(snapshot.overdueTasks.map(\.id), ["older"])
        XCTAssertEqual(snapshot.overdueTaskOverflowCount, 1)
        XCTAssertEqual(snapshot.actionableCount, 4)
    }

    func testMutationBucketsAvoidDoubleCountingConflictAndInvalidPayloadRows() {
        let mutations = [
            mutation(id: "00000000-0000-0000-0000-000000000001", quarantined: true, conflicted: true),
            mutation(id: "00000000-0000-0000-0000-000000000002", quarantined: true, error: "Invalid payload - bad"),
            mutation(id: "00000000-0000-0000-0000-000000000003", quarantined: true, error: "Rate limited"),
            mutation(id: "00000000-0000-0000-0000-000000000004")
        ]

        let snapshot = ActionCenterBuilder.build(
            tasks: [],
            events: [],
            pendingMutations: mutations,
            notificationSummary: nil,
            authState: .signedOut,
            syncState: .idle,
            isSyncPaused: false,
            mutationError: nil,
            syncFailureKind: nil,
            networkReachability: .online,
            referenceDate: now,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.issues.map(\.kind), [.conflicts, .invalidPayloads, .quarantined])
        XCTAssertEqual(snapshot.issues.map(\.count), [1, 1, 1])
        XCTAssertEqual(snapshot.actionableCount, 3)
    }

    func testSyncMutationAndDeferredReminderRowsContributeActionableCounts() {
        let summary = NotificationScheduleSummary(
            scheduledEvents: 10,
            scheduledTasks: 4,
            deferredEvents: 2,
            deferredTasks: 3,
            failedEvents: 0,
            failedTasks: 0,
            windowDays: 30,
            computedAt: now
        )

        let snapshot = ActionCenterBuilder.build(
            tasks: [],
            events: [],
            pendingMutations: [],
            notificationSummary: summary,
            authState: .signedOut,
            syncState: .failed(message: "Timed out"),
            isSyncPaused: false,
            mutationError: "Could not save the last edit.",
            syncFailureKind: .offline,
            networkReachability: .offline,
            referenceDate: now,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.issues.map(\.kind), [.mutationError, .syncFailure, .deferredReminders])
        XCTAssertEqual(snapshot.issues.map(\.count), [1, 1, 5])
        XCTAssertEqual(snapshot.actionableCount, 7)
    }

    func testAuthFailureSuppressesDuplicateAuthRequiredSyncFailure() {
        let snapshot = ActionCenterBuilder.build(
            tasks: [],
            events: [],
            pendingMutations: [],
            notificationSummary: nil,
            authState: .failed("Token expired"),
            syncState: .failed(message: "Token expired"),
            isSyncPaused: false,
            mutationError: nil,
            syncFailureKind: .authRequired,
            networkReachability: .online,
            referenceDate: now,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.issues.map(\.kind), [.authFailure])
        XCTAssertEqual(snapshot.actionableCount, 1)
    }

    private func task(
        id: String,
        due: Date?,
        completed: Bool = false,
        hidden: Bool = false,
        deleted: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: "list",
            parentID: nil,
            title: id,
            notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: completed ? now : nil,
            isDeleted: deleted,
            isHidden: hidden,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    private func event(
        id: String,
        start: Date,
        end: Date,
        status: CalendarEventStatus = .confirmed,
        availabilityHold: AvailabilityHoldMetadata? = nil
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: "primary",
            summary: id,
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: false,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            reminderMinutes: [],
            availabilityHold: availabilityHold
        )
    }

    private func hold(
        id: String,
        groupID: String,
        title: String,
        start: Date,
        end: Date,
        status: CalendarEventStatus = .confirmed
    ) -> CalendarEventMirror {
        event(
            id: id,
            start: start,
            end: end,
            status: status,
            availabilityHold: AvailabilityHoldMetadata(
                groupID: groupID,
                title: title,
                durationMinutes: 30,
                createdAt: now
            )
        )
    }

    private func mutation(
        id: String,
        quarantined: Bool = false,
        conflicted: Bool = false,
        error: String? = nil
    ) -> PendingMutation {
        PendingMutation(
            id: UUID(uuidString: id)!,
            createdAt: now,
            resourceType: .task,
            resourceID: id,
            action: .update,
            payload: Data(),
            lastErrorSummary: error,
            quarantinedAt: quarantined ? now : nil,
            conflictedAt: conflicted ? now : nil
        )
    }
}
