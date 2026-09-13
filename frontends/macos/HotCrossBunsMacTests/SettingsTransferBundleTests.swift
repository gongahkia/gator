import XCTest
@testable import HotCrossBunsMac

final class SettingsTransferBundleTests: XCTestCase {
    func testSettingsBundleRoundTripsPreferences() throws {
        var settings = AppSettings.default
        settings.syncMode = .nearRealtime
        settings.colorSchemeID = "terminal"
        settings.uiLayoutScale = 1.2
        settings.uiTextSizePoints = 15
        settings.disableAnimations = true
        settings.showDockBadge = false
        settings.showCompletedItemsInCalendar = true
        settings.sidebarPlacement = .top
        settings.hiddenSidebarItems = ["notes"]
        settings.shortcutOverrides = [
            HCBShortcutCommand.refresh.rawValue: HCBKeyBinding(
                key: .char("r"),
                modifiers: [.command, .shift]
            )
        ]

        let bundle = SettingsTransferBundle(
            settings: settings,
            exportedAt: Date(timeIntervalSince1970: 1_777_777_777),
            appVersion: "test"
        )
        let data = try SettingsTransferBundle.encode(bundle)
        let decoded = try SettingsTransferBundle.decode(data)

        XCTAssertEqual(decoded.formatVersion, SettingsTransferBundle.currentFormatVersion)
        XCTAssertEqual(decoded.appVersion, "test")
        XCTAssertEqual(decoded.settings, settings)
        XCTAssertTrue(decoded.excludedFields.contains("Google account tokens"))
    }

    func testRejectsUnsupportedSettingsBundleVersion() throws {
        let json = """
        {
          "formatVersion": 999,
          "exportedAt": "2026-04-29T00:00:00Z",
          "appVersion": "test",
          "excludedFields": [],
          "settings": {}
        }
        """.data(using: .utf8)!

        XCTAssertThrowsError(try SettingsTransferBundle.decode(json)) { error in
            XCTAssertEqual(error as? SettingsTransferError, .unsupportedVersion(999))
        }
    }
}
