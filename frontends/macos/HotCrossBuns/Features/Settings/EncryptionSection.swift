import SwiftUI

// Settings block for §6.12 local-cache encryption. Off by default. Enabling
// prompts for a passphrase; disabling requires the current passphrase so a
// stolen laptop can't flip the toggle. A third option rotates the passphrase
// without disabling encryption.
//
// Explicit warning: Google tasks + events are never encrypted — only the
// local mirror. Forgetting the passphrase means a re-sign-in re-downloads
// everything except offline pending mutations, which are local-only.
struct EncryptionSection: View {
    @Environment(AppModel.self) private var model
    @State private var sheet: Sheet?
    @State private var statusMessage: String?

    private enum Sheet: Identifiable {
        case enable
        case disable
        case change
        var id: String { switch self { case .enable: "e"; case .disable: "d"; case .change: "c" } }
    }

    var body: some View {
        Section("Local privacy encryption") {
            Toggle(isOn: toggleBinding) {
                Label("Encrypt local cache", systemImage: model.settings.cacheEncryptionEnabled ? "lock.fill" : "lock.open")
            }
            HStack {
                Label("Encrypt history audit log", systemImage: model.settings.auditLogEncryptionEnabled ? "clock.badge.checkmark" : "clock.badge.exclamationmark")
                Spacer()
                Text(model.settings.auditLogEncryptionEnabled ? "On" : "Off")
                    .foregroundStyle(model.settings.auditLogEncryptionEnabled ? AppColor.moss : .secondary)
            }
            .hcbFont(.footnote)
            if model.settings.cacheEncryptionEnabled {
                Button {
                    sheet = .change
                } label: {
                    Label("Change passphrase…", systemImage: "key.horizontal")
                }
                .buttonStyle(.borderless)
            }
            if let statusMessage {
                Text(statusMessage)
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
            Text("Encrypts the local JSON mirror, offline mutation queue, and history audit log with AES-256-GCM. Google remains the source of truth — re-signing-in always restores task and event data. Forgetting the passphrase means any unsynced offline mutations and encrypted history are lost; synced Google data is not.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .sheet(item: $sheet) { which in
            switch which {
            case .enable:
                PassphrasePromptSheet(
                    mode: .enable,
                    onSubmit: { new, _ in
                        Task {
                            let ok = await model.enableCacheEncryption(passphrase: new)
                            statusMessage = ok ? "Encryption enabled. Cache and history log rewritten." : (model.lastMutationError ?? "Could not enable.")
                            sheet = nil
                        }
                    },
                    onCancel: { sheet = nil }
                )
            case .disable:
                PassphrasePromptSheet(
                    mode: .disable,
                    onSubmit: { current, _ in
                        Task {
                            let ok = await model.disableCacheEncryption(currentPassphrase: current)
                            statusMessage = ok ? "Encryption disabled. Cache and history log rewritten as plaintext." : (model.lastMutationError ?? "Could not disable.")
                            sheet = nil
                        }
                    },
                    onCancel: { sheet = nil }
                )
            case .change:
                PassphrasePromptSheet(
                    mode: .change,
                    onSubmit: { current, next in
                        Task {
                            let ok = await model.changeCachePassphrase(from: current, to: next)
                            statusMessage = ok ? "Passphrase changed. Cache and history log re-encrypted." : (model.lastMutationError ?? "Could not change passphrase.")
                            sheet = nil
                        }
                    },
                    onCancel: { sheet = nil }
                )
            }
        }
    }

    private var toggleBinding: Binding<Bool> {
        Binding(
            get: { model.settings.cacheEncryptionEnabled },
            set: { wantEnabled in
                if wantEnabled {
                    sheet = .enable
                } else {
                    sheet = .disable
                }
            }
        )
    }
}

private struct PassphrasePromptSheet: View {
    @Environment(\.dismiss) private var dismiss
    enum Mode { case enable, disable, change }
    let mode: Mode
    let onSubmit: (String, String) -> Void // (firstField, secondField)
    let onCancel: () -> Void

    @State private var first: String = ""
    @State private var second: String = ""
    @State private var confirm: String = ""

    var body: some View {
        NavigationStack {
            Form {
                switch mode {
                case .enable:
                    Section("New passphrase") {
                        SecureField("Passphrase", text: $first)
                        SecureField("Confirm", text: $confirm)
                    }
                case .disable:
                    Section("Current passphrase") {
                        SecureField("Passphrase", text: $first)
                        Text("Disabling will rewrite the cache as plaintext on disk. The passphrase is required so a stolen laptop can't flip this off.")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                    }
                case .change:
                    Section("Current passphrase") {
                        SecureField("Current", text: $first)
                    }
                    Section("New passphrase") {
                        SecureField("New", text: $second)
                        SecureField("Confirm new", text: $confirm)
                    }
                }
            }
            .navigationTitle(title)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel", action: onCancel)
                        .keyboardShortcut(.cancelAction)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Apply") {
                        onSubmit(first, mode == .change ? second : "")
                    }
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
                    .disabled(canSubmit == false)
                }
            }
        }
        .hcbScaledFrame(minWidth: 380, minHeight: 240)
    }

    private var title: String {
        switch mode {
        case .enable: "Enable encryption"
        case .disable: "Disable encryption"
        case .change: "Change passphrase"
        }
    }

    private var canSubmit: Bool {
        switch mode {
        case .enable:
            return first.isEmpty == false && first == confirm
        case .disable:
            return first.isEmpty == false
        case .change:
            return first.isEmpty == false && second.isEmpty == false && second == confirm
        }
    }
}
