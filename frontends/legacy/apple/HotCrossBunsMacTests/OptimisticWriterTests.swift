import XCTest
@testable import HotCrossBunsMac

final class OptimisticWriterTests: XCTestCase {
    func testGenerateProducesPendingID() {
        let id = OptimisticID.generate()
        XCTAssertTrue(OptimisticID.isPending(id))
    }

    func testIsPendingRejectsGoogleIDs() {
        XCTAssertFalse(OptimisticID.isPending("abc123"))
        XCTAssertFalse(OptimisticID.isPending("MTIzNDU2"))
    }

    func testTaskPayloadRoundTripsThroughPendingMutation() throws {
        let payload = PendingTaskCreatePayload(
            localID: OptimisticID.generate(),
            taskListID: "list-1",
            title: "Pay rent",
            notes: "ACH set up",
            dueDate: Date(timeIntervalSince1970: 1_745_000_000),
            parentID: nil
        )
        let mutation = try PendingMutation.taskCreate(payload: payload)
        XCTAssertEqual(mutation.resourceType, .task)
        XCTAssertEqual(mutation.action, .create)
        XCTAssertEqual(mutation.resourceID, payload.localID)

        let decoded = try PendingMutationEncoder.decodeTaskCreate(mutation.payload)
        XCTAssertEqual(decoded, payload)
    }

    func testEventPayloadRoundTripsThroughPendingMutation() throws {
        let payload = PendingEventCreatePayload(
            localID: OptimisticID.generate(),
            calendarID: "primary",
            summary: "Planning",
            details: "Sprint review",
            startDate: Date(timeIntervalSince1970: 1_745_000_000),
            endDate: Date(timeIntervalSince1970: 1_745_003_600),
            isAllDay: false,
            reminderMinutes: 10
        )
        let mutation = try PendingMutation.eventCreate(payload: payload)
        XCTAssertEqual(mutation.resourceType, .event)
        XCTAssertEqual(mutation.action, .create)
        let decoded = try PendingMutationEncoder.decodeEventCreate(mutation.payload)
        XCTAssertEqual(decoded, payload)
    }

    func testTransientErrorClassification() {
        XCTAssertTrue(GoogleAPIError.httpStatus(429, nil).isTransient)
        XCTAssertTrue(GoogleAPIError.httpStatus(503, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(400, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(401, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.httpStatus(404, nil).isTransient)
        XCTAssertFalse(GoogleAPIError.preconditionFailed.isTransient)
    }
}
