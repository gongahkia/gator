import AppKit
import SwiftUI
import GatorCore

struct ModelsAndProvidersButton: View {
    let sessions: ManagedSessionCoordinator
    @Environment(\.openWindow) private var openWindow
    var body: some View {
        Button("Models and Providers…", systemImage: "key.horizontal") {
            sessions.showProviders(); openWindow(id: "providers"); NSApp.activate(ignoringOtherApps: true)
        }.disabled(!sessions.current.isConnected || sessions.current.closing)
            .help("Configure Gator-managed model API keys without making a model request")
            .accessibilityIdentifier("gator.providers.open")
    }
}

struct ProviderSetupView: View {
    let sessions: ManagedSessionCoordinator
    @Bindable private var setup: NativeProviderSetup
    @Environment(\.openWindow) private var openWindow
    @Environment(\.dismissWindow) private var dismissWindow
    init(sessions: ManagedSessionCoordinator) { self.sessions = sessions; setup = sessions.providers }
    var body: some View {
        VStack(spacing: 0) {
            HStack {
                Text("Models and Providers").font(.headline)
                Spacer()
                Button("Refresh Status", systemImage: "arrow.clockwise") { Task { await setup.refresh() } }.disabled(setup.busy)
            }.padding(14)
            Divider()
            HSplitView {
                List(selection: $setup.selection) {
                    ForEach(setup.rows) { row in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(verbatim: row.id).fontWeight(.medium)
                            if row.isDefault == true { Text("Future default").font(.caption).foregroundStyle(.secondary) }
                            Text(row.ready ? "Locally ready" : row.configured ? "Configured · needs attention" : "Not configured")
                                .font(.caption).foregroundStyle(.secondary)
                        }.padding(.vertical, 4).tag(row.id)
                    }
                }.listStyle(.sidebar).frame(minWidth: 145, idealWidth: 190, maxWidth: 240)
                    .accessibilityLabel("Built-in model providers")
                ScrollView {
                    VStack(alignment: .leading, spacing: 16) {
                        if let row = setup.selected {
                            ProviderCredentialEditor(setup: setup, row: row, cancel: close).id(row.id + setup.editorEpoch.uuidString)
                        } else {
                            Text(setup.busy ? "Reading provider status…" : "Select a built-in provider to configure it.").foregroundStyle(.secondary)
                        }
                        if !setup.issue.isEmpty { NativeStatusMessage(text: setup.issue, severity: .error) }
                        if !setup.notice.isEmpty { Label(setup.notice, systemImage: "checkmark.circle").textSelection(.enabled) }
                        if setup.busy { ProgressView("Waiting for Go confirmation…").controlSize(.small) }
                        Text("Configured means settings or credentials exist. Locally ready means local checks pass. Neither means live verified. This screen never makes an inference request.")
                            .font(.caption).foregroundStyle(.secondary)
                        Text("This workflow supports built-in API-key providers. Vendor CLI sign-in, OAuth, custom provider definitions and local runtime installation are not supported here. It does not import another application's credentials.")
                            .font(.caption).foregroundStyle(.secondary)
                    }.padding(18).frame(maxWidth: .infinity, alignment: .leading)
                }.frame(minWidth: 335)
            }
            Divider()
            HStack {
                Text("Credentials stay in Gator’s private Go credential store.").font(.caption).foregroundStyle(.secondary)
                Spacer()
                Button("Return to Chat") { close() }.keyboardShortcut(setup.review == nil ? KeyboardShortcut.cancelAction : nil)
            }.padding(14)
        }
        .background(NativeWindowConfiguration(compact: false, registersConversation: false).frame(width: 0, height: 0))
        .tint(GatorTheme.accent)
        .task { sessions.showProviders() }
        .onDisappear { setup.dismiss() }
        .onExitCommand { if setup.review != nil && !setup.busy { setup.cancelReview() } else { close() } }
        .accessibilityIdentifier("gator.providers.window")
    }
    private func close() { setup.dismiss(); dismissWindow(id: "providers"); openWindow(id: "conversation"); NSApp.activate(ignoringOtherApps: true) }
}

