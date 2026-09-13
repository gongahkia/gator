import Foundation

enum OptimisticID {
    static let prefix = "local-"

    static func generate() -> String {
        "\(prefix)\(UUID().uuidString)"
    }

    static func isPending(_ id: String) -> Bool {
        id.hasPrefix(prefix)
    }
}

struct PendingTaskCreatePayload: Codable, Sendable, Equatable {
    var localID: String
    var taskListID: String
    var title: String
    var notes: String
    var dueDate: Date?
    var parentID: String?
}

struct PendingEventCreatePayload: Codable, Sendable, Equatable {
    var localID: String
    var calendarID: String
    var summary: String
    var details: String
    var startDate: Date
    var endDate: Date
    var isAllDay: Bool
    var reminderMinutes: Int?
    var location: String = ""
    var recurrence: [String] = []
    var attendeeEmails: [String] = []
    var notifyGuests: Bool = false
    var addGoogleMeet: Bool = false
    var colorId: String? = nil
    var startTimeZoneID: String? = nil
    var endTimeZoneID: String? = nil
    var transparency: CalendarEventTransparency? = nil
    var visibility: CalendarEventVisibility? = nil
    // HCB-only backlink stored on the event's Google extendedProperties.private
    // bag (not visible in other Google clients). Nil for plain event creates;
    // set when the event was spawned by a task → time-block drag.
    var hcbTaskID: String? = nil
    var availabilityHold: AvailabilityHoldMetadata? = nil

    enum CodingKeys: String, CodingKey {
        case localID, calendarID, summary, details, startDate, endDate, isAllDay
        case reminderMinutes, location, recurrence, attendeeEmails, notifyGuests
        case addGoogleMeet, colorId, startTimeZoneID, endTimeZoneID
        case transparency, visibility, hcbTaskID, availabilityHold
    }

    init(
        localID: String,
        calendarID: String,
        summary: String,
        details: String,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        reminderMinutes: Int?,
        location: String = "",
        recurrence: [String] = [],
        attendeeEmails: [String] = [],
        notifyGuests: Bool = false,
        addGoogleMeet: Bool = false,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        transparency: CalendarEventTransparency? = nil,
        visibility: CalendarEventVisibility? = nil,
        hcbTaskID: String? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil
    ) {
        self.localID = localID
        self.calendarID = calendarID
        self.summary = summary
        self.details = details
        self.startDate = startDate
        self.endDate = endDate
        self.isAllDay = isAllDay
        self.reminderMinutes = reminderMinutes
        self.location = location
        self.recurrence = recurrence
        self.attendeeEmails = attendeeEmails
        self.notifyGuests = notifyGuests
        self.addGoogleMeet = addGoogleMeet
        self.colorId = colorId
        self.startTimeZoneID = TimezoneSupport.validatedIdentifier(startTimeZoneID)
        self.endTimeZoneID = TimezoneSupport.validatedIdentifier(endTimeZoneID) ?? TimezoneSupport.validatedIdentifier(startTimeZoneID)
        self.transparency = transparency
        self.visibility = visibility
        self.hcbTaskID = hcbTaskID
        self.availabilityHold = availabilityHold
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        localID = try c.decode(String.self, forKey: .localID)
        calendarID = try c.decode(String.self, forKey: .calendarID)
        summary = try c.decode(String.self, forKey: .summary)
        details = try c.decode(String.self, forKey: .details)
        startDate = try c.decode(Date.self, forKey: .startDate)
        endDate = try c.decode(Date.self, forKey: .endDate)
        isAllDay = try c.decode(Bool.self, forKey: .isAllDay)
        reminderMinutes = try c.decodeIfPresent(Int.self, forKey: .reminderMinutes)
        location = try c.decodeIfPresent(String.self, forKey: .location) ?? ""
        recurrence = try c.decodeIfPresent([String].self, forKey: .recurrence) ?? []
        attendeeEmails = try c.decodeIfPresent([String].self, forKey: .attendeeEmails) ?? []
        notifyGuests = try c.decodeIfPresent(Bool.self, forKey: .notifyGuests) ?? false
        addGoogleMeet = try c.decodeIfPresent(Bool.self, forKey: .addGoogleMeet) ?? false
        colorId = try c.decodeIfPresent(String.self, forKey: .colorId)
        startTimeZoneID = TimezoneSupport.validatedIdentifier(try c.decodeIfPresent(String.self, forKey: .startTimeZoneID))
        endTimeZoneID = TimezoneSupport.validatedIdentifier(try c.decodeIfPresent(String.self, forKey: .endTimeZoneID)) ?? startTimeZoneID
        transparency = try c.decodeIfPresent(CalendarEventTransparency.self, forKey: .transparency)
        visibility = try c.decodeIfPresent(CalendarEventVisibility.self, forKey: .visibility)
        hcbTaskID = try c.decodeIfPresent(String.self, forKey: .hcbTaskID)
        availabilityHold = try c.decodeIfPresent(AvailabilityHoldMetadata.self, forKey: .availabilityHold)
    }
}

