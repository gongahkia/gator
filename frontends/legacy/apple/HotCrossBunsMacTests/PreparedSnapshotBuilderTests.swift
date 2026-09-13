import XCTest
@testable import HotCrossBunsMac

final class PreparedCalendarSnapshotBuilderTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        cal.firstWeekday = 1
        return cal
    }()

    private func day(_ y: Int, _ m: Int, _ d: Int, hour: Int = 0, minute: Int = 0) -> Date {
        calendar.date(from: DateComponents(year: y, month: m, day: d, hour: hour, minute: minute))!
    }

    private func event(
        id: String,
        calendarID: String = "primary",
        summary: String,
        details: String = "",
        start: Date,
        end: Date,
        allDay: Bool = false,
        status: CalendarEventStatus = .confirmed,
        colorID: String? = nil
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: summary,
            details: details,
            startDate: start,
            endDate: end,
            isAllDay: allDay,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil,
            colorId: colorID
        )
    }

    private func task(
        id: String,
        listID: String = "list-a",
        title: String,
        due: Date?,
        completed: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: listID,
            parentID: nil,
            title: title,
            notes: "",
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: completed ? day(2026, 5, 1, hour: 12) : nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    func testDaySnapshotFiltersSearchCalendarSelectionAndPrecomputesLayout() {
        let focus = event(
            id: "focus",
            summary: "Design Review #work",
            details: "Large account planning",
            start: day(2026, 5, 11, hour: 9),
            end: day(2026, 5, 11, hour: 10)
        )
        let hiddenCalendar = event(
            id: "hidden-calendar",
            calendarID: "secondary",
            summary: "Design Review",
            start: day(2026, 5, 11, hour: 11),
            end: day(2026, 5, 11, hour: 12)
        )
        let searchMiss = event(
            id: "search-miss",
            summary: "Lunch",
            start: day(2026, 5, 11, hour: 13),
            end: day(2026, 5, 11, hour: 14)
        )

        let input = calendarInput(
            key: PreparedSnapshotKey("day-old"),
            anchor: day(2026, 5, 11),
            events: [focus, hiddenCalendar, searchMiss],
            tasks: [],
            selectedCalendarIDs: ["primary"],
            searchQuery: "design"
        )

        let snapshot = CalendarDisplaySnapshotBuilder.daySnapshot(input)

        XCTAssertEqual(snapshot.timedEvents.map(\.id), ["focus"])
        XCTAssertTrue(snapshot.allDayEvents.isEmpty)
        XCTAssertEqual(snapshot.laidOutTimedEvents.map(\.event.id), ["focus"])
        XCTAssertEqual(snapshot.eventMetadataByID["focus"]?.colorHex, "#448AFF")
        XCTAssertTrue(snapshot.eventMetadataByID["focus"]?.timeRangeLabel.contains(" - ") == true)
        XCTAssertTrue(snapshot.eventMetadataByID["focus"]?.accessibilityLabel.contains("Design Review") == true)
    }

    func testWeekSnapshotPreparesAllDaySpansTimedLayoutsTasksAndMetadata() {
        let multiDay = event(
            id: "offsite",
            summary: "Offsite #team",
            start: day(2026, 5, 11),
            end: day(2026, 5, 14),
            allDay: true
        )
        let timed = event(
            id: "timed",
            summary: "1:1",
            start: day(2026, 5, 12, hour: 14),
            end: day(2026, 5, 12, hour: 15)
        )
        let dueTask = task(id: "task-1", title: "Ship #focus", due: day(2026, 5, 12))

        let input = calendarInput(
            key: PreparedSnapshotKey("week"),
            anchor: day(2026, 5, 12),
            events: [multiDay, timed],
            tasks: [dueTask],
            selectedCalendarIDs: ["primary"],
            visibleTaskListIDs: ["list-a"]
        )

        let snapshot = CalendarDisplaySnapshotBuilder.weekSnapshot(input)
        let tuesdayKey = CalendarDisplaySnapshotBuilder.dayKey(day(2026, 5, 12), calendar: calendar)

        XCTAssertEqual(snapshot.allDaySpans.map(\.event.id), ["offsite"])
        XCTAssertEqual(snapshot.timedEventsByDay[tuesdayKey]?.map(\.id), ["timed"])
        XCTAssertEqual(snapshot.laidOutTimedEventsByDay[tuesdayKey]?.map(\.event.id), ["timed"])
        XCTAssertEqual(snapshot.tasksByDay[tuesdayKey]?.map(\.id), ["task-1"])
        XCTAssertEqual(snapshot.taskMetadataByID["task-1"]?.strippedTitle, "Ship")
        XCTAssertEqual(snapshot.taskMetadataByID["task-1"]?.listTitle, "Inbox")
    }

    func testAgendaAndYearSnapshotsRespectSearchAndSelectedCalendars() {
        let matching = event(
            id: "matching",
            summary: "Focus Sprint",
            start: day(2026, 5, 11, hour: 9),
            end: day(2026, 5, 11, hour: 10)
        )
        let hiddenCalendar = event(
            id: "hidden-calendar",
            calendarID: "secondary",
            summary: "Focus Sprint",
            start: day(2026, 5, 12, hour: 9),
            end: day(2026, 5, 12, hour: 10)
        )
        let searchMiss = event(
            id: "search-miss",
            summary: "Planning",
            start: day(2026, 5, 13, hour: 9),
            end: day(2026, 5, 13, hour: 10)
        )
        let dueTask = task(id: "task-1", title: "Focus task", due: day(2026, 5, 12))

        let input = calendarInput(
            key: PreparedSnapshotKey("agenda"),
            anchor: day(2026, 5, 11),
            events: [matching, hiddenCalendar, searchMiss],
            tasks: [dueTask],
            selectedCalendarIDs: ["primary"],
            visibleTaskListIDs: ["list-a"],
            searchQuery: "focus"
        )

        let agenda = CalendarDisplaySnapshotBuilder.agendaSnapshot(input, dayCount: 3)
        let year = CalendarDisplaySnapshotBuilder.yearSnapshot(input)
        let may11Key = CalendarDisplaySnapshotBuilder.dayKey(day(2026, 5, 11), calendar: calendar)
        let may12Key = CalendarDisplaySnapshotBuilder.dayKey(day(2026, 5, 12), calendar: calendar)

        XCTAssertEqual(agenda.days.flatMap(\.events).map(\.id), ["matching"])
        XCTAssertEqual(agenda.days.flatMap(\.tasks).map(\.id), ["task-1"])
        XCTAssertEqual(year.countsByDay[may11Key], 1)
        XCTAssertNil(year.countsByDay[may12Key])
        XCTAssertEqual(year.maxCount, 1)
        XCTAssertEqual(year.months.count, 12)
    }

    func testPreparedSnapshotKeyMismatchRejectsStaleAsyncResult() {
        let olderKey = PreparedSnapshotKeys.calendar(
            mode: .day,
            dataRevision: 1,
            selectedCalendarIDs: ["primary"],
            visibleTaskListIDs: ["list-a"],
            filterKey: "all",
            searchQuery: "",
            rangeKey: "2026-05-11",
            settings: .default
        )
        let newerKey = PreparedSnapshotKeys.calendar(
            mode: .day,
            dataRevision: 2,
            selectedCalendarIDs: ["primary"],
            visibleTaskListIDs: ["list-a"],
            filterKey: "all",
            searchQuery: "",
            rangeKey: "2026-05-11",
            settings: .default
        )
        let input = calendarInput(key: olderKey, anchor: day(2026, 5, 11), events: [], tasks: [])

        let staleResult = CalendarDisplaySnapshotBuilder.daySnapshot(input)

        XCTAssertEqual(staleResult.key, olderKey)
        XCTAssertNotEqual(staleResult.key, newerKey)
    }

    func testDenseDaySnapshotCapsRichTimedLayoutButKeepsFullEventList() {
        let events = (0..<360).map { index in
            event(
                id: "dense-\(index)",
                summary: "Dense event \(index)",
                start: day(2026, 5, 11, hour: 8 + (index % 8), minute: (index % 4) * 15),
                end: day(2026, 5, 11, hour: 9 + (index % 8), minute: (index % 4) * 15)
            )
        }
        let input = calendarInput(
            key: PreparedSnapshotKey("dense-day"),
            anchor: day(2026, 5, 11),
            events: events,
            tasks: [],
            selectedCalendarIDs: ["primary"]
        )

        let snapshot = CalendarDisplaySnapshotBuilder.daySnapshot(input)

        XCTAssertEqual(snapshot.timedEvents.count, 360)
        XCTAssertEqual(snapshot.laidOutTimedEvents.count, CalendarDensityRendering.dayTimedEventLimit)
        XCTAssertTrue(snapshot.density.usesDenseTimedRendering)
        XCTAssertEqual(snapshot.density.hiddenTimedEventCount, 360 - CalendarDensityRendering.dayTimedEventLimit)
    }

    func testDenseWeekSnapshotCapsPerDayTimedLayout() {
        let events = (0..<300).map { index in
            event(
                id: "week-dense-\(index)",
                summary: "Week dense \(index)",
                start: day(2026, 5, 12, hour: 7 + (index % 10), minute: (index % 4) * 15),
                end: day(2026, 5, 12, hour: 8 + (index % 10), minute: (index % 4) * 15)
            )
        }
        let input = calendarInput(
            key: PreparedSnapshotKey("dense-week"),
            anchor: day(2026, 5, 12),
            events: events,
            tasks: [],
            selectedCalendarIDs: ["primary"]
        )
        let tuesdayKey = CalendarDisplaySnapshotBuilder.dayKey(day(2026, 5, 12), calendar: calendar)

        let snapshot = CalendarDisplaySnapshotBuilder.weekSnapshot(input)

        XCTAssertEqual(snapshot.timedEventsByDay[tuesdayKey]?.count, 300)
        XCTAssertEqual(snapshot.laidOutTimedEventsByDay[tuesdayKey]?.count, CalendarDensityRendering.weekTimedEventLimit)
        XCTAssertEqual(snapshot.densityByDay[tuesdayKey]?.hiddenTimedEventCount, 300 - CalendarDensityRendering.weekTimedEventLimit)
    }

    private func calendarInput(
        key: PreparedSnapshotKey,
        anchor: Date,
        events: [CalendarEventMirror],
        tasks: [TaskMirror],
        selectedCalendarIDs: Set<CalendarListMirror.ID> = ["primary"],
        visibleTaskListIDs: Set<TaskListMirror.ID> = ["list-a"],
        searchQuery: String = "",
        settings: AppSettings = .default
    ) -> CalendarDisplayInput {
        let eventsByDate = CalendarGridLayout.eventsByDay(
            events,
            from: day(2026, 1, 1),
            to: day(2026, 12, 31),
            calendar: calendar
        )
        let eventsByDay = Dictionary(uniqueKeysWithValues: eventsByDate.map { date, dayEvents in
            (date.timeIntervalSinceReferenceDate, dayEvents.map(\.id))
        })
        let taskByID = Dictionary(uniqueKeysWithValues: tasks.map { ($0.id, $0) })
        var tasksByDueDate: [TimeInterval: [TaskMirror.ID]] = [:]
        for task in tasks {
            guard let due = task.dueDate else { continue }
            tasksByDueDate[calendar.startOfDay(for: due).timeIntervalSinceReferenceDate, default: []].append(task.id)
        }

        let range = CalendarVisibleRange(kind: .month, start: day(2026, 1, 1), end: day(2026, 12, 31), calendar: calendar)
        let visibleRange = CalendarVisibleRangeProjection(
            range: range,
            revisionKey: "test",
            eventsByDay: eventsByDay,
            tasksByDueDate: tasksByDueDate,
            eventByID: Dictionary(uniqueKeysWithValues: events.map { ($0.id, $0) }),
            taskByID: taskByID
        )

        return CalendarDisplayInput(
            key: key,
            anchorDate: anchor,
            selectedCalendarIDs: selectedCalendarIDs,
            eventViewFilter: CalendarEventViewFilter(),
            visibleTaskListIDs: visibleTaskListIDs,
            searchQuery: searchQuery,
            visibleRange: visibleRange,
            calendarColorHexByID: ["primary": "#448AFF", "secondary": "#FF7043"],
            taskListTitleByID: ["list-a": "Inbox", "list-b": "Hidden"],
            settings: settings,
            referenceDate: day(2026, 5, 11, hour: 8),
            calendar: calendar
        )
    }
}

