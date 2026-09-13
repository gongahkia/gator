import AppKit
import Foundation
import Observation

@MainActor
@Observable
final class UpdaterController {
    struct ToastState: Equatable {
        let title: String
        let message: String
        let isWarning: Bool
        let target: ToastTarget

        init(
            title: String,
            message: String,
            isWarning: Bool,
            target: ToastTarget = .main
        ) {
            self.title = title
            self.message = message
            self.isWarning = isWarning
            self.target = target
        }
    }

    enum ToastTarget: Equatable {
        case main
        case settings
    }

    struct AvailableRelease: Equatable {
        let title: String?
        let version: String
        let tagName: String
        let htmlURL: URL
        let downloadURL: URL?
        let downloadFilename: String?
        let publishedAt: Date?
        let notesMarkdown: String
    }

    struct DownloadState: Equatable {
        enum Phase: Equatable {
            case idle
            case downloading
            case ready
            case failed
        }

        let phase: Phase
        let releaseTag: String?
        let progress: Double?
        let fileURL: URL?
        let message: String?

        static let idle = DownloadState(
            phase: .idle,
            releaseTag: nil,
            progress: nil,
            fileURL: nil,
            message: nil
        )
    }

    enum CheckTrigger {
        case manual
        case automatic
    }

    private enum DefaultsKey {
        static let autoCheckEnabled = "hcb.updates.autoCheckEnabled"
        static let lastCheckAt = "hcb.updates.lastCheckAt"
    }

    private struct GitHubAsset: Decodable {
        let name: String
        let browserDownloadURL: URL

        enum CodingKeys: String, CodingKey {
            case name
            case browserDownloadURL = "browser_download_url"
        }
    }

    private struct GitHubRelease: Decodable {
        let name: String?
        let tagName: String
        let htmlURL: URL
        let publishedAt: Date?
        let body: String?
        let assets: [GitHubAsset]

        enum CodingKeys: String, CodingKey {
            case name
            case tagName = "tag_name"
            case htmlURL = "html_url"
            case publishedAt = "published_at"
            case body
            case assets
        }
    }

    typealias ReleaseAssetDownloader = @Sendable (
        URL,
        URL,
        @escaping @Sendable (Double?) -> Void
    ) async throws -> Void

    private struct ReleaseVersion: Comparable {
        let components: [Int]

        init(_ rawValue: String) {
            components = rawValue
                .split(whereSeparator: { $0.isNumber == false })
                .compactMap { Int($0) }
        }

        var isKnown: Bool {
            components.isEmpty == false
        }

        static func < (lhs: ReleaseVersion, rhs: ReleaseVersion) -> Bool {
            let maxCount = max(lhs.components.count, rhs.components.count)
            for index in 0..<maxCount {
                let left = lhs.components.indices.contains(index) ? lhs.components[index] : 0
                let right = rhs.components.indices.contains(index) ? rhs.components[index] : 0
                if left != right {
                    return left < right
                }
            }
            return false
        }
    }

    private let bundle: Bundle
    private let userDefaults: UserDefaults
    private let urlSession: URLSession
    private let openURL: (URL) -> Bool
    private let now: () -> Date
    private let downloadsDirectory: () -> URL?
    private let releaseAssetDownloader: ReleaseAssetDownloader
    private(set) var toastState: ToastState?
    private(set) var availableRelease: AvailableRelease?
    private(set) var downloadState: DownloadState = .idle
    private(set) var updatePromptSequence: Int = 0
    private(set) var installGuideSequence: Int = 0
    private(set) var isChecking = false

    private let githubLatestReleaseURL = URL(string: "https://api.github.com/repos/gongahkia/hot-cross-buns/releases/latest")!
    private let githubReleasesPageURL = URL(string: "https://github.com/gongahkia/hot-cross-buns/releases")!

