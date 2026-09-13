import SwiftUI
import AppKit

private enum OnboardingStage {
    case language
    case introDetails
    case setup
}

struct OnboardingView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.dismiss) private var dismiss
    @State private var stage: OnboardingStage = .language

    var body: some View {
        Group {
            switch stage {
            case .language:
                LanguageWelcomeView(
                    onContinue: { stage = .introDetails }
                )
            case .introDetails:
                IntroDetailsView(
                    onContinue: { stage = .setup },
                    onLater: finish
                )
            case .setup:
                setupBody
            }
        }
        .environment(\.locale, model.settings.appLanguage.locale)
        .appBackground()
        .interactiveDismissDisabled(model.settings.hasCompletedOnboarding == false)
        .hcbScaledFrame(minWidth: 560, idealWidth: 600, minHeight: 560, idealHeight: 640)
    }

    private var setupBody: some View {
        Form {
            Section { GoogleOAuthClientSetupView() }
            Section { ConnectGoogleCard() }
            Section { SyncPreferenceCard() }
            SourceSelectionCard()
            Section { ReminderPreferenceCard() }
            Section { FinishOnboardingCard(finish: finish) }
        }
        .formStyle(.grouped)
    }

    private func finish() {
        model.completeOnboarding()
        dismiss()
    }
}

private struct LanguageWelcomeView: View {
    @Environment(AppModel.self) private var model
    let onContinue: () -> Void

    var body: some View {
        VStack(spacing: 28) {
            Spacer(minLength: 0)

            VStack(spacing: 12) {
                Text(verbatim: model.settings.appLanguage.helloWordmark)
                    .font(.system(size: 66, weight: .semibold, design: .rounded))
                    .lineLimit(1)
                    .minimumScaleFactor(0.55)
                    .frame(maxWidth: .infinity)
                    .accessibilityLabel("Welcome")

                Text("Choose your app language")
                    .hcbFont(.title2, weight: .semibold)
                    .multilineTextAlignment(.center)

                Text("Pick the language Hot Cross Buns uses before setup. You can change this later in Settings.")
                    .hcbFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
            }

            VStack(alignment: .leading, spacing: 10) {
                AppLanguagePicker(title: "App language")
                    .pickerStyle(.radioGroup)
            }
            .frame(maxWidth: 320, alignment: .leading)

            Spacer(minLength: 0)

            Button("Continue", action: onContinue)
                .buttonStyle(.borderedProminent)
                .keyboardShortcut(.defaultAction)
                .frame(maxWidth: .infinity, alignment: .trailing)
        }
        .hcbScaledPadding(32)
        .cardSurface(cornerRadius: 16)
        .hcbScaledPadding(20)
    }
}

struct AppLanguagePicker: View {
    @Environment(AppModel.self) private var model
    let title: LocalizedStringKey

    var body: some View {
        Picker(title, selection: languageBinding) {
            ForEach(AppLanguage.allCases) { language in
                Text(verbatim: language.title)
                    .tag(language)
            }
        }
    }

    private var languageBinding: Binding<AppLanguage> {
        Binding(
            get: { model.settings.appLanguage },
            set: { model.setAppLanguage($0) }
        )
    }
}

