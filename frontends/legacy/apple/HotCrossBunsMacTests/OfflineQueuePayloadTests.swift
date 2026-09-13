import XCTest
@testable import HotCrossBunsMac

final class OfflineQueuePayloadTests: XCTestCase {
    func testTaskCreatePayloadRoundTrip() throws {
        let payload = PendingTaskCreatePayload(
            localID: "local-ABC",
            taskListID: "list1",
            title: "Write tests",
            notes: "Cover the offline queue",
            dueDate: Date(timeIntervalSince1970: 1_742_793_600),
            parentID: nil
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeTaskCreate(encoded)
        XCTAssertEqual(decoded, payload)
    }

    func testEventCreatePayloadRoundTrip() throws {
        let payload = PendingEventCreatePayload(
            localID: "local-EVT",
            calendarID: "primary",
            summary: "Review",
            details: "**notes**",
            startDate: Date(timeIntervalSince1970: 1_742_793_600),
            endDate: Date(timeIntervalSince1970: 1_742_797_200),
            isAllDay: false,
            reminderMinutes: 15,
            location: "HQ",
            recurrence: ["RRULE:FREQ=WEEKLY"],
            attendeeEmails: ["a@example.com"],
            notifyGuests: true,
            addGoogleMeet: true,
            colorId: "5",
            startTimeZoneID: "Asia/Singapore",
            endTimeZoneID: "Asia/Tokyo",
            transparency: .opaque,
            visibility: .privateVisibility,
            availabilityHold: AvailabilityHoldMetadata(
                groupID: "group-1",
                title: "Review",
                durationMinutes: 30,
                createdAt: Date(timeIntervalSince1970: 1_742_790_000)
            )
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeEventCreate(encoded)
        XCTAssertEqual(decoded, payload)
        XCTAssertEqual(decoded.startTimeZoneID, "Asia/Singapore")
        XCTAssertEqual(decoded.endTimeZoneID, "Asia/Tokyo")
        XCTAssertEqual(decoded.visibility, .privateVisibility)
        XCTAssertEqual(decoded.availabilityHold?.groupID, "group-1")
    }

    func testTaskUpdatePayloadRoundTripPreservesEtagSnapshot() throws {
        let payload = PendingTaskUpdatePayload(
            taskListID: "list1",
            taskID: "task-42",
            title: "Renamed",
            notes: "New notes",
            dueDate: nil,
            etagSnapshot: "etag-xyz"
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeTaskUpdate(encoded)
        XCTAssertEqual(decoded, payload)
        XCTAssertEqual(decoded.etagSnapshot, "etag-xyz")
    }

    func testTaskCompletionPayloadRoundTrip() throws {
        let payload = PendingTaskCompletionPayload(
            taskListID: "list1",
            taskID: "task-42",
            isCompleted: true,
            etagSnapshot: "etag-abc"
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeTaskCompletion(encoded)
        XCTAssertEqual(decoded, payload)
    }

    func testTaskDeletePayloadRoundTrip() throws {
        let payload = PendingTaskDeletePayload(
            taskListID: "list1",
            taskID: "task-42",
            etagSnapshot: "etag-del"
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeTaskDelete(encoded)
        XCTAssertEqual(decoded, payload)
    }

    func testEventUpdatePayloadRoundTrip() throws {
        let payload = PendingEventUpdatePayload(
            calendarID: "primary",
            eventID: "event-1",
            summary: "Moved",
            details: "Body",
            startDate: Date(timeIntervalSince1970: 1_742_793_600),
            endDate: Date(timeIntervalSince1970: 1_742_797_200),
            isAllDay: false,
            reminderMinutes: 30,
            location: "",
            recurrence: [],
            attendeeEmails: [],
            notifyGuests: false,
            etagSnapshot: "evt-etag",
            addGoogleMeet: false,
            colorId: nil,
            startTimeZoneID: "America/New_York",
            endTimeZoneID: "America/Los_Angeles",
            transparency: .opaque,
            visibility: .defaultVisibility,
            clearAvailabilityHoldMetadata: true
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeEventUpdate(encoded)
        XCTAssertEqual(decoded, payload)
        XCTAssertEqual(decoded.startTimeZoneID, "America/New_York")
        XCTAssertEqual(decoded.endTimeZoneID, "America/Los_Angeles")
        XCTAssertEqual(decoded.visibility, .defaultVisibility)
        XCTAssertTrue(decoded.clearAvailabilityHoldMetadata)
    }

    func testOlderEventPayloadsDecodeWithoutTimezoneFields() throws {
        let createJSON = #"""
        {
          "localID": "local-EVT",
          "calendarID": "primary",
          "summary": "Review",
          "details": "",
          "startDate": 1742793600,
          "endDate": 1742797200,
          "isAllDay": false,
          "reminderMinutes": 15
        }
        """#
        let updateJSON = #"""
        {
          "calendarID": "primary",
          "eventID": "event-1",
          "summary": "Review",
          "details": "",
          "startDate": 1742793600,
          "endDate": 1742797200,
          "isAllDay": false,
          "reminderMinutes": 15,
          "location": "",
          "recurrence": [],
          "attendeeEmails": [],
          "notifyGuests": false
        }
        """#

        let create = try PendingMutationEncoder.decodeEventCreate(Data(createJSON.utf8))
        let update = try PendingMutationEncoder.decodeEventUpdate(Data(updateJSON.utf8))
        XCTAssertNil(create.startTimeZoneID)
        XCTAssertNil(create.endTimeZoneID)
        XCTAssertNil(update.startTimeZoneID)
        XCTAssertNil(update.endTimeZoneID)
        XCTAssertNil(create.availabilityHold)
        XCTAssertNil(update.availabilityHold)
        XCTAssertFalse(update.clearAvailabilityHoldMetadata)
    }

    func testEventDeletePayloadRoundTrip() throws {
        let payload = PendingEventDeletePayload(
            calendarID: "primary",
            eventID: "event-1",
            etagSnapshot: "evt-etag"
        )
        let encoded = try PendingMutationEncoder.encode(payload)
        let decoded = try PendingMutationEncoder.decodeEventDelete(encoded)
        XCTAssertEqual(decoded, payload)
    }

    func testMutationFactoriesProduceCorrectDiscriminators() throws {
        let taskCreate = try PendingMutation.taskCreate(payload: .init(
            localID: "local-1", taskListID: "l", title: "t", notes: "", dueDate: nil, parentID: nil
        ))
        XCTAssertEqual(taskCreate.resourceType, .task)
        XCTAssertEqual(taskCreate.action, .create)

        let taskUpdate = try PendingMutation.taskUpdate(payload: .init(
            taskListID: "l", taskID: "t", title: "x", notes: "", dueDate: nil, etagSnapshot: nil
        ))
        XCTAssertEqual(taskUpdate.resourceType, .task)
        XCTAssertEqual(taskUpdate.action, .update)

        let taskCompletion = try PendingMutation.taskCompletion(payload: .init(
            taskListID: "l", taskID: "t", isCompleted: true, etagSnapshot: nil
        ))
        XCTAssertEqual(taskCompletion.resourceType, .task)
        XCTAssertEqual(taskCompletion.action, .completion)

        let taskDelete = try PendingMutation.taskDelete(payload: .init(
            taskListID: "l", taskID: "t", etagSnapshot: nil
        ))
        XCTAssertEqual(taskDelete.resourceType, .task)
        XCTAssertEqual(taskDelete.action, .delete)

        let eventUpdate = try PendingMutation.eventUpdate(payload: .init(
            calendarID: "c", eventID: "e", summary: "s", details: "", startDate: .distantPast, endDate: .distantFuture, isAllDay: false, reminderMinutes: nil, location: "", recurrence: [], attendeeEmails: [], notifyGuests: false, etagSnapshot: nil
        ))
        XCTAssertEqual(eventUpdate.resourceType, .event)
        XCTAssertEqual(eventUpdate.action, .update)

        let eventDelete = try PendingMutation.eventDelete(payload: .init(
            calendarID: "c", eventID: "e", etagSnapshot: nil
        ))
        XCTAssertEqual(eventDelete.resourceType, .event)
        XCTAssertEqual(eventDelete.action, .delete)
    }

    func testIsTransientClassification() {
        XCTAssertTrue(GoogleAPIError.httpStatus(429, nil).isTransient)
        XCTAssertTrue(GoogleAPIError.httpStatus(500, nil).isTransient)
        XCTAssertTrue(GoogleAPIError.httpStatus(503, nil).isTransient)
        XCTAssertTrue(GoogleAPIError.httpStatus(599, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(429, #"{"error":{"reason":"quotaExceeded"}}"#).isTransient)
        XCTAssertFalse(GoogleAPIError.invalidPayload(nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(400, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(401, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(403, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(404, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.preconditionFailed.isTransient)
        XCTAssertFalse(GoogleAPIError.invalidResponse.isTransient)
        XCTAssertFalse(GoogleAPIError.invalidURL.isTransient)
    }

    func testSyncFailureKindClassification() {
        XCTAssertEqual(SyncFailureKind.classify(GoogleAPIError.httpStatus(429, nil)), .rateLimited)
        XCTAssertEqual(
            SyncFailureKind.classify(GoogleAPIError.httpStatus(403, #"{"error":{"reason":"quotaExceeded"}}"#)),
            .quotaExceeded
        )
        XCTAssertEqual(SyncFailureKind.classify(GoogleAPIError.httpStatus(503, nil)), .serviceUnavailable)
        XCTAssertEqual(SyncFailureKind.classify(GoogleAPIError.invalidPayload(nil)), .invalidPayload)
        XCTAssertEqual(SyncFailureKind.classify(GoogleAPIError.httpStatus(404, nil)), .notFound)
        XCTAssertEqual(
            SyncFailureKind.classify(URLError(.notConnectedToInternet)),
            .offline
        )
    }

    func testSyncFailureCopyUsesOfflineMessaging() {
        let copy = AppStatusBanner.syncFailureCopy(
            fallbackMessage: "ignored",
            isPaused: false,
            failureKind: .other,
            networkReachability: .offline
        )

        XCTAssertEqual(copy.title, "You're offline")
        XCTAssertEqual(copy.message, "Changes are queued locally and will sync when you reconnect.")
        XCTAssertEqual(copy.systemImage, "wifi.slash")
    }

    func testSyncFailureCopyUsesRateLimitMessaging() {
        let copy = AppStatusBanner.syncFailureCopy(
            fallbackMessage: "ignored",
            isPaused: false,
            failureKind: .rateLimited,
            networkReachability: .online
        )

        XCTAssertEqual(copy.title, "Google is rate-limiting requests")
        XCTAssertEqual(copy.message, "Hot Cross Buns will retry automatically in the current backoff window, usually about 1-2 minutes. Your local changes are safe.")
    }

    func testSyncFailureCopyUsesQuotaMessaging() {
        let copy = AppStatusBanner.syncFailureCopy(
            fallbackMessage: "ignored",
            isPaused: true,
            failureKind: .quotaExceeded,
            networkReachability: .online
        )

        XCTAssertEqual(copy.title, "Sync paused because Google quota is exhausted")
        XCTAssertTrue(copy.message.contains("Automatic retry will not help"))
    }

    func testSyncFailureCopyUsesServiceUnavailableMessaging() {
        let copy = AppStatusBanner.syncFailureCopy(
            fallbackMessage: "ignored",
            isPaused: false,
            failureKind: .serviceUnavailable,
            networkReachability: .online
        )

        XCTAssertEqual(copy.title, "Google Calendar or Tasks is briefly unavailable")
        XCTAssertEqual(copy.message, "Hot Cross Buns will retry automatically as soon as the service recovers.")
    }

    func testTimezoneSupportNormalizesOnulLegacyIdentifiers() {
        XCTAssertEqual(TimezoneSupport.validatedIdentifier("US/Eastern"), "America/New_York")
        XCTAssertEqual(TimezoneSupport.validatedIdentifier("Asia/Calcutta"), "Asia/Kolkata")
        XCTAssertEqual(TimezoneSupport.validatedIdentifier("GMT"), "UTC")
        XCTAssertEqual(TimezoneSupport.validatedIdentifier("Z"), "UTC")
        XCTAssertEqual(TimezoneSupport.validatedIdentifier("Asia/Singapore"), "Asia/Singapore")
        XCTAssertNil(TimezoneSupport.validatedIdentifier("Not/AZone"))
    }

    func testTimezoneSupportCompactDisplayNameUsesOffsetAndPlace() {
        XCTAssertEqual(TimezoneSupport.compactDisplayName(for: "UTC"), "GMT+00:00 UTC")

        let singapore = TimezoneSupport.compactDisplayName(for: "Asia/Singapore")
        XCTAssertTrue(singapore.hasPrefix("GMT+08:00 "))
        XCTAssertTrue(singapore.hasSuffix("Singapore"))
    }

    func testSyncFailureCopyUsesNotFoundMessaging() {
        let copy = AppStatusBanner.syncFailureCopy(
            fallbackMessage: #"{"error":{"code":404,"message":"Not Found"}}"#,
            isPaused: false,
            failureKind: .notFound,
            networkReachability: .online
        )

        XCTAssertEqual(copy.title, "Signed in, but Google couldn't find cached sync data")
        XCTAssertTrue(copy.message.contains("Login worked"))
        XCTAssertTrue(copy.message.contains("different Google account"))
        XCTAssertTrue(copy.message.contains("Force Full Resync"))
        XCTAssertFalse(copy.message.contains("\"error\""))
    }

    func testFirstSyncBannerIncludesScopeWhenAvailable() {
        XCTAssertEqual(
            AppStatusBanner.syncingInfoTitle(
                daysSinceLastLaunch: 3,
                syncScope: SyncScopeSummary(tasks: 12, events: 7)
            ),
            "3 days since last open — fetching roughly 12 tasks and 7 events from Google…"
        )
    }
}