struct PendingTaskUpdatePayload: Codable, Sendable, Equatable {
    var taskListID: String
    var taskID: String
    var title: String
    var notes: String
    var dueDate: Date?
    var etagSnapshot: String?
}

struct PendingTaskCompletionPayload: Codable, Sendable, Equatable {
    var taskListID: String
    var taskID: String
    var isCompleted: Bool
    var etagSnapshot: String?
}

struct PendingTaskDeletePayload: Codable, Sendable, Equatable {
    var taskListID: String
    var taskID: String
    var etagSnapshot: String?
}

struct PendingEventUpdatePayload: Codable, Sendable, Equatable {
    var calendarID: String
    var eventID: String
    var summary: String
    var details: String
    var startDate: Date
    var endDate: Date
    var isAllDay: Bool
    var reminderMinutes: Int?
    var location: String
    var recurrence: [String]
    var attendeeEmails: [String]
    var notifyGuests: Bool
    var etagSnapshot: String?
    var addGoogleMeet: Bool = false
    var colorId: String? = nil
    var startTimeZoneID: String? = nil
    var endTimeZoneID: String? = nil
    var hcbTaskID: String? = nil
    var transparency: CalendarEventTransparency? = nil
    var visibility: CalendarEventVisibility? = nil
    var availabilityHold: AvailabilityHoldMetadata? = nil
    var clearAvailabilityHoldMetadata: Bool = false

    enum CodingKeys: String, CodingKey {
        case calendarID, eventID, summary, details, startDate, endDate, isAllDay
        case reminderMinutes, location, recurrence, attendeeEmails, notifyGuests
        case etagSnapshot, addGoogleMeet, colorId, startTimeZoneID, endTimeZoneID, hcbTaskID
        case transparency, visibility, availabilityHold, clearAvailabilityHoldMetadata
    }

    init(
        calendarID: String,
        eventID: String,
        summary: String,
        details: String,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        reminderMinutes: Int?,
        location: String,
        recurrence: [String],
        attendeeEmails: [String],
        notifyGuests: Bool,
        etagSnapshot: String?,
        addGoogleMeet: Bool = false,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        hcbTaskID: String? = nil,
        transparency: CalendarEventTransparency? = nil,
        visibility: CalendarEventVisibility? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil,
        clearAvailabilityHoldMetadata: Bool = false
    ) {
        self.calendarID = calendarID
        self.eventID = eventID
        self.summary = summary
        self.details = details
        self.startDate = startDate
        self.endDate = endDate
        self.isAllDay = isAllDay
        self.reminderMinutes = reminderMinutes
        self.location = location
        self.recurrence = recurrence
        self.attendeeEmails = attendeeEmails
        self.notifyGuests = notifyGuests
        self.etagSnapshot = etagSnapshot
        self.addGoogleMeet = addGoogleMeet
        self.colorId = colorId
        self.startTimeZoneID = TimezoneSupport.validatedIdentifier(startTimeZoneID)
        self.endTimeZoneID = TimezoneSupport.validatedIdentifier(endTimeZoneID) ?? TimezoneSupport.validatedIdentifier(startTimeZoneID)
        self.hcbTaskID = hcbTaskID
        self.transparency = transparency
        self.visibility = visibility
        self.availabilityHold = availabilityHold
        self.clearAvailabilityHoldMetadata = clearAvailabilityHoldMetadata
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        calendarID = try c.decode(String.self, forKey: .calendarID)
        eventID = try c.decode(String.self, forKey: .eventID)
        summary = try c.decode(String.self, forKey: .summary)
        details = try c.decode(String.self, forKey: .details)
        startDate = try c.decode(Date.self, forKey: .startDate)
        endDate = try c.decode(Date.self, forKey: .endDate)
        isAllDay = try c.decode(Bool.self, forKey: .isAllDay)
        reminderMinutes = try c.decodeIfPresent(Int.self, forKey: .reminderMinutes)
        location = try c.decode(String.self, forKey: .location)
        recurrence = try c.decode([String].self, forKey: .recurrence)
        attendeeEmails = try c.decode([String].self, forKey: .attendeeEmails)
        notifyGuests = try c.decode(Bool.self, forKey: .notifyGuests)
        etagSnapshot = try c.decodeIfPresent(String.self, forKey: .etagSnapshot)
        addGoogleMeet = try c.decodeIfPresent(Bool.self, forKey: .addGoogleMeet) ?? false
        colorId = try c.decodeIfPresent(String.self, forKey: .colorId)
        startTimeZoneID = TimezoneSupport.validatedIdentifier(try c.decodeIfPresent(String.self, forKey: .startTimeZoneID))
        endTimeZoneID = TimezoneSupport.validatedIdentifier(try c.decodeIfPresent(String.self, forKey: .endTimeZoneID)) ?? startTimeZoneID
        hcbTaskID = try c.decodeIfPresent(String.self, forKey: .hcbTaskID)
        transparency = try c.decodeIfPresent(CalendarEventTransparency.self, forKey: .transparency)
        visibility = try c.decodeIfPresent(CalendarEventVisibility.self, forKey: .visibility)
        availabilityHold = try c.decodeIfPresent(AvailabilityHoldMetadata.self, forKey: .availabilityHold)
        clearAvailabilityHoldMetadata = try c.decodeIfPresent(Bool.self, forKey: .clearAvailabilityHoldMetadata) ?? false
    }
}