private struct IntroDetailsView: View {
    let onContinue: () -> Void
    let onLater: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 28) {
                VStack(alignment: .leading, spacing: 14) {
                    Text("Hot Cross Buns")
                        .font(.system(.largeTitle, weight: .bold))
                        .foregroundStyle(AppColor.ink)
                    Text("A Mac-native client for Google Tasks and Google Calendar.")
                        .hcbFont(.title3)
                        .foregroundStyle(.secondary)
                }

                VStack(alignment: .leading, spacing: 14) {
                    introPoint(
                        icon: "cloud.fill",
                        title: "Your data stays with Google",
                        body: "Hot Cross Buns is a viewer and editor. Every task and event lives in your Google account; edits in Gmail, the Calendar web UI, or your phone show up here and vice versa."
                    )
                    introPoint(
                        icon: "bolt.fill",
                        title: "Fast and offline-tolerant",
                        body: "New tasks and events appear instantly, even without a connection. They show a pending badge until Google accepts them, then the local ID is swapped for the server one."
                    )
                    introPoint(
                        icon: "lock.fill",
                        title: "Bring your own Google access",
                        body: "Hot Cross Buns has no backend. Downloaded builds can use your own Google Cloud Desktop OAuth client, source builds can embed one at build time, and tokens stay in your Mac Keychain."
                    )
                    introPoint(
                        icon: "exclamationmark.shield",
                        title: "Unsigned preview install",
                        body: "If macOS blocks the first launch, open Hot Cross Buns once, then go to System Settings > Privacy & Security and click Open Anyway. You should only need to do this once per Mac."
                    )
                }

                FirstLaunchWarningsCard()

                HStack {
                    Button("Later", action: onLater)
                        .buttonStyle(.bordered)
                    Spacer()
                    Button("Continue", action: onContinue)
                        .buttonStyle(.borderedProminent)
                        .keyboardShortcut(.defaultAction)
                }
            }
            .hcbScaledPadding(28)
            .cardSurface(cornerRadius: 16)
            .hcbScaledPadding(20)
        }
    }

    private func introPoint(icon: String, title: String, body: String) -> some View {
        Label {
            VStack(alignment: .leading, spacing: 4) {
                Text(title).hcbFont(.headline)
                Text(body).hcbFont(.subheadline).foregroundStyle(.secondary)
            }
        } icon: {
            Image(systemName: icon)
                .hcbFont(.title3)
                .foregroundStyle(.secondary)
        }
    }
}

private struct FirstLaunchWarningsCard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("First-launch warnings you may see")
                .hcbFont(.headline)
                .foregroundStyle(AppColor.ink)

            Text("Unsigned DMGs and personal Google OAuth clients can show scary system copy. If you built Hot Cross Buns yourself or downloaded it from a trusted release, these are the approval paths to expect.")
                .hcbFont(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            VStack(alignment: .leading, spacing: 10) {
                warningImage(
                    resource: "macos-not-opened-warning",
                    extensionName: "jpeg",
                    accessibilityLabel: "macOS says Hot Cross Buns was not opened because Apple could not verify it"
                )
                warningImage(
                    resource: "macos-open-anyway-warning",
                    extensionName: "jpeg",
                    accessibilityLabel: "System Settings Privacy and Security shows Open Anyway for Hot Cross Buns"
                )
                GoogleUnverifiedWarningPreview()
            }

            Text("For Google, click Advanced, then Go to Hot Cross Buns only if you trust the app and the OAuth project is yours. Personal unverified OAuth clients can keep showing this warning.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
        .hcbScaledPadding(16)
        .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .strokeBorder(.quaternary)
        )
    }

    @ViewBuilder
    private func warningImage(resource: String, extensionName: String, accessibilityLabel: String) -> some View {
        if let image = onboardingImage(resource: resource, extensionName: extensionName) {
            Image(nsImage: image)
                .resizable()
                .scaledToFit()
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 10, style: .continuous)
                        .strokeBorder(.quaternary)
                )
                .accessibilityLabel(accessibilityLabel)
        }
    }

    private func onboardingImage(resource: String, extensionName: String) -> NSImage? {
        guard let url = Bundle.main.url(
            forResource: resource,
            withExtension: extensionName,
            subdirectory: "Onboarding"
        ) else {
            return nil
        }
        return NSImage(contentsOf: url)
    }
}

private struct GoogleUnverifiedWarningPreview: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack(spacing: 10) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .hcbFont(.largeTitle)
                    .foregroundStyle(.red)
                VStack(alignment: .leading, spacing: 4) {
                    Text("Google hasn't verified this app")
                        .hcbFont(.headline)
                    Text("You may see this while Google is reviewing Hot Cross Buns.")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }

            HStack(alignment: .top, spacing: 18) {
                VStack(alignment: .leading, spacing: 6) {
                    Text("1. Click Advanced")
                    Text("2. Click Go to Hot Cross Buns")
                }
                .hcbFont(.caption)
                .foregroundStyle(.secondary)

                Spacer(minLength: 12)

                Text("BACK TO SAFETY")
                    .hcbFont(.caption)
                    .fontWeight(.semibold)
                    .foregroundStyle(.white)
                    .padding(.horizontal, 12)
                    .padding(.vertical, 8)
                    .background(Color.blue, in: RoundedRectangle(cornerRadius: 4, style: .continuous))
            }
        }
        .hcbScaledPadding(14)
        .background(Color(nsColor: .textBackgroundColor), in: RoundedRectangle(cornerRadius: 10, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .strokeBorder(.quaternary)
        )
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Google has not verified this app warning preview")
    }
}