final class PreparedTaskBoardDisplaySnapshotBuilderTests: XCTestCase {
    private let calendar: Calendar = {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }()

    private var now: Date {
        calendar.date(from: DateComponents(year: 2026, month: 5, day: 11, hour: 9))!
    }

    private func day(_ offset: Int) -> Date {
        calendar.date(byAdding: .day, value: offset, to: calendar.startOfDay(for: now))!
    }

    private func task(
        id: String,
        listID: String = "list-a",
        title: String,
        notes: String = "",
        due: Date? = nil,
        completed: Bool = false,
        deleted: Bool = false
    ) -> TaskMirror {
        TaskMirror(
            id: id,
            taskListID: listID,
            parentID: nil,
            title: title,
            notes: notes,
            status: completed ? .completed : .needsAction,
            dueDate: due,
            completedAt: completed ? day(-1) : nil,
            isDeleted: deleted,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: nil
        )
    }

    private let lists = [
        TaskListMirror(id: "list-a", title: "Inbox"),
        TaskListMirror(id: "list-b", title: "Writing")
    ]

    func testTaskBoardSnapshotPrecomputesCardsGroupingSplitsAndBadges() {
        let open = task(id: "open", title: "Draft launch #work", notes: "Use **short** copy", due: now)
        let completed = task(id: "done", title: "Archive notes", due: day(-2), completed: true)
        let otherList = task(id: "other", listID: "list-b", title: "Essay", due: day(3))
        let input = boardInput(
            key: PreparedSnapshotKey("tasks"),
            surface: .tasks,
            tasks: [open, completed, otherList],
            mode: .byList,
            duplicateIDs: ["open"]
        )

        let snapshot = TaskBoardDisplaySnapshotBuilder.snapshot(input)
        let inbox = snapshot.columns.first { $0.title == "Inbox" }
        let openCard = inbox?.openTasks.first
        let doneCard = inbox?.completedTasks.first

        XCTAssertEqual(snapshot.taskCount, 3)
        XCTAssertEqual(snapshot.columns.map(\.title), ["Inbox", "Writing"])
        XCTAssertEqual(openCard?.strippedTitle, "Draft launch")
        XCTAssertEqual(openCard?.tags, ["work"])
        XCTAssertEqual(openCard?.listTitle, "Inbox")
        XCTAssertEqual(openCard?.dueDateBadge, "Today")
        XCTAssertEqual(openCard?.dueDateTone, .today)
        XCTAssertEqual(openCard?.notePreview, "Use **short** copy")
        XCTAssertTrue(openCard?.isDuplicate == true)
        XCTAssertTrue(openCard?.accessibilityLabel.contains("possible duplicate") == true)
        XCTAssertTrue(doneCard?.completedText?.hasPrefix("Completed") == true)
    }

