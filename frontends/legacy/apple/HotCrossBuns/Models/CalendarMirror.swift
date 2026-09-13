import Foundation

struct CalendarListMirror: Identifiable, Hashable, Codable, Sendable {
    let id: String
    var summary: String
    var colorHex: String
    var isSelected: Bool
    var accessRole: String
    var etag: String?
    var defaultReminderMinutes: [Int]
    var timeZoneID: String?

    init(
        id: String,
        summary: String,
        colorHex: String,
        isSelected: Bool,
        accessRole: String,
        etag: String? = nil,
        defaultReminderMinutes: [Int] = [],
        timeZoneID: String? = nil
    ) {
        self.id = id
        self.summary = summary
        self.colorHex = colorHex
        self.isSelected = isSelected
        self.accessRole = accessRole
        self.etag = etag
        self.defaultReminderMinutes = defaultReminderMinutes
        self.timeZoneID = TimezoneSupport.validatedIdentifier(timeZoneID)
    }

    enum CodingKeys: String, CodingKey {
        case id
        case summary
        case colorHex
        case isSelected
        case accessRole
        case etag
        case defaultReminderMinutes
        case timeZoneID
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        summary = try container.decode(String.self, forKey: .summary)
        colorHex = try container.decode(String.self, forKey: .colorHex)
        isSelected = try container.decode(Bool.self, forKey: .isSelected)
        accessRole = try container.decode(String.self, forKey: .accessRole)
        etag = try container.decodeIfPresent(String.self, forKey: .etag)
        defaultReminderMinutes = try container.decodeIfPresent([Int].self, forKey: .defaultReminderMinutes) ?? []
        timeZoneID = TimezoneSupport.validatedIdentifier(try container.decodeIfPresent(String.self, forKey: .timeZoneID))
    }
}

struct CalendarEventMirror: Identifiable, Hashable, Codable, Sendable {
    let id: String
    var calendarID: CalendarListMirror.ID
    var summary: String
    var details: String
    var startDate: Date
    var endDate: Date
    var isAllDay: Bool
    var status: CalendarEventStatus
    var recurrence: [String]
    var etag: String?
    var updatedAt: Date?
    var reminderMinutes: [Int]
    var usedDefaultReminders: Bool
    var location: String
    var attendeeEmails: [String]
    var attendeeResponses: [CalendarEventAttendee]
    var meetLink: String
    var htmlLink: String?
    var colorId: String?
    var startTimeZoneID: String
    var endTimeZoneID: String
    var transparency: CalendarEventTransparency
    var visibility: CalendarEventVisibility
    // HCB-only metadata stored in Google's native `extendedProperties.private`
    // bag so app state stays out of user-visible fields like description.
    var hcbTaskID: String?
    var availabilityHold: AvailabilityHoldMetadata?

    init(
        id: String,
        calendarID: CalendarListMirror.ID,
        summary: String,
        details: String,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        status: CalendarEventStatus,
        recurrence: [String],
        etag: String?,
        updatedAt: Date?,
        reminderMinutes: [Int] = [],
        usedDefaultReminders: Bool = false,
        location: String = "",
        attendeeEmails: [String] = [],
        attendeeResponses: [CalendarEventAttendee] = [],
        meetLink: String = "",
        htmlLink: String? = nil,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        transparency: CalendarEventTransparency = .opaque,
        visibility: CalendarEventVisibility = .defaultVisibility,
        hcbTaskID: String? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil
    ) {
        self.id = id
        self.calendarID = calendarID
        self.summary = summary
        self.details = details
        self.startDate = startDate
        self.endDate = endDate
        self.isAllDay = isAllDay
        self.status = status
        self.recurrence = recurrence
        self.etag = etag
        self.updatedAt = updatedAt
        self.reminderMinutes = reminderMinutes
        self.usedDefaultReminders = usedDefaultReminders
        self.location = location
        self.attendeeEmails = attendeeEmails
        self.attendeeResponses = attendeeResponses
        self.meetLink = meetLink
        self.htmlLink = htmlLink
        self.colorId = colorId
        let resolvedStartTimeZoneID = TimezoneSupport.validatedIdentifier(startTimeZoneID) ?? TimezoneSupport.currentIdentifier
        self.startTimeZoneID = resolvedStartTimeZoneID
        self.endTimeZoneID = TimezoneSupport.validatedIdentifier(endTimeZoneID) ?? resolvedStartTimeZoneID
        self.transparency = transparency
        self.visibility = visibility
        self.hcbTaskID = hcbTaskID
        self.availabilityHold = availabilityHold
    }

