import Foundation

struct DisconnectImpactSummary: Equatable {
    var accountName: String
    var cacheFootprint: String
    var pendingMutationCount: Int
    var conflictedMutationCount: Int
    var quarantinedMutationCount: Int
    var invalidPayloadMutationCount: Int

    var confirmationMessage: String {
        let pendingNoun = pendingMutationCount == 1 ? "queued local write" : "queued local writes"
        var lines = [
            "This signs out \(accountName) on this Mac and removes the saved Google session from Keychain.",
            "Google Tasks and Calendar data in your Google account will not be deleted.",
            "Local cache retained on this Mac: \(cacheFootprint).",
            "Local Spotlight search results will be removed until you reconnect and sync again.",
            "Pending sync work: \(pendingMutationCount) \(pendingNoun)."
        ]

        let flagged = flaggedMutationSummary
        if flagged.isEmpty == false {
            lines.append("Needs attention: \(flagged).")
        }

        if pendingMutationCount > 0 {
            lines.append("Queued writes stay stored locally, but they cannot reach Google while disconnected.")
        }

        return lines.joined(separator: "\n\n")
    }

    private var flaggedMutationSummary: String {
        var parts: [String] = []
        if conflictedMutationCount > 0 {
            parts.append("\(conflictedMutationCount) conflict\(conflictedMutationCount == 1 ? "" : "s")")
        }
        if quarantinedMutationCount > 0 {
            parts.append("\(quarantinedMutationCount) quarantined")
        }
        if invalidPayloadMutationCount > 0 {
            parts.append("\(invalidPayloadMutationCount) invalid")
        }
        return parts.joined(separator: ", ")
    }
}

struct AccountDisconnectImpactResolver: Equatable {
    var activeAccountID: GoogleAccount.ID?
    var activePendingMutations: [PendingMutation]
    var accountWorkspaces: [AccountWorkspaceSnapshot]

    func pendingMutations(for accountID: GoogleAccount.ID?) -> [PendingMutation] {
        guard let accountID else { return activePendingMutations }
        if accountID == activeAccountID {
            return activePendingMutations
        }
        return accountWorkspaces.first { $0.accountID == accountID }?.pendingMutations ?? []
    }

    func summary(
        for accountID: GoogleAccount.ID?,
        accountName: String,
        cacheFootprint: String
    ) -> DisconnectImpactSummary {
        let mutations = pendingMutations(for: accountID)
        return DisconnectImpactSummary(
            accountName: accountName,
            cacheFootprint: cacheFootprint,
            pendingMutationCount: mutations.count,
            conflictedMutationCount: mutations.filter(\.isConflict).count,
            quarantinedMutationCount: mutations.filter(\.isQuarantined).count,
            invalidPayloadMutationCount: mutations.filter {
                $0.isQuarantined
                    && $0.isConflict == false
                    && (($0.lastErrorSummary ?? "").hasPrefix("Invalid payload"))
            }.count
        )
    }

    var cacheInvalidationKey: String {
        let activeKey = mutationKey(accountID: activeAccountID ?? "active", mutations: activePendingMutations)
        let workspaceKeys = accountWorkspaces
            .sorted { $0.accountID < $1.accountID }
            .map { mutationKey(accountID: $0.accountID, mutations: $0.pendingMutations) }
        return ([activeKey] + workspaceKeys).joined(separator: "|")
    }

    private func mutationKey(accountID: GoogleAccount.ID, mutations: [PendingMutation]) -> String {
        let conflicts = mutations.filter(\.isConflict).count
        let quarantined = mutations.filter(\.isQuarantined).count
        let invalid = mutations.filter {
            $0.isQuarantined
                && $0.isConflict == false
                && (($0.lastErrorSummary ?? "").hasPrefix("Invalid payload"))
        }.count
        return "\(accountID):\(mutations.count):\(conflicts):\(quarantined):\(invalid)"
    }
}
