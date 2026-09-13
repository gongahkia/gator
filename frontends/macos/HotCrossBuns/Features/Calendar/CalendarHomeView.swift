import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct CalendarHomeView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.routerPath) private var router
    @Environment(NetworkMonitor.self) private var networkMonitor
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(\.hcbAppBackgroundConfiguration) private var backgroundConfiguration
    @Environment(\.calendarEventViewFilter) private var calendarEventViewFilter
    @State private var selectedDate = Date()
    @SceneStorage("calendarGridMode") private var storedMode: String = CalendarGridMode.month.rawValue
    @State private var mode: CalendarGridMode = .month
    @State private var searchQuery: String = ""
    @State private var pendingCrossCalendarMove: CrossCalendarMoveRequest?
    @State private var importResultMessage: String?
    @State private var selectedEventIDs: Set<String> = []
    @State private var isGoToDateShown = false
    @State private var snoozeCustomTask: TaskMirror?
    @State private var pendingDeleteEvent: CalendarEventMirror?
    @State private var preparedAgendaSnapshot: CalendarAgendaDisplaySnapshot?
    @State private var agendaSnapshotBuildTask: Task<Void, Never>?
    @State private var agendaSnapshotPrewarmTask: Task<Void, Never>?
    @State private var isShareAvailabilityShown = false
    @State private var availabilityDraft = ShareAvailabilityDraft()
    @State private var availabilityMessage: String?
    @State private var availabilityToastMessage: String?
    @State private var availabilityToastIsWarning = false
    @State private var cachedAvailabilityHoldGroups: [AvailabilityHoldGroup] = []
    @State private var isWritingAvailabilityHolds = false
    @State private var activeModeTransition: HCBTransitionMeasurement?

    private var calendarStore: CalendarStore { model.calendarStore }

    var body: some View {
        VStack(spacing: 0) {
            CalendarDropChipsStrip(
                calendars: calendarStore.calendars,
                onDrop: { droppedEvent, destinationCalendarID in
                    Task {
                        await handleCrossCalendarDrop(droppedEvent, destinationCalendarID: destinationCalendarID)
                    }
                }
            )
            navigationBar
            if mode != .month {
                Divider()
            }
            contentArea
            .hcbSurface(.calendarGrid) // §6.11 — covers day/week/month/year/agenda subtree
        }
        .appBackground()
        .transaction { transaction in
            if case .syncing = calendarStore.syncState {
                transaction.animation = nil
            }
        }
        .focusedSceneValue(\.calendarCommandActions, calendarCommandActions)
        .onReceive(NotificationCenter.default.publisher(for: .hcbOpenShareAvailability)) { _ in
            openShareAvailability()
        }
        .onReceive(NotificationCenter.default.publisher(for: .hcbProfileNextCalendarMode)) { _ in
            selectNextProfileMode()
        }
        .onDrop(of: [.fileURL], isTargeted: nil) { providers in
            handleICSDrop(providers)
        }
        .alert(
            "ICS import",
            isPresented: Binding(
                get: { importResultMessage != nil },
                set: { if $0 == false { importResultMessage = nil } }
            )
        ) {
            Button("OK") { importResultMessage = nil }
        } message: {
            Text(importResultMessage ?? "")
        }
        .sheet(item: $snoozeCustomTask) { task in
            SnoozePickerSheet(task: task) { newDate in
                Task {
                    _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: newDate)
                }
            }
        }
        .confirmationDialog(
            "Move which occurrences?",
            isPresented: Binding(
                get: { pendingCrossCalendarMove != nil },
                set: { if $0 == false { pendingCrossCalendarMove = nil } }
            ),
            titleVisibility: .visible,
            presenting: pendingCrossCalendarMove
        ) { request in
            Button("This event only") {
                Task { await performCrossCalendarMove(request, scope: .thisOccurrence) }
            }
            Button("Every event in the series", role: .destructive) {
                Task { await performCrossCalendarMove(request, scope: .allInSeries) }
            }
            Button("Cancel", role: .cancel) {
                pendingCrossCalendarMove = nil
            }
        } message: { request in
            Text("\"\(request.eventSummary)\" is part of a recurring series. \"Every event in the series\" moves past occurrences too — not just upcoming ones. This is destructive and can't be undone.")
        }
        .confirmationDialog(
            "Delete this event?",
            isPresented: Binding(
                get: { pendingDeleteEvent != nil },
                set: { if $0 == false { pendingDeleteEvent = nil } }
            ),
            titleVisibility: .visible
        ) {
            if let event = pendingDeleteEvent {
                Button("Delete", role: .destructive) {
                    Task {
                        _ = await model.deleteEvent(event)
                        pendingDeleteEvent = nil
                    }
                }
            }
            Button("Cancel", role: .cancel) { pendingDeleteEvent = nil }
        } message: {
            if let event = pendingDeleteEvent {
                Text("Delete \"\(event.summary)\" from Google Calendar?")
            }
        }
        .onAppear {
            let restored = CalendarGridMode(rawValue: storedMode) ?? .month
            mode = visibleCalendarModes.contains(restored) ? restored : (visibleCalendarModes.first ?? .month)
        }
        .onChange(of: mode) { _, newValue in
            activeModeTransition?.scheduleSettled(
                after: .milliseconds(260),
                metadata: ["mode": newValue.rawValue]
            )
            storedMode = newValue.rawValue
        }
        .onChange(of: calendarStore.settings.hiddenCalendarViewModes) { _, _ in
            // if the user just hid the currently-selected mode, fall back to
            // the first still-visible one so the detail area doesn't render
            // a mode that's gone from the picker.
            if visibleCalendarModes.contains(mode) == false, let first = visibleCalendarModes.first {
                selectMode(first, source: "hiddenModeFallback")
            }
        }
        .onChange(of: calendarEventViewFilter.cacheKey) { _, _ in
            selectedEventIDs = selectedEventIDs.filter { eventID in
                calendarStore.event(id: eventID).map(calendarEventViewFilter.allows) ?? false
            }
        }
        .onChange(of: agendaSnapshotKey) { _, _ in
            if mode == .agenda { rebuildAgendaSnapshotIfNeeded() }
        }
        .onChange(of: isShareAvailabilityShown) { _, shown in
            if shown {
                Task { await refreshAvailabilityHoldGroups() }
            }
        }
        .onChange(of: calendarStore.calendarDisplayRevision) { _, _ in
            if isShareAvailabilityShown {
                Task { await refreshAvailabilityHoldGroups() }
            }
        }
        .onDisappear {
            agendaSnapshotBuildTask?.cancel()
            agendaSnapshotPrewarmTask?.cancel()
        }
    }

    @ViewBuilder
    private var contentArea: some View {
        Group {
            if calendarStore.account == nil && calendarStore.authState == .authenticating {
                restoringPrompt
            } else if calendarStore.account == nil {
                connectPrompt
            } else if calendarStore.calendarSnapshot.selectedCalendars.isEmpty {
                calendarsEmptyPrompt
            } else {
                activeCalendarSurface
            }
        }
    }

    private var activeCalendarSurface: some View {
        ZStack(alignment: .trailing) {
            ZStack(alignment: .bottom) {
                currentModeContent

                if selectedEventIDs.count >= 2 {
                    EventBulkActionBar(
                        selection: $selectedEventIDs,
                        events: selectedEvents
                    )
                    .hcbScaledPadding(.bottom, 16)
                    .transition(HCBMotion.transition(.move(edge: .bottom).combined(with: .opacity), reduceMotion: reduceMotion))
                }
            }

            if isShareAvailabilityShown {
                ShareAvailabilityPanel(
                    draft: $availabilityDraft,
                    calendars: writableCalendars,
                    holdGroups: cachedAvailabilityHoldGroups,
                    snippet: availabilitySnippet,
                    message: availabilityMessage,
                    supportsSelection: supportsAvailabilitySelection,
                    isWriting: isWritingAvailabilityHolds,
                    onClose: { isShareAvailabilityShown = false },
                    onCopy: copyAvailabilitySnippet,
                    onCreateHolds: createAvailabilityHolds,
                    onRemoveSlot: removeAvailabilitySlot,
                    onConfirmHold: { hold in
                        Task { await confirmAvailabilityHold(hold) }
                    },
                    onCancelGroup: { group in
                        Task { await cancelAvailabilityHoldGroup(group) }
                    },
                    onOpenHold: { hold in
                        router?.present(.editEvent(hold.id))
                    }
                )
                .transition(HCBMotion.transition(.move(edge: .trailing).combined(with: .opacity), reduceMotion: reduceMotion))
            }
        }
        .overlay {
            BulkResultToast(
                message: $availabilityToastMessage,
                isWarning: availabilityToastIsWarning,
                successTitle: "Availability",
                warningTitle: "Availability",
                successSymbol: "calendar.badge.plus",
                warningSymbol: "calendar.badge.exclamationmark"
            )
        }
        .animation(HCBMotion.animation(.easeOut(duration: 0.12), reduceMotion: reduceMotion), value: selectedEventIDs.count >= 2)
        .animation(HCBMotion.animation(.easeOut(duration: 0.16), reduceMotion: reduceMotion), value: isShareAvailabilityShown)
    }

    @ViewBuilder
    private var currentModeContent: some View {
        Group {
            switch mode {
            case .agenda:
                agendaContent
            case .day:
                DayGridView(
                    anchorDate: $selectedDate,
                    searchQuery: searchQuery,
                    availabilitySelection: availabilityGridSelection
                )
            case .multiDay:
                VStack(spacing: 0) {
                    multiDayStepperBar
                    WeekGridView(
                        anchorDate: $selectedDate,
                        searchQuery: searchQuery,
                        selectedEventIDs: $selectedEventIDs,
                        multiDayCount: calendarStore.settings.multiDayCount,
                        availabilitySelection: availabilityGridSelection
                    )
                }
            case .week:
                WeekGridView(
                    anchorDate: $selectedDate,
                    searchQuery: searchQuery,
                    selectedEventIDs: $selectedEventIDs,
                    availabilitySelection: availabilityGridSelection
                )
            case .month:
                MonthGridView(anchorDate: $selectedDate, searchQuery: searchQuery)
            case .year:
                YearGridView(anchorDate: $selectedDate, searchQuery: searchQuery, onPickDay: { day in
                    selectedDate = day
                    selectMode(.day, source: "year.pickDay")
                })
            }
        }
        .hcbTransitionFirstContent(
            activeModeTransition,
            metadata: ["mode": mode.rawValue]
        )
    }

    private var navigationBar: some View {
        HStack(spacing: 12) {
            Button { shift(by: -1) } label: {
                Image(systemName: "chevron.left").hcbFont(.body, weight: .semibold)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Previous \(mode.title.lowercased())")

            Button("Today") { selectedDate = Date() }
                .buttonStyle(.bordered)
                .controlSize(.small)
                .accessibilityLabel("Jump to today")

            Button { shift(by: 1) } label: {
                Image(systemName: "chevron.right").hcbFont(.body, weight: .semibold)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Next \(mode.title.lowercased())")

            if mode != .month {
                Button {
                    isGoToDateShown.toggle()
                } label: {
                    Text(periodTitle)
                        .hcbFont(.title3, weight: .semibold)
                        .foregroundStyle(AppColor.ink)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Jump to date")
                .popover(isPresented: $isGoToDateShown, arrowEdge: .bottom) {
                    GoToDateSheet(initialDate: selectedDate) { newDate in
                        selectedDate = newDate
                    }
                }
            }

            Spacer(minLength: 0)

            Button {
                openShareAvailability()
            } label: {
                Label("Share Availability", systemImage: "calendar.badge.clock")
                    .labelStyle(.iconOnly)
            }
            .buttonStyle(.bordered)
            .controlSize(.small)
            .help("Share availability")
            .accessibilityLabel("Share availability")
            .disabled(canOpenShareAvailability == false)

            calendarViewModeControl
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 10)
        .background {
            if usesReadableCalendarChrome {
                Rectangle()
                    .fill(.regularMaterial)
                    .overlay(AppColor.cream.opacity(0.24))
            }
        }
        .disabled(calendarStore.account == nil)
    }

    private var calendarViewModeControl: some View {
        ViewThatFits(in: .horizontal) {
            Picker("View", selection: modeBinding) {
                calendarViewModePickerOptions
            }
            .pickerStyle(.segmented)
            .fixedSize()

            Picker("View", selection: modeBinding) {
                calendarViewModePickerOptions
            }
            .pickerStyle(.menu)
            .labelsHidden()
            .fixedSize()
        }
        .help("Choose calendar view")
        .accessibilityLabel("Calendar view")
    }

    @ViewBuilder
    private var calendarViewModePickerOptions: some View {
        ForEach(visibleCalendarModes, id: \.self) { mode in
            Label(mode.title, systemImage: mode.systemImage).tag(mode)
        }
    }

    private var usesReadableCalendarChrome: Bool {
        backgroundConfiguration.customImagePath != nil || backgroundConfiguration.isTranslucent
    }

    private var selectedEvents: [CalendarEventMirror] {
        selectedEventIDs
            .compactMap { calendarStore.event(id: $0) }
            .filter(calendarEventViewFilter.allows)
            .sorted { lhs, rhs in
                if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
                return lhs.id < rhs.id
            }
    }

    private var writableCalendars: [CalendarListMirror] {
        let selectedIDs = calendarStore.calendarSnapshot.selectedCalendarIDs
        let writable = calendarStore.calendars.filter { calendar in
            (calendar.accessRole == "owner" || calendar.accessRole == "writer")
                && (selectedIDs.isEmpty || selectedIDs.contains(calendar.id))
        }
        if writable.isEmpty {
            return calendarStore.calendars.filter { $0.accessRole == "owner" || $0.accessRole == "writer" }
        }
        return writable
    }

    private var canOpenShareAvailability: Bool {
        calendarStore.account != nil && writableCalendars.isEmpty == false
    }

    private var supportsAvailabilitySelection: Bool {
        switch mode {
        case .day, .week, .multiDay:
            true
        case .agenda, .month, .year:
            false
        }
    }

    private var availabilityGridSelection: AvailabilityGridSelection? {
        guard isShareAvailabilityShown, supportsAvailabilitySelection else { return nil }
        return AvailabilityGridSelection(
            slots: availabilityDraft.slots,
            defaultDurationMinutes: availabilityDraft.durationMinutes,
            onSelect: addAvailabilitySlot,
            onReject: { availabilityMessage = $0 },
            isSlotAvailable: isAvailabilitySlotAvailable
        )
    }

    private var availabilitySnippet: String {
        AvailabilitySnippetFormatter.snippet(
            title: availabilityDraft.title,
            durationMinutes: availabilityDraft.durationMinutes,
            timeZoneID: availabilityDraft.timeZoneID,
            slots: availabilityDraft.slots
        )
    }

    // CalendarGridMode.allCases filtered by user-hidden set from Layout settings.
    // If the user hides every mode, allCases still returns something — the
    // AppModel.setCalendarViewModeHidden setter refuses to hide the last one.
    private var visibleCalendarModes: [CalendarGridMode] {
        CalendarGridMode.allCases.filter { calendarStore.settings.hiddenCalendarViewModes.contains($0.rawValue) == false }
    }

    private var modeBinding: Binding<CalendarGridMode> {
        Binding(
            get: { mode },
            set: { selectMode($0, source: "picker") }
        )
    }

    private var calendarCommandActions: CalendarCommandActions {
        let canNavigate = calendarStore.account != nil
        return CalendarCommandActions(
            previous: { shift(by: -1) },
            today: { selectedDate = Date() },
            next: { shift(by: 1) },
            jumpBack: { jumpLarge(by: -1) },
            jumpForward: { jumpLarge(by: 1) },
            goToDate: { isGoToDateShown = true },
            focusSearch: {
                NotificationCenter.default.post(name: .hcbFocusCalendarSearch, object: nil)
            },
            shareAvailability: { openShareAvailability() },
            showAgenda: { selectMode(.agenda) },
            showDay: { selectMode(.day) },
            showWeek: { selectMode(.week) },
            showMonth: { selectMode(.month) },
            showMultiDay: { selectMode(.multiDay) },
            showYear: { selectMode(.year) },
            canNavigate: canNavigate,
            canShareAvailability: canNavigate && canOpenShareAvailability,
            canShowAgenda: canNavigate && visibleCalendarModes.contains(.agenda),
            canShowDay: canNavigate && visibleCalendarModes.contains(.day),
            canShowWeek: canNavigate && visibleCalendarModes.contains(.week),
            canShowMonth: canNavigate && visibleCalendarModes.contains(.month),
            canShowMultiDay: canNavigate && visibleCalendarModes.contains(.multiDay),
            canShowYear: canNavigate && visibleCalendarModes.contains(.year)
        )
    }

    // Keyboard shortcuts route here so hidden modes no-op rather than forcing
    // the picker back onto a mode the user chose to remove.
    private func selectMode(_ target: CalendarGridMode) {
        selectMode(target, source: "command")
    }

    private func selectMode(_ target: CalendarGridMode, source: String) {
        guard visibleCalendarModes.contains(target) else { return }
        guard mode != target else { return }
        let transition = HCBTransitionProfiler.start(
            "calendarMode.\(mode.rawValue)->\(target.rawValue)",
            metadata: [
                "from": mode.rawValue,
                "source": source,
                "to": target.rawValue
            ]
        )
        activeModeTransition = transition
        mode = target
        Task { @MainActor in
            await Task.yield()
            transition?.markFirstContent(metadata: ["mode": target.rawValue])
        }
    }

    private func selectNextProfileMode() {
        let modes = visibleCalendarModes.isEmpty ? CalendarGridMode.allCases : visibleCalendarModes
        guard let index = modes.firstIndex(of: mode) else {
            selectMode(modes.first ?? .month, source: "profileScenario")
            return
        }
        let next = modes[modes.index(after: index) == modes.endIndex ? modes.startIndex : modes.index(after: index)]
        selectMode(next, source: "profileScenario")
    }

    private func openShareAvailability() {
        if availabilityDraft.calendarID == nil || writableCalendars.contains(where: { $0.id == availabilityDraft.calendarID }) == false {
            availabilityDraft.calendarID = writableCalendars.first?.id
        }
        if let calendarID = availabilityDraft.calendarID,
           let calendarTimeZone = calendarStore.calendars.first(where: { $0.id == calendarID })?.timeZoneID {
            availabilityDraft.timeZoneID = calendarTimeZone
        }
        isShareAvailabilityShown = true
        availabilityMessage = nil
        Task { await refreshAvailabilityHoldGroups() }
    }

    @MainActor
    private func refreshAvailabilityHoldGroups() async {
        cachedAvailabilityHoldGroups = await model.availabilityHoldGroupsFromQuery()
    }

    private func addAvailabilitySlot(_ slot: AvailabilitySlot) {
        availabilityDraft.slots = AvailabilitySlotResolver.normalized(availabilityDraft.slots + [slot])
        availabilityMessage = "Added \(availabilitySlotLabel(slot))."
    }

    private func removeAvailabilitySlot(_ slot: AvailabilitySlot) {
        availabilityDraft.slots.removeAll { $0.id == slot.id }
        availabilityMessage = nil
    }

    private func isAvailabilitySlotAvailable(_ slot: AvailabilitySlot) -> Bool {
        guard slot.endDate > slot.startDate else { return false }
        var calendarIDs = calendarStore.calendarSnapshot.selectedCalendarIDs.isEmpty
            ? Set(calendarStore.calendars.map(\.id))
            : calendarStore.calendarSnapshot.selectedCalendarIDs
        if let holdCalendarID = availabilityDraft.calendarID ?? writableCalendars.first?.id {
            calendarIDs.insert(holdCalendarID)
        }
        let calendar = Calendar.current
        let lastDay = calendar.startOfDay(for: slot.endDate)
        var cursor = calendar.startOfDay(for: slot.startDate)
        var seen: Set<CalendarEventMirror.ID> = []
        var candidates: [CalendarEventMirror] = []
        var steps = 0
        while cursor <= lastDay && steps < 366 {
            for eventID in calendarStore.eventIDs(on: cursor, calendar: calendar) where seen.insert(eventID).inserted {
                if let event = calendarStore.event(id: eventID) {
                    candidates.append(event)
                }
            }
            guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
            steps += 1
        }
        return AvailabilitySlotResolver.blockingEvents(
            for: slot,
            events: candidates,
            calendarIDs: calendarIDs
        ).isEmpty
    }

    private func copyAvailabilitySnippet() {
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(availabilitySnippet, forType: .string)
        availabilityMessage = "Copied availability."
    }

    private func createAvailabilityHolds() {
        guard let calendarID = availabilityDraft.calendarID ?? writableCalendars.first?.id else {
            availabilityMessage = "Choose a writable calendar."
            return
        }
        let slots = AvailabilitySlotResolver.normalized(availabilityDraft.slots)
        guard slots.isEmpty == false else {
            if supportsAvailabilitySelection {
                availabilityMessage = "Select at least one slot."
            } else {
                availabilityMessage = nil
                showAvailabilityToast("Switch to Day, Week, or Multi-Day, then select at least one slot.", isWarning: true)
            }
            return
        }
        isWritingAvailabilityHolds = true
        Task {
            let didCreate = await model.createAvailabilityHoldGroup(
                title: availabilityDraft.title,
                durationMinutes: availabilityDraft.durationMinutes,
                slots: slots,
                calendarID: calendarID,
                timeZoneID: availabilityDraft.timeZoneID
            )
            isWritingAvailabilityHolds = false
            if didCreate {
                availabilityDraft.slots = []
                availabilityMessage = "Created \(slots.count) hold\(slots.count == 1 ? "" : "s")."
            } else {
                availabilityMessage = calendarStore.lastMutationError ?? "Could not create holds."
            }
        }
    }

    private func confirmAvailabilityHold(_ hold: CalendarEventMirror) async {
        let didConfirm = await model.confirmAvailabilityHold(hold)
        availabilityMessage = didConfirm ? "Confirmed \(hold.summary)." : (calendarStore.lastMutationError ?? "Could not confirm hold.")
    }

    private func cancelAvailabilityHoldGroup(_ group: AvailabilityHoldGroup) async {
        let didCancel = await model.cancelAvailabilityHoldGroup(groupID: group.id)
        availabilityMessage = didCancel ? "Cancelled \(group.events.count) hold\(group.events.count == 1 ? "" : "s")." : (calendarStore.lastMutationError ?? "Could not cancel holds.")
    }

    private func showAvailabilityToast(_ message: String, isWarning: Bool = false) {
        availabilityToastIsWarning = isWarning
        availabilityToastMessage = message
    }

    private func availabilitySlotLabel(_ slot: AvailabilitySlot) -> String {
        let start = slot.startDate.formatted(.dateTime.weekday(.abbreviated).hour().minute())
        let end = slot.endDate.formatted(.dateTime.hour().minute())
        return "\(start)-\(end)"
    }

    private var periodTitle: String {
        let calendar = Calendar.current
        switch mode {
        case .agenda:
            return selectedDate.formatted(.dateTime.weekday(.wide).month(.wide).day())
        case .day:
            return selectedDate.formatted(.dateTime.weekday(.wide).month(.wide).day().year())
        case .multiDay:
            let count = max(2, min(7, calendarStore.settings.multiDayCount))
            let first = calendar.startOfDay(for: selectedDate)
            let last = calendar.date(byAdding: .day, value: count - 1, to: first) ?? first
            return "\(first.formatted(.dateTime.month(.abbreviated).day())) – \(last.formatted(.dateTime.month(.abbreviated).day().year()))"
        case .week:
            let days = CalendarGridLayout.weekDays(containing: selectedDate, calendar: calendar)
            let first = days.first ?? selectedDate
            let last = days.last ?? selectedDate
            let sameMonth = calendar.component(.month, from: first) == calendar.component(.month, from: last)
            if sameMonth {
                return "\(first.formatted(.dateTime.month(.wide))) \(calendar.component(.day, from: first))–\(calendar.component(.day, from: last)), \(calendar.component(.year, from: first))"
            }
            return "\(first.formatted(.dateTime.month(.abbreviated).day())) – \(last.formatted(.dateTime.month(.abbreviated).day().year()))"
        case .month:
            return selectedDate.formatted(.dateTime.month(.wide).year())
        case .year:
            return selectedDate.formatted(.dateTime.year())
        }
    }

    // §7.01 Phase D2 — ± stepper for Multi-Day view day count.
    private var multiDayStepperBar: some View {
        HStack(spacing: 12) {
            Button {
                var next = calendarStore.settings
                next.multiDayCount = max(2, next.multiDayCount - 1)
                model.updateSettings(next)
            } label: {
                Image(systemName: "minus.circle")
            }
            .buttonStyle(.plain)
            .disabled(calendarStore.settings.multiDayCount <= 2)
            .help("Fewer days")
            Text("\(calendarStore.settings.multiDayCount) days")
                .hcbFont(.caption, weight: .semibold)
                .monospacedDigit()
                .frame(minWidth: 50)
            Button {
                var next = calendarStore.settings
                next.multiDayCount = min(7, next.multiDayCount + 1)
                model.updateSettings(next)
            } label: {
                Image(systemName: "plus.circle")
            }
            .buttonStyle(.plain)
            .disabled(calendarStore.settings.multiDayCount >= 7)
            .help("More days")
            Spacer(minLength: 8)
        }
        .hcbScaledPadding(.horizontal, 12)
        .hcbScaledPadding(.vertical, 6)
    }

    @ViewBuilder
    private var connectPrompt: some View {
        ContentUnavailableView {
            Label("Not connected to Google", systemImage: "person.crop.circle.badge.plus")
        } description: {
            Text("Connect your Google account to see your calendars here.")
        } actions: {
            Button("Open Settings") {
                NotificationCenter.default.post(name: .hcbOpenSettingsWindow, object: nil)
            }
            .buttonStyle(.borderedProminent)
            .tint(AppColor.ember)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    @ViewBuilder
    private var restoringPrompt: some View {
        // Shown during the brief pre-load window where we haven't yet decided
        // if the user is signed in. Replaces the alarming "Not connected"
        // message that used to flash during cold launch.
        VStack(spacing: 12) {
            ProgressView()
                .controlSize(.large)
            Text("Connecting to Google…")
                .hcbFont(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    @ViewBuilder
    private var calendarsEmptyPrompt: some View {
        ContentUnavailableView {
            Label("No calendars selected", systemImage: "calendar.badge.exclamationmark")
        } description: {
            if calendarStore.calendars.isEmpty {
                if case .syncing = calendarStore.syncState {
                    Text("Loading calendars from Google…")
                } else {
                    Text("We haven't seen any calendars yet. Try Refresh, or check Settings → Calendars.")
                }
            } else {
                Text("Pick at least one calendar in Settings → Calendars to see events here.")
            }
        } actions: {
            if calendarStore.calendars.isEmpty {
                Button("Refresh") {
                    Task { await model.refreshNow() }
                }
                .buttonStyle(.borderedProminent)
                .tint(AppColor.ember)
            } else {
                Button("Open Settings") {
                    NotificationCenter.default.post(name: .hcbOpenSettingsWindow, object: nil)
                }
                .buttonStyle(.borderedProminent)
                .tint(AppColor.ember)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func handleICSDrop(_ providers: [NSItemProvider]) -> Bool {
        let calendarID = calendarStore.calendarSnapshot.selectedCalendars.first?.id ?? calendarStore.calendars.first?.id
        guard let calendarID else {
            importResultMessage = "No calendar selected to import events into."
            return true
        }
        Task {
            var imported = 0
            var failed = 0
            var duplicates = 0
            for provider in providers {
                guard let url = await loadFileURL(from: provider) else { continue }
                guard url.pathExtension.lowercased() == "ics" else { continue }
                guard let contents = try? String(contentsOf: url, encoding: .utf8) else {
                    failed += 1
                    continue
                }
                let drafts = ICSImporter.parse(contents)
                for draft in drafts {
                    if isDuplicateOfExistingEvent(draft, targetCalendarID: calendarID) {
                        duplicates += 1
                        continue
                    }
                    let didCreate = await model.createEvent(
                        summary: draft.summary,
                        details: draft.description,
                        startDate: draft.startDate,
                        endDate: draft.endDate,
                        isAllDay: draft.isAllDay,
                        reminderMinutes: nil,
                        calendarID: calendarID,
                        location: draft.location,
                        recurrence: draft.recurrence,
                        startTimeZoneID: draft.startTimeZoneID,
                        endTimeZoneID: draft.endTimeZoneID
                    )
                    if didCreate { imported += 1 } else { failed += 1 }
                }
            }
            var parts: [String] = []
            if imported > 0 {
                parts.append("Imported \(imported) event\(imported == 1 ? "" : "s")")
            }
            if duplicates > 0 {
                parts.append("skipped \(duplicates) duplicate\(duplicates == 1 ? "" : "s")")
            }
            if failed > 0 {
                parts.append("\(failed) failed")
            }
            if parts.isEmpty {
                importResultMessage = "No events found in the dropped files."
            } else {
                importResultMessage = parts.joined(separator: ", ") + "."
            }
        }
        return true
    }

    // Dropping the same .ics twice (the user re-exports weekly and
    // drags the file for a backup, then drags again for another backup)
    // would otherwise produce exact duplicates in Google Calendar —
    // there's no canonical ID to key on since Google assigns a fresh
    // one per insertEvent. Match against existing events by summary,
    // start time within a minute, and isAllDay alignment in the
    // destination calendar.
    private func isDuplicateOfExistingEvent(
        _ draft: ICSEventDraft,
        targetCalendarID: CalendarListMirror.ID
    ) -> Bool {
        let tolerance: TimeInterval = 60
        let calendar = Calendar.current
        let candidateDays = [
            calendar.date(byAdding: .day, value: -1, to: draft.startDate),
            draft.startDate,
            calendar.date(byAdding: .day, value: 1, to: draft.startDate)
        ].compactMap { $0 }
        var seen: Set<CalendarEventMirror.ID> = []
        for day in candidateDays {
            for eventID in calendarStore.eventIDs(on: day, calendar: calendar) where seen.insert(eventID).inserted {
                guard let existing = calendarStore.event(id: eventID) else { continue }
                guard existing.calendarID == targetCalendarID else { continue }
                guard existing.status != .cancelled else { continue }
                guard existing.isAllDay == draft.isAllDay else { continue }
                guard existing.summary.caseInsensitiveCompare(draft.summary) == .orderedSame else { continue }
                if abs(existing.startDate.timeIntervalSince(draft.startDate)) < tolerance {
                    return true
                }
            }
        }
        return false
    }

    private func loadFileURL(from provider: NSItemProvider) async -> URL? {
        await withCheckedContinuation { continuation in
            _ = provider.loadObject(ofClass: URL.self) { url, _ in
                continuation.resume(returning: url)
            }
        }
    }

    private func handleCrossCalendarDrop(_ dropped: DraggedEvent, destinationCalendarID: CalendarListMirror.ID) async {
        guard destinationCalendarID != dropped.calendarID else { return }
        guard let event = calendarStore.event(id: dropped.eventID) else { return }
        let summary = event.summary
        let isRecurring = CalendarEventInstance.isRecurring(event)
        if isRecurring {
            // Recurring cross-calendar moves need scope — this occurrence vs.
            // the whole series. Pose the question and defer until the user
            // picks one.
            pendingCrossCalendarMove = CrossCalendarMoveRequest(
                eventID: event.id,
                destinationCalendarID: destinationCalendarID,
                eventSummary: summary
            )
            return
        }
        _ = await model.updateEvent(
            event,
            summary: event.summary,
            details: event.details,
            startDate: event.startDate,
            endDate: event.isAllDay
                ? (Calendar.current.date(byAdding: .day, value: -1, to: event.endDate) ?? event.endDate)
                : event.endDate,
            isAllDay: event.isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: destinationCalendarID,
            location: event.location,
            recurrence: event.recurrence,
            attendeeEmails: event.attendeeEmails,
            notifyGuests: false,
            scope: .thisOccurrence,
            colorId: event.colorId
        )
    }

    private func performCrossCalendarMove(_ request: CrossCalendarMoveRequest, scope: AppModel.RecurringEventScope) async {
        pendingCrossCalendarMove = nil
        guard let event = calendarStore.event(id: request.eventID) else { return }
        _ = await model.updateEvent(
            event,
            summary: event.summary,
            details: event.details,
            startDate: event.startDate,
            endDate: event.isAllDay
                ? (Calendar.current.date(byAdding: .day, value: -1, to: event.endDate) ?? event.endDate)
                : event.endDate,
            isAllDay: event.isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: request.destinationCalendarID,
            location: event.location,
            recurrence: event.recurrence,
            attendeeEmails: event.attendeeEmails,
            notifyGuests: false,
            scope: scope,
            colorId: event.colorId
        )
    }

    private func shift(by direction: Int) {
        let calendar = Calendar.current
        switch mode {
        case .agenda, .day:
            selectedDate = calendar.date(byAdding: .day, value: direction, to: selectedDate) ?? selectedDate
        case .multiDay:
            let step = max(1, calendarStore.settings.multiDayCount)
            selectedDate = calendar.date(byAdding: .day, value: direction * step, to: selectedDate) ?? selectedDate
        case .week:
            selectedDate = calendar.date(byAdding: .weekOfYear, value: direction, to: selectedDate) ?? selectedDate
        case .month:
            selectedDate = calendar.date(byAdding: .month, value: direction, to: selectedDate) ?? selectedDate
        case .year:
            selectedDate = calendar.date(byAdding: .year, value: direction, to: selectedDate) ?? selectedDate
        }
    }

    private func jumpLarge(by direction: Int) {
        let calendar = Calendar.current
        switch mode {
        case .agenda, .day:
            selectedDate = calendar.date(byAdding: .weekOfYear, value: direction, to: selectedDate) ?? selectedDate
        case .multiDay:
            selectedDate = calendar.date(byAdding: .weekOfYear, value: direction, to: selectedDate) ?? selectedDate
        case .week:
            selectedDate = calendar.date(byAdding: .month, value: direction, to: selectedDate) ?? selectedDate
        case .month:
            selectedDate = calendar.date(byAdding: .year, value: direction, to: selectedDate) ?? selectedDate
        case .year:
            selectedDate = calendar.date(byAdding: .year, value: direction * 5, to: selectedDate) ?? selectedDate
        }
    }

    // §7.01 Phase D4 — TickTick-style flat chronological agenda. Date-header
    // sections, events (time-sorted) and tasks (due that day) interleaved.
    // Range: 14 days starting at selectedDate. DatePicker at top scrolls range.
    private var agendaContent: some View {
        Group {
            if let snapshot = preparedAgendaSnapshot, snapshot.key == agendaSnapshotKey, calendarStore.isRebuildingDerivedSnapshots == false {
                List {
                    Section {
                        DatePicker("Agenda starts", selection: $selectedDate, displayedComponents: [.date])
                    }
                    ForEach(snapshot.days) { daySnapshot in
                        if daySnapshot.isEmpty == false {
                            Section {
                                ForEach(daySnapshot.events) { event in
                                    Button {
                                        router?.present(.editEvent(event.id))
                                    } label: {
                                        EventListRow(event: event, accentColor: calendarColor(for: event, in: snapshot))
                                    }
                                    .buttonStyle(.plain)
                                    .contextMenu {
                                        EventContextMenu(
                                            event: event,
                                            onOpen: { router?.present(.editEvent(event.id)) },
                                            onConvertToTask: { router?.present(.convertEventToTask(event.id)) },
                                            onConvertToNote: { router?.present(.convertEventToNote(event.id)) },
                                            onDelete: { pendingDeleteEvent = event }
                                        )
                                    }
                                }
                                ForEach(daySnapshot.tasks) { task in
                                    agendaTaskRow(task, snapshot: snapshot)
                                }
                            } header: {
                                agendaDateHeader(daySnapshot.date)
                            }
                        }
                    }
                    if snapshot.isEmpty {
                        Section {
                            Text("Nothing scheduled in the next \(snapshot.days.count) days.")
                                .foregroundStyle(.secondary)
                        }
                    }
                }
            } else {
                PreparedSnapshotOverlay(
                    title: "Preparing agenda...",
                    message: "Collecting upcoming events and tasks before enabling interactions."
                )
                .onAppear { rebuildAgendaSnapshotIfNeeded() }
            }
        }
    }

    private func agendaDays(count: Int = 14) -> [Date] {
        agendaDays(starting: selectedDate, count: count)
    }

    private func agendaDays(starting date: Date, count: Int = 14) -> [Date] {
        let cal = Calendar.current
        let start = cal.startOfDay(for: date)
        return (0..<count).compactMap { cal.date(byAdding: .day, value: $0, to: start) }
    }

    private func agendaEventsByDay(for range: [Date]) -> [Date: [CalendarEventMirror]] {
        // Use the prebuilt calendarStore day IDs (rebuildSnapshots already walks
        // the full event corpus once) instead of re-iterating on every body
        // eval. Apply calendar-selection filter per lookup; sort per day.
        guard range.isEmpty == false else { return [:] }
        let calendar = Calendar.current
        let selectedIDs = calendarStore.calendarSnapshot.selectedCalendarIDs
        var bucket: [Date: [CalendarEventMirror]] = [:]
        for day in range {
            let eventIDs = calendarStore.eventIDs(on: day, calendar: calendar)
            guard eventIDs.isEmpty == false else { continue }
            let entries = eventIDs.compactMap { calendarStore.event(id: $0) }
            let q = searchQuery.trimmingCharacters(in: .whitespacesAndNewlines)
            let filtered = entries.filter { event in
                selectedIDs.contains(event.calendarID)
                    && calendarEventViewFilter.allows(event)
                    && (
                        q.isEmpty
                        || event.summary.localizedCaseInsensitiveContains(q)
                        || event.details.localizedCaseInsensitiveContains(q)
                        || event.location.localizedCaseInsensitiveContains(q)
                    )
            }
            guard filtered.isEmpty == false else { continue }
            bucket[day] = filtered.sorted { lhs, rhs in
                if lhs.isAllDay != rhs.isAllDay { return lhs.isAllDay && rhs.isAllDay == false }
                return lhs.startDate < rhs.startDate
            }
        }
        return bucket
    }

    private func agendaTasksByDay(for range: [Date]) -> [Date: [TaskMirror]] {
        // Use calendarStore due-date IDs; rebuildSnapshots already walks tasks
        // once and applies the completed-item calendar setting. We only need
        // to filter to the visible task-list set and sort alphabetically per day.
        let calendar = Calendar.current
        let visibleLists = calendarStore.visibleTaskListIDs
        var bucket: [Date: [TaskMirror]] = [:]
        for day in range {
            let taskIDs = calendarStore.taskIDs(dueOn: day, calendar: calendar)
            guard taskIDs.isEmpty == false else { continue }
            let entries = taskIDs.compactMap { calendarStore.task(id: $0) }
            let filtered = entries.filter { visibleLists.contains($0.taskListID) }
            guard filtered.isEmpty == false else { continue }
            bucket[day] = filtered.sorted { $0.title.localizedCaseInsensitiveCompare($1.title) == .orderedAscending }
        }
        return bucket
    }

    private func agendaDateHeader(_ day: Date) -> some View {
        let cal = Calendar.current
        let isToday = cal.isDateInToday(day)
        return HStack(spacing: 10) {
            Text(day.formatted(.dateTime.weekday(.wide).month(.abbreviated).day()))
                .hcbFont(.subheadline, weight: .semibold)
                .foregroundStyle(isToday ? AppColor.ember : AppColor.ink)
            if isToday {
                Text("Today")
                    .hcbFont(.caption2, weight: .semibold)
                    .hcbScaledPadding(.horizontal, 6)
                    .hcbScaledPadding(.vertical, 2)
                    .background(Capsule().fill(AppColor.ember.opacity(0.18)))
                    .foregroundStyle(AppColor.ember)
            }
            Spacer(minLength: 0)
        }
    }

    private func agendaTaskRow(_ task: TaskMirror, snapshot: CalendarAgendaDisplaySnapshot) -> some View {
        let metadata = snapshot.taskMetadataByID[task.id]
        return HStack(spacing: 8) {
            CalendarTaskCheckbox(task: task, size: 14)
            Button {
                router?.present(.editTask(task.id))
            } label: {
                HStack(spacing: 8) {
                    Text(metadata?.title ?? task.title)
                        .hcbFont(.subheadline)
                        .strikethrough(task.isCompleted)
                        .foregroundStyle(AppColor.ink)
                    let listTitle = metadata?.listTitle ?? calendarStore.taskListTitle(for: task.taskListID, fallback: "")
                    if listTitle.isEmpty == false {
                        Text(listTitle)
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                    }
                    Spacer(minLength: 0)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
        }
        .opacity(metadata?.opacity ?? (task.isCompleted ? 0.6 : 1.0))
        .accessibilityLabel(metadata?.accessibilityLabel ?? task.title)
        .contextMenu {
            TaskContextMenu(
                task: task,
                onOpen: { router?.present(.editTask(task.id)) },
                onCustomSnooze: { snoozeCustomTask = task },
                onConvertToEvent: { router?.present(.convertTaskToEvent(task.id)) },
                onConvertToTaskOrNote: {
                    if task.dueDate == nil {
                        router?.present(.convertNoteToTask(task.id))
                    } else {
                        router?.present(.convertTaskToNote(task.id))
                    }
                },
                onDelete: {
                    Task { _ = await model.deleteTask(task) }
                }
            )
        }
    }

    private func events(on date: Date) -> [CalendarEventMirror] {
        let calendar = Calendar.current
        let startOfDay = calendar.startOfDay(for: date)
        let endOfDay = calendar.date(byAdding: .day, value: 1, to: startOfDay) ?? startOfDay
        let selectedCalendarIDs = calendarStore.calendarSnapshot.selectedCalendarIDs

        return calendarStore.eventIDs(on: startOfDay, calendar: calendar)
            .compactMap { calendarStore.event(id: $0) }
            .filter { event in
                (calendarStore.settings.showCompletedItemsInCalendar || event.status != .cancelled)
                    && selectedCalendarIDs.contains(event.calendarID)
                    && calendarEventViewFilter.allows(event)
                    && event.startDate < endOfDay
                    && event.endDate > startOfDay
            }
            .sorted { lhs, rhs in
                lhs.startDate < rhs.startDate
            }
    }

    private func calendarColor(for event: CalendarEventMirror) -> Color {
        if let hex = CalendarEventColor.from(colorId: event.colorId).hex {
            return Color(hex: hex)
        }
        guard let hex = calendarStore.calendarSnapshot.calendarColorHexByID[event.calendarID] else {
            return AppColor.blue
        }
        return Color(hex: hex)
    }

    private var agendaSnapshotKey: PreparedSnapshotKey {
        agendaSnapshotKey(starting: selectedDate)
    }

    private func agendaSnapshotKey(starting date: Date) -> PreparedSnapshotKey {
        let days = agendaDays(starting: date)
        return PreparedSnapshotKeys.calendar(
            mode: .agenda,
            dataRevision: calendarStore.calendarDisplayRevisionKey(for: days, calendar: .current),
            selectedCalendarIDs: calendarStore.calendarSnapshot.selectedCalendarIDs,
            visibleTaskListIDs: calendarStore.visibleTaskListIDs,
            filterKey: calendarEventViewFilter.cacheKey,
            searchQuery: searchQuery,
            rangeKey: PreparedSnapshotKeys.dateRangeKey(days),
            settings: calendarStore.settings
        )
    }

    private func rebuildAgendaSnapshotIfNeeded() {
        let key = agendaSnapshotKey
        guard preparedAgendaSnapshot?.key != key else { return }
        if let snapshot = calendarStore.cachedCalendarAgendaSnapshot(for: key) {
            preparedAgendaSnapshot = snapshot
            prewarmAdjacentAgendaSnapshots(around: selectedDate)
            return
        }
        agendaSnapshotBuildTask?.cancel()
        agendaSnapshotBuildTask = Task { @MainActor in
            let started = HCBPerformanceTelemetry.timestamp()
            let input = await model.calendarDisplayInputFromQuery(
                key: key,
                kind: .agenda,
                anchorDate: selectedDate,
                eventViewFilter: calendarEventViewFilter,
                searchQuery: searchQuery,
                referenceDate: Date(),
                calendar: .current
            )
            let snapshot = await Task.detached(priority: .utility) {
                CalendarDisplaySnapshotBuilder.agendaSnapshot(input)
            }.value
            guard Task.isCancelled == false, snapshot.key == agendaSnapshotKey else { return }
            calendarStore.storeCalendarAgendaSnapshot(snapshot)
            preparedAgendaSnapshot = snapshot
            prewarmAdjacentAgendaSnapshots(around: selectedDate)
            HCBPerformanceTelemetry.debug(
                "calendar agenda snapshot built",
                metadata: [
                    "buildMs": HCBPerformanceTelemetry.elapsedMilliseconds(since: started),
                    "days": "\(snapshot.days.count)",
                    "events": "\(snapshot.eventMetadataByID.count)",
                    "tasks": "\(snapshot.taskMetadataByID.count)"
                ]
            )
        }
    }

    private func prewarmAdjacentAgendaSnapshots(around date: Date) {
        guard calendarStore.settings.performanceMode == .snappy else { return }
        let cal = Calendar.current
        let start = cal.startOfDay(for: date)
        let adjacentStarts = [-14, 14].compactMap { cal.date(byAdding: .day, value: $0, to: start) }
        guard adjacentStarts.isEmpty == false else { return }
        agendaSnapshotPrewarmTask?.cancel()
        agendaSnapshotPrewarmTask = Task { @MainActor in
            for prewarmStart in adjacentStarts {
                let key = agendaSnapshotKey(starting: prewarmStart)
                if calendarStore.cachedCalendarAgendaSnapshot(for: key) != nil { continue }
                let input = await model.calendarDisplayInputFromQuery(
                    key: key,
                    kind: .agenda,
                    anchorDate: prewarmStart,
                    eventViewFilter: calendarEventViewFilter,
                    searchQuery: searchQuery,
                    referenceDate: Date(),
                    calendar: cal
                )
                let snapshot = await Task.detached(priority: .utility) {
                    CalendarDisplaySnapshotBuilder.agendaSnapshot(input)
                }.value
                guard Task.isCancelled == false, snapshot.key == key else { return }
                calendarStore.storeCalendarAgendaSnapshot(snapshot)
            }
        }
    }

    private func calendarColor(for event: CalendarEventMirror, in snapshot: CalendarAgendaDisplaySnapshot) -> Color {
        guard let hex = snapshot.eventMetadataByID[event.id]?.colorHex else {
            return calendarColor(for: event)
        }
        return Color(hex: hex)
    }
}

private struct ShareAvailabilityDraft: Equatable {
    var title: String = "Meeting"
    var durationMinutes: Int = 30
    var timeZoneID: String = TimezoneSupport.currentIdentifier
    var calendarID: CalendarListMirror.ID?
    var slots: [AvailabilitySlot] = []
}

private struct ShareAvailabilityPanel: View {
    @Binding var draft: ShareAvailabilityDraft
    let calendars: [CalendarListMirror]
    let holdGroups: [AvailabilityHoldGroup]
    let snippet: String
    let message: String?
    let supportsSelection: Bool
    let isWriting: Bool
    let onClose: () -> Void
    let onCopy: () -> Void
    let onCreateHolds: () -> Void
    let onRemoveSlot: (AvailabilitySlot) -> Void
    let onConfirmHold: (CalendarEventMirror) -> Void
    let onCancelGroup: (AvailabilityHoldGroup) -> Void
    let onOpenHold: (CalendarEventMirror) -> Void

    private let durations = [15, 30, 45, 60, 90, 120]

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            header
            controls
            Divider()
            selectedSlotsSection
            snippetSection
            Divider()
            holdGroupsSection
            Spacer(minLength: 0)
        }
        .hcbScaledPadding(16)
        .frame(width: 360)
        .frame(maxHeight: .infinity)
        .background(.regularMaterial)
        .overlay(alignment: .leading) {
            Rectangle()
                .fill(AppColor.cardStroke)
                .frame(width: 1)
        }
    }

    private var header: some View {
        HStack(spacing: 10) {
            Label("Share Availability", systemImage: "calendar.badge.clock")
                .hcbFont(.headline, weight: .semibold)
            Spacer(minLength: 0)
            Button(action: onClose) {
                Image(systemName: "xmark")
            }
            .buttonStyle(.plain)
            .keyboardShortcut(.cancelAction)
            .help("Close Share Availability")
            .accessibilityLabel("Close Share Availability")
        }
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: 10) {
            TextField("Meeting title", text: $draft.title)
                .textFieldStyle(.roundedBorder)

            HStack(spacing: 10) {
                Picker("Duration", selection: $draft.durationMinutes) {
                    ForEach(durations, id: \.self) { minutes in
                        Text("\(minutes)m").tag(minutes)
                    }
                }
                .pickerStyle(.menu)

                Picker("Calendar", selection: calendarSelectionBinding) {
                    ForEach(calendars) { calendar in
                        Text(calendar.summary).tag(Optional(calendar.id))
                    }
                }
                .pickerStyle(.menu)
                .disabled(calendars.isEmpty)
            }

            Picker("Timezone", selection: $draft.timeZoneID) {
                ForEach(TimezoneSupport.pickerIdentifiers, id: \.self) { identifier in
                    Text(TimezoneSupport.displayName(for: identifier)).tag(identifier)
                }
            }
            .pickerStyle(.menu)

            if let message {
                Text(message)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }

            if supportsSelection == false {
                Label("Switch to Day, Week, or Multi-Day to select slots.", systemImage: "calendar")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var selectedSlotsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Selected Slots")
                    .hcbFont(.subheadline, weight: .semibold)
                Spacer(minLength: 0)
                Text("\(draft.slots.count) selected")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
            }

            if draft.slots.isEmpty {
                Text(supportsSelection ? "No slots selected." : "Switch views to select slots.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                VStack(spacing: 6) {
                    ForEach(AvailabilitySlotResolver.normalized(draft.slots)) { slot in
                        HStack(spacing: 8) {
                            Image(systemName: "clock")
                                .foregroundStyle(.secondary)
                            Text(slotLabel(slot))
                                .hcbFont(.caption)
                                .lineLimit(1)
                            Spacer(minLength: 0)
                            Button {
                                onRemoveSlot(slot)
                            } label: {
                                Image(systemName: "minus.circle")
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel("Remove slot")
                        }
                        .hcbScaledPadding(.horizontal, 8)
                        .hcbScaledPadding(.vertical, 6)
                        .background(RoundedRectangle(cornerRadius: 7).fill(AppColor.cardSurface))
                    }
                }
            }

            Button {
                onCreateHolds()
            } label: {
                Label(isWriting ? "Creating" : "Create Holds", systemImage: "calendar.badge.plus")
            }
            .buttonStyle(.borderedProminent)
            .tint(AppColor.ember)
            .disabled(isWriting)
        }
    }

    private var snippetSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Snippet")
                    .hcbFont(.subheadline, weight: .semibold)
                Spacer(minLength: 0)
                Button {
                    onCopy()
                } label: {
                    Label("Copy", systemImage: "doc.on.doc")
                }
                .buttonStyle(.bordered)
                .controlSize(.small)
            }

            ScrollView {
                Text(snippet)
                    .font(.system(.caption, design: .monospaced))
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .textSelection(.enabled)
                    .hcbScaledPadding(8)
            }
            .frame(height: 116)
            .background(RoundedRectangle(cornerRadius: 7).fill(AppColor.cardSurface))
            .overlay(
                RoundedRectangle(cornerRadius: 7)
                    .strokeBorder(AppColor.cardStroke, lineWidth: 1)
            )
        }
    }

    private var holdGroupsSection: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text("Pending Holds")
                    .hcbFont(.subheadline, weight: .semibold)
                Spacer(minLength: 0)
                Text("\(holdGroups.count)")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
            }

            if holdGroups.isEmpty {
                Text("No pending holds.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                ScrollView {
                    VStack(spacing: 10) {
                        ForEach(holdGroups) { group in
                            holdGroupCard(group)
                        }
                    }
                }
            }
        }
    }

    private func holdGroupCard(_ group: AvailabilityHoldGroup) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text(group.metadata.title)
                        .hcbFont(.caption, weight: .semibold)
                        .lineLimit(1)
                    Text("\(group.events.count) hold\(group.events.count == 1 ? "" : "s")")
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 0)
                Button {
                    onCancelGroup(group)
                } label: {
                    Image(systemName: "trash")
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Cancel hold group")
            }

            ForEach(group.events) { event in
                HStack(spacing: 8) {
                    Button {
                        onOpenHold(event)
                    } label: {
                        Text(slotLabel(AvailabilitySlot(startDate: event.startDate, endDate: event.endDate)))
                            .hcbFont(.caption)
                            .lineLimit(1)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .buttonStyle(.plain)

                    Button {
                        onConfirmHold(event)
                    } label: {
                        Image(systemName: "checkmark.circle")
                    }
                    .buttonStyle(.plain)
                    .disabled(OptimisticID.isPending(event.id))
                    .accessibilityLabel("Confirm hold")
                }
            }
        }
        .hcbScaledPadding(10)
        .background(RoundedRectangle(cornerRadius: 8).fill(AppColor.cardSurface))
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(AppColor.cardStroke, lineWidth: 1)
        )
    }

    private var calendarSelectionBinding: Binding<CalendarListMirror.ID?> {
        Binding(
            get: {
                if let id = draft.calendarID, calendars.contains(where: { $0.id == id }) {
                    return id
                }
                return calendars.first?.id
            },
            set: { newValue in
                draft.calendarID = newValue
                if let newValue,
                   let timeZoneID = calendars.first(where: { $0.id == newValue })?.timeZoneID {
                    draft.timeZoneID = timeZoneID
                }
            }
        )
    }

    private func slotLabel(_ slot: AvailabilitySlot) -> String {
        let dateFormatter = DateFormatter()
        dateFormatter.timeZone = TimezoneSupport.timeZone(for: draft.timeZoneID)
        dateFormatter.dateFormat = "EEE, MMM d"
        let timeFormatter = DateFormatter()
        timeFormatter.timeZone = TimezoneSupport.timeZone(for: draft.timeZoneID)
        timeFormatter.dateStyle = .none
        timeFormatter.timeStyle = .short
        return "\(dateFormatter.string(from: slot.startDate)) \(timeFormatter.string(from: slot.startDate))-\(timeFormatter.string(from: slot.endDate))"
    }
}

struct EventRowView: View {
    let event: CalendarEventMirror
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            EventListRow(event: event)
                .cardSurface(cornerRadius: 18)
        }
        .buttonStyle(.plain)
        .accessibilityLabel(event.summary)
    }
}