    enum CodingKeys: String, CodingKey {
        case id
        case calendarID
        case summary
        case details
        case startDate
        case endDate
        case isAllDay
        case status
        case recurrence
        case etag
        case updatedAt
        case reminderMinutes
        case usedDefaultReminders
        case location
        case attendeeEmails
        case attendeeResponses
        case meetLink
        case htmlLink
        case colorId
        case startTimeZoneID
        case endTimeZoneID
        case transparency
        case visibility
        case hcbTaskID
        case availabilityHold
    }

    init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        calendarID = try container.decode(CalendarListMirror.ID.self, forKey: .calendarID)
        summary = try container.decode(String.self, forKey: .summary)
        details = try container.decode(String.self, forKey: .details)
        startDate = try container.decode(Date.self, forKey: .startDate)
        endDate = try container.decode(Date.self, forKey: .endDate)
        isAllDay = try container.decode(Bool.self, forKey: .isAllDay)
        status = try container.decode(CalendarEventStatus.self, forKey: .status)
        recurrence = try container.decode([String].self, forKey: .recurrence)
        etag = try container.decodeIfPresent(String.self, forKey: .etag)
        updatedAt = try container.decodeIfPresent(Date.self, forKey: .updatedAt)
        reminderMinutes = try container.decodeIfPresent([Int].self, forKey: .reminderMinutes) ?? []
        usedDefaultReminders = try container.decodeIfPresent(Bool.self, forKey: .usedDefaultReminders) ?? false
        location = try container.decodeIfPresent(String.self, forKey: .location) ?? ""
        attendeeEmails = try container.decodeIfPresent([String].self, forKey: .attendeeEmails) ?? []
        attendeeResponses = try container.decodeIfPresent([CalendarEventAttendee].self, forKey: .attendeeResponses) ?? []
        meetLink = try container.decodeIfPresent(String.self, forKey: .meetLink) ?? ""
        htmlLink = try container.decodeIfPresent(String.self, forKey: .htmlLink)
        colorId = try container.decodeIfPresent(String.self, forKey: .colorId)
        let decodedStartTimeZoneID = TimezoneSupport.validatedIdentifier(try container.decodeIfPresent(String.self, forKey: .startTimeZoneID))
            ?? TimezoneSupport.currentIdentifier
        startTimeZoneID = decodedStartTimeZoneID
        endTimeZoneID = TimezoneSupport.validatedIdentifier(try container.decodeIfPresent(String.self, forKey: .endTimeZoneID))
            ?? decodedStartTimeZoneID
        transparency = try container.decodeIfPresent(CalendarEventTransparency.self, forKey: .transparency) ?? .opaque
        visibility = try container.decodeIfPresent(CalendarEventVisibility.self, forKey: .visibility) ?? .defaultVisibility
        hcbTaskID = try container.decodeIfPresent(String.self, forKey: .hcbTaskID)
        availabilityHold = try container.decodeIfPresent(AvailabilityHoldMetadata.self, forKey: .availabilityHold)
    }

    var isAvailabilityHold: Bool {
        availabilityHold != nil
    }
}

enum CalendarEventTransparency: String, Codable, Hashable, Sendable {
    case opaque
    case transparent

    init(wire: String?) {
        self = CalendarEventTransparency(rawValue: wire ?? "") ?? .opaque
    }
}

enum CalendarEventVisibility: String, Codable, Hashable, Sendable {
    case defaultVisibility = "default"
    case publicVisibility = "public"
    case privateVisibility = "private"
    case confidential

    init(wire: String?) {
        self = CalendarEventVisibility(rawValue: wire ?? "") ?? .defaultVisibility
    }
}