private struct ConnectGoogleCard: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            OnboardingStepHeader(number: 1, title: "Connect Google", systemImage: "person.crop.circle.badge.checkmark")

            switch model.authState {
            case .signedIn(let account):
                Label(account.displayName, systemImage: "checkmark.circle.fill")
                    .foregroundStyle(AppColor.moss)
                Text("Signed in with Tasks + Calendar scopes.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            case .authenticating:
                HStack(spacing: 8) {
                    ProgressView().controlSize(.small)
                    Text("Opening Google Sign-In…")
                        .hcbFont(.subheadline)
                        .foregroundStyle(.secondary)
                }
            case .cancelled(let message):
                Text(message)
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
                connectButton
                scopeFootnote
            case .failed(let message):
                Text(message)
                    .hcbFont(.footnote)
                    .foregroundStyle(.red)
                connectButton
                scopeFootnote
            case .signedOut:
                if isOAuthConfigured {
                    Text("Sign in once to grant access. Hot Cross Buns asks for Google Tasks + Calendar only — no Gmail, Drive, or contacts.")
                        .foregroundStyle(.secondary)
                } else {
                    Text("This build is missing a Google OAuth client ID. Follow the Google Cloud setup above, rebuild the app, then connect your account.")
                        .foregroundStyle(.red)
                }
                connectButton
                scopeFootnote
            }
        }
    }

    private var isOAuthConfigured: Bool {
        model.customOAuthClientConfiguration != nil || GoogleAuthService.isConfigured(
            clientID: Bundle.main.object(forInfoDictionaryKey: "GIDClientID") as? String
        )
    }

    private var connectButton: some View {
        Button {
            Task {
                await model.connectGoogleAccount()
            }
        } label: {
            Label("Connect Google", systemImage: "person.crop.circle.badge.plus")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(.borderedProminent)
        .tint(AppColor.ember)
        .disabled(isOAuthConfigured == false)
    }

    private var scopeFootnote: some View {
        Text("Tokens stay in your Mac Keychain. Disconnecting in Settings wipes local state.")
            .hcbFont(.caption)
            .foregroundStyle(.secondary)
    }
}

private struct SyncPreferenceCard: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            OnboardingStepHeader(number: 2, title: "Pick Sync Behavior", systemImage: "arrow.triangle.2.circlepath")

            Picker("Sync mode", selection: syncModeBinding) {
                ForEach(SyncMode.allCases) { mode in
                    Text(mode.title).tag(mode)
                }
            }
            .syncModePickerStyle()

            Text(model.settings.syncMode.detail)
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)

            VStack(alignment: .leading, spacing: 6) {
                ForEach(SyncMode.allCases) { mode in
                    HStack(alignment: .top, spacing: 8) {
                        Image(systemName: mode == model.settings.syncMode ? "checkmark.circle.fill" : "circle")
                            .foregroundStyle(mode == model.settings.syncMode ? AppColor.moss : .secondary)
                        Text("\(mode.title): \(mode.guidance)")
                            .hcbFont(.caption)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
            }
        }
    }

    private var syncModeBinding: Binding<SyncMode> {
        Binding(
            get: { model.settings.syncMode },
            set: { model.updateSyncMode($0) }
        )
    }
}

private struct SourceSelectionCard: View {
    @Environment(AppModel.self) private var model