private struct EventListRow: View {
    let event: CalendarEventMirror
    var accentColor: Color = AppColor.blue

    var body: some View {
        HStack(spacing: 12) {
            RoundedRectangle(cornerRadius: 6)
                .fill(accentColor)
                .hcbScaledFrame(width: 6)
            VStack(alignment: .leading, spacing: 5) {
                Text(event.summary)
                    .hcbFont(.headline)
                    .foregroundStyle(AppColor.ink)
                Text(timeRange)
                    .hcbFont(.subheadline, weight: .medium)
                    .foregroundStyle(.secondary)
                if !event.details.isEmpty {
                    Text(event.details)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(2)
                }
            }
            Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
    }

    private var timeRange: String {
        if event.isAllDay {
            return event.startDate.formatted(date: .abbreviated, time: .omitted) + " - All day"
        }
        return "\(event.startDate.formatted(date: .abbreviated, time: .shortened)) - \(event.endDate.formatted(date: .omitted, time: .shortened))"
    }
}

private struct CalendarBadgeRow: View {
    let calendar: CalendarListMirror

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(Color(hex: calendar.colorHex))
                .hcbScaledFrame(width: 12, height: 12)
            Text(calendar.summary)
                .hcbFont(.headline)
            Spacer()
            Text(calendar.accessRole)
                .hcbFont(.caption, weight: .medium)
                .foregroundStyle(.secondary)
        }
    }
}