    init(
        bundle: Bundle = .main,
        userDefaults: UserDefaults = .standard,
        urlSession: URLSession = .shared,
        openURL: @escaping (URL) -> Bool = { NSWorkspace.shared.open($0) },
        now: @escaping () -> Date = Date.init,
        downloadsDirectory: @escaping () -> URL? = {
            FileManager.default.urls(for: .downloadsDirectory, in: .userDomainMask).first
        },
        releaseAssetDownloader: @escaping ReleaseAssetDownloader = { remoteURL, destinationURL, progress in
            try await UpdaterController.defaultReleaseAssetDownloader(
                from: remoteURL,
                to: destinationURL,
                progress: progress
            )
        }
    ) {
        self.bundle = bundle
        self.userDefaults = userDefaults
        self.urlSession = urlSession
        self.openURL = openURL
        self.now = now
        self.downloadsDirectory = downloadsDirectory
        self.releaseAssetDownloader = releaseAssetDownloader
    }

    var updateSourceLabel: String {
        "GitHub Releases"
    }

    var automaticCheckLabel: String {
        "Check GitHub releases automatically"
    }

    var automaticallyChecksForUpdates: Bool {
        get {
            if userDefaults.object(forKey: DefaultsKey.autoCheckEnabled) == nil {
                return true
            }
            return userDefaults.bool(forKey: DefaultsKey.autoCheckEnabled)
        }
        set {
            userDefaults.set(newValue, forKey: DefaultsKey.autoCheckEnabled)
        }
    }

    var lastUpdateCheckDate: Date? {
        return userDefaults.object(forKey: DefaultsKey.lastCheckAt) as? Date
    }

    func checkForUpdates(toastTarget: ToastTarget = .main) {
        Task {
            await checkForUpdatesNow(trigger: .manual, toastTarget: toastTarget)
        }
    }

    func checkForUpdatesNow(trigger: CheckTrigger, toastTarget: ToastTarget = .main) async {
        guard isChecking == false else { return }
        isChecking = true
        defer { isChecking = false }

        do {
            let request = githubLatestReleaseRequest()
            let (data, response) = try await urlSession.data(for: request)
            try validateGitHubResponse(response, data: data)
            let release = try decodeGitHubRelease(from: data)

            let checkedAt = now()
            userDefaults.set(checkedAt, forKey: DefaultsKey.lastCheckAt)

            if isNewerThanCurrentVersion(release.tagName) {
                availableRelease = makeAvailableRelease(from: release)
                if availableRelease?.downloadURL != nil {
                    await downloadAvailableRelease(shouldPresentPrompt: true, toastTarget: toastTarget)
                } else {
                    downloadState = .idle
                    requestAvailableReleasePrompt()
                    if trigger == .manual || trigger == .automatic {
                        toastState = ToastState(
                            title: "Update available",
                            message: "Hot Cross Buns \(availableRelease?.version ?? release.tagName) is available from GitHub Releases.",
                            isWarning: false,
                            target: toastTarget
                        )
                    }
                }
            } else {
                availableRelease = nil
                downloadState = .idle
                if trigger == .manual {
                    toastState = ToastState(
                        title: "You're on the latest version",
                        message: "Hot Cross Buns didn't find anything newer on GitHub Releases.",
                        isWarning: false,
                        target: toastTarget
                    )
                }
            }
        } catch {
            if trigger == .manual {
                toastState = ToastState(
                    title: "Couldn't reach GitHub Releases",
                    message: error.localizedDescription,
                    isWarning: true,
                    target: toastTarget
                )
            }
        }
    }

    func performAutomaticCheckIfNeeded() async {
        guard automaticallyChecksForUpdates else { return }
        if let lastUpdateCheckDate,
           now().timeIntervalSince(lastUpdateCheckDate) < 60 * 60 * 24 {
            return
        }
        await checkForUpdatesNow(trigger: .automatic)
    }

    func openAvailableReleaseDownload() {
        let target = activeDownloadURL ?? availableRelease?.downloadURL ?? availableRelease?.htmlURL ?? githubReleasesPageURL
        guard openURL(target) else {
            toastState = ToastState(
                title: "Couldn't open update",
                message: target.absoluteString,
                isWarning: true
            )
            return
        }
        if shouldPresentInstallGuide(for: target) {
            requestInstallGuidePrompt()
        }
    }

