import Foundation
import UserNotifications

protocol UserNotificationCentering: AnyObject {
    func authorizationStatus() async -> UNAuthorizationStatus
    func pendingNotificationRequests() async -> [UNNotificationRequest]
    func requestAuthorization(options: UNAuthorizationOptions) async throws -> Bool
    func add(_ request: UNNotificationRequest) async throws
    func removePendingNotificationRequests(withIdentifiers identifiers: [String])
    func removeDeliveredNotifications(withIdentifiers identifiers: [String])
}

extension UNUserNotificationCenter: UserNotificationCentering {
    func authorizationStatus() async -> UNAuthorizationStatus {
        await notificationSettings().authorizationStatus
    }
}

// macOS / iOS cap at 64 pending UNNotificationRequest slots per app.
// Previously we took the 64 nearest-sortDate items from *every*
// task/event the user owned — a year-out event would never schedule if
// 64 nearer items existed, silently dropping distant reminders.
//
// We now cap on a 30-day rolling window. Items beyond the window will
// be scheduled on the next sync that moves the window past their
// trigger. If the window still overflows (>64 items in 30 days, rare
// for daily-driver use), we tier: events first (they have exact
// scheduled times), then task reminders. A summary of the last
// scheduling pass is exposed so DiagnosticsView can flag truncation.
struct NotificationScheduleSummary: Equatable, Sendable {
    var scheduledEvents: Int
    var scheduledTasks: Int
    var deferredEvents: Int
    var deferredTasks: Int
    var failedEvents: Int
    var failedTasks: Int
    var windowDays: Int
    var computedAt: Date

    var hasDeferred: Bool { deferredEvents + deferredTasks > 0 }
    var hasFailures: Bool { failedEvents + failedTasks > 0 }
    var totalScheduled: Int { scheduledEvents + scheduledTasks }
    var totalFailed: Int { failedEvents + failedTasks }
}

enum NotificationAuthorizationOutcome: Equatable, Sendable {
    case authorized
    case denied
    case notDetermined
}