struct AvailabilityHoldMetadata: Hashable, Codable, Sendable {
    static let roleValue = "hold"
    static let groupIDKey = "hcbAvailabilityGroupID"
    static let roleKey = "hcbAvailabilityRole"
    static let titleKey = "hcbAvailabilityTitle"
    static let durationKey = "hcbAvailabilityDuration"
    static let createdAtKey = "hcbAvailabilityCreatedAt"

    var groupID: String
    var title: String
    var durationMinutes: Int
    var createdAt: Date

    init(
        groupID: String,
        title: String,
        durationMinutes: Int,
        createdAt: Date = Date()
    ) {
        self.groupID = groupID
        self.title = title.trimmingCharacters(in: .whitespacesAndNewlines)
        self.durationMinutes = max(15, durationMinutes)
        self.createdAt = createdAt
    }

    init?(privateProperties: [String: String]) {
        guard privateProperties[Self.roleKey] == Self.roleValue,
              let groupID = privateProperties[Self.groupIDKey],
              groupID.isEmpty == false
        else {
            return nil
        }

        self.groupID = groupID
        let rawTitle = privateProperties[Self.titleKey]?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        self.title = rawTitle.isEmpty ? "Meeting" : rawTitle
        let rawDuration = privateProperties[Self.durationKey].flatMap(Int.init) ?? 30
        self.durationMinutes = max(15, rawDuration)
        if let rawCreated = privateProperties[Self.createdAtKey],
           let parsed = ISO8601DateFormatter.google.date(from: rawCreated) {
            self.createdAt = parsed
        } else {
            self.createdAt = Date()
        }
    }

    var privateProperties: [String: String] {
        [
            Self.groupIDKey: groupID,
            Self.roleKey: Self.roleValue,
            Self.titleKey: title,
            Self.durationKey: String(durationMinutes),
            Self.createdAtKey: ISO8601DateFormatter.google.string(from: createdAt)
        ]
    }

    static var clearPrivateProperties: [String: String?] {
        [
            Self.groupIDKey: nil,
            Self.roleKey: nil,
            Self.titleKey: nil,
            Self.durationKey: nil,
            Self.createdAtKey: nil
        ]
    }
}

struct CalendarEventAttendee: Hashable, Codable, Sendable {
    var email: String
    var displayName: String?
    var responseStatus: AttendeeResponseStatus
}

enum AttendeeResponseStatus: String, Hashable, Codable, Sendable {
    case needsAction
    case declined
    case tentative
    case accepted

    init(wire: String?) {
        switch wire {
        case "accepted": self = .accepted
        case "declined": self = .declined
        case "tentative": self = .tentative
        default: self = .needsAction
        }
    }

    var displayTitle: String {
        switch self {
        case .needsAction: "No reply"
        case .declined: "Declined"
        case .tentative: "Maybe"
        case .accepted: "Going"
        }
    }

    var symbol: String {
        switch self {
        case .needsAction: "questionmark.circle"
        case .declined: "xmark.circle.fill"
        case .tentative: "questionmark.circle.fill"
        case .accepted: "checkmark.circle.fill"
        }
    }
}

enum CalendarEventStatus: String, Codable, Hashable, Sendable {
    case confirmed
    case tentative
    case cancelled
}

enum TimezoneSupport {
    static var currentIdentifier: String { TimeZone.current.identifier }

    static let pickerIdentifiers: [String] = {
        var identifiers = Set(TimeZone.knownTimeZoneIdentifiers)
        identifiers.insert("UTC")
        return identifiers.sorted { lhs, rhs in
            displayName(for: lhs).localizedCaseInsensitiveCompare(displayName(for: rhs)) == .orderedAscending
        }
    }()

    static func normalize(_ raw: String?) -> String? {
        guard let raw else { return nil }
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        return deprecatedTimezones[trimmed.uppercased()] ?? trimmed
    }

    static func validatedIdentifier(_ raw: String?) -> String? {
        guard let normalized = normalize(raw),
              TimeZone(identifier: normalized) != nil
        else {
            return nil
        }
        return normalized
    }

    static func timeZone(for identifier: String?) -> TimeZone {
        guard let valid = validatedIdentifier(identifier),
              let timeZone = TimeZone(identifier: valid)
        else {
            return .current
        }
        return timeZone
    }

