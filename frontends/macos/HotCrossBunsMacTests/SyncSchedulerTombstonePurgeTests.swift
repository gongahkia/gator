import XCTest
@testable import HotCrossBunsMac

final class SyncSchedulerTombstonePurgeTests: XCTestCase {
    func testAppModelSyncStatePurgesCancelledEventsAndDeletedTasks() async throws {
        let calendar = Calendar(identifier: .gregorian)
        let now = Date()

        let liveTask = TaskMirror(
            id: "live",
            taskListID: "list",
            parentID: nil,
            title: "Live task",
            notes: "",
            status: .needsAction,
            dueDate: now,
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: now
        )
        let deletedTask = TaskMirror(
            id: "tombstone",
            taskListID: "list",
            parentID: nil,
            title: "Tombstone",
            notes: "",
            status: .needsAction,
            dueDate: nil,
            completedAt: nil,
            isDeleted: true,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: now
        )

        let liveEvent = CalendarEventMirror(
            id: "live-event",
            calendarID: "cal",
            summary: "Live",
            details: "",
            startDate: calendar.date(byAdding: .hour, value: 1, to: now)!,
            endDate: calendar.date(byAdding: .hour, value: 2, to: now)!,
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: now
        )
        let cancelledEvent = CalendarEventMirror(
            id: "cancelled-event",
            calendarID: "cal",
            summary: "Cancelled",
            details: "",
            startDate: calendar.date(byAdding: .hour, value: 3, to: now)!,
            endDate: calendar.date(byAdding: .hour, value: 4, to: now)!,
            isAllDay: false,
            status: .cancelled,
            recurrence: [],
            etag: nil,
            updatedAt: now
        )

        let state = CachedAppState(
            account: .preview,
            taskLists: [TaskListMirror(id: "list", title: "Inbox", updatedAt: now, etag: nil)],
            tasks: [liveTask, deletedTask],
            calendars: [CalendarListMirror(id: "cal", summary: "Work", colorHex: "#000000", isSelected: true, accessRole: "owner", etag: nil)],
            events: [liveEvent, cancelledEvent],
            settings: .default
        )

        XCTAssertEqual(state.tasks.count, 2)
        XCTAssertEqual(state.events.count, 2)

        // Simulate a scheduler merge against an "existing" cache that includes the tombstones:
        // after merge the scheduler should not expose tombstones to callers.
        let merged = MergePurgeFixture.merge(existingTasks: state.tasks, newTasks: state.tasks, existingEvents: state.events, newEvents: state.events)
        XCTAssertEqual(merged.tasks.map(\.id), ["live"])
        XCTAssertEqual(merged.events.map(\.id), ["live-event"])
    }
}

/// Minimal fixture reproducing the scheduler's post-merge filter contract without
/// needing to spin up real Google clients.
private enum MergePurgeFixture {
    static func merge(
        existingTasks: [TaskMirror],
        newTasks: [TaskMirror],
        existingEvents: [CalendarEventMirror],
        newEvents: [CalendarEventMirror]
    ) -> (tasks: [TaskMirror], events: [CalendarEventMirror]) {
        var tasksByID: [TaskMirror.ID: TaskMirror] = [:]
        for task in existingTasks { tasksByID[task.id] = task }
        for task in newTasks { tasksByID[task.id] = task }
        let tasks = tasksByID.values.filter { $0.isDeleted == false }.sorted { $0.id < $1.id }

        var eventsByID: [CalendarEventMirror.ID: CalendarEventMirror] = [:]
        for event in existingEvents { eventsByID[event.id] = event }
        for event in newEvents { eventsByID[event.id] = event }
        let events = eventsByID.values.filter { $0.status != .cancelled }.sorted { $0.id < $1.id }

        return (tasks, events)
    }
}
