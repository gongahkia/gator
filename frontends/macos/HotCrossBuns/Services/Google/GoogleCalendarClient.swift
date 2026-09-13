import Foundation

struct GoogleCalendarClient: Sendable {
    private let transport: GoogleAPITransport

    init(transport: GoogleAPITransport) {
        self.transport = transport
    }

    func listCalendars() async throws -> [CalendarListMirror] {
        do {
            let response: GoogleCalendarListResponse = try await transport.get(
                path: "/calendar/v3/users/me/calendarList",
                queryItems: [URLQueryItem(name: "fields", value: Self.calendarListFields)]
            )
            let calendars = response.items.map(calendarMirror)
            AppLogger.info("google calendars listed", category: .google, metadata: ["count": String(calendars.count)])
            return calendars
        } catch {
            AppLogger.warn("google calendars list failed", category: .google, metadata: GoogleDiagnostics.errorMetadata(error))
            throw error
        }
    }

    private func calendarMirror(_ item: GoogleCalendarListItemDTO) -> CalendarListMirror {
        CalendarListMirror(
            id: item.id,
            summary: item.summary,
            colorHex: item.backgroundColor ?? "#F66B3D",
            isSelected: item.selected ?? true,
            accessRole: item.accessRole,
            etag: item.etag,
            defaultReminderMinutes: item.defaultReminders?
                .filter { $0.method == "popup" }
                .map(\.minutes)
                .sorted() ?? [],
            timeZoneID: item.timeZone
        )
    }

    func listEvents(
        calendarID: String,
        syncToken: String?,
        timeMin: Date? = Date(),
        defaultTimeZoneID: String? = nil
    ) async throws -> GoogleCalendarEventsPage {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let mode = (syncToken?.isEmpty == false) ? "incremental" : "full"
        AppLogger.info("google calendar events list start", category: .google, metadata: [
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "mode": mode,
            "hasSyncToken": String(syncToken?.isEmpty == false),
            "hasTimeMin": String(timeMin != nil)
        ])
        let baseQueryItems = [
            URLQueryItem(name: "singleEvents", value: "true"),
            URLQueryItem(name: "showDeleted", value: "true"),
            // Google Calendar events endpoint supports up to 2500 per page.
            // Previously 250 — meant a 1000-event calendar took 4 round
            // trips of ~1s each. 2500 keeps most calendars to a single
            // request. Response size scales but decode cost is linear in
            // events either way.
            URLQueryItem(name: "maxResults", value: "2500"),
            URLQueryItem(name: "fields", value: Self.eventsFields)
        ]
        var pageToken: String?
        var events: [CalendarEventMirror] = []
        var nextSyncToken: String?
        var pageCount = 0

        do {
            repeat {
                pageCount += 1
                var queryItems = baseQueryItems

                if let syncToken, !syncToken.isEmpty {
                    queryItems.append(URLQueryItem(name: "syncToken", value: syncToken))
                } else if let timeMin {
                    queryItems.append(URLQueryItem(name: "timeMin", value: ISO8601DateFormatter.google.string(from: timeMin)))
                }

                if let pageToken {
                    queryItems.append(URLQueryItem(name: "pageToken", value: pageToken))
                }

                let response: GoogleEventsResponse = try await transport.get(
                    path: "/calendar/v3/calendars/\(encodedCalendarID)/events",
                    queryItems: queryItems
                )

                events.reserveCapacity(events.count + response.items.count)
                for item in response.items {
                    events.append(item.mirror(calendarID: calendarID, defaultTimeZoneID: defaultTimeZoneID))
                }
                nextSyncToken = response.nextSyncToken ?? nextSyncToken
                pageToken = response.nextPageToken
            } while pageToken != nil
        } catch {
            AppLogger.warn("google calendar events list failed", category: .google, metadata: [
                "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
                "mode": mode,
                "pages": String(pageCount)
            ].merging(GoogleDiagnostics.errorMetadata(error)) { _, new in new })
            throw error
        }

        AppLogger.info("google calendar events list succeeded", category: .google, metadata: [
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "mode": mode,
            "pages": String(pageCount),
            "count": String(events.count),
            "hasNextSyncToken": String(nextSyncToken?.isEmpty == false)
        ])

        return GoogleCalendarEventsPage(
            events: events,
            nextSyncToken: nextSyncToken
        )
    }

