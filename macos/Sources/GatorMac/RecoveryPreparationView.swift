import SwiftUI
import GatorCore

struct RecoveryPreparationView: View {
    @Environment(ManagedSessionCoordinator.self) private var sessions
    let owner: GatorApplicationState
    let recovery: NativeRequestRecovery
    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Reviewed recovery preparation").font(.headline).accessibilityAddTraits(.isHeader)
            Text(verbatim: recovery.summary).textSelection(.enabled)
            Text("Preparing may use the original model and consume usage to propose a revised plan. It does not approve that plan, grant account permissions or repeat external actions.").font(.callout).textSelection(.enabled)
            if !recovery.needs.isEmpty {
                Button("Review Configured Account Access") {
                    Task { await sessions.reviewRecoveryAccess(owner: owner, recovery: recovery, proposed: owner.recoveryAccess[recovery.id]?.recovery == recovery) }
                }.focusable().disabled(sessions.recoveryUnavailable(owner: owner, recovery: recovery, action: .reviewAccess) != nil)
                    .accessibilityLabel("Review configured account access for the original task")
                if let access = owner.recoveryAccess[recovery.id] {
                    ForEach(Array(access.requirements.enumerated()), id: \.offset) { index, requirement in
                        VStack(alignment: .leading, spacing: 6) {
                            Text(verbatim: "Requirement \(index + 1): " + requirement["need"]["operation"].string).font(.body.weight(.medium))
                            Text(verbatim: "Original account: " + requirement["need"]["connector_id"].string).font(.caption).textSelection(.enabled)
                            let selected = access.selections.indices.contains(index) ? access.selections[index] : ""
                            Picker("Configured account for requirement \(index + 1)", selection: Binding(get: { selected }, set: { value in
                                if owner.proposeRecoveryAccount(recovery, index: index, account: value) {
                                    Task { await sessions.reviewRecoveryAccess(owner: owner, recovery: recovery, proposed: true) }
                                }
                            })) {
                                if !requirement["choices"].array.contains(where: { $0["id"].string == selected }) {
                                    Text(selected.isEmpty ? "Choose a configured account" : selected + " (unavailable)").tag(selected).disabled(true)
                                }
                                ForEach(requirement["choices"].array, id: \.self) { choice in
                                    Text(verbatim: choice["label"].string + " (" + choice["id"].string + ")" + (choice["issue"].string.isEmpty ? "" : " · needs repair"))
                                        .tag(choice["id"].string).disabled(!choice["issue"].string.isEmpty)
                                }
                            }.focusable()
                                .onKeyPress(.downArrow) { moveAccount(requirement, selected: selected, index: index, direction: 1); return .handled }
                                .onKeyPress(.upArrow) { moveAccount(requirement, selected: selected, index: index, direction: -1); return .handled }
                                .onKeyPress(.return) { .handled }
                                .help("Use Up or Down to choose a configured account. Wait for Go to review each selection. Return never prepares work.")
                                .disabled(!owner.isCurrentRecovery(recovery) || owner !== sessions.current || access.recovery != recovery)
                                .accessibilityLabel("Configured account for requirement \(index + 1): " + requirement["need"]["operation"].string)
                            ForEach(requirement["choices"].array, id: \.self) { choice in
                                VStack(alignment: .leading, spacing: 3) {
                                    Text(verbatim: (choice["id"].string == selected ? "Selected: " : "Configured choice: ") + choice["label"].string + " · " + choice["id"].string)
                                    Text(verbatim: "Destination: " + choice["destination"].string + "\nOperation: " + choice["operation"].string + "\nPermission: " + choice["permission"].string)
                                    if !choice["issue"].string.isEmpty { Label { Text(verbatim: choice["issue"].string) } icon: { Image(systemName: "exclamationmark.triangle") } }
                                }.font(.caption).textSelection(.enabled)
                            }
                        }
                    }
                    if let review = access.review, owner.isCurrentRecoveryAccess(access) {
                        Label(access.ready ? "Configured access reviewed and ready" : "Access requires repair or another configured choice", systemImage: access.ready ? "checkmark.shield" : "exclamationmark.triangle")
                        Text("Readiness reflects local configuration, not a provider connectivity test.").font(.caption)
                        Text(verbatim: review["summary"].string).textSelection(.enabled)
                    } else { Text("Access review is stale or pending. Fresh Go confirmation is required before preparation.").font(.callout) }
                    if !access.issue.isEmpty { Text(verbatim: access.issue).font(.caption).textSelection(.enabled) }
                }
            }
            let action: NativeRecoveryAction = recovery.needs.isEmpty ? .prepareRetry : .prepareAccess
            let reason = sessions.recoveryUnavailable(owner: owner, recovery: recovery, action: action)
            ExactDecisionButton(title: action == .prepareAccess ? "Prepare Plan With Reviewed Access" : "Prepare Recovery Plan", enabled: reason == nil) {
                let reviewedAccess = owner.recoveryAccess[recovery.id]
                Task { await sessions.prepareRecovery(owner: owner, recovery: recovery, action: action, reviewedAccess: reviewedAccess) }
            }
            if let reason { NativeStatusMessage(text: reason, severity: .information).font(.caption) }
            Text("For account repair, open /connections in Gator’s terminal app; inspect permissions with /permissions. Provider setup uses /models. Recovery does not run these commands or change those settings.").font(.caption).foregroundStyle(.secondary)
        }.accessibilityIdentifier("gator.recovery.prepare." + recovery.id)
    }
    private func moveAccount(_ requirement: JSONValue, selected: String, index: Int, direction: Int) {
        guard owner.operation.isEmpty else { return }
        let choices = requirement["choices"].array.filter { $0["issue"].string.isEmpty }
        guard !choices.isEmpty else { return }
        let current = choices.firstIndex { $0["id"].string == selected }
        let next = current.map { min(max($0 + direction, 0), choices.count - 1) } ?? (direction > 0 ? 0 : choices.count - 1)
        let account = choices[next]["id"].string
        guard account != selected, owner.proposeRecoveryAccount(recovery, index: index, account: account) else { return }
        Task { await sessions.reviewRecoveryAccess(owner: owner, recovery: recovery, proposed: true) }
    }
}
