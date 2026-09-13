import SwiftUI

struct GoogleOAuthClientSetupView: View {
    @Environment(AppModel.self) private var model
    @State private var clientID: String = ""
    @State private var clientSecret: String = ""
    @State private var didLoad = false
    @State private var didSave = false

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label("Google Cloud OAuth client", systemImage: model.customOAuthClientConfiguration == nil ? "key.fill" : "checkmark.shield.fill")
                    .hcbFont(.headline)
                    .foregroundStyle(model.customOAuthClientConfiguration == nil ? AppColor.ink : AppColor.moss)
                Spacer()
                if let configuration = model.customOAuthClientConfiguration {
                    Text(configuration.redactedClientID)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            DisclosureGroup("Setup checklist") {
                VStack(alignment: .leading, spacing: 8) {
                    setupStep("Create a Google Cloud project.")
                    setupStep("Enable the Google Tasks API and Google Calendar API.")
                    setupStep("Configure the Google Auth platform. For personal Gmail use, choose External and add yourself while testing.")
                    setupStep("In Clients, create a new OAuth client with application type Desktop app. Do not reuse the iOS/macOS client used for source builds.")
                    setupStep("Paste the Desktop app client ID below. If Google shows a client secret, paste it too.")
                    setupStep("Click Save OAuth Client, then Connect Google, and approve the browser consent flow.")
                    setupStep("For daily use without weekly re-consent, publish the OAuth app to In production after setup.")
                    // Keep the storage caveats behind the expanded checklist so the default settings view stays compact.
                    setupStep("Downloaded DMGs can sync through your own Google Cloud project. Set this up once, then Hot Cross Buns keeps the OAuth client and refresh token in your Mac Keychain for normal future launches.")
                    setupStep("The app stores this OAuth client and the resulting refresh token in your macOS Keychain. Clearing the client removes the saved custom client and token from this Mac.")
                }
                .hcbScaledPadding(.top, 8)
                .frame(maxWidth: .infinity, alignment: .leading)
            }

            TextField("Desktop OAuth client ID", text: $clientID)
                .textFieldStyle(.roundedBorder)
                .textContentType(.username)
                .autocorrectionDisabled()

            SecureField("Client secret (optional)", text: $clientSecret)
                .textFieldStyle(.roundedBorder)
                .textContentType(.password)

            HStack {
                Button {
                    model.saveCustomOAuthClientConfiguration(
                        clientID: clientID,
                        clientSecret: clientSecret
                    )
                    didSave = model.customOAuthClientConfiguration != nil
                } label: {
                    Label("Save OAuth Client", systemImage: "square.and.arrow.down")
                }
                .buttonStyle(.borderedProminent)
                .disabled(clientID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)

                if model.customOAuthClientConfiguration != nil {
                    Button(role: .destructive) {
                        model.clearCustomOAuthClientConfiguration()
                        clientID = ""
                        clientSecret = ""
                        didSave = false
                    } label: {
                        Label("Clear", systemImage: "trash")
                    }
                    .buttonStyle(.bordered)
                }
            }

            if didSave {
                Label("Saved. Use Connect Google to finish OAuth in your browser.", systemImage: "checkmark.circle.fill")
                    .hcbFont(.caption)
                    .foregroundStyle(AppColor.moss)
            }

        }
        .onAppear(perform: loadExistingConfiguration)
    }

    private func setupStep(_ text: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Circle()
                .fill(Color.secondary)
                .frame(width: 4, height: 4)
            Text(text)
                .foregroundStyle(.secondary)
        }
        .hcbFont(.caption)
        .fixedSize(horizontal: false, vertical: true)
    }

    private func loadExistingConfiguration() {
        guard didLoad == false else { return }
        didLoad = true
        if let configuration = model.customOAuthClientConfiguration {
            clientID = configuration.clientID
            clientSecret = configuration.clientSecret ?? ""
        }
    }
}