struct PendingEventDeletePayload: Codable, Sendable, Equatable {
    var calendarID: String
    var eventID: String
    var etagSnapshot: String?
}

enum PendingMutationEncoder {
    static func encode(_ payload: PendingTaskCreatePayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func encode(_ payload: PendingEventCreatePayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func encode(_ payload: PendingTaskUpdatePayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func encode(_ payload: PendingTaskCompletionPayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func encode(_ payload: PendingTaskDeletePayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func encode(_ payload: PendingEventUpdatePayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func encode(_ payload: PendingEventDeletePayload) throws -> Data {
        try JSONEncoder().encode(payload)
    }

    static func decodeTaskCreate(_ data: Data) throws -> PendingTaskCreatePayload {
        try JSONDecoder().decode(PendingTaskCreatePayload.self, from: data)
    }

    static func decodeEventCreate(_ data: Data) throws -> PendingEventCreatePayload {
        try JSONDecoder().decode(PendingEventCreatePayload.self, from: data)
    }

    static func decodeTaskUpdate(_ data: Data) throws -> PendingTaskUpdatePayload {
        try JSONDecoder().decode(PendingTaskUpdatePayload.self, from: data)
    }

    static func decodeTaskCompletion(_ data: Data) throws -> PendingTaskCompletionPayload {
        try JSONDecoder().decode(PendingTaskCompletionPayload.self, from: data)
    }

    static func decodeTaskDelete(_ data: Data) throws -> PendingTaskDeletePayload {
        try JSONDecoder().decode(PendingTaskDeletePayload.self, from: data)
    }

    static func decodeEventUpdate(_ data: Data) throws -> PendingEventUpdatePayload {
        try JSONDecoder().decode(PendingEventUpdatePayload.self, from: data)
    }

    static func decodeEventDelete(_ data: Data) throws -> PendingEventDeletePayload {
        try JSONDecoder().decode(PendingEventDeletePayload.self, from: data)
    }
}

enum PendingMutationReplayOutcome: Sendable {
    case accepted(acceptedID: String)
    case retryLater
    case terminal(message: String)
}

extension PendingMutation {
    static func taskCreate(payload: PendingTaskCreatePayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .task,
            resourceID: payload.localID,
            action: .create,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }

    static func eventCreate(payload: PendingEventCreatePayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .event,
            resourceID: payload.localID,
            action: .create,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }

    static func taskUpdate(payload: PendingTaskUpdatePayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .task,
            resourceID: payload.taskID,
            action: .update,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }

    static func taskCompletion(payload: PendingTaskCompletionPayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .task,
            resourceID: payload.taskID,
            action: .completion,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }

    static func taskDelete(payload: PendingTaskDeletePayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .task,
            resourceID: payload.taskID,
            action: .delete,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }

    static func eventUpdate(payload: PendingEventUpdatePayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .event,
            resourceID: payload.eventID,
            action: .update,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }

    static func eventDelete(payload: PendingEventDeletePayload) throws -> PendingMutation {
        PendingMutation(
            id: UUID(),
            createdAt: Date(),
            resourceType: .event,
            resourceID: payload.eventID,
            action: .delete,
            payload: try PendingMutationEncoder.encode(payload)
        )
    }
}

extension GoogleAPIError {
    var isTransient: Bool {
        switch self {
        case .preconditionFailed, .invalidURL, .invalidResponse, .invalidPayload:
            return false
        case .httpStatus(let status, let body):
            if GoogleAPIError.isQuotaBody(body) {
                return false
            }
            return status == 429 || (500...599).contains(status)
        }
    }
}