    private static let calendarListFields = [
        "items(id,summary,backgroundColor,selected,accessRole,etag,defaultReminders(method,minutes),timeZone)"
    ].joined(separator: ",")

    private static let eventsFields = [
        "nextPageToken",
        "nextSyncToken",
        "items(id,summary,description,location,status,start,end,recurrence,reminders(useDefault,overrides(method,minutes)),attendees(email,displayName,responseStatus),etag,updated,conferenceData(entryPoints(entryPointType,uri,label),conferenceSolution(key(type),name),conferenceId),htmlLink,colorId,transparency,visibility,extendedProperties)"
    ].joined(separator: ",")

    func insertEvent(
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
        sendUpdates: String = "none",
        addGoogleMeet: Bool = false,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        transparency: CalendarEventTransparency? = nil,
        visibility: CalendarEventVisibility? = nil,
        hcbTaskID: String? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil
    ) async throws -> CalendarEventMirror {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let htmlDetails = MarkdownHTML.markdownToCalendarHTML(details)
        let conference = addGoogleMeet
            ? GoogleConferenceCreateDTO(
                createRequest: GoogleConferenceCreateRequestDTO(
                    requestId: UUID().uuidString,
                    conferenceSolutionKey: GoogleConferenceSolutionKeyEncodeDTO(type: "hangoutsMeet")
                )
            )
            : nil
        let resolvedStartTimeZoneID = isAllDay ? nil : TimezoneSupport.validatedIdentifier(startTimeZoneID)
        let resolvedEndTimeZoneID = isAllDay ? nil : (TimezoneSupport.validatedIdentifier(endTimeZoneID) ?? resolvedStartTimeZoneID)
        let requestBody = GoogleEventMutationDTO(
            summary: summary,
            description: htmlDetails.isEmpty ? nil : htmlDetails,
            location: location.isEmpty ? nil : location,
            start: GoogleEventMutationDateDTO(date: isAllDay ? GoogleDateOnlyFormatter.string(from: startDate) : nil, dateTime: isAllDay ? nil : GoogleDateTimeFormatter.string(from: startDate, timeZoneID: resolvedStartTimeZoneID), timeZone: resolvedStartTimeZoneID),
            end: GoogleEventMutationDateDTO(date: isAllDay ? GoogleDateOnlyFormatter.exclusiveEndString(from: endDate) : nil, dateTime: isAllDay ? nil : GoogleDateTimeFormatter.string(from: endDate, timeZoneID: resolvedEndTimeZoneID), timeZone: resolvedEndTimeZoneID),
            recurrence: recurrence.isEmpty ? nil : recurrence,
            reminders: GoogleEventMutationRemindersDTO.custom(minutes: reminderMinutes),
            attendees: attendeeEmails.isEmpty ? nil : attendeeEmails.map { GoogleEventAttendeeMutationDTO(email: $0) },
            conferenceData: conference,
            colorId: colorId,
            transparency: transparency?.rawValue,
            visibility: visibility?.rawValue,
            extendedProperties: GoogleEventExtendedPropertiesEncodeDTO.hcb(
                taskID: hcbTaskID,
                availabilityHold: availabilityHold
            )
        )
        var queryItems: [URLQueryItem] = [URLQueryItem(name: "sendUpdates", value: sendUpdates)]
        if addGoogleMeet {
            // conferenceDataVersion=1 is required for Google to honour a
            // createRequest and materialize the Meet link server-side.
            queryItems.append(URLQueryItem(name: "conferenceDataVersion", value: "1"))
        }
        let response: GoogleEventDTO = try await transport.request(
            method: "POST",
            path: "/calendar/v3/calendars/\(encodedCalendarID)/events",
            queryItems: queryItems,
            body: requestBody
        )
        let mirror = response.mirror(calendarID: calendarID)
        AppLogger.info("google calendar event write accepted", category: .google, metadata: [
            "action": "create",
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(mirror.id),
            "isAllDay": String(isAllDay),
            "hasDetails": String(details.isEmpty == false),
            "hasLocation": String(location.isEmpty == false),
            "attendeeCount": String(attendeeEmails.count),
            "recurrenceCount": String(recurrence.count),
            "notifyGuests": String(sendUpdates != "none"),
            "addGoogleMeet": String(addGoogleMeet),
            "isAvailabilityHold": String(availabilityHold != nil)
        ])
        return mirror
    }

