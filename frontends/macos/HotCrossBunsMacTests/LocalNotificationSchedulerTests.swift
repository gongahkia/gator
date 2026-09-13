import Foundation
import UserNotifications
import XCTest
@testable import HotCrossBunsMac

final class LocalNotificationSchedulerTests: XCTestCase {
    func testDisabledNotificationsClearsExistingRequestsAndSummary() async {
        let center = FakeUserNotificationCenter()
        center.pendingRequests = [
            makePendingRequest(identifier: "hot-cross-buns.task.1"),
            makePendingRequest(identifier: "hot-cross-buns.event.1"),
            makePendingRequest(identifier: "other.app.1")
        ]
        let scheduler = LocalNotificationScheduler(notificationCenter: center)
        let enabledSettings = AppSettings(
            syncMode: .manual,
            selectedCalendarIDs: [],
            selectedTaskListIDs: [],
            enableLocalNotifications: true
        )
        let disabledSettings = AppSettings(
            syncMode: .manual,
            selectedCalendarIDs: [],
            selectedTaskListIDs: [],
            enableLocalNotifications: false
        )

        await scheduler.synchronize(
            tasks: [],
            events: [makeEvent(id: "evt-1", startOffsetDays: 1, isAllDay: false)],
            settings: enabledSettings,
            referenceDate: referenceDate
        )
        let initialSummary = await scheduler.lastSummary
        XCTAssertNotNil(initialSummary)

        await scheduler.synchronize(
            tasks: [],
            events: [],
            settings: disabledSettings,
            referenceDate: referenceDate
        )

        let disabledSummary = await scheduler.lastSummary
        XCTAssertNil(disabledSummary)
        XCTAssertEqual(center.removedPendingIdentifiers, ["hot-cross-buns.task.1", "hot-cross-buns.event.1"])
        XCTAssertEqual(center.removedDeliveredIdentifiers, ["hot-cross-buns.task.1", "hot-cross-buns.event.1"])
    }

    func testSynchronizePrioritizesEventsAndDefersOverflow() async {
        let center = FakeUserNotificationCenter()
        let scheduler = LocalNotificationScheduler(notificationCenter: center)
        let settings = AppSettings(
            syncMode: .manual,
            selectedCalendarIDs: [],
            selectedTaskListIDs: [],
            enableLocalNotifications: true,
            taskReminderThresholdDays: 7
        )

        let events = (0..<70).map { index in
            makeEvent(
                id: "event-\(index)",
                startOffsetDays: 1 + index / 3,
                isAllDay: false
            )
        }
        let tasks = (0..<10).map { index in
            makeTask(
                id: "task-\(index)",
                dueOffsetDays: 8 + index
            )
        }

        await scheduler.synchronize(
            tasks: tasks,
            events: events,
            settings: settings,
            referenceDate: referenceDate
        )

        let summary = await scheduler.lastSummary
        XCTAssertEqual(center.addedRequests.count, 64)
        XCTAssertTrue(center.addedRequests.allSatisfy { $0.identifier.hasPrefix("hot-cross-buns.event.") })
        XCTAssertEqual(summary?.scheduledEvents, 64)
        XCTAssertEqual(summary?.scheduledTasks, 0)
        XCTAssertEqual(summary?.deferredEvents, 6)
        XCTAssertEqual(summary?.deferredTasks, 10)
    }

    func testSynchronizeSchedulesTaskTimedEventAndAllDayEventAtExpectedTimes() async {
        let center = FakeUserNotificationCenter()
        let scheduler = LocalNotificationScheduler(notificationCenter: center)
        let settings = AppSettings(
            syncMode: .manual,
            selectedCalendarIDs: [],
            selectedTaskListIDs: [],
            enableLocalNotifications: true,
            taskReminderThresholdDays: 7,
            taskReminderHour: 8,
            taskReminderMinute: 15
        )

        let task = makeTask(id: "task-1", dueOffsetDays: 8)
        let timedEvent = makeEvent(id: "event-timed", startOffsetDays: 1, isAllDay: false, reminderMinutes: [30])
        let allDayEvent = makeEvent(id: "event-all-day", startOffsetDays: 2, isAllDay: true)

        await scheduler.synchronize(
            tasks: [task],
            events: [timedEvent, allDayEvent],
            settings: settings,
            referenceDate: referenceDate
        )

        let taskRequest = center.addedRequests.first { $0.identifier == "hot-cross-buns.task.task-1" }
        let timedRequest = center.addedRequests.first { $0.identifier == "hot-cross-buns.event.event-timed" }
        let allDayRequest = center.addedRequests.first { $0.identifier == "hot-cross-buns.event.event-all-day" }

        let taskTrigger = try? XCTUnwrap(taskRequest?.trigger as? UNCalendarNotificationTrigger)
        let timedTrigger = try? XCTUnwrap(timedRequest?.trigger as? UNCalendarNotificationTrigger)
        let allDayTrigger = try? XCTUnwrap(allDayRequest?.trigger as? UNCalendarNotificationTrigger)

        XCTAssertEqual(taskTrigger?.dateComponents.hour, 8)
        XCTAssertEqual(taskTrigger?.dateComponents.minute, 15)
        XCTAssertEqual(timedTrigger?.dateComponents.hour, 9)
        XCTAssertEqual(timedTrigger?.dateComponents.minute, 30)
        XCTAssertEqual(allDayTrigger?.dateComponents.hour, 9)
        XCTAssertEqual(allDayTrigger?.dateComponents.minute, 0)
    }

