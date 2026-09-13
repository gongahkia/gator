import SwiftUI

struct MenuBarIconPickerLabel: View {
    let icon: AppSettings.MenuBarIcon

    var body: some View {
        HStack(spacing: 8) {
            MenuBarIconGlyph(icon: icon)
                .frame(width: 16, height: 16)
            Text(icon.title)
        }
    }
}

struct MenuBarIconGlyph: View {
    let icon: AppSettings.MenuBarIcon

    @ViewBuilder
    var body: some View {
        if let systemImageName = icon.systemImageName {
            Image(systemName: systemImageName)
                .symbolRenderingMode(.monochrome)
        } else {
            Image("MenuBarIcon")
                .renderingMode(.template)
                .resizable()
                .scaledToFit()
        }
    }
}

struct MenuBarIconPickerRow: View {
    @Environment(AppModel.self) private var model
    @State private var isShowingGrid = false

    private let columns = Array(repeating: GridItem(.fixed(74), spacing: 8), count: 5)

    var body: some View {
        HStack(alignment: .center, spacing: 12) {
            Text("Menu bar icon")
            Spacer(minLength: 16)
            MenuBarIconPickerLabel(icon: model.settings.menuBarIcon)
                .foregroundStyle(.secondary)
            Button("Change…") {
                isShowingGrid = true
            }
            .popover(isPresented: $isShowingGrid, arrowEdge: .bottom) {
                iconGrid
            }
        }
    }

    private var iconGrid: some View {
        ScrollView {
            LazyVGrid(columns: columns, spacing: 8) {
                ForEach(AppSettings.MenuBarIcon.allCases) { icon in
                    Button {
                        model.setMenuBarIcon(icon)
                        isShowingGrid = false
                    } label: {
                        MenuBarIconGridCell(icon: icon, isSelected: model.settings.menuBarIcon == icon)
                    }
                    .buttonStyle(.plain)
                    .help(icon.title)
                }
            }
            .padding(12)
        }
        .frame(width: 410, height: 500)
    }
}

private struct MenuBarIconGridCell: View {
    let icon: AppSettings.MenuBarIcon
    let isSelected: Bool

    var body: some View {
        VStack(spacing: 6) {
            MenuBarIconGlyph(icon: icon)
                .frame(width: 22, height: 22)
            Text(icon.gridTitle)
                .hcbFont(.caption2, weight: isSelected ? .semibold : .regular)
                .multilineTextAlignment(.center)
                .lineLimit(2)
                .frame(height: 26)
        }
        .frame(width: 66, height: 62)
        .background(selectionBackground)
        .overlay(selectionBorder)
    }

    private var selectionBackground: some View {
        RoundedRectangle(cornerRadius: 7)
            .fill(isSelected ? Color.accentColor.opacity(0.18) : Color.clear)
    }

    private var selectionBorder: some View {
        RoundedRectangle(cornerRadius: 7)
            .strokeBorder(isSelected ? Color.accentColor : Color.secondary.opacity(0.35), lineWidth: 1)
    }
}

private extension AppSettings.MenuBarIcon {
    var gridTitle: String {
        switch self {
        case .calendarPlus: "Cal +"
        case .calendarMinus: "Cal -"
        case .calendarCircle: "Cal O"
        case .checkCircle: "Check O"
        case .checkSquare: "Check Sq"
        case .listRectangle: "List Box"
        case .textCheck: "Text"
        case .archiveBox: "Archive"
        case .shippingBox: "Ship"
        case .paperplane: "Plane"
        case .cloudSun: "Cloud Sun"
        case .mapPin: "Map Pin"
        default: title
        }
    }
}

extension View {
    func syncModePickerStyle() -> some View {
        pickerStyle(.menu)
    }
}

struct AccountStatusView: View {
    @Environment(AppModel.self) private var model
    let authState: AuthState
    let account: GoogleAccount?
    let accounts: [GoogleAccount]
    let activeAccountID: GoogleAccount.ID?
    let canConnect: Bool
    let connect: () -> Void
    let disconnect: () -> Void
    let switchAccount: (GoogleAccount.ID) -> Void
    let disconnectAccount: (GoogleAccount.ID) -> Void

    @State private var confirmingDisconnectAccountID: GoogleAccount.ID?
    @State private var cacheFootprint = "Calculating..."

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            statusHeader
            authMessage
            Divider()
            identityProviderRow

            if displayAccounts.isEmpty == false {
                Divider()
                VStack(alignment: .leading, spacing: 10) {
                    ForEach(displayAccounts) { account in
                        accountRow(account, isActive: account.id == resolvedActiveAccountID)
                    }
                }
            }