struct AddEventSheet: View {
    @Environment(\.dismiss) private var dismiss
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(AppModel.self) private var model
    @Environment(\.routerPath) private var router
    @State private var selectedCalendarID: CalendarListMirror.ID?
    @State private var summary = ""
    @State private var details = ""
    @State private var location = ""
    @State private var startDate: Date
    @State private var endDate: Date
    @State private var isAllDay: Bool
    @State private var reminderOption: EventReminderOption = .preset(15)
    @State private var recurrenceRule: RecurrenceRule?
    @State private var attendees: [String] = []
    @State private var attendeeDraft: String = ""
    @State private var notifyGuests: Bool = false
    @State private var addGoogleMeet: Bool = false
    @State private var eventColor: CalendarEventColor = .defaultColor
    @State private var startTimeZoneID: String
    @State private var endTimeZoneID: String
    @State private var usesSeparateEndTimeZone: Bool
    @State private var userManuallyPickedTimeZone: Bool = false
    @State private var isSaving = false
    @State private var quickCreateText = ""
    @State private var parsedPreview: ParsedQuickAddEvent?
    // Non-nil when the sheet is editing an existing event. Editing mode
    // hides the Quick Create block, flips the title + primary button, and
    // dispatches updateEvent instead of createEvent.
    @State private var editingEvent: CalendarEventMirror?
    // Editing-state flag: true in create mode; false when opened on an
    // existing event (view-only). The view-only pass disables every field
    // and swaps the primary toolbar button to "Edit" — clicking Edit flips
    // this true and the form becomes live. Clicking Open on the hover
    // preview now lands users here first, not in a destructive edit.
    @State private var isEditing: Bool
    // Recurring-scope picker state (shown only when saving a recurring event
    // in edit mode). Mirrors the legacy EditEventSheet behaviour.
    @State private var pendingRecurringScope = false
    // Save-as-template state (edit-mode overflow menu).
    @State private var isNamingTemplate = false
    @State private var templateName: String = ""
    // Delete confirmation state for the view-only toolbar Delete button.
    // Simple single confirm for one-off events; recurring events go through
    // a scope picker in the same flow so users can choose occurrence vs
    // series without re-opening the overflow menu.
    @State private var isConfirmingDelete = false
    @State private var pendingRecurringDeleteScope = false
    // Birthday mode mirrors QuickCreatePopover's third creation type. When
    // true: hide attendees / location / Meet / reminder / recurrence /
    // color blocks, force isAllDay + single-day + RRULE:FREQ=YEARLY on
    // save. Only togglable in create mode (editing keeps the sheet's
    // shape so users editing existing birthday events still see all
    // fields if they need to change calendar/notes).
    @State private var isBirthdayMode: Bool = false

