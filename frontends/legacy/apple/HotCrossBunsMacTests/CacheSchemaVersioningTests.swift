import XCTest
@testable import HotCrossBunsMac

final class CacheSchemaVersioningTests: XCTestCase {
    func testNewlyCreatedStateIsAtCurrentVersion() {
        XCTAssertEqual(CachedAppState.empty.schemaVersion, CachedAppState.currentSchemaVersion)
    }

    func testEncodedStateIncludesSchemaVersion() throws {
        let encoder = JSONEncoder()
        let data = try encoder.encode(CachedAppState.empty)
        let json = try XCTUnwrap(
            try JSONSerialization.jsonObject(with: data) as? [String: Any]
        )
        XCTAssertEqual(json["schemaVersion"] as? Int, CachedAppState.currentSchemaVersion)
    }

    func testLegacyCacheWithoutSchemaVersionLoadsAsCurrent() throws {
        // Simulates a cache written before schema versioning existed. All
        // the existing fields decode as-is; the missing schemaVersion
        // should default to zero and then be migrated forward so the next
        // save emits the current version.
        let legacyJSON = """
        {
            "taskLists": [],
            "tasks": [],
            "calendars": [],
            "events": [],
            "settings": {
                "syncMode": "balanced",
                "selectedCalendarIDs": [],
                "selectedTaskListIDs": [],
                "enableLocalNotifications": false,
                "hasCompletedOnboarding": true
            },
            "syncCheckpoints": [],
            "pendingMutations": []
        }
        """
        let data = Data(legacyJSON.utf8)
        let state = try JSONDecoder().decode(CachedAppState.self, from: data)
        XCTAssertEqual(state.schemaVersion, CachedAppState.currentSchemaVersion)
        XCTAssertTrue(state.settings.hasCompletedOnboarding)
    }

    func testFutureVersionIsPreservedNotDowngraded() throws {
        // If a user installs a newer build, then downgrades, the older
        // build will see a cache with schemaVersion > currentSchemaVersion.
        // We should not silently rewrite it to a lower number; leave it
        // alone and let the decode-as-latest fallback handle what it can.
        let futureSchema = CachedAppState.currentSchemaVersion + 5
        let futureJSON = """
        {
            "schemaVersion": \(futureSchema),
            "taskLists": [],
            "tasks": [],
            "calendars": [],
            "events": [],
            "settings": {
                "syncMode": "balanced",
                "selectedCalendarIDs": [],
                "selectedTaskListIDs": [],
                "enableLocalNotifications": false,
                "hasCompletedOnboarding": false
            },
            "syncCheckpoints": [],
            "pendingMutations": []
        }
        """
        let data = Data(futureJSON.utf8)
        let state = try JSONDecoder().decode(CachedAppState.self, from: data)
        XCTAssertEqual(state.schemaVersion, futureSchema)
    }

    func testMigratorIsNoOpForCurrentToCurrent() {
        let result = CacheSchemaMigrator.migrateInPlace(
            from: CachedAppState.currentSchemaVersion,
            to: CachedAppState.currentSchemaVersion
        )
        XCTAssertEqual(result, CachedAppState.currentSchemaVersion)
    }
}