private struct ProviderCredentialEditor: View {
    @Bindable var setup: NativeProviderSetup
    let row: NativeProviderRow
    let cancel: @MainActor () -> Void
    @State private var secret = ""
    @State private var entryIssue = ""
    private var editing: Bool { !setup.busy && setup.review == nil && !setup.needsRefresh }
    private var form: Binding<NativeProviderForm> {
        Binding(get: { setup.forms[row.id] ?? NativeProviderForm() }, set: { setup.forms[row.id] = $0 })
    }
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(verbatim: row.id).font(.title2).textSelection(.enabled)
            VStack(alignment: .leading, spacing: 5) {
                Label(row.stored ? "Gator credential present · \(row.kind)" : "No Gator-stored credential", systemImage: row.stored ? "key.fill" : "key")
                Text(row.ambient ? "Ambient credential source available" : "No ambient credential source detected").font(.callout).foregroundStyle(.secondary)
                if row.expired { NativeStatusMessage(text: "The stored credential is expired.", severity: .warning) }
                if !row.issue.isEmpty { NativeStatusMessage(text: row.issue, severity: .warning) }
                Text("\(row.isDefault == true ? "Current default model" : "Suggested model for setup"): \(row.model.isEmpty ? "not selected" : row.model)").font(.callout).textSelection(.enabled)
                Text("Current processing destination: \(row.destination.isEmpty ? "not available" : row.destination)").font(.callout).textSelection(.enabled)
            }
            if !entryIssue.isEmpty { NativeStatusMessage(text: entryIssue, severity: .error) }
            if let review = setup.review, review.provider == row.id {
                reviewContent(review)
            } else {
                VStack(alignment: .leading, spacing: 10) {
                    TextField("Model / deployment ID", text: form.model).textFieldStyle(.roundedBorder).accessibilityIdentifier("gator.providers.model")
                    Picker("Endpoint", selection: form.endpointMode) {
                        Text("Keep current endpoint").tag("retain")
                        Text("Use provider default").tag("default")
                        Text("Explicit override…").tag("override")
                    }.accessibilityIdentifier("gator.providers.endpoint-mode")
                    if form.wrappedValue.endpointMode == "override" {
                        TextField("HTTP(S) endpoint · no credentials or secret query parameters", text: form.endpoint).textFieldStyle(.roundedBorder)
                            .accessibilityLabel("Explicit provider endpoint").accessibilityIdentifier("gator.providers.endpoint")
                        Text("This explicitly changes where future model context is processed. Review the destination before saving.").font(.caption).foregroundStyle(.secondary)
                    }
                    ForEach(row.fields, id: \.self) { field in
                        if field == "api_version" {
                            TextField("Azure API version", text: form.apiVersion).textFieldStyle(.roundedBorder)
                        } else if field == "gateway_protocol" {
                            Picker("Gateway protocol", selection: option(field)) {
                                Text("Choose protocol").tag("")
                                Text("OpenAI Responses").tag("openai-responses")
                                Text("Anthropic Messages").tag("anthropic-messages")
                                Text("Workers AI Chat Completions").tag("workers-ai-chat-completions")
                            }
                        } else {
                            TextField(fieldLabel(field), text: option(field)).textFieldStyle(.roundedBorder)
                        }
                    }
                    Text("API key").font(.headline)
                    NativeProviderSecretEntry(text: $secret, enabled: editing, onCancel: cancel).frame(height: 24)
                    Text("Empty keeps the existing credential. Saving also selects this provider/model as the future default. No request is sent to the provider.").font(.caption).foregroundStyle(.secondary)
                }.disabled(!editing)
                HStack {
                    Button("Cancel") { secret = ""; setup.cancelReview() }.disabled(setup.busy)
                    Spacer()
                    if row.stored { Button("Remove Credential…") { secret = ""; Task { _ = await setup.prepare(replacing: false, removal: true) } }.disabled(!setup.canReview) }
                    Button("Review Changes…") { reviewChanges() }
                        .disabled(!setup.canReview).accessibilityIdentifier("gator.providers.review")
                }
            }
        }.onDisappear { secret = "" }
    }
    private func reviewChanges() {
        entryIssue = ""
        let entered = secret.trimmingCharacters(in: .whitespacesAndNewlines), fields = form.wrappedValue
        if !entered.isEmpty && ([fields.model, fields.endpoint, fields.apiVersion] + Array(fields.options.values)).contains(where: { $0.contains(entered) }) {
            secret = ""; entryIssue = "API keys belong only in the secure key field. Remove credential material from configuration fields, then review again."
            return
        }
        let replacing = !entered.isEmpty
        Task { _ = await setup.prepare(replacing: replacing) }
    }
    private func option(_ field: String) -> Binding<String> {
        Binding(get: { form.wrappedValue.options[field] ?? "" }, set: { value in var updated = form.wrappedValue; updated.options[field] = value; form.wrappedValue = updated })
    }
    private func fieldLabel(_ field: String) -> String {
        switch field { case "account_id": "Cloudflare account ID"; case "gateway_id": "Cloudflare gateway ID"; case "gateway": "Radius gateway URL"; default: field }
    }
    private func reviewContent(_ review: NativeProviderReview) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(review.operation == "remove" ? "Review credential removal" : "Review provider setup").font(.headline)
            Text("Credential: \(review.credentialAction)")
            if review.setsDefaults {
                Text("Future default: \(review.provider) / \(review.model)").textSelection(.enabled)
                Text("Processing destination: \(review.destination)").textSelection(.enabled)
                Text("Endpoint: \(review.endpointMode == "retain" ? "keep current" : review.endpointMode == "default" ? "provider default" : "explicit override")")
                ForEach((review.options ?? [:]).keys.sorted(), id: \.self) { key in Text("\(key): \(review.options?[key] ?? "")").textSelection(.enabled) }
                Text("Existing work keeps its captured model. Future submissions use this default after Go confirms Save.").font(.caption).foregroundStyle(.secondary)
            } else {
                Text("Only this Gator-stored credential will be removed. Models and endpoints stay configured. Environment or vendor credentials may still authenticate requests.").font(.callout)
            }
            HStack {
                Button("Cancel Review") { secret = ""; setup.cancelReview() }.keyboardShortcut(.cancelAction).disabled(setup.busy)
                Spacer()
                Button(review.operation == "remove" ? "Remove Gator Credential" : "Save Reviewed Changes") {
                    let entered = secret; secret = ""
                    Task { _ = await setup.save(apiKey: entered) }
                }.disabled(setup.busy).accessibilityIdentifier("gator.providers.save")
            }
        }.padding(12).background(.quaternary.opacity(0.4), in: RoundedRectangle(cornerRadius: 8))
    }
}
