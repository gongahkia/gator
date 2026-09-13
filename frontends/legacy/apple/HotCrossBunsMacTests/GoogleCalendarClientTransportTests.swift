import Foundation
import XCTest
@testable import HotCrossBunsMac

final class GoogleCalendarClientTransportTests: XCTestCase {
    private var client: GoogleCalendarClient!

    override func setUp() {
        super.setUp()
        MockURLProtocol.reset()
        let transport = GoogleAPITransport(
            baseURL: URL(string: "https://www.googleapis.com")!,
            tokenProvider: StaticAccessTokenProvider(token: "test-token"),
            urlSession: MockURLProtocol.testSession()
        )
        client = GoogleCalendarClient(transport: transport)
    }

    override func tearDown() {
        MockURLProtocol.reset()
        super.tearDown()
    }

    func testListCalendarsMapsPopupRemindersOnly() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "GET")
            XCTAssertEqual(request.url?.path, "/calendar/v3/users/me/calendarList")
            let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
            XCTAssertTrue(query["fields"]?.contains("items(id,summary,backgroundColor,selected,accessRole,etag") == true)
            XCTAssertTrue(query["fields"]?.contains("defaultReminders(method,minutes)") == true)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "items": [
                {
                  "id": "primary",
                  "summary": "Primary",
                  "backgroundColor": "#123456",
                  "selected": true,
                  "accessRole": "owner",
                  "etag": "calendar-etag",
                  "timeZone": "Asia/Singapore",
                  "defaultReminders": [
                    { "method": "email", "minutes": 120 },
                    { "method": "popup", "minutes": 30 },
                    { "method": "popup", "minutes": 10 }
                  ]
                }
              ]
            }
            """#
            return (response, Data(body.utf8))
        }

        let calendars = try await client.listCalendars()
        XCTAssertEqual(calendars.count, 1)
        XCTAssertEqual(calendars[0].id, "primary")
        XCTAssertEqual(calendars[0].summary, "Primary")
        XCTAssertEqual(calendars[0].colorHex, "#123456")
        XCTAssertEqual(calendars[0].defaultReminderMinutes, [10, 30])
        XCTAssertEqual(calendars[0].timeZoneID, "Asia/Singapore")
    }

    func testListEventsPaginatesAndCarriesSyncToken() async throws {
        var requestCount = 0
        MockURLProtocol.requestHandler = { request in
            requestCount += 1
            XCTAssertEqual(request.httpMethod, "GET")
            XCTAssertEqual(
                request.url?.path,
                "/calendar/v3/calendars/team@example.com/events"
            )
            let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
            XCTAssertEqual(query["singleEvents"], "true")
            XCTAssertEqual(query["showDeleted"], "true")
            XCTAssertEqual(query["maxResults"], "2500")
            XCTAssertEqual(query["syncToken"], "sync-123")
            XCTAssertNil(query["timeMin"])
            XCTAssertTrue(query["fields"]?.contains("nextPageToken") == true)
            XCTAssertTrue(query["fields"]?.contains("nextSyncToken") == true)
            XCTAssertTrue(query["fields"]?.contains("items(id,summary,description") == true)
            XCTAssertTrue(query["fields"]?.contains("reminders(useDefault,overrides(method,minutes))") == true)
            if requestCount == 1 {
                XCTAssertNil(query["pageToken"])
            } else {
                XCTAssertEqual(query["pageToken"], "page-2")
            }

            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body: String
            if requestCount == 1 {
                body = #"""
                {
                  "items": [
                    {
                      "id": "evt-1",
                      "summary": "First",
                      "status": "confirmed",
                      "start": { "dateTime": "2026-04-24T09:00:00Z", "timeZone": "Asia/Tokyo" },
                      "end": { "dateTime": "2026-04-24T10:00:00Z", "timeZone": "Asia/Tokyo" }
                    }
                  ],
                  "nextPageToken": "page-2"
                }
                """#
            } else {
                body = #"""
                {
                  "items": [
                    {
                      "id": "evt-2",
                      "summary": "Second",
                      "status": "confirmed",
                      "description": "Linked task: Task title\nhcb://task/task-123",
                      "start": { "date": "2026-04-25" },
                      "end": { "date": "2026-04-26" }
                    }
                  ],
                  "nextSyncToken": "sync-next"
                }
                """#
            }
            return (response, Data(body.utf8))
        }

        let page = try await client.listEvents(calendarID: "team@example.com", syncToken: "sync-123")
        XCTAssertEqual(requestCount, 2)
        XCTAssertEqual(page.events.map(\.id), ["evt-1", "evt-2"])
        XCTAssertEqual(page.nextSyncToken, "sync-next")
        XCTAssertFalse(page.events[0].isAllDay)
        XCTAssertEqual(page.events[0].startTimeZoneID, "Asia/Tokyo")
        XCTAssertEqual(page.events[0].endTimeZoneID, "Asia/Tokyo")
        XCTAssertTrue(page.events[1].isAllDay)
        XCTAssertEqual(page.events[1].hcbTaskID, "task-123")
        XCTAssertEqual(page.events[1].details, "")
    }

    func testListEventsPreservesDescriptionAndDateVariants() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "GET")
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "items": [
                {
                  "id": "plain",
                  "summary": "Plain",
                  "description": "  Plain notes\r\nsecond line  ",
                  "status": "confirmed",
                  "updated": "2026-05-01T01:02:03Z",
                  "start": { "dateTime": "2026-05-01T09:00:00Z", "timeZone": "UTC" },
                  "end": { "dateTime": "2026-05-01T10:00:00Z", "timeZone": "UTC" }
                },
                {
                  "id": "html",
                  "summary": "HTML",
                  "description": "<b>Team</b> &amp; launch<br>Next",
                  "status": "confirmed",
                  "updated": "2026-05-01T01:02:03.123Z",
                  "start": { "dateTime": "2026-05-01T11:00:00.123Z", "timeZone": "UTC" },
                  "end": { "dateTime": "2026-05-01T12:00:00.123Z", "timeZone": "UTC" }
                },
                {
                  "id": "entity",
                  "summary": "Entity",
                  "description": "Tom &amp; Jerry",
                  "status": "confirmed",
                  "start": { "dateTime": "2026-05-01T13:00:00+08:00", "timeZone": "Asia/Singapore" },
                  "end": { "dateTime": "2026-05-01T14:00:00+08:00", "timeZone": "Asia/Singapore" }
                },
                {
                  "id": "legacy",
                  "summary": "Legacy",
                  "description": "Notes before\n\nLinked task: Focus\nhcb://task/task-legacy",
                  "status": "confirmed",
                  "start": { "dateTime": "2026-05-02T09:00:00Z" },
                  "end": { "dateTime": "2026-05-02T10:00:00Z" }
                },
                {
                  "id": "all-day",
                  "summary": "All day",
                  "description": "All day notes",
                  "status": "cancelled",
                  "start": { "date": "2026-05-03" },
                  "end": { "date": "2026-05-05" },
                  "recurrence": ["RRULE:FREQ=WEEKLY;COUNT=2"]
                }
              ],
              "nextSyncToken": "sync-next"
            }
            """#
            return (response, Data(body.utf8))
        }

        let page = try await client.listEvents(calendarID: "primary", syncToken: nil, timeMin: nil, defaultTimeZoneID: "UTC")
        let eventsByID = Dictionary(uniqueKeysWithValues: page.events.map { ($0.id, $0) })
        let plain = try XCTUnwrap(eventsByID["plain"])
        let html = try XCTUnwrap(eventsByID["html"])
        let entity = try XCTUnwrap(eventsByID["entity"])
        let legacy = try XCTUnwrap(eventsByID["legacy"])
        let allDay = try XCTUnwrap(eventsByID["all-day"])
        let internetFormatter = ISO8601DateFormatter()

        XCTAssertEqual(plain.details, "Plain notes\nsecond line")
        XCTAssertEqual(plain.startDate, try XCTUnwrap(internetFormatter.date(from: "2026-05-01T09:00:00Z")))
        XCTAssertEqual(plain.updatedAt, try XCTUnwrap(internetFormatter.date(from: "2026-05-01T01:02:03Z")))
        XCTAssertEqual(html.details, "**Team** & launch\nNext")
        XCTAssertEqual(html.startDate, try XCTUnwrap(ISO8601DateFormatter.google.date(from: "2026-05-01T11:00:00.123Z")))
        XCTAssertEqual(html.updatedAt, try XCTUnwrap(ISO8601DateFormatter.google.date(from: "2026-05-01T01:02:03.123Z")))
        XCTAssertEqual(entity.details, "Tom & Jerry")
        XCTAssertEqual(entity.startTimeZoneID, "Asia/Singapore")
        XCTAssertEqual(legacy.details, "Notes before")
        XCTAssertEqual(legacy.hcbTaskID, "task-legacy")
        XCTAssertTrue(allDay.isAllDay)
        XCTAssertEqual(allDay.status, .cancelled)
        XCTAssertEqual(allDay.recurrence, ["RRULE:FREQ=WEEKLY;COUNT=2"])
        XCTAssertEqual(Calendar.current.component(.day, from: allDay.startDate), 3)
        XCTAssertEqual(Calendar.current.component(.day, from: allDay.endDate), 5)
        XCTAssertEqual(page.nextSyncToken, "sync-next")
    }

    func testGoogleAPIDateDecoderParsesCommonCalendarWireForms() throws {
        let cases: [(wire: String, expected: Date)] = [
            ("2024-02-29", Self.utcDate(2024, 2, 29)),
            ("2026-03-08", Self.utcDate(2026, 3, 8)),
            ("2026-05-01T09:10:11Z", Self.utcDate(2026, 5, 1, 9, 10, 11)),
            ("2026-05-01T09:10:11.123Z", Self.utcDate(2026, 5, 1, 9, 10, 11, millisecond: 123)),
            ("2026-05-01T09:10:11+08:00", Self.utcDate(2026, 5, 1, 1, 10, 11)),
            ("2026-05-01T09:10:11.123+08:00", Self.utcDate(2026, 5, 1, 1, 10, 11, millisecond: 123)),
            ("2026-05-01T00:15:30-03:30", Self.utcDate(2026, 5, 1, 3, 45, 30)),
            ("2026-01-01T00:30:00+14:00", Self.utcDate(2025, 12, 31, 10, 30, 0)),
            ("2026-01-01T00:30:00-12:00", Self.utcDate(2026, 1, 1, 12, 30, 0)),
            ("2026-03-08T02:30:00-05:00", Self.utcDate(2026, 3, 8, 7, 30, 0)),
            ("2026-11-01T01:30:00-04:00", Self.utcDate(2026, 11, 1, 5, 30, 0))
        ]

        for testCase in cases {
            try Self.assertGoogleDate(testCase.wire, equals: testCase.expected)
        }
    }

    func testGoogleAPIDateDecoderPreservesFallbacksAndFailures() throws {
        let internetFormatter = ISO8601DateFormatter()
        internetFormatter.formatOptions = [.withInternetDateTime]
        let fractionalFormatter = ISO8601DateFormatter()
        fractionalFormatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        let lenientDateOnlyFormatter = DateFormatter()
        lenientDateOnlyFormatter.calendar = Calendar(identifier: .gregorian)
        lenientDateOnlyFormatter.locale = Locale(identifier: "en_US_POSIX")
        lenientDateOnlyFormatter.timeZone = TimeZone(secondsFromGMT: 0)
        lenientDateOnlyFormatter.dateFormat = "yyyy-MM-dd"

        try Self.assertGoogleDate(
            "2026-05-01T09:00:00+0800",
            equals: try XCTUnwrap(internetFormatter.date(from: "2026-05-01T09:00:00+0800"))
        )
        try Self.assertGoogleDate(
            "2026-05-01T09:00:00.123456Z",
            equals: try XCTUnwrap(fractionalFormatter.date(from: "2026-05-01T09:00:00.123456Z"))
        )
        try Self.assertGoogleDate(
            "2026-02-29",
            equals: try XCTUnwrap(lenientDateOnlyFormatter.date(from: "2026-02-29"))
        )

        for malformed in [
            "not-a-date",
            "2026-13-01",
            "2026-05-01T09:00Z",
            "2026-05-01T99:99:99Z"
        ] {
            XCTAssertThrowsError(try Self.decodedGoogleDate(malformed), malformed)
        }
    }

    func testListEventsMapsAvailabilityHoldMetadataVisibilityAndTransparency() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.httpMethod, "GET")
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "items": [
                {
                  "id": "hold-1",
                  "summary": "Hold: Design review",
                  "status": "confirmed",
                  "transparency": "opaque",
                  "visibility": "private",
                  "start": { "dateTime": "2026-05-01T09:00:00Z", "timeZone": "UTC" },
                  "end": { "dateTime": "2026-05-01T09:45:00Z", "timeZone": "UTC" },
                  "extendedProperties": {
                    "private": {
                      "hcbAvailabilityGroupID": "group-42",
                      "hcbAvailabilityRole": "hold",
                      "hcbAvailabilityTitle": "Design review",
                      "hcbAvailabilityDuration": "45",
                      "hcbAvailabilityCreatedAt": "2026-05-01T01:02:03.000Z"
                    }
                  }
                }
              ],
              "nextSyncToken": "sync-next"
            }
            """#
            return (response, Data(body.utf8))
        }

        let page = try await client.listEvents(calendarID: "primary", syncToken: nil)
        let event = try XCTUnwrap(page.events.first)
        let metadata = try XCTUnwrap(event.availabilityHold)
        let expectedCreatedAt = try XCTUnwrap(ISO8601DateFormatter.google.date(from: "2026-05-01T01:02:03.000Z"))

        XCTAssertEqual(event.visibility, .privateVisibility)
        XCTAssertEqual(event.transparency, .opaque)
        XCTAssertTrue(event.isAvailabilityHold)
        XCTAssertEqual(metadata.groupID, "group-42")
        XCTAssertEqual(metadata.title, "Design review")
        XCTAssertEqual(metadata.durationMinutes, 45)
        XCTAssertEqual(metadata.createdAt, expectedCreatedAt)
    }

    func testListEventsUsesTimeMinForFullSync() async throws {
        let timeMin = Date(timeIntervalSince1970: 1_713_900_000)
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let queryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []
            let query = Dictionary(uniqueKeysWithValues: queryItems.map { ($0.name, $0.value ?? "") })
            XCTAssertNil(query["syncToken"])
            XCTAssertEqual(query["timeMin"], ISO8601DateFormatter.google.string(from: timeMin))
            return (response, Data(#"{"items":[],"nextSyncToken":"full-sync"}"#.utf8))
        }

        let page = try await client.listEvents(calendarID: "primary", syncToken: nil, timeMin: timeMin)
        XCTAssertEqual(page.events.count, 0)
        XCTAssertEqual(page.nextSyncToken, "full-sync")
    }

    func testListEventsOmitsTimeMinForForeverFullSync() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let queryItems = URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []
            let query = Dictionary(uniqueKeysWithValues: queryItems.map { ($0.name, $0.value ?? "") })
            XCTAssertNil(query["syncToken"])
            XCTAssertNil(query["timeMin"])
            return (response, Data(#"{"items":[],"nextSyncToken":"full-sync"}"#.utf8))
        }

        let page = try await client.listEvents(calendarID: "primary", syncToken: nil, timeMin: nil)
        XCTAssertEqual(page.events.count, 0)
        XCTAssertEqual(page.nextSyncToken, "full-sync")
    }

    func testInsertEventPostsConferenceReminderAttendeesAndExtendedProperties() async throws {
        var capturedBody = ""
        MockURLProtocol.requestHandler = { request in
            capturedBody = Self.requestBodyString(from: request)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "id": "srv-1",
              "summary": "Planning",
              "description": "line one<br>line two",
              "location": "Room 1",
              "status": "confirmed",
              "start": { "dateTime": "2026-05-01T09:00:00Z" },
              "end": { "dateTime": "2026-05-01T10:00:00Z" },
              "reminders": {
                "useDefault": false,
                "overrides": [{ "method": "popup", "minutes": 30 }]
              },
              "attendees": [
                { "email": "alice@example.com", "responseStatus": "accepted" },
                { "email": "bob@example.com", "responseStatus": "tentative" }
              ],
              "conferenceData": {
                "entryPoints": [
                  { "entryPointType": "video", "uri": "https://meet.google.com/abc-defg-hij" }
                ]
              },
              "colorId": "11",
              "extendedProperties": {
                "private": { "hcbTaskID": "task-9" }
              }
            }
            """#
            return (response, Data(body.utf8))
        }

        let start = Date(timeIntervalSince1970: 1_714_553_200)
        let end = start.addingTimeInterval(3600)
        let event = try await client.insertEvent(
            calendarID: "primary",
            summary: "Planning",
            details: "line one\nline two",
            startDate: start,
            endDate: end,
            isAllDay: false,
            reminderMinutes: 30,
            location: "Room 1",
            recurrence: ["RRULE:FREQ=WEEKLY"],
            attendeeEmails: ["alice@example.com", "bob@example.com"],
            sendUpdates: "externalOnly",
            addGoogleMeet: true,
            colorId: "11",
            startTimeZoneID: "Asia/Singapore",
            endTimeZoneID: "Asia/Tokyo",
            hcbTaskID: "task-9"
        )

        let captured = try XCTUnwrap(MockURLProtocol.capturedRequests.first)
        XCTAssertEqual(captured.httpMethod, "POST")
        XCTAssertEqual(captured.url?.path, "/calendar/v3/calendars/primary/events")
        let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: captured.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
        XCTAssertEqual(query["sendUpdates"], "externalOnly")
        XCTAssertEqual(query["conferenceDataVersion"], "1")
        XCTAssertEqual(captured.value(forHTTPHeaderField: "Authorization"), "Bearer test-token")
        XCTAssertTrue(capturedBody.contains(#""summary":"Planning""#))
        XCTAssertTrue(capturedBody.contains(#""description":"line one<br>line two""#))
        XCTAssertTrue(capturedBody.contains(#""location":"Room 1""#))
        XCTAssertTrue(capturedBody.contains(#""recurrence":["RRULE:FREQ=WEEKLY"]"#))
        XCTAssertTrue(capturedBody.contains(#""minutes":30"#))
        XCTAssertTrue(capturedBody.contains(#""email":"alice@example.com""#))
        XCTAssertTrue(capturedBody.contains(#""email":"bob@example.com""#))
        XCTAssertTrue(capturedBody.contains(#""conferenceData":{"createRequest":"#))
        XCTAssertTrue(capturedBody.contains(#""colorId":"11""#))
        XCTAssertTrue(Self.body(capturedBody, containsJSONValue: "Asia/Singapore"))
        XCTAssertTrue(Self.body(capturedBody, containsJSONValue: "Asia/Tokyo"))
        XCTAssertTrue(capturedBody.contains(#""extendedProperties":{"private":{"hcbTaskID":"task-9"}}"#))
        XCTAssertEqual(event.meetLink, "https://meet.google.com/abc-defg-hij")
        XCTAssertEqual(event.hcbTaskID, "task-9")
        XCTAssertEqual(event.reminderMinutes, [30])
        XCTAssertEqual(event.attendeeEmails, ["alice@example.com", "bob@example.com"])
    }

    func testInsertAvailabilityHoldWritesPrivateOpaqueEventMetadata() async throws {
        var capturedBody = ""
        let createdAt = try XCTUnwrap(ISO8601DateFormatter.google.date(from: "2026-05-01T01:02:03.000Z"))
        MockURLProtocol.requestHandler = { request in
            capturedBody = Self.requestBodyString(from: request)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "id": "hold-1",
              "summary": "Hold: Design review",
              "status": "confirmed",
              "transparency": "opaque",
              "visibility": "private",
              "start": { "dateTime": "2026-05-01T09:00:00Z", "timeZone": "UTC" },
              "end": { "dateTime": "2026-05-01T09:45:00Z", "timeZone": "UTC" },
              "extendedProperties": {
                "private": {
                  "hcbAvailabilityGroupID": "group-42",
                  "hcbAvailabilityRole": "hold",
                  "hcbAvailabilityTitle": "Design review",
                  "hcbAvailabilityDuration": "45",
                  "hcbAvailabilityCreatedAt": "2026-05-01T01:02:03.000Z"
                }
              }
            }
            """#
            return (response, Data(body.utf8))
        }

        let start = Date(timeIntervalSince1970: 1_714_556_800)
        let event = try await client.insertEvent(
            calendarID: "primary",
            summary: "Hold: Design review",
            details: "Availability hold",
            startDate: start,
            endDate: start.addingTimeInterval(45 * 60),
            isAllDay: false,
            reminderMinutes: nil,
            startTimeZoneID: "UTC",
            endTimeZoneID: "UTC",
            transparency: .opaque,
            visibility: .privateVisibility,
            availabilityHold: AvailabilityHoldMetadata(
                groupID: "group-42",
                title: "Design review",
                durationMinutes: 45,
                createdAt: createdAt
            )
        )

        XCTAssertTrue(capturedBody.contains(#""transparency":"opaque""#))
        XCTAssertTrue(capturedBody.contains(#""visibility":"private""#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityGroupID":"group-42""#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityRole":"hold""#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityTitle":"Design review""#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityDuration":"45""#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityCreatedAt":"2026-05-01T01:02:03.000Z""#))
        XCTAssertEqual(event.availabilityHold?.groupID, "group-42")
        XCTAssertEqual(event.visibility, .privateVisibility)
        XCTAssertEqual(event.transparency, .opaque)
    }

    func testUpdateEventSendsIfMatchAndOmitsNilOptionalFields() async throws {
        var capturedBody = ""
        MockURLProtocol.requestHandler = { request in
            capturedBody = Self.requestBodyString(from: request)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "id": "srv-2",
              "summary": "Retitled",
              "status": "confirmed",
              "start": { "dateTime": "2026-05-01T11:00:00Z" },
              "end": { "dateTime": "2026-05-01T12:00:00Z" }
            }
            """#
            return (response, Data(body.utf8))
        }

        let start = Date(timeIntervalSince1970: 1_714_560_400)
        let end = start.addingTimeInterval(3600)
        _ = try await client.updateEvent(
            calendarID: "team@example.com",
            eventID: "evt/1",
            summary: "Retitled",
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: false,
            reminderMinutes: nil,
            location: "",
            recurrence: [],
            attendeeEmails: [],
            sendUpdates: "none",
            addGoogleMeet: false,
            colorId: nil,
            startTimeZoneID: "America/New_York",
            endTimeZoneID: "America/New_York",
            hcbTaskID: nil,
            ifMatch: "etag-123"
        )

        let captured = try XCTUnwrap(MockURLProtocol.capturedRequests.first)
        XCTAssertEqual(captured.httpMethod, "PATCH")
        XCTAssertEqual(
            captured.url?.path,
            "/calendar/v3/calendars/team@example.com/events/evt%2F1"
        )
        XCTAssertEqual(captured.value(forHTTPHeaderField: "If-Match"), "etag-123")
        let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: captured.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
        XCTAssertEqual(query["sendUpdates"], "none")
        XCTAssertNil(query["conferenceDataVersion"])
        XCTAssertFalse(capturedBody.contains(#""conferenceData""#))
        XCTAssertFalse(capturedBody.contains(#""colorId""#))
        XCTAssertFalse(capturedBody.contains(#""extendedProperties""#))
        XCTAssertTrue(Self.body(capturedBody, containsJSONValue: "America/New_York"))
    }

    func testUpdateEventCanClearAvailabilityHoldMetadata() async throws {
        var capturedBody = ""
        MockURLProtocol.requestHandler = { request in
            capturedBody = Self.requestBodyString(from: request)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "id": "hold-1",
              "summary": "Design review",
              "status": "confirmed",
              "transparency": "opaque",
              "visibility": "default",
              "start": { "dateTime": "2026-05-01T09:00:00Z", "timeZone": "UTC" },
              "end": { "dateTime": "2026-05-01T09:45:00Z", "timeZone": "UTC" },
              "extendedProperties": { "private": {} }
            }
            """#
            return (response, Data(body.utf8))
        }

        let start = Date(timeIntervalSince1970: 1_714_556_800)
        let event = try await client.updateEvent(
            calendarID: "primary",
            eventID: "hold-1",
            summary: "Design review",
            details: "Confirmed",
            startDate: start,
            endDate: start.addingTimeInterval(45 * 60),
            isAllDay: false,
            reminderMinutes: nil,
            startTimeZoneID: "UTC",
            endTimeZoneID: "UTC",
            transparency: .opaque,
            visibility: .defaultVisibility,
            clearAvailabilityHoldMetadata: true
        )

        XCTAssertTrue(capturedBody.contains(#""visibility":"default""#))
        XCTAssertTrue(capturedBody.contains(#""transparency":"opaque""#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityGroupID":null"#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityRole":null"#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityTitle":null"#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityDuration":null"#))
        XCTAssertTrue(capturedBody.contains(#""hcbAvailabilityCreatedAt":null"#))
        XCTAssertNil(event.availabilityHold)
        XCTAssertEqual(event.visibility, .defaultVisibility)
    }

    func testAllDayInsertOmitsTimezone() async throws {
        var capturedBody = ""
        MockURLProtocol.requestHandler = { request in
            capturedBody = Self.requestBodyString(from: request)
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let body = #"""
            {
              "id": "all-day",
              "summary": "Holiday",
              "status": "confirmed",
              "start": { "date": "2026-05-01" },
              "end": { "date": "2026-05-02" }
            }
            """#
            return (response, Data(body.utf8))
        }

        let start = Date(timeIntervalSince1970: 1_714_521_600)
        _ = try await client.insertEvent(
            calendarID: "primary",
            summary: "Holiday",
            details: "",
            startDate: start,
            endDate: start,
            isAllDay: true,
            reminderMinutes: nil,
            startTimeZoneID: "Asia/Singapore",
            endTimeZoneID: "Asia/Tokyo"
        )

        XCTAssertFalse(capturedBody.contains(#""timeZone""#))
        XCTAssertTrue(capturedBody.contains(#""date":"#))
    }

    func testMoveEventUsesDestinationCalendarAndReturnsMovedMirror() async throws {
        MockURLProtocol.requestHandler = { request in
            let response = HTTPURLResponse(
                url: request.url!,
                statusCode: 200,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
            XCTAssertEqual(request.httpMethod, "POST")
            XCTAssertEqual(request.url?.path, "/calendar/v3/calendars/source/events/evt-1/move")
            XCTAssertEqual(query["destination"], "dest")
            XCTAssertEqual(query["sendUpdates"], "none")
            let body = #"""
            {
              "id": "evt-1",
              "summary": "Moved",
              "status": "confirmed",
              "start": { "dateTime": "2026-05-02T09:00:00Z" },
              "end": { "dateTime": "2026-05-02T10:00:00Z" }
            }
            """#
            return (response, Data(body.utf8))
        }

        let moved = try await client.moveEvent(calendarID: "source", eventID: "evt-1", destinationCalendarID: "dest")
        XCTAssertEqual(moved.calendarID, "dest")
        XCTAssertEqual(moved.summary, "Moved")
    }

    func testDeleteAndPatchRecurrenceCarryIfMatch() async throws {
        var requestIndex = 0
        MockURLProtocol.requestHandler = { request in
            requestIndex += 1
            if requestIndex == 1 {
                XCTAssertEqual(request.httpMethod, "PATCH")
                XCTAssertEqual(request.value(forHTTPHeaderField: "If-Match"), "etag-recur")
                XCTAssertEqual(request.url?.path, "/calendar/v3/calendars/primary/events/series-1")
                let body = Self.requestBodyString(from: request)
                XCTAssertEqual(body, #"{"recurrence":["RRULE:FREQ=DAILY"]}"#)
                let response = HTTPURLResponse(
                    url: request.url!,
                    statusCode: 200,
                    httpVersion: nil,
                    headerFields: ["Content-Type": "application/json"]
                )!
                let json = #"""
                {
                  "id": "series-1",
                  "summary": "Series",
                  "status": "confirmed",
                  "start": { "dateTime": "2026-05-03T09:00:00Z" },
                  "end": { "dateTime": "2026-05-03T10:00:00Z" },
                  "recurrence": ["RRULE:FREQ=DAILY"]
                }
                """#
                return (response, Data(json.utf8))
            } else {
                XCTAssertEqual(request.httpMethod, "DELETE")
                XCTAssertEqual(request.value(forHTTPHeaderField: "If-Match"), "etag-delete")
                XCTAssertEqual(request.url?.path, "/calendar/v3/calendars/primary/events/evt-9")
                let query = Dictionary(uniqueKeysWithValues: (URLComponents(url: request.url!, resolvingAgainstBaseURL: false)?.queryItems ?? []).map { ($0.name, $0.value ?? "") })
                XCTAssertEqual(query["sendUpdates"], "none")
                let response = HTTPURLResponse(url: request.url!, statusCode: 204, httpVersion: nil, headerFields: nil)!
                return (response, Data())
            }
        }

        let patched = try await client.patchEventRecurrence(
            calendarID: "primary",
            eventID: "series-1",
            recurrence: ["RRULE:FREQ=DAILY"],
            ifMatch: "etag-recur"
        )
        XCTAssertEqual(patched.recurrence, ["RRULE:FREQ=DAILY"])

        try await client.deleteEvent(calendarID: "primary", eventID: "evt-9", ifMatch: "etag-delete")
        XCTAssertEqual(requestIndex, 2)
    }

    private static func requestBodyString(from request: URLRequest) -> String {
        if let body = request.httpBody {
            return String(decoding: body, as: UTF8.self)
        }
        if let stream = request.httpBodyStream {
            return read(stream: stream)
        }
        return ""
    }

    private static func body(_ body: String, containsJSONValue value: String) -> Bool {
        body.contains(value)
            || body.contains(value.replacingOccurrences(of: "/", with: #"\/"#))
    }

    private static func assertGoogleDate(
        _ wireValue: String,
        equals expected: Date,
        accuracy: TimeInterval = 0.000_001
    ) throws {
        let decoded = try decodedGoogleDate(wireValue)
        XCTAssertEqual(
            decoded.timeIntervalSince1970,
            expected.timeIntervalSince1970,
            accuracy: accuracy,
            wireValue
        )
    }

    private static func decodedGoogleDate(_ wireValue: String) throws -> Date {
        let data = try JSONSerialization.data(withJSONObject: ["value": wireValue], options: [])
        return try JSONDecoder.googleAPI.decode(GoogleDateDecodeBox.self, from: data).value
    }

    private static func utcDate(
        _ year: Int,
        _ month: Int,
        _ day: Int,
        _ hour: Int = 0,
        _ minute: Int = 0,
        _ second: Int = 0,
        millisecond: Int = 0
    ) -> Date {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(secondsFromGMT: 0)!
        var components = DateComponents()
        components.calendar = calendar
        components.timeZone = calendar.timeZone
        components.year = year
        components.month = month
        components.day = day
        components.hour = hour
        components.minute = minute
        components.second = second
        components.nanosecond = millisecond * 1_000_000
        return calendar.date(from: components)!
    }

    private static func read(stream: InputStream) -> String {
        var data = Data()
        stream.open()
        defer { stream.close() }

        let bufferSize = 1024
        let buffer = UnsafeMutablePointer<UInt8>.allocate(capacity: bufferSize)
        defer { buffer.deallocate() }

        while stream.hasBytesAvailable {
            let count = stream.read(buffer, maxLength: bufferSize)
            guard count > 0 else { break }
            data.append(buffer, count: count)
        }

        return String(decoding: data, as: UTF8.self)
    }
}

private struct GoogleDateDecodeBox: Decodable {
    var value: Date
}
