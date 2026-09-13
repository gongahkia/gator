import XCTest
@testable import HotCrossBunsMac

// Tests for A2 — pre-bucketed eventsByCalendar index. Verifies the index
// is correctly populated after rebuildSnapshots, excludes cancelled
// events, and that grouped lookups are equivalent to direct filter
// results across the full events array.
@MainActor
final class AppModelEventsByCalendarTests: XCTestCase {
    override func tearDown() {
        MockURLProtocol.reset()
        super.tearDown()
    }

    func testEventsByCalendarMatchesDirectFilter() async {
        let model = AppModel.preview
        // preview seeds events across the preview calendars; rebuildSnapshots
        // runs in apply() during installPreviewData.
        let calendarIDs = Set(model.calendars.map(\.id))

        for calID in calendarIDs {
            let directIDs = Set(model.events.filter {
                $0.calendarID == calID && $0.status != .cancelled
            }.map(\.id))
            let bucketedIDs = Set(model.eventsByCalendar[calID] ?? [])
            XCTAssertEqual(
                directIDs,
                bucketedIDs,
                "bucket for \(calID) should match the direct filter result"
            )
        }
    }

    func testEventsByCalendarOmitsCancelledEvents() {
        let model = AppModel.preview
        for (_, bucket) in model.eventsByCalendar {
            for eventID in bucket {
                guard let event = model.event(id: eventID) else {
                    XCTFail("bucket index referenced missing event \(eventID)")
                    continue
                }
                XCTAssertNotEqual(
                    event.status,
                    .cancelled,
                    "cancelled events must not appear in the bucket index"
                )
            }
        }
    }

    func testEventsByCalendarTotalDoesNotExceedRawEvents() {
        let model = AppModel.preview
        let bucketedTotal = model.eventsByCalendar.values.reduce(0) { $0 + $1.count }
        let activeRawTotal = model.events.filter { $0.status != .cancelled }.count
        XCTAssertEqual(bucketedTotal, activeRawTotal,
                       "every active event must appear in exactly one calendar bucket")
    }

    func testScheduledDerivedSnapshotRebuildAppliesIndexesBeforeRevisionBump() async {
        let model = AppModel.preview
        let initialRevision = model.dataRevision
        guard let listID = model.taskLists.first?.id else {
            XCTFail("preview data should include task lists")
            return
        }

        model.toggleTaskList(listID)

        XCTAssertTrue(model.isRebuildingDerivedSnapshots)
        await waitForRevisionChange(model, from: initialRevision)

        XCTAssertFalse(model.isRebuildingDerivedSnapshots)
        XCTAssertGreaterThan(model.dataRevision, initialRevision)
        XCTAssertEqual(model.visibleTaskListIDs, model.settings.selectedTaskListIDs)
        XCTAssertEqual(model.taskByIDSnapshot.count, model.tasks.count)
        XCTAssertEqual(model.eventByIDSnapshot.count, model.events.count)
        XCTAssertTrue(model.tasksByDueDate.values.joined().allSatisfy { model.task(id: $0) != nil })
        XCTAssertTrue(model.eventsByDay.values.joined().allSatisfy { model.event(id: $0) != nil })
    }