    func testNotesSnapshotHonorsLocalOrderAndPrecomputesNoteMetadata() {
        let first = task(id: "n1", title: "First note #home", notes: "- buy milk")
        let second = task(id: "n2", title: "Second note", notes: "Plain note")
        let input = boardInput(
            key: PreparedSnapshotKey("notes"),
            surface: .notes,
            tasks: [first, second],
            mode: .byList,
            localOrder: ["n2", "n1"]
        )

        let snapshot = TaskBoardDisplaySnapshotBuilder.snapshot(input)
        let cards = snapshot.columns.first { $0.title == "Inbox" }?.openTasks ?? []

        XCTAssertEqual(cards.map(\.id), ["n2", "n1"])
        XCTAssertEqual(snapshot.columns.first { $0.title == "Inbox" }?.subtitle, "2 notes")
        XCTAssertEqual(cards[0].notePreview, "Plain note")
        XCTAssertTrue(cards[0].accessibilityLabel.hasPrefix("Note, Second note"))
        XCTAssertEqual(cards[1].tags, ["home"])
    }

    func testListSnapshotsOmitEmptyListColumns() {
        let task = task(id: "open", title: "Only populated list", due: now)
        let input = boardInput(
            key: PreparedSnapshotKey("tasks-non-empty-lists"),
            surface: .tasks,
            tasks: [task],
            mode: .byList
        )

        let snapshot = TaskBoardDisplaySnapshotBuilder.snapshot(input)

        // Empty lists should not render placeholder Kanban columns.
        XCTAssertEqual(snapshot.columns.map(\.title), ["Inbox"])
    }