    private var calendarStore: CalendarStore { model.calendarStore }

    init(prefilledStart: Date? = nil, prefilledIsAllDay: Bool = false, prefilledEnd: Date? = nil) {
        let start = prefilledStart ?? Date()
        let defaultEnd = prefilledIsAllDay ? start : start.addingTimeInterval(3600)
        _startDate = State(initialValue: start)
        _endDate = State(initialValue: prefilledEnd ?? defaultEnd)
        _isAllDay = State(initialValue: prefilledIsAllDay)
        _startTimeZoneID = State(initialValue: TimezoneSupport.currentIdentifier)
        _endTimeZoneID = State(initialValue: TimezoneSupport.currentIdentifier)
        _usesSeparateEndTimeZone = State(initialValue: false)
        _isEditing = State(initialValue: true) // create mode is always editable
    }

    // Edit-mode init: prefill every field from `existingEvent` so the same
    // "New Event" sheet becomes an edit surface. Starts in VIEW-ONLY mode
    // (isEditing = false) per user-requested flow — clicking Open on the
    // hover preview shouldn't immediately drop users into a destructive
    // edit; they tap "Edit" in the toolbar to unlock fields.
    init(existingEvent: CalendarEventMirror) {
        let cal = Calendar.current
        _startDate = State(initialValue: existingEvent.startDate)
        _endDate = State(initialValue: existingEvent.isAllDay
            ? (cal.date(byAdding: .day, value: -1, to: existingEvent.endDate) ?? existingEvent.endDate)
            : existingEvent.endDate)
        _isAllDay = State(initialValue: existingEvent.isAllDay)
        _isEditing = State(initialValue: false)
        _summary = State(initialValue: existingEvent.summary)
        _details = State(initialValue: existingEvent.details)
        _location = State(initialValue: existingEvent.location)
        _selectedCalendarID = State(initialValue: existingEvent.calendarID)
        _reminderOption = State(initialValue: EventReminderOption(minutes: existingEvent.reminderMinutes.first))
        _recurrenceRule = State(initialValue: existingEvent.recurrence.lazy.compactMap(RecurrenceRule.parse).first)
        _attendees = State(initialValue: existingEvent.attendeeEmails)
        _addGoogleMeet = State(initialValue: existingEvent.meetLink.isEmpty == false)
        _eventColor = State(initialValue: CalendarEventColor.from(colorId: existingEvent.colorId))
        _startTimeZoneID = State(initialValue: existingEvent.startTimeZoneID)
        _endTimeZoneID = State(initialValue: existingEvent.endTimeZoneID)
        _usesSeparateEndTimeZone = State(initialValue: existingEvent.startTimeZoneID != existingEvent.endTimeZoneID)
        _editingEvent = State(initialValue: existingEvent)
    }