    func testSynchronizeRequestsAuthorizationAndSkipsCancelledEvents() async {
        let center = FakeUserNotificationCenter()
        center.currentAuthorizationStatus = .notDetermined
        center.authorizationRequestResult = true
        let scheduler = LocalNotificationScheduler(notificationCenter: center)
        let settings = AppSettings(
            syncMode: .manual,
            selectedCalendarIDs: [],
            selectedTaskListIDs: [],
            enableLocalNotifications: true
        )

        await scheduler.synchronize(
            tasks: [],
            events: [
                makeEvent(id: "cancelled", startOffsetDays: 1, isAllDay: false, status: .cancelled),
                makeEvent(id: "kept", startOffsetDays: 1, isAllDay: false)
            ],
            settings: settings,
            requestAuthorization: true,
            referenceDate: referenceDate
        )

        let summary = await scheduler.lastSummary
        XCTAssertTrue(center.didRequestAuthorization)
        XCTAssertEqual(center.addedRequests.map(\.identifier), ["hot-cross-buns.event.kept"])
        XCTAssertEqual(summary?.scheduledEvents, 1)
        XCTAssertEqual(summary?.scheduledTasks, 0)
    }

    func testAuthorizationOutcomeReturnsDeniedWhenPromptIsRejected() async {
        let center = FakeUserNotificationCenter()
        center.currentAuthorizationStatus = .notDetermined
        center.authorizationRequestResult = false
        let scheduler = LocalNotificationScheduler(notificationCenter: center)

        let outcome = await scheduler.authorizationOutcome(requestAuthorization: true)

        XCTAssertEqual(outcome, .denied)
        XCTAssertTrue(center.didRequestAuthorization)
    }

    @MainActor
    func testAppModelRequestEnableLocalNotificationsTurnsSettingOnWhenAuthorized() async {
        let center = FakeUserNotificationCenter()
        center.currentAuthorizationStatus = .notDetermined
        center.authorizationRequestResult = true
        let scheduler = LocalNotificationScheduler(notificationCenter: center)
        let model = makeModel(
            notificationScheduler: scheduler,
            settings: AppSettings(
                syncMode: .manual,
                selectedCalendarIDs: [],
                selectedTaskListIDs: [],
                enableLocalNotifications: false
            )
        )

        let outcome = await model.requestEnableLocalNotifications()

        XCTAssertEqual(outcome, .authorized)
        XCTAssertTrue(center.didRequestAuthorization)
        XCTAssertTrue(model.settings.enableLocalNotifications)
    }

    @MainActor
    func testAppModelRequestEnableLocalNotificationsDenialTurnsSettingOffAndClearsRequests() async {
        let center = FakeUserNotificationCenter()
        center.currentAuthorizationStatus = .notDetermined
        center.authorizationRequestResult = false
        center.pendingRequests = [
            makePendingRequest(identifier: "hot-cross-buns.task.existing"),
            makePendingRequest(identifier: "hot-cross-buns.event.existing"),
            makePendingRequest(identifier: "other.app.existing")
        ]
        let scheduler = LocalNotificationScheduler(notificationCenter: center)
        let model = makeModel(
            notificationScheduler: scheduler,
            settings: AppSettings(
                syncMode: .manual,
                selectedCalendarIDs: [],
                selectedTaskListIDs: [],
                enableLocalNotifications: true
            )
        )

        let outcome = await model.requestEnableLocalNotifications()

        XCTAssertEqual(outcome, .denied)
        XCTAssertTrue(center.didRequestAuthorization)
        XCTAssertFalse(model.settings.enableLocalNotifications)
        XCTAssertEqual(center.removedPendingIdentifiers, [
            "hot-cross-buns.task.existing",
            "hot-cross-buns.event.existing"
        ])
        XCTAssertEqual(center.removedDeliveredIdentifiers, [
            "hot-cross-buns.task.existing",
            "hot-cross-buns.event.existing"
        ])
    }