    static func displayName(for identifier: String) -> String {
        let timeZone = timeZone(for: identifier)
        let offset = gmtOffset(for: timeZone)
        let name = timeZone.localizedName(for: .standard, locale: .current) ?? identifier
        return "(\(offset)) \(name)"
    }

    static func compactDisplayName(for identifier: String) -> String {
        let timeZone = timeZone(for: identifier)
        let normalized = validatedIdentifier(identifier) ?? identifier
        let placeName = normalized == "UTC"
            ? "UTC"
            : normalized
                .split(separator: "/")
                .last
                .map { String($0).replacingOccurrences(of: "_", with: " ") }
        return "\(gmtOffset(for: timeZone)) \(placeName ?? normalized)"
    }

    private static func gmtOffset(for timeZone: TimeZone, at date: Date = Date()) -> String {
        let seconds = timeZone.secondsFromGMT(for: date)
        let sign = seconds >= 0 ? "+" : "-"
        let absolute = abs(seconds)
        let hours = absolute / 3600
        let minutes = (absolute % 3600) / 60
        return String(format: "GMT%@%02d:%02d", sign, hours, minutes)
    }

    static func reinterpretingWallClock(
        _ date: Date,
        from oldIdentifier: String,
        to newIdentifier: String
    ) -> Date {
        let oldTimeZone = timeZone(for: oldIdentifier)
        let newTimeZone = timeZone(for: newIdentifier)
        var oldCalendar = Calendar(identifier: .gregorian)
        oldCalendar.timeZone = oldTimeZone
        let components = oldCalendar.dateComponents([.year, .month, .day, .hour, .minute, .second], from: date)
        var newCalendar = Calendar(identifier: .gregorian)
        newCalendar.timeZone = newTimeZone
        return newCalendar.date(from: components) ?? date
    }

    private static let deprecatedTimezones: [String: String] = [
        "US/EASTERN": "America/New_York",
        "US/CENTRAL": "America/Chicago",
        "US/MOUNTAIN": "America/Denver",
        "US/PACIFIC": "America/Los_Angeles",
        "US/ALASKA": "America/Anchorage",
        "US/HAWAII": "Pacific/Honolulu",
        "US/ARIZONA": "America/Phoenix",
        "ASIA/CALCUTTA": "Asia/Kolkata",
        "ASIA/SAIGON": "Asia/Ho_Chi_Minh",
        "ASIA/KATMANDU": "Asia/Kathmandu",
        "ASIA/RANGOON": "Asia/Yangon",
        "AUSTRALIA/ACT": "Australia/Sydney",
        "AUSTRALIA/NORTH": "Australia/Darwin",
        "AUSTRALIA/QUEENSLAND": "Australia/Brisbane",
        "AUSTRALIA/SOUTH": "Australia/Adelaide",
        "AUSTRALIA/TASMANIA": "Australia/Hobart",
        "AUSTRALIA/VICTORIA": "Australia/Melbourne",
        "AUSTRALIA/WEST": "Australia/Perth",
        "BRAZIL/EAST": "America/Sao_Paulo",
        "CANADA/ATLANTIC": "America/Halifax",
        "CANADA/CENTRAL": "America/Winnipeg",
        "CANADA/EASTERN": "America/Toronto",
        "CANADA/MOUNTAIN": "America/Edmonton",
        "CANADA/PACIFIC": "America/Vancouver",
        "CANADA/SASKATCHEWAN": "America/Regina",
        "CANADA/YUKON": "America/Whitehorse",
        "CET": "Europe/Paris",
        "CST6CDT": "America/Chicago",
        "EST": "America/New_York",
        "EST5EDT": "America/New_York",
        "ETC/GMT": "UTC",
        "ETC/UTC": "UTC",
        "GMT": "UTC",
        "HST": "Pacific/Honolulu",
        "MST": "America/Denver",
        "MST7MDT": "America/Denver",
        "PST": "America/Los_Angeles",
        "PST8PDT": "America/Los_Angeles",
        "UTC": "UTC",
        "Z": "UTC"
    ]
}