    var body: some View {
        NavigationStack {
            Group {
                if calendarStore.calendars.isEmpty {
                    ContentUnavailableView(
                        "No calendars loaded",
                        systemImage: "calendar.badge.exclamationmark",
                        description: Text("Connect Google and refresh before creating an event.")
                    )
                } else if editingEvent != nil, isEditing == false {
                    // Purpose-formatted read card: renders only fields that
                    // actually carry a value. Details render as markdown, not
                    // raw text. Two-column layout when the event has enough
                    // meat to fill it; single stack otherwise.
                    ScrollView {
                        viewOnlyBody
                            .hcbScaledPadding(18)
                    }
                } else {
                    ScrollView {
                        twoColumnBody
                            .hcbScaledPadding(18)
                    }
                }
            }
            .appBackground()
            .navigationTitle(navTitle)
            .task {
                selectedCalendarID = selectedCalendarID ?? defaultCalendarID
                applyDefaultTimeZoneIfNeeded()
                applyDeepLinkPrefillIfAny()
            }
            .onChange(of: selectedCalendarID) { _, _ in
                applyDefaultTimeZoneIfNeeded()
            }
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(isSaving)
                }
                // Overflow menu with Delete / Duplicate / Copy as Markdown /
                // Export .ics / Save as Template only surfaces once the user
                // actively enters edit mode — the view-only pass keeps the
                // toolbar minimal apart from the explicit Delete button
                // added below.
                if editingEvent != nil, isEditing {
                    ToolbarItem(placement: .primaryAction) {
                        editMoreMenu
                    }
                }
                // View-only pass gets an explicit Delete button alongside
                // Cancel / Edit so users don't have to enter edit mode just
                // to remove an event. Routes through the confirmation /
                // recurring-scope picker below.
                if editingEvent != nil, isEditing == false {
                    ToolbarItem(placement: .destructiveAction) {
                        Button(role: .destructive) {
                            if isRecurringEvent {
                                pendingRecurringDeleteScope = true
                            } else {
                                isConfirmingDelete = true
                            }
                        } label: {
                            Text("Delete")
                        }
                        .disabled(isSaving)
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    if editingEvent != nil, isEditing == false {
                        // View-only pass: primary button escalates to edit.
                        Button("Edit") {
                            HCBMotion.perform(reduceMotion: reduceMotion, animation: .easeInOut(duration: 0.12)) {
                                details = HCBTextMarkup.markdownSource(from: details)
                                isEditing = true
                            }
                        }
                    } else {
                        Button(editingEvent == nil ? "Create" : "Save") {
                            if editingEvent != nil, isRecurringEvent {
                                pendingRecurringScope = true
                            } else {
                                Task { await createOrUpdateEvent(scope: .thisOccurrence) }
                            }
                        }
                        .disabled(canCreate == false || isSaving)
                    }
                }
            }
            .confirmationDialog(
                "Delete this event?",
                isPresented: $isConfirmingDelete,
                titleVisibility: .visible
            ) {
                Button("Delete", role: .destructive) {
                    guard let existing = editingEvent else { return }
                    Task { await deleteExistingEvent(existing) }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This deletes the event from Google Calendar. It cannot be undone.")
            }
            .confirmationDialog(
                "Delete which occurrences?",
                isPresented: $pendingRecurringDeleteScope,
                titleVisibility: .visible
            ) {
                Button("This event only", role: .destructive) {
                    guard let existing = editingEvent else { return }
                    Task { await deleteEventWithScope(existing, scope: .thisOccurrence) }
                }
                Button("This and following events", role: .destructive) {
                    guard let existing = editingEvent else { return }
                    Task { await deleteEventWithScope(existing, scope: .thisAndFollowing) }
                }
                Button("Every event in the series (past + future)", role: .destructive) {
                    guard let existing = editingEvent else { return }
                    Task { await deleteEventWithScope(existing, scope: .allInSeries) }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This event is part of a recurring series. \"Every event in the series\" deletes past occurrences too — not just upcoming. No guest updates are sent.")
            }
            .confirmationDialog(
                CalendarEventInstance.isRecurring(editingEvent ?? placeholderEvent)
                    ? "Update which occurrences?"
                    : "Update",
                isPresented: $pendingRecurringScope,
                titleVisibility: .visible
            ) {
                Button("This event only") {
                    Task { await createOrUpdateEvent(scope: .thisOccurrence) }
                }
                Button("This and following events", role: .destructive) {
                    Task { await createOrUpdateEvent(scope: .thisAndFollowing) }
                }
                Button("Every event in the series (past + future)", role: .destructive) {
                    Task { await createOrUpdateEvent(scope: .allInSeries) }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This event is part of a recurring series. \"Every event in the series\" updates past occurrences too — not just upcoming. No guest updates are sent.")
            }
            .alert("Save as template", isPresented: $isNamingTemplate) {
                TextField("Template name", text: $templateName)
                Button("Save") {
                    let trimmed = templateName.trimmingCharacters(in: .whitespacesAndNewlines)
                    guard let existing = editingEvent, trimmed.isEmpty == false else { return }
                    model.saveAsEventTemplate(existing, name: trimmed)
                }
                .keyboardShortcut(.defaultAction)
                Button("Cancel", role: .cancel) {}
                    .keyboardShortcut(.cancelAction)
            } message: {
                Text("Saves this event's summary, duration, reminders, attendees, and color as a reusable blueprint in Settings.")
            }
        }
        .hcbScaledFrame(minWidth: 820, idealWidth: 920, minHeight: 560, idealHeight: 680)
        .focusedSceneValue(\.calendarEventEditorCommandActions, calendarEventEditorCommandActions)
        .interactiveDismissDisabled(isSaving)
    }

    private var calendarEventEditorCommandActions: CalendarEventEditorCommandActions {
        CalendarEventEditorCommandActions(
            duplicateEvent: { duplicateExisting(offsetDays: 0) },
            canDuplicateEvent: editingEvent != nil && isSaving == false
        )
    }

    // True if the event being edited carries a recurrence rule or is an
    // instance of a recurring series. Drives the scope-picker dialog.
    private var isRecurringEvent: Bool {
        guard let editingEvent else { return false }
        return CalendarEventInstance.isRecurring(editingEvent)
    }

    // Sheet title flips through three states: create (New Event), view-only
    // on an existing event (Event), and active edit (Edit Event).
    private var navTitle: String {
        guard editingEvent != nil else {
            return isBirthdayMode ? "New Birthday" : "New Event"
        }
        return isEditing ? "Edit Event" : "Event"
    }

    // Sentinel used only to satisfy the confirmationDialog title interpolation
    // when editingEvent is nil (dialog won't be shown in that case; this is
    // belt-and-braces).
    private var placeholderEvent: CalendarEventMirror {
        CalendarEventMirror(
            id: "", calendarID: "", summary: "", details: "",
            startDate: Date(), endDate: Date(), isAllDay: false,
            status: .confirmed, recurrence: [], etag: nil, updatedAt: nil
        )
    }

    // Overflow menu shown in edit mode — migrated from the old
    // EventActionPanel on EventDetailView: Delete, Duplicate (5 offsets),
    // Copy as Markdown, Export .ics, Save as Template.
    @ViewBuilder
    private var editMoreMenu: some View {
        Menu {
            Button(role: .destructive) {
                guard let existing = editingEvent else { return }
                Task { await deleteExistingEvent(existing) }
            } label: {
                Label("Delete Event", systemImage: "trash")
            }
            Divider()
            Menu {
                Button {
                    guard let existing = editingEvent else { return }
                    router?.present(.convertEventToTask(existing.id))
                } label: {
                    Label("Convert to Task…", systemImage: "checklist")
                }
                Button {
                    guard let existing = editingEvent else { return }
                    router?.present(.convertEventToNote(existing.id))
                } label: {
                    Label("Convert to Note…", systemImage: "note.text")
                }
            } label: {
                Label("Convert", systemImage: "arrow.triangle.swap")
            }
            Divider()
            Menu("Duplicate") {
                Button("Duplicate here") { duplicateExisting(offsetDays: 0) }
                Button("Duplicate to tomorrow") { duplicateExisting(offsetDays: 1) }
                Button("Duplicate to next week") { duplicateExisting(offsetDays: 7) }
                Button("Duplicate in 2 weeks") { duplicateExisting(offsetDays: 14) }
                Button("Duplicate to next month") { duplicateExisting(offsetDays: 30) }
            }
            Divider()
            Button {
                guard let existing = editingEvent else { return }
                let calendarTitle = calendarStore.calendars.first(where: { $0.id == existing.calendarID })?.summary
                let markdown = EventMarkdownExporter.markdown(for: existing, calendarTitle: calendarTitle)
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(markdown, forType: .string)
            } label: {
                Label("Copy as Markdown", systemImage: "doc.on.clipboard")
            }
            Button {
                guard let existing = editingEvent else { return }
                exportEventAsICS(existing)
            } label: {
                Label("Export .ics…", systemImage: "square.and.arrow.up")
            }
            Button {
                templateName = editingEvent?.summary ?? ""
                isNamingTemplate = true
            } label: {
                Label("Save as Template…", systemImage: "doc.badge.plus")
            }
        } label: {
            Label("More", systemImage: "ellipsis.circle")
        }
        .disabled(isSaving)
    }

    private func duplicateExisting(offsetDays: Int) {
        guard let existing = editingEvent else { return }
        Task {
            _ = await model.duplicateEvent(existing, offsetDays: offsetDays)
            dismiss()
        }
    }

    private func deleteExistingEvent(_ existing: CalendarEventMirror) async {
        await deleteEventWithScope(existing, scope: .thisOccurrence)
    }

    // Scope-aware delete for the view-only toolbar Delete button. Recurring
    // events land here via the scope picker; one-offs always use
    // .thisOccurrence (scope is a no-op server-side for non-recurring IDs).
    private func deleteEventWithScope(_ existing: CalendarEventMirror, scope: AppModel.RecurringEventScope) async {
        // Close the sheet IMMEDIATELY. model.deleteEvent optimistically strips
        // the event from the mirror before awaiting the Google API; while the
        // await is in flight the router's .editEvent route re-evaluates
        // the routed event lookup sees nil and swaps the sheet content to
        // "Event not found" — a 5-15s phantom dialog the user can't dismiss.
        // Closing first lets the delete + undo toast land cleanly.
        dismiss()
        _ = await model.deleteEvent(existing, scope: scope)
    }

