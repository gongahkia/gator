import Foundation
import XCTest
@testable import HotCrossBunsMac

@MainActor
final class ReleaseConfigGateTests: XCTestCase {
    func testUpdaterControllerUsesGitHubReleaseChecks() throws {
        let suiteName = "ReleaseConfigGateTests-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.removePersistentDomain(forName: suiteName)
        addTeardownBlock {
            defaults.removePersistentDomain(forName: suiteName)
        }

        let controller = UpdaterController(
            bundle: try makeBundle(info: [:]),
            userDefaults: defaults,
            urlSession: .shared,
            openURL: { _ in true }
        )

        XCTAssertEqual(controller.updateSourceLabel, "GitHub Releases")
        XCTAssertEqual(controller.automaticCheckLabel, "Check GitHub releases automatically")
        XCTAssertTrue(controller.automaticallyChecksForUpdates)
    }

    func testGitHubReleaseCheckDetectsNewerVersionAndDownload() async throws {
        let session = try makeStubbedSession(statusCode: 200, body: """
        {
          "name": "Spring Refresh",
          "tag_name": "v1.2.0",
          "html_url": "https://github.com/gongahkia/hot-cross-buns/releases/tag/v1.2.0",
          "published_at": "2026-04-24T00:00:00Z",
          "body": "## Changes\\n- Added update prompts",
          "assets": [
            {
              "name": "HotCrossBuns-macOS.dmg",
              "browser_download_url": "https://github.com/gongahkia/hot-cross-buns/releases/download/v1.2.0/HotCrossBuns-macOS.dmg"
            }
          ]
        }
        """)
        let suiteName = "ReleaseConfigGateTests-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.removePersistentDomain(forName: suiteName)
        let downloadsURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: downloadsURL, withIntermediateDirectories: true, attributes: nil)
        addTeardownBlock {
            defaults.removePersistentDomain(forName: suiteName)
            try? FileManager.default.removeItem(at: downloadsURL)
        }

        let controller = UpdaterController(
            bundle: try makeBundle(info: ["CFBundleShortVersionString": "1.0.0"]),
            userDefaults: defaults,
            urlSession: session,
            openURL: { _ in true },
            now: { Date(timeIntervalSince1970: 1_777_000_000) },
            downloadsDirectory: { downloadsURL },
            releaseAssetDownloader: { _, destinationURL, progress in
                progress(0.5)
                try Data("fixture".utf8).write(to: destinationURL)
                progress(1)
            }
        )

        await controller.checkForUpdatesNow(trigger: .manual)

