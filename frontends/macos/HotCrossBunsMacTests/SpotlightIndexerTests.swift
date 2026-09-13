@preconcurrency import CoreSpotlight
import XCTest
@testable import HotCrossBunsMac

final class SpotlightIndexerTests: XCTestCase {
    func testUpdateFiltersDeletedTasksAndCancelledEvents() async {
        let index = FakeSpotlightIndex()
        let indexer = SpotlightIndexer(index: index)

        let task = TaskMirror(
            id: "task-1",
            taskListID: "tasks",
            parentID: nil,
            title: "Ship build",
            notes: "Check release notes",
            status: .needsAction,
            dueDate: Date(timeIntervalSince1970: 1_714_000_000),
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: Date(timeIntervalSince1970: 1_714_000_100)
        )
        let deletedTask = TaskMirror(
            id: "task-2",
            taskListID: "tasks",
            parentID: nil,
            title: "Deleted",
            notes: "",
            status: .needsAction,
            dueDate: nil,
            completedAt: nil,
            isDeleted: true,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
        let event = CalendarEventMirror(
            id: "event-1",
            calendarID: "calendar",
            summary: "Planning review",
            details: "Discuss launch",
            startDate: Date(timeIntervalSince1970: 1_714_003_600),
            endDate: Date(timeIntervalSince1970: 1_714_007_200),
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: Date(timeIntervalSince1970: 1_714_000_200),
            reminderMinutes: [],
            usedDefaultReminders: false,
            location: "Room 1",
            attendeeEmails: ["alice@example.com"],
            attendeeResponses: [],
            meetLink: "https://meet.google.com/aaa-bbbb-ccc"
        )
        let cancelledEvent = CalendarEventMirror(
            id: "event-2",
            calendarID: "calendar",
            summary: "Cancelled",
            details: "",
            startDate: Date(timeIntervalSince1970: 1_714_003_600),
            endDate: Date(timeIntervalSince1970: 1_714_007_200),
            isAllDay: false,
            status: .cancelled,
            recurrence: [],
            etag: nil,
            updatedAt: nil
        )

        await indexer.update(tasks: [task, deletedTask], events: [event, cancelledEvent])

        XCTAssertEqual(index.deletedDomainCalls, [
            [SpotlightIndexer.taskDomain],
            [SpotlightIndexer.eventDomain]
        ])
        XCTAssertEqual(index.deletedIdentifierCalls, [])
        XCTAssertEqual(index.indexedBatches.count, 2)
        XCTAssertEqual(index.indexedBatches[0].count, 1)
        XCTAssertEqual(index.indexedBatches[1].count, 1)

        let taskItem = index.indexedBatches[0][0]
        XCTAssertEqual(taskItem.uniqueIdentifier, SpotlightIndexer.taskURLScheme + "task-1")
        XCTAssertEqual(taskItem.domainIdentifier, SpotlightIndexer.taskDomain)
        XCTAssertEqual(taskItem.attributeSet.title, "Ship build")
        XCTAssertEqual(taskItem.attributeSet.contentDescription, "Check release notes")
        XCTAssertEqual(taskItem.attributeSet.contentURL?.absoluteString, SpotlightIndexer.taskURLScheme + "task-1")

        let eventItem = index.indexedBatches[1][0]
        XCTAssertEqual(eventItem.uniqueIdentifier, SpotlightIndexer.eventURLScheme + "event-1")
        XCTAssertEqual(eventItem.domainIdentifier, SpotlightIndexer.eventDomain)
        XCTAssertEqual(eventItem.attributeSet.title, "Planning review")
        XCTAssertEqual(eventItem.attributeSet.namedLocation, "Room 1")
        XCTAssertEqual(eventItem.attributeSet.recipientEmailAddresses ?? [], ["alice@example.com"])
        XCTAssertTrue(eventItem.attributeSet.keywords?.contains("meet") == true)
        XCTAssertTrue(eventItem.attributeSet.keywords?.contains("meeting") == true)
    }

    func testRemoveAllDeletesBothDomains() async {
        let index = FakeSpotlightIndex()
        let indexer = SpotlightIndexer(index: index)

        await indexer.removeAll()

        XCTAssertEqual(index.deletedDomainCalls, [[SpotlightIndexer.taskDomain, SpotlightIndexer.eventDomain]])
    }

    func testUpdateAfterInitialPrimeIndexesOnlyChangedItemsAndDeletesRemovedItems() async {
        let index = FakeSpotlightIndex()
        let indexer = SpotlightIndexer(index: index)

        let task = TaskMirror(
            id: "task-1",
            taskListID: "tasks",
            parentID: nil,
            title: "Ship build",
            notes: "Check release notes",
            status: .needsAction,
            dueDate: Date(timeIntervalSince1970: 1_714_000_000),
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: Date(timeIntervalSince1970: 1_714_000_100)
        )
        let event = CalendarEventMirror(
            id: "event-1",
            calendarID: "calendar",
            summary: "Planning review",
            details: "Discuss launch",
            startDate: Date(timeIntervalSince1970: 1_714_003_600),
            endDate: Date(timeIntervalSince1970: 1_714_007_200),
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: Date(timeIntervalSince1970: 1_714_000_200),
            reminderMinutes: [],
            usedDefaultReminders: false,
            location: "Room 1"
        )

        await indexer.update(tasks: [task], events: [event])
        index.resetCalls()

        await indexer.update(tasks: [task], events: [event])
        XCTAssertEqual(index.deletedDomainCalls, [])
        XCTAssertEqual(index.deletedIdentifierCalls, [])
        XCTAssertEqual(index.indexedBatches, [])

        var changedTask = task
        changedTask.notes = "Updated release notes"
        await indexer.update(tasks: [changedTask], events: [event])
        XCTAssertEqual(index.deletedDomainCalls, [])
        XCTAssertEqual(index.deletedIdentifierCalls, [])
        XCTAssertEqual(index.indexedBatches.count, 1)
        XCTAssertEqual(index.indexedBatches[0].map(\.uniqueIdentifier), [SpotlightIndexer.taskURLScheme + "task-1"])

        index.resetCalls()
        var cancelledEvent = event
        cancelledEvent.status = .cancelled
        await indexer.update(tasks: [changedTask], events: [cancelledEvent])
        XCTAssertEqual(index.deletedIdentifierCalls, [[SpotlightIndexer.eventURLScheme + "event-1"]])
        XCTAssertEqual(index.indexedBatches, [])
    }

    func testConcurrentUpdatesDuringInitialPrimeDoNotRebuildDomainsTwice() async {
        let gate = AsyncGate()
        let index = FakeSpotlightIndex(firstDomainDeleteGate: gate)
        let indexer = SpotlightIndexer(index: index)

        let task = TaskMirror(
            id: "task-1",
            taskListID: "tasks",
            parentID: nil,
            title: "Ship build",
            notes: "Check release notes",
            status: .needsAction,
            dueDate: Date(timeIntervalSince1970: 1_714_000_000),
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: Date(timeIntervalSince1970: 1_714_000_100)
        )

        async let initialPrime: Void = indexer.update(tasks: [task], events: [])
        await gate.waitUntilPaused()

        var changedTask = task
        changedTask.notes = "Updated release notes"
        async let overlappingUpdate: Void = indexer.update(tasks: [changedTask], events: [])

        await gate.release()
        _ = await (initialPrime, overlappingUpdate)

        XCTAssertEqual(index.deletedDomainCalls, [
            [SpotlightIndexer.taskDomain],
            [SpotlightIndexer.eventDomain]
        ])
        XCTAssertEqual(index.deletedIdentifierCalls, [])
        XCTAssertEqual(index.indexedBatches.map { $0.map(\.uniqueIdentifier) }, [
            [SpotlightIndexer.taskURLScheme + "task-1"],
            [SpotlightIndexer.taskURLScheme + "task-1"]
        ])

        let summary = await indexer.summary()
        XCTAssertEqual(summary.indexedTaskCount, 1)
        XCTAssertFalse(summary.rebuiltDomains)
    }

    func testTaskAndEventTextContentIncludeUsefulPreviewFields() {
        let task = TaskMirror(
            id: "task-1",
            taskListID: "tasks",
            parentID: nil,
            title: "T",
            notes: "notes",
            status: .completed,
            dueDate: Date(timeIntervalSince1970: 1_714_000_000),
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
        let event = CalendarEventMirror(
            id: "event-1",
            calendarID: "calendar",
            summary: "Event",
            details: "Bring notes",
            startDate: Date(timeIntervalSince1970: 1_714_003_600),
            endDate: Date(timeIntervalSince1970: 1_714_007_200),
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            reminderMinutes: [],
            usedDefaultReminders: false,
            location: "HQ",
            attendeeEmails: ["alice@example.com"],
            attendeeResponses: [],
            meetLink: "https://meet.google.com/demo"
        )

        let taskText = SpotlightIndexer.taskTextContent(task)
        XCTAssertTrue(taskText.contains("Due"))
        XCTAssertTrue(taskText.contains("Completed"))
        XCTAssertTrue(taskText.contains("notes"))

        let eventText = SpotlightIndexer.eventTextContent(event)
        XCTAssertTrue(eventText.contains("HQ"))
        XCTAssertTrue(eventText.contains("https://meet.google.com/demo"))
        XCTAssertTrue(eventText.contains("alice@example.com"))
        XCTAssertTrue(eventText.contains("Bring notes"))
    }
}

private final class FakeSpotlightIndex: SpotlightIndexing {
    var deletedDomainCalls: [[String]] = []
    var deletedIdentifierCalls: [[String]] = []
    var indexedBatches: [[CSSearchableItem]] = []

    private let firstDomainDeleteGate: AsyncGate?

    init(firstDomainDeleteGate: AsyncGate? = nil) {
        self.firstDomainDeleteGate = firstDomainDeleteGate
    }

    func deleteSearchableItems(withDomainIdentifiers domainIdentifiers: [String]) async throws {
        deletedDomainCalls.append(domainIdentifiers)
        if deletedDomainCalls.count == 1 {
            await firstDomainDeleteGate?.pauseOnce()
        }
    }

    func deleteSearchableItems(withIdentifiers identifiers: [String]) async throws {
        deletedIdentifierCalls.append(identifiers)
    }

    func indexSearchableItems(_ items: [CSSearchableItem]) async throws {
        indexedBatches.append(items)
    }

    func resetCalls() {
        deletedDomainCalls = []
        deletedIdentifierCalls = []
        indexedBatches = []
    }
}

private actor AsyncGate {
    private var paused = false
    private var waiters: [CheckedContinuation<Void, Never>] = []
    private var releaseContinuation: CheckedContinuation<Void, Never>?

    func pauseOnce() async {
        guard paused == false else { return }
        paused = true
        waiters.forEach { $0.resume() }
        waiters = []
        await withCheckedContinuation { continuation in
            releaseContinuation = continuation
        }
    }

    func waitUntilPaused() async {
        if paused { return }
        await withCheckedContinuation { continuation in
            waiters.append(continuation)
        }
    }

    func release() {
        releaseContinuation?.resume()
        releaseContinuation = nil
    }
}