    private func exportEventAsICS(_ existing: CalendarEventMirror) {
        let content = EventICSExporter.ics(for: existing)
        let panel = NSSavePanel()
        let sanitized = existing.summary
            .replacingOccurrences(of: "/", with: "-")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let base = sanitized.isEmpty ? "event" : sanitized
        panel.nameFieldStringValue = "\(base).ics"
        panel.allowedContentTypes = [UTType(filenameExtension: "ics") ?? .data]
        panel.canCreateDirectories = true
        if panel.runModal() == .OK, let url = panel.url {
            try? content.data(using: .utf8)?.write(to: url)
        }
    }

    private var twoColumnBody: some View {
        VStack(alignment: .leading, spacing: 14) {
            // Mode picker matches QuickCreatePopover so the detailed sheet
            // exposes the same Event/Birthday split. Hidden in edit mode —
            // changing mode mid-edit would scramble the field set.
            if editingEvent == nil {
                Picker("", selection: $isBirthdayMode) {
                    Text("Event").tag(false)
                    Text("Birthday").tag(true)
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                .frame(maxWidth: 320)
            }
            if isBirthdayMode {
                birthdayBody
            } else {
                eventTwoColumnBody
            }
        }
    }

    @ViewBuilder
    private var eventTwoColumnBody: some View {
        HStack(alignment: .top, spacing: 18) {
            VStack(alignment: .leading, spacing: 14) {
                if editingEvent == nil {
                    quickCreateBlock
                }
                summaryBlock
                timeBlock
                detailsBlock
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)

            VStack(alignment: .leading, spacing: 14) {
                calendarBlock
                colorReminderBlock
                guestsBlock
                repeatBlock
                meetBlock
                if editingEvent == nil, calendarStore.settings.eventTemplates.isEmpty == false {
                    templateBlock
                }
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }

    // Birthday body: only the fields that apply. Save path forces all-day
    // single-day + RRULE:FREQ=YEARLY (handled in createEvent).
    @ViewBuilder
    private var birthdayBody: some View {
        VStack(alignment: .leading, spacing: 14) {
            sectionCard("Birthday") {
                TextField("Whose birthday?", text: $summary)
                    .textFieldStyle(.roundedBorder)
            }
            sectionCard("Date") {
                DatePicker("Date", selection: $startDate, displayedComponents: [.date])
                    .labelsHidden()
                Text("Repeats yearly. Saved as an all-day event on the selected calendar.")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
            calendarBlock
            sectionCard("Notes") {
                MarkdownEditor(text: $details, placeholder: "Optional notes (markdown supported)", minHeight: 90, maxHeight: 200)
            }
        }
        .frame(maxWidth: 560, alignment: .topLeading)
    }

    @ViewBuilder
    private func sectionCard<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(title.uppercased())
                .hcbFont(.caption2, weight: .bold)
                .foregroundStyle(.secondary)
            content()
        }
        .hcbScaledPadding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 12)
                .fill(AppColor.cream.opacity(0.5))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(AppColor.cardStroke, lineWidth: 0.6)
        )
    }

    private var quickCreateBlock: some View {
        sectionCard("Quick Create") {
            TextField("e.g., \"Lunch with Bob tomorrow 1pm at Philz for 45 min\"", text: $quickCreateText)
                .textFieldStyle(.roundedBorder)
                .onSubmit(applyQuickCreate)
                .onChange(of: quickCreateText) { _, newValue in
                    parsedPreview = newValue.isEmpty ? nil : NaturalLanguageEventParser().parse(newValue)
                }
            if let preview = parsedPreview, preview.hasParsedMetadata || quickCreateColorTagResolution(for: preview) != nil || quickCreateCalendarTagResolution(for: preview) != nil {
                HStack(spacing: 6) {
                    ForEach(Array(preview.matchedTokens.enumerated()), id: \.offset) { _, token in
                        Text(token.display)
                            .hcbFont(.caption, weight: .medium)
                            .hcbScaledPadding(.horizontal, 6)
                            .hcbScaledPadding(.vertical, 2)
                            .background(Capsule().fill(AppColor.blue.opacity(0.15)))
                            .foregroundStyle(AppColor.blue)
                    }
                    if let color = quickCreateEventColor(for: preview) {
                        Text(color.title)
                            .hcbFont(.caption, weight: .medium)
                            .hcbScaledPadding(.horizontal, 6)
                            .hcbScaledPadding(.vertical, 2)
                            .background(Capsule().fill(quickCreateColorTint(color).opacity(0.15)))
                            .foregroundStyle(quickCreateColorTint(color))
                    }
                    if let calendar = quickCreateCalendarTagResolution(for: preview)?.calendar {
                        Text(calendar.summary)
                            .hcbFont(.caption, weight: .medium)
                            .hcbScaledPadding(.horizontal, 6)
                            .hcbScaledPadding(.vertical, 2)
                            .background(Capsule().fill(Color(hex: calendar.colorHex).opacity(0.15)))
                            .foregroundStyle(Color(hex: calendar.colorHex))
                    }
                    Spacer(minLength: 0)
                    Button("Apply") { applyQuickCreate() }
                        .buttonStyle(.borderless)
                }
            }
        }
    }

