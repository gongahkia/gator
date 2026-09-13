import CryptoKit
import Foundation

// Detects duplicate tasks / notes (same Google Tasks resource, distinguished
// purely by whether they have a due date). A duplicate group = 2+ incomplete,
// non-deleted tasks sharing exact (case-sensitive) title + notes body. Events
// are intentionally excluded — duplicate events are often legitimate (multi-
// calendar invitations, recurring meeting expansions, draft/final copies).
struct DuplicateIndex: Sendable {
    // groupKey → member IDs. groupKey is a stable hash of the sorted member
    // IDs, so editing a member out of the group (or adding one) changes the
    // key — which in turn invalidates any user-dismissal keyed on that hash.
    let groups: [String: [TaskMirror.ID]]
    // taskID → groupKey. Reverse lookup for the card badge.
    let memberToGroup: [TaskMirror.ID: String]

    static let empty = DuplicateIndex(groups: [:], memberToGroup: [:])

    static func build(tasks: [TaskMirror], dismissedGroupKeys: Set<String>) -> DuplicateIndex {
        // Filter first: active tasks only. Completed + deleted items never flag.
        let active = tasks.filter { $0.isCompleted == false && $0.isDeleted == false }
        // Group by exact (title, notes) tuple. Dictionary keying on a hashable
        // tuple would be nicer but Swift requires us to flatten into a String.
        // Delimiter is a control character unlikely to appear in user text so
        // "A\u{001F}B" and "" + "AB" don't collide.
        var byContent: [String: [TaskMirror.ID]] = [:]
        for task in active {
            let key = "\(task.title)\u{001F}\(task.notes)"
            byContent[key, default: []].append(task.id)
        }
        var groups: [String: [TaskMirror.ID]] = [:]
        var memberToGroup: [TaskMirror.ID: String] = [:]
        for (_, ids) in byContent where ids.count >= 2 {
            let sorted = ids.sorted()
            let hashKey = groupKeyHash(for: sorted)
            if dismissedGroupKeys.contains(hashKey) { continue }
            groups[hashKey] = sorted
            for id in sorted { memberToGroup[id] = hashKey }
        }
        return DuplicateIndex(groups: groups, memberToGroup: memberToGroup)
    }

    // Stable hash of sorted IDs. SHA256 truncated to 16 hex chars is plenty —
    // collisions here only mean two unrelated groups share a dismissal, which
    // self-corrects the next time either group's composition changes.
    static func groupKeyHash(for sortedIDs: [TaskMirror.ID]) -> String {
        let joined = sortedIDs.joined(separator: "|")
        let digest = SHA256.hash(data: Data(joined.utf8))
        return digest.prefix(8).map { String(format: "%02x", $0) }.joined()
    }

    // Siblings of `id` in the same duplicate group, excluding `id` itself.
    func siblings(of id: TaskMirror.ID) -> [TaskMirror.ID] {
        guard let key = memberToGroup[id], let members = groups[key] else { return [] }
        return members.filter { $0 != id }
    }

    func groupKey(for id: TaskMirror.ID) -> String? {
        memberToGroup[id]
    }

    func isMember(_ id: TaskMirror.ID) -> Bool {
        memberToGroup[id] != nil
    }
}
