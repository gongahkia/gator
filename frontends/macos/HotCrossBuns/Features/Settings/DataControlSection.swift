import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct DataControlSection: View {
    @Environment(AppModel.self) private var model
    @State private var cachePath = ""
    @State private var cacheFootprint = "Calculating..."
    @State private var exportMessage: String?
    @State private var exportIsWarning = false
    @State private var portableImportPreview: PortableImportPreview?
    @State private var isShowingAttachmentRepair = false
    @State private var isShowingPortableImportDetails = false
    @State private var exportSelectedTaskListsOnly = false
    @State private var exportSelectedCalendarsOnly = false
    @State private var exportFutureEventsOnly = false

    var body: some View {
        Section("Data control") {
            cloudTargets
            Divider()
            storageLocations
            Divider()
            portableExport
        }
        .task {
            await refreshStorageSummary()
        }
        .confirmationDialog(
            "Import portable archive?",
            isPresented: Binding(
                get: { portableImportPreview != nil },
                set: { isPresented in
                    if isPresented == false {
                        portableImportPreview = nil
                    }
                }
            ),
            titleVisibility: .visible
        ) {
            Button("View import details...") {
                isShowingPortableImportDetails = true
            }
            Button("Replace local data with archive", role: .destructive) {
                importPreviewedArchive()
            }
            Button("Cancel", role: .cancel) {
                portableImportPreview = nil
            }
        } message: {
            if let portableImportPreview {
                Text(importConfirmationMessage(for: portableImportPreview))
            }
        }
        .sheet(isPresented: $isShowingAttachmentRepair) {
            AttachmentRepairSheet()
                .environment(model)
        }
        .sheet(isPresented: $isShowingPortableImportDetails) {
            if let portableImportPreview {
                PortableImportDetailsSheet(
                    preview: portableImportPreview,
                    onCancel: { isShowingPortableImportDetails = false },
                    onImport: { options in
                        isShowingPortableImportDetails = false
                        importPreviewedArchive(options: options)
                    }
                )
            }
        }
    }

    private var cloudTargets: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Cloud sync")
                .hcbFont(.subheadline, weight: .medium)
            ForEach(CloudSyncTarget.allCases) { target in
                Toggle(isOn: binding(for: target)) {
                    Label(target.title, systemImage: target.systemImage)
                }
                Text(target.detail)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
            Text("Turning a surface off keeps existing local data and queued edits on this Mac. Turning it back on resumes replay and refresh for that surface.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var storageLocations: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Local storage")
                .hcbFont(.subheadline, weight: .medium)
            LabeledContent("Cache") {
                Text(cacheFootprint)
                    .foregroundStyle(.secondary)
            }
            if cachePath.isEmpty == false {
                Text(cachePath)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
            HStack {
                Button {
                    Task { await model.openLocalCacheFolder() }
                } label: {
                    Label("Open cache", systemImage: "folder")
                }
                Button {
                    Task { await model.openLocalBackupFolder() }
                } label: {
                    Label("Open backups", systemImage: "archivebox")
                }
            }
        }
    }

    private var portableExport: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Portable export")
                .hcbFont(.subheadline, weight: .medium)
            Text("Export settings, cached tasks, notes, events, sync metadata, and reachable local image/file pointers into a folder package for migration.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
            Button {
                exportPortableArchive()
            } label: {
                Label("Export portable archive...", systemImage: "shippingbox")
            }
            Toggle("Only selected task lists", isOn: $exportSelectedTaskListsOnly)
            Toggle("Only selected calendars", isOn: $exportSelectedCalendarsOnly)
            Toggle("Only future/current events", isOn: $exportFutureEventsOnly)
            Button {
                previewPortableImport()
            } label: {
                Label("Import portable archive...", systemImage: "square.and.arrow.down")
            }
            Button {
                isShowingAttachmentRepair = true
            } label: {
                Label("Review local pointers...", systemImage: "stethoscope")
            }
            if let exportMessage {
                Label(exportMessage, systemImage: exportIsWarning ? "exclamationmark.triangle" : "checkmark.circle")
                    .hcbFont(.caption2)
                    .foregroundStyle(exportIsWarning ? .red : .secondary)
            }
        }
    }

    private func binding(for target: CloudSyncTarget) -> Binding<Bool> {
        Binding(
            get: { model.settings.cloudSyncTargets.contains(target) },
            set: { model.setCloudSyncTarget(target, enabled: $0) }
        )
    }

    private func refreshStorageSummary() async {
        cachePath = await model.cacheFilePath()
        cacheFootprint = await model.cacheFootprintDescription()
        await model.refreshLocalBackupSummary()
    }

    private func exportPortableArchive() {
        let panel = NSSavePanel()
        panel.title = "Export Portable Hot Cross Buns Archive"
        panel.nameFieldStringValue = "HotCrossBuns-Portable-\(Self.dateStamp()).hcbexport"
        panel.allowedContentTypes = [UTType(filenameExtension: "hcbexport") ?? .folder]
        panel.canCreateDirectories = true

        guard panel.runModal() == .OK, let url = panel.url else { return }

        do {
            let summary = try model.exportPortableArchive(to: url, options: portableExportOptions())
            exportIsWarning = summary.skippedAttachmentCount > 0
            exportMessage = "Exported \(summary.copiedAttachmentCount) attachment pointer\(summary.copiedAttachmentCount == 1 ? "" : "s") to \(summary.directoryURL.lastPathComponent)."
            if summary.skippedAttachmentCount > 0 {
                exportMessage?.append(" Skipped \(summary.skippedAttachmentCount) missing, unreadable, or corrupted pointer\(summary.skippedAttachmentCount == 1 ? "" : "s").")
            }
        } catch {
            exportIsWarning = true
            exportMessage = error.localizedDescription
        }
    }

    private func previewPortableImport() {
        let panel = NSOpenPanel()
        panel.title = "Import Portable Hot Cross Buns Archive"
        panel.canChooseFiles = false
        panel.canChooseDirectories = true
        panel.allowsMultipleSelection = false

        guard panel.runModal() == .OK, let url = panel.url else { return }

        do {
            portableImportPreview = try model.previewPortableImport(from: url)
        } catch {
            exportIsWarning = true
            exportMessage = error.localizedDescription
        }
    }

    private func importPreviewedArchive(options: PortableImportOptions = .all) {
        guard let preview = portableImportPreview else { return }
        portableImportPreview = nil

        Task {
            do {
                let summary = try await model.importPortableArchive(from: preview.archiveURL, options: options)
                exportIsWarning = summary.missingBundledAttachmentCount > 0 || summary.corruptBundledAttachmentCount > 0 || summary.skippedPointerCount > 0
                exportMessage = "Imported \(importSummaryScope(options)) and relinked \(summary.importedAttachmentCount) bundled attachment\(summary.importedAttachmentCount == 1 ? "" : "s")."
                if let backupURL = summary.preImportBackupURL {
                    exportMessage?.append(" Pre-import backup: \(backupURL.lastPathComponent).")
                }
                if summary.missingBundledAttachmentCount > 0 {
                    exportMessage?.append(" \(summary.missingBundledAttachmentCount) bundled attachment\(summary.missingBundledAttachmentCount == 1 ? " was" : "s were") missing.")
                }
                if summary.corruptBundledAttachmentCount > 0 {
                    exportMessage?.append(" \(summary.corruptBundledAttachmentCount) bundled attachment\(summary.corruptBundledAttachmentCount == 1 ? " failed" : "s failed") integrity checks.")
                }
                if summary.skippedPointerCount > 0 {
                    exportMessage?.append(" \(summary.skippedPointerCount) original pointer\(summary.skippedPointerCount == 1 ? "" : "s") could not be bundled by the exporting Mac.")
                }
            } catch {
                exportIsWarning = true
                exportMessage = "Import did not run: \(error.localizedDescription)"
            }
        }
    }

    private func importConfirmationMessage(for preview: PortableImportPreview) -> String {
        var message = "This will replace local cached Hot Cross Buns data with \(preview.taskCount) task\(preview.taskCount == 1 ? "" : "s"), \(preview.eventCount) event\(preview.eventCount == 1 ? "" : "s"), \(preview.calendarCount) calendar\(preview.calendarCount == 1 ? "" : "s"), and \(preview.taskListCount) task list\(preview.taskListCount == 1 ? "" : "s")."
        if let diff = preview.diff {
            message.append(" Dry run: \(diffLine("tasks", diff.tasks)); \(diffLine("events", diff.events)); \(diffLine("calendars", diff.calendars)); \(diffLine("task lists", diff.taskLists)).")
            if diff.settingsWillChange {
                message.append(" Settings will be replaced, with this Mac's encryption toggles preserved.")
            }
            if diff.pendingMutationCount > 0 {
                message.append(" The archive contains \(diff.pendingMutationCount) queued mutation\(diff.pendingMutationCount == 1 ? "" : "s").")
            }
            if diff.hasChanges == false {
                message.append(" No task, event, calendar, task-list, settings, or queued-mutation differences were detected.")
            }
        }
        message.append(" \(preview.bundledAttachmentCount) bundled attachment\(preview.bundledAttachmentCount == 1 ? "" : "s") will be copied into this Mac's attachment folder and relinked.")
        if preview.missingBundledAttachmentCount > 0 {
            message.append(" \(preview.missingBundledAttachmentCount) bundled attachment\(preview.missingBundledAttachmentCount == 1 ? " is" : "s are") missing from the archive.")
        }
        if preview.corruptBundledAttachmentCount > 0 {
            message.append(" \(preview.corruptBundledAttachmentCount) bundled attachment\(preview.corruptBundledAttachmentCount == 1 ? " fails" : "s fail") integrity checks and will not be relinked.")
        }
        if preview.skippedPointerCount > 0 {
            message.append(" \(preview.skippedPointerCount) pointer\(preview.skippedPointerCount == 1 ? "" : "s") were already skipped during export because the original file was missing, unreadable, or corrupted.")
        }
        return message
    }

    private func diffLine(_ label: String, _ diff: PortableImportResourceDiff) -> String {
        "\(label) +\(diff.added) -\(diff.removed) changed \(diff.changed)"
    }

    private func portableExportOptions() -> PortableExportOptions {
        PortableExportOptions(
            taskListIDs: exportSelectedTaskListsOnly && model.settings.hasConfiguredTaskListSelection
                ? model.settings.selectedTaskListIDs
                : nil,
            calendarIDs: exportSelectedCalendarsOnly && model.settings.hasConfiguredCalendarSelection
                ? model.settings.selectedCalendarIDs
                : nil,
            eventsStartingAtOrAfter: exportFutureEventsOnly ? Date() : nil
        )
    }

    private func importSummaryScope(_ options: PortableImportOptions) -> String {
        var parts: [String] = []
        if options.includeTasks { parts.append("tasks") }
        if options.includeEvents { parts.append("events") }
        if options.includeSettings { parts.append("settings") }
        if options.includeSyncMetadata { parts.append("sync metadata") }
        return parts.isEmpty ? "no data sections" : parts.joined(separator: ", ")
    }

    private static func dateStamp() -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.dateFormat = "yyyyMMdd-HHmmss"
        return formatter.string(from: Date())
    }
}

