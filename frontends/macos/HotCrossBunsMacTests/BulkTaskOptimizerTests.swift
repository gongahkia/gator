import XCTest
@testable import HotCrossBunsMac

final class BulkTaskOptimizerTests: XCTestCase {
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
            id: id,
            taskListID: list,
            parentID: nil,
            title: title,
            notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    // MARK: - empty / trivial

    func testEmptyInput() {
        let result = BulkTaskOptimizer.optimize([], currentTasks: [], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 0)
    }

    func testUnknownTaskDropsAllOps() {
        let ops: [BulkTaskOperation] = [
            .complete(taskId: "ghost"),
            .setDue(taskId: "ghost", dueDate: day(1))
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 2)
    }

    // MARK: - delete dominates

    func testDeleteDropsAllOtherOpsForTask() {
        let t = task(id: "a", completed: false)
        let ops: [BulkTaskOperation] = [
            .complete(taskId: "a"),
            .setDue(taskId: "a", dueDate: day(1)),
            .delete(taskId: "a"),
            .addTag(taskId: "a", tag: "work")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations, [.delete(taskId: "a")])
        XCTAssertEqual(result.droppedCount, 3)
    }

    // MARK: - completion last-wins

    func testCompleteThenReopenCancelsIfAlreadyReopen() {
        let t = task(id: "a", completed: false)
        let ops: [BulkTaskOperation] = [
            .complete(taskId: "a"),
            .reopen(taskId: "a")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        // Last-wins is reopen, current is already reopened → no-op → dropped.
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 2)
    }

    func testReopenThenCompleteAppliesIfCurrentlyOpen() {
        let t = task(id: "a", completed: false)
        let ops: [BulkTaskOperation] = [
            .reopen(taskId: "a"),
            .complete(taskId: "a")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations, [.complete(taskId: "a")])
        XCTAssertEqual(result.droppedCount, 1)
    }

    func testCompleteAlreadyCompletedIsDropped() {
        let t = task(id: "a", completed: true)
        let result = BulkTaskOptimizer.optimize([.complete(taskId: "a")], currentTasks: [t], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 1)
    }

    // MARK: - setDue coalesce + no-op

    func testRepeatedSetDueKeepsLast() {
        let t = task(id: "a", due: day(0))
        let ops: [BulkTaskOperation] = [
            .setDue(taskId: "a", dueDate: day(1)),
            .setDue(taskId: "a", dueDate: day(5)),
            .setDue(taskId: "a", dueDate: day(3))
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations, [.setDue(taskId: "a", dueDate: day(3))])
        XCTAssertEqual(result.droppedCount, 2)
    }

    func testSetDueSameDayIsDropped() {
        let t = task(id: "a", due: day(3))
        // Different time-of-day but same calendar day → no-op.
        let sameDay = calendar.date(byAdding: .hour, value: 17, to: day(3))!
        let result = BulkTaskOptimizer.optimize([.setDue(taskId: "a", dueDate: sameDay)], currentTasks: [t], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 1)
    }

    func testClearDueOnAlreadyNoneIsDropped() {
        let t = task(id: "a", due: nil)
        let result = BulkTaskOptimizer.optimize([.setDue(taskId: "a", dueDate: nil)], currentTasks: [t], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 1)
    }

    // MARK: - move list

