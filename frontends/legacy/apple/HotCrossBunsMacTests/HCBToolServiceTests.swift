import XCTest
@testable import HotCrossBunsMac

@MainActor
final class HCBToolServiceTests: XCTestCase {
    func testReadToolsReturnSanitizedLocalData() async throws {
        let service = HCBToolService(model: previewModel())

        let search = try await service.callTool(name: "hcb_search", arguments: ["query": "Draft", "limit": 5])
        let items = try XCTUnwrap(search["items"] as? [[String: Any]])
        XCTAssertEqual(items.first?["id"] as? String, "task-1")
        XCTAssertNil(items.first?["etag"])

        let today = try await service.callTool(name: "hcb_today", arguments: [:])
        let agenda = try XCTUnwrap(today["item"] as? [String: Any])
        XCTAssertNotNil(agenda["tasks"])
        XCTAssertNotNil(agenda["events"])

        let task = try await service.callTool(name: "hcb_get_task", arguments: ["id": "task-1"])
        XCTAssertEqual((task["item"] as? [String: Any])?["deepLink"] as? String, "hotcrossbuns://task/task-1")

        let event = try await service.callTool(name: "hcb_get_event", arguments: ["id": "event-1"])
        XCTAssertEqual((event["item"] as? [String: Any])?["deepLink"] as? String, "hotcrossbuns://event/event-1")

        let lists = try await service.callTool(name: "hcb_list_task_lists", arguments: [:])
        XCTAssertEqual((lists["items"] as? [[String: Any]])?.count, 2)

        let calendars = try await service.callTool(name: "hcb_list_calendars", arguments: [:])
        XCTAssertEqual((calendars["items"] as? [[String: Any]])?.count, 2)
    }

    func testReadOnlyModeDeniesWrites() async throws {
        let model = previewModel(permissionMode: .readOnly)
        let service = HCBToolService(model: model)

        do {
            _ = try await service.callTool(name: "hcb_create_task", arguments: ["title": "Blocked", "dryRun": true])
            XCTFail("Expected read-only denial")
        } catch let error as HCBToolError {
            XCTAssertEqual(error, .permissionDenied("MCP is in read-only mode."))
        }
    }

    func testConfirmWritesDryRunThenAppliesWithMatchingConfirmation() async throws {
        let model = previewModel(permissionMode: .confirmWrites, syncTargets: [])
        let service = HCBToolService(model: model)

        let dryRun = try await service.callTool(
            name: "hcb_create_note",
            arguments: ["title": "Agent note", "notes": "From MCP", "dryRun": true]
        )
        XCTAssertEqual(dryRun["applied"] as? Bool, false)
        let confirmationId = try XCTUnwrap(dryRun["confirmationId"] as? String)

        let applied = try await service.callTool(
            name: "hcb_create_note",
            arguments: ["title": "Agent note", "notes": "From MCP", "confirmationId": confirmationId]
        )
        XCTAssertEqual(applied["applied"] as? Bool, true)
        XCTAssertTrue(model.tasks.contains { $0.title == "Agent note" && $0.dueDate == nil })
    }

    func testConfirmWritesDoesNotMintConfirmationForRealWriteAttempt() async throws {
        let service = HCBToolService(model: previewModel(permissionMode: .confirmWrites, syncTargets: []))

        do {
            _ = try await service.callTool(
                name: "hcb_create_note",
                arguments: ["title": "No preview"]
            )
            XCTFail("Expected confirmation requirement")
        } catch let error as HCBToolError {
            XCTAssertEqual(error, .confirmationRequired("Dry-run confirmation is required before this write can apply.", nil))
            XCTAssertNil(error.confirmationId)
        }
    }

    func testConfirmWritesCanCreateDatedTaskAndEventAfterDryRun() async throws {
        let model = previewModel(permissionMode: .confirmWrites, syncTargets: [])
        let service = HCBToolService(model: model)

        let taskDryRun = try await service.callTool(
            name: "hcb_create_task",
            arguments: [
                "title": "Agent task",
                "notes": "From MCP",
                "dueDate": "2026-05-14",
                "dryRun": true
            ]
        )
        let taskConfirmationId = try XCTUnwrap(taskDryRun["confirmationId"] as? String)
        let taskApplied = try await service.callTool(
            name: "hcb_create_task",
            arguments: [
                "title": "Agent task",
                "notes": "From MCP",
                "dueDate": "2026-05-14",
                "confirmationId": taskConfirmationId
            ]
        )
        XCTAssertEqual(taskApplied["applied"] as? Bool, true)
        XCTAssertTrue(model.tasks.contains { $0.title == "Agent task" && $0.dueDate != nil })

        let eventDryRun = try await service.callTool(
            name: "hcb_create_event",
            arguments: [
                "title": "Agent event",
                "details": "From MCP",
                "startDate": "2026-05-14T09:00:00Z",
                "endDate": "2026-05-14T10:00:00Z",
                "calendarID": "planning",
                "dryRun": true
            ]
        )
        let eventConfirmationId = try XCTUnwrap(eventDryRun["confirmationId"] as? String)
        let eventApplied = try await service.callTool(
            name: "hcb_create_event",
            arguments: [
                "title": "Agent event",
                "details": "From MCP",
                "startDate": "2026-05-14T09:00:00Z",
                "endDate": "2026-05-14T10:00:00Z",
                "calendarID": "planning",
                "confirmationId": eventConfirmationId
            ]
        )
        XCTAssertEqual(eventApplied["applied"] as? Bool, true)
        XCTAssertTrue(model.events.contains { $0.summary == "Agent event" && $0.calendarID == "planning" })
    }

