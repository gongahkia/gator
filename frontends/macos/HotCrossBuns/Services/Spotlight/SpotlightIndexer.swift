@preconcurrency import CoreSpotlight
import Foundation
import UniformTypeIdentifiers

protocol SpotlightIndexing {
    func deleteSearchableItems(withDomainIdentifiers domainIdentifiers: [String]) async throws
    func deleteSearchableItems(withIdentifiers identifiers: [String]) async throws
    func indexSearchableItems(_ items: [CSSearchableItem]) async throws
}

extension CSSearchableIndex: SpotlightIndexing {}

struct SpotlightIndexingSummary: Equatable, Sendable {
    var updatedAt: Date?
    var durationMilliseconds: Double?
    var indexedTaskCount: Int
    var indexedEventCount: Int
    var deletedItemCount: Int
    var rebuiltDomains: Bool
    var trackedTaskCount: Int
    var trackedEventCount: Int
}

actor SpotlightIndexer {
    static let taskDomain = "com.gongahkia.hotcrossbuns.tasks"
    static let eventDomain = "com.gongahkia.hotcrossbuns.events"
    static let taskURLScheme = "hotcrossbuns://task/"
    static let eventURLScheme = "hotcrossbuns://event/"

    private let index: SpotlightIndexing
    private var didPrimeDomains = false
    private var isApplyingUpdate = false
    private var pendingUpdate: SpotlightUpdatePayload?
    private var taskFingerprints: [String: Int] = [:]
    private var eventFingerprints: [String: Int] = [:]
    private var lastSummary = SpotlightIndexingSummary(
        updatedAt: nil,
        durationMilliseconds: nil,
        indexedTaskCount: 0,
        indexedEventCount: 0,
        deletedItemCount: 0,
        rebuiltDomains: false,
        trackedTaskCount: 0,
        trackedEventCount: 0
    )

    init(index: SpotlightIndexing = CSSearchableIndex.default()) {
        self.index = index
    }

    func summary() -> SpotlightIndexingSummary {
        lastSummary
    }

    func isPrimed() -> Bool {
        didPrimeDomains
    }

    func update(tasks: [TaskMirror], events: [CalendarEventMirror]) async {
        pendingUpdate = SpotlightUpdatePayload(tasks: tasks, events: events)
        guard isApplyingUpdate == false else { return }

        isApplyingUpdate = true
        defer { isApplyingUpdate = false }

        while let update = pendingUpdate {
            pendingUpdate = nil
            await applyUpdate(tasks: update.tasks, events: update.events)
        }
    }

    func applyDirty(
        changedTasks: [TaskMirror],
        deletedTaskIDs: Set<String>,
        changedEvents: [CalendarEventMirror],
        deletedEventIDs: Set<String>
    ) async {
        guard didPrimeDomains else {
            return
        }
        let started = Date()
        do {
            var deletedIdentifiers: [String] = []
            deletedIdentifiers += deletedTaskIDs.map { Self.taskURLScheme + $0 }
            deletedIdentifiers += deletedEventIDs.map { Self.eventURLScheme + $0 }

            let activeChangedTasks = changedTasks.filter { $0.isDeleted == false }
            let removedChangedTaskIDs = Set(changedTasks.filter(\.isDeleted).map(\.id))
            deletedIdentifiers += removedChangedTaskIDs.map { Self.taskURLScheme + $0 }

            let activeChangedEvents = changedEvents.filter { $0.status != .cancelled }
            let removedChangedEventIDs = Set(changedEvents.filter { $0.status == .cancelled }.map(\.id))
            deletedIdentifiers += removedChangedEventIDs.map { Self.eventURLScheme + $0 }

            let uniqueDeletedIdentifiers = Array(Set(deletedIdentifiers))
            if uniqueDeletedIdentifiers.isEmpty == false {
                try await index.deleteSearchableItems(withIdentifiers: uniqueDeletedIdentifiers)
            }

            if activeChangedTasks.isEmpty == false {
                try await index.indexSearchableItems(activeChangedTasks.map(taskItem))
            }
            if activeChangedEvents.isEmpty == false {
                try await index.indexSearchableItems(activeChangedEvents.map(eventItem))
            }

            for id in deletedTaskIDs.union(removedChangedTaskIDs) {
                taskFingerprints[id] = nil
            }
            for task in activeChangedTasks {
                taskFingerprints[task.id] = Self.taskFingerprint(task)
            }
            for id in deletedEventIDs.union(removedChangedEventIDs) {
                eventFingerprints[id] = nil
            }
            for event in activeChangedEvents {
                eventFingerprints[event.id] = Self.eventFingerprint(event)
            }

            recordSummary(
                started: started,
                indexedTaskCount: activeChangedTasks.count,
                indexedEventCount: activeChangedEvents.count,
                deletedItemCount: uniqueDeletedIdentifiers.count,
                rebuiltDomains: false
            )
        } catch {
            // best effort
        }
    }

    private func applyUpdate(tasks: [TaskMirror], events: [CalendarEventMirror]) async {
        let started = Date()
        let activeTasks = tasks
            .filter { $0.isDeleted == false }
        let activeEvents = events
            .filter { $0.status != .cancelled }
        let nextTaskFingerprints = Dictionary(uniqueKeysWithValues: activeTasks.map { ($0.id, Self.taskFingerprint($0)) })
        let nextEventFingerprints = Dictionary(uniqueKeysWithValues: activeEvents.map { ($0.id, Self.eventFingerprint($0)) })

        do {
            if didPrimeDomains == false {
                try await replace(items: activeTasks.map(taskItem), in: Self.taskDomain)
                try await replace(items: activeEvents.map(eventItem), in: Self.eventDomain)
                didPrimeDomains = true
                taskFingerprints = nextTaskFingerprints
                eventFingerprints = nextEventFingerprints
                recordSummary(
                    started: started,
                    indexedTaskCount: activeTasks.count,
                    indexedEventCount: activeEvents.count,
                    deletedItemCount: 0,
                    rebuiltDomains: true
                )
                return
            }

            let removedTaskCount = try await deleteRemovedItems(
                previous: taskFingerprints,
                next: nextTaskFingerprints,
                urlPrefix: Self.taskURLScheme
            )
            let removedEventCount = try await deleteRemovedItems(
                previous: eventFingerprints,
                next: nextEventFingerprints,
                urlPrefix: Self.eventURLScheme
            )

            let changedTasks = activeTasks.filter { taskFingerprints[$0.id] != nextTaskFingerprints[$0.id] }
            let changedEvents = activeEvents.filter { eventFingerprints[$0.id] != nextEventFingerprints[$0.id] }
            if changedTasks.isEmpty == false {
                try await index.indexSearchableItems(changedTasks.map(taskItem))
            }
            if changedEvents.isEmpty == false {
                try await index.indexSearchableItems(changedEvents.map(eventItem))
            }
            taskFingerprints = nextTaskFingerprints
            eventFingerprints = nextEventFingerprints
            recordSummary(
                started: started,
                indexedTaskCount: changedTasks.count,
                indexedEventCount: changedEvents.count,
                deletedItemCount: removedTaskCount + removedEventCount,
                rebuiltDomains: false
            )
        } catch {
            // Spotlight is best-effort; ignore indexing failures rather than surface to the user.
        }
    }

    func removeAll() async {
        do {
            try await index.deleteSearchableItems(withDomainIdentifiers: [Self.taskDomain, Self.eventDomain])
            didPrimeDomains = false
            taskFingerprints = [:]
            eventFingerprints = [:]
            lastSummary = SpotlightIndexingSummary(
                updatedAt: Date(),
                durationMilliseconds: nil,
                indexedTaskCount: 0,
                indexedEventCount: 0,
                deletedItemCount: 0,
                rebuiltDomains: true,
                trackedTaskCount: 0,
                trackedEventCount: 0
            )
        } catch {
            // best effort
        }
    }

    private func replace(items: [CSSearchableItem], in domain: String) async throws {
        try await index.deleteSearchableItems(withDomainIdentifiers: [domain])
        guard items.isEmpty == false else { return }
        try await index.indexSearchableItems(items)
    }

    private func deleteRemovedItems(previous: [String: Int], next: [String: Int], urlPrefix: String) async throws -> Int {
        let removed = previous.keys.filter { next[$0] == nil }.map { urlPrefix + $0 }
        guard removed.isEmpty == false else { return 0 }
        try await index.deleteSearchableItems(withIdentifiers: removed)
        return removed.count
    }

    private func recordSummary(
        started: Date,
        indexedTaskCount: Int,
        indexedEventCount: Int,
        deletedItemCount: Int,
        rebuiltDomains: Bool
    ) {
        lastSummary = SpotlightIndexingSummary(
            updatedAt: Date(),
            durationMilliseconds: Date().timeIntervalSince(started) * 1000,
            indexedTaskCount: indexedTaskCount,
            indexedEventCount: indexedEventCount,
            deletedItemCount: deletedItemCount,
            rebuiltDomains: rebuiltDomains,
            trackedTaskCount: taskFingerprints.count,
            trackedEventCount: eventFingerprints.count
        )
    }

    nonisolated private static func taskFingerprint(_ task: TaskMirror) -> Int {
        var hasher = Hasher()
        hasher.combine(task.id)
        hasher.combine(task.title)
        hasher.combine(task.notes)
        hasher.combine(task.status)
        hasher.combine(task.dueDate)
        hasher.combine(task.completedAt)
        hasher.combine(task.isHidden)
        hasher.combine(task.etag)
        hasher.combine(task.updatedAt)
        return hasher.finalize()
    }

    nonisolated private static func eventFingerprint(_ event: CalendarEventMirror) -> Int {
        var hasher = Hasher()
        hasher.combine(event.id)
        hasher.combine(event.summary)
        hasher.combine(event.details)
        hasher.combine(event.startDate)
        hasher.combine(event.endDate)
        hasher.combine(event.isAllDay)
        hasher.combine(event.status)
        hasher.combine(event.etag)
        hasher.combine(event.updatedAt)
        hasher.combine(event.location)
        hasher.combine(event.attendeeEmails)
        hasher.combine(event.meetLink)
        return hasher.finalize()
    }

    nonisolated private func taskItem(_ task: TaskMirror) -> CSSearchableItem {
        let attrs = CSSearchableItemAttributeSet(contentType: UTType.text)
        attrs.title = task.title
        attrs.contentDescription = task.notes.isEmpty ? nil : task.notes
        attrs.textContent = Self.taskTextContent(task)
        attrs.dueDate = task.dueDate
        attrs.displayName = task.title
        attrs.keywords = ["task", task.status.rawValue]
        attrs.contentURL = URL(string: Self.taskURLScheme + task.id)
        attrs.contentModificationDate = task.updatedAt
        attrs.metadataModificationDate = task.updatedAt

        return CSSearchableItem(
            uniqueIdentifier: Self.taskURLScheme + task.id,
            domainIdentifier: Self.taskDomain,
            attributeSet: attrs
        )
    }

    nonisolated private func eventItem(_ event: CalendarEventMirror) -> CSSearchableItem {
        let attrs = CSSearchableItemAttributeSet(contentType: UTType.calendarEvent)
        attrs.title = event.summary
        attrs.contentDescription = event.details.isEmpty ? nil : event.details
        attrs.textContent = Self.eventTextContent(event)
        attrs.startDate = event.startDate
        attrs.endDate = event.endDate
        attrs.allDay = event.isAllDay as NSNumber
        attrs.displayName = event.summary
        attrs.keywords = eventKeywords(for: event)
        attrs.contentURL = URL(string: Self.eventURLScheme + event.id)
        attrs.contentModificationDate = event.updatedAt
        attrs.metadataModificationDate = event.updatedAt
        if event.location.isEmpty == false {
            attrs.namedLocation = event.location
        }
        if event.attendeeEmails.isEmpty == false {
            attrs.recipientEmailAddresses = event.attendeeEmails
        }

        return CSSearchableItem(
            uniqueIdentifier: Self.eventURLScheme + event.id,
            domainIdentifier: Self.eventDomain,
            attributeSet: attrs
        )
    }

    nonisolated private func eventKeywords(for event: CalendarEventMirror) -> [String] {
        var keywords = ["event", event.status.rawValue]
        if event.meetLink.isEmpty == false { keywords.append("meet") }
        if event.attendeeEmails.isEmpty == false { keywords.append("meeting") }
        if event.isAllDay { keywords.append("all-day") }
        return keywords
    }

    // Spotlight shows the inline preview (cmd-space → space) using
    // textContent. Assembling a rich single-blurb here gives users the
    // full "who / when / where / link" summary without leaving Spotlight.
    nonisolated static func taskTextContent(_ task: TaskMirror) -> String {
        var parts: [String] = []
        if let due = task.dueDate {
            parts.append("Due \(due.formatted(date: .abbreviated, time: .omitted))")
        }
        if task.isCompleted {
            parts.append("Completed")
        }
        if task.notes.isEmpty == false {
            parts.append(task.notes)
        }
        return parts.joined(separator: "\n")
    }

    nonisolated static func eventTextContent(_ event: CalendarEventMirror) -> String {
        var parts: [String] = []
        let timeLabel: String
        if event.isAllDay {
            let inclusiveEnd = Calendar.current.date(byAdding: .day, value: -1, to: event.endDate) ?? event.endDate
            let startString = event.startDate.formatted(date: .abbreviated, time: .omitted)
            let endString = inclusiveEnd.formatted(date: .abbreviated, time: .omitted)
            timeLabel = startString == endString ? "All day · \(startString)" : "All day · \(startString) – \(endString)"
        } else {
            let startString = event.startDate.formatted(date: .abbreviated, time: .shortened)
            let endString = event.endDate.formatted(date: .omitted, time: .shortened)
            timeLabel = "\(startString) – \(endString)"
        }
        parts.append(timeLabel)
        if event.location.isEmpty == false {
            parts.append("📍 \(event.location)")
        }
        if event.meetLink.isEmpty == false {
            parts.append("📹 \(event.meetLink)")
        }
        if event.attendeeEmails.isEmpty == false {
            parts.append("👥 \(event.attendeeEmails.joined(separator: ", "))")
        }
        if event.details.isEmpty == false {
            parts.append(event.details)
        }
        return parts.joined(separator: "\n")
    }
}

private struct SpotlightUpdatePayload: Sendable {
    var tasks: [TaskMirror]
    var events: [CalendarEventMirror]
}

enum SpotlightIdentifier {
    case task(String)
    case event(String)

    init?(uniqueIdentifier: String) {
        if uniqueIdentifier.hasPrefix(SpotlightIndexer.taskURLScheme) {
            let id = String(uniqueIdentifier.dropFirst(SpotlightIndexer.taskURLScheme.count))
            self = .task(id)
        } else if uniqueIdentifier.hasPrefix(SpotlightIndexer.eventURLScheme) {
            let id = String(uniqueIdentifier.dropFirst(SpotlightIndexer.eventURLScheme.count))
            self = .event(id)
        } else {
            return nil
        }
    }
}