    var body: some View {
        Group {
            Section {
                if model.account == nil {
                    Text("Finish step 1 first — your task lists and calendars will appear here once Google is connected.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                } else if model.taskLists.isEmpty && model.calendars.isEmpty {
                    if case .syncing = model.syncState {
                        VStack(alignment: .leading, spacing: 6) {
                            Label("Loading from Google…", systemImage: "arrow.triangle.2.circlepath")
                            Text("First load can take 10-30 seconds on larger Google accounts.")
                        }
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                    } else {
                        Text("No lists or calendars yet. Tap Refresh above, then give Google up to 30 seconds on larger accounts. You can also create a list in Google Tasks and try again.")
                            .hcbFont(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
            } header: {
                HStack {
                    OnboardingStepHeader(number: 3, title: "Choose Sources", systemImage: "line.3.horizontal.decrease.circle")
                    Spacer()
                    Button("Refresh") {
                        Task { await model.refreshNow() }
                    }
                    .buttonStyle(.link)
                    .controlSize(.small)
                    .accessibilityLabel("Refresh Google lists and calendars")
                }
            }

            if model.taskLists.isEmpty == false {
                Section("Task lists") {
                    ForEach(model.taskLists) { taskList in
                        Toggle(isOn: taskListBinding(taskList.id)) {
                            Text(taskList.title)
                        }
                    }
                }
            }

            if model.calendars.isEmpty == false {
                Section("Calendars") {
                    ForEach(model.calendars) { calendar in
                        Toggle(isOn: calendarBinding(calendar.id)) {
                            HStack(spacing: 10) {
                                Circle()
                                    .fill(Color(hex: calendar.colorHex))
                                    .hcbScaledFrame(width: 10, height: 10)
                                Text(calendar.summary)
                            }
                        }
                    }
                }
            }
        }
    }

    private func taskListBinding(_ id: TaskListMirror.ID) -> Binding<Bool> {
        Binding(
            get: { model.isTaskListSelected(id) },
            set: { _ in model.toggleTaskList(id) }
        )
    }

    private func calendarBinding(_ id: CalendarListMirror.ID) -> Binding<Bool> {
        Binding(
            get: { model.calendars.first(where: { $0.id == id })?.isSelected ?? false },
            set: { _ in model.toggleCalendar(id) }
        )
    }
}

private struct ReminderPreferenceCard: View {
    @Environment(AppModel.self) private var model
    @State private var primer: PermissionPrimer?
    @State private var showNotificationsDeniedAlert = false
    @State private var showLocalNotificationsInfo = false

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            OnboardingStepHeader(number: 4, title: "Local Reminders", systemImage: "bell.badge")
            Toggle("Schedule device-local reminders", isOn: remindersBinding)
            Text("Task reminders fire at 9:00 AM on the due date. Timed events fire 15 minutes before start.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .sheet(item: $primer) { current in
            PermissionPrimerView(primer: current) {
                primer = nil
                Task {
                    let result = await model.requestEnableLocalNotifications()
                    await MainActor.run {
                        if result == .authorized {
                            showLocalNotificationsInfo = true
                        } else {
                            showNotificationsDeniedAlert = true
                        }
                    }
                }
            } onCancel: {
                primer = nil
            }
        }
        .alert("Local reminders enabled", isPresented: $showLocalNotificationsInfo) {
            Button("OK") { showLocalNotificationsInfo = false }
        } message: {
            Text("Hot Cross Buns will schedule up to 64 pending reminders on this Mac for the soonest-upcoming due tasks and Calendar events. 64 is an Apple-imposed ceiling for local notifications per app — later items get scheduled automatically as earlier ones fire or complete.")
        }
        .alert("Notifications are off for Hot Cross Buns", isPresented: $showNotificationsDeniedAlert) {
            Button("Open Notifications Settings") {
                HotCrossBunsSystemSettings.open(HotCrossBunsSystemSettings.notificationsURL)
            }
            Button("Cancel", role: .cancel) {
                showNotificationsDeniedAlert = false
            }
        } message: {
            Text("macOS blocked notifications for Hot Cross Buns. Open System Settings > Notifications > Hot Cross Buns to allow device-local reminders.")
        }
    }

    private var remindersBinding: Binding<Bool> {
        Binding(
            get: { model.settings.enableLocalNotifications },
            set: { newValue in
                if newValue {
                    primer = .notifications
                } else {
                    model.updateLocalNotificationsEnabled(false)
                }
            }
        )
    }
}

private struct FinishOnboardingCard: View {
    @Environment(AppModel.self) private var model
    let finish: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("You can change all of this later in Settings.")
                .foregroundStyle(.secondary)
            Button(action: finish) {
                Label(model.account == nil ? "Finish Without Connecting" : "Finish Setup", systemImage: "checkmark.circle.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .tint(model.account == nil ? AppColor.ember : AppColor.moss)
            if model.account == nil {
                Text("Hot Cross Buns cannot store tasks, events, or notes until a Google account is connected. You can connect later from Settings, but the app will be read-only empty setup until then.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }
}

private struct OnboardingStepHeader: View {
    let number: Int
    let title: String
    let systemImage: String

    var body: some View {
        Label("\(number). \(title)", systemImage: systemImage)
            .hcbFont(.headline)
    }
}

#Preview {
    OnboardingView()
        .environment(AppModel.preview)
}
