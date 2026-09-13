import XCTest
@testable import HotCrossBunsMac

final class CommandPaletteEntityCacheTests: XCTestCase {
    private let baseDate = Date(timeIntervalSince1970: 1_800_000_000)

    func testSearchRebuildsWhenSnapshotKeyChanges() async throws {
        let cache = CommandPaletteEntityCache()
        let lists = [TaskListMirror(id: "list-1", title: "Work")]
        let calendars = [calendar(id: "calendar-1", summary: "Primary")]

        let first = CommandPaletteEntitySnapshot(
            key: "rev-1",
            tasks: [task(id: "task-1", title: "Original task", listID: "list-1")],
            events: [],
            taskLists: lists,
            calendars: calendars,
            customFilters: []
        )
        let firstResult = try await cache.search(snapshot: first, query: "renamed")
        XCTAssertTrue(firstResult.entities.isEmpty)

        let second = CommandPaletteEntitySnapshot(
            key: "rev-2",
            tasks: [task(id: "task-1", title: "Renamed target task", listID: "list-1")],
            events: [],
            taskLists: lists,
            calendars: calendars,
            customFilters: []
        )
        let secondResult = try await cache.search(snapshot: second, query: "renamed")

        XCTAssertEqual(secondResult.snapshotKey, "rev-2")
        XCTAssertTrue(secondResult.entities.contains { entity in
            if case .task(let task) = entity {
                return task.id == "task-1"
            }
            return false
        })
    }

    func testLargeSyntheticIndexSearchBenchmark() async throws {
        let cache = CommandPaletteEntityCache()
        let lists = (0..<12).map { TaskListMirror(id: "list-\($0)", title: "List \($0)") }
        let calendars = (0..<8).map { calendar(id: "calendar-\($0)", summary: "Calendar \($0)") }

        var tasks: [TaskMirror] = []
        tasks.reserveCapacity(10_000)
        for index in 0..<10_000 {
            let title: String
            let notes: String
            let dueDate: Date?
            if index == 9_876 {
                title = "Launch Target task #deepwork"
                notes = "Contains launch target notes"
            } else {
                title = "Task \(index) #tag\(index % 25)"
                notes = "Notes \(index)"
            }
            if index.isMultiple(of: 3) {
                dueDate = baseDate.addingTimeInterval(TimeInterval(index * 60))
            } else {
                dueDate = nil
            }
            tasks.append(task(
                id: "task-\(index)",
                title: title,
                notes: notes,
                listID: "list-\(index % lists.count)",
                dueDate: dueDate
            ))
        }

        var events: [CalendarEventMirror] = []
        events.reserveCapacity(10_000)
        for index in 0..<10_000 {
            let summary = index == 8_765 ? "Planning Review Target" : "Calendar Event \(index)"
            let details = index == 8_765 ? "VIP Attendee agenda" : "Details \(index)"
            events.append(event(
                id: "event-\(index)",
                calendarID: "calendar-\(index % calendars.count)",
                summary: summary,
                details: details
            ))
        }
        let snapshot = CommandPaletteEntitySnapshot(
            key: "large-rev-1",
            tasks: tasks,
            events: events,
            taskLists: lists,
            calendars: calendars,
            customFilters: [
                CustomFilterDefinition(name: "Deep Work", tagsAny: ["deepwork"], pinnedToMenuBar: true)
            ]
        )

        let start = Date()
        let fuzzyResult = try await cache.search(snapshot: snapshot, query: "Launch Target")
        let structuredResult = try await cache.search(snapshot: snapshot, query: "kind:event title:Planning")
        let regexResult = try await cache.search(snapshot: snapshot, query: "/VIP Attendee/")
        let elapsed = Date().timeIntervalSince(start)

        await XCTContext.runActivity(named: "Large command palette index/search elapsed: \(elapsed)s") { _ in }
        XCTAssertTrue(fuzzyResult.entities.contains { entity in
            if case .task(let task) = entity {
                return task.id == "task-9876"
            }
            return false
        })
        XCTAssertTrue(structuredResult.entities.allSatisfy { entity in
            if case .event = entity { return true }
            return false
        })
        XCTAssertTrue(structuredResult.entities.contains { entity in
            if case .event(let event) = entity {
                return event.id == "event-8765"
            }
            return false
        })
        XCTAssertTrue(regexResult.entities.contains { entity in
            if case .event(let event) = entity {
                return event.id == "event-8765"
            }
            return false
        })
    }

    func testPaletteOpenSkipsFallbackPrewarmAndLiveFullSnapshotsWhenDatabaseSearchIsAvailable() throws {
        let sourceURL = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .appending(path: "HotCrossBuns/App/CommandPaletteView.swift")
        let source = try String(contentsOf: sourceURL)

        XCTAssertTrue(source.contains("guard commandPaletteStore.usesDatabaseEntitySearch == false else { return }"))
        XCTAssertTrue(source.contains("CommandPaletteEntitySnapshot("))
        XCTAssertTrue(source.contains("taskStore: taskStore"))
        XCTAssertTrue(source.contains("calendarStore: calendarStore"))
        XCTAssertTrue(source.contains("commandPaletteStore: commandPaletteStore"))
        XCTAssertTrue(source.contains("try await Task.sleep(for: .milliseconds(90))"))
        let liveSearchRangeStart = try XCTUnwrap(source.range(of: ".task(id: entitySearchTaskID)")?.lowerBound)
        let submitRangeStart = try XCTUnwrap(source.range(of: "private func executeFirstMatch()")?.lowerBound)
        let liveSearchSource = String(source[liveSearchRangeStart..<submitRangeStart])
        XCTAssertTrue(liveSearchSource.contains("AdvancedSearchParser.parse(liveQuery).regex != nil"))
        XCTAssertFalse(liveSearchSource.contains("CommandPaletteEntitySnapshot(model: model"))
        XCTAssertFalse(liveSearchSource.contains("includeEntities: true"))
        let submitSource = String(source[submitRangeStart...])
        XCTAssertTrue(submitSource.contains("key: snapshotKeyAtStart"))
        XCTAssertTrue(submitSource.contains("includeEntities: true"))
        XCTAssertTrue(source.contains("resultSnapshotKey = snapshotKeyAtStart"))
    }

    private func task(
        id: String,
        title: String,
        notes: String = "",
        listID: String,
        dueDate: Date? = nil
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: listID,
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

    private func calendar(id: String, summary: String) -> CalendarListMirror {
        CalendarListMirror(
            id: id,
            summary: summary,
            colorHex: "#3366cc",
            isSelected: true,
            accessRole: "owner"
        )
    }

    private func event(
        id: String,
        calendarID: String,
        summary: String,
        details: String
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: summary,
            details: details,
            startDate: baseDate,
            endDate: baseDate.addingTimeInterval(3_600),
            isAllDay: false,
            status: .confirmed,
            recurrence: [],
            etag: nil,
            updatedAt: nil
        )
    }
}