    func testConfirmationRejectsArgumentMismatch() async throws {
        let service = HCBToolService(model: previewModel(permissionMode: .confirmWrites, syncTargets: []))
        let dryRun = try await service.callTool(
            name: "hcb_create_task",
            arguments: ["title": "Original", "dryRun": true]
        )
        let confirmationId = try XCTUnwrap(dryRun["confirmationId"] as? String)

        do {
            _ = try await service.callTool(
                name: "hcb_create_task",
                arguments: ["title": "Changed", "confirmationId": confirmationId]
            )
            XCTFail("Expected confirmation mismatch")
        } catch let error as HCBToolError {
            XCTAssertEqual(error, .confirmationMismatch)
            XCTAssertNil(error.confirmationId)
        }
    }

    func testDestructiveToolsRequireConfirmationEvenWhenWritesAreAllowed() async throws {
        let service = HCBToolService(model: previewModel(permissionMode: .allowWrites, syncTargets: []))

        do {
            _ = try await service.callTool(name: "hcb_delete_task", arguments: ["id": "task-1"])
            XCTFail("Expected confirmation requirement")
        } catch let error as HCBToolError {
            XCTAssertNil(error.confirmationId)
        }

        let dryRun = try await service.callTool(
            name: "hcb_delete_task",
            arguments: ["id": "task-1", "dryRun": true]
        )
        let confirmationId = try XCTUnwrap(dryRun["confirmationId"] as? String)
        let applied = try await service.callTool(
            name: "hcb_delete_task",
            arguments: ["id": "task-1", "confirmationId": confirmationId]
        )
        XCTAssertEqual(applied["applied"] as? Bool, true)
    }

    func testInvalidIdsAndCredentialLikePatchFieldsAreHandledSafely() async throws {
        let service = HCBToolService(model: previewModel(permissionMode: .confirmWrites, syncTargets: []))

        do {
            _ = try await service.callTool(name: "hcb_get_task", arguments: ["id": "missing"])
            XCTFail("Expected not found")
        } catch let error as HCBToolError {
            XCTAssertEqual(error, .notFound("Task 'missing' was not found."))
        }

        let dryRun = try await service.callTool(
            name: "hcb_update_task",
            arguments: [
                "id": "task-1",
                "patch": [
                    "title": "New title",
                    "apiKey": "secret-value",
                    "nested": [
                        "refreshToken": "nested-secret-value",
                        "children": [
                            ["clientSecret": "array-secret-value"]
                        ]
                    ]
                ],
                "dryRun": true
            ]
        )
        let item = try XCTUnwrap(dryRun["item"] as? [String: Any])
        let patch = try XCTUnwrap(item["patch"] as? [String: Any])
        XCTAssertEqual(patch["apiKey"] as? String, "[redacted]")
        let nested = try XCTUnwrap(patch["nested"] as? [String: Any])
        XCTAssertEqual(nested["refreshToken"] as? String, "[redacted]")
        let children = try XCTUnwrap(nested["children"] as? [[String: Any]])
        XCTAssertEqual(children.first?["clientSecret"] as? String, "[redacted]")
        XCTAssertFalse(MCPServerController.jsonString(dryRun).contains("secret-value"))
        XCTAssertFalse(MCPServerController.jsonString(dryRun).contains("nested-secret-value"))
        XCTAssertFalse(MCPServerController.jsonString(dryRun).contains("array-secret-value"))
    }

    private func previewModel(
        permissionMode: MCPPermissionMode = .confirmWrites,
        syncTargets: Set<CloudSyncTarget> = CloudSyncTarget.all
    ) -> AppModel {
        let model = AppModel.preview
        var settings = model.settings
        settings.mcpPermissionMode = permissionMode
        settings.cloudSyncTargets = syncTargets
        model.updateSettings(settings)
        return model
    }
}