    func testEventsByDayBucketsTimedAllDayMultiDayMidnightAndCancelledEvents() async {
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = TimeZone(identifier: "UTC")!
        defer { NSTimeZone.default = originalTimeZone }

        var hiddenSettings = AppSettings.default
        hiddenSettings.showCompletedItemsInCalendar = false
        let hiddenModel = await loadedModel(
            events: [
                event(id: "same-day", start: date(2026, 4, 18, hour: 9), end: date(2026, 4, 18, hour: 10)),
                event(id: "all-day", start: date(2026, 4, 19), end: date(2026, 4, 21), isAllDay: true),
                event(id: "multi-day", start: date(2026, 4, 22, hour: 10), end: date(2026, 4, 24, hour: 11)),
                event(id: "midnight", start: date(2026, 4, 25, hour: 23, minute: 30), end: date(2026, 4, 26, hour: 0, minute: 30)),
                event(id: "ends-at-midnight", start: date(2026, 4, 27, hour: 23), end: date(2026, 4, 28)),
                event(id: "cancelled", start: date(2026, 4, 18, hour: 11), end: date(2026, 4, 18, hour: 12), status: .cancelled)
            ],
            settings: hiddenSettings
        )

        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 18)] ?? [], ["same-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 19)] ?? [], ["all-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 20)] ?? [], ["all-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 21)] ?? [], ["all-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 22)] ?? [], ["multi-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 23)] ?? [], ["multi-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 24)] ?? [], ["multi-day"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 25)] ?? [], ["midnight"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 26)] ?? [], ["midnight"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 27)] ?? [], ["ends-at-midnight"])
        XCTAssertEqual(hiddenModel.eventsByDay[key(2026, 4, 28)] ?? [], ["ends-at-midnight"])
        XCTAssertFalse(hiddenModel.eventsByDay.values.joined().contains("cancelled"))

        var shownSettings = hiddenSettings
        shownSettings.showCompletedItemsInCalendar = true
        let shownModel = await loadedModel(
            events: [
                event(id: "cancelled", start: date(2026, 4, 18, hour: 11), end: date(2026, 4, 18, hour: 12), status: .cancelled)
            ],
            settings: shownSettings
        )

        XCTAssertEqual(shownModel.eventsByDay[key(2026, 4, 18)] ?? [], ["cancelled"])
    }

    func testEventsByDayBucketsDSTAdjacentEvent() async {
        guard let timeZone = TimeZone(identifier: "America/Los_Angeles") else {
            XCTFail("expected Los Angeles time zone to be available")
            return
        }
        let originalTimeZone = NSTimeZone.default
        NSTimeZone.default = timeZone
        defer { NSTimeZone.default = originalTimeZone }

        let model = await loadedModel(
            events: [
                event(id: "dst", start: date(2026, 3, 8, hour: 1, minute: 30), end: date(2026, 3, 8, hour: 3, minute: 30))
            ],
            settings: .default
        )

        XCTAssertEqual(model.eventsByDay[key(2026, 3, 8)] ?? [], ["dst"])
        XCTAssertNil(model.eventsByDay[key(2026, 3, 9)])
    }

    func testTodaySnapshotPrefilteredScheduledEventsMatchEventBuilder() {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        let referenceDate = calendar.date(from: DateComponents(year: 2026, month: 4, day: 18, hour: 12))!
        func makeDate(_ day: Int, _ hour: Int) -> Date {
            calendar.date(from: DateComponents(year: 2026, month: 4, day: day, hour: hour))!
        }

        let events = [
            event(id: "later", start: makeDate(18, 14), end: makeDate(18, 15)),
            event(id: "cancelled", start: makeDate(18, 9), end: makeDate(18, 10), status: .cancelled),
            event(id: "earlier", start: makeDate(18, 8), end: makeDate(18, 9)),
            event(id: "tomorrow", start: makeDate(19, 8), end: makeDate(19, 9))
        ]
        let scheduledEvents = events.filter { event in
            event.status != .cancelled && calendar.isDate(event.startDate, inSameDayAs: referenceDate)
        }

        let scannedSnapshot = TodaySnapshot.build(
            tasks: [],
            events: events,
            referenceDate: referenceDate,
            calendar: calendar
        )
        let prefilteredSnapshot = TodaySnapshot.build(
            tasks: [],
            scheduledEvents: scheduledEvents,
            referenceDate: referenceDate,
            calendar: calendar
        )

        XCTAssertEqual(prefilteredSnapshot, scannedSnapshot)
        XCTAssertEqual(prefilteredSnapshot.scheduledEvents.map(\.id), ["earlier", "later"])
    }

    private func waitForRevisionChange(_ model: AppModel, from revision: UInt64, file: StaticString = #filePath, line: UInt = #line) async {
        let deadline = Date().addingTimeInterval(2)
        while model.dataRevision == revision, Date() < deadline {
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        if model.dataRevision == revision {
            XCTFail("timed out waiting for derived snapshot rebuild", file: file, line: line)
        }
    }

    private func loadedModel(events: [CalendarEventMirror], settings: AppSettings) async -> AppModel {
        let account = GoogleAccount(
            id: "events-by-day-account",
            email: "events-by-day@example.com",
            displayName: "Events By Day",
            grantedScopes: [GoogleScope.calendar],
            authProvider: .customDesktopOAuth
        )
        let state = CachedAppState(
            account: account,
            accounts: [account],
            activeAccountID: account.id,
            taskLists: [],
            tasks: [],
            calendars: [
                CalendarListMirror(
                    id: "calendar",
                    summary: "Calendar",
                    colorHex: "#039be5",
                    isSelected: true,
                    accessRole: "owner"
                )
            ],
            events: events,
            settings: settings
        )
        let model = makeModel(cachedState: state)
        await model.loadInitialState()
        return model
    }

    private func makeModel(cachedState: CachedAppState) -> AppModel {
        let customOAuthService = CustomGoogleOAuthService(tokenStore: InMemoryGoogleOAuthTokenStore(
            clientConfiguration: GoogleOAuthClientConfiguration(clientID: "events-by-day.apps.googleusercontent.com", clientSecret: nil)
        ))
        let authService = GoogleAuthService(customOAuthService: customOAuthService)
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://example.test")!,
            tokenProvider: StaticAccessTokenProvider(token: "events-by-day-token"),
            urlSession: MockURLProtocol.testSession()
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: authService,
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient),
            cacheStore: LocalCacheStore(fileURL: nil, cachedState: cachedState)
        )
    }

    private func event(
        id: CalendarEventMirror.ID,
        start: Date,
        end: Date,
        isAllDay: Bool = false,
        status: CalendarEventStatus = .confirmed
    ) -> CalendarEventMirror {
        CalendarEventMirror(
            id: id,
            calendarID: "calendar",
            summary: id,
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: isAllDay,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: nil
        )
    }

    private func key(_ year: Int, _ month: Int, _ day: Int) -> TimeInterval {
        Calendar.current.startOfDay(for: date(year, month, day)).timeIntervalSinceReferenceDate
    }

    private func date(
        _ year: Int,
        _ month: Int,
        _ day: Int,
        hour: Int = 0,
        minute: Int = 0
    ) -> Date {
        Calendar.current.date(from: DateComponents(year: year, month: month, day: day, hour: hour, minute: minute))!
    }
}
