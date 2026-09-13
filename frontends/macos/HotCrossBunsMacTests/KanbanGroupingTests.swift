import XCTest
@testable import HotCrossBunsMac

final class KanbanGroupingTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 10))!
    }

    private func day(_ offset: Int) -> Date {
        calendar.date(byAdding: .day, value: offset, to: calendar.startOfDay(for: now))!
    }

    private func task(
        id: String,
        title: String = "task",
        list: String = "L1",
        due: Date? = nil,
        completed: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id, taskListID: list, parentID: nil,
            title: title, notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: due, completedAt: nil,
            isDeleted: false, isHidden: false,
            position: nil, etag: nil, updatedAt: nil
        )
    }

    private let lists: [TaskListMirror] = [
        TaskListMirror(id: "L1", title: "Work"),
        TaskListMirror(id: "L2", title: "Home")
    ]

    private func group(_ tasks: [TaskMirror], mode: KanbanColumnMode) -> [KanbanColumn] {
        KanbanGrouping.columns(for: tasks, mode: mode, taskLists: lists, now: now, calendar: calendar)
    }

    // MARK: - by list

    func testByListOneColumnPerList() {
        let cols = group([], mode: .byList)
        XCTAssertEqual(cols.count, 2)
        XCTAssertEqual(cols.map(\.title), ["Work", "Home"])
        // Every list column is a drop target.
        XCTAssertTrue(cols.allSatisfy { $0.dropIntent != nil })
    }

    func testByListBucketsTasks() {
        let t1 = task(id: "a", list: "L1")
        let t2 = task(id: "b", list: "L2")
        let t3 = task(id: "c", list: "L1")
        let cols = group([t1, t2, t3], mode: .byList)
        XCTAssertEqual(cols.first(where: { $0.title == "Work" })?.tasks.count, 2)
        XCTAssertEqual(cols.first(where: { $0.title == "Home" })?.tasks.count, 1)
    }

    // MARK: - by due bucket

    func testByDueBucketBuckets() {
        let overdueTask = task(id: "a", due: day(-2))
        let todayTask = task(id: "b", due: day(0))
        let thisWeekTask = task(id: "c", due: day(3))
        let laterTask = task(id: "d", due: day(30))
        let noDateTask = task(id: "e", due: nil)

        let cols = group([overdueTask, todayTask, thisWeekTask, laterTask, noDateTask], mode: .byDueBucket)
        XCTAssertEqual(cols.count, 5)
        XCTAssertEqual(cols[0].title, "Overdue"); XCTAssertEqual(cols[0].tasks.map(\.id), ["a"])
        XCTAssertEqual(cols[1].title, "Today"); XCTAssertEqual(cols[1].tasks.map(\.id), ["b"])
        XCTAssertEqual(cols[2].title, "This week"); XCTAssertEqual(cols[2].tasks.map(\.id), ["c"])
        XCTAssertEqual(cols[3].title, "Later"); XCTAssertEqual(cols[3].tasks.map(\.id), ["d"])
        XCTAssertEqual(cols[4].title, "No date"); XCTAssertEqual(cols[4].tasks.map(\.id), ["e"])
    }

    func testByDueBucketNoDateColumnClearsDueOnDrop() {
        let cols = group([], mode: .byDueBucket)
        let noDate = cols.first(where: { $0.title == "No date" })!
        // "No date" column drops must clear the due date.
        if case .setDue(let date) = noDate.dropIntent! {
            XCTAssertNil(date)
        } else {
            XCTFail("Expected setDue intent")
        }
    }

    func testByDueBucketTodayColumnSetsToday() {
        let cols = group([], mode: .byDueBucket)
        let today = cols.first(where: { $0.title == "Today" })!
        if case .setDue(let date) = today.dropIntent! {
            XCTAssertEqual(date, calendar.startOfDay(for: now))
        } else {
            XCTFail("Expected setDue intent")
        }
    }

    // MARK: - by tag

    func testByTagCreatesColumnPerTag() {
        let t1 = task(id: "a", title: "fix bug #work #urgent")
        let t2 = task(id: "b", title: "read book #personal")
        let t3 = task(id: "c", title: "plain task")
        let cols = group([t1, t2, t3], mode: .byTag)
        // personal, urgent, work (alpha sort) + Untagged
        XCTAssertEqual(cols.count, 4)
        XCTAssertEqual(cols[0].title, "#personal")
        XCTAssertEqual(cols[1].title, "#urgent")
        XCTAssertEqual(cols[2].title, "#work")
        XCTAssertEqual(cols[3].title, "Untagged")
        XCTAssertEqual(cols[3].tasks.map(\.id), ["c"])
    }

    func testByTagUntaggedColumnHasNoDropIntent() {
        let cols = group([task(id: "a")], mode: .byTag)
        let untagged = cols.first(where: { $0.title == "Untagged" })!
        XCTAssertNil(untagged.dropIntent)
    }

    // MARK: - operation derivation

    func testDropIntentToOperationMoveToList() {
        let op = KanbanDropIntent.moveToList(listId: "L2").operation(for: "t1")
        XCTAssertEqual(op, .moveToList(taskId: "t1", targetListId: "L2"))
    }

    func testDropIntentToOperationSetDue() {
        let op = KanbanDropIntent.setDue(date: day(3)).operation(for: "t1")
        XCTAssertEqual(op, .setDue(taskId: "t1", dueDate: day(3)))
    }

    func testDropIntentToOperationAddRemoveTag() {
        XCTAssertEqual(KanbanDropIntent.setTag(add: "work", remove: nil).operation(for: "t"),
                       .addTag(taskId: "t", tag: "work"))
        XCTAssertEqual(KanbanDropIntent.setTag(add: nil, remove: "work").operation(for: "t"),
                       .removeTag(taskId: "t", tag: "work"))
    }

    // MARK: - invariants

    func testDeletedTasksExcluded() {
        let live = task(id: "a")
        var deleted = task(id: "b")
        deleted.isDeleted = true
        let cols = group([live, deleted], mode: .byTag)
        let total = cols.reduce(0) { $0 + $1.tasks.count }
        XCTAssertEqual(total, 1)
    }

    func testTaskBoardSnapshotSeparatesDatedAndUndatedTasks() {
        let datedVisible = task(id: "dated-visible", list: "L1", due: day(1))
        let datedHiddenList = task(id: "dated-hidden-list", list: "L2", due: day(1))
        let noteVisible = task(id: "note-visible", list: "L2")
        let noteHiddenList = task(id: "note-hidden-list", list: "L1")

        var settings = AppSettings.default
        settings.hasConfiguredTasksTabSelection = true
        settings.tasksTabSelectedListIDs = ["L1"]
        settings.hasConfiguredNotesTabSelection = true
        settings.notesTabSelectedListIDs = ["L2"]

        let snapshot = TaskBoardSnapshot.build(
            tasks: [datedVisible, datedHiddenList, noteVisible, noteHiddenList],
            tasksTabVisibleListIDs: settings.tasksTabSelectedListIDs,
            notesTabVisibleListIDs: settings.notesTabSelectedListIDs,
            settings: settings,
            referenceDate: now,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.datedTasks.map(\.id), ["dated-visible"])
        XCTAssertEqual(snapshot.undatedTasks.map(\.id), ["note-visible"])
    }

    func testTaskBoardSnapshotTreatsDatedTasksWithNoteBodiesAsNotesSurfaceItems() {
        var datedWithNotes = task(id: "dated-with-notes", list: "L1", due: day(1))
        datedWithNotes.notes = "Meeting context"
        let datedWithoutNotes = task(id: "dated-without-notes", list: "L1", due: day(2))
        let standaloneNote = task(id: "standalone-note", list: "L1")

        let snapshot = TaskBoardSnapshot.build(
            tasks: [datedWithNotes, datedWithoutNotes, standaloneNote],
            tasksTabVisibleListIDs: ["L1"],
            notesTabVisibleListIDs: ["L1"],
            settings: .default,
            referenceDate: now,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.datedTasks.map(\.id), ["dated-with-notes", "dated-without-notes"])
        XCTAssertEqual(snapshot.undatedTasks.map(\.id), ["dated-with-notes", "standalone-note"])
    }

    func testTaskBoardSnapshotHidesOverdueTasksWhenConfigured() {
        let overdue = task(id: "overdue", list: "L1", due: day(-1))
        let today = task(id: "today", list: "L1", due: day(0))

        var settings = AppSettings.default
        settings.overdueTaskBehavior = .hide

        let snapshot = TaskBoardSnapshot.build(
            tasks: [overdue, today],
            tasksTabVisibleListIDs: ["L1"],
            notesTabVisibleListIDs: ["L1"],
            settings: settings,
            referenceDate: now,
            calendar: calendar
        )

        XCTAssertEqual(snapshot.datedTasks.map(\.id), ["today"])
    }
}