            if account != nil {
                Button(action: connect) {
                    Label("Add Google Account", systemImage: "person.crop.circle.badge.plus")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(AppColor.ember)
                .disabled(isAuthenticating || canConnect == false)
            } else {
                Button(action: connect) {
                    Label("Connect Google", systemImage: "person.crop.circle.badge.plus")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .tint(AppColor.ember)
                .hcbScaledFrame(maxWidth: 320, alignment: .leading)
                .disabled(isAuthenticating || canConnect == false)
                if canConnect == false {
                    Text("Save a desktop OAuth client above, or build the app with an embedded Google Sign-In client.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
        }
        .hcbScaledPadding(.vertical, 6)
        .task(id: disconnectImpactID) {
            cacheFootprint = await model.cacheFootprintDescription()
        }
        .confirmationDialog(
            "Disconnect Google?",
            isPresented: Binding(
                get: { confirmingDisconnectAccountID != nil },
                set: { if $0 == false { confirmingDisconnectAccountID = nil } }
            ),
            titleVisibility: .visible
        ) {
            Button("Disconnect Google", role: .destructive) {
                if let confirmingDisconnectAccountID {
                    disconnectAccount(confirmingDisconnectAccountID)
                } else {
                    disconnect()
                }
                confirmingDisconnectAccountID = nil
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text(disconnectConfirmationMessage)
        }
    }

    private var statusHeader: some View {
        HStack {
            Label(statusTitle, systemImage: iconName)
                .hcbFont(.headline)
            Spacer()
            if case .authenticating = authState {
                ProgressView()
                    .controlSize(.small)
            }
        }
    }

    @ViewBuilder
    private var authMessage: some View {
        if case .failed(let message) = authState {
            Text(message)
                .hcbFont(.footnote)
                .foregroundStyle(.red)
        } else if case .cancelled(let message) = authState {
            Text(message)
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var identityProviderRow: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "globe")
                .foregroundStyle(.secondary)
                .frame(width: 22)
            VStack(alignment: .leading, spacing: 3) {
                Text("Google")
                    .hcbFont(.subheadline, weight: .semibold)
                Text(identityProviderDetail)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 0)
        }
    }

    private func accountRow(_ account: GoogleAccount, isActive: Bool) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "person.crop.circle.fill")
                .foregroundStyle(isActive ? AppColor.moss : .secondary)
                .frame(width: 22)
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    Text(account.displayName)
                        .hcbFont(.subheadline, weight: .semibold)
                        .lineLimit(1)
                    if isActive {
                        Text("Active")
                            .hcbFont(.caption2, weight: .semibold)
                            .foregroundStyle(AppColor.moss)
                            .padding(.horizontal, 6)
                            .padding(.vertical, 1)
                            .background(Capsule().fill(AppColor.moss.opacity(0.14)))
                    }
                }
                Text(account.email)
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                Text(scopeSummary(for: account))
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            Spacer(minLength: 0)
            if isActive == false {
                Button {
                    switchAccount(account.id)
                } label: {
                    Label("Switch Account", systemImage: "arrow.left.arrow.right")
                }
                .buttonStyle(.bordered)
                .help("Make this the active Google account")
            }
            Button(role: .destructive) {
                confirmingDisconnectAccountID = account.id
            } label: {
                Image(systemName: "person.crop.circle.badge.xmark")
            }
            .buttonStyle(.borderless)
            .accessibilityLabel("Disconnect \(account.displayName)")
            .help("Disconnect this Google account")
        }
        .hcbScaledPadding(.vertical, 3)
    }

    private var statusTitle: String {
        if account != nil {
            return "Connected"
        }
        return authState.title
    }

    private var identityProviderDetail: String {
        if let account {
            return "Identity provider - \(account.authProvider.title)"
        }
        return canConnect ? "Identity provider ready" : "Desktop OAuth client required"
    }

    private var isAuthenticating: Bool {
        if case .authenticating = authState {
            return true
        }
        return false
    }

    private var displayAccounts: [GoogleAccount] {
        var seen: Set<GoogleAccount.ID> = []
        var ordered: [GoogleAccount] = []
        if let account {
            ordered.append(account)
            seen.insert(account.id)
        }
        for account in accounts where seen.insert(account.id).inserted {
            ordered.append(account)
        }
        return ordered
    }

    private var resolvedActiveAccountID: GoogleAccount.ID? {
        activeAccountID ?? account?.id
    }

    private var iconName: String {
        switch authState {
        case .signedIn:
            "person.crop.circle.badge.checkmark"
        case .authenticating:
            "hourglass"
        case .cancelled:
            "info.circle"
        case .failed:
            "exclamationmark.triangle"
        case .signedOut:
            "person.crop.circle.badge.plus"
        }
    }

    private var disconnectImpactID: String {
        [
            account?.id ?? "signed-out",
            disconnectImpactResolver.cacheInvalidationKey
        ].joined(separator: ":")
    }

    private var disconnectConfirmationMessage: String {
        let targetID = confirmingDisconnectAccountID ?? account?.id
        let targetAccount = targetID.flatMap { id in displayAccounts.first { $0.id == id } } ?? account
        return disconnectImpactResolver.summary(
            for: targetID,
            accountName: targetAccount?.displayName ?? "this Google account",
            cacheFootprint: cacheFootprint
        ).confirmationMessage
    }

    private var disconnectImpactResolver: AccountDisconnectImpactResolver {
        AccountDisconnectImpactResolver(
            activeAccountID: resolvedActiveAccountID,
            activePendingMutations: model.pendingMutations,
            accountWorkspaces: model.accountWorkspaces
        )
    }

    private func scopeSummary(for account: GoogleAccount) -> String {
        let granted = [
            account.grantedScopes.contains(GoogleScope.tasks) ? "Tasks" : nil,
            account.grantedScopes.contains(GoogleScope.calendar) ? "Calendar" : nil
        ]
        .compactMap { $0 }

        guard granted.isEmpty == false else {
            return "Google profile connected. Tasks and Calendar scopes still need consent."
        }

        return "Granted scopes: \(granted.joined(separator: ", "))"
    }
}

struct SyncSettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(AppModel.self) private var model

    var body: some View {
        NavigationStack {
            List {
                Section("Current mode") {
                    Text(model.settings.syncMode.title)
                        .hcbFont(.headline)
                    Text(model.settings.syncMode.detail)
                        .foregroundStyle(.secondary)
                }

                Section("Reality check") {
                    Text("Manual only refreshes on request. Balanced refreshes on launch and foreground. Near real-time adds foreground polling every 90 seconds with backoff on rate-limits.")
                        .hcbFont(.callout)
                }
            }
            .navigationTitle("Sync Details")
            .toolbar {
                Button("Close") {
                    dismiss()
                }
            }
        }
        .frame(minWidth: 520, minHeight: 360)
    }
}
