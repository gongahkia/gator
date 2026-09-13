import SwiftUI

struct UpdatesSection: View {
    @Environment(UpdaterController.self) private var updater
    @Environment(\.openWindow) private var openWindow
    @State private var autoCheck: Bool = true

    var body: some View {
        Section("Updates") {
            Toggle(updater.automaticCheckLabel, isOn: autoCheckBinding)

            Button {
                updater.checkForUpdates(toastTarget: .settings)
            } label: {
                Label(
                    updater.isChecking ? "Checking GitHub Releases…" : "Check for Updates Now",
                    systemImage: "arrow.down.circle"
                )
            }
            .disabled(updater.isChecking)

            if let release = updater.availableRelease {
                LabeledContent("Latest available") {
                    Text("Version \(release.version)")
                }
                if let publishedAt = release.publishedAt {
                    Text("Published \(publishedAt.formatted(date: .abbreviated, time: .shortened))")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                HStack(spacing: 12) {
                    Button("What's New") {
                        updater.presentAvailableReleasePrompt()
                        openWindow(id: "update-available")
                    }
                    Button(primaryButtonLabel(for: release)) {
                        performPrimaryAction(for: release)
                    }
                    .disabled(updater.downloadState.phase == .downloading && updater.downloadState.releaseTag == release.tagName)
                    Button("View Releases Page") {
                        updater.openReleasesPage()
                    }
                }
                if let downloadStatus = downloadStatusText(for: release) {
                    Label(downloadStatus, systemImage: downloadStatusIcon(for: release))
                        .hcbFont(.footnote)
                        .foregroundStyle(downloadStatusIsWarning(for: release) ? .orange : .secondary)
                }
            } else {
                Label(
                    "This checks GitHub Releases, downloads the newest DMG into Downloads, and opens install guidance for a manual replace.",
                    systemImage: "shippingbox"
                )
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }

            if let last = updater.lastUpdateCheckDate {
                Text("Last checked \(last.formatted(date: .abbreviated, time: .shortened)) via \(updater.updateSourceLabel)")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
        .onAppear {
            autoCheck = updater.automaticallyChecksForUpdates
        }
    }

    private var autoCheckBinding: Binding<Bool> {
        Binding(
            get: { autoCheck },
            set: { newValue in
                autoCheck = newValue
                updater.automaticallyChecksForUpdates = newValue
            }
        )
    }

    private func primaryButtonLabel(for release: UpdaterController.AvailableRelease) -> String {
        let state = updater.downloadState
        guard state.releaseTag == release.tagName else {
            return release.downloadURL == nil ? "Open Release Notes" : "Download Latest DMG"
        }
        switch state.phase {
        case .ready:
            return "Open Downloaded DMG"
        case .downloading:
            return "Downloading Latest DMG…"
        case .failed:
            return release.downloadURL == nil ? "Open Release Notes" : "Retry Download"
        case .idle:
            return release.downloadURL == nil ? "Open Release Notes" : "Download Latest DMG"
        }
    }

    private func performPrimaryAction(for release: UpdaterController.AvailableRelease) {
        let state = updater.downloadState
        guard state.releaseTag == release.tagName else {
            if release.downloadURL == nil {
                updater.openAvailableReleaseDownload()
            } else {
                updater.retryAvailableReleaseDownload()
            }
            return
        }

        switch state.phase {
        case .ready:
            updater.openAvailableReleaseDownload()
        case .downloading:
            break
        case .failed:
            if release.downloadURL == nil {
                updater.openAvailableReleaseDownload()
            } else {
                updater.retryAvailableReleaseDownload()
            }
        case .idle:
            if release.downloadURL == nil {
                updater.openAvailableReleaseDownload()
            } else {
                updater.retryAvailableReleaseDownload()
            }
        }
    }

    private func downloadStatusText(for release: UpdaterController.AvailableRelease) -> String? {
        let state = updater.downloadState
        guard state.releaseTag == release.tagName else { return nil }
        switch state.phase {
        case .idle:
            return nil
        case .downloading:
            if let progress = state.progress {
                return "Downloading DMG: \(Int(progress * 100))% complete."
            }
            return "Downloading DMG into Downloads…"
        case .ready:
            if let fileURL = state.fileURL {
                return "Downloaded to \(fileURL.lastPathComponent) in Downloads."
            }
            return "Downloaded to Downloads."
        case .failed:
            return state.message ?? "The update download failed. Use What's New to retry or open the release page."
        }
    }

    private func downloadStatusIcon(for release: UpdaterController.AvailableRelease) -> String {
        let state = updater.downloadState
        guard state.releaseTag == release.tagName else { return "shippingbox" }
        switch state.phase {
        case .idle:
            return "shippingbox"
        case .downloading:
            return "arrow.down.circle"
        case .ready:
            return "externaldrive.badge.checkmark"
        case .failed:
            return "exclamationmark.triangle"
        }
    }

    private func downloadStatusIsWarning(for release: UpdaterController.AvailableRelease) -> Bool {
        updater.downloadState.releaseTag == release.tagName && updater.downloadState.phase == .failed
    }
}

struct UpdateAvailableWindow: View {
    @Environment(UpdaterController.self) private var updater

    var body: some View {
        Group {
            if let release = updater.availableRelease {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 18) {
                        header(release)
                        actions(release)
                        notes(release)
                        installGuidance
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(20)
                }
                .background(Color.clear)
            } else {
                ContentUnavailableView(
                    "No Update Available",
                    systemImage: "checkmark.circle",
                    description: Text("Hot Cross Buns did not find a newer release yet.")
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .padding(24)
            }
        }
    }

    @ViewBuilder
    private func header(_ release: UpdaterController.AvailableRelease) -> some View {
        let resolvedTitle: String = {
            let candidate = release.title?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            return candidate.isEmpty ? "Update Available" : candidate
        }()
        VStack(alignment: .leading, spacing: 8) {
            Text(resolvedTitle)
                .font(.title2.weight(.semibold))
            Text("Hot Cross Buns \(release.version) is available from GitHub Releases.")
                .foregroundStyle(.secondary)
            if let publishedAt = release.publishedAt {
                Text("Published \(publishedAt.formatted(date: .abbreviated, time: .shortened))")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private func actions(_ release: UpdaterController.AvailableRelease) -> some View {
        HStack(spacing: 12) {
            switch downloadPhase(for: release) {
            case .ready:
                Button("Open Downloaded DMG") {
                    updater.openAvailableReleaseDownload()
                }
                .buttonStyle(.borderedProminent)

                Button("Reveal in Finder") {
                    updater.revealDownloadedReleaseInFinder()
                }
                .buttonStyle(.bordered)
            case .downloading:
                Button("Downloading Latest DMG…") {}
                    .buttonStyle(.borderedProminent)
                    .disabled(true)
            case .failed:
                Button("Retry Download") {
                    updater.retryAvailableReleaseDownload()
                }
                .buttonStyle(.borderedProminent)

                Button("Open Release Page") {
                    updater.openAvailableReleaseDownload()
                }
                .buttonStyle(.bordered)
            case .idle:
                Button(release.downloadURL == nil ? "Open Release Page" : "Download Latest DMG") {
                    updater.openAvailableReleaseDownload()
                }
                .buttonStyle(.borderedProminent)
            }

            Button("View All Releases") {
                updater.openReleasesPage()
            }
            .buttonStyle(.bordered)
        }
    }

    @ViewBuilder
    private func notes(_ release: UpdaterController.AvailableRelease) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Release Notes")
                .font(.headline)

            if release.notesMarkdown.isEmpty {
                Text("No release notes were published for this version.")
                    .foregroundStyle(.secondary)
            } else {
                MarkdownBlock(source: release.notesMarkdown, lineSpacing: 6)
                    .textSelection(.enabled)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var installGuidance: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Install Guidance")
                .font(.headline)
            Text("Installing the new DMG replaces the app bundle only. Hot Cross Buns cannot safely trash its own running app from the sandbox, so Finder still asks you to confirm Replace. Your settings, local cache, and history stay on this Mac because they live outside the app in UserDefaults and Application Support.")
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            if let status = detailedDownloadStatus {
                Text(status)
                    .hcbFont(.footnote)
                    .foregroundStyle(updater.downloadState.phase == .failed ? .orange : .secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func downloadPhase(for release: UpdaterController.AvailableRelease) -> UpdaterController.DownloadState.Phase {
        if updater.downloadState.releaseTag == release.tagName {
            return updater.downloadState.phase
        }
        return .idle
    }

    private var detailedDownloadStatus: String? {
        switch updater.downloadState.phase {
        case .idle:
            return nil
        case .downloading:
            if let progress = updater.downloadState.progress {
                return "Download progress: \(Int(progress * 100))%."
            }
            return "Downloading the DMG into Downloads now."
        case .ready:
            if let fileURL = updater.downloadState.fileURL {
                return "The DMG is already downloaded at \(fileURL.path)."
            }
            return "The DMG is ready in Downloads."
        case .failed:
            return updater.downloadState.message ?? "The DMG download failed. You can retry or open the release page directly."
        }
    }
}

struct InstallUpdateWindow: View {
    @Environment(UpdaterController.self) private var updater

    var body: some View {
        Group {
            if let release = updater.availableRelease {
                VStack(alignment: .leading, spacing: 18) {
                    Text("Install Hot Cross Buns \(release.version)")
                        .font(.title2.weight(.semibold))

                    Text("The DMG is open. Finish the update in Finder, then relaunch the app.")
                        .foregroundStyle(.secondary)

                    VStack(alignment: .leading, spacing: 12) {
                        installStep(
                            number: 1,
                            title: "Wait for the disk image to appear",
                            detail: "Finder should mount the downloaded Hot Cross Buns DMG automatically."
                        )
                        installStep(
                            number: 2,
                            title: "Replace the old app",
                            detail: "Drag the new Hot Cross Buns app into Applications. When macOS asks, confirm Replace; this removes the old app bundle and keeps your settings."
                        )
                        installStep(
                            number: 3,
                            title: "Open the updated app",
                            detail: "Launch Hot Cross Buns again from Applications. Your settings, cache, and history stay on this Mac."
                        )
                    }

                    HStack(spacing: 12) {
                        Button("Open Downloaded DMG") {
                            updater.openAvailableReleaseDownload()
                        }
                        .buttonStyle(.borderedProminent)
                        .disabled(updater.downloadState.phase != .ready)

                        Button("Reveal in Finder") {
                            updater.revealDownloadedReleaseInFinder()
                        }
                        .buttonStyle(.bordered)
                        .disabled(updater.downloadState.phase != .ready)

                        Button("Show Current App") {
                            updater.revealCurrentAppInFinder()
                        }
                        .buttonStyle(.bordered)

                        Button("Quit to Finish Update") {
                            updater.quitForUpdateInstall()
                        }
                        .buttonStyle(.bordered)
                    }

                    if let fileURL = updater.downloadState.fileURL {
                        Text("Downloaded file: \(fileURL.path)")
                            .hcbFont(.footnote)
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                    }

                    Spacer(minLength: 0)
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
                .padding(20)
            } else {
                ContentUnavailableView(
                    "No Downloaded Update",
                    systemImage: "shippingbox",
                    description: Text("Download an update first, then the install guide will appear here.")
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)
                .padding(24)
            }
        }
    }

    private func installStep(number: Int, title: String, detail: String) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Text("\(number)")
                .font(.headline)
                .frame(width: 22, height: 22)
                .background(Circle().fill(Color.accentColor.opacity(0.15)))
            VStack(alignment: .leading, spacing: 4) {
                Text(title)
                    .font(.headline)
                Text(detail)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }
}