    func updateEvent(
        calendarID: String,
        eventID: String,
        summary: String,
        details: String,
        startDate: Date,
        endDate: Date,
        isAllDay: Bool,
        reminderMinutes: Int?,
        location: String = "",
        recurrence: [String] = [],
        attendeeEmails: [String] = [],
        sendUpdates: String = "none",
        addGoogleMeet: Bool = false,
        colorId: String? = nil,
        startTimeZoneID: String? = nil,
        endTimeZoneID: String? = nil,
        hcbTaskID: String? = nil,
        transparency: CalendarEventTransparency? = nil,
        visibility: CalendarEventVisibility? = nil,
        availabilityHold: AvailabilityHoldMetadata? = nil,
        clearAvailabilityHoldMetadata: Bool = false,
        ifMatch: String? = nil
    ) async throws -> CalendarEventMirror {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let encodedEventID = eventID.googlePathComponentEncoded
        let htmlDetails = MarkdownHTML.markdownToCalendarHTML(details)
        let conference = addGoogleMeet
            ? GoogleConferenceCreateDTO(
                createRequest: GoogleConferenceCreateRequestDTO(
                    requestId: UUID().uuidString,
                    conferenceSolutionKey: GoogleConferenceSolutionKeyEncodeDTO(type: "hangoutsMeet")
                )
            )
            : nil
        let resolvedStartTimeZoneID = isAllDay ? nil : TimezoneSupport.validatedIdentifier(startTimeZoneID)
        let resolvedEndTimeZoneID = isAllDay ? nil : (TimezoneSupport.validatedIdentifier(endTimeZoneID) ?? resolvedStartTimeZoneID)
        let requestBody = GoogleEventMutationDTO(
            summary: summary,
            description: htmlDetails,
            location: location,
            start: GoogleEventMutationDateDTO(date: isAllDay ? GoogleDateOnlyFormatter.string(from: startDate) : nil, dateTime: isAllDay ? nil : GoogleDateTimeFormatter.string(from: startDate, timeZoneID: resolvedStartTimeZoneID), timeZone: resolvedStartTimeZoneID),
            end: GoogleEventMutationDateDTO(date: isAllDay ? GoogleDateOnlyFormatter.exclusiveEndString(from: endDate) : nil, dateTime: isAllDay ? nil : GoogleDateTimeFormatter.string(from: endDate, timeZoneID: resolvedEndTimeZoneID), timeZone: resolvedEndTimeZoneID),
            recurrence: recurrence,
            reminders: GoogleEventMutationRemindersDTO.custom(minutes: reminderMinutes),
            attendees: attendeeEmails.map { GoogleEventAttendeeMutationDTO(email: $0) },
            conferenceData: conference,
            colorId: colorId,
            transparency: transparency?.rawValue,
            visibility: visibility?.rawValue,
            extendedProperties: GoogleEventExtendedPropertiesEncodeDTO.hcb(
                taskID: hcbTaskID,
                availabilityHold: availabilityHold,
                clearAvailabilityHoldMetadata: clearAvailabilityHoldMetadata
            )
        )
        var queryItems: [URLQueryItem] = [URLQueryItem(name: "sendUpdates", value: sendUpdates)]
        if addGoogleMeet {
            queryItems.append(URLQueryItem(name: "conferenceDataVersion", value: "1"))
        }
        let response: GoogleEventDTO = try await transport.request(
            method: "PATCH",
            path: "/calendar/v3/calendars/\(encodedCalendarID)/events/\(encodedEventID)",
            queryItems: queryItems,
            body: requestBody,
            ifMatch: ifMatch
        )
        let mirror = response.mirror(calendarID: calendarID)
        AppLogger.info("google calendar event write accepted", category: .google, metadata: [
            "action": "update",
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(eventID),
            "isAllDay": String(isAllDay),
            "hasDetails": String(details.isEmpty == false),
            "hasLocation": String(location.isEmpty == false),
            "attendeeCount": String(attendeeEmails.count),
            "recurrenceCount": String(recurrence.count),
            "notifyGuests": String(sendUpdates != "none"),
            "addGoogleMeet": String(addGoogleMeet),
            "isAvailabilityHold": String(availabilityHold != nil),
            "clearsAvailabilityHold": String(clearAvailabilityHoldMetadata),
            "hasIfMatch": String(ifMatch?.isEmpty == false)
        ])
        return mirror
    }