actor LocalNotificationScheduler {
    private static let notificationPrefix = "hot-cross-buns."
    private static let schedulingWindowDays = 30
    private static let pendingRequestLimit = 64
    private let notificationCenter: UserNotificationCentering
    private(set) var lastSummary: NotificationScheduleSummary?

    init(notificationCenter: UserNotificationCentering = UNUserNotificationCenter.current()) {
        self.notificationCenter = notificationCenter
    }

    func synchronize(
        tasks: [TaskMirror],
        events: [CalendarEventMirror],
        settings: AppSettings,
        requestAuthorization: Bool = false,
        referenceDate: Date = Date(),
        calendar: Calendar = .current
    ) async {
        guard settings.enableLocalNotifications else {
            await removeScheduledNotifications()
            lastSummary = nil
            return
        }

        let isAuthorized = await hasAuthorization(requestAuthorization: requestAuthorization)
        guard isAuthorized else {
            return
        }

        await removeScheduledNotifications()

        let windowEnd = calendar.date(byAdding: .day, value: Self.schedulingWindowDays, to: referenceDate) ?? referenceDate
        let eventNotifications = events
            .compactMap { eventNotificationRequest(for: $0, referenceDate: referenceDate, calendar: calendar) }
            .filter { $0.sortDate < windowEnd }
            .sorted { $0.sortDate < $1.sortDate }
        let taskNotifications = tasks
            .flatMap { taskNotificationRequests(for: $0, settings: settings, referenceDate: referenceDate, calendar: calendar) }
            .filter { $0.sortDate < windowEnd }
            .sorted { $0.sortDate < $1.sortDate }

        // Events first (they have exact times users rely on); tasks fill
        // the rest up to the OS cap. Anything beyond is deferred until the
        // next sync pushes the window forward.
        let eventSlice = Array(eventNotifications.prefix(Self.pendingRequestLimit))
        let remainingSlots = max(0, Self.pendingRequestLimit - eventSlice.count)
        let taskSlice = Array(taskNotifications.prefix(remainingSlots))

        var failedEvents = 0
        var failedTasks = 0
        let eventIdentifiers = Set(eventSlice.map(\.request.identifier))
        for notification in eventSlice + taskSlice {
            do {
                try await add(notification.request)
            } catch {
                if eventIdentifiers.contains(notification.request.identifier) {
                    failedEvents += 1
                } else {
                    failedTasks += 1
                }
                AppLogger.warn("notification add failed", category: .notifications, metadata: [
                    "identifier": notification.request.identifier,
                    "error": String(describing: error)
                ])
            }
        }
        let totalFailed = failedEvents + failedTasks
        if totalFailed > 0 {
            AppLogger.warn("notifications partial failure", category: .notifications, metadata: [
                "failed": String(totalFailed),
                "failedEvents": String(failedEvents),
                "failedTasks": String(failedTasks)
            ])
        } else if eventSlice.isEmpty == false || taskSlice.isEmpty == false {
            AppLogger.info("notifications scheduled", category: .notifications, metadata: [
                "events": String(eventSlice.count),
                "tasks": String(taskSlice.count)
            ])
        }

        lastSummary = NotificationScheduleSummary(
            scheduledEvents: eventSlice.count - failedEvents,
            scheduledTasks: taskSlice.count - failedTasks,
            deferredEvents: max(0, eventNotifications.count - eventSlice.count),
            deferredTasks: max(0, taskNotifications.count - taskSlice.count),
            failedEvents: failedEvents,
            failedTasks: failedTasks,
            windowDays: Self.schedulingWindowDays,
            computedAt: Date()
        )
    }

    func synchronizeDirty(
        changedTasks: [TaskMirror],
        deletedTaskIDs: Set<String>,
        changedEvents: [CalendarEventMirror],
        deletedEventIDs: Set<String>,
        settings: AppSettings,
        requestAuthorization: Bool = false,
        referenceDate: Date = Date(),
        calendar: Calendar = .current
    ) async {
        guard settings.enableLocalNotifications else {
            await removeScheduledNotifications()
            lastSummary = nil
            return
        }

        let isAuthorized = await hasAuthorization(requestAuthorization: requestAuthorization)
        guard isAuthorized else {
            return
        }

        let changedTaskIDs = Set(changedTasks.map(\.id))
        let changedEventIDs = Set(changedEvents.map(\.id))
        removeScheduledNotifications(
            taskIDs: deletedTaskIDs.union(changedTaskIDs),
            eventIDs: deletedEventIDs.union(changedEventIDs)
        )

        let windowEnd = calendar.date(byAdding: .day, value: Self.schedulingWindowDays, to: referenceDate) ?? referenceDate
        let eventNotifications = changedEvents
            .compactMap { eventNotificationRequest(for: $0, referenceDate: referenceDate, calendar: calendar) }
            .filter { $0.sortDate < windowEnd }
            .sorted { $0.sortDate < $1.sortDate }
        let taskNotifications = changedTasks
            .flatMap { taskNotificationRequests(for: $0, settings: settings, referenceDate: referenceDate, calendar: calendar) }
            .filter { $0.sortDate < windowEnd }
            .sorted { $0.sortDate < $1.sortDate }

        var failedEvents = 0
        var failedTasks = 0
        let eventIdentifiers = Set(eventNotifications.map(\.request.identifier))
        for notification in eventNotifications + taskNotifications {
            do {
                try await add(notification.request)
            } catch {
                if eventIdentifiers.contains(notification.request.identifier) {
                    failedEvents += 1
                } else {
                    failedTasks += 1
                }
            }
        }

        lastSummary = NotificationScheduleSummary(
            scheduledEvents: eventNotifications.count - failedEvents,
            scheduledTasks: taskNotifications.count - failedTasks,
            deferredEvents: 0,
            deferredTasks: 0,
            failedEvents: failedEvents,
            failedTasks: failedTasks,
            windowDays: Self.schedulingWindowDays,
            computedAt: Date()
        )
    }

    func authorizationOutcome(requestAuthorization: Bool) async -> NotificationAuthorizationOutcome {
        let authorizationStatus = await notificationCenter.authorizationStatus()

        switch authorizationStatus {
        case .authorized, .provisional, .ephemeral:
            return .authorized
        case .notDetermined where requestAuthorization:
            return await requestNotificationAuthorization() ? .authorized : .denied
        case .notDetermined:
            return .notDetermined
        default:
            return .denied
        }
    }

    private func hasAuthorization(requestAuthorization: Bool) async -> Bool {
        await authorizationOutcome(requestAuthorization: requestAuthorization) == .authorized
    }

    private func taskNotificationRequests(
        for task: TaskMirror,
        settings: AppSettings,
        referenceDate: Date,
        calendar: Calendar
    ) -> [ScheduledNotification] {
        guard task.isCompleted == false, task.isDeleted == false, let dueDate = task.dueDate else {
            return []
        }
        // App-wide threshold: one notification per task at (due - N days),
        // fired at the user's configured hour:minute. 0 disables entirely.
        let threshold = settings.taskReminderThresholdDays
        guard threshold > 0 else { return [] }
        guard let reminderDay = calendar.date(byAdding: .day, value: -threshold, to: dueDate) else { return [] }
        var components = calendar.dateComponents([.year, .month, .day], from: reminderDay)
        components.hour = settings.taskReminderHour
        components.minute = settings.taskReminderMinute

        guard let notificationDate = calendar.date(from: components), notificationDate > referenceDate else {
            return []
        }

        let content = UNMutableNotificationContent()
        content.title = titleForThreshold(days: threshold)
        content.body = task.title
        content.sound = .default

        let trigger = UNCalendarNotificationTrigger(dateMatching: components, repeats: false)
        let identifier = Self.notificationPrefix + "task." + task.id
        let request = UNNotificationRequest(identifier: identifier, content: content, trigger: trigger)
        return [ScheduledNotification(sortDate: notificationDate, request: request)]
    }

    private func titleForThreshold(days: Int) -> String {
        switch days {
        case 1: return "Task due tomorrow"
        case 7: return "Task due in a week"
        default: return "Task due in \(days) day\(days == 1 ? "" : "s")"
        }
    }

    private func eventNotificationRequest(
        for event: CalendarEventMirror,
        referenceDate: Date,
        calendar: Calendar
    ) -> ScheduledNotification? {
        guard event.status != .cancelled else {
            return nil
        }

        let leadMinutes: Int = event.reminderMinutes.first ?? 15
        let notificationDate = event.isAllDay
            ? allDayNotificationDate(for: event.startDate, calendar: calendar)
            : event.startDate.addingTimeInterval(TimeInterval(-leadMinutes * 60))

        guard notificationDate > referenceDate else {
            return nil
        }

        let content = UNMutableNotificationContent()
        content.title = event.isAllDay ? "All-day event today" : "Event starts soon"
        content.body = event.summary
        content.sound = .default

        let components = calendar.dateComponents([.year, .month, .day, .hour, .minute], from: notificationDate)
        let trigger = UNCalendarNotificationTrigger(dateMatching: components, repeats: false)
        let request = UNNotificationRequest(
            identifier: Self.notificationPrefix + "event." + event.id,
            content: content,
            trigger: trigger
        )
        return ScheduledNotification(sortDate: notificationDate, request: request)
    }

    private func allDayNotificationDate(for date: Date, calendar: Calendar) -> Date {
        var components = calendar.dateComponents([.year, .month, .day], from: date)
        components.hour = 9
        components.minute = 0
        return calendar.date(from: components) ?? date
    }

    private func removeScheduledNotifications() async {
        let pendingRequests = await pendingNotificationRequests()
        let identifiers = pendingRequests
            .map(\.identifier)
            .filter { $0.hasPrefix(Self.notificationPrefix) }

        guard identifiers.isEmpty == false else {
            return
        }

        notificationCenter.removePendingNotificationRequests(withIdentifiers: identifiers)
        notificationCenter.removeDeliveredNotifications(withIdentifiers: identifiers)
    }

    private func removeScheduledNotifications(taskIDs: Set<String>, eventIDs: Set<String>) {
        var identifiers = taskIDs.map { Self.notificationPrefix + "task." + $0 }
        identifiers += eventIDs.map { Self.notificationPrefix + "event." + $0 }
        let uniqueIdentifiers = Array(Set(identifiers))
        guard uniqueIdentifiers.isEmpty == false else { return }
        notificationCenter.removePendingNotificationRequests(withIdentifiers: uniqueIdentifiers)
        notificationCenter.removeDeliveredNotifications(withIdentifiers: uniqueIdentifiers)
    }

    private func pendingNotificationRequests() async -> [UNNotificationRequest] {
        await notificationCenter.pendingNotificationRequests()
    }

    private func requestNotificationAuthorization() async -> Bool {
        (try? await notificationCenter.requestAuthorization(options: [.alert, .badge, .sound])) ?? false
    }

    private func add(_ request: UNNotificationRequest) async throws {
        try await notificationCenter.add(request)
    }
}

private struct ScheduledNotification {
    var sortDate: Date
    var request: UNNotificationRequest
}
