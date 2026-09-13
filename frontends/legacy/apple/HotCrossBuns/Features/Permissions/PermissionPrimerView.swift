import AppKit
import SwiftUI

struct PermissionPrimer: Identifiable, Equatable {
    let id: String
    let eyebrow: String
    let title: String
    let message: String
    let systemPromptLead: String
    let systemPromptTitle: String
    let bullets: [String]
    let continueTitle: String
    let systemImage: String

    static let notifications = PermissionPrimer(
        id: "notifications",
        eyebrow: "Before macOS asks",
        title: "Allow local reminders on this Mac",
        message: "Hot Cross Buns uses macOS notifications only for device-local task and event reminders. Nothing new is sent to Google when this is on.",
        systemPromptLead: "You’ll see a macOS dialog next",
        systemPromptTitle: "\"Hot Cross Buns\" Would Like to Send You Notifications",
        bullets: [
            "Task reminders fire on this Mac using your app-wide reminder rule.",
            "Calendar event reminders fire before start using the event's reminder settings.",
            "If you decline, the app stays usable and you can enable reminders later in Settings."
        ],
        continueTitle: "Continue to macOS Prompt",
        systemImage: "bell.badge.fill"
    )

    static let accessibility = PermissionPrimer(
        id: "accessibility",
        eyebrow: "System permission needed",
        title: "Allow the global quick-add hotkey",
        message: "macOS only delivers system-wide keyboard shortcuts to apps that are allowed in Accessibility settings.",
        systemPromptLead: "You’ll open System Settings next",
        systemPromptTitle: "Privacy & Security > Accessibility > Hot Cross Buns",
        bullets: [
            "The permission is used only for the global quick-add hotkey.",
            "Hot Cross Buns does not inspect other apps' content or keystrokes.",
            "If you leave it off, the app stays usable and the global hotkey remains disabled."
        ],
        continueTitle: "Open Accessibility Settings",
        systemImage: "keyboard.badge.eye"
    )
}

enum HotCrossBunsSystemSettings {
    static let notificationsURL = URL(string: "x-apple.systempreferences:com.apple.preference.notifications?id=com.gongahkia.hotcrossbuns.mac")
    static let accessibilityURL = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")

    static func open(_ url: URL?) {
        guard let url else { return }
        NSWorkspace.shared.open(url)
    }
}

struct PermissionPrimerView: View {
    let primer: PermissionPrimer
    let onContinue: () -> Void
    let onCancel: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            Text(primer.eyebrow)
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
                .textCase(.uppercase)

            HStack(alignment: .top, spacing: 14) {
                Image(systemName: primer.systemImage)
                    .hcbFont(.largeTitle)
                    .foregroundStyle(AppColor.ember)
                    .symbolRenderingMode(.hierarchical)
                VStack(alignment: .leading, spacing: 8) {
                    Text(primer.title)
                        .hcbFont(.title3, weight: .semibold)
                    Text(primer.message)
                        .hcbFont(.body)
                        .foregroundStyle(.secondary)
                }
            }

            VStack(alignment: .leading, spacing: 10) {
                Text(primer.systemPromptLead)
                    .hcbFont(.headline)
                VStack(alignment: .leading, spacing: 8) {
                    Text(primer.systemPromptTitle)
                        .hcbFont(.subheadline, weight: .semibold)
                    Text("Allow")
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                        .hcbScaledPadding(.horizontal, 8)
                        .hcbScaledPadding(.vertical, 4)
                        .background(.thinMaterial, in: Capsule())
                }
                .hcbScaledPadding(16)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14))
                .overlay(
                    RoundedRectangle(cornerRadius: 14)
                        .strokeBorder(.separator.opacity(0.8), lineWidth: 1)
                )
            }

            VStack(alignment: .leading, spacing: 8) {
                ForEach(primer.bullets, id: \.self) { bullet in
                    Text("• \(bullet)")
                        .hcbFont(.callout)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }
            }

            HStack {
                Button("Not now", action: onCancel)
                    .buttonStyle(.bordered)
                Spacer()
                Button(primer.continueTitle, action: onContinue)
                    .buttonStyle(.borderedProminent)
                    .keyboardShortcut(.defaultAction)
            }
        }
        .hcbScaledPadding(24)
        .frame(minWidth: 460, idealWidth: 520)
        .appBackground()
    }
}