    func moveEvent(
        calendarID: String,
        eventID: String,
        destinationCalendarID: String
    ) async throws -> CalendarEventMirror {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let encodedEventID = eventID.googlePathComponentEncoded
        let response: GoogleEventDTO = try await transport.request(
            method: "POST",
            path: "/calendar/v3/calendars/\(encodedCalendarID)/events/\(encodedEventID)/move",
            queryItems: [
                URLQueryItem(name: "destination", value: destinationCalendarID),
                URLQueryItem(name: "sendUpdates", value: "none")
            ]
        )
        let mirror = response.mirror(calendarID: destinationCalendarID)
        AppLogger.info("google calendar event write accepted", category: .google, metadata: [
            "action": "move",
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(eventID),
            "destinationCalendarID": GoogleDiagnostics.redactedIdentifier(destinationCalendarID)
        ])
        return mirror
    }

    // Fetches a single event by id — used when we need the master event's
    // current recurrence rules for "this and following" truncation, since
    // instances returned via singleEvents=true don't carry the master RRULE.
    func getEvent(calendarID: String, eventID: String, defaultTimeZoneID: String? = nil) async throws -> CalendarEventMirror {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let encodedEventID = eventID.googlePathComponentEncoded
        let response: GoogleEventDTO = try await transport.request(
            method: "GET",
            path: "/calendar/v3/calendars/\(encodedCalendarID)/events/\(encodedEventID)"
        )
        let mirror = response.mirror(calendarID: calendarID, defaultTimeZoneID: defaultTimeZoneID)
        AppLogger.info("google calendar event fetched", category: .google, metadata: [
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(eventID)
        ])
        return mirror
    }

    // Patches only the recurrence array on the master event. Used by the
    // "this and following" flow, which rewrites the master's RRULE with a
    // new UNTIL clause and leaves all other fields untouched.
    func patchEventRecurrence(
        calendarID: String,
        eventID: String,
        recurrence: [String],
        ifMatch: String? = nil
    ) async throws -> CalendarEventMirror {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let encodedEventID = eventID.googlePathComponentEncoded
        struct RecurrencePatch: Encodable { var recurrence: [String] }
        let response: GoogleEventDTO = try await transport.request(
            method: "PATCH",
            path: "/calendar/v3/calendars/\(encodedCalendarID)/events/\(encodedEventID)",
            body: RecurrencePatch(recurrence: recurrence),
            ifMatch: ifMatch
        )
        let mirror = response.mirror(calendarID: calendarID)
        AppLogger.info("google calendar event write accepted", category: .google, metadata: [
            "action": "patchRecurrence",
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(eventID),
            "recurrenceCount": String(recurrence.count),
            "hasIfMatch": String(ifMatch?.isEmpty == false)
        ])
        return mirror
    }

    func deleteEvent(calendarID: String, eventID: String, ifMatch: String? = nil) async throws {
        let encodedCalendarID = calendarID.googlePathComponentEncoded
        let encodedEventID = eventID.googlePathComponentEncoded
        try await transport.send(
            method: "DELETE",
            path: "/calendar/v3/calendars/\(encodedCalendarID)/events/\(encodedEventID)",
            queryItems: [URLQueryItem(name: "sendUpdates", value: "none")],
            ifMatch: ifMatch
        )
        AppLogger.info("google calendar event write accepted", category: .google, metadata: [
            "action": "delete",
            "calendarID": GoogleDiagnostics.redactedIdentifier(calendarID),
            "eventID": GoogleDiagnostics.redactedIdentifier(eventID),
            "hasIfMatch": String(ifMatch?.isEmpty == false)
        ])
    }
}

#if DEBUG
struct GoogleCalendarEventDecodeProfile: Sendable {
    var decodedItemCount: Int
    var mappedEventCount: Int
    var nextSyncToken: String?
    var decodeMilliseconds: Double
    var mirrorMilliseconds: Double
    var totalMilliseconds: Double
}

