import SwiftUI

struct SyncIssuesWindow: View {
    @Environment(AppModel.self) private var model

    private var conflictedMutations: [PendingMutation] {
        model.pendingMutations.filter(\.isConflict)
    }

    private var retryableQuarantined: [PendingMutation] {
        model.pendingMutations.filter {
            $0.isQuarantined
                && $0.isConflict == false
                && $0.hasInvalidPayloadIssue == false
        }
    }

    private var invalidPayloadMutations: [PendingMutation] {
        model.pendingMutations.filter {
            $0.isQuarantined
                && $0.isConflict == false
                && $0.hasInvalidPayloadIssue
        }
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                header

                if let summary = model.lastNotificationScheduleSummary, summary.hasDeferred {
                    reminderCapacityCard(summary)
                }

                if conflictedMutations.isEmpty == false {
                    sectionTitle("Sync conflicts")
                    ForEach(conflictedMutations) { mutation in
                        ConflictMutationCard(
                            mutation: mutation,
                            onKeepMine: {
                                Task { _ = await model.forceOverwriteConflictedMutation(id: mutation.id) }
                            },
                            onKeepServer: { _ = model.clearPendingMutation(id: mutation.id) }
                        )
                    }
                }

                if invalidPayloadMutations.isEmpty == false {
                    sectionTitle("Invalid queued payloads")
                    ForEach(invalidPayloadMutations) { mutation in
                        PendingMutationCard(
                            mutation: mutation,
                            onDrop: { _ = model.clearPendingMutation(id: mutation.id) },
                            onCopyPayload: { mutation.copyPayloadToPasteboard() }
                        )
                    }
                }

                if retryableQuarantined.isEmpty == false {
                    sectionTitle("Retryable queued writes")
                    ForEach(retryableQuarantined) { mutation in
                        PendingMutationCard(
                            mutation: mutation,
                            onDrop: { _ = model.clearPendingMutation(id: mutation.id) },
                            onRetry: { _ = model.requeueQuarantinedMutation(id: mutation.id) },
                            onCopyPayload: { mutation.copyPayloadToPasteboard() }
                        )
                    }
                }

                if conflictedMutations.isEmpty
                    && invalidPayloadMutations.isEmpty
                    && retryableQuarantined.isEmpty
                    && (model.lastNotificationScheduleSummary?.hasDeferred ?? false) == false
                {
                    ContentUnavailableView(
                        "Nothing needs attention",
                        systemImage: "checkmark.shield",
                        description: Text("Deferred reminders, sync conflicts, and quarantined writes will appear here when they need action.")
                    )
                    .frame(maxWidth: .infinity)
                    .hcbScaledPadding(.top, 40)
                }
            }
            .hcbScaledPadding(20)
        }
        .appBackground()
        .navigationTitle("Sync Issues")
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Review sync attention items without changing the underlying replay model.")
                .hcbFont(.headline)
            Text("These controls operate on the existing pending mutation queue and notification scheduler only. They do not change how Hot Cross Buns creates, retries, or reconciles Google writes.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    @ViewBuilder
    private func sectionTitle(_ title: String) -> some View {
        Text(title)
            .hcbFont(.subheadline, weight: .semibold)
            .foregroundStyle(.secondary)
            .hcbScaledPadding(.top, 4)
    }

    @ViewBuilder
    private func reminderCapacityCard(_ summary: NotificationScheduleSummary) -> some View {
        SyncIssueCard {
            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 8) {
                    Image(systemName: "bell.badge")
                        .foregroundStyle(AppColor.ember)
                    Text("Some reminders were deferred on this Mac")
                        .hcbFont(.subheadline, weight: .semibold)
                }

                Text("macOS allows up to 64 pending local notifications per app. Hot Cross Buns scheduled the nearest items first and deferred the rest until the queue frees up.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)

                HStack(spacing: 12) {
                    statPill("Deferred events", value: summary.deferredEvents)
                    statPill("Deferred tasks", value: summary.deferredTasks)
                    statPill("Scheduled now", value: summary.totalScheduled)
                }

                Text("Last computed \(summary.computedAt.formatted(date: .abbreviated, time: .shortened))")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private func statPill(_ title: String, value: Int) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(value.formatted())
                .hcbFont(.body, weight: .semibold)
            Text(title)
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
        .hcbScaledPadding(.vertical, 8)
        .hcbScaledPadding(.horizontal, 10)
        .background(
            RoundedRectangle(cornerRadius: 10)
                .fill(AppColor.cardSurface)
        )
    }
}

private struct SyncIssueCard<Content: View>: View {
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            content
        }
        .hcbScaledPadding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 14)
                .fill(.ultraThinMaterial)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 14)
                .stroke(AppColor.cardStroke, lineWidth: 0.8)
        )
    }
}

private struct ConflictMutationCard: View {
    let mutation: PendingMutation
    let onKeepMine: () -> Void
    let onKeepServer: () -> Void

    var body: some View {
        SyncIssueCard {
            VStack(alignment: .leading, spacing: 8) {
                HStack(spacing: 8) {
                    Image(systemName: "arrow.triangle.branch")
                        .foregroundStyle(.red)
                    Text(title)
                        .hcbFont(.subheadline, weight: .semibold)
                    Spacer(minLength: 0)
                }
                Text(payloadSummary)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(4)
                    .textSelection(.enabled)
                Text("Queued \(mutation.createdAt.formatted(date: .abbreviated, time: .shortened)) · resource \(mutation.resourceID)")
                    .font(.caption2.monospacedDigit())
                    .foregroundStyle(.secondary)
                HStack(spacing: 8) {
                    Button(action: onKeepMine) {
                        Label("Keep my change", systemImage: "arrow.up.forward.circle")
                    }
                    .buttonStyle(.borderedProminent)

                    Button(role: .destructive, action: onKeepServer) {
                        Label("Keep server version", systemImage: "icloud.and.arrow.down")
                    }
                    .buttonStyle(.bordered)
                }
            }
        }
    }

