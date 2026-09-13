import SwiftUI

struct DayGridView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.routerPath) private var router
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.hcbAppBackgroundConfiguration) private var backgroundConfiguration
    @Environment(\.calendarEventViewFilter) private var calendarEventViewFilter
    @Binding var anchorDate: Date
    var searchQuery: String = ""
    var availabilitySelection: AvailabilityGridSelection?

    private let hourHeight: CGFloat = 48
    private let hourStart = 0
    private let hourEnd = 24
    private let calendar = Calendar.current
    private var calendarGridReduceMotion: Bool {
        reduceMotion || scenePhase != .active || ProcessInfo.processInfo.isLowPowerModeEnabled
    }
    private var usesReadableCalendarBackings: Bool {
        backgroundConfiguration.customImagePath != nil || backgroundConfiguration.isTranslucent
    }

    @State private var timedDrag: TimedDrag?
    // Click-to-create feedback. Flashes a brief tint at the tapped hour
    // slot so users see their click register before the popover paints.
    @State private var flashTimedSlot: Date?
    @State private var preparedDaySnapshot: CalendarDayDisplaySnapshot?
    @State private var daySnapshotBuildTask: Task<Void, Never>?
    @State private var daySnapshotPrewarmTask: Task<Void, Never>?

    private var calendarStore: CalendarStore { model.calendarStore }
    private var showsInlineTimedSlotButtons: Bool {
        calendarStore.settings.performanceMode == .rich
    }

    private struct TimedDrag: Equatable {
        var startY: CGFloat
        var endY: CGFloat
    }

    var body: some View {
        ZStack {
            if let snapshot = preparedDaySnapshot, snapshot.key == daySnapshotKey, calendarStore.isRebuildingDerivedSnapshots == false {
                eventsColumn(snapshot)
                    .frame(maxWidth: .infinity)
                    .hcbScaledPadding(12)
            } else {
                PreparedSnapshotOverlay(
                    title: "Preparing day...",
                    message: "Laying out events before enabling interactions."
                )
                .onAppear { rebuildDaySnapshotIfNeeded() }
            }
        }
        .background { readableCalendarBackdrop }
        .onAppear { rebuildDaySnapshotIfNeeded() }
        .onChange(of: daySnapshotKey) { _, _ in rebuildDaySnapshotIfNeeded() }
        .onDisappear {
            daySnapshotBuildTask?.cancel()
            daySnapshotPrewarmTask?.cancel()
        }
        .hcbDebugBodyProbe("DayGridView")
    }

    @ViewBuilder
    private var readableCalendarBackdrop: some View {
        if usesReadableCalendarBackings {
            Rectangle()
                .fill(AppColor.cardSurface.opacity(0.84))
                .overlay(AppColor.cream.opacity(0.18))
        }
    }

    private var dayStart: Date { calendar.startOfDay(for: anchorDate) }
    private var dayEnd: Date { calendar.date(byAdding: .day, value: 1, to: dayStart) ?? dayStart }

    private var daySnapshotKey: PreparedSnapshotKey {
        daySnapshotKey(for: anchorDate)
    }

    private func daySnapshotKey(for date: Date) -> PreparedSnapshotKey {
        PreparedSnapshotKeys.calendar(
            mode: .day,
            dataRevision: calendarStore.calendarDisplayRevisionKey(for: [calendar.startOfDay(for: date)], calendar: calendar),
            selectedCalendarIDs: calendarStore.calendarSnapshot.selectedCalendarIDs,
            visibleTaskListIDs: calendarStore.visibleTaskListIDs,
            filterKey: calendarEventViewFilter.cacheKey,
            searchQuery: searchQuery,
            rangeKey: PreparedSnapshotKeys.dateKey(date, calendar: calendar),
            settings: calendarStore.settings
        )
    }

    private func eventsColumn(_ snapshot: CalendarDayDisplaySnapshot) -> some View {
        // Capture router locally so the DragGesture / SpatialTapGesture
        // closures inside ScrollView reference a stable reference (custom
        // EnvironmentKey reads inside ScrollView/GeometryReader gesture
        // closures have shown propagation gaps).
        let capturedRouter = router
        return VStack(alignment: .leading, spacing: 8) {
            if snapshot.allDayEvents.isEmpty == false {
                VStack(alignment: .leading, spacing: 4) {
                    Text("ALL-DAY")
                        .hcbFont(.caption2, weight: .bold)
                        .foregroundStyle(.secondary)
                    ForEach(snapshot.allDayEvents.prefix(snapshot.density.visibleAllDayEventCount)) { event in
                        allDayEventTile(event, snapshot: snapshot)
                    }
                    if snapshot.density.hiddenAllDayEventCount > 0 {
                        MonthMoreButton(
                            count: snapshot.density.hiddenAllDayEventCount,
                            day: snapshot.dayStart,
                            events: snapshot.allDayEvents,
                            tasks: [],
                            calendarColor: { calendarColor(for: $0, in: snapshot) }
                        )
                    }
                }
            }

            ScrollView {
                ZStack(alignment: .topLeading) {
                    hourGridBackground
                    Rectangle()
                        .fill(Color.clear)
                        .contentShape(Rectangle())
                        .frame(height: CGFloat(hourEnd - hourStart) * hourHeight)
                        .overlay(alignment: .topLeading) {
                            if let drag = timedDrag {
                                let top = min(drag.startY, drag.endY)
                                let height = max(abs(drag.endY - drag.startY), hourHeight / 4)
                                RoundedRectangle(cornerRadius: 6)
                                    .fill(AppColor.ember.opacity(0.22))
                                    .overlay(
                                        RoundedRectangle(cornerRadius: 6)
                                            .strokeBorder(AppColor.ember.opacity(0.6), lineWidth: 1.2)
                                    )
                                    .offset(x: 56, y: top)
                                    .frame(height: height)
                                    .hcbScaledPadding(.trailing, 8)
                                    .allowsHitTesting(false)
                            }
                        }
                        .gesture(
                            // Drag creates a time-blocked event; a short tap
                            // (< minimumDistance) falls through to quick-create.
                            DragGesture(minimumDistance: 6)
                                .onChanged { value in
                                    timedDrag = TimedDrag(startY: value.startLocation.y, endY: value.location.y)
                                }
                                .onEnded { value in
                                    let start = CalendarDropComputer.snappedStart(
                                        for: min(value.startLocation.y, value.location.y),
                                        hourHeight: hourHeight,
                                        dayStart: dayStart,
                                        calendar: calendar
                                    )
                                    let end = CalendarDropComputer.snappedStart(
                                        for: max(value.startLocation.y, value.location.y),
                                        hourHeight: hourHeight,
                                        dayStart: dayStart,
                                        calendar: calendar
                                    )
                                    timedDrag = nil
                                    let adjustedEnd = end <= start ? start.addingTimeInterval(1800) : end
                                    if let availabilitySelection {
                                        selectAvailabilitySlot(
                                            AvailabilitySlot(startDate: start, endDate: adjustedEnd),
                                            using: availabilitySelection
                                        )
                                        return
                                    }
                                    capturedRouter?.present(.quickCreateRange(start, adjustedEnd, allDay: false))
                                }
                        )
                        .simultaneousGesture(
                            SpatialTapGesture(count: 1)
                                .onEnded { value in
                                    guard timedDrag == nil else { return }
                                    let start = CalendarDropComputer.snappedStart(
                                        for: value.location.y,
                                        hourHeight: hourHeight,
                                        dayStart: dayStart,
                                        calendar: calendar
                                    )
                                    createTimedSlot(start: start, router: capturedRouter)
                                }
                        )
                    GeometryReader { geo in
                        ZStack(alignment: .topLeading) {
                            availabilitySlotOverlays(
                                dayStart: snapshot.dayStart,
                                dayEnd: snapshot.dayEnd,
                                availableWidth: geo.size.width
                            )
                            ForEach(Array(snapshot.laidOutTimedEvents.enumerated()), id: \.offset) { _, placed in
                                eventTile(placed, availableWidth: geo.size.width - 56, snapshot: snapshot)
                                    .opacity(snapshot.eventMetadataByID[placed.event.id]?.opacity ?? 1.0)
                            }
                            if snapshot.density.hiddenTimedEventCount > 0 {
                                denseTimedOverflowButton(snapshot)
                                    .frame(width: max(geo.size.width - 64, 120), height: 26, alignment: .leading)
                                    .offset(x: 56, y: CGFloat(hourEnd - hourStart) * hourHeight - 34)
                            }
                        }
                    }
                    .frame(height: CGFloat(hourEnd - hourStart) * hourHeight)
                    .allowsHitTesting(true)
                    if showsInlineTimedSlotButtons {
                        dayTimedSlotButtons(dayStart: dayStart, router: capturedRouter)
                    }
                    timedSlotFlashOverlay(dayStart: dayStart)
                    if let offset = currentTimeOffset() {
                        Rectangle()
                            .fill(AppColor.ember)
                            .hcbScaledFrame(height: 1)
                            .offset(x: 52, y: offset)
                    }
                }
                .frame(minHeight: CGFloat(hourEnd - hourStart) * hourHeight)
            }
        }
    }

    @ViewBuilder
    private func timedSlotFlashOverlay(dayStart: Date) -> some View {
        // Momentary tint over the tapped hour slot so the click lands visibly
        // before the popover paints.
        if let flash = flashTimedSlot {
            let minutesIntoDay = flash.timeIntervalSince(dayStart) / 60.0
            let startHourOffset = Double(hourStart) * 60.0
            let y = CGFloat(max(0, minutesIntoDay - startHourOffset)) / 60.0 * hourHeight
            RoundedRectangle(cornerRadius: 6)
                .fill(AppColor.ember.opacity(0.22))
                .frame(height: hourHeight * 0.5)
                .offset(x: 56, y: y)
                .padding(.trailing, 8)
                .allowsHitTesting(false)
                .transition(.opacity)
                .animation(HCBMotion.animation(.easeOut(duration: 0.18), reduceMotion: calendarGridReduceMotion), value: flashTimedSlot)
        }
    }

    private func dayTimedSlotButtons(dayStart: Date, router capturedRouter: RouterPath?) -> some View {
        VStack(spacing: 0) {
            ForEach(hourStart..<hourEnd, id: \.self) { hour in
                let start = calendar.date(byAdding: .hour, value: hour, to: dayStart) ?? dayStart
                HStack {
                    Spacer(minLength: 0)
                    Button {
                        createTimedSlot(start: start, router: capturedRouter)
                    } label: {
                        Image(systemName: "plus.circle")
                            .imageScale(.small)
                    }
                    .buttonStyle(.borderless)
                    .controlSize(.mini)
                    .help(timedSlotActionTitle(for: start))
                    .accessibilityLabel(timedSlotAccessibilityLabel(for: start))
                }
                .hcbScaledFrame(width: 54, height: hourHeight, alignment: .topTrailing)
            }
        }
        .accessibilityElement(children: .contain)
    }

    private func createTimedSlot(start: Date, router capturedRouter: RouterPath?) {
        if let availabilitySelection {
            let end = calendar.date(
                byAdding: .minute,
                value: max(15, availabilitySelection.defaultDurationMinutes),
                to: start
            ) ?? start.addingTimeInterval(1800)
            selectAvailabilitySlot(
                AvailabilitySlot(startDate: start, endDate: end),
                using: availabilitySelection
            )
            return
        }
        flashTimedStart(start)
        capturedRouter?.present(.quickCreate(start, allDay: false))
    }

    private func timedSlotActionTitle(for start: Date) -> String {
        let time = start.formatted(.dateTime.hour().minute())
        if availabilitySelection != nil {
            return "Select availability at \(time)"
        }
        return "New event at \(time)"
    }

    private func timedSlotAccessibilityLabel(for start: Date) -> String {
        let dateTime = start.formatted(.dateTime.weekday(.wide).month(.wide).day().hour().minute())
        if availabilitySelection != nil {
            return "Select availability on \(dateTime)"
        }
        return "New event on \(dateTime)"
    }

    private var hourGridBackground: some View {
        VStack(alignment: .leading, spacing: 0) {
            ForEach(hourStart..<hourEnd, id: \.self) { hour in
                HStack(alignment: .top, spacing: 8) {
                    Text(hourLabel(hour))
                        .font(.caption2.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .hcbScaledFrame(width: 48, alignment: .trailing)
                    Rectangle()
                        .fill(AppColor.cardStroke)
                        .hcbScaledFrame(height: 0.5)
                        .frame(maxWidth: .infinity)
                }
                .frame(height: hourHeight, alignment: .top)
            }
        }
    }

    // Flash the tapped timed slot for ~220ms.
    private func flashTimedStart(_ start: Date) {
        flashTimedSlot = start
        Task {
            try? await Task.sleep(for: .milliseconds(220))
            if flashTimedSlot == start { flashTimedSlot = nil }
        }
    }

    private func hourLabel(_ hour: Int) -> String {
        CalendarHourLabelCache.label(for: hour)
    }

    private func allDayEventTile(_ event: CalendarEventMirror, snapshot: CalendarDayDisplaySnapshot) -> some View {
        let fill = calendarColor(for: event, in: snapshot)
        return CalendarEventPreviewButton(event: event) {
            Text(event.summary)
                .hcbFont(.caption, weight: .medium)
                .lineLimit(1)
                .opacity(snapshot.eventMetadataByID[event.id]?.opacity ?? 1.0)
                .hcbScaledPadding(.horizontal, 8)
                .hcbScaledPadding(.vertical, 4)
                .frame(maxWidth: .infinity, alignment: .leading)
                .background(Capsule().fill(fill.opacity(0.25)))
                .foregroundStyle(AppColor.ink)
        }
        .accessibilityLabel("\(event.summary), all day")
    }

    private func denseTimedOverflowButton(_ snapshot: CalendarDayDisplaySnapshot) -> some View {
        MonthMoreButton(
            count: snapshot.density.hiddenTimedEventCount,
            day: snapshot.dayStart,
            events: snapshot.timedEvents,
            tasks: [],
            calendarColor: { calendarColor(for: $0, in: snapshot) }
        )
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 6, style: .continuous)
                .strokeBorder(AppColor.cardStroke.opacity(0.45), lineWidth: 0.6)
        )
    }

    @ViewBuilder
    private func eventTile(
        _ placed: CalendarGridLayout.LaidOutEvent,
        availableWidth: CGFloat,
        snapshot: CalendarDayDisplaySnapshot
    ) -> some View {
        let event = placed.event
        let clampedStart = max(event.startDate, snapshot.dayStart)
        let clampedEnd = min(event.endDate, snapshot.dayEnd)
        let startMinutes = clampedStart.timeIntervalSince(snapshot.dayStart) / 60
        let durationMinutes = max(clampedEnd.timeIntervalSince(clampedStart) / 60, 20)
        let yOffset = CGFloat(startMinutes) * (hourHeight / 60)
        let height = CGFloat(durationMinutes) * (hourHeight / 60)
        let slotWidth = availableWidth / CGFloat(max(placed.columnCount, 1))
        let xOffsetWithinDay = CGFloat(placed.columnIndex) * slotWidth
        let tileWidth = max(slotWidth - 3, 1)
        let tileHeight = max(height - 2, 1)
        let fill = calendarColor(for: event, in: snapshot)

        if snapshot.density.usesDenseTimedRendering {
            Button {
                router?.present(.editEvent(event.id))
            } label: {
                VStack(alignment: .leading, spacing: 1) {
                    Text(event.summary)
                        .hcbFont(.caption2, weight: .semibold)
                        .lineLimit(1)
                }
                .hcbScaledPadding(.horizontal, 5)
                .hcbScaledPadding(.vertical, 3)
                .frame(width: tileWidth, height: tileHeight, alignment: .topLeading)
                .clipped()
                .background(RoundedRectangle(cornerRadius: 5).fill(fill.opacity(0.18)))
                .overlay(RoundedRectangle(cornerRadius: 5).strokeBorder(fill.opacity(0.42), lineWidth: 0.6))
            }
            .buttonStyle(.plain)
            .offset(x: 56 + xOffsetWithinDay, y: yOffset)
            .accessibilityLabel(snapshot.eventMetadataByID[event.id]?.accessibilityLabel ?? event.summary)
        } else {
            CalendarEventPreviewButton(event: event) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(event.summary)
                        .hcbFont(.caption, weight: .semibold)
                        .lineLimit(height > 38 ? 2 : 1)
                    if height > 38 {
                        Text(snapshot.eventMetadataByID[event.id]?.timeRangeLabel ?? "")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                    if height > 60, slotWidth > 120, event.location.isEmpty == false {
                        Text(event.location)
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                .hcbScaledPadding(.horizontal, 8)
                .hcbScaledPadding(.vertical, 4)
                .frame(width: tileWidth, height: tileHeight, alignment: .topLeading)
                .clipped()
                .background(RoundedRectangle(cornerRadius: 8).fill(fill.opacity(0.22)))
                .overlay(RoundedRectangle(cornerRadius: 8).strokeBorder(fill.opacity(0.55), lineWidth: 0.8))
            }
            .offset(x: 56 + xOffsetWithinDay, y: yOffset)
            .accessibilityLabel(snapshot.eventMetadataByID[event.id]?.accessibilityLabel ?? event.summary)
        }
    }

    @ViewBuilder
    private func availabilitySlotOverlays(
        dayStart: Date,
        dayEnd: Date,
        availableWidth: CGFloat
    ) -> some View {
        if let availabilitySelection {
            ForEach(availabilitySelection.slots.filter { $0.startDate < dayEnd && $0.endDate > dayStart }) { slot in
                let clampedStart = max(slot.startDate, dayStart)
                let clampedEnd = min(slot.endDate, dayEnd)
                let startMinutes = clampedStart.timeIntervalSince(dayStart) / 60
                let durationMinutes = max(clampedEnd.timeIntervalSince(clampedStart) / 60, 15)
                let yOffset = CGFloat(startMinutes) * (hourHeight / 60)
                let height = CGFloat(durationMinutes) * (hourHeight / 60)
                RoundedRectangle(cornerRadius: 6)
                    .fill(AppColor.blue.opacity(0.18))
                    .overlay(
                        RoundedRectangle(cornerRadius: 6)
                            .strokeBorder(AppColor.blue.opacity(0.7), lineWidth: 1)
                    )
                    .frame(width: max(availableWidth - 64, 40), height: max(height - 2, 12))
                    .offset(x: 56, y: yOffset)
                    .allowsHitTesting(false)
                    .accessibilityHidden(true)
            }
        }
    }

    private func selectAvailabilitySlot(
        _ slot: AvailabilitySlot,
        using selection: AvailabilityGridSelection
    ) {
        if AvailabilitySlotResolver.overlapsSelectedSlots(slot, selectedSlots: selection.slots) {
            selection.onReject("That slot overlaps another selected slot.")
            return
        }
        guard selection.isSlotAvailable(slot) else {
            selection.onReject("That slot overlaps a blocking event.")
            return
        }
        selection.onSelect(slot)
    }

    private func currentTimeOffset() -> CGFloat? {
        guard calendar.isDate(anchorDate, inSameDayAs: Date()) else { return nil }
        let now = Date()
        guard now >= dayStart, now <= dayEnd else { return nil }
        let minutes = now.timeIntervalSince(dayStart) / 60
        return CGFloat(minutes) * (hourHeight / 60)
    }

    private func rebuildDaySnapshotIfNeeded() {
        let key = daySnapshotKey
        guard preparedDaySnapshot?.key != key else { return }
        if let snapshot = calendarStore.cachedCalendarDaySnapshot(for: key) {
            preparedDaySnapshot = snapshot
            prewarmAdjacentDaySnapshots(around: anchorDate)
            return
        }
        daySnapshotBuildTask?.cancel()
        daySnapshotBuildTask = Task { @MainActor in
            let started = HCBPerformanceTelemetry.timestamp()
            let input = await model.calendarDisplayInputFromQuery(
                key: key,
                kind: .day,
                anchorDate: anchorDate,
                eventViewFilter: calendarEventViewFilter,
                searchQuery: searchQuery,
                referenceDate: Date(),
                calendar: calendar
            )
            let snapshot = await Task.detached(priority: .utility) {
                CalendarDisplaySnapshotBuilder.daySnapshot(input)
            }.value
            guard Task.isCancelled == false, snapshot.key == daySnapshotKey else { return }
            calendarStore.storeCalendarDaySnapshot(snapshot)
            preparedDaySnapshot = snapshot
            prewarmAdjacentDaySnapshots(around: anchorDate)
            HCBPerformanceTelemetry.debug(
                "calendar day snapshot built",
                metadata: [
                    "buildMs": HCBPerformanceTelemetry.elapsedMilliseconds(since: started),
                    "events": "\(snapshot.allDayEvents.count + snapshot.timedEvents.count)"
                ]
            )
        }
    }

    private func prewarmAdjacentDaySnapshots(around date: Date) {
        guard calendarStore.settings.performanceMode == .snappy else { return }
        let base = calendar.startOfDay(for: date)
        let adjacentDays = [-1, 1].compactMap { calendar.date(byAdding: .day, value: $0, to: base) }
        guard adjacentDays.isEmpty == false else { return }
        daySnapshotPrewarmTask?.cancel()
        daySnapshotPrewarmTask = Task { @MainActor in
            for prewarmDate in adjacentDays {
                let key = daySnapshotKey(for: prewarmDate)
                if calendarStore.cachedCalendarDaySnapshot(for: key) != nil { continue }
                let input = await model.calendarDisplayInputFromQuery(
                    key: key,
                    kind: .day,
                    anchorDate: prewarmDate,
                    eventViewFilter: calendarEventViewFilter,
                    searchQuery: searchQuery,
                    referenceDate: Date(),
                    calendar: calendar
                )
                let snapshot = await Task.detached(priority: .utility) {
                    CalendarDisplaySnapshotBuilder.daySnapshot(input)
                }.value
                guard Task.isCancelled == false, snapshot.key == key else { return }
                calendarStore.storeCalendarDaySnapshot(snapshot)
            }
        }
    }

    private func calendarColor(for event: CalendarEventMirror, in snapshot: CalendarDayDisplaySnapshot) -> Color {
        guard let hex = snapshot.eventMetadataByID[event.id]?.colorHex else { return AppColor.blue }
        return Color(hex: hex)
    }
}
