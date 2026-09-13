import XCTest
@testable import HotCrossBunsMac

final class TaskDraftTests: XCTestCase {
    private func makeTask(
        id: String = "task-1",
        title: String = "Write report",
        notes: String = "cover Q2 metrics",
        dueDate: Date? = nil
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: "list-1",
            parentID: nil,
            title: title,
            notes: notes,
            status: .needsAction,
            dueDate: dueDate,
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    func testFreshDraftIsClean() {
        let task = makeTask()
        let draft = TaskDraft(task: task)
        XCTAssertFalse(draft.differs(from: task))
    }

    func testTitleChangeMarksDirty() {
        let task = makeTask()
        var draft = TaskDraft(task: task)
        draft.title = "Write report (rev 2)"
        XCTAssertTrue(draft.differs(from: task))
    }

    func testWhitespaceOnlyTitleChangeStaysClean() {
        let task = makeTask()
        var draft = TaskDraft(task: task)
        draft.title = "  Write report  "
        XCTAssertFalse(draft.differs(from: task))
    }

    func testNotesChangeMarksDirty() {
        let task = makeTask()
        var draft = TaskDraft(task: task)
        draft.notes = "cover Q2 metrics and forecast"
        XCTAssertTrue(draft.differs(from: task))
    }

    func testTogglingDueDateOnMarksDirty() {
        let task = makeTask(dueDate: nil)
        var draft = TaskDraft(task: task)
        draft.hasDueDate = true
        draft.dueDate = Date(timeIntervalSince1970: 1_714_608_000)
        XCTAssertTrue(draft.differs(from: task))
    }

    func testTogglingDueDateOffMarksDirty() {
        let base = Date(timeIntervalSince1970: 1_714_608_000)
        let task = makeTask(dueDate: base)
        var draft = TaskDraft(task: task)
        draft.hasDueDate = false
        XCTAssertTrue(draft.differs(from: task))
    }

    func testSameDayDifferentTimeStaysClean() {
        let calendar = Calendar.current
        let original = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 0))!
        let sameDayDifferentMoment = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 17, minute: 32))!
        let task = makeTask(dueDate: original)
        var draft = TaskDraft(task: task)
        draft.dueDate = sameDayDifferentMoment
        XCTAssertFalse(draft.differs(from: task), "Day-only semantics should treat same-day edits as no-op")
    }

    func testResolvedDueDateIsStartOfDay() {
        let calendar = Calendar.current
        let midday = calendar.date(from: DateComponents(year: 2026, month: 5, day: 10, hour: 13, minute: 45))!
        let task = makeTask(dueDate: nil)
        var draft = TaskDraft(task: task)
        draft.hasDueDate = true
        draft.dueDate = midday
        let resolved = draft.resolvedDueDate()
        XCTAssertNotNil(resolved)
        XCTAssertEqual(resolved, calendar.startOfDay(for: midday))
    }

    func testResolvedDueDateNilWhenToggledOff() {
        let task = makeTask(dueDate: Date())
        var draft = TaskDraft(task: task)
        draft.hasDueDate = false
        XCTAssertNil(draft.resolvedDueDate())
    }

    func testHasUsableTitleRejectsWhitespace() {
        let task = makeTask()
        var draft = TaskDraft(task: task)
        draft.title = "   "
        XCTAssertFalse(draft.hasUsableTitle)
        draft.title = "hello"
        XCTAssertTrue(draft.hasUsableTitle)
    }
}
