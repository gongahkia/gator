import Foundation

struct TaskHierarchyNode: Equatable, Identifiable, Sendable {
    let parent: TaskMirror
    let children: [TaskMirror]

    var id: TaskMirror.ID { parent.id }
}

enum TaskHierarchy {
    static let maxDepth = 1

    static func build(tasks: [TaskMirror]) -> [TaskHierarchyNode] {
        let visible = tasks.filter { $0.isDeleted == false }
        var childrenByParent: [TaskMirror.ID: [TaskMirror]] = [:]
        var roots: [TaskMirror] = []

        let knownIDs = Set(visible.map(\.id))
        for task in visible {
            if let parentID = task.parentID, knownIDs.contains(parentID) {
                childrenByParent[parentID, default: []].append(task)
            } else {
                roots.append(task)
            }
        }

        let sortedRoots = sortByPosition(roots)
        return sortedRoots.map { root in
            TaskHierarchyNode(parent: root, children: sortByPosition(childrenByParent[root.id] ?? []))
        }
    }

    static func canIndent(_ task: TaskMirror, within tasks: [TaskMirror]) -> Bool {
        guard task.parentID == nil else { return false }
        guard hasChildren(task.id, in: tasks) == false else { return false }
        return precedingSibling(of: task, in: tasks) != nil
    }

    static func canOutdent(_ task: TaskMirror) -> Bool {
        task.parentID != nil
    }

    static func precedingSibling(of task: TaskMirror, in tasks: [TaskMirror]) -> TaskMirror? {
        let siblings = tasks.filter { $0.taskListID == task.taskListID && $0.parentID == task.parentID && $0.isDeleted == false }
        let sorted = sortByPosition(siblings)
        guard let index = sorted.firstIndex(where: { $0.id == task.id }), index > 0 else { return nil }
        return sorted[index - 1]
    }

    static func hasChildren(_ parentID: TaskMirror.ID, in tasks: [TaskMirror]) -> Bool {
        tasks.contains { $0.parentID == parentID && $0.isDeleted == false }
    }

    static func sortByPosition(_ tasks: [TaskMirror]) -> [TaskMirror] {
        tasks.sorted { lhs, rhs in
            switch (lhs.position, rhs.position) {
            case let (l?, r?) where l != r:
                return l < r
            default:
                return lhs.id < rhs.id
            }
        }
    }
}
