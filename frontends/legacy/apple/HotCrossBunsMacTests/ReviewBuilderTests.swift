import XCTest
@testable import HotCrossBunsMac

final class ReviewBuilderTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18))!
    }

    private func day(_ offset: Int) -> Date {
        calendar.date(byAdding: .day, value: offset, to: now)!
    }

    private func list(_ id: String, title: String, updated: Date? = nil) -> TaskListMirror {
        TaskListMirror(id: id, title: title, updatedAt: updated, etag: nil)
    }

    private func task(
        id: String,
        list: String,
        updated: Date? = nil,
        completed: Bool = false,
        deleted: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: list,
            parentID: nil,
            title: id,
            notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: nil,
            completedAt: nil,
            isDeleted: deleted,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: updated
        )
    }

    func testSortsOldestFirst() {
        let lists = [list("fresh", title: "Fresh"), list("stale", title: "Stale")]
        let tasks = [
            task(id: "a", list: "fresh", updated: day(-1)),
            task(id: "b", list: "stale", updated: day(-30))
        ]
        let result = ReviewBuilder.build(
            taskLists: lists,
            tasks: tasks,
            visibleTaskListIDs: ["fresh", "stale"],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(result.map(\.taskList.id), ["stale", "fresh"])
    }

    func testDaysSinceActivityComputed() {
        let lists = [list("l", title: "L")]
        let tasks = [task(id: "a", list: "l", updated: day(-5))]
        let result = ReviewBuilder.build(
            taskLists: lists,
            tasks: tasks,
            visibleTaskListIDs: ["l"],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(result.first?.daysSinceActivity, 5)
    }

    func testDeletedTasksExcluded() {
        let lists = [list("l", title: "L")]
        let tasks = [
            task(id: "a", list: "l", updated: day(-1), deleted: true),
            task(id: "b", list: "l", updated: day(-2))
        ]
        let result = ReviewBuilder.build(
            taskLists: lists,
            tasks: tasks,
            visibleTaskListIDs: ["l"],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(result.first?.openTasks.map(\.id), ["b"])
    }

    func testOpenTasksExcludeCompleted() {
        let lists = [list("l", title: "L")]
        let tasks = [
            task(id: "done", list: "l", updated: day(-2), completed: true),
            task(id: "open", list: "l", updated: day(-2))
        ]
        let result = ReviewBuilder.build(
            taskLists: lists,
            tasks: tasks,
            visibleTaskListIDs: ["l"],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(result.first?.openTasks.map(\.id), ["open"])
    }

    func testInvisibleListsAreSkipped() {
        let lists = [list("l1", title: "L1"), list("l2", title: "L2")]
        let tasks = [task(id: "a", list: "l1", updated: day(-1))]
        let result = ReviewBuilder.build(
            taskLists: lists,
            tasks: tasks,
            visibleTaskListIDs: ["l1"],
            referenceDate: now,
            calendar: calendar
        )
        XCTAssertEqual(result.map(\.taskList.id), ["l1"])
    }
}