extension GoogleCalendarClient {
    static func decodeAndMapEventsForBenchmark(
        data: Data,
        calendarID: String,
        defaultTimeZoneID: String? = nil
    ) throws -> GoogleCalendarEventDecodeProfile {
        let totalStart = DispatchTime.now().uptimeNanoseconds
        let decodeStart = DispatchTime.now().uptimeNanoseconds
        let response = try JSONDecoder.googleAPI.decode(GoogleEventsResponse.self, from: data)
        let decodeEnd = DispatchTime.now().uptimeNanoseconds

        var mappedEvents: [CalendarEventMirror] = []
        mappedEvents.reserveCapacity(response.items.count)
        let mirrorStart = DispatchTime.now().uptimeNanoseconds
        for item in response.items {
            mappedEvents.append(item.mirror(calendarID: calendarID, defaultTimeZoneID: defaultTimeZoneID))
        }
        let mirrorEnd = DispatchTime.now().uptimeNanoseconds

        return GoogleCalendarEventDecodeProfile(
            decodedItemCount: response.items.count,
            mappedEventCount: mappedEvents.count,
            nextSyncToken: response.nextSyncToken,
            decodeMilliseconds: Self.milliseconds(from: decodeStart, to: decodeEnd),
            mirrorMilliseconds: Self.milliseconds(from: mirrorStart, to: mirrorEnd),
            totalMilliseconds: Self.milliseconds(from: totalStart, to: mirrorEnd)
        )
    }

    private static func milliseconds(from start: UInt64, to end: UInt64) -> Double {
        Double(end - start) / 1_000_000
    }
}
#endif

struct GoogleCalendarEventsPage: Sendable {
    var events: [CalendarEventMirror]
    var nextSyncToken: String?
}

private struct GoogleCalendarListResponse: Decodable, Sendable {
    var items: [GoogleCalendarListItemDTO]
}

private struct GoogleCalendarListItemDTO: Decodable, Sendable {
    var id: String
    var summary: String
    var backgroundColor: String?
    var selected: Bool?
    var accessRole: String
    var etag: String?
    var defaultReminders: [GoogleEventReminderDTO]?
    var timeZone: String?
}

private struct GoogleEventsResponse: Decodable, Sendable {
    var items: [GoogleEventDTO]
    var nextPageToken: String?
    var nextSyncToken: String?
}

private struct GoogleEventDTO: Decodable, Sendable {
    var id: String
    var summary: String?
    var description: String?
    var location: String?
    var status: String?
    var start: GoogleEventDateDTO?
    var end: GoogleEventDateDTO?
    var recurrence: [String]?
    var reminders: GoogleEventRemindersDTO?
    var attendees: [GoogleEventAttendeeDTO]?
    var etag: String?
    var updated: Date?
    var conferenceData: GoogleConferenceDataDTO?
    var htmlLink: String?
    var colorId: String?
    var transparency: String?
    var visibility: String?
    var extendedProperties: GoogleEventExtendedPropertiesDTO?
    private static let legacyBacklinkRegex = try? NSRegularExpression(
        pattern: "(?m)(?:^|\\n)Linked task:[^\\n]*\\nhcb://task/([^\\s<]+)",
        options: []
    )

