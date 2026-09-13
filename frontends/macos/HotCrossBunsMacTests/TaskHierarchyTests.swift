import XCTest
@testable import HotCrossBunsMac

final class TaskHierarchyTests: XCTestCase {
    private func task(
        id: String,
        parent: String? = nil,
        position: String? = nil,
        list: String = "L1",
        deleted: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: list,
            parentID: parent,
            title: id,
            notes: "",
            status: .needsAction,
            dueDate: nil,
            completedAt: nil,
            isDeleted: deleted,
            isHidden: false,
            position: position,
            etag: nil,
            updatedAt: nil
        )
    }

    func testBuildGroupsChildrenUnderParents() {
        let tasks = [
            task(id: "a", position: "0001"),
            task(id: "a1", parent: "a", position: "0001"),
            task(id: "a2", parent: "a", position: "0002"),
            task(id: "b", position: "0002")
        ]
        let nodes = TaskHierarchy.build(tasks: tasks)
        XCTAssertEqual(nodes.map(\.parent.id), ["a", "b"])
        XCTAssertEqual(nodes[0].children.map(\.id), ["a1", "a2"])
        XCTAssertTrue(nodes[1].children.isEmpty)
    }

    func testOrphanChildrenPromoteToRoots() {
        let tasks = [
            task(id: "a", position: "0001"),
            task(id: "orphan", parent: "ghost", position: "0005")
        ]
        let nodes = TaskHierarchy.build(tasks: tasks)
        XCTAssertEqual(Set(nodes.map(\.parent.id)), ["a", "orphan"])
    }

    func testDeletedTasksExcluded() {
        let tasks = [
            task(id: "a", position: "0001"),
            task(id: "b", position: "0002", deleted: true),
            task(id: "a1", parent: "a", position: "0001"),
            task(id: "a2", parent: "a", position: "0002", deleted: true)
        ]
        let nodes = TaskHierarchy.build(tasks: tasks)
        XCTAssertEqual(nodes.map(\.parent.id), ["a"])
        XCTAssertEqual(nodes[0].children.map(\.id), ["a1"])
    }

    func testSortByPositionFallsBackToID() {
        let tasks = [
            task(id: "b", position: nil),
            task(id: "a", position: nil)
        ]
        let sorted = TaskHierarchy.sortByPosition(tasks)
        XCTAssertEqual(sorted.map(\.id), ["a", "b"])
    }

    func testCanIndentRequiresPrecedingSibling() {
        let first = task(id: "a", position: "0001")
        let second = task(id: "b", position: "0002")
        XCTAssertFalse(TaskHierarchy.canIndent(first, within: [first, second]))
        XCTAssertTrue(TaskHierarchy.canIndent(second, within: [first, second]))
    }

    func testCanIndentRejectsTaskWithChildren() {
        let predecessor = task(id: "p", position: "0000")
        let parent = task(id: "a", position: "0001")
        let child = task(id: "a1", parent: "a", position: "0001")
        // parent 'a' has a preceding sibling but its own children — allowing indent would push grandchildren to depth 2
        XCTAssertFalse(TaskHierarchy.canIndent(parent, within: [predecessor, parent, child]))
    }

    func testCanIndentRejectsAlreadyChild() {
        let root = task(id: "a", position: "0001")
        let sub = task(id: "a1", parent: "a", position: "0001")
        XCTAssertFalse(TaskHierarchy.canIndent(sub, within: [root, sub]))
    }

    func testCanOutdentDetectsChildStatus() {
        XCTAssertTrue(TaskHierarchy.canOutdent(task(id: "a1", parent: "a")))
        XCTAssertFalse(TaskHierarchy.canOutdent(task(id: "a", parent: nil)))
    }

    func testPrecedingSiblingRespectsTaskListAndParent() {
        let tasks = [
            task(id: "a", position: "0001"),
            task(id: "b", position: "0002"),
            task(id: "c", position: "0003", list: "OtherList")
        ]
        let preceding = TaskHierarchy.precedingSibling(of: tasks[1], in: tasks)
        XCTAssertEqual(preceding?.id, "a")
        XCTAssertNil(TaskHierarchy.precedingSibling(of: tasks[2], in: tasks))
    }
}