    func revealDownloadedReleaseInFinder() {
        guard let fileURL = activeDownloadURL else { return }
        NSWorkspace.shared.activateFileViewerSelecting([fileURL])
    }

    func revealCurrentAppInFinder() {
        NSWorkspace.shared.activateFileViewerSelecting([bundle.bundleURL])
    }

    func quitForUpdateInstall() {
        NSApp.terminate(nil)
    }

    func retryAvailableReleaseDownload() {
        Task {
            await downloadAvailableRelease(shouldPresentPrompt: true)
        }
    }

    func presentAvailableReleasePrompt() {
        guard availableRelease != nil else { return }
        requestAvailableReleasePrompt()
    }

    func openReleasesPage() {
        guard openURL(githubReleasesPageURL) else {
            toastState = ToastState(
                title: "Couldn't open release page",
                message: githubReleasesPageURL.absoluteString,
                isWarning: true
            )
            return
        }
    }

    func clearToast() {
        toastState = nil
    }

    private func githubLatestReleaseRequest() -> URLRequest {
        var request = URLRequest(url: githubLatestReleaseURL)
        request.setValue("application/vnd.github+json", forHTTPHeaderField: "Accept")
        request.setValue("HotCrossBunsMac/\(currentVersionString)", forHTTPHeaderField: "User-Agent")
        request.timeoutInterval = 15
        return request
    }

    private func validateGitHubResponse(_ response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else {
            throw UpdateCheckError.invalidResponse
        }
        guard (200..<300).contains(http.statusCode) else {
            let message = String(data: data, encoding: .utf8)?
                .trimmingCharacters(in: .whitespacesAndNewlines)
            throw UpdateCheckError.httpStatus(code: http.statusCode, message: message)
        }
    }

    private func decodeGitHubRelease(from data: Data) throws -> GitHubRelease {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return try decoder.decode(GitHubRelease.self, from: data)
    }

    private func makeAvailableRelease(from release: GitHubRelease) -> AvailableRelease {
        let asset = preferredDMGAsset(in: release.assets)
        let download = asset?.browserDownloadURL
        return AvailableRelease(
            title: release.name,
            version: normalizedVersionString(release.tagName),
            tagName: release.tagName,
            htmlURL: release.htmlURL,
            downloadURL: download,
            downloadFilename: asset?.name,
            publishedAt: release.publishedAt,
            notesMarkdown: release.body?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        )
    }

    private func requestAvailableReleasePrompt() {
        updatePromptSequence += 1
    }

    private func requestInstallGuidePrompt() {
        installGuideSequence += 1
    }

    private var activeDownloadURL: URL? {
        guard let release = availableRelease,
              downloadState.phase == .ready,
              downloadState.releaseTag == release.tagName,
              let fileURL = downloadState.fileURL,
              FileManager.default.fileExists(atPath: fileURL.path) else {
            return nil
        }
        return fileURL
    }

