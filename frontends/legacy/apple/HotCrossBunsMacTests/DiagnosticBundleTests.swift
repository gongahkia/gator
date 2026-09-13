import Foundation
import XCTest
@testable import HotCrossBunsMac

@MainActor
final class DiagnosticBundleTests: XCTestCase {
    func testBuildRedactsSecretsAndIncludesSummarySections() throws {
        let model = AppModel.preview
        let summary = NotificationScheduleSummary(
            scheduledEvents: 2,
            scheduledTasks: 3,
            deferredEvents: 1,
            deferredTasks: 4,
            failedEvents: 0,
            failedTasks: 0,
            windowDays: 30,
            computedAt: Date(timeIntervalSince1970: 1_714_000_000)
        )
        let bundle = try makeBundle(info: [
            "CFBundleShortVersionString": "9.9.9",
            "CFBundleVersion": "42"
        ])
        let environment = DiagnosticBundleEnvironment(
            now: { Date(timeIntervalSince1970: 1_714_000_000) },
            bundle: bundle,
            loadPersistedLog: {
                """
                Bearer top-secret-token
                Access token ya29.super-secret
                user tester@example.com
                """
            },
            readLastCrash: { "last user was crash@example.com" },
            logDirectoryPath: { "/tmp/hcb/logs" }
        )

        let output = DiagnosticBundle.build(
            model: model,
            cachePath: "/tmp/hcb/cache.json",
            notificationSummary: summary,
            environment: environment
        )

        XCTAssertTrue(output.contains("=== Hot Cross Buns Diagnostic Bundle ==="))
        XCTAssertTrue(output.contains("Version: 9.9.9 (build 42)"))
        XCTAssertTrue(output.contains("Cache path: /tmp/hcb/cache.json"))
        XCTAssertTrue(output.contains("Reminders scheduled: events=2 tasks=3 deferred_events=1 deferred_tasks=4 window=30d"))
        XCTAssertTrue(output.contains("Field-redacted Google payload logs: Disabled"))
        XCTAssertTrue(output.contains("Log folder: /tmp/hcb/logs"))
        XCTAssertTrue(output.contains("=== Pending Mutations (0) ==="))
        XCTAssertTrue(output.contains("=== Recent Logs (info+, all rotations) ==="))
        XCTAssertTrue(output.contains("=== Last Crash Breadcrumb ==="))
        XCTAssertTrue(output.contains("pe***@example.com"))
        XCTAssertTrue(output.contains("te***@example.com"))
        XCTAssertTrue(output.contains("cr***@example.com"))
        XCTAssertTrue(output.contains("Bearer <redacted>"))
        XCTAssertTrue(output.contains("ya29.<redacted>"))
        XCTAssertFalse(output.contains("personal@example.com"))
        XCTAssertFalse(output.contains("tester@example.com"))
        XCTAssertFalse(output.contains("crash@example.com"))
        XCTAssertFalse(output.contains("top-secret-token"))
        XCTAssertFalse(output.contains("ya29.super-secret"))
    }