    private var summaryBlock: some View {
        sectionCard("Event") {
            TextField("Summary", text: $summary)
                .textFieldStyle(.roundedBorder)
            TextField("Location", text: $location)
                .textFieldStyle(.roundedBorder)
            if location.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                // Editable binding — the full-view sheet surfaces a text
                // field that writes back to `location` so users can refine
                // the address from the map surface.
                LocationMapPreview(locationText: $location, isEditable: true)
            }
        }
    }

    private var timeBlock: some View {
        sectionCard("Time") {
            Toggle("All-day event", isOn: $isAllDay)
            if isAllDay {
                DatePicker("Starts", selection: $startDate, displayedComponents: [.date])
                DatePicker("Ends", selection: $endDate, displayedComponents: [.date])
            } else {
                DatePicker("Starts", selection: $startDate, displayedComponents: [.date, .hourAndMinute])
                    .environment(\.timeZone, startTimeZone)
                HStack(alignment: .firstTextBaseline, spacing: 8) {
                    DatePicker("Ends", selection: $endDate, displayedComponents: [.date, .hourAndMinute])
                        .environment(\.timeZone, endTimeZone)
                    EventEndTimePickerMenu(
                        startDate: startDate,
                        endDate: $endDate,
                        timeZoneID: effectiveEndTimeZoneID
                    )
                }
                timeZoneControls
            }
            if isValidDateRange == false {
                Text(isAllDay ? "End date cannot be before start date." : "End time must be after start time.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.red)
            }
        }
    }

    private var timeZoneControls: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                EventTimeZonePickerRow(title: "Time zone", selection: startTimeZoneBinding)
                Spacer(minLength: 8)
                Toggle("Separate end", isOn: separateTimeZoneBinding)
                    .toggleStyle(.checkbox)
                    .hcbFont(.caption)
            }
            if usesSeparateEndTimeZone {
                EventTimeZonePickerRow(title: "End time zone", selection: endTimeZoneBinding)
            }
        }
        .hcbScaledPadding(.top, 4)
    }

    private var detailsBlock: some View {
        sectionCard("Details") {
            MarkdownEditor(text: $details, placeholder: "Notes (markdown supported)", minHeight: 90, maxHeight: 200)
        }
    }

    private var calendarBlock: some View {
        sectionCard("Calendar") {
            Picker("Calendar", selection: $selectedCalendarID) {
                ForEach(calendarStore.calendars) { calendar in
                    Text(calendar.summary).tag(Optional(calendar.id))
                }
            }
            .labelsHidden()
        }
    }

    private var colorReminderBlock: some View {
        sectionCard("Color · Reminder") {
            EventColorPicker(selection: $eventColor)
            EventReminderPicker(selection: $reminderOption)
        }
    }

    private var guestsBlock: some View {
        sectionCard("Guests") {
            // Read-only response chips — only shown in edit mode when
            // responses exist. Migrated from AttendeeResponsesCard on the
            // removed EventDetailView.
            if let existing = editingEvent, existing.attendeeResponses.isEmpty == false {
                AttendeeResponsesCard(responses: existing.attendeeResponses)
                    .hcbScaledPadding(.bottom, 6)
            }
            GuestsSection(attendees: $attendees, draft: $attendeeDraft, notifyGuests: $notifyGuests)
        }
    }

    private var repeatBlock: some View {
        sectionCard("Repeat") {
            RecurrenceEditor(rule: $recurrenceRule, startDate: startDate)
        }
    }

    private var meetBlock: some View {
        sectionCard("Video call") {
            Toggle("Add Google Meet video conferencing", isOn: $addGoogleMeet)
            // Read-only link row when an existing event already has a Meet
            // URL — migrated from MeetLinkCard on the removed EventDetailView.
            if let existing = editingEvent, existing.meetLink.isEmpty == false,
               let meetURL = URL(string: existing.meetLink) {
                Link(destination: meetURL) {
                    Label(existing.meetLink, systemImage: "video.fill")
                        .foregroundStyle(AppColor.blue)
                        .lineLimit(1)
                }
                .hcbScaledPadding(.top, 4)
            }
        }
    }

    private var templateBlock: some View {
        sectionCard("Template") {
            Menu {
                ForEach(calendarStore.settings.eventTemplates) { template in
                    Button(template.name) { applyTemplate(template) }
                }
            } label: {
                Label("New from template…", systemImage: "doc.on.doc")
            }
        }
    }

    // MARK: - View-only card (read-only surface for the Open flow).

    // Conditional two-column layout: if a populated location or 2+ optional
    // right-side sections warrant the horizontal weight, split; else stack
    // vertically so minimal events don't look lopsided.
    @ViewBuilder
    private var viewOnlyBody: some View {
        let hasLocation = location.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
        let rightExtras = countOfPopulatedRightSections()
        let useTwoColumn = hasLocation || rightExtras >= 2

        if useTwoColumn {
            HStack(alignment: .top, spacing: 18) {
                VStack(alignment: .leading, spacing: 14) {
                    readSummaryCard
                    readTimeCard
                    if details.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                        readDetailsCard
                    }
                }
                .frame(maxWidth: .infinity, alignment: .topLeading)
                VStack(alignment: .leading, spacing: 14) {
                    readCalendarCard
                    if hasCustomColorOrReminder { readColorReminderCard }
                    if hasGuests { readGuestsCard }
                    if recurrenceRule != nil { readRepeatCard }
                    if hasMeet { readMeetCard }
                }
                .frame(maxWidth: .infinity, alignment: .topLeading)
            }
        } else {
            VStack(alignment: .leading, spacing: 14) {
                readSummaryCard
                readTimeCard
                if details.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                    readDetailsCard
                }
                readCalendarCard
                if hasCustomColorOrReminder { readColorReminderCard }
                if hasGuests { readGuestsCard }
                if recurrenceRule != nil { readRepeatCard }
                if hasMeet { readMeetCard }
            }
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
    }

    private func countOfPopulatedRightSections() -> Int {
        var n = 1 // calendar is always shown
        if hasCustomColorOrReminder { n += 1 }
        if hasGuests { n += 1 }
        if recurrenceRule != nil { n += 1 }
        if hasMeet { n += 1 }
        return n - 1 // "extras" beyond the mandatory calendar card
    }

    private var hasCustomColorOrReminder: Bool {
        eventColor != .defaultColor || reminderOption != .useDefault
    }

    private var hasGuests: Bool {
        attendees.isEmpty == false || (editingEvent?.attendeeResponses.isEmpty == false)
    }

    private var hasMeet: Bool {
        guard let existing = editingEvent else { return false }
        return existing.meetLink.isEmpty == false
    }

    private var readSummaryCard: some View {
        sectionCard("Event") {
            Text(summary.isEmpty ? "Untitled" : summary)
                .hcbFont(.title3, weight: .semibold)
                .foregroundStyle(AppColor.ink)
                .textSelection(.enabled)
            if location.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                Label(location, systemImage: "mappin.and.ellipse")
                    .hcbFont(.subheadline)
                    .foregroundStyle(.secondary)
                    .textSelection(.enabled)
                LocationMapPreview(locationText: .constant(location), isEditable: false)
            }
        }
    }

    private var readTimeCard: some View {
        sectionCard("Time") {
            if isAllDay {
                let cal = Calendar.current
                let sameDay = cal.isDate(startDate, inSameDayAs: endDate)
                if sameDay {
                    Text(startDate.formatted(.dateTime.weekday(.wide).day().month(.wide).year()))
                        .hcbFont(.subheadline)
                        .foregroundStyle(AppColor.ink)
                } else {
                    Text("\(startDate.formatted(.dateTime.weekday(.abbreviated).day().month(.abbreviated))) → \(endDate.formatted(.dateTime.weekday(.abbreviated).day().month(.abbreviated)))")
                        .hcbFont(.subheadline)
                        .foregroundStyle(AppColor.ink)
                }
                Text("All-day")
                    .hcbFont(.caption, weight: .medium)
                    .foregroundStyle(.secondary)
            } else {
                Text(startDate.formatted(.dateTime.weekday(.wide).day().month(.wide).year()))
                    .hcbFont(.subheadline)
                    .foregroundStyle(AppColor.ink)
                Text("\(formattedClockTime(startDate, timeZoneID: startTimeZoneID)) – \(formattedClockTime(endDate, timeZoneID: effectiveEndTimeZoneID))")
                    .hcbFont(.subheadline, weight: .medium)
                    .foregroundStyle(.secondary)
                Text(timeZoneSummary)
                    .hcbFont(.caption, weight: .medium)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var readDetailsCard: some View {
        sectionCard("Details") {
            MarkdownBlock(source: details)
                .hcbFont(.body)
                .foregroundStyle(AppColor.ink)
                .textSelection(.enabled)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var readCalendarCard: some View {
        sectionCard("Calendar") {
            let cal = calendarStore.calendars.first(where: { $0.id == selectedCalendarID })
            HStack(spacing: 8) {
                if let hex = cal?.colorHex {
                    Circle()
                        .fill(Color(hex: hex))
                        .hcbScaledFrame(width: 10, height: 10)
                }
                Text(cal?.summary ?? "Calendar")
                    .hcbFont(.subheadline)
                    .foregroundStyle(AppColor.ink)
            }
        }
    }

    private var readColorReminderCard: some View {
        sectionCard("Color · Reminder") {
            if eventColor != .defaultColor {
                HStack(spacing: 8) {
                    if let hex = eventColor.hex {
                        Circle()
                            .fill(Color(hex: hex))
                            .hcbScaledFrame(width: 14, height: 14)
                    }
                    Text(eventColor.title)
                        .hcbFont(.subheadline)
                        .foregroundStyle(AppColor.ink)
                }
            }
            if reminderOption != .useDefault {
                Label(reminderOption.title, systemImage: "bell")
                    .hcbFont(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private var readGuestsCard: some View {
        sectionCard("Guests") {
            if let existing = editingEvent, existing.attendeeResponses.isEmpty == false {
                AttendeeResponsesCard(responses: existing.attendeeResponses)
            } else {
                VStack(alignment: .leading, spacing: 6) {
                    ForEach(attendees, id: \.self) { email in
                        GuestParticipantRow(
                            email: email,
                            statusTitle: "Invited",
                            statusSymbol: "paperplane",
                            statusTint: .secondary
                        )
                    }
                }
            }
        }
    }

    private var readRepeatCard: some View {
        sectionCard("Repeat") {
            if let rule = recurrenceRule {
                Label(rule.summary, systemImage: "repeat")
                    .hcbFont(.subheadline)
                    .foregroundStyle(AppColor.ink)
            }
        }
    }

    private var readMeetCard: some View {
        sectionCard("Video call") {
            if let existing = editingEvent, let url = URL(string: existing.meetLink) {
                Link(destination: url) {
                    Label(existing.meetLink, systemImage: "video.fill")
                        .hcbFont(.subheadline)
                        .foregroundStyle(AppColor.blue)
                        .lineLimit(1)
                }
            }
        }
    }

    private var defaultCalendarID: CalendarListMirror.ID? {
        calendarStore.calendarSnapshot.selectedCalendars.first?.id ?? calendarStore.calendars.first?.id
    }

    private var startTimeZone: TimeZone {
        TimezoneSupport.timeZone(for: startTimeZoneID)
    }

    private var endTimeZone: TimeZone {
        TimezoneSupport.timeZone(for: effectiveEndTimeZoneID)
    }

    private var effectiveEndTimeZoneID: String {
        usesSeparateEndTimeZone ? endTimeZoneID : startTimeZoneID
    }

    private var timeZoneSummary: String {
        if usesSeparateEndTimeZone, startTimeZoneID != endTimeZoneID {
            return "\(TimezoneSupport.compactDisplayName(for: startTimeZoneID)) → \(TimezoneSupport.compactDisplayName(for: endTimeZoneID))"
        }
        return TimezoneSupport.compactDisplayName(for: startTimeZoneID)
    }

    private var startTimeZoneBinding: Binding<String> {
        Binding(
            get: { startTimeZoneID },
            set: { newValue in
                let normalized = TimezoneSupport.validatedIdentifier(newValue) ?? TimezoneSupport.currentIdentifier
                let oldValue = startTimeZoneID
                guard normalized != oldValue else { return }
                startDate = TimezoneSupport.reinterpretingWallClock(startDate, from: oldValue, to: normalized)
                if usesSeparateEndTimeZone == false {
                    endDate = TimezoneSupport.reinterpretingWallClock(endDate, from: oldValue, to: normalized)
                    endTimeZoneID = normalized
                }
                startTimeZoneID = normalized
                userManuallyPickedTimeZone = true
            }
        )
    }

    private var endTimeZoneBinding: Binding<String> {
        Binding(
            get: { endTimeZoneID },
            set: { newValue in
                let normalized = TimezoneSupport.validatedIdentifier(newValue) ?? startTimeZoneID
                let oldValue = endTimeZoneID
                guard normalized != oldValue else { return }
                endDate = TimezoneSupport.reinterpretingWallClock(endDate, from: oldValue, to: normalized)
                endTimeZoneID = normalized
                userManuallyPickedTimeZone = true
            }
        )
    }

    private var separateTimeZoneBinding: Binding<Bool> {
        Binding(
            get: { usesSeparateEndTimeZone },
            set: { newValue in
                usesSeparateEndTimeZone = newValue
                if newValue == false {
                    endTimeZoneID = startTimeZoneID
                }
                userManuallyPickedTimeZone = true
            }
        )
    }

    private func applyDefaultTimeZoneIfNeeded() {
        guard editingEvent == nil, userManuallyPickedTimeZone == false else { return }
        let calendarTimeZoneID = selectedCalendarID
            .flatMap { id in calendarStore.calendars.first(where: { $0.id == id })?.timeZoneID }
        let resolved = TimezoneSupport.validatedIdentifier(calendarTimeZoneID) ?? TimezoneSupport.currentIdentifier
        let old = startTimeZoneID
        if old != resolved, isAllDay == false {
            startDate = TimezoneSupport.reinterpretingWallClock(startDate, from: old, to: resolved)
            endDate = TimezoneSupport.reinterpretingWallClock(endDate, from: old, to: resolved)
        }
        startTimeZoneID = resolved
        endTimeZoneID = resolved
        usesSeparateEndTimeZone = false
    }

    private func formattedClockTime(_ date: Date, timeZoneID: String) -> String {
        let formatter = DateFormatter()
        formatter.locale = .current
        formatter.timeZone = TimezoneSupport.timeZone(for: timeZoneID)
        formatter.timeStyle = .short
        formatter.dateStyle = .none
        return formatter.string(from: date)
    }

    private func applyDeepLinkPrefillIfAny() {
        // hotcrossbuns://new/event?… stages a prefill struct on AppModel. The
        // sheet consumes it here and nils it so a subsequent plain New Event
        // doesn't inherit stale values.
        guard let prefill = model.pendingEventPrefill else { return }
        defer { model.pendingEventPrefill = nil }

        if let t = prefill.title, t.isEmpty == false, summary.isEmpty {
            summary = t
        }
        if let loc = prefill.location, loc.isEmpty == false, location.isEmpty {
            location = loc
        }
        if let start = prefill.startDate {
            startDate = start
            if let end = prefill.endDate, end > start {
                endDate = end
            } else if prefill.isAllDay {
                endDate = start
            } else {
                endDate = start.addingTimeInterval(3600)
            }
        }
        if prefill.isAllDay { isAllDay = true }
        if let calRef = prefill.calendarIdOrSummary, calRef.isEmpty == false,
           let match = resolveCalendar(calRef) {
            selectedCalendarID = match.id
        }
    }

    private func resolveCalendar(_ ref: String) -> CalendarListMirror? {
        if let exact = calendarStore.calendars.first(where: { $0.id == ref }) { return exact }
        return calendarStore.calendars.first(where: { $0.summary.localizedCaseInsensitiveCompare(ref) == .orderedSame })
    }

    private var canCreate: Bool {
        summary.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
            && selectedCalendarID != nil
            && (isBirthdayMode || isValidDateRange)
            && calendarStore.account != nil
    }

    private var isValidDateRange: Bool {
        if isAllDay {
            return Calendar.current.startOfDay(for: endDate) >= Calendar.current.startOfDay(for: startDate)
        }

        return endDate > startDate
    }

    private func applyTemplate(_ template: EventTemplate) {
        summary = template.summary
        details = template.details
        location = template.location
        isAllDay = template.isAllDay
        let start = startDate
        endDate = template.isAllDay ? start : start.addingTimeInterval(TimeInterval(template.durationMinutes) * 60)
        if let mins = template.reminderMinutes {
            reminderOption = .preset(mins)
        }
        if let colorId = template.colorId {
            eventColor = CalendarEventColor.from(colorId: colorId)
        }
        attendees = template.attendees
        addGoogleMeet = template.addGoogleMeet
        model.markEventTemplateUsed(template.id)
    }

    private func applyQuickCreate() {
        let parsed = NaturalLanguageEventParser().parse(quickCreateText)
        guard parsed.summary.isEmpty == false else { return }
        summary = quickCreateSummaryForSubmission(parsed)
        if let color = quickCreateEventColor(for: parsed) {
            eventColor = color
        }
        if let calendar = quickCreateCalendarTagResolution(for: parsed)?.calendar {
            selectedCalendarID = calendar.id
        }
        if let loc = parsed.location { location = loc }
        if let start = parsed.startDate {
            startDate = start
            isAllDay = parsed.isAllDay
            if let end = parsed.endDate {
                endDate = parsed.isAllDay ? start : end
            }
        }
        quickCreateText = ""
        parsedPreview = nil
    }

    private func quickCreateColorTagResolution(for parsed: ParsedQuickAddEvent) -> ColorTagResolver.Resolution? {
        guard calendarStore.settings.colorTagAutoApplyEnabled else { return nil }
        return ColorTagResolver.resolve(
            title: parsed.summary,
            bindings: calendarStore.settings.colorTagBindings,
            policy: calendarStore.settings.colorTagMatchPolicy
        )
    }

    private func quickCreateEventColor(for parsed: ParsedQuickAddEvent) -> CalendarEventColor? {
        guard let colorId = quickCreateColorTagResolution(for: parsed)?.colorId else { return nil }
        return CalendarEventColor.from(colorId: colorId)
    }

    private func quickCreateCalendarTagResolution(for parsed: ParsedQuickAddEvent) -> CalendarTagResolver.Resolution? {
        CalendarTagResolver.resolve(title: parsed.summary, calendars: calendarStore.calendarSnapshot.selectedCalendars)
            ?? CalendarTagResolver.resolve(title: parsed.summary, calendars: calendarStore.calendars)
    }

    private func quickCreateColorTint(_ color: CalendarEventColor) -> Color {
        if let hex = color.hex { return Color(hex: hex) }
        return AppColor.blue
    }

    private func quickCreateSummaryForSubmission(_ parsed: ParsedQuickAddEvent) -> String {
        let raw = parsed.summary.trimmingCharacters(in: .whitespacesAndNewlines)
        var stripped = raw
        for tag in [
            quickCreateColorTagResolution(for: parsed)?.matchedTag,
            quickCreateCalendarTagResolution(for: parsed)?.matchedTag
        ].compactMap({ $0 }) {
            stripped = ColorTagResolver.stripTag(tag, from: stripped)
        }
        return stripped.isEmpty ? raw : stripped
    }

    private func createOrUpdateEvent(scope: AppModel.RecurringEventScope = .thisOccurrence) async {
        if let existing = editingEvent {
            await updateExistingEvent(existing, scope: scope)
        } else {
            await createEvent()
        }
    }

    private func createEvent() async {
        guard let selectedCalendarID else {
            return
        }

        isSaving = true
        defer { isSaving = false }

        let didCreate: Bool
        if isBirthdayMode {
            // Force the birthday shape regardless of what the user touched
            // in the (hidden) event-only fields. Same payload shape as
            // QuickCreatePopover's birthday branch — keeps the two surfaces
            // bit-identical so users get the same Google representation
            // either way.
            let day = Calendar.current.startOfDay(for: startDate)
            didCreate = await model.createEvent(
                summary: summary,
                details: details,
                startDate: day,
                endDate: day,
                isAllDay: true,
                reminderMinutes: nil,
                calendarID: selectedCalendarID,
                location: "",
                recurrence: ["RRULE:FREQ=YEARLY"],
                attendeeEmails: [],
                notifyGuests: false,
                addGoogleMeet: false,
                colorId: nil,
                startTimeZoneID: nil,
                endTimeZoneID: nil
            )
        } else {
            didCreate = await model.createEvent(
                summary: summary,
                details: details,
                startDate: normalizedStartDate,
                endDate: normalizedEndDate,
                isAllDay: isAllDay,
                reminderMinutes: reminderOption.minutes,
                calendarID: selectedCalendarID,
                location: location,
                recurrence: recurrenceRule.map { [$0.rruleString()] } ?? [],
                attendeeEmails: attendees,
                notifyGuests: notifyGuests,
                addGoogleMeet: addGoogleMeet,
                colorId: eventColor.wireValue,
                startTimeZoneID: startTimeZoneID,
                endTimeZoneID: effectiveEndTimeZoneID
            )
        }

        if didCreate {
            dismiss()
        }
    }

    // Edit-mode save path. Scope defaults to .thisOccurrence for non-
    // recurring events; the toolbar's confirmation dialog asks for
    // this-occurrence / this-and-following / all-in-series when the event
    // is part of a recurring series.
    private func updateExistingEvent(_ existing: CalendarEventMirror, scope: AppModel.RecurringEventScope) async {
        guard let selectedCalendarID else { return }
        isSaving = true
        defer { isSaving = false }
        let didUpdate = await model.updateEvent(
            existing,
            summary: summary,
            details: details,
            startDate: normalizedStartDate,
            endDate: normalizedEndDate,
            isAllDay: isAllDay,
            reminderMinutes: reminderOption.minutes,
            calendarID: selectedCalendarID,
            location: location,
            recurrence: recurrenceRule.map { [$0.rruleString()] } ?? [],
            attendeeEmails: attendees,
            notifyGuests: notifyGuests,
            scope: scope,
            addGoogleMeet: addGoogleMeet,
            colorId: eventColor.wireValue,
            startTimeZoneID: startTimeZoneID,
            endTimeZoneID: effectiveEndTimeZoneID
        )
        if didUpdate {
            dismiss()
        }
    }

    private var normalizedStartDate: Date {
        isAllDay ? Calendar.current.startOfDay(for: startDate) : startDate
    }

    private var normalizedEndDate: Date {
        isAllDay ? Calendar.current.startOfDay(for: endDate) : endDate
    }
}

struct EventDurationFormatter {
    static func label(minutes rawMinutes: Int) -> String {
        let minutes = max(1, rawMinutes)
        if minutes < 60 {
            return "\(minutes) min"
        }

        let hours = minutes / 60
        let remainder = minutes % 60
        if hours < 24 {
            if remainder == 0 {
                return "\(hours)h"
            }
            return "\(hours)h \(remainder)m"
        }

        let days = hours / 24
        let dayHours = hours % 24
        let dayPrefix = "\(days)d"
        if dayHours == 0 && remainder == 0 {
            return dayPrefix
        }
        if remainder == 0 {
            return "\(dayPrefix) \(dayHours)h"
        }
        return "\(dayPrefix) \(dayHours)h \(remainder)m"
    }
}

struct EventEndTimeOption: Hashable, Identifiable {
    let durationMinutes: Int
    let endDate: Date
    let timeTitle: String
    let durationTitle: String

    var id: Int { durationMinutes }

    static let standardDurationMinutes: [Int] = Array(stride(from: 15, through: 240, by: 15)) + [
        270,
        300,
        360,
        420,
        480
    ]

    static func options(
        startDate: Date,
        currentEndDate: Date,
        timeZoneID: String,
        calendar: Calendar = .current
    ) -> [EventEndTimeOption] {
        var durations = Set(standardDurationMinutes)
        let currentDuration = Int((currentEndDate.timeIntervalSince(startDate) / 60).rounded())
        if currentDuration > 0 {
            durations.insert(currentDuration)
        }

        return durations
            .sorted()
            .map { duration in
                let endDate = startDate.addingTimeInterval(TimeInterval(duration * 60))
                return EventEndTimeOption(
                    durationMinutes: duration,
                    endDate: endDate,
                    timeTitle: timeTitle(for: endDate, startDate: startDate, timeZoneID: timeZoneID, calendar: calendar),
                    durationTitle: EventDurationFormatter.label(minutes: duration)
                )
            }
    }

    private static func timeTitle(
        for endDate: Date,
        startDate: Date,
        timeZoneID: String,
        calendar: Calendar
    ) -> String {
        let timeZone = TimezoneSupport.timeZone(for: timeZoneID)
        var scopedCalendar = calendar
        scopedCalendar.timeZone = timeZone

        let formatter = DateFormatter()
        formatter.timeZone = timeZone
        formatter.locale = .current
        if scopedCalendar.isDate(startDate, inSameDayAs: endDate) {
            formatter.dateStyle = .none
            formatter.timeStyle = .short
        } else {
            formatter.dateStyle = .medium
            formatter.timeStyle = .short
        }
        return formatter.string(from: endDate)
    }
}

struct EventEndTimePickerMenu: View {
    let startDate: Date
    @Binding var endDate: Date
    let timeZoneID: String

    private var options: [EventEndTimeOption] {
        EventEndTimeOption.options(
            startDate: startDate,
            currentEndDate: endDate,
            timeZoneID: timeZoneID
        )
    }

    var body: some View {
        Menu {
            ForEach(options) { option in
                Button("\(option.timeTitle) · \(option.durationTitle)") {
                    endDate = option.endDate
                }
            }
        } label: {
            Label("End time options", systemImage: "clock")
                .labelStyle(.iconOnly)
        }
        .menuStyle(.borderlessButton)
        .fixedSize()
        .help("Choose an end time by duration")
        .accessibilityLabel("End time options")
        .accessibilityHint("Shows common end times in fifteen minute increments")
    }
}

struct EventTimeZonePickerRow: View {
    let title: String
    @Binding var selection: String

    var body: some View {
        Picker(selection: $selection) {
            ForEach(TimezoneSupport.pickerIdentifiers, id: \.self) { identifier in
                Text(TimezoneSupport.compactDisplayName(for: identifier))
                    .tag(identifier)
            }
        } label: {
            Label(title, systemImage: "globe")
                .labelStyle(.titleAndIcon)
        }
        .pickerStyle(.menu)
        .accessibilityLabel(title)
    }
}

enum EventReminderOption: Hashable, Identifiable {
    case useDefault
    case preset(Int) // minutes
    case custom(Int) // minutes

    var id: String {
        switch self {
        case .useDefault: "default"
        case .preset(let m): "preset-\(m)"
        case .custom(let m): "custom-\(m)"
        }
    }

    var minutes: Int? {
        switch self {
        case .useDefault: nil
        case .preset(let m), .custom(let m): m
        }
    }

    var title: String {
        switch self {
        case .useDefault: "Calendar default"
        case .preset(let m), .custom(let m): Self.label(forMinutes: m)
        }
    }

    static func label(forMinutes minutes: Int) -> String {
        if minutes == 0 { return "At start" }
        if minutes < 60 { return "\(minutes) minutes before" }
        if minutes < 1440 {
            let hours = minutes / 60
            let remainder = minutes % 60
            if remainder == 0 { return "\(hours) hour\(hours == 1 ? "" : "s") before" }
            return "\(hours)h \(remainder)m before"
        }
        let days = minutes / 1440
        let remainder = minutes % 1440
        if remainder == 0 { return "\(days) day\(days == 1 ? "" : "s") before" }
        return "\(days)d \((remainder + 59) / 60)h before"
    }

    static let presets: [EventReminderOption] = [
        .useDefault,
        .preset(0),
        .preset(5),
        .preset(10),
        .preset(15),
        .preset(30),
        .preset(60),
        .preset(120),
        .preset(1440)
    ]

    init(minutes: Int?) {
        guard let minutes else { self = .useDefault; return }
        if Self.presets.contains(where: { $0.minutes == minutes }) {
            self = .preset(minutes)
        } else {
            self = .custom(minutes)
        }
    }
}

struct CrossCalendarMoveRequest: Identifiable, Hashable {
    let eventID: CalendarEventMirror.ID
    let destinationCalendarID: CalendarListMirror.ID
    let eventSummary: String
    var id: String { "\(eventID)->\(destinationCalendarID)" }
}

struct CalendarDropChipsStrip: View {
    let calendars: [CalendarListMirror]
    let onDrop: (DraggedEvent, CalendarListMirror.ID) -> Void

    var body: some View {
        // Only render calendars the user can actually write to — dropping
        // onto a read-only calendar would fail at the API layer anyway.
        let writable = calendars.filter { role in
            role.accessRole == "owner" || role.accessRole == "writer"
        }
        Group {
            if writable.count <= 1 {
                EmptyView()
            } else {
                ScrollView(.horizontal) {
                    HStack(spacing: 6) {
                        Text("Drop on a calendar to move:")
                            .hcbFont(.caption2, weight: .semibold)
                            .foregroundStyle(.secondary)
                            .hcbScaledPadding(.leading, 6)
                        ForEach(writable) { calendar in
                            chip(for: calendar)
                        }
                    }
                    .hcbScaledPadding(.vertical, 6)
                    .hcbScaledPadding(.horizontal, 10)
                }
                .scrollIndicators(.hidden)
            }
        }
    }

    private func chip(for calendar: CalendarListMirror) -> some View {
        HStack(spacing: 6) {
            Circle()
                .fill(Color(hex: calendar.colorHex))
                .hcbScaledFrame(width: 8, height: 8)
            Text(calendar.summary)
                .hcbFont(.caption)
                .lineLimit(1)
        }
        .hcbScaledPadding(.horizontal, 10)
        .hcbScaledPadding(.vertical, 4)
        .background(
            Capsule().fill(AppColor.cream.opacity(0.4))
        )
        .overlay(
            Capsule().strokeBorder(AppColor.cardStroke, lineWidth: 0.6)
        )
        .dropDestination(for: DraggedEvent.self) { items, _ in
            guard let dropped = items.first else { return false }
            onDrop(dropped, calendar.id)
            return true
        }
        .help("Drop an event here to move it to \(calendar.summary)")
    }
}

struct CalendarSearchField: View {
    @Binding var text: String
    @FocusState private var isFocused: Bool

    var body: some View {
        HStack(spacing: 6) {
            Image(systemName: "magnifyingglass")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
            TextField("Filter events", text: $text)
                .textFieldStyle(.plain)
                .hcbScaledFrame(width: 180)
                .hcbFont(.subheadline)
                .focused($isFocused)
            if text.isEmpty == false {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear search")
            }
        }
        .hcbScaledPadding(.horizontal, 8)
        .hcbScaledPadding(.vertical, 4)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(.quaternary.opacity(0.4))
        )
        .onReceive(NotificationCenter.default.publisher(for: .hcbFocusCalendarSearch)) { _ in
            isFocused = true
        }
    }
}

struct EventReminderPicker: View {
    @Binding var selection: EventReminderOption
    @State private var customMinutes: String = ""
    @State private var isEditingCustom = false

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Picker("Alert", selection: Binding(
                get: { presetTag(for: selection) },
                set: { newTag in
                    if newTag == -2 {
                        isEditingCustom = true
                        if case .custom(let m) = selection {
                            customMinutes = "\(m)"
                        }
                    } else {
                        isEditingCustom = false
                        selection = EventReminderOption.presets.first { presetTag(for: $0) == newTag } ?? .useDefault
                    }
                }
            )) {
                ForEach(EventReminderOption.presets) { option in
                    Text(option.title).tag(presetTag(for: option))
                }
                Divider()
                Text("Custom…").tag(-2)
            }
            .accessibilityLabel("Event alert")
            .accessibilityHint("Choose when the reminder fires before the event starts")
            if case .custom(let m) = selection {
                Text("Currently \(EventReminderOption.label(forMinutes: m))")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            }
            if isEditingCustom {
                HStack(spacing: 8) {
                    TextField("Minutes before", text: $customMinutes)
                        .textFieldStyle(.roundedBorder)
                        .hcbScaledFrame(maxWidth: 120)
                        .accessibilityLabel("Custom minutes before event")
                    Button("Set") {
                        if let minutes = Int(customMinutes), minutes >= 0, minutes <= 40320 {
                            selection = .custom(minutes)
                            isEditingCustom = false
                        }
                    }
                    .disabled(Int(customMinutes).map { $0 < 0 || $0 > 40320 } ?? true)
                    Button("Cancel", role: .cancel) {
                        isEditingCustom = false
                        customMinutes = ""
                    }
                }
            }
        }
    }

    private func presetTag(for option: EventReminderOption) -> Int {
        switch option {
        case .useDefault: -1
        case .preset(let m), .custom(let m): m
        }
    }
}

struct EventColorPicker: View {
    @Binding var selection: CalendarEventColor

    private let columns = Array(repeating: GridItem(.flexible(minimum: 28), spacing: 8), count: 6)

    var body: some View {
        LazyVGrid(columns: columns, spacing: 8) {
            ForEach(CalendarEventColor.allCases) { color in
                Button {
                    selection = color
                } label: {
                    ZStack {
                        if let hex = color.hex {
                            Circle()
                                .fill(Color(hex: hex))
                                .hcbScaledFrame(width: 24, height: 24)
                        } else {
                            Circle()
                                .strokeBorder(AppColor.cardStroke, lineWidth: 1)
                                .hcbScaledFrame(width: 24, height: 24)
                                .overlay(
                                    Image(systemName: "slash.circle")
                                        .hcbFont(.caption2)
                                        .foregroundStyle(.secondary)
                                )
                        }
                        if selection == color {
                            Circle()
                                .strokeBorder(AppColor.ink, lineWidth: 2)
                                .hcbScaledFrame(width: 30, height: 30)
                        }
                    }
                    .hcbScaledFrame(width: 32, height: 32)
                }
                .buttonStyle(.plain)
                .help(color.title)
                .accessibilityLabel(color.title)
                .accessibilityAddTraits(selection == color ? [.isSelected, .isButton] : .isButton)
                .accessibilityHint("Sets event color to \(color.title)")
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Event color")
    }
}
struct AttendeeResponsesCard: View {
    let responses: [CalendarEventAttendee]

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("GUESTS (\(responses.count))")
                .hcbFont(.caption, weight: .bold)
                .foregroundStyle(.secondary)
            ForEach(responses, id: \.email) { attendee in
                GuestParticipantRow(
                    email: attendee.email,
                    displayName: attendee.displayName,
                    statusTitle: attendee.responseStatus.displayTitle,
                    statusSymbol: attendee.responseStatus.symbol,
                    statusTint: tint(for: attendee.responseStatus)
                )
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .cardSurface(cornerRadius: 18)
    }

    private func tint(for status: AttendeeResponseStatus) -> Color {
        switch status {
        case .accepted: AppColor.moss
        case .declined: AppColor.ember
        case .tentative: AppColor.blue
        case .needsAction: .secondary
        }
    }
}

struct CalendarTodayStatusHeader: View {
    let snapshot: TodaySnapshot
    let syncState: SyncState
    let pendingCount: Int
    var reachability: NetworkReachability = .unknown
    let refresh: () -> Void

    var body: some View {
        HStack(alignment: .center, spacing: 14) {
            VStack(alignment: .leading, spacing: 2) {
                Text(snapshot.date.formatted(.dateTime.weekday(.wide).month(.wide).day()))
                    .hcbFont(.headline)
                    .foregroundStyle(AppColor.ink)
                HStack(spacing: 6) {
                    Image(systemName: reachability.systemSymbol)
                        .hcbFont(.caption)
                        .foregroundStyle(reachabilityTint)
                        .help("Network: \(reachability.displayTitle)")
                    Text(syncState.title)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                    if pendingCount > 0 {
                        PendingSyncPill(count: pendingCount)
                    }
                }
            }
            Spacer(minLength: 12)
            StatusPill(value: "\(snapshot.dueTasks.count)", label: "Tasks")
            StatusPill(value: "\(snapshot.scheduledEvents.count)", label: "Events")
            StatusPill(value: "\(snapshot.overdueCount)", label: "Overdue", tint: snapshot.overdueCount > 0 ? AppColor.ember : nil)
            Button(action: refresh) {
                Label("Refresh", systemImage: "arrow.clockwise")
                    .labelStyle(.iconOnly)
            }
            .buttonStyle(.bordered)
            .accessibilityLabel("Refresh Google data")
        }
        .hcbScaledPadding(.horizontal, 16)
        .hcbScaledPadding(.vertical, 8)
    }

    private var reachabilityTint: Color {
        switch reachability {
        case .online: AppColor.moss
        case .constrained: AppColor.ember
        case .offline: .red
        case .unknown: .secondary
        }
    }
}

struct PendingSyncPill: View {
    let count: Int

    var body: some View {
        Label("\(count) pending", systemImage: "icloud.slash")
            .font(.caption2.weight(.semibold).monospacedDigit())
            .labelStyle(.titleAndIcon)
            .hcbScaledPadding(.horizontal, 8)
            .hcbScaledPadding(.vertical, 2)
            .background(Capsule().fill(AppColor.ember.opacity(0.18)))
            .foregroundStyle(AppColor.ember)
            .help("Tasks or events created while offline are waiting for Google to accept them.")
            .accessibilityLabel("\(count) pending sync operations")
    }
}

private struct StatusPill: View {
    let value: String
    let label: String
    var tint: Color? = nil

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            Text(value)
                .font(.title3.bold().monospacedDigit())
                .foregroundStyle(tint ?? AppColor.ink)
            Text(label)
                .hcbFont(.caption2, weight: .medium)
                .foregroundStyle(.secondary)
        }
        .hcbScaledPadding(.vertical, 4)
        .hcbScaledPadding(.horizontal, 10)
        .background(
            Capsule().fill((tint ?? AppColor.cream).opacity(0.35))
        )
    }
}

#Preview {
    NavigationStack {
        CalendarHomeView()
            .environment(AppModel.preview)
            .environment(\.routerPath, RouterPath())
    }
}
