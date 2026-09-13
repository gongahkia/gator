import XCTest
@testable import HotCrossBunsMac

final class CustomFilterTests: XCTestCase {
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

    func testDueWindowAny() {
        let filter = CustomFilterDefinition(name: "all")
        XCTAssertTrue(filter.matches(task(id: "a"), now: now, calendar: calendar))
    }

    func testDueWindowOverdue() {
        let filter = CustomFilterDefinition(name: "o", dueWindow: .overdue)
        XCTAssertTrue(filter.matches(task(id: "a", due: day(-1)), now: now, calendar: calendar))
        XCTAssertFalse(filter.matches(task(id: "b", due: day(0)), now: now, calendar: calendar))
        XCTAssertFalse(filter.matches(task(id: "c", due: nil), now: now, calendar: calendar))
    }

    func testCompletedExcludedByDefault() {
        let filter = CustomFilterDefinition(name: "x")
        XCTAssertFalse(filter.matches(task(id: "a", completed: true), now: now, calendar: calendar))
    }

    func testCompletedIncludedWhenRequested() {
        let filter = CustomFilterDefinition(name: "x", includeCompleted: true)
        XCTAssertTrue(filter.matches(task(id: "a", completed: true), now: now, calendar: calendar))
    }

    func testTagsAnyMatch() {
        let filter = CustomFilterDefinition(name: "t", tagsAny: ["work", "urgent"])
        XCTAssertTrue(filter.matches(task(id: "a", title: "fix bug #work"), now: now, calendar: calendar))
        XCTAssertFalse(filter.matches(task(id: "b", title: "read book #personal"), now: now, calendar: calendar))
    }

    func testListRestriction() {
        let filter = CustomFilterDefinition(name: "l", taskListIDs: ["L1"])
        XCTAssertTrue(filter.matches(task(id: "a", list: "L1"), now: now, calendar: calendar))
        XCTAssertFalse(filter.matches(task(id: "b", list: "L2"), now: now, calendar: calendar))
    }

    func testDuplicatedFilterGetsNewIDCopyNameAndPreservesFields() {
        let id = UUID()
        let copyID = UUID()
        let filter = CustomFilterDefinition(
            id: id,
            name: "Menu",
            systemImage: "star",
            dueWindow: .next7Days,
            includeCompleted: true,
            taskListIDs: ["L1", "L2"],
            tagsAny: ["work", "deep"],
            queryExpression: "tag:work",
            pinnedToMenuBar: true
        )

        let copy = filter.duplicated(id: copyID)

        XCTAssertNotEqual(copy.id, filter.id)
        XCTAssertEqual(copy.id, copyID)
        XCTAssertEqual(copy.name, "Menu Copy")
        XCTAssertEqual(copy.systemImage, filter.systemImage)
        XCTAssertEqual(copy.dueWindow, filter.dueWindow)
        XCTAssertEqual(copy.includeCompleted, filter.includeCompleted)
        XCTAssertEqual(copy.taskListIDs, filter.taskListIDs)
        XCTAssertEqual(copy.tagsAny, filter.tagsAny)
        XCTAssertEqual(copy.queryExpression, filter.queryExpression)
        XCTAssertEqual(copy.pinnedToMenuBar, filter.pinnedToMenuBar)
        XCTAssertNil(copy.lastUsedAt)
        XCTAssertEqual(copy.useCount, 0)
    }

    func testCustomFilterDecodesMissingUsageMetadata() throws {
        let id = UUID()
        let json = """
        {
          "id": "\(id.uuidString)",
          "name": "Legacy"
        }
        """.data(using: .utf8)!

        let decoded = try JSONDecoder().decode(CustomFilterDefinition.self, from: json)

        XCTAssertNil(decoded.lastUsedAt)
        XCTAssertEqual(decoded.useCount, 0)
    }

    @MainActor
    func testMarkCustomFilterUsedOnlyUpdatesTarget() {
        let targetID = UUID()
        let otherID = UUID()
        let model = AppModel.bootstrap()
        model.settings.customFilters = [
            CustomFilterDefinition(id: targetID, name: "Target"),
            CustomFilterDefinition(id: otherID, name: "Other")
        ]

        model.markCustomFilterUsed(targetID)

        let target = model.settings.customFilters.first { $0.id == targetID }
        let other = model.settings.customFilters.first { $0.id == otherID }
        XCTAssertEqual(target?.useCount, 1)
        XCTAssertNotNil(target?.lastUsedAt)
        XCTAssertEqual(other?.useCount, 0)
        XCTAssertNil(other?.lastUsedAt)
    }

    func testSettingsSearchIndexFindsHighValueQueries() {
        let results = SettingsSearchIndex.results(
            customShortcutCount: 2,
            shortcutConflictCount: 1,
            customFilterCount: 3,
            taskTemplateCount: 4,
            eventTemplateCount: 5,
            updateStatus: "Update"
        )

        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "update").first?.anchor, .updates)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "template").first?.anchor, .templates)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "filter").first?.anchor, .customFilters)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "shortcut").first?.anchor, .hotkeys)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "menu bar").first?.anchor, .menuBar)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "background").first?.anchor, .background)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "diagnostics").first?.anchor, .diagnostics)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "google account").first?.tab, .profile)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "connect account").first?.anchor, .profileAccounts)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "oauth client").first?.tab, .profile)
        XCTAssertEqual(SettingsSearchIndex.filter(results, query: "profile").first?.tab, .profile)
    }
}