    func mirror(calendarID: String, defaultTimeZoneID: String? = nil) -> CalendarEventMirror {
        let fallbackDate = updated ?? Date()
        // Details rendering + one-shot migration of the legacy time-blocking
        // backlink that HCB used to embed in `description`:
        //   "Linked task: <title>\nhcb://task/<id>"
        // On read we extract the id into `hcbTaskID` and strip the block from
        // the rendered details so the user never sees the marker. On next
        // write, the stripped description gets written back — completing the
        // migration to extendedProperties.private.
        let (scrubbedDescription, legacyTaskID) = GoogleEventDTO.stripLegacyBacklink(description ?? "")
        let renderedDetails = GoogleEventDTO.renderedDescription(from: scrubbedDescription)
        let privateProperties = extendedProperties?.privateProperties ?? [:]
        let hcbTaskID = privateProperties["hcbTaskID"] ?? legacyTaskID
        let availabilityHold = AvailabilityHoldMetadata(privateProperties: privateProperties)
        // Google Calendar returns `date` (for all-day) as "yyyy-MM-dd" decoded
        // by GoogleDateParser.dateOnly as UTC midnight. Comparing that against
        // a local-TZ reference date is unsafe: in UTC-N an event whose date is
        // "2026-04-19" decodes to April 18 8pm local and appears as April 18
        // in all snapshot/forecast filters. Re-anchor date-only values to the
        // user's local midnight of the same Y/M/D so every downstream
        // comparison uses matching timezones.
        let isAllDay = start?.date != nil
        let resolvedStartTimeZoneID = TimezoneSupport.validatedIdentifier(start?.timeZone)
            ?? TimezoneSupport.validatedIdentifier(defaultTimeZoneID)
            ?? TimezoneSupport.currentIdentifier
        let resolvedEndTimeZoneID = TimezoneSupport.validatedIdentifier(end?.timeZone)
            ?? resolvedStartTimeZoneID
        let startDate = GoogleEventDTO.resolveDate(
            dateTime: start?.dateTime,
            dateOnly: start?.date,
            fallback: fallbackDate
        )
        let endDate = GoogleEventDTO.resolveDate(
            dateTime: end?.dateTime,
            dateOnly: end?.date,
            fallback: fallbackDate
        )
        let attendeeList = attendees ?? []
        let attendeeResponses: [CalendarEventAttendee] = attendeeList.compactMap { dto in
            guard let email = dto.email, email.isEmpty == false else { return nil }
            return CalendarEventAttendee(
                email: email,
                displayName: dto.displayName,
                responseStatus: AttendeeResponseStatus(wire: dto.responseStatus)
            )
        }
        return CalendarEventMirror(
            id: id,
            calendarID: calendarID,
            summary: summary ?? "Untitled event",
            details: renderedDetails,
            startDate: startDate,
            endDate: endDate,
            isAllDay: isAllDay,
            status: CalendarEventStatus(rawValue: status ?? "confirmed") ?? .confirmed,
            recurrence: recurrence ?? [],
            etag: etag,
            updatedAt: updated,
            reminderMinutes: reminders?.customPopupMinutes ?? [],
            usedDefaultReminders: reminders?.useDefault == true,
            location: location ?? "",
            attendeeEmails: attendeeList.compactMap(\.email),
            attendeeResponses: attendeeResponses,
            meetLink: conferenceData?.meetLink ?? "",
            htmlLink: htmlLink,
            colorId: colorId,
            startTimeZoneID: resolvedStartTimeZoneID,
            endTimeZoneID: resolvedEndTimeZoneID,
            transparency: CalendarEventTransparency(wire: transparency),
            visibility: CalendarEventVisibility(wire: visibility),
            hcbTaskID: hcbTaskID,
            availabilityHold: availabilityHold
        )
    }

    // Strips any legacy "Linked task: X\nhcb://task/<id>" footer from the
    // event description. Returns the scrubbed description and the extracted
    // task id (if present). The footer is HCB-only schema in a Google-visible
    // field and is being retired in favour of extendedProperties.private.
    fileprivate static func stripLegacyBacklink(_ raw: String) -> (scrubbed: String, taskID: String?) {
        guard raw.isEmpty == false,
              raw.contains("Linked task:"),
              raw.contains("hcb://task/"),
              let regex = legacyBacklinkRegex
        else {
            return (raw, nil)
        }
        let range = NSRange(raw.startIndex..., in: raw)
        guard let match = regex.firstMatch(in: raw, range: range),
              match.numberOfRanges >= 2,
              let idRange = Range(match.range(at: 1), in: raw),
              let fullRange = Range(match.range, in: raw)
        else {
            return (raw, nil)
        }
        let taskID = String(raw[idRange])
        var scrubbed = raw
        scrubbed.removeSubrange(fullRange)
        scrubbed = scrubbed
            .replacingOccurrences(of: "\n{3,}", with: "\n\n", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return (scrubbed, taskID)
    }

    private static func renderedDescription(from raw: String) -> String {
        guard raw.isEmpty == false else { return "" }
        guard raw.contains("<") || raw.contains("&") else {
            return raw
                .replacingOccurrences(of: "\r\n", with: "\n")
                .trimmingCharacters(in: .whitespacesAndNewlines)
        }
        return MarkdownHTML.calendarHTMLToMarkdown(raw)
    }

    fileprivate static func resolveDate(dateTime: Date?, dateOnly: Date?, fallback: Date) -> Date {
        if let dateTime {
            return dateTime
        }
        guard let dateOnly else { return fallback }
        // Re-extract Y/M/D from the UTC-anchored decoded date, then rebuild
        // at local midnight of the same calendar day.
        var utcCalendar = Calendar(identifier: .gregorian)
        utcCalendar.timeZone = TimeZone(secondsFromGMT: 0) ?? .current
        let comps = utcCalendar.dateComponents([.year, .month, .day], from: dateOnly)
        var localComps = DateComponents()
        localComps.year = comps.year
        localComps.month = comps.month
        localComps.day = comps.day
        return Calendar.current.date(from: localComps) ?? fallback
    }
}

private struct GoogleEventAttendeeDTO: Codable, Sendable {
    var email: String?
    var displayName: String?
    var responseStatus: String?
}

private struct GoogleEventDateDTO: Decodable, Sendable {
    var date: Date?
    var dateTime: Date?
    var timeZone: String?