private struct PortableImportDetailsSheet: View {
    let preview: PortableImportPreview
    let onCancel: () -> Void
    let onImport: (PortableImportOptions) -> Void
    @State private var includeTasks = true
    @State private var includeEvents = true
    @State private var includeSettings = true
    @State private var includeSyncMetadata = true

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Portable import details")
                        .hcbFont(.title3, weight: .semibold)
                    Text("Read-only dry run for \(preview.archiveURL.lastPathComponent)")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                }
                Spacer()
                Button("Cancel", action: onCancel)
                    .keyboardShortcut(.cancelAction)
                Button(role: .destructive) {
                    onImport(importOptions)
                } label: {
                    Label("Replace local data", systemImage: "square.and.arrow.down")
                }
                .keyboardShortcut(.defaultAction)
            }

            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    importSectionFilters
                    importAttachmentSummary

                    if let diff = preview.diff {
                        PortableImportResourceDiffSection(title: "Tasks", diff: diff.tasks)
                        PortableImportResourceDiffSection(title: "Events", diff: diff.events)
                        PortableImportResourceDiffSection(title: "Calendars", diff: diff.calendars)
                        PortableImportResourceDiffSection(title: "Task lists", diff: diff.taskLists)
                        if diff.settingsWillChange || diff.pendingMutationCount > 0 {
                            VStack(alignment: .leading, spacing: 6) {
                                Text("Other state")
                                    .hcbFont(.subheadline, weight: .medium)
                                if diff.settingsWillChange {
                                    Label("Settings will be replaced. This Mac's encryption toggles are preserved.", systemImage: "gearshape")
                                }
                                if diff.pendingMutationCount > 0 {
                                    Label("\(diff.pendingMutationCount) queued mutation\(diff.pendingMutationCount == 1 ? "" : "s") will be imported.", systemImage: "arrow.triangle.2.circlepath")
                                }
                            }
                            .hcbScaledPadding(10)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
                        }
                    } else {
                        Text("No current-state comparison was available for this preview.")
                            .foregroundStyle(.secondary)
                    }
                }
                .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .hcbScaledPadding(20)
        .frame(minWidth: 720, minHeight: 520)
    }

    private var importOptions: PortableImportOptions {
        PortableImportOptions(
            includeTasks: includeTasks,
            includeEvents: includeEvents,
            includeSettings: includeSettings,
            includeSyncMetadata: includeSyncMetadata
        )
    }

    private var importSectionFilters: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Import sections")
                .hcbFont(.subheadline, weight: .medium)
            Toggle("Tasks and task lists", isOn: $includeTasks)
            Toggle("Events and calendars", isOn: $includeEvents)
            Toggle("Settings", isOn: $includeSettings)
            Toggle("Sync metadata and queued mutations", isOn: $includeSyncMetadata)
            Text("Unchecked sections are preserved from this Mac. A pre-import backup is written before any selected section is replaced.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
        }
        .hcbScaledPadding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
    }

    private var importAttachmentSummary: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Attachments")
                .hcbFont(.subheadline, weight: .medium)
            Label("\(preview.bundledAttachmentCount) bundled attachment\(preview.bundledAttachmentCount == 1 ? "" : "s") will be copied and relinked when reachable.", systemImage: "paperclip")
            if preview.missingBundledAttachmentCount > 0 {
                Label("\(preview.missingBundledAttachmentCount) bundled attachment\(preview.missingBundledAttachmentCount == 1 ? " is" : "s are") missing.", systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.red)
            }
            if preview.corruptBundledAttachmentCount > 0 {
                Label("\(preview.corruptBundledAttachmentCount) bundled attachment\(preview.corruptBundledAttachmentCount == 1 ? " fails" : "s fail") integrity checks.", systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.red)
            }
            if preview.skippedPointerCount > 0 {
                Label("\(preview.skippedPointerCount) pointer\(preview.skippedPointerCount == 1 ? "" : "s") were skipped by the exporting Mac.", systemImage: "minus.circle")
                    .foregroundStyle(.secondary)
            }
        }
        .hcbScaledPadding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
    }
}