    func testNotesSnapshotKeepsCompletedOnlyListColumn() {
        let note = task(id: "done-note", listID: "list-b", title: "Archived note", completed: true)
        let input = boardInput(
            key: PreparedSnapshotKey("notes-completed-only-list"),
            surface: .notes,
            tasks: [note],
            mode: .byList
        )

        let snapshot = TaskBoardDisplaySnapshotBuilder.snapshot(input)

        // Completed-only lists still count as populated, so the notes column remains visible.
        XCTAssertEqual(snapshot.columns.map(\.title), ["Writing"])
        XCTAssertEqual(snapshot.columns.first?.openTasks.map(\.id), [])
        XCTAssertEqual(snapshot.columns.first?.completedTasks.map(\.id), ["done-note"])
    }

    func testDeletedTasksDoNotContributeToPreparedColumnsOrCount() {
        let live = task(id: "live", title: "Live")
        let deleted = task(id: "deleted", title: "Deleted", deleted: true)
        let input = boardInput(
            key: PreparedSnapshotKey("deleted"),
            surface: .tasks,
            tasks: [live, deleted],
            mode: .byList
        )

        let snapshot = TaskBoardDisplaySnapshotBuilder.snapshot(input)

        XCTAssertEqual(snapshot.taskCount, 1)
        XCTAssertEqual(snapshot.columns.flatMap(\.openTasks).map(\.id), ["live"])
    }

    private func boardInput(
        key: PreparedSnapshotKey,
        surface: TaskBoardSurface,
        tasks: [TaskMirror],
        mode: KanbanColumnMode,
        duplicateIDs: Set<TaskMirror.ID> = [],
        localOrder: [TaskMirror.ID] = []
    ) -> TaskBoardDisplayInput {
        TaskBoardDisplayInput(
            key: key,
            surface: surface,
            tasks: tasks,
            columnMode: mode,
            taskLists: lists,
            taskListTitleByID: Dictionary(uniqueKeysWithValues: lists.map { ($0.id, $0.title) }),
            duplicateTaskIDs: duplicateIDs,
            localOrder: localOrder,
            referenceDate: now,
            calendar: calendar
        )
    }
}