    var resolvedDate: Date {
        dateTime ?? date ?? Date()
    }
}

private struct GoogleEventRemindersDTO: Decodable, Sendable {
    var useDefault: Bool?
    var overrides: [GoogleEventReminderDTO]?

    var customPopupMinutes: [Int] {
        guard useDefault == false else {
            return []
        }

        return overrides?
            .filter { $0.method == "popup" }
            .map(\.minutes)
            .sorted() ?? []
    }
}

private struct GoogleEventReminderDTO: Decodable, Sendable {
    var method: String
    var minutes: Int
}

private struct GoogleConferenceDataDTO: Decodable, Sendable {
    var conferenceId: String?
    var entryPoints: [GoogleConferenceEntryPointDTO]?
    var conferenceSolution: GoogleConferenceSolutionDTO?

    var meetLink: String {
        guard let video = entryPoints?.first(where: { $0.entryPointType == "video" }) else {
            return ""
        }
        return video.uri ?? ""
    }
}

private struct GoogleConferenceEntryPointDTO: Decodable, Sendable {
    var entryPointType: String?
    var uri: String?
    var label: String?
}

private struct GoogleConferenceSolutionDTO: Decodable, Sendable {
    var key: GoogleConferenceSolutionKeyDTO?
    var name: String?
}

private struct GoogleConferenceSolutionKeyDTO: Decodable, Sendable {
    var type: String?
}

private struct GoogleEventMutationDTO: Encodable, Sendable {
    var summary: String
    var description: String?
    var location: String?
    var start: GoogleEventMutationDateDTO
    var end: GoogleEventMutationDateDTO
    var recurrence: [String]?
    var reminders: GoogleEventMutationRemindersDTO?
    var attendees: [GoogleEventAttendeeMutationDTO]?
    var conferenceData: GoogleConferenceCreateDTO?
    var colorId: String?
    var transparency: String?
    var visibility: String?
    var extendedProperties: GoogleEventExtendedPropertiesEncodeDTO?

    enum CodingKeys: String, CodingKey {
        case summary, description, location, start, end
        case recurrence, reminders, attendees, conferenceData, colorId
        case transparency, visibility, extendedProperties
    }

    // Custom encoding so nil optionals are omitted rather than emitted as
    // `null`. On a PATCH, Google Calendar treats explicit null as "clear this
    // field" — accidental clearing of conferenceData or colorId on every
    // update would be destructive.
    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encode(summary, forKey: .summary)
        try container.encodeIfPresent(description, forKey: .description)
        try container.encodeIfPresent(location, forKey: .location)
        try container.encode(start, forKey: .start)
        try container.encode(end, forKey: .end)
        try container.encodeIfPresent(recurrence, forKey: .recurrence)
        try container.encodeIfPresent(reminders, forKey: .reminders)
        try container.encodeIfPresent(attendees, forKey: .attendees)
        try container.encodeIfPresent(conferenceData, forKey: .conferenceData)
        try container.encodeIfPresent(colorId, forKey: .colorId)
        try container.encodeIfPresent(transparency, forKey: .transparency)
        try container.encodeIfPresent(visibility, forKey: .visibility)
        try container.encodeIfPresent(extendedProperties, forKey: .extendedProperties)
    }
}