private struct PortableImportResourceDiffSection: View {
    let title: String
    let diff: PortableImportResourceDiff

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(title)
                    .hcbFont(.subheadline, weight: .medium)
                Spacer()
                Text("+\(diff.added) -\(diff.removed) changed \(diff.changed)")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }

            if diff.hasChanges {
                PortableImportDiffGroup(title: "Added", systemImage: "plus.circle", items: diff.addedItems)
                PortableImportDiffGroup(title: "Removed", systemImage: "minus.circle", items: diff.removedItems)
                PortableImportDiffGroup(title: "Changed", systemImage: "pencil.circle", items: diff.changedItems, showsTitleTransition: true)
            } else {
                Text("No changes.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }
        }
        .hcbScaledPadding(10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
    }
}

private struct PortableImportDiffGroup: View {
    let title: String
    let systemImage: String
    let items: [PortableImportDiffItem]
    var showsTitleTransition = false

    var body: some View {
        if items.isEmpty == false {
            VStack(alignment: .leading, spacing: 4) {
                Label(title, systemImage: systemImage)
                    .hcbFont(.caption, weight: .medium)
                    .foregroundStyle(.secondary)
                ForEach(items.prefix(40)) { item in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(label(for: item))
                            .hcbFont(.caption)
                            .lineLimit(1)
                        Text(item.id)
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                            .textSelection(.enabled)
                            .lineLimit(1)
                    }
                }
                if items.count > 40 {
                    Text("\(items.count - 40) more not shown.")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
            }
        }
    }