    private var title: String {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .update): "Task edit conflict"
        case (.task, .completion): "Task completion conflict"
        case (.task, .delete): "Task delete conflict"
        case (.event, .update): "Event edit conflict"
        case (.event, .delete): "Event delete conflict"
        default: "\(mutation.resourceType.rawValue) \(mutation.action.rawValue) conflict"
        }
    }

    private var payloadSummary: String {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .update):
            if let payload = try? PendingMutationEncoder.decodeTaskUpdate(mutation.payload) {
                var parts = ["title: \(payload.title)"]
                if payload.notes.isEmpty == false { parts.append("notes: \(payload.notes.prefix(80))") }
                if let due = payload.dueDate { parts.append("due: \(due.formatted(date: .abbreviated, time: .omitted))") }
                return parts.joined(separator: " · ")
            }
        case (.task, .completion):
            if let payload = try? PendingMutationEncoder.decodeTaskCompletion(mutation.payload) {
                return payload.isCompleted ? "mark complete" : "mark needs action"
            }
        case (.task, .delete):
            return "delete task"
        case (.event, .update):
            if let payload = try? PendingMutationEncoder.decodeEventUpdate(mutation.payload) {
                var parts = ["summary: \(payload.summary)"]
                parts.append("start: \(payload.startDate.formatted(date: .abbreviated, time: payload.isAllDay ? .omitted : .shortened))")
                parts.append("end: \(payload.endDate.formatted(date: .abbreviated, time: payload.isAllDay ? .omitted : .shortened))")
                if payload.location.isEmpty == false { parts.append("at \(payload.location)") }
                return parts.joined(separator: " · ")
            }
        case (.event, .delete):
            return "delete event"
        default:
            break
        }
        return "Mutation id \(mutation.id.uuidString)"
    }
}

private struct PendingMutationCard: View {
    let mutation: PendingMutation
    let onDrop: () -> Void
    var onRetry: (() -> Void)? = nil
    var onCopyPayload: (() -> Void)? = nil

    var body: some View {
        SyncIssueCard {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: symbol)
                    .foregroundStyle(tint)
                    .hcbScaledFrame(width: 18, alignment: .center)
                VStack(alignment: .leading, spacing: 2) {
                    Text(title)
                        .hcbFont(.subheadline, weight: .medium)
                    Text(subtitle)
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.middle)
                    if let err = mutation.lastErrorSummary, err.isEmpty == false {
                        Text(err)
                            .hcbFont(.caption2)
                            .foregroundStyle(.red)
                            .lineLimit(2)
                    }
                }
                Spacer(minLength: 8)
                if let onRetry {
                    Button(action: onRetry) {
                        Label("Retry", systemImage: "arrow.clockwise")
                    }
                    .buttonStyle(.bordered)
                }
                if let onCopyPayload {
                    Button(action: onCopyPayload) {
                        Label("Copy payload", systemImage: "doc.on.doc")
                    }
                    .buttonStyle(.bordered)
                }
                Button(role: .destructive, action: onDrop) {
                    Label("Drop", systemImage: "trash")
                }
                .buttonStyle(.bordered)
            }
        }
    }

    private var title: String {
        switch (mutation.resourceType, mutation.action) {
        case (.task, .create): "New task"
        case (.task, .update): "Task edit"
        case (.task, .completion): "Task completion"
        case (.task, .delete): "Task delete"
        case (.event, .create): "New event"
        case (.event, .update): "Event edit"
        case (.event, .delete): "Event delete"
        default: "Pending \(mutation.resourceType.rawValue) \(mutation.action.rawValue)"
        }
    }

    private var subtitle: String {
        let attempts = mutation.attemptCount
        let suffix = attempts > 0 ? " · \(attempts) attempt\(attempts == 1 ? "" : "s")" : ""
        return "\(mutation.resourceID) · queued \(mutation.createdAt.formatted(date: .abbreviated, time: .shortened))\(suffix)"
    }

    private var symbol: String {
        switch mutation.action {
        case .create: "plus.circle"
        case .update: "pencil.circle"
        case .completion: "checkmark.circle"
        case .delete: "trash.circle"
        }
    }

    private var tint: Color {
        switch mutation.action {
        case .create: AppColor.moss
        case .update: AppColor.blue
        case .completion: AppColor.moss
        case .delete: AppColor.ember
        }
    }
}

private extension PendingMutation {
    var hasInvalidPayloadIssue: Bool {
        (lastErrorSummary ?? "").hasPrefix("Invalid payload")
    }

    func copyPayloadToPasteboard() {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(prettyPayloadText, forType: .string)
    }

    var prettyPayloadText: String {
        guard let object = try? JSONSerialization.jsonObject(with: payload),
              JSONSerialization.isValidJSONObject(object),
              let data = try? JSONSerialization.data(withJSONObject: object, options: [.prettyPrinted, .sortedKeys]),
              let string = String(data: data, encoding: .utf8)
        else {
            let raw = String(data: payload, encoding: .utf8) ?? payload.base64EncodedString()
            return raw
        }
        return string
    }
}
