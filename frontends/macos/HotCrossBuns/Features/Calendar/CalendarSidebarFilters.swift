import SwiftUI

struct CalendarSidebarFilters: View {
    @Environment(AppModel.self) private var model
    @Binding var state: CalendarViewFilterState

    private struct ColorOption: Identifiable {
        let color: CalendarEventColor
        let count: Int

        var id: String { color.rawValue }
    }

    private struct TagOption: Identifiable {
        let key: String
        let title: String
        let count: Int
        let boundColorID: String?

        var id: String { key }
    }

    private var calendarStore: CalendarStore { model.calendarStore }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            calendarSection
            colorSection
            tagSection
        }
        .hcbScaledPadding(.vertical, 8)
        .accessibilityElement(children: .contain)
    }

    private var header: some View {
        HStack(spacing: 8) {
            Text("View filters")
                .font(.caption.weight(.semibold))
                .foregroundStyle(.secondary)
            Spacer(minLength: 0)
            if state.hasActiveFilters {
                Button {
                    state = .allVisible
                } label: {
                    Image(systemName: "arrow.counterclockwise")
                }
                .buttonStyle(.plain)
                .help("Show all calendar events")
                .accessibilityLabel("Clear calendar view filters")
            }
        }
    }

    private var calendarSection: some View {
        filterSection(
            title: "Calendars",
            setAll: { state.visibleCalendarIDs = nil },
            setNone: { state.visibleCalendarIDs = [] }
        ) {
            ForEach(calendarStore.calendarSnapshot.selectedCalendars) { calendar in
                filterRow(
                    title: calendar.summary,
                    count: calendarEventCounts[calendar.id, default: 0],
                    isOn: isCalendarVisible(calendar.id),
                    swatch: AnyView(
                        Circle()
                            .fill(Color(hex: calendar.colorHex))
                            .hcbScaledFrame(width: 9, height: 9)
                    ),
                    action: { setCalendar(calendar.id, visible: isCalendarVisible(calendar.id) == false) }
                )
                .accessibilityLabel("\(calendar.summary) calendar")
            }
        }
    }

    @ViewBuilder
    private var colorSection: some View {
        let options = colorOptions
        if options.isEmpty == false {
            filterSection(
                title: "Colors",
                setAll: { state.visibleColorIDs = nil },
                setNone: { state.visibleColorIDs = [] }
            ) {
                ForEach(options) { option in
                    filterRow(
                        title: option.color.title,
                        count: option.count,
                        isOn: isColorVisible(option.color.rawValue),
                        swatch: AnyView(colorSwatch(option.color)),
                        action: { setColor(option.color.rawValue, visible: isColorVisible(option.color.rawValue) == false) }
                    )
                }
            }
        }
    }

    @ViewBuilder
    private var tagSection: some View {
        let options = tagOptions
        if options.isEmpty {
            VStack(alignment: .leading, spacing: 4) {
                Text("Tags")
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(.secondary)
                Text("No event tags yet")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
        } else {
            filterSection(
                title: "Tags",
                setAll: { state.visibleTagNames = nil },
                setNone: { state.visibleTagNames = [] }
            ) {
                ForEach(options) { option in
                    filterRow(
                        title: option.title,
                        count: option.count,
                        isOn: isTagVisible(option.key),
                        swatch: AnyView(tagSwatch(option.boundColorID)),
                        action: { setTag(option.key, visible: isTagVisible(option.key) == false) }
                    )
                }
            }
        }
    }

    private func filterSection<Content: View>(
        title: String,
        setAll: @escaping () -> Void,
        setNone: @escaping () -> Void,
        @ViewBuilder content: () -> Content
    ) -> some View {
        VStack(alignment: .leading, spacing: 5) {
            HStack(spacing: 6) {
                Text(title)
                    .font(.caption2.weight(.semibold))
                    .foregroundStyle(.secondary)
                Spacer(minLength: 0)
                Button("All", action: setAll)
                    .buttonStyle(.plain)
                    .font(.caption2)
                    .help("Show all \(title.lowercased())")
                Button("None", action: setNone)
                    .buttonStyle(.plain)
                    .font(.caption2)
                    .help("Hide all \(title.lowercased())")
            }
            content()
        }
    }

    private func filterRow(
        title: String,
        count: Int,
        isOn: Bool,
        swatch: AnyView,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            HStack(spacing: 7) {
                Image(systemName: isOn ? "checkmark.square.fill" : "square")
                    .foregroundStyle(isOn ? AppColor.ember : .secondary)
                    .hcbScaledFrame(width: 13)
                swatch
                Text(title)
                    .font(.caption)
                    .lineLimit(1)
                    .foregroundStyle(AppColor.ink)
                Spacer(minLength: 0)
                Text("\(count)")
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityValue(isOn ? "visible, \(count) events" : "hidden, \(count) events")
    }

    @ViewBuilder
    private func colorSwatch(_ color: CalendarEventColor) -> some View {
        if let hex = color.hex {
            Circle()
                .fill(Color(hex: hex))
                .hcbScaledFrame(width: 9, height: 9)
        } else {
            Circle()
                .strokeBorder(AppColor.cardStroke, lineWidth: 1)
                .background(Circle().fill(AppColor.cardSurface))
                .hcbScaledFrame(width: 9, height: 9)
        }
    }

    @ViewBuilder
    private func tagSwatch(_ boundColorID: String?) -> some View {
        if let boundColorID,
           let hex = CalendarEventColor(rawValue: boundColorID)?.hex {
            Circle()
                .fill(Color(hex: hex))
                .hcbScaledFrame(width: 9, height: 9)
        } else {
            Image(systemName: "number")
                .foregroundStyle(.secondary)
                .font(.caption2)
                .hcbScaledFrame(width: 9)
        }
    }

    private var calendarEventCounts: [CalendarListMirror.ID: Int] {
        calendarStore.calendarSnapshot.eventCountsByCalendarID
    }

    private var colorOptions: [ColorOption] {
        let counts = calendarStore.calendarSnapshot.eventCountsByColorID
        let boundColorIDs = Set(calendarStore.settings.colorTagBindings.keys.map(CalendarEventViewFilter.normalizedColorID))
        let ids = Set(counts.keys).union(boundColorIDs).filter { id in
            id.isEmpty || CalendarEventColor(rawValue: id) != nil
        }
        return CalendarEventColor.allCases
            .filter { ids.contains($0.rawValue) }
            .map { ColorOption(color: $0, count: counts[$0.rawValue, default: 0]) }
    }

    private var tagOptions: [TagOption] {
        let bindingIndex = CalendarEventViewFilter.colorTagIndex(from: calendarStore.settings.colorTagBindings)
        var eventCountsByTag = calendarStore.calendarSnapshot.eventCountsByTagName
        for tag in bindingIndex.keys where eventCountsByTag[tag] == nil {
            eventCountsByTag[tag] = 0
        }
        if let visibleTagNames = state.visibleTagNames {
            for tag in visibleTagNames where eventCountsByTag[tag] == nil {
                eventCountsByTag[tag] = 0
            }
        }
        return eventCountsByTag.map { entry in
            TagOption(
                key: entry.key,
                title: "#\(entry.key)",
                count: entry.value,
                boundColorID: bindingIndex[entry.key]
            )
        }
        .sorted { lhs, rhs in
            if lhs.count != rhs.count { return lhs.count > rhs.count }
            return lhs.title.localizedCaseInsensitiveCompare(rhs.title) == .orderedAscending
        }
    }

    private var availableCalendarIDs: Set<CalendarListMirror.ID> {
        Set(calendarStore.calendarSnapshot.selectedCalendars.map(\.id))
    }

    private var availableColorIDs: Set<String> {
        Set(colorOptions.map(\.id))
    }

    private var availableTagNames: Set<String> {
        Set(tagOptions.map(\.key))
    }

    private func isCalendarVisible(_ id: CalendarListMirror.ID) -> Bool {
        state.visibleCalendarIDs?.contains(id) ?? true
    }

    private func isColorVisible(_ id: String) -> Bool {
        state.visibleColorIDs?.contains(id) ?? true
    }

    private func isTagVisible(_ key: String) -> Bool {
        state.visibleTagNames?.contains(key) ?? true
    }

    private func setCalendar(_ id: CalendarListMirror.ID, visible: Bool) {
        var visibleIDs = state.visibleCalendarIDs ?? availableCalendarIDs
        if visible {
            visibleIDs.insert(id)
        } else {
            visibleIDs.remove(id)
        }
        state.visibleCalendarIDs = visibleIDs == availableCalendarIDs ? nil : visibleIDs
    }

    private func setColor(_ id: String, visible: Bool) {
        var visibleIDs = state.visibleColorIDs ?? availableColorIDs
        if visible {
            visibleIDs.insert(id)
        } else {
            visibleIDs.remove(id)
        }
        state.visibleColorIDs = visibleIDs == availableColorIDs ? nil : visibleIDs
    }

    private func setTag(_ key: String, visible: Bool) {
        let normalized = CalendarEventViewFilter.normalizedTagName(key)
        var visibleNames = state.visibleTagNames ?? availableTagNames
        if visible {
            visibleNames.insert(normalized)
        } else {
            visibleNames.remove(normalized)
        }
        state.visibleTagNames = visibleNames == availableTagNames ? nil : visibleNames
    }
}
