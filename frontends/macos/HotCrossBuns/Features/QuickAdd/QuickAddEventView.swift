import AppKit
import SwiftUI

struct QuickAddEventView: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model

    @State private var input: String = ""
    @State private var parsed: ParsedQuickAddEvent = ParsedQuickAddEvent(summary: "", startDate: nil, endDate: nil, location: nil, isAllDay: false, matchedTokens: [])
    @State private var selectedCalendarID: CalendarListMirror.ID?
    @State private var autoAppliedCalendarTag: String?
    @State private var userManuallyPickedCalendar = false
    @State private var isSubmitting = false
    @State private var errorMessage: String?
    @State private var isGrammarExpanded = false
    @FocusState private var focusedField: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header

            TextField("Add an event — try \"Lunch with Bob tomorrow 1pm at Philz for 45 min\"", text: $input, axis: .vertical)
                .textFieldStyle(.plain)
                .font(.system(.title3, design: .rounded, weight: .medium))
                .lineLimit(1...4)
                .focused($focusedField)
                .onSubmit { Task { await submit() } }
                .onChange(of: input) { _, newValue in reparse(newValue) }
                .accessibilityLabel("Event summary with optional date, time, duration, location, calendar, and color")
                .accessibilityHint("Type an event summary. Phrases like tomorrow 1pm, for 45 min, at Philz, calendar tags, and color tags are parsed.")
                .hcbScaledPadding(14)
                .background(
                    RoundedRectangle(cornerRadius: 16)
                        .fill(AppColor.cream.opacity(0.8))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 16)
                        .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
                )

            if isGrammarExpanded {
                grammarReference
            }

            previewStrip

            if let errorMessage {
                Text(errorMessage)
                    .hcbFont(.caption)
                    .foregroundStyle(AppColor.ember)
            }

            HStack {
                calendarPicker
                Spacer()
                Button("Cancel") { dismiss() }
                    .keyboardShortcut(.cancelAction)
                Button {
                    Task { await submit() }
                } label: {
                    if isSubmitting {
                        ProgressView().controlSize(.small)
                    } else {
                        Text("Add Event")
                    }
                }
                .buttonStyle(.borderedProminent)
                .tint(AppColor.ember)
                .keyboardShortcut(.return, modifiers: [])
                .disabled(canSubmit == false)
            }
        }
        .hcbScaledPadding(22)
        .hcbScaledFrame(width: 560)
        .onAppear {
            selectedCalendarID = selectedCalendarID ?? defaultCalendarID
            focusedField = true
        }
    }

    private var header: some View {
        HStack(spacing: 10) {
            Image(systemName: "calendar.badge.plus")
                .foregroundStyle(AppColor.ember)
            Text("New Event")
                .hcbFont(.headline)
            Spacer(minLength: 0)
            Button {
                HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.18)) {
                    isGrammarExpanded.toggle()
                }
            } label: {
                Image(systemName: isGrammarExpanded ? "info.circle.fill" : "info.circle")
            }
            .buttonStyle(.borderless)
            .help(isGrammarExpanded ? "Hide quick-add grammar" : "Show quick-add grammar")
            .accessibilityLabel(isGrammarExpanded ? "Hide quick-add grammar" : "Show quick-add grammar")
            Text("Return to add, Esc to cancel")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var previewStrip: some View {
        HStack(spacing: 8) {
            if parsed.summary.isEmpty {
                Label("Type a summary", systemImage: "text.cursor")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                chip(icon: "text.alignleft", text: summaryForDisplay(), tint: AppColor.ink)
            }
            if let start = parsed.startDate {
                chip(icon: "clock", text: formattedTime(start), tint: AppColor.moss)
            }
            if let color = matchedEventColor {
                chip(icon: "circle.fill", text: color.title, tint: colorTint(color))
            }
            if let calendar = matchedCalendar {
                chip(icon: "calendar.circle", text: calendar.summary, tint: Color(hex: calendar.colorHex))
            }
            if let loc = parsed.location, loc.isEmpty == false {
                chip(icon: "mappin.and.ellipse", text: loc, tint: AppColor.blue)
            }
            Spacer(minLength: 0)
        }
        .hcbScaledFrame(minHeight: 26)
    }

    private var calendarPicker: some View {
        Picker("Calendar", selection: Binding(
            get: { selectedCalendarID ?? defaultCalendarID },
            set: { newValue in
                selectedCalendarID = newValue
                userManuallyPickedCalendar = true
                autoAppliedCalendarTag = nil
            }
        )) {
            ForEach(model.calendars) { cal in
                Text(cal.summary).tag(Optional(cal.id))
            }
        }
        .pickerStyle(.menu)
        .fixedSize()
        .labelsHidden()
    }

    private var grammarReference: some View {
        VStack(alignment: .leading, spacing: 8) {
            ForEach(NaturalLanguageEventParser.helpEntries) { entry in
                VStack(alignment: .leading, spacing: 4) {
                    Text(entry.title)
                        .hcbFont(.caption, weight: .semibold)
                    Text(entry.examples.joined(separator: "  ·  "))
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .textSelection(.enabled)
                }
            }
        }
        .hcbScaledPadding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.thinMaterial, in: RoundedRectangle(cornerRadius: 10))
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.8)
        )
        .transition(.opacity.combined(with: .move(edge: .top)))
    }

    private func chip(icon: String, text: String, tint: Color) -> some View {
        Label {
            Text(text).lineLimit(1)
        } icon: {
            Image(systemName: icon)
        }
        .hcbFont(.caption, weight: .medium)
        .hcbScaledPadding(.horizontal, 10)
        .hcbScaledPadding(.vertical, 5)
        .background(Capsule().fill(tint.opacity(0.15)))
        .foregroundStyle(tint)
    }

    private var canSubmit: Bool {
        summaryForSubmission().isEmpty == false && (selectedCalendarID ?? defaultCalendarID) != nil && model.account != nil && isSubmitting == false
    }

    private var defaultCalendarID: CalendarListMirror.ID? {
        model.calendarSnapshot.selectedCalendars.first?.id ?? model.calendars.first?.id
    }

    private func reparse(_ text: String) {
        parsed = NaturalLanguageEventParser().parse(text)
        applyCalendarTagAutoSelection()
        if selectedCalendarID == nil, let id = defaultCalendarID {
            selectedCalendarID = id
        }
    }

    private func formattedTime(_ start: Date) -> String {
        if parsed.isAllDay {
            return start.formatted(.dateTime.month(.abbreviated).day())
        }
        guard let end = parsed.endDate else {
            return start.formatted(.dateTime.month(.abbreviated).day().hour().minute())
        }
        if Calendar.current.isDate(start, inSameDayAs: end) {
            return "\(start.formatted(.dateTime.month(.abbreviated).day().hour().minute()))-\(end.formatted(.dateTime.hour().minute()))"
        }
        return "\(start.formatted(.dateTime.month(.abbreviated).day().hour().minute()))-\(end.formatted(.dateTime.month(.abbreviated).day().hour().minute()))"
    }

    private var colorTagResolution: ColorTagResolver.Resolution? {
        guard model.settings.colorTagAutoApplyEnabled else { return nil }
        return ColorTagResolver.resolve(
            title: parsed.summary,
            bindings: model.settings.colorTagBindings,
            policy: model.settings.colorTagMatchPolicy
        )
    }

    private var matchedEventColor: CalendarEventColor? {
        guard let colorId = colorTagResolution?.colorId else { return nil }
        return CalendarEventColor.from(colorId: colorId)
    }

    private var calendarTagResolution: CalendarTagResolver.Resolution? {
        CalendarTagResolver.resolve(title: parsed.summary, calendars: model.calendarSnapshot.selectedCalendars)
            ?? CalendarTagResolver.resolve(title: parsed.summary, calendars: model.calendars)
    }

    private var matchedCalendar: CalendarListMirror? {
        calendarTagResolution?.calendar
    }

    private func applyCalendarTagAutoSelection() {
        guard userManuallyPickedCalendar == false else { return }
        if let resolution = calendarTagResolution {
            if selectedCalendarID != resolution.calendar.id {
                selectedCalendarID = resolution.calendar.id
            }
            autoAppliedCalendarTag = resolution.matchedTag
        } else if autoAppliedCalendarTag != nil {
            selectedCalendarID = defaultCalendarID
            autoAppliedCalendarTag = nil
        }
    }

    private func colorTint(_ color: CalendarEventColor) -> Color {
        if let hex = color.hex { return Color(hex: hex) }
        return AppColor.ember
    }

    private func summaryForDisplay() -> String {
        summaryForSubmission()
    }

    private func summaryForSubmission() -> String {
        let raw = parsed.summary.trimmingCharacters(in: .whitespacesAndNewlines)
        var stripped = raw
        for tag in [colorTagResolution?.matchedTag, autoAppliedCalendarTag].compactMap({ $0 }) {
            stripped = ColorTagResolver.stripTag(tag, from: stripped)
        }
        return stripped.isEmpty ? raw : stripped
    }

    private func submit() async {
        guard canSubmit else { return }
        guard let calendarID = selectedCalendarID ?? defaultCalendarID else { return }
        isSubmitting = true
        errorMessage = nil
        let start = parsed.startDate ?? Date()
        let end = parsed.endDate ?? start.addingTimeInterval(3600)
        let didCreate = await model.createEvent(
            summary: summaryForSubmission(),
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: parsed.isAllDay,
            reminderMinutes: nil,
            calendarID: calendarID,
            location: parsed.location ?? "",
            recurrence: [],
            attendeeEmails: [],
            notifyGuests: false,
            addGoogleMeet: false,
            colorId: matchedEventColor?.wireValue
        )
        isSubmitting = false
        if didCreate {
            dismiss()
        } else {
            errorMessage = model.lastMutationError ?? "Couldn't add event."
        }
    }
}
