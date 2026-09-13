import SwiftUI

struct GuestsSection: View {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Binding var attendees: [String]
    @Binding var draft: String
    @Binding var notifyGuests: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                TextField("Add guest email", text: $draft)
                    .textFieldStyle(.roundedBorder)
                    .onSubmit(addDraft)
                Button {
                    addDraft()
                } label: {
                    Label("Add", systemImage: "plus")
                        .labelStyle(.iconOnly)
                }
                .buttonStyle(.bordered)
                .disabled(isDraftValid == false)
            }
            if isDraftValid == false, draft.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                Text("That doesn't look like a valid email address.")
                    .hcbFont(.caption2)
                    .foregroundStyle(AppColor.ember)
            }

            if attendees.isEmpty == false {
                VStack(spacing: 6) {
                    ForEach(attendees, id: \.self) { email in
                        GuestParticipantRow(
                            email: email,
                            statusTitle: "Invited",
                            statusSymbol: "paperplane",
                            statusTint: .secondary
                        ) {
                            HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.12)) {
                                attendees.removeAll { $0 == email }
                            }
                        }
                    }
                }
            }

            Toggle(isOn: $notifyGuests) {
                Label("Notify guests by email", systemImage: "paperplane")
            }
            .disabled(attendees.isEmpty)
            Text(attendees.isEmpty
                ? "Add at least one guest before notifications can be sent."
                : "Off saves the event silently. Turn this on when guests should receive the invite or update.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private func addDraft() {
        let trimmed = draft.trimmingCharacters(in: .whitespacesAndNewlines)
        guard Self.isPlausibleEmail(trimmed) else { return }
        if attendees.contains(where: { $0.caseInsensitiveCompare(trimmed) == .orderedSame }) == false {
            attendees.append(trimmed)
        }
        draft = ""
    }

    private var isDraftValid: Bool {
        Self.isPlausibleEmail(draft.trimmingCharacters(in: .whitespacesAndNewlines))
    }

    // Lightweight RFC-5322-subset check. Requires exactly one '@', at
    // least one character before and after, a '.' in the domain, and no
    // whitespace anywhere. Rejects the common bad pastes ("@channel",
    // "name@", "@email") that the previous contains("@") accepted.
    static func isPlausibleEmail(_ candidate: String) -> Bool {
        guard candidate.isEmpty == false else { return false }
        let pattern = #"^[^@\s]+@[^@\s]+\.[^@\s]+$"#
        return candidate.range(of: pattern, options: .regularExpression) != nil
    }
}

struct GuestParticipantProfile: Hashable {
    let title: String
    let subtitle: String
    let initials: String

    static func make(email: String, displayName: String? = nil) -> GuestParticipantProfile {
        let trimmedEmail = email.trimmingCharacters(in: .whitespacesAndNewlines)
        let cleanDisplayName = displayName?.trimmingCharacters(in: .whitespacesAndNewlines)
        let fallbackTitle = titleFromEmail(trimmedEmail)
        let title = cleanDisplayName?.isEmpty == false ? cleanDisplayName! : fallbackTitle
        return GuestParticipantProfile(
            title: title,
            subtitle: trimmedEmail,
            initials: initials(from: title, email: trimmedEmail)
        )
    }

    private static func titleFromEmail(_ email: String) -> String {
        let localPart = email.split(separator: "@", maxSplits: 1).first.map(String.init) ?? email
        let words = splitNameParts(localPart)
        guard words.isEmpty == false else { return email }
        return words.joined(separator: " ").capitalized
    }

    private static func initials(from title: String, email: String) -> String {
        let titleParts = splitNameParts(title)
        let sourceParts = titleParts.isEmpty ? splitNameParts(email) : titleParts
        let letters = sourceParts
            .prefix(2)
            .compactMap { $0.first }
            .map { String($0).uppercased() }
            .joined()
        if letters.isEmpty == false {
            return letters
        }
        return "?"
    }

    private static func splitNameParts(_ value: String) -> [String] {
        value
            .split { character in
                character == "." ||
                    character == "_" ||
                    character == "-" ||
                    character == "+" ||
                    character == " " ||
                    character == "@"
            }
            .map(String.init)
            .filter { $0.isEmpty == false }
    }
}

struct GuestParticipantRow: View {
    let profile: GuestParticipantProfile
    let statusTitle: String?
    let statusSymbol: String?
    let statusTint: Color
    let onRemove: (() -> Void)?

    init(
        email: String,
        displayName: String? = nil,
        statusTitle: String? = nil,
        statusSymbol: String? = nil,
        statusTint: Color = .secondary,
        onRemove: (() -> Void)? = nil
    ) {
        self.profile = GuestParticipantProfile.make(email: email, displayName: displayName)
        self.statusTitle = statusTitle
        self.statusSymbol = statusSymbol
        self.statusTint = statusTint
        self.onRemove = onRemove
    }

    var body: some View {
        HStack(spacing: 10) {
            GuestInitialsAvatar(initials: profile.initials)
            VStack(alignment: .leading, spacing: 1) {
                Text(profile.title)
                    .hcbFont(.subheadline, weight: .medium)
                    .foregroundStyle(AppColor.ink)
                    .lineLimit(1)
                Text(profile.subtitle)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
                    .textSelection(.enabled)
            }
            Spacer(minLength: 8)
            if let statusTitle {
                Label(statusTitle, systemImage: statusSymbol ?? "person.crop.circle.badge.checkmark")
                    .hcbFont(.caption2, weight: .medium)
                    .labelStyle(.titleAndIcon)
                    .foregroundStyle(statusTint)
                    .lineLimit(1)
            }
            if let onRemove {
                Button {
                    onRemove()
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Remove \(profile.subtitle)")
            }
        }
        .hcbScaledPadding(.horizontal, 8)
        .hcbScaledPadding(.vertical, 6)
        .background(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .fill(AppColor.cream.opacity(0.26))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .strokeBorder(AppColor.ink.opacity(0.08), lineWidth: 1)
        )
    }
}

private struct GuestInitialsAvatar: View {
    let initials: String

    var body: some View {
        ZStack {
            Circle()
                .fill(AppColor.blue.opacity(0.16))
            Text(initials)
                .hcbFont(.caption2, weight: .bold)
                .foregroundStyle(AppColor.blue)
                .lineLimit(1)
                .minimumScaleFactor(0.7)
        }
        .hcbScaledFrame(width: 30, height: 30)
        .accessibilityHidden(true)
    }
}