    private func label(for item: PortableImportDiffItem) -> String {
        if showsTitleTransition,
           let current = item.currentTitle,
           let incoming = item.incomingTitle,
           current != incoming {
            return "\(current) -> \(incoming)"
        }
        return item.displayTitle
    }
}

private struct AttachmentRepairSheet: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var diagnostics: [LocalAttachmentDiagnostic] = []
    @State private var includeHealthy = false
    @State private var statusMessage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Local pointer repair")
                        .hcbFont(.title3, weight: .semibold)
                    Text(summaryText)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                Button("Done") {
                    dismiss()
                }
                .keyboardShortcut(.cancelAction)
            }

            HStack {
                Toggle("Show healthy pointers", isOn: $includeHealthy)
                Spacer()
                Button {
                    refreshDiagnostics()
                } label: {
                    Label("Refresh", systemImage: "arrow.clockwise")
                }
            }

            if diagnostics.isEmpty {
                ContentUnavailableView(
                    includeHealthy ? "No local pointers found" : "No broken pointers found",
                    systemImage: includeHealthy ? "paperclip" : "checkmark.circle",
                    description: Text(includeHealthy ? "Add an image or file pointer to a task, note, or event to inspect it here." : "Missing, unreadable, and corrupted image pointers will appear here.")
                )
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                List(diagnostics) { diagnostic in
                    AttachmentDiagnosticRow(diagnostic: diagnostic) {
                        relink(diagnostic)
                    }
                }
                .listStyle(.inset)
            }

            if let statusMessage {
                Text(statusMessage)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
            }
        }
        .hcbScaledPadding(20)
        .frame(minWidth: 620, minHeight: 440)
        .onAppear(perform: refreshDiagnostics)
        .onChange(of: includeHealthy) { _, _ in
            refreshDiagnostics()
        }
    }

    private var summaryText: String {
        let brokenCount = diagnostics.filter { $0.health.isAvailable == false }.count
        let total = diagnostics.count
        if includeHealthy {
            return "\(brokenCount) broken of \(total) local pointer\(total == 1 ? "" : "s")."
        }
        return "\(brokenCount) pointer\(brokenCount == 1 ? "" : "s") need attention."
    }

    private func refreshDiagnostics() {
        diagnostics = model.localAttachmentDiagnostics(includeHealthy: includeHealthy)
        statusMessage = nil
    }

    private func relink(_ diagnostic: LocalAttachmentDiagnostic) {
        let panel = NSOpenPanel()
        panel.title = "Choose Replacement"
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = false
        if diagnostic.attachmentKind == .image {
            panel.allowedContentTypes = [.image]
        }

        guard panel.runModal() == .OK, let replacement = panel.url else { return }

        do {
            let updatedCount = try model.relinkLocalAttachment(
                originalURL: diagnostic.url,
                replacementURL: replacement,
                kind: diagnostic.attachmentKind
            )
            statusMessage = "Relinked \(updatedCount) field\(updatedCount == 1 ? "" : "s") to \(replacement.lastPathComponent)."
            diagnostics = model.localAttachmentDiagnostics(includeHealthy: includeHealthy)
        } catch {
            statusMessage = error.localizedDescription
        }
    }
}

private struct AttachmentDiagnosticRow: View {
    let diagnostic: LocalAttachmentDiagnostic
    let relink: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 12) {
            Image(systemName: diagnostic.attachmentKind.systemImage)
                .foregroundStyle(diagnostic.health.isAvailable ? Color.secondary : Color.red)
                .frame(width: 20)

            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 6) {
                    Text(diagnostic.displayName)
                        .hcbFont(.subheadline, weight: .medium)
                        .lineLimit(1)
                    Text(diagnostic.health.repairLabel)
                        .hcbFont(.caption2)
                        .foregroundStyle(diagnostic.health.isAvailable ? Color.secondary : Color.red)
                }
                Text("\(diagnostic.sourceKind): \(diagnostic.sourceTitle)")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Text(diagnostic.url.path)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                    .lineLimit(2)
            }

            Spacer()

            Button {
                relink()
            } label: {
                Label("Relink...", systemImage: "link")
            }
            .disabled(diagnostic.health.isAvailable)
        }
        .hcbScaledPadding(.vertical, 6)
    }
}