    func testBuildRedactsRawGooglePayloadFieldsFromPersistedLogs() throws {
        let model = AppModel.preview
        let bundle = try makeBundle(info: [
            "CFBundleShortVersionString": "9.9.9",
            "CFBundleVersion": "42"
        ])
        let environment = DiagnosticBundleEnvironment(
            now: { Date(timeIntervalSince1970: 1_714_000_000) },
            bundle: bundle,
            loadPersistedLog: {
                #"""
                [2026-05-16T00:00:00.000Z] [INFO] [google] google request start requestBodySnippet={\"notes\":\"Private task body\",\"status\":\"needsAction\",\"title\":\"Private task title\"}
                [2026-05-16T00:00:01.000Z] [INFO] [google] google request succeeded responseBodySnippet={\"attendees\":[{\"displayName\":\"Alice Attendee\",\"email\":\"alice@example.com\"}],\"description\":\"Private event details\",\"hangoutLink\":\"https://meet.google.com/private\",\"id\":\"event-1\",\"location\":\"Secret room\",\"summary\":\"Private event title\"}
                """#
            },
            readLastCrash: { nil },
            logDirectoryPath: { nil }
        )

        let output = DiagnosticBundle.build(
            model: model,
            cachePath: "/tmp/hcb/cache.json",
            notificationSummary: nil,
            environment: environment
        )

        XCTAssertTrue(output.contains("requestBodySnippet="), output)
        XCTAssertTrue(output.contains("responseBodySnippet="), output)
        XCTAssertTrue(output.contains(#"\"status\":\"needsAction\""#), output)
        XCTAssertTrue(output.contains(#"\"id\":\"event-1\""#), output)
        XCTAssertTrue(output.contains(#"\"title\":\"<redacted>\""#), output)
        XCTAssertTrue(output.contains(#"\"summary\":\"<redacted>\""#), output)
        XCTAssertFalse(output.contains("Private task title"))
        XCTAssertFalse(output.contains("Private task body"))
        XCTAssertFalse(output.contains("Private event title"))
        XCTAssertFalse(output.contains("Private event details"))
        XCTAssertFalse(output.contains("Secret room"))
        XCTAssertFalse(output.contains("Alice Attendee"))
        XCTAssertFalse(output.contains("alice@example.com"))
        XCTAssertFalse(output.contains("https://meet.google.com/private"))
    }

    func testAppSettingsDecodesRawGoogleDiagnosticsDisabledWhenMissing() throws {
        let settings = try JSONDecoder().decode(AppSettings.self, from: Data("{}".utf8))
        XCTAssertFalse(settings.rawGoogleDiagnosticsEnabled)
    }

    func testRawGoogleDiagnosticsToggleUpdatesSettingsAndRuntimeFlag() {
        let model = AppModel.preview
        addTeardownBlock {
            GoogleDiagnostics.setRawPayloadLoggingEnabled(false)
        }

        model.setRawGoogleDiagnosticsEnabled(true)

        XCTAssertTrue(model.settings.rawGoogleDiagnosticsEnabled)
        XCTAssertTrue(GoogleDiagnostics.isRawPayloadLoggingEnabled)

        model.setRawGoogleDiagnosticsEnabled(false)

        XCTAssertFalse(model.settings.rawGoogleDiagnosticsEnabled)
        XCTAssertFalse(GoogleDiagnostics.isRawPayloadLoggingEnabled)
    }

    private func makeBundle(info: [String: Any]) throws -> Bundle {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString)
            .appendingPathExtension("bundle")
        let contentsURL = rootURL.appendingPathComponent("Contents", isDirectory: true)
        try FileManager.default.createDirectory(at: contentsURL, withIntermediateDirectories: true)
        addTeardownBlock {
            try? FileManager.default.removeItem(at: rootURL)
        }

        var plist = info
        plist["CFBundleIdentifier"] = plist["CFBundleIdentifier"] ?? "com.gongahkia.tests.bundle.\(UUID().uuidString)"
        plist["CFBundleName"] = plist["CFBundleName"] ?? "Fixture"
        plist["CFBundlePackageType"] = plist["CFBundlePackageType"] ?? "BNDL"
        plist["CFBundleVersion"] = plist["CFBundleVersion"] ?? "1"
        plist["CFBundleShortVersionString"] = plist["CFBundleShortVersionString"] ?? "1.0"

        let infoURL = contentsURL.appendingPathComponent("Info.plist")
        let data = try PropertyListSerialization.data(fromPropertyList: plist, format: .xml, options: 0)
        try data.write(to: infoURL)

        guard let bundle = Bundle(url: rootURL) else {
            XCTFail("Failed to create fixture bundle")
            throw FixtureBundleError.failedToLoadBundle
        }
        return bundle
    }
}

private enum FixtureBundleError: Error {
    case failedToLoadBundle
}