    func testMoveToCurrentListIsDropped() {
        let t = task(id: "a", list: "L1")
        let result = BulkTaskOptimizer.optimize(
            [.moveToList(taskId: "a", targetListId: "L1")],
            currentTasks: [t],
            calendar: calendar
        )
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 1)
    }

    func testRepeatedMoveCoalesces() {
        let t = task(id: "a", list: "L1")
        let ops: [BulkTaskOperation] = [
            .moveToList(taskId: "a", targetListId: "L2"),
            .moveToList(taskId: "a", targetListId: "L3")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations, [.moveToList(taskId: "a", targetListId: "L3")])
        XCTAssertEqual(result.droppedCount, 1)
    }

    // MARK: - tags

    func testAddRemoveSameTagNetsZeroWhenAbsent() {
        let t = task(id: "a", title: "plain")
        let ops: [BulkTaskOperation] = [
            .addTag(taskId: "a", tag: "work"),
            .removeTag(taskId: "a", tag: "work")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        // Last-wins for "work" is remove. Tag is currently absent → remove is no-op → drop.
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 2)
    }

    func testAddTagAlreadyPresentDropped() {
        let t = task(id: "a", title: "do it #work")
        let result = BulkTaskOptimizer.optimize([.addTag(taskId: "a", tag: "work")], currentTasks: [t], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty)
        XCTAssertEqual(result.droppedCount, 1)
    }

    func testRemoveTagWhenPresentKept() {
        let t = task(id: "a", title: "do it #work")
        let result = BulkTaskOptimizer.optimize([.removeTag(taskId: "a", tag: "work")], currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations, [.removeTag(taskId: "a", tag: "work")])
        XCTAssertEqual(result.droppedCount, 0)
    }

    func testTagCaseInsensitive() {
        let t = task(id: "a", title: "x #WORK")
        let result = BulkTaskOptimizer.optimize([.addTag(taskId: "a", tag: "work")], currentTasks: [t], calendar: calendar)
        XCTAssertTrue(result.operations.isEmpty, "case-insensitive duplicate tag should be dropped")
    }

    // MARK: - multi-task batch

    func testCrossTaskOpsAreIndependent() {
        let t1 = task(id: "a", list: "L1", completed: false)
        let t2 = task(id: "b", list: "L1", completed: true)
        let ops: [BulkTaskOperation] = [
            .complete(taskId: "a"),
            .complete(taskId: "b"), // already completed — drop
            .delete(taskId: "b")   // delete dominates for b
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t1, t2], calendar: calendar)
        XCTAssertTrue(result.operations.contains(.complete(taskId: "a")))
        XCTAssertTrue(result.operations.contains(.delete(taskId: "b")))
        XCTAssertEqual(result.operations.count, 2)
        XCTAssertEqual(result.droppedCount, 1)
    }

    func testDeterministicOutputOrder() {
        // Two tasks, same mix of ops — expect a stable ordering.
        let t1 = task(id: "a")
        let t2 = task(id: "b")
        let ops: [BulkTaskOperation] = [
            .addTag(taskId: "b", tag: "x"),
            .complete(taskId: "a"),
            .setDue(taskId: "b", dueDate: day(1)),
            .moveToList(taskId: "a", targetListId: "L2")
        ]
        let a = BulkTaskOptimizer.optimize(ops, currentTasks: [t1, t2], calendar: calendar)
        let b = BulkTaskOptimizer.optimize(ops, currentTasks: [t1, t2], calendar: calendar)
        XCTAssertEqual(a.operations, b.operations)
        // Ordering: a_01 (move), a_02 (complete), b_03 (setDue), b_05 (addTag)
        XCTAssertEqual(a.operations, [
            .moveToList(taskId: "a", targetListId: "L2"),
            .complete(taskId: "a"),
            .setDue(taskId: "b", dueDate: day(1)),
            .addTag(taskId: "b", tag: "x")
        ])
    }

    // MARK: - safety

    func testNoOpsEverExpandsInputCount() {
        // Invariant: optimized count + dropped == input count.
        let t = task(id: "a", due: day(0))
        let ops: [BulkTaskOperation] = [
            .complete(taskId: "a"),
            .reopen(taskId: "a"),
            .setDue(taskId: "a", dueDate: day(0)),
            .setDue(taskId: "a", dueDate: day(5)),
            .addTag(taskId: "a", tag: "x"),
            .removeTag(taskId: "a", tag: "x"),
            .moveToList(taskId: "a", targetListId: "L1")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations.count + result.droppedCount, ops.count)
    }

    func testDeleteAlsoRespectsInvariant() {
        let t = task(id: "a")
        let ops: [BulkTaskOperation] = [
            .complete(taskId: "a"),
            .setDue(taskId: "a", dueDate: day(1)),
            .delete(taskId: "a")
        ]
        let result = BulkTaskOptimizer.optimize(ops, currentTasks: [t], calendar: calendar)
        XCTAssertEqual(result.operations.count + result.droppedCount, ops.count)
    }

    // MARK: - execution result helpers

    func testBulkTaskExecutionResultFlags() {
        let empty = BulkTaskExecutionResult(submitted: 0, succeeded: 0, failures: [], droppedAsNoOp: 0)
        XCTAssertTrue(empty.nothingToDo)
        XCTAssertFalse(empty.allSucceeded)

        let success = BulkTaskExecutionResult(submitted: 3, succeeded: 3, failures: [], droppedAsNoOp: 1)
        XCTAssertTrue(success.allSucceeded)
        XCTAssertFalse(success.nothingToDo)

        let partial = BulkTaskExecutionResult(
            submitted: 3,
            succeeded: 2,
            failures: [BulkTaskFailure(operation: .complete(taskId: "x"), message: "nope")],
            droppedAsNoOp: 0
        )
        XCTAssertFalse(partial.allSucceeded)
        XCTAssertEqual(partial.failedCount, 1)
    }
}
