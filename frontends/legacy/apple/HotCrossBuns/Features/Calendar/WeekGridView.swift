import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct WeekGridView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.routerPath) private var router
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.hcbAppBackgroundConfiguration) private var backgroundConfiguration
    @Environment(\.calendarEventViewFilter) private var calendarEventViewFilter
    @Binding var anchorDate: Date
    var searchQuery: String = ""
    @Binding var selectedEventIDs: Set<String>
    var availabilitySelection: AvailabilityGridSelection?
    // §7.01 Phase D2 — multi-day variant. When non-nil, renders N consecutive
    // days starting at `anchorDate` instead of the full calendar week. nil
    // preserves the classic 7-day week-aligned layout.
    var multiDayCount: Int? = nil

    init(
        anchorDate: Binding<Date>,
        searchQuery: String = "",
        selectedEventIDs: Binding<Set<String>> = .constant([]),
        multiDayCount: Int? = nil,
        availabilitySelection: AvailabilityGridSelection? = nil
    ) {
        _anchorDate = anchorDate
        self.searchQuery = searchQuery
        _selectedEventIDs = selectedEventIDs
        self.multiDayCount = multiDayCount
        self.availabilitySelection = availabilitySelection
    }

    private let hourHeight: CGFloat = 44
    private let hourStart = 0
    private let hourEnd = 24
    private let allDayLaneHeight: CGFloat = 22
    private let allDayVisibleLimit = 3
    private let taskVisibleLimit = 3
    private let taskRowHeight: CGFloat = 24
    private let taskRowSpacing: CGFloat = 2
    private let calendar = Calendar.current
    private var calendarGridReduceMotion: Bool {
        reduceMotion || scenePhase != .active || ProcessInfo.processInfo.isLowPowerModeEnabled
    }
    private var usesReadableCalendarBackings: Bool {
        backgroundConfiguration.customImagePath != nil || backgroundConfiguration.isTranslucent
    }

    @State private var allDayDrag: WeekDaySelection?
    @State private var timedDrag: TimedWeekDrag?
    // Click-to-create feedback. flashTimedSlot is the tapped start-date for
    // a timed slot; clears after ~220ms so the user sees an acknowledgement
    // before the quick-create popover paints. Independent from timedDrag so
    // drag previews still behave.
    @State private var flashTimedSlot: Date?
    @State private var preparedWeekSnapshot: CalendarWeekDisplaySnapshot?
    @State private var weekSnapshotBuildTask: Task<Void, Never>?
    @State private var weekSnapshotPrewarmTask: Task<Void, Never>?

    private var calendarStore: CalendarStore { model.calendarStore }
    private var isWeekHoverPreviewEnabled: Bool {
        calendarStore.settings.performanceMode == .rich
    }
    private var showsInlineTimedSlotMenus: Bool {
        calendarStore.settings.performanceMode == .rich
    }

    private struct WeekDaySelection: Equatable {
        var startCol: Int
        var endCol: Int
        var normalized: (Int, Int) {
            startCol <= endCol ? (startCol, endCol) : (endCol, startCol)
        }
    }

    // Grid-level timed drag: carries both axes so a drag that starts in
    // Tuesday 10 AM and ends in Thursday 2 PM creates a multi-day event
    // (Tue 10 AM → Thu 2 PM) instead of clamping to the starting column.
    private struct TimedWeekDrag: Equatable {
        var startColumn: Int
        var startY: CGFloat
        var endColumn: Int
        var endY: CGFloat
    }

    var body: some View {
        ZStack {
            if let snapshot = preparedWeekSnapshot, snapshot.key == weekSnapshotKey, calendarStore.isRebuildingDerivedSnapshots == false {
                VStack(spacing: 0) {
                    weekHeader(snapshot)
                    Divider()
                    allDayStrip(snapshot)
                    tasksStrip(snapshot)
                    Divider()
                    ScrollView {
                        timeGrid(snapshot)
                    }
                }
            } else {
                PreparedSnapshotOverlay(
                    title: multiDayCount == nil ? "Preparing week..." : "Preparing days...",
                    message: "Laying out events and tasks before enabling interactions."
                )
                .onAppear { rebuildWeekSnapshotIfNeeded() }
            }
        }
        .background { readableCalendarBackdrop }
        .onAppear { rebuildWeekSnapshotIfNeeded() }
        .onChange(of: weekSnapshotKey) { _, _ in rebuildWeekSnapshotIfNeeded() }
        .onDisappear {
            weekSnapshotBuildTask?.cancel()
            weekSnapshotPrewarmTask?.cancel()
        }
        .hcbDebugBodyProbe("WeekGridView")
    }

    @ViewBuilder
    private var readableCalendarBackdrop: some View {
        if usesReadableCalendarBackings {
            Rectangle()
                .fill(AppColor.cardSurface.opacity(0.84))
                .overlay(AppColor.cream.opacity(0.18))
        }
    }

    private var weekSnapshotKey: PreparedSnapshotKey {
        weekSnapshotKey(for: weekDays)
    }

    private func weekSnapshotKey(for days: [Date]) -> PreparedSnapshotKey {
        PreparedSnapshotKeys.calendar(
            mode: multiDayCount == nil ? .week : .multiDay,
            dataRevision: calendarStore.calendarDisplayRevisionKey(for: days, calendar: calendar),
            selectedCalendarIDs: calendarStore.calendarSnapshot.selectedCalendarIDs,
            visibleTaskListIDs: calendarStore.visibleTaskListIDs,
            filterKey: calendarEventViewFilter.cacheKey,
            searchQuery: searchQuery,
            rangeKey: PreparedSnapshotKeys.dateRangeKey(days),
            settings: calendarStore.settings
        )
    }

    private var weekDays: [Date] {
        weekDays(for: anchorDate)
    }

    private func weekDays(for date: Date) -> [Date] {
        if let count = multiDayCount, count > 0 {
            let start = calendar.startOfDay(for: date)
            return (0..<count).compactMap { calendar.date(byAdding: .day, value: $0, to: start) }
        }
        return CalendarGridLayout.weekDays(containing: date, calendar: calendar)
    }

    private func weekHeader(_ snapshot: CalendarWeekDisplaySnapshot) -> some View {
        HStack(spacing: 0) {
            Text("")
                .hcbScaledFrame(width: 54)
            ForEach(snapshot.dayLabels) { label in
                VStack(spacing: 2) {
                    Text(label.weekday)
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                    Text(label.dayNumber)
                        .font(.title3.weight(label.isToday ? .bold : .regular))
                        .foregroundStyle(label.isToday ? AppColor.ember : AppColor.ink)
                        .hcbScaledFrame(width: 30, height: 30)
                        .background(
                            Circle()
                                .fill(label.isToday ? AppColor.ember.opacity(0.15) : .clear)
                        )
                }
                .frame(maxWidth: .infinity)
            }
        }
        .hcbScaledPadding(.vertical, 10)
    }

    private func allDayStrip(_ snapshot: CalendarWeekDisplaySnapshot) -> some View {
        let spans = snapshot.allDaySpans
        let laneCount = (spans.map(\.laneIndex).max() ?? -1) + 1
        let visibleLaneCount = min(laneCount, allDayVisibleLimit)
        let hasOverflow = laneCount > allDayVisibleLimit
        let rowCount = max(visibleLaneCount + (hasOverflow ? 1 : 0), 1)
        let stripHeight = CGFloat(rowCount) * allDayLaneHeight
        let visibleSpans = spans.filter { $0.laneIndex < allDayVisibleLimit }
        let allDayByDay = snapshot.allDayEventsByDay
        // Captured outside GeometryReader so DragGesture closures see a
        // reliable router reference (custom-EnvironmentKey reads inside
        // GeometryReader closures have shown propagation gaps).
        let capturedRouter = router
        return HStack(spacing: 0) {
            HStack(spacing: 3) {
                Text("All-day")
                    .hcbFont(.caption2, weight: .semibold)
                    .foregroundStyle(.secondary)
                weekAllDayCreateMenu(days: snapshot.days, router: capturedRouter)
            }
            .hcbScaledFrame(width: 54, alignment: .trailing)
            .hcbScaledPadding(.trailing, 4)
            GeometryReader { geo in
                let columnWidth = geo.size.width / CGFloat(max(snapshot.days.count, 1))
                let laneHeight: CGFloat = 22
                ZStack(alignment: .topLeading) {
                    Rectangle()
                        .fill(Color.clear)
                        .contentShape(Rectangle())
                    if let drag = allDayDrag {
                        let (a, b) = drag.normalized
                        let left = CGFloat(a) * columnWidth
                        let width = CGFloat(b - a + 1) * columnWidth
                        RoundedRectangle(cornerRadius: 6)
                            .fill(AppColor.ember.opacity(0.22))
                            .overlay(
                                RoundedRectangle(cornerRadius: 6)
                                    .strokeBorder(AppColor.ember.opacity(0.6), lineWidth: 1.2)
                            )
                            .frame(width: max(width - 4, 4), height: stripHeight - 4)
                            .offset(x: left + 2, y: 2)
                            .allowsHitTesting(false)
                    }
                    ForEach(visibleSpans) { span in
                        allDaySpanTile(span, columnWidth: columnWidth, laneHeight: laneHeight, snapshot: snapshot)
                    }
                    ForEach(Array(snapshot.days.enumerated()), id: \.offset) { idx, day in
                        let dayStart = calendar.startOfDay(for: day)
                        let dayStartKey = CalendarDisplaySnapshotBuilder.dayKey(dayStart, calendar: calendar)
                        let events = allDayByDay[dayStartKey] ?? []
                        let hiddenCount = hiddenAllDaySpanCount(onColumn: idx, spans: spans)
                        if hiddenCount > 0 {
                            MonthMoreButton(
                                count: hiddenCount,
                                day: dayStart,
                                events: events,
                                tasks: [],
                                calendarColor: { calendarColor(for: $0, in: snapshot) }
                            )
                            .frame(width: max(columnWidth - 4, 20), height: laneHeight - 4, alignment: .leading)
                            .offset(
                                x: CGFloat(idx) * columnWidth + 2,
                                y: CGFloat(visibleLaneCount) * laneHeight + 2
                            )
                        }
                    }
                }
                .contentShape(Rectangle())
                .gesture(
                    DragGesture(minimumDistance: 6)
                        .onChanged { value in
                            let startCol = columnIndex(for: value.startLocation.x, columnWidth: columnWidth)
                            let endCol = columnIndex(for: value.location.x, columnWidth: columnWidth)
                            allDayDrag = WeekDaySelection(startCol: startCol, endCol: endCol)
                        }
                        .onEnded { _ in
                            guard let drag = allDayDrag else { return }
                            allDayDrag = nil
                            let (a, b) = drag.normalized
                            guard snapshot.days.indices.contains(a), snapshot.days.indices.contains(b) else { return }
                            let start = calendar.startOfDay(for: snapshot.days[a])
                            let endExclusive = calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: snapshot.days[b])) ?? snapshot.days[b]
                            capturedRouter?.present(.quickCreateRange(start, endExclusive, allDay: true))
                        }
                )
            }
            .frame(height: stripHeight)
            .clipped()
        }
        .frame(minHeight: stripHeight, idealHeight: stripHeight, maxHeight: stripHeight)
        .clipped()
        .hcbScaledPadding(.vertical, 4)
    }

    private func weekAllDayCreateMenu(days: [Date], router capturedRouter: RouterPath?) -> some View {
        Menu {
            ForEach(Array(days.enumerated()), id: \.offset) { _, day in
                let dayStart = calendar.startOfDay(for: day)
                Button(dayStart.formatted(.dateTime.weekday(.wide).month(.abbreviated).day())) {
                    capturedRouter?.present(.quickCreate(dayStart, allDay: true))
                }
            }
        } label: {
            Image(systemName: "plus.circle")
                .imageScale(.small)
        }
        .menuStyle(.borderlessButton)
        .controlSize(.mini)
        .fixedSize()
        .help("New all-day event")
        .accessibilityLabel("New all-day event")
    }

    private func hiddenAllDaySpanCount(onColumn column: Int, spans: [CalendarWeekDisplaySnapshot.AllDaySpan]) -> Int {
        spans.filter { span in
            span.laneIndex >= allDayVisibleLimit
                && column >= span.startColumn
                && column <= span.endColumn
        }.count
    }

    private func columnIndex(for x: CGFloat, columnWidth: CGFloat) -> Int {
        guard columnWidth > 0 else { return 0 }
        let lastColumn = max(weekDays.count - 1, 0)
        return max(0, min(lastColumn, Int(x / columnWidth)))
    }

    private func allDaySpanTile(
        _ span: CalendarWeekDisplaySnapshot.AllDaySpan,
        columnWidth: CGFloat,
        laneHeight: CGFloat,
        snapshot: CalendarWeekDisplaySnapshot
    ) -> some View {
        let x = CGFloat(span.startColumn) * columnWidth + 2
        let width = CGFloat(span.columnCount) * columnWidth - 4
        let y = CGFloat(span.laneIndex) * laneHeight + 2
        let fill = calendarColor(for: span.event, in: snapshot)
        return CalendarEventPreviewButton(event: span.event) {
            Text(span.event.summary)
                .hcbFont(.caption)
                .lineLimit(1)
                .hcbScaledPadding(.horizontal, 6)
                .hcbScaledPadding(.vertical, 3)
                .frame(width: max(width, 20), height: laneHeight - 4, alignment: .leading)
                .background(
                    RoundedRectangle(cornerRadius: 5)
                        .fill(fill.opacity(0.3))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 5)
                        .strokeBorder(fill.opacity(0.5), lineWidth: 0.8)
                )
                .foregroundStyle(AppColor.ink)
        }
        .offset(x: x, y: y)
        .accessibilityLabel(snapshot.eventMetadataByID[span.event.id]?.accessibilityLabel ?? span.event.summary)
    }

    private func exportSingleEventICS(_ event: CalendarEventMirror) {
        let ics = EventICSExporter.ics(for: event)
        let sanitized = event.summary
            .replacingOccurrences(of: "/", with: "-")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let base = sanitized.isEmpty ? "event" : sanitized
        let panel = NSSavePanel()
        panel.nameFieldStringValue = "\(base).ics"
        panel.allowedContentTypes = [UTType(filenameExtension: "ics") ?? .data]
        panel.canCreateDirectories = true
        if panel.runModal() == .OK, let url = panel.url {
            try? ics.data(using: .utf8)?.write(to: url)
        }
    }

    private func copyEventMarkdown(_ event: CalendarEventMirror) {
        let title = calendarStore.calendars.first(where: { $0.id == event.calendarID })?.summary
        let md = EventMarkdownExporter.markdown(for: event, calendarTitle: title)
        NSPasteboard.general.clearContents()
        NSPasteboard.general.setString(md, forType: .string)
    }

    private func toggleSelection(_ id: String) {
        if selectedEventIDs.contains(id) {
            selectedEventIDs.remove(id)
        } else {
            selectedEventIDs.insert(id)
        }
    }

    private func eventAccessibilityLabel(_ event: CalendarEventMirror) -> String {
        if event.isAllDay {
            return "\(event.summary), all day \(event.startDate.formatted(.dateTime.weekday(.wide).month(.abbreviated).day()))"
        }
        let start = event.startDate.formatted(.dateTime.weekday(.wide).hour().minute())
        let end = event.endDate.formatted(.dateTime.hour().minute())
        return "\(event.summary), \(start) to \(end)"
    }

    private func tasksStrip(_ snapshot: CalendarWeekDisplaySnapshot) -> some View {
        let tasksByDay = snapshot.tasksByDay
        let maxLanes = tasksByDay.values.map(\.count).max() ?? 0
        let visibleRows = min(maxLanes, taskVisibleLimit)
        let overflowRow = maxLanes > taskVisibleLimit ? 1 : 0
        let rowCount = visibleRows + overflowRow
        let stripHeight = CGFloat(rowCount) * taskRowHeight
            + CGFloat(max(rowCount - 1, 0)) * taskRowSpacing
        return Group {
            if maxLanes == 0 {
                EmptyView()
            } else {
                HStack(spacing: 0) {
                    Text("Tasks")
                        .hcbFont(.caption2, weight: .semibold)
                        .foregroundStyle(.secondary)
                        .hcbScaledFrame(width: 54, alignment: .trailing)
                        .hcbScaledPadding(.trailing, 6)
                    GeometryReader { geo in
                        let columnWidth = geo.size.width / CGFloat(max(snapshot.days.count, 1))
                        ZStack(alignment: .topLeading) {
                            ForEach(Array(snapshot.days.enumerated()), id: \.offset) { idx, day in
                                let dayStart = calendar.startOfDay(for: day)
                                let dayStartKey = CalendarDisplaySnapshotBuilder.dayKey(dayStart, calendar: calendar)
                                VStack(alignment: .leading, spacing: taskRowSpacing) {
                                    ForEach((tasksByDay[dayStartKey] ?? []).prefix(taskVisibleLimit)) { task in
                                        HStack(spacing: 4) {
                                            CalendarTaskCheckbox(task: task, size: 10)
                                            CalendarTaskPreviewButton(task: task) {
                                                Text(snapshot.taskMetadataByID[task.id]?.title ?? task.title)
                                                    .hcbFont(.caption)
                                                    .lineLimit(1)
                                                    .frame(maxWidth: .infinity, alignment: .leading)
                                                    .contentShape(Rectangle())
                                            }
                                        }
                                        .padding(.horizontal, 6)
                                        .frame(height: taskRowHeight)
                                        .background(
                                            RoundedRectangle(cornerRadius: 5)
                                                .fill(AppColor.ember.opacity(0.15))
                                        )
                                        .overlay(
                                            RoundedRectangle(cornerRadius: 5)
                                                .strokeBorder(AppColor.ember.opacity(0.35), lineWidth: 0.8)
                                        )
                                        .foregroundStyle(AppColor.ink)
                                        .strikethrough(task.isCompleted, color: .secondary)
                                        .opacity(snapshot.taskMetadataByID[task.id]?.opacity ?? (task.isCompleted ? 0.55 : 1.0))
                                        .accessibilityLabel(snapshot.taskMetadataByID[task.id]?.accessibilityLabel ?? "Task due \(day.formatted(.dateTime.weekday(.wide).month(.abbreviated).day())): \(task.title)")
                                    }
                                    if let tasks = tasksByDay[dayStartKey], tasks.count > taskVisibleLimit {
                                        MonthMoreButton(
                                            count: tasks.count - taskVisibleLimit,
                                            day: dayStart,
                                            events: [],
                                            tasks: tasks,
                                            calendarColor: { calendarColor(for: $0, in: snapshot) }
                                        )
                                        .frame(height: taskRowHeight, alignment: .leading)
                                    }
                                }
                                .hcbScaledPadding(.horizontal, 2)
                                .frame(width: columnWidth, height: stripHeight, alignment: .topLeading)
                                .offset(x: CGFloat(idx) * columnWidth)
                            }
                        }
                    }
                    .frame(height: stripHeight)
                    .clipped()
                }
                .frame(minHeight: stripHeight, idealHeight: stripHeight, maxHeight: stripHeight)
                .clipped()
                .hcbScaledPadding(.vertical, 4)
            }
        }
    }

    private func timeGrid(_ snapshot: CalendarWeekDisplaySnapshot) -> some View {
        // Captured outside GeometryReader so DragGesture closures see a
        // reliable router reference (see allDayStrip for rationale).
        let capturedRouter = router
        return HStack(alignment: .top, spacing: 0) {
            hoursColumn(days: snapshot.days, router: capturedRouter)
            GeometryReader { geo in
                let columnWidth = geo.size.width / CGFloat(max(snapshot.days.count, 1))
                let totalHeight = CGFloat(hourEnd - hourStart) * hourHeight
                ZStack(alignment: .topLeading) {
                    gridLines
                    ForEach(Array(snapshot.days.enumerated()), id: \.offset) { idx, day in
                        let dayStart = calendar.startOfDay(for: day)
                        let dayStartKey = CalendarDisplaySnapshotBuilder.dayKey(dayStart, calendar: calendar)
                        dayColumn(
                            day: day,
                            xOffset: CGFloat(idx) * columnWidth,
                            width: columnWidth,
                            laidOutEvents: snapshot.laidOutTimedEventsByDay[dayStartKey] ?? [],
                            snapshot: snapshot
                        )
                    }
                    if let nowOffset = currentTimeOffset() {
                        nowIndicator(offset: nowOffset)
                    }
                    // Multi-day drag preview. Drawn above the day columns so
                    // it's visible across the whole grid without being eaten
                    // by individual column hit-testing.
                    timedDragPreview(columnWidth: columnWidth, totalHeight: totalHeight)
                }
                .frame(height: totalHeight)
                // Grid-level drag — tracks both x (day column) AND y (time).
                // Taps inside a day column still route to TapToCreateLayer's
                // SpatialTapGesture because DragGesture has minimumDistance.
                .simultaneousGesture(
                    DragGesture(minimumDistance: 6)
                        .onChanged { value in
                            let startCol = columnIndex(for: value.startLocation.x, columnWidth: columnWidth)
                            let endCol = columnIndex(for: value.location.x, columnWidth: columnWidth)
                            timedDrag = TimedWeekDrag(
                                startColumn: startCol,
                                startY: clampedY(value.startLocation.y, totalHeight: totalHeight),
                                endColumn: endCol,
                                endY: clampedY(value.location.y, totalHeight: totalHeight)
                            )
                        }
                        .onEnded { _ in
                            guard let drag = timedDrag else { return }
                            timedDrag = nil
                            if let availabilitySelection {
                                guard drag.startColumn == drag.endColumn else {
                                    availabilitySelection.onReject("Drag within one day to select an availability slot.")
                                    return
                                }
                                guard let (start, end) = resolveTimedDrag(drag) else { return }
                                selectAvailabilitySlot(
                                    AvailabilitySlot(startDate: start, endDate: end),
                                    using: availabilitySelection
                                )
                                return
                            }
                            guard let (start, end) = resolveTimedDrag(drag) else { return }
                            capturedRouter?.present(.quickCreateRange(start, end, allDay: false))
                        }
                )
            }
        }
    }

    private func clampedY(_ y: CGFloat, totalHeight: CGFloat) -> CGFloat {
        max(0, min(totalHeight, y))
    }

    // Converts a TimedWeekDrag into the (start, end) Date pair that
    // gets handed to the QuickCreatePopover. Single-day drags snap the
    // end up to a minimum 30-minute block; cross-day drags honour the
    // full span so the create sheet lands with the right Date values.
    // Flash the tapped timed slot for ~220ms so click-to-create has a
    // visible acknowledgement before the popover paints.
    private func flashTimedStart(_ start: Date) {
        flashTimedSlot = start
        Task {
            try? await Task.sleep(for: .milliseconds(220))
            if flashTimedSlot == start { flashTimedSlot = nil }
        }
    }

    private func resolveTimedDrag(_ drag: TimedWeekDrag) -> (Date, Date)? {
        let startCol = min(drag.startColumn, drag.endColumn)
        let endCol = max(drag.startColumn, drag.endColumn)
        guard weekDays.indices.contains(startCol), weekDays.indices.contains(endCol) else { return nil }

        // Single-column drag: preserve existing behaviour — snap both
        // y values in the same day and ensure a ≥30min block.
        if startCol == endCol {
            let dayStart = calendar.startOfDay(for: weekDays[startCol])
            let lowY = min(drag.startY, drag.endY)
            let highY = max(drag.startY, drag.endY)
            let start = CalendarDropComputer.snappedStart(for: lowY, hourHeight: hourHeight, dayStart: dayStart, calendar: calendar)
            let end = CalendarDropComputer.snappedStart(for: highY, hourHeight: hourHeight, dayStart: dayStart, calendar: calendar)
            let adjustedEnd = end <= start ? start.addingTimeInterval(1800) : end
            return (start, adjustedEnd)
        }

        // Multi-column drag: start anchors to the drag origin's (col, y);
        // end to the drag terminus's (col, y). Normalise into chronological
        // order so a right→left drag still produces start < end.
        let originStart = CalendarDropComputer.snappedStart(
            for: drag.startY, hourHeight: hourHeight,
            dayStart: calendar.startOfDay(for: weekDays[drag.startColumn]),
            calendar: calendar
        )
        let originEnd = CalendarDropComputer.snappedStart(
            for: drag.endY, hourHeight: hourHeight,
            dayStart: calendar.startOfDay(for: weekDays[drag.endColumn]),
            calendar: calendar
        )
        let start = min(originStart, originEnd)
        let end = max(originStart, originEnd)
        let adjustedEnd = end <= start ? start.addingTimeInterval(1800) : end
        return (start, adjustedEnd)
    }

    // Renders the ember-tinted drag preview. Single-column drags show one
    // rectangle; multi-column drags fill the origin column from startY to
    // end-of-day, fill middle columns fully, and fill the terminus column
    // from start-of-day to endY — mirroring how the event will actually
    // render once created.
    @ViewBuilder
    private func timedDragPreview(columnWidth: CGFloat, totalHeight: CGFloat) -> some View {
        if let drag = timedDrag {
            let startCol = min(drag.startColumn, drag.endColumn)
            let endCol = max(drag.startColumn, drag.endColumn)
            // Determine which y belongs to which column after normalisation.
            let (firstY, lastY): (CGFloat, CGFloat) = {
                if drag.startColumn <= drag.endColumn {
                    return (drag.startY, drag.endY)
                } else {
                    return (drag.endY, drag.startY)
                }
            }()
            ForEach(startCol...endCol, id: \.self) { col in
                let x = CGFloat(col) * columnWidth
                let (top, height): (CGFloat, CGFloat) = {
                    if startCol == endCol {
                        let lo = min(firstY, lastY)
                        let hi = max(firstY, lastY)
                        return (lo, max(hi - lo, hourHeight / 4))
                    }
                    if col == startCol { return (firstY, max(totalHeight - firstY, hourHeight / 4)) }
                    if col == endCol { return (0, max(lastY, hourHeight / 4)) }
                    return (0, totalHeight)
                }()
                RoundedRectangle(cornerRadius: 6)
                    .fill(AppColor.ember.opacity(0.22))
                    .overlay(
                        RoundedRectangle(cornerRadius: 6)
                            .strokeBorder(AppColor.ember.opacity(0.6), lineWidth: 1.2)
                    )
                    .frame(width: max(columnWidth - 4, 4), height: height)
                    .offset(x: x + 2, y: top)
                    .allowsHitTesting(false)
            }
        }
    }

    private func hoursColumn(days: [Date], router capturedRouter: RouterPath?) -> some View {
        VStack(spacing: 0) {
            ForEach(hourStart..<hourEnd, id: \.self) { hour in
                HStack(spacing: 4) {
                    Text(labelForHour(hour))
                        .hcbFont(.caption2)
                        .foregroundStyle(.secondary)
                        .offset(y: -6)
                    if showsInlineTimedSlotMenus {
                        weekTimedSlotMenu(hour: hour, days: days, router: capturedRouter)
                    }
                }
                .frame(height: hourHeight, alignment: .top)
            }
        }
        .hcbScaledFrame(width: 54)
    }

    private func weekTimedSlotMenu(hour: Int, days: [Date], router capturedRouter: RouterPath?) -> some View {
        Menu {
            ForEach(Array(days.enumerated()), id: \.offset) { _, day in
                let dayStart = calendar.startOfDay(for: day)
                let start = calendar.date(byAdding: .hour, value: hour, to: dayStart) ?? dayStart
                Button(start.formatted(.dateTime.weekday(.wide).month(.abbreviated).day())) {
                    createTimedSlot(start: start, router: capturedRouter)
                }
            }
        } label: {
            Image(systemName: "plus.circle")
                .imageScale(.small)
        }
        .menuStyle(.borderlessButton)
        .controlSize(.mini)
        .fixedSize()
        .help(weekTimedSlotActionTitle(hour: hour))
        .accessibilityLabel(weekTimedSlotAccessibilityLabel(hour: hour))
    }

    private func weekTimedSlotActionTitle(hour: Int) -> String {
        let dayStart = calendar.startOfDay(for: anchorDate)
        let start = calendar.date(byAdding: .hour, value: hour, to: dayStart) ?? dayStart
        let time = start.formatted(.dateTime.hour().minute())
        return availabilitySelection == nil ? "New event at \(time)" : "Select availability at \(time)"
    }

    private func weekTimedSlotAccessibilityLabel(hour: Int) -> String {
        let dayStart = calendar.startOfDay(for: anchorDate)
        let start = calendar.date(byAdding: .hour, value: hour, to: dayStart) ?? dayStart
        let time = start.formatted(.dateTime.hour().minute())
        return availabilitySelection == nil ? "New event at \(time)" : "Select availability at \(time)"
    }

    private var gridLines: some View {
        VStack(spacing: 0) {
            ForEach(hourStart..<hourEnd, id: \.self) { _ in
                Divider()
                    .frame(height: hourHeight, alignment: .top)
            }
        }
    }

    private func dayColumn(
        day: Date,
        xOffset: CGFloat,
        width: CGFloat,
        laidOutEvents: [CalendarGridLayout.LaidOutEvent],
        snapshot: CalendarWeekDisplaySnapshot
    ) -> some View {
        let startOfDay = calendar.startOfDay(for: day)
        let endOfDay = calendar.date(byAdding: .day, value: 1, to: startOfDay) ?? startOfDay
        let dayStartKey = CalendarDisplaySnapshotBuilder.dayKey(startOfDay, calendar: calendar)
        let density = snapshot.densityByDay[dayStartKey]
        let capturedRouter = router

        return ZStack(alignment: .topLeading) {
            TapToCreateLayer(
                hourHeight: hourHeight,
                dayStart: startOfDay,
                calendar: calendar,
                onTap: { start in
                    createTimedSlot(start: start, router: capturedRouter)
                }
            )
            .dropDestination(for: DraggedTask.self) { items, location in
                guard let dropped = items.first else { return false }
                Task {
                    await scheduleTaskAsEvent(dropped, dropY: location.y, dayStart: startOfDay)
                }
                return true
            }
            .dropDestination(for: DraggedEvent.self) { items, location in
                guard let dropped = items.first else { return false }
                Task {
                    await rescheduleEvent(dropped, dropY: location.y, dayStart: startOfDay)
                }
                return true
            }
            availabilitySlotOverlays(
                dayStart: startOfDay,
                dayEnd: endOfDay,
                columnWidth: width
            )
            ForEach(Array(laidOutEvents.enumerated()), id: \.offset) { _, placed in
                eventTile(
                    placed: placed,
                    dayStart: startOfDay,
                    dayEnd: endOfDay,
                    columnWidth: width,
                    isDense: density?.usesDenseTimedRendering == true,
                    snapshot: snapshot
                )
            }
            if let density, density.hiddenTimedEventCount > 0 {
                MonthMoreButton(
                    count: density.hiddenTimedEventCount,
                    day: startOfDay,
                    events: snapshot.timedEventsByDay[dayStartKey] ?? [],
                    tasks: [],
                    calendarColor: { calendarColor(for: $0, in: snapshot) }
                )
                .frame(width: max(width - 8, 80), height: 24, alignment: .leading)
                .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 6, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: 6, style: .continuous)
                        .strokeBorder(AppColor.cardStroke.opacity(0.45), lineWidth: 0.6)
                )
                .offset(x: 4, y: CGFloat(hourEnd - hourStart) * hourHeight - 32)
            }
            // Momentary tint at the tapped hour slot so users see their
            // click register before the popover paints. Drawn above event
            // tiles so it's visible even over a crowded column.
            if let flash = flashTimedSlot, flash >= startOfDay && flash < endOfDay {
                let minutesIntoDay = flash.timeIntervalSince(startOfDay) / 60.0
                let startHourOffset = Double(hourStart) * 60.0
                let y = CGFloat(max(0, minutesIntoDay - startHourOffset)) / 60.0 * hourHeight
                RoundedRectangle(cornerRadius: 4)
                    .fill(AppColor.ember.opacity(0.22))
                    .frame(height: hourHeight * 0.5)
                    .offset(x: 4, y: y)
                    .padding(.trailing, 8)
                    .allowsHitTesting(false)
                    .transition(.opacity)
            }
        }
        .frame(width: width)
        .offset(x: xOffset)
        .animation(HCBMotion.animation(.easeOut(duration: 0.18), reduceMotion: calendarGridReduceMotion), value: flashTimedSlot)
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

    // Tap-only hit-test layer for each day column. Multi-day drag is now
    // handled at the grid level (timeGrid → simultaneousGesture) so a drag
    // can span columns without the column's hit-testing eating the gesture.
    // Single taps still land here and open the quick-create popover at the
    // snapped hour slot.
    private struct TapToCreateLayer: View {
        let hourHeight: CGFloat
        let dayStart: Date
        let calendar: Calendar
        let onTap: (Date) -> Void

        var body: some View {
            Rectangle()
                .fill(Color.clear)
                .contentShape(Rectangle())
                .simultaneousGesture(
                    SpatialTapGesture(count: 1)
                        .onEnded { value in
                            let start = CalendarDropComputer.snappedStart(
                                for: value.location.y,
                                hourHeight: hourHeight,
                                dayStart: dayStart,
                                calendar: calendar
                            )
                            onTap(start)
                        }
                )
        }
    }

    @ViewBuilder
    private func availabilitySlotOverlays(
        dayStart: Date,
        dayEnd: Date,
        columnWidth: CGFloat
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
                    .frame(width: max(columnWidth - 8, 20), height: max(height - 2, 12))
                    .offset(x: 4, y: yOffset)
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

    private func scheduleTaskAsEvent(_ dropped: DraggedTask, dropY: CGFloat, dayStart: Date) async {
        let start = CalendarDropComputer.snappedStart(for: dropY, hourHeight: hourHeight, dayStart: dayStart, calendar: calendar)
        let end = CalendarDropComputer.defaultEndDate(from: start, calendar: calendar)
        let destinationCalendar = primaryEditableCalendarID() ?? calendarStore.calendarSnapshot.selectedCalendars.first?.id
        guard let calendarID = destinationCalendar else { return }
        // Backlink travels in Google's native extendedProperties.private bag
        // (HCB-only; invisible in google.com web UI) — not in the event
        // description, which was the legacy schema-polluting behaviour.
        _ = await model.createEvent(
            summary: dropped.title,
            details: "",
            startDate: start,
            endDate: end,
            isAllDay: false,
            reminderMinutes: nil,
            calendarID: calendarID,
            hcbTaskID: dropped.taskID
        )
    }

    private func rescheduleEvent(_ dropped: DraggedEvent, dropY: CGFloat, dayStart: Date) async {
        guard dropped.isAllDay == false else { return }
        guard let event = calendarStore.event(id: dropped.eventID) else { return }
        let snappedStart = CalendarDropComputer.snappedStart(for: dropY, hourHeight: hourHeight, dayStart: dayStart, calendar: calendar)
        guard let newEnd = calendar.date(byAdding: .minute, value: dropped.durationMinutes, to: snappedStart) else { return }
        if event.startDate == snappedStart && event.endDate == newEnd { return }
        _ = await model.updateEvent(
            event,
            summary: event.summary,
            details: event.details,
            startDate: snappedStart,
            endDate: newEnd,
            isAllDay: false,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: event.calendarID,
            location: event.location,
            attendeeEmails: event.attendeeEmails,
            notifyGuests: false
        )
    }

    private func primaryEditableCalendarID() -> CalendarListMirror.ID? {
        calendarStore.calendarSnapshot.selectedCalendars.first(where: { $0.accessRole == "owner" || $0.accessRole == "writer" })?.id
    }

    @ViewBuilder
    private func eventTile(
        placed: CalendarGridLayout.LaidOutEvent,
        dayStart: Date,
        dayEnd: Date,
        columnWidth: CGFloat,
        isDense: Bool,
        snapshot: CalendarWeekDisplaySnapshot
    ) -> some View {
        let clampedStart = max(placed.event.startDate, dayStart)
        let clampedEnd = min(placed.event.endDate, dayEnd)
        let startMinutes = clampedStart.timeIntervalSince(dayStart) / 60
        let durationMinutes = max(clampedEnd.timeIntervalSince(clampedStart) / 60, 20)
        let yOffset = CGFloat(startMinutes) * (hourHeight / 60)
        let height = CGFloat(durationMinutes) * (hourHeight / 60)
        let slotWidth = columnWidth / CGFloat(placed.columnCount)
        let xOffsetWithinDay = CGFloat(placed.columnIndex) * slotWidth
        let tileWidth = max(slotWidth - 2, 1)
        let tileHeight = max(height - 2, 1)
        let fill = calendarColor(for: placed.event, in: snapshot)
        let fullDurationMinutes = Int(max(placed.event.endDate.timeIntervalSince(placed.event.startDate) / 60, 15))

        if isDense {
            Button {
                router?.present(.editEvent(placed.event.id))
            } label: {
                Text(placed.event.summary)
                    .hcbFont(.caption2, weight: .semibold)
                    .lineLimit(1)
                    .hcbScaledPadding(.horizontal, 5)
                    .hcbScaledPadding(.vertical, 3)
                    .frame(width: tileWidth, height: tileHeight, alignment: .topLeading)
                    .clipped()
                    .background(
                        RoundedRectangle(cornerRadius: 5)
                            .fill(fill.opacity(0.18))
                    )
                    .overlay(
                        RoundedRectangle(cornerRadius: 5)
                            .strokeBorder(fill.opacity(0.42), lineWidth: 0.6)
                    )
            }
            .buttonStyle(.plain)
            .offset(x: xOffsetWithinDay + 1, y: yOffset)
            .overlay(
                RoundedRectangle(cornerRadius: 5)
                    .strokeBorder(selectedEventIDs.contains(placed.event.id) ? AppColor.blue : Color.clear, lineWidth: 2)
                    .frame(width: tileWidth, height: tileHeight)
                    .offset(x: xOffsetWithinDay + 1, y: yOffset)
                    .allowsHitTesting(false)
            )
            .simultaneousGesture(
                TapGesture().modifiers(.command).onEnded {
                    toggleSelection(placed.event.id)
                }
            )
            .accessibilityLabel(snapshot.eventMetadataByID[placed.event.id]?.accessibilityLabel ?? placed.event.summary)
        } else {
            CalendarEventPreviewButton(event: placed.event) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(placed.event.summary)
                        .hcbFont(.caption, weight: .semibold)
                        .lineLimit(height > 38 ? 2 : 1)
                    if height > 34 {
                        Text(snapshot.eventMetadataByID[placed.event.id]?.timeRangeLabel ?? "")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                            .lineLimit(1)
                    }
                }
                .hcbScaledPadding(.horizontal, 6)
                .hcbScaledPadding(.vertical, 4)
                .frame(width: tileWidth, height: tileHeight, alignment: .topLeading)
                .clipped()
                .background(
                    RoundedRectangle(cornerRadius: 6)
                        .fill(fill.opacity(0.2))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 6)
                        .strokeBorder(fill.opacity(0.55), lineWidth: 0.8)
                )
            }
            .offset(x: xOffsetWithinDay + 1, y: yOffset)
            .overlay(
                RoundedRectangle(cornerRadius: 6)
                    .strokeBorder(selectedEventIDs.contains(placed.event.id) ? AppColor.blue : Color.clear, lineWidth: 2)
                    .frame(width: tileWidth, height: tileHeight)
                    .offset(x: xOffsetWithinDay + 1, y: yOffset)
                    .allowsHitTesting(false)
            )
            .simultaneousGesture(
                TapGesture().modifiers(.command).onEnded {
                    toggleSelection(placed.event.id)
                }
            )
            .accessibilityLabel(snapshot.eventMetadataByID[placed.event.id]?.accessibilityLabel ?? placed.event.summary)
            .accessibilityHint("Opens event details")
            .modifier(EventHoverPreviewModifier(event: placed.event, isEnabled: isWeekHoverPreviewEnabled))
            .draggable(DraggedEvent(
                eventID: placed.event.id,
                calendarID: placed.event.calendarID,
                durationMinutes: fullDurationMinutes,
                isAllDay: placed.event.isAllDay
            )) {
                Text(placed.event.summary)
                    .hcbFont(.caption, weight: .semibold)
                    .hcbScaledPadding(.horizontal, 10)
                    .hcbScaledPadding(.vertical, 6)
                    .background(Capsule().fill(fill.opacity(0.35)))
            }
        }
    }

    private func currentTimeOffset() -> CGFloat? {
        let now = Date()
        guard let todayInWeek = weekDays.first(where: { calendar.isDate($0, inSameDayAs: now) }) else { return nil }
        _ = todayInWeek
        let minutes = calendar.component(.hour, from: now) * 60 + calendar.component(.minute, from: now)
        return CGFloat(minutes) * (hourHeight / 60)
    }

    private func nowIndicator(offset: CGFloat) -> some View {
        Rectangle()
            .fill(AppColor.ember)
            .hcbScaledFrame(height: 1)
            .offset(y: offset)
    }

    private func isToday(_ day: Date) -> Bool {
        calendar.isDateInToday(day)
    }

    private func labelForHour(_ hour: Int) -> String {
        CalendarHourLabelCache.label(for: hour)
    }

    private func rebuildWeekSnapshotIfNeeded() {
        let key = weekSnapshotKey
        guard preparedWeekSnapshot?.key != key else { return }
        if let snapshot = calendarStore.cachedCalendarWeekSnapshot(for: key) {
            preparedWeekSnapshot = snapshot
            prewarmAdjacentWeekSnapshots(around: anchorDate)
            return
        }
        let count = multiDayCount
        weekSnapshotBuildTask?.cancel()
        weekSnapshotBuildTask = Task { @MainActor in
            let started = HCBPerformanceTelemetry.timestamp()
            let input = await model.calendarDisplayInputFromQuery(
                key: key,
                kind: .week,
                anchorDate: anchorDate,
                dayCount: count,
                eventViewFilter: calendarEventViewFilter,
                searchQuery: searchQuery,
                referenceDate: Date(),
                calendar: calendar
            )
            let snapshot = await Task.detached(priority: .utility) {
                CalendarDisplaySnapshotBuilder.weekSnapshot(input, multiDayCount: count)
            }.value
            guard Task.isCancelled == false, snapshot.key == weekSnapshotKey else { return }
            calendarStore.storeCalendarWeekSnapshot(snapshot)
            preparedWeekSnapshot = snapshot
            prewarmAdjacentWeekSnapshots(around: anchorDate)
            HCBPerformanceTelemetry.debug(
                "calendar week snapshot built",
                metadata: [
                    "buildMs": HCBPerformanceTelemetry.elapsedMilliseconds(since: started),
                    "days": "\(snapshot.days.count)",
                    "events": "\(snapshot.eventMetadataByID.count)",
                    "tasks": "\(snapshot.taskMetadataByID.count)"
                ]
            )
        }
    }

    private func prewarmAdjacentWeekSnapshots(around date: Date) {
        guard calendarStore.settings.performanceMode == .snappy else { return }
        let step = max(1, multiDayCount ?? 7)
        let anchors = [-step, step].compactMap {
            calendar.date(byAdding: .day, value: $0, to: calendar.startOfDay(for: date))
        }
        guard anchors.isEmpty == false else { return }
        let count = multiDayCount
        weekSnapshotPrewarmTask?.cancel()
        weekSnapshotPrewarmTask = Task { @MainActor in
            for prewarmAnchor in anchors {
                let days = weekDays(for: prewarmAnchor)
                let key = weekSnapshotKey(for: days)
                if calendarStore.cachedCalendarWeekSnapshot(for: key) != nil { continue }
                let input = await model.calendarDisplayInputFromQuery(
                    key: key,
                    kind: .week,
                    anchorDate: prewarmAnchor,
                    dayCount: count,
                    eventViewFilter: calendarEventViewFilter,
                    searchQuery: searchQuery,
                    referenceDate: Date(),
                    calendar: calendar
                )
                let snapshot = await Task.detached(priority: .utility) {
                    CalendarDisplaySnapshotBuilder.weekSnapshot(input, multiDayCount: count)
                }.value
                guard Task.isCancelled == false, snapshot.key == key else { return }
                calendarStore.storeCalendarWeekSnapshot(snapshot)
            }
        }
    }

    private func calendarColor(for event: CalendarEventMirror, in snapshot: CalendarWeekDisplaySnapshot) -> Color {
        guard let hex = snapshot.eventMetadataByID[event.id]?.colorHex else { return AppColor.blue }
        return Color(hex: hex)
    }
}

enum CalendarHourLabelCache {
    private static let labels: [Int: String] = {
        var calendar = Calendar.current
        calendar.locale = Locale.current
        return Dictionary(uniqueKeysWithValues: (0..<24).map { hour in
            let date = calendar.date(from: DateComponents(hour: hour)) ?? Date()
            return (hour, date.formatted(.dateTime.hour()))
        })
    }()

    static func label(for hour: Int) -> String {
        labels[hour] ?? ""
    }
}