        XCTAssertEqual(controller.availableRelease?.version, "1.2.0")
        XCTAssertEqual(controller.availableRelease?.title, "Spring Refresh")
        XCTAssertEqual(controller.availableRelease?.notesMarkdown, "## Changes\n- Added update prompts")
        XCTAssertEqual(controller.availableRelease?.downloadURL?.absoluteString, "https://github.com/gongahkia/hot-cross-buns/releases/download/v1.2.0/HotCrossBuns-macOS.dmg")
        XCTAssertEqual(controller.toastState?.title, "Update ready to install")
        XCTAssertEqual(controller.updatePromptSequence, 1)
        XCTAssertEqual(controller.downloadState.phase, .ready)
        XCTAssertEqual(controller.downloadState.fileURL?.lastPathComponent, "HotCrossBuns-macOS.dmg")
        XCTAssertTrue(FileManager.default.fileExists(atPath: try XCTUnwrap(controller.downloadState.fileURL).path))
        XCTAssertNotNil(controller.lastUpdateCheckDate)
    }

    func testGitHubReleaseCheckReportsLatestVersion() async throws {
        let session = try makeStubbedSession(statusCode: 200, body: """
        {
          "tag_name": "v1.0.0",
          "html_url": "https://github.com/gongahkia/hot-cross-buns/releases/tag/v1.0.0",
          "published_at": "2026-04-24T00:00:00Z",
          "assets": []
        }
        """)
        let suiteName = "ReleaseConfigGateTests-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.removePersistentDomain(forName: suiteName)
        addTeardownBlock {
            defaults.removePersistentDomain(forName: suiteName)
        }

        let controller = UpdaterController(
            bundle: try makeBundle(info: ["CFBundleShortVersionString": "1.0.0"]),
            userDefaults: defaults,
            urlSession: session,
            openURL: { _ in true }
        )

        await controller.checkForUpdatesNow(trigger: .manual)

        XCTAssertNil(controller.availableRelease)
        XCTAssertEqual(controller.toastState?.title, "You're on the latest version")
        XCTAssertEqual(controller.toastState?.target, .main)
    }

    func testSettingsUpdateCheckTargetsSettingsToastOnly() async throws {
        let session = try makeStubbedSession(statusCode: 200, body: """
        {
          "tag_name": "v1.0.0",
          "html_url": "https://github.com/gongahkia/hot-cross-buns/releases/tag/v1.0.0",
          "published_at": "2026-04-24T00:00:00Z",
          "assets": []
        }
        """)
        let suiteName = "ReleaseConfigGateTests-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.removePersistentDomain(forName: suiteName)
        addTeardownBlock {
            defaults.removePersistentDomain(forName: suiteName)
        }

        let controller = UpdaterController(
            bundle: try makeBundle(info: ["CFBundleShortVersionString": "1.0.0"]),
            userDefaults: defaults,
            urlSession: session,
            openURL: { _ in true }
        )

        await controller.checkForUpdatesNow(trigger: .manual, toastTarget: .settings)

        XCTAssertNil(controller.availableRelease)
        XCTAssertEqual(controller.toastState?.title, "You're on the latest version")
        XCTAssertEqual(controller.toastState?.target, .settings)
    }

    func testOpenAvailableReleaseDownloadPrefersDownloadedLocalDMG() async throws {
        let session = try makeStubbedSession(statusCode: 200, body: """
        {
          "tag_name": "v1.2.0",
          "html_url": "https://github.com/gongahkia/hot-cross-buns/releases/tag/v1.2.0",
          "published_at": "2026-04-24T00:00:00Z",
          "assets": [
            {
              "name": "HotCrossBuns-macOS.dmg",
              "browser_download_url": "https://github.com/gongahkia/hot-cross-buns/releases/download/v1.2.0/HotCrossBuns-macOS.dmg"
            }
          ]
        }
        """)
        let suiteName = "ReleaseConfigGateTests-\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suiteName)!
        defaults.removePersistentDomain(forName: suiteName)
        let downloadsURL = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
        try FileManager.default.createDirectory(at: downloadsURL, withIntermediateDirectories: true, attributes: nil)
        addTeardownBlock {
            defaults.removePersistentDomain(forName: suiteName)
            try? FileManager.default.removeItem(at: downloadsURL)
        }

        var openedURL: URL?
        let controller = UpdaterController(
            bundle: try makeBundle(info: ["CFBundleShortVersionString": "1.0.0"]),
            userDefaults: defaults,
            urlSession: session,
            openURL: {
                openedURL = $0
                return true
            },
            downloadsDirectory: { downloadsURL },
            releaseAssetDownloader: { _, destinationURL, progress in
                progress(nil)
                try Data("fixture".utf8).write(to: destinationURL)
                progress(1)
            }
        )

        await controller.checkForUpdatesNow(trigger: .manual)
        controller.openAvailableReleaseDownload()

        XCTAssertEqual(openedURL?.isFileURL, true)
        XCTAssertEqual(openedURL?.lastPathComponent, "HotCrossBuns-macOS.dmg")
        XCTAssertEqual(controller.installGuideSequence, 1)
    }

    func testGoogleAuthServiceRequiresConcreteClientID() throws {
        func service(info: [String: Any]) throws -> GoogleAuthService {
            GoogleAuthService(
                bundle: try makeBundle(info: info),
                customOAuthService: CustomGoogleOAuthService(tokenStore: InMemoryGoogleOAuthTokenStore(clientConfiguration: nil))
            )
        }

        XCTAssertFalse(try service(info: [:]).isConfigured)
        XCTAssertFalse(try service(info: [
            "GIDClientID": ""
        ]).isConfigured)
        XCTAssertFalse(try service(info: [
            "GIDClientID": "your-macos-oauth-client-id.apps.googleusercontent.com"
        ]).isConfigured)
        XCTAssertFalse(try service(info: [
            "GIDClientID": "$(GOOGLE_MACOS_CLIENT_ID)"
        ]).isConfigured)
        XCTAssertTrue(try service(info: [
            "GIDClientID": "123456789012-syntheticclientid.apps.googleusercontent.com"
        ]).isConfigured)
    }

    private func makeBundle(info: [String: Any]) throws -> Bundle {
        let rootURL = FileManager.default.temporaryDirectory
            .appendingPathComponent(UUID().uuidString)
            .appendingPathExtension("bundle")
        let contentsURL = rootURL.appendingPathComponent("Contents", isDirectory: true)
        try FileManager.default.createDirectory(at: contentsURL, withIntermediateDirectories: true, attributes: nil)
        addTeardownBlock {
            try? FileManager.default.removeItem(at: rootURL)
        }

        var plist = info
        plist["CFBundleIdentifier"] = plist["CFBundleIdentifier"] ?? "com.gongahkia.tests.\(UUID().uuidString)"
        plist["CFBundleName"] = plist["CFBundleName"] ?? "Fixture"
        plist["CFBundlePackageType"] = plist["CFBundlePackageType"] ?? "BNDL"
        plist["CFBundleVersion"] = plist["CFBundleVersion"] ?? "1"
        plist["CFBundleShortVersionString"] = plist["CFBundleShortVersionString"] ?? "1.0"

        let infoURL = contentsURL.appendingPathComponent("Info.plist")
        let data = try PropertyListSerialization.data(fromPropertyList: plist, format: .xml, options: 0)
        try data.write(to: infoURL)

        guard let bundle = Bundle(url: rootURL) else {
            XCTFail("Failed to create temporary fixture bundle at \(rootURL.path)")
            throw FixtureBundleError.failedToLoadBundle
        }
        return bundle
    }

    private func makeStubbedSession(statusCode: Int, body: String) throws -> URLSession {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [StubURLProtocol.self]
        StubURLProtocol.handler = { request in
            let response = HTTPURLResponse(
                url: try XCTUnwrap(request.url),
                statusCode: statusCode,
                httpVersion: nil,
                headerFields: ["Content-Type": "application/json"]
            )!
            return (response, Data(body.utf8))
        }
        addTeardownBlock {
            StubURLProtocol.handler = nil
        }
        return URLSession(configuration: configuration)
    }
}

private enum FixtureBundleError: Error {
    case failedToLoadBundle
}

private final class StubURLProtocol: URLProtocol {
    static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool {
        true
    }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest {
        request
    }

    override func startLoading() {
        guard let handler = Self.handler else {
            client?.urlProtocol(self, didFailWithError: URLError(.badServerResponse))
            return
        }
        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}
