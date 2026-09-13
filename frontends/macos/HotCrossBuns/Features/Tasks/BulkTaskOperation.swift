import Foundation

// Typed operations the bulk-action surface can queue. The optimizer normalises
// a user-issued batch into a minimal set of Google Tasks API calls; the AppModel
// executor then dispatches each through the existing optimistic-write paths
// (setTaskCompleted, updateTask, deleteTask, moveTaskToList).
//
// Why a typed intermediate representation rather than ad-hoc loops:
//  - Dedup: `addTag("foo")` + `removeTag("foo")` on the same task nets to zero.
//  - Coalesce: two `setDue` on the same task collapse to the last-wins.
//  - Short-circuit no-ops: `complete` on an already-completed task is dropped
//    before it wastes a request against the Google quota.
//  - Testable without Google network mocks — the optimizer is pure.
enum BulkTaskOperation: Equatable, Hashable, Sendable {
    case complete(taskId: String)
    case reopen(taskId: String)
    case delete(taskId: String)
    case setDue(taskId: String, dueDate: Date?) // nil = clear due date
    case moveToList(taskId: String, targetListId: String)
    case addTag(taskId: String, tag: String)
    case removeTag(taskId: String, tag: String)

    var taskId: String {
        switch self {
        case .complete(let id), .reopen(let id), .delete(let id): return id
        case .setDue(let id, _), .moveToList(let id, _): return id
        case .addTag(let id, _), .removeTag(let id, _): return id
        }
    }

    // Human-readable label for partial-failure messages.
    var summary: String {
        switch self {
        case .complete: return "complete"
        case .reopen: return "reopen"
        case .delete: return "delete"
        case .setDue(_, let d): return d == nil ? "clear due" : "set due"
        case .moveToList: return "move list"
        case .addTag(_, let t): return "+#\(t)"
        case .removeTag(_, let t): return "−#\(t)"
        }
    }
}

struct BulkTaskOptimizeResult: Equatable, Sendable {
    let operations: [BulkTaskOperation]
    // Count of original ops that the optimizer removed because they were
    // redundant, no-ops against current state, or cancelled by a later op.
    let droppedCount: Int
}

enum BulkTaskOptimizer {
    // Reduces a user-issued batch of operations to the minimum number of
    // Google API calls needed to realise the user's final intent.
    //
    // Invariants:
    //  - For the same task, `.delete` dominates (everything else for that task
    //    is dropped; `.delete` moves to the head of that task's subsequence).
    //  - Per scalar dimension (completion, due, list), last-wins.
    //  - Tag add+remove of the same tag net to zero; repeated add/remove
    //    collapse to last-wins.
    //  - No-op against current state is dropped: completing an already-completed
    //    task, setting due to the same day, adding a tag that already exists,
    //    removing a tag that doesn't.
    //  - Ops whose taskId is missing from `currentTasks` are dropped — no
    //    phantom API calls against deleted / unmirrored tasks.
    //  - Output order is deterministic (by taskId, then op kind) so tests and
    //    replay paths can rely on it.
    static func optimize(
        _ ops: [BulkTaskOperation],
        currentTasks: [TaskMirror],
        calendar: Calendar = .current
    ) -> BulkTaskOptimizeResult {
        guard ops.isEmpty == false else {
            return BulkTaskOptimizeResult(operations: [], droppedCount: 0)
        }

        var byTask: [String: [BulkTaskOperation]] = [:]
        for op in ops {
            byTask[op.taskId, default: []].append(op)
        }

        var kept: [BulkTaskOperation] = []
        var dropped = 0

        for (taskId, taskOps) in byTask {
            guard let current = currentTasks.first(where: { $0.id == taskId }) else {
                // Unknown / deleted task — drop everything.
                dropped += taskOps.count
                continue
            }

            if taskOps.contains(where: { if case .delete = $0 { return true } else { return false } }) {
                kept.append(.delete(taskId: taskId))
                dropped += taskOps.count - 1
                continue
            }

            let reduced = reduce(taskOps, current: current, calendar: calendar)
            kept.append(contentsOf: reduced.kept)
            dropped += reduced.droppedCount
        }

        kept.sort(by: { orderKey($0) < orderKey($1) })
        return BulkTaskOptimizeResult(operations: kept, droppedCount: dropped)
    }

    private struct Reduction {
        var kept: [BulkTaskOperation]
        var droppedCount: Int
    }

    private static func reduce(
        _ ops: [BulkTaskOperation],
        current: TaskMirror,
        calendar: Calendar
    ) -> Reduction {
        var finalCompleted: Bool?
        var finalDue: (set: Bool, date: Date?) = (false, nil)
        var finalMoveList: String?
        var tagIntents: [String: Bool] = [:] // tag (lowercased) → true=add, false=remove; last wins

        for op in ops {
            switch op {
            case .complete: finalCompleted = true
            case .reopen: finalCompleted = false
            case .delete: break // handled by caller
            case .setDue(_, let d): finalDue = (true, d)
            case .moveToList(_, let target): finalMoveList = target
            case .addTag(_, let t): tagIntents[t.lowercased()] = true
            case .removeTag(_, let t): tagIntents[t.lowercased()] = false
            }
        }

        var kept: [BulkTaskOperation] = []
        var dropped = 0
        let originalCount = ops.count

        if let want = finalCompleted {
            if current.isCompleted == want {
                // no-op — drop
            } else {
                kept.append(want ? .complete(taskId: current.id) : .reopen(taskId: current.id))
            }
        }
        if finalDue.set {
            let currentDay = current.dueDate.map { calendar.startOfDay(for: $0) }
            let targetDay = finalDue.date.map { calendar.startOfDay(for: $0) }
            if currentDay == targetDay {
                // no-op — drop
            } else {
                kept.append(.setDue(taskId: current.id, dueDate: finalDue.date))
            }
        }
        if let target = finalMoveList {
            if current.taskListID == target {
                // no-op — drop
            } else {
                kept.append(.moveToList(taskId: current.id, targetListId: target))
            }
        }

        let currentTags = Set(TagExtractor.tags(in: current.title).map { $0.lowercased() })
        for (tag, wantAdd) in tagIntents {
            let present = currentTags.contains(tag)
            if wantAdd && present { continue }         // no-op
            if !wantAdd && !present { continue }       // no-op
            kept.append(wantAdd ? .addTag(taskId: current.id, tag: tag) : .removeTag(taskId: current.id, tag: tag))
        }

        dropped = originalCount - kept.count
        return Reduction(kept: kept, droppedCount: dropped)
    }

    private static func orderKey(_ op: BulkTaskOperation) -> String {
        let rank: Int
        switch op {
        case .delete: rank = 0
        case .moveToList: rank = 1
        case .complete, .reopen: rank = 2
        case .setDue: rank = 3
        case .addTag: rank = 4
        case .removeTag: rank = 5
        }
        // Pad rank to preserve numeric ordering as string sort.
        return "\(op.taskId)_\(String(format: "%02d", rank))"
    }
}

// Executor result — returned by AppModel.performBulkTaskOperations so the
// caller can surface a "N updated · M failed · K skipped" toast.
struct BulkTaskFailure: Equatable, Sendable {
    let operation: BulkTaskOperation
    let message: String
}

struct BulkTaskExecutionResult: Equatable, Sendable {
    let submitted: Int
    let succeeded: Int
    let failures: [BulkTaskFailure]
    let droppedAsNoOp: Int

    var failedCount: Int { failures.count }
    var allSucceeded: Bool { failures.isEmpty && submitted > 0 }
    var nothingToDo: Bool { submitted == 0 && droppedAsNoOp == 0 }
}