// Google's field is `extendedProperties.private` — `private` is a Swift
// keyword, so encode/decode through custom CodingKeys. We only read/write
// the `private` bag; `shared` is reserved for future cross-client metadata.
private struct GoogleEventExtendedPropertiesDTO: Decodable, Sendable {
    var privateProperties: [String: String]?

    enum CodingKeys: String, CodingKey {
        case privateProperties = "private"
    }
}

private struct GoogleEventExtendedPropertiesEncodeDTO: Encodable, Sendable {
    var privateProperties: [String: String?]

    enum CodingKeys: String, CodingKey {
        case privateProperties = "private"
    }

    static func hcb(
        taskID: String?,
        availabilityHold: AvailabilityHoldMetadata? = nil,
        clearAvailabilityHoldMetadata: Bool = false
    ) -> GoogleEventExtendedPropertiesEncodeDTO? {
        var properties: [String: String?] = [:]

        if let taskID, taskID.isEmpty == false {
            properties["hcbTaskID"] = taskID
        }

        if let availabilityHold {
            properties.merge(availabilityHold.privateProperties.mapValues { Optional($0) }) { _, new in new }
        } else if clearAvailabilityHoldMetadata {
            properties.merge(AvailabilityHoldMetadata.clearPrivateProperties) { _, new in new }
        }

        guard properties.isEmpty == false else { return nil }
        return GoogleEventExtendedPropertiesEncodeDTO(privateProperties: properties)
    }
}

private struct GoogleConferenceCreateDTO: Encodable, Sendable {
    var createRequest: GoogleConferenceCreateRequestDTO
}

private struct GoogleConferenceCreateRequestDTO: Encodable, Sendable {
    var requestId: String
    var conferenceSolutionKey: GoogleConferenceSolutionKeyEncodeDTO
}

private struct GoogleConferenceSolutionKeyEncodeDTO: Encodable, Sendable {
    var type: String
}

private struct GoogleEventAttendeeMutationDTO: Encodable, Sendable {
    var email: String
}

private struct GoogleEventMutationDateDTO: Encodable, Sendable {
    var date: String?
    var dateTime: String?
    var timeZone: String?

    enum CodingKeys: String, CodingKey {
        case date, dateTime, timeZone
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.container(keyedBy: CodingKeys.self)
        try container.encodeIfPresent(date, forKey: .date)
        try container.encodeIfPresent(dateTime, forKey: .dateTime)
        try container.encodeIfPresent(timeZone, forKey: .timeZone)
    }
}

private struct GoogleEventMutationRemindersDTO: Encodable, Sendable {
    var useDefault: Bool
    var overrides: [GoogleEventMutationReminderDTO]?

    static func custom(minutes: Int?) -> GoogleEventMutationRemindersDTO? {
        guard let minutes else {
            return nil
        }

        return GoogleEventMutationRemindersDTO(
            useDefault: false,
            overrides: [GoogleEventMutationReminderDTO(method: "popup", minutes: minutes)]
        )
    }
}

private struct GoogleEventMutationReminderDTO: Encodable, Sendable {
    var method: String
    var minutes: Int
}

private enum GoogleDateOnlyFormatter {
    static let calendar = Calendar(identifier: .gregorian)

    static func string(from date: Date) -> String {
        let components = calendar.dateComponents([.year, .month, .day], from: date)
        guard let year = components.year, let month = components.month, let day = components.day else {
            return "1970-01-01"
        }

        return String(format: "%04d-%02d-%02d", year, month, day)
    }

    static func exclusiveEndString(from inclusiveEndDate: Date) -> String {
        let nextDay = calendar.date(byAdding: .day, value: 1, to: inclusiveEndDate) ?? inclusiveEndDate
        return string(from: nextDay)
    }
}

private enum GoogleDateTimeFormatter {
    static func string(from date: Date, timeZoneID: String?) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimezoneSupport.timeZone(for: timeZoneID)
        formatter.dateFormat = "yyyy-MM-dd'T'HH:mm:ssXXXXX"
        return formatter.string(from: date)
    }
}

private extension String {
    var googlePathComponentEncoded: String {
        var allowedCharacters = CharacterSet.urlPathAllowed
        allowedCharacters.remove(charactersIn: "/?#")
        return addingPercentEncoding(withAllowedCharacters: allowedCharacters) ?? self
    }
}