    private var referenceDate: Date {
        var components = DateComponents()
        components.year = 2026
        components.month = 4
        components.day = 24
        components.hour = 9
        components.minute = 0
        components.timeZone = TimeZone(secondsFromGMT: 0)
        return Calendar(identifier: .gregorian).date(from: components)!
    }

    private func makeTask(id: String, dueOffsetDays: Int) -> TaskMirror {
        let dueDate = Calendar(identifier: .gregorian).date(byAdding: .day, value: dueOffsetDays, to: referenceDate)!
        return TaskMirror(
            id: id,
            taskListID: "tasks",
            parentID: nil,
            title: "Task \(id)",
            notes: "",
            status: .needsAction,
            dueDate: dueDate,
            completedAt: nil,
            isDeleted: false,
            isHidden: false,
            position: nil,
            etag: nil,
            updatedAt: referenceDate
        )
    }

    private func makeEvent(
        id: String,
        startOffsetDays: Int,
        isAllDay: Bool,
        reminderMinutes: [Int] = [],
        status: CalendarEventStatus = .confirmed
    ) -> CalendarEventMirror {
        let calendar = Calendar(identifier: .gregorian)
        let startBase = calendar.date(byAdding: .day, value: startOffsetDays, to: referenceDate)!
        let startDate: Date
        let endDate: Date
        if isAllDay {
            startDate = calendar.startOfDay(for: startBase)
            endDate = calendar.date(byAdding: .day, value: 1, to: startDate)!
        } else {
            startDate = calendar.date(bySettingHour: 10, minute: 0, second: 0, of: startBase)!
            endDate = calendar.date(byAdding: .hour, value: 1, to: startDate)!
        }

        return CalendarEventMirror(
            id: id,
            calendarID: "calendar",
            summary: "Event \(id)",
            details: "",
            startDate: startDate,
            endDate: endDate,
            isAllDay: isAllDay,
            status: status,
            recurrence: [],
            etag: nil,
            updatedAt: referenceDate,
            reminderMinutes: reminderMinutes,
            usedDefaultReminders: false
        )
    }

    private func makePendingRequest(identifier: String) -> UNNotificationRequest {
        let content = UNMutableNotificationContent()
        content.title = "Existing"
        return UNNotificationRequest(
            identifier: identifier,
            content: content,
            trigger: UNTimeIntervalNotificationTrigger(timeInterval: 60, repeats: false)
        )
    }

    @MainActor
    private func makeModel(
        notificationScheduler: LocalNotificationScheduler,
        settings: AppSettings
    ) -> AppModel {
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: StaticAccessTokenProvider(token: "test-token")
        )
        let tasksClient = GoogleTasksClient(transport: transport)
        let calendarClient = GoogleCalendarClient(transport: transport)
        return AppModel(
            authService: GoogleAuthService(),
            tasksClient: tasksClient,
            calendarClient: calendarClient,
            syncScheduler: SyncScheduler(tasksClient: tasksClient, calendarClient: calendarClient),
            cacheStore: LocalCacheStore(fileURL: nil),
            notificationScheduler: notificationScheduler,
            settings: settings
        )
    }
}

private final class FakeUserNotificationCenter: UserNotificationCentering {
    var currentAuthorizationStatus: UNAuthorizationStatus = .authorized
    var authorizationRequestResult = true
    var didRequestAuthorization = false
    var pendingRequests: [UNNotificationRequest] = []
    var addedRequests: [UNNotificationRequest] = []
    var removedPendingIdentifiers: [String] = []
    var removedDeliveredIdentifiers: [String] = []

    func authorizationStatus() async -> UNAuthorizationStatus {
        currentAuthorizationStatus
    }

    func pendingNotificationRequests() async -> [UNNotificationRequest] {
        pendingRequests
    }

    func requestAuthorization(options: UNAuthorizationOptions) async throws -> Bool {
        didRequestAuthorization = true
        return authorizationRequestResult
    }

    func add(_ request: UNNotificationRequest) async throws {
        addedRequests.append(request)
    }

    func removePendingNotificationRequests(withIdentifiers identifiers: [String]) {
        removedPendingIdentifiers = identifiers
        pendingRequests.removeAll { identifiers.contains($0.identifier) }
    }

    func removeDeliveredNotifications(withIdentifiers identifiers: [String]) {
        removedDeliveredIdentifiers = identifiers
    }
}