    private func downloadAvailableRelease(shouldPresentPrompt: Bool, toastTarget: ToastTarget = .main) async {
        guard let release = availableRelease,
              let remoteURL = release.downloadURL else {
            if shouldPresentPrompt {
                requestAvailableReleasePrompt()
            }
            return
        }

        if let existingFileURL = existingDownloadURL(for: release) {
            downloadState = DownloadState(
                phase: .ready,
                releaseTag: release.tagName,
                progress: 1,
                fileURL: existingFileURL,
                message: nil
            )
            toastState = ToastState(
                title: "Update ready to install",
                message: "Hot Cross Buns \(release.version) is already downloaded in Downloads.",
                isWarning: false,
                target: toastTarget
            )
            if shouldPresentPrompt {
                requestAvailableReleasePrompt()
            }
            return
        }

        let destinationURL: URL
        do {
            destinationURL = try downloadDestination(for: release, remoteURL: remoteURL)
        } catch {
            downloadState = DownloadState(
                phase: .failed,
                releaseTag: release.tagName,
                progress: nil,
                fileURL: nil,
                message: error.localizedDescription
            )
            toastState = ToastState(
                title: "Couldn't prepare update download",
                message: error.localizedDescription,
                isWarning: true,
                target: toastTarget
            )
            if shouldPresentPrompt {
                requestAvailableReleasePrompt()
            }
            return
        }

        downloadState = DownloadState(
            phase: .downloading,
            releaseTag: release.tagName,
            progress: nil,
            fileURL: nil,
            message: nil
        )

        do {
            try await releaseAssetDownloader(remoteURL, destinationURL) { [weak self] progress in
                Task { @MainActor in
                    guard let self,
                          self.availableRelease?.tagName == release.tagName,
                          self.downloadState.phase == .downloading else {
                        return
                    }
                    self.downloadState = DownloadState(
                        phase: .downloading,
                        releaseTag: release.tagName,
                        progress: progress,
                        fileURL: nil,
                        message: nil
                    )
                }
            }
            downloadState = DownloadState(
                phase: .ready,
                releaseTag: release.tagName,
                progress: 1,
                fileURL: destinationURL,
                message: nil
            )
            toastState = ToastState(
                title: "Update ready to install",
                message: "Hot Cross Buns \(release.version) downloaded to Downloads.",
                isWarning: false,
                target: toastTarget
            )
        } catch {
            downloadState = DownloadState(
                phase: .failed,
                releaseTag: release.tagName,
                progress: nil,
                fileURL: nil,
                message: error.localizedDescription
            )
            toastState = ToastState(
                title: "Couldn't download update",
                message: error.localizedDescription,
                isWarning: true,
                target: toastTarget
            )
        }

        if shouldPresentPrompt {
            requestAvailableReleasePrompt()
        }
    }

    private func existingDownloadURL(for release: AvailableRelease) -> URL? {
        guard let destinationURL = try? downloadDestination(for: release, remoteURL: release.downloadURL),
              FileManager.default.fileExists(atPath: destinationURL.path) else {
            return nil
        }
        return destinationURL
    }

    private func downloadDestination(for release: AvailableRelease, remoteURL: URL?) throws -> URL {
        guard let downloadsURL = downloadsDirectory() else {
            throw UpdateDownloadError.missingDownloadsDirectory
        }
        let baseName = sanitizedFilename(
            release.downloadFilename
                ?? remoteURL?.lastPathComponent
                ?? "HotCrossBuns-\(release.version).dmg"
        )
        let filename: String
        if baseName.lowercased().hasSuffix(".dmg") {
            filename = baseName
        } else {
            filename = "\(baseName).dmg"
        }
        return downloadsURL.appendingPathComponent(filename, isDirectory: false)
    }

    private func sanitizedFilename(_ rawValue: String) -> String {
        let fallback = "HotCrossBuns-\(currentVersionString).dmg"
        let trimmed = rawValue.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return fallback }
        return trimmed.replacingOccurrences(
            of: #"[/\\:\n\r\t]+"#,
            with: "-",
            options: .regularExpression
        )
    }

    private func shouldPresentInstallGuide(for target: URL) -> Bool {
        if target.isFileURL {
            return target.pathExtension.caseInsensitiveCompare("dmg") == .orderedSame
        }
        return false
    }

    private func preferredDMGAsset(in assets: [GitHubAsset]) -> GitHubAsset? {
        if let exact = assets.first(where: { $0.name == "HotCrossBuns-macOS.dmg" }) {
            return exact
        }
        return assets.first(where: { $0.name.hasSuffix(".dmg") })
    }

    private var currentVersionString: String {
        (bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String) ?? "0"
    }

    private func normalizedVersionString(_ rawValue: String) -> String {
        rawValue.replacingOccurrences(of: #"^[^0-9]+"#, with: "", options: .regularExpression)
    }

    private func isNewerThanCurrentVersion(_ releaseTag: String) -> Bool {
        let current = ReleaseVersion(currentVersionString)
        let remote = ReleaseVersion(normalizedVersionString(releaseTag))
        if current.isKnown && remote.isKnown {
            return current < remote
        }
        return normalizedVersionString(releaseTag) != currentVersionString
    }

    nonisolated private static func defaultReleaseAssetDownloader(
        from remoteURL: URL,
        to destinationURL: URL,
        progress: @escaping @Sendable (Double?) -> Void
    ) async throws {
        try await ReleaseAssetDownloadCoordinator.download(
            from: remoteURL,
            to: destinationURL,
            progress: progress
        )
    }
}

