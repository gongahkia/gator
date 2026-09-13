import SwiftUI

struct LocalBackupSection: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Section("Local backups") {
            Toggle("Daily local backup", isOn: dailyBackupBinding)
            Stepper(value: retentionBinding, in: 1...90) {
                HStack {
                    Text("Keep backups")
                    Spacer()
                    Text("\(model.settings.dailyLocalBackupRetentionCount)")
                        .foregroundStyle(.secondary)
                        .monospacedDigit()
                }
            }
            .disabled(model.settings.dailyLocalBackupEnabled == false)

            HStack {
                Button {
                    Task { await model.runDailyLocalBackupNow() }
                } label: {
                    Label("Back up now", systemImage: "externaldrive.badge.plus")
                }
                Button {
                    Task { await model.openLocalBackupFolder() }
                } label: {
                    Label("Open in Finder", systemImage: "folder")
                }
            }

            if let summary = model.localBackupSummary {
                Text(summaryText(summary))
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            } else {
                Text(model.settings.cacheEncryptionEnabled
                     ? "Backups are encrypted with your local cache key and stored in Application Support on this Mac."
                     : "Backups are stored in Application Support on this Mac. Open the folder to remove old backups manually.")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
        .task {
            await model.refreshLocalBackupSummary()
        }
    }

    private var dailyBackupBinding: Binding<Bool> {
        Binding(
            get: { model.settings.dailyLocalBackupEnabled },
            set: { model.setDailyLocalBackupEnabled($0) }
        )
    }

    private var retentionBinding: Binding<Int> {
        Binding(
            get: { model.settings.dailyLocalBackupRetentionCount },
            set: { model.setDailyLocalBackupRetentionCount($0) }
        )
    }

    private func summaryText(_ summary: LocalBackupService.BackupSummary) -> String {
        var parts: [String] = []
        if summary.backupCount > 0 {
            parts.append("\(summary.backupCount) backup\(summary.backupCount == 1 ? "" : "s")")
            parts.append(ByteCountFormatter.string(fromByteCount: summary.totalBytes, countStyle: .file))
        } else {
            parts.append("No backups yet")
        }
        if let last = model.settings.lastDailyLocalBackupAt {
            parts.append("Last run \(last.formatted(date: .abbreviated, time: .shortened))")
        }
        if summary.directoryPath.isEmpty == false {
            parts.append(summary.directoryPath)
        }
        if model.settings.cacheEncryptionEnabled {
            parts.append("Encrypted")
        }
        return parts.joined(separator: " • ")
    }
}