private enum UpdateCheckError: LocalizedError {
    case invalidResponse
    case httpStatus(code: Int, message: String?)

    var errorDescription: String? {
        switch self {
        case .invalidResponse:
            return "GitHub Releases returned an invalid response."
        case .httpStatus(let code, let message):
            if let message, message.isEmpty == false {
                return "GitHub Releases responded with \(code): \(message)"
            }
            return "GitHub Releases responded with HTTP \(code)."
        }
    }
}

private enum UpdateDownloadError: LocalizedError {
    case missingDownloadsDirectory

    var errorDescription: String? {
        switch self {
        case .missingDownloadsDirectory:
            return "Couldn't resolve the Downloads folder for this account."
        }
    }
}

private final class ReleaseAssetDownloadCoordinator: NSObject, URLSessionDownloadDelegate, URLSessionTaskDelegate {
    private let progressHandler: @Sendable (Double?) -> Void
    private var continuation: CheckedContinuation<Void, Error>?
    private var destinationURL: URL?
    private var completionHandled = false

    private init(progressHandler: @escaping @Sendable (Double?) -> Void) {
        self.progressHandler = progressHandler
    }

    static func download(
        from remoteURL: URL,
        to destinationURL: URL,
        progress: @escaping @Sendable (Double?) -> Void
    ) async throws {
        let coordinator = ReleaseAssetDownloadCoordinator(progressHandler: progress)
        try await coordinator.download(from: remoteURL, to: destinationURL)
    }

    private func download(from remoteURL: URL, to destinationURL: URL) async throws {
        self.destinationURL = destinationURL
        try FileManager.default.createDirectory(
            at: destinationURL.deletingLastPathComponent(),
            withIntermediateDirectories: true,
            attributes: nil
        )
        if FileManager.default.fileExists(atPath: destinationURL.path) {
            try FileManager.default.removeItem(at: destinationURL)
        }

        let configuration = URLSessionConfiguration.ephemeral
        let session = URLSession(configuration: configuration, delegate: self, delegateQueue: nil)
        defer {
            session.finishTasksAndInvalidate()
        }

        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            self.continuation = continuation
            let task = session.downloadTask(with: remoteURL)
            task.resume()
        }
    }

    func urlSession(
        _ session: URLSession,
        downloadTask: URLSessionDownloadTask,
        didWriteData bytesWritten: Int64,
        totalBytesWritten: Int64,
        totalBytesExpectedToWrite: Int64
    ) {
        let fraction: Double?
        if totalBytesExpectedToWrite > 0 {
            fraction = Double(totalBytesWritten) / Double(totalBytesExpectedToWrite)
        } else {
            fraction = nil
        }
        progressHandler(fraction)
    }

    func urlSession(
        _ session: URLSession,
        downloadTask: URLSessionDownloadTask,
        didFinishDownloadingTo location: URL
    ) {
        guard let destinationURL else {
            finish(.failure(UpdateDownloadError.missingDownloadsDirectory))
            return
        }

        do {
            if FileManager.default.fileExists(atPath: destinationURL.path) {
                try FileManager.default.removeItem(at: destinationURL)
            }
            try FileManager.default.moveItem(at: location, to: destinationURL)
            progressHandler(1)
            finish(.success(()))
        } catch {
            finish(.failure(error))
        }
    }

    func urlSession(_ session: URLSession, task: URLSessionTask, didCompleteWithError error: Error?) {
        if let error {
            finish(.failure(error))
        }
    }

    private func finish(_ result: Result<Void, Error>) {
        guard completionHandled == false, let continuation else { return }
        completionHandled = true
        self.continuation = nil
        continuation.resume(with: result)
    }
}
