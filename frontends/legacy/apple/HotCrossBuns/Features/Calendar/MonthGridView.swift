import SwiftUI
import AppKit

enum CalendarMonthScrollWindow {
    static let pastMonthsKey = "calendar.monthScroll.pastMonths"
    static let futureMonthsKey = "calendar.monthScroll.futureMonths"
    static let defaultPastMonths = 0
    static let defaultFutureMonths = 1
    static let pastRange = 0...24
    static let futureRange = 1...36

    static func clampedPast(_ value: Int) -> Int {
        min(max(value, pastRange.lowerBound), pastRange.upperBound)
    }

    static func clampedFuture(_ value: Int) -> Int {
        min(max(value, futureRange.lowerBound), futureRange.upperBound)
    }
}

struct MonthGridView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.routerPath) private var router
    @Environment(\.hcbReduceMotion) private var reduceMotion
    @Environment(\.hcbAppBackgroundConfiguration) private var backgroundConfiguration
    @Environment(\.scenePhase) private var scenePhase
    @Environment(\.calendarEventViewFilter) private var calendarEventViewFilter
    @AppStorage(CalendarMonthScrollWindow.pastMonthsKey) private var configuredPastMonths = CalendarMonthScrollWindow.defaultPastMonths
    @AppStorage(CalendarMonthScrollWindow.futureMonthsKey) private var configuredFutureMonths = CalendarMonthScrollWindow.defaultFutureMonths
    @Binding var anchorDate: Date
    var searchQuery: String = ""

    private let calendar = Calendar.current
    private var calendarGridReduceMotion: Bool {
        reduceMotion || scenePhase != .active || ProcessInfo.processInfo.isLowPowerModeEnabled
    }
    private let weekdaySymbols: [String] = {
        var cal = Calendar.current
        cal.locale = Locale.current
        let formatter = DateFormatter()
        formatter.calendar = cal
        let symbols = formatter.shortWeekdaySymbols ?? []
        let firstWeekday = cal.firstWeekday - 1
        guard firstWeekday < symbols.count else { return symbols }
        return Array(symbols[firstWeekday...]) + Array(symbols[..<firstWeekday])
    }()

    private let laneHeight: CGFloat = 18
    private let laneSpacing: CGFloat = 2
    private let maxVisibleLanes: Int = 3
    // Continuous-scroll rewrite: fixed row height lets us resolve drag points
    // to (weekIndex, dayColumn) via simple division, and guarantees uniform
    // layout across weeks regardless of band-event count. Matches the legacy
    // paged-month cell height (geo.height ÷ 6 for a 660pt region ≈ 110).
    private let fixedRowHeight: CGFloat = 110
    private let previousMonthLoaderHeight: CGFloat = 34
    private let nextMonthLoaderHeight: CGFloat = 34
    private var usesReadableMonthBackings: Bool {
        backgroundConfiguration.customImagePath != nil || backgroundConfiguration.isTranslucent
    }
    private var monthCellBackingOpacity: Double {
        usesReadableMonthBackings ? 0.68 : 0.04
    }
    private var monthCellStrokeOpacity: Double {
        usesReadableMonthBackings ? 0.50 : 1.0
    }
    private var monthEventFillOpacity: Double {
        usesReadableMonthBackings ? 0.36 : 0.25
    }
    private var monthTaskFillOpacity: Double {
        usesReadableMonthBackings ? 0.26 : 0.15
    }
    private var monthTaskStrokeOpacity: Double {
        usesReadableMonthBackings ? 0.50 : 0.35
    }
    // Named coordinate space the drag gesture and the highlight overlay both
    // resolve into. Needs to wrap the LazyVStack so both gesture points and
    // overlay positioning share the same frame.
    private let gridCoordinateSpace = "monthGridContent"
    // Separate named space for the outer ScrollView — the inner
    // GeometryReader reports its frame within this space to derive scroll
    // offset. Kept distinct from gridCoordinateSpace because the gesture
    // resolution wants content-local coords, not scroll-view-local.
    private let scrollCoordinateSpace = "monthGridScroll"

    @State private var dragSelection: DragSelection?
    // Grid-content cache. Rebuilt only when the underlying inputs (calendar
    // selection, search query, window anchor, event corpus count) change —
    // NOT on every body eval. Critical for drag-create responsiveness: the
    // DragGesture .onChanged fires at ~60Hz, and without this cache every
    // drag tick ran the filteredEvents + eventsByDay pipelines over 17k+
    // events, producing ~50ms stalls per pixel moved.
    @State private var cachedWeekSnapshots: [MonthWeekSnapshot] = []
    @State private var cachedGridKey: String = ""
    @State private var renderedGridKey: String = ""
    @State private var isGridCachePreparing = false
    @State private var pendingAnchorScroll: PendingAnchorScroll?
    @State private var cachedEventSearchTextByID: [CalendarEventMirror.ID: String] = [:]
    @State private var cachedEventSearchRevision: String = ""
    @State private var gridCacheBuildTask: Task<Void, Never>?
    // Rolling window of week-start dates rendered by the ScrollView. Rebuilt
    // by buildWindow() when the anchor lands outside the current window.
    @State private var weekStarts: [Date] = []
    @State private var windowStart: Date = Date()
    // Guard against a feedback loop: when we update anchorDate from a scroll
    // event, onChange(of: anchorDate) would otherwise programmatically scroll
    // (and potentially fight with the user's momentum). This flag short-
    // circuits one onChange pass.
    @State private var skipNextScrollSync: Bool = false
    // First-render guard. Before the onAppear scrollToAnchor has positioned
    // the viewport on today's week, the initial preference-change callback
    // fires with scrollOffset = 0 → center week = start of the 104-week
    // window (~1 year ago) → would stomp anchorDate back to last year before
    // we ever got a chance to scroll. Flipped true once the initial scroll
    // runs, so subsequent user-driven preference changes work normally.
    @State private var didPerformInitialScroll: Bool = false
    @State private var isLoadingPreviousMonth: Bool = false
    @State private var isLoadingNextMonth: Bool = false
    @State private var windowFirstMonthStart: Date = Date()
    @State private var windowLastMonthStart: Date = Date()
    @State private var lastObservedScrollOffset: CGFloat = .nan

    private var calendarStore: CalendarStore { model.calendarStore }

    private struct DragSelection: Equatable {
        var start: Date
        var end: Date
        var normalized: (Date, Date) {
            start <= end ? (start, end) : (end, start)
        }
    }

    private struct PendingAnchorScroll {
        var animated: Bool
        var completesInitialScroll: Bool
    }

    private struct MonthDaySnapshot: Identifiable, Equatable, Sendable {
        var day: Date
        var dayStart: Date
        var dayStartKey: TimeInterval
        var dayLabel: String
        var accessibilityDateLabel: String
        var isToday: Bool
        var allEvents: [CalendarEventMirror]
        var timedEvents: [CalendarEventMirror]
        var tasks: [TaskMirror]
        var eventLabels: [CalendarEventMirror.ID: String]
        var eventAccessibilityLabels: [CalendarEventMirror.ID: String]
        var eventColorHexes: [CalendarEventMirror.ID: String]
        var taskAccessibilityLabels: [TaskMirror.ID: String]

        var id: Date { dayStart }
    }

    private struct MonthWeekSnapshot: Identifiable, Equatable, Sendable {
        var weekStart: Date
        var days: [MonthDaySnapshot]
        var bands: [CalendarGridLayout.MonthBand]
        var bandReserves: [CGFloat]
        var bandAccessibilityLabels: [String: String]
        var bandColorHexes: [String: String]

        var id: Date { weekStart }
    }

    private struct MonthBandRenderItem: Identifiable {
        var id: String
        var band: CalendarGridLayout.MonthBand
        var accessibilityLabel: String
        var colorHex: String?
        var frameWidth: CGFloat
        var xOffset: CGFloat
        var yOffset: CGFloat
        var dragPayload: DraggedEvent
    }

    private struct MonthGridCachePayload: Sendable {
        var key: String
        var anchorDate: Date
        var weekStarts: [Date]
        var selectedCalendarIDs: Set<CalendarListMirror.ID>
        var eventViewFilter: CalendarEventViewFilter
        var visibleTaskListIDs: Set<TaskListMirror.ID>
        var searchQuery: String
        var visibleRange: CalendarVisibleRangeProjection
        var dataRevision: String
        var eventSearchTextByID: [CalendarEventMirror.ID: String]
        var calendarColorHexByID: [CalendarListMirror.ID: String]
        var calendar: Calendar
        var laneHeight: CGFloat
        var laneSpacing: CGFloat
        var maxVisibleLanes: Int
    }

    private struct MonthGridCacheResult: Sendable {
        var key: String
        var weekSnapshots: [MonthWeekSnapshot]
        var eventSearchTextByID: [CalendarEventMirror.ID: String]
        var eventSearchRevision: String
        var visibleEventCount: Int
        var visibleTaskCount: Int
        var buildDurationMilliseconds: String
    }

    var body: some View {
        VStack(spacing: 0) {
            weekdayHeader
            Divider()
            continuousGrid
        }
        .hcbDebugBodyProbe("MonthGridView")
        .onDisappear {
            gridCacheBuildTask?.cancel()
            gridCacheBuildTask = nil
            if isGridCachePreparing {
                cachedGridKey = renderedGridKey
                isGridCachePreparing = false
            }
        }
    }

    private var isGridReadyForInteraction: Bool {
        cachedWeekSnapshots.isEmpty == false
            && isGridCachePreparing == false
            && renderedGridKey == currentGridCacheKey
            && calendarStore.isRebuildingDerivedSnapshots == false
    }

    private var shouldShowGridPreparationOverlay: Bool {
            cachedWeekSnapshots.isEmpty
            || isGridCachePreparing
            || renderedGridKey != currentGridCacheKey
            || calendarStore.isRebuildingDerivedSnapshots
    }

    // Replaces the paged month grid. A LazyVStack of week rows inside a
    // ScrollView — natural continuous vertical scrolling, partial months
    // visible at any scroll position. ScrollViewReader lets external anchor
    // updates (chevron / today / mini-calendar) programmatically scroll; the
    // onScrollGeometryChange reverse-binds the visible-center week back into
    // anchorDate so the nav bar's month title follows live scrolling.
    //
    // §7.02 cache: `filteredEvents` + windowed `eventsByDay` are computed
    // ONCE per window/search/events change and threaded into every weekRow
    // so `monthBands` doesn't re-iterate the full events list per week on
    // each scroll tick.
    private var continuousGrid: some View {
        GeometryReader { outerGeo in
            ScrollViewReader { proxy in
                ScrollView(.vertical) {
                    LazyVStack(spacing: 0) {
                        previousMonthLoader(proxy: proxy)
                            .frame(height: previousMonthLoaderHeight)

                        ForEach(cachedWeekSnapshots) { weekSnapshot in
                            weekRow(snapshot: weekSnapshot)
                            .frame(height: fixedRowHeight)
                            .id(weekSnapshot.weekStart)
                        }

                        nextMonthLoader(proxy: proxy)
                            .frame(height: nextMonthLoaderHeight)
                    }
                    // Named coordinate space wraps the content so (a) the
                    // drag gesture can resolve points in absolute content
                    // coords and (b) the drag highlight overlay positions
                    // rectangles against the same frame — gesture and paint
                    // agree on pixel origin.
                    .coordinateSpace(name: gridCoordinateSpace)
                    // Background GeometryReader captures the content width
                    // and reports the current scroll offset via a
                    // PreferenceKey. Both dateAtContentPoint and the
                    // highlight overlay read visibleGridWidth; the scroll
                    // offset drives the anchor-from-scroll binding.
                    .background(
                        GeometryReader { innerGeo in
                            Color.clear
                                .onAppear { visibleGridWidth = innerGeo.size.width }
                                .onChange(of: innerGeo.size.width) { _, new in visibleGridWidth = new }
                                .preference(
                                    key: ScrollOffsetKey.self,
                                    value: -innerGeo.frame(in: .named(scrollCoordinateSpace)).minY
                                )
                        }
                    )
                    .overlay(alignment: .topLeading) { dragHighlightOverlay }
                    .simultaneousGesture(
                        DragGesture(minimumDistance: 6, coordinateSpace: .named(gridCoordinateSpace))
                            .onChanged { value in
                                guard let start = dateAtContentPoint(value.startLocation),
                                      let curr = dateAtContentPoint(value.location) else { return }
                                dragSelection = DragSelection(start: start, end: curr)
                            }
                            .onEnded { _ in
                                guard let sel = dragSelection else { return }
                                dragSelection = nil
                                let (from, to) = sel.normalized
                                let endExclusive = calendar.date(byAdding: .day, value: 1, to: calendar.startOfDay(for: to)) ?? to
                                router?.present(.quickCreateRange(calendar.startOfDay(for: from), endExclusive, allDay: true))
                            }
                    )
                }
                .allowsHitTesting(isGridReadyForInteraction)
                .overlay {
                    if shouldShowGridPreparationOverlay {
                        gridPreparationOverlay
                    }
                }
                .scrollContentBackground(.hidden)
                .coordinateSpace(name: scrollCoordinateSpace)
                .onAppear {
                    // Sequence matters: weekStarts must be populated before
                    // scrollTo, and SwiftUI must get one pass to render those
                    // IDs in the ForEach before the proxy can resolve them.
                    // Otherwise the first scroll is unreliable and the grid
                    // can open at the wrong week until Today is clicked.
                    if weekStarts.isEmpty {
                        buildWindow(centeredOn: anchorDate)
                    }
                    requestAnchorScroll(animated: false, completesInitialScroll: true)
                    rebuildGridCacheIfNeeded()
                    flushPendingAnchorScroll(proxy: proxy)
                }
                .onChange(of: anchorDate) { _, newValue in
                    if skipNextScrollSync {
                        skipNextScrollSync = false
                        return
                    }
                    // If the new anchor left the window, rebuild starting at
                    // its month. Month mode is a bounded continuous surface:
                    // programmatic jumps open at the month start, while
                    // history loads on demand from the top boundary.
                    if isDateInWindow(newValue) == false {
                        buildWindow(centeredOn: newValue)
                        rebuildGridCacheIfNeeded()
                        requestAnchorScroll(animated: true)
                        flushPendingAnchorScroll(proxy: proxy)
                        return
                    }
                    requestAnchorScroll(animated: true)
                    flushPendingAnchorScroll(proxy: proxy)
                }
                .onChange(of: currentGridCacheKey) { _, _ in rebuildGridCacheIfNeeded() }
                .onChange(of: renderedGridKey) { _, _ in flushPendingAnchorScroll(proxy: proxy) }
                .onChange(of: monthWindowPreferenceKey) { _, _ in
                    buildWindow(centeredOn: anchorDate)
                    rebuildGridCacheIfNeeded()
                    requestAnchorScroll(animated: false)
                    flushPendingAnchorScroll(proxy: proxy)
                }
                .onPreferenceChange(ScrollOffsetKey.self) { offset in
                    guard lastObservedScrollOffset.isNaN || abs(offset - lastObservedScrollOffset) >= 8 else { return }
                    lastObservedScrollOffset = offset
                    let centerY = offset + outerGeo.size.height / 2
                    updateAnchorFromScroll(centerY: centerY)
                }
            }
        }
    }

    // Fingerprints every input the grid cache depends on. Cheap to compute —
    // O(selectedCalendars) — and triggers a rebuild only on real changes,
    // skipping every drag-induced body re-eval.
    private var currentGridCacheKey: String {
        let selectedIds = calendarStore.calendarSnapshot.selectedCalendarIDs.sorted().joined(separator: ",")
        // calendarDisplayRevision replaces the prior all-events count fingerprint.
        // renames / reschedules / recolors with unchanged total count now
        // bust the cache correctly while unrelated model changes keep the
        // grid cache warm.
        return "\(selectedIds)|\(calendarEventViewFilter.cacheKey)|\(visibleTaskListKey)|\(searchQuery)|\(windowKey)|\(currentCalendarRevisionKey)"
    }

    private var windowKey: String {
        guard let first = weekStarts.first else { return "empty" }
        return "\(Int(first.timeIntervalSince1970))-\(weekStarts.count)"
    }

    private var currentCalendarRevisionKey: String {
        guard let first = weekStarts.first else {
            return calendarStore.calendarDisplayRevisionKey(for: [], calendar: calendar)
        }
        let lastWeek = weekStarts.last ?? first
        let last = calendar.date(byAdding: .day, value: 6, to: lastWeek) ?? lastWeek
        return calendarStore.calendarDisplayRevisionKey(from: first, to: last, calendar: calendar)
    }

    private var visibleTaskListKey: String {
        calendarStore.visibleTaskListIDs.sorted().joined(separator: ",")
    }

    private var monthWindowPreferenceKey: String {
        "\(CalendarMonthScrollWindow.clampedPast(configuredPastMonths))|\(CalendarMonthScrollWindow.clampedFuture(configuredFutureMonths))"
    }

    private var gridPreparationOverlay: some View {
        VStack(spacing: 12) {
            ProgressView()
                .controlSize(.large)
            Text("Preparing calendar…")
                .hcbFont(.subheadline, weight: .semibold)
                .foregroundStyle(AppColor.ink)
            Text("Optimizing \(calendarStore.events.count.formatted()) events for smooth scrolling.")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
        }
        .hcbScaledPadding(20)
        .background {
            if calendarStore.settings.performanceMode == .rich {
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(.regularMaterial)
            } else {
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(AppColor.cardSurface)
            }
        }
        .overlay(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .strokeBorder(AppColor.cardStroke.opacity(0.45), lineWidth: 0.5)
        )
        .shadow(
            color: .black.opacity(calendarStore.settings.performanceMode == .rich ? 0.08 : 0.02),
            radius: calendarStore.settings.performanceMode == .rich ? 18 : 4,
            x: 0,
            y: calendarStore.settings.performanceMode == .rich ? 10 : 2
        )
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background {
            if calendarStore.settings.performanceMode == .rich {
                Rectangle().fill(.regularMaterial.opacity(0.18))
            }
        }
    }

    private func rebuildGridCacheIfNeeded() {
        let key = currentGridCacheKey
        guard key != cachedGridKey else { return }
        cachedGridKey = key
        let requestedAt = HCBPerformanceTelemetry.timestamp()
        let rangeStart = weekStarts.first ?? anchorDate
        let rangeEnd = weekStarts.last.flatMap {
            calendar.date(byAdding: .day, value: 6, to: $0)
        } ?? rangeStart

        gridCacheBuildTask?.cancel()
        isGridCachePreparing = true
        gridCacheBuildTask = Task { @MainActor in
            let revisionKey = currentCalendarRevisionKey
            let visibleRange = await model.calendarVisibleRangeProjectionFromQuery(
                kind: .month,
                start: rangeStart,
                end: rangeEnd,
                eventViewFilter: calendarEventViewFilter,
                searchQuery: searchQuery,
                calendar: calendar
            )
            let payload = MonthGridCachePayload(
                key: key,
                anchorDate: anchorDate,
                weekStarts: weekStarts,
                selectedCalendarIDs: calendarStore.calendarSnapshot.selectedCalendarIDs,
                eventViewFilter: calendarEventViewFilter,
                visibleTaskListIDs: calendarStore.visibleTaskListIDs,
                searchQuery: searchQuery,
                visibleRange: visibleRange,
                dataRevision: revisionKey,
                eventSearchTextByID: cachedEventSearchRevision == revisionKey ? cachedEventSearchTextByID : [:],
                calendarColorHexByID: calendarStore.calendarSnapshot.calendarColorHexByID,
                calendar: calendar,
                laneHeight: laneHeight,
                laneSpacing: laneSpacing,
                maxVisibleLanes: maxVisibleLanes
            )
            let result = await Task.detached(priority: .utility) {
                Self.buildMonthGridCache(payload)
            }.value

            guard Task.isCancelled == false, result.key == cachedGridKey else { return }
            cachedWeekSnapshots = result.weekSnapshots
            cachedEventSearchTextByID = result.eventSearchTextByID
            cachedEventSearchRevision = result.eventSearchRevision
            renderedGridKey = result.key
            isGridCachePreparing = false
            HCBPerformanceTelemetry.debug(
                "month grid cache rebuilt",
                metadata: [
                    "buildMs": result.buildDurationMilliseconds,
                    "events": "\(result.visibleEventCount)",
                    "tasks": "\(result.visibleTaskCount)",
                    "totalMs": HCBPerformanceTelemetry.elapsedMilliseconds(since: requestedAt),
                    "weeks": "\(result.weekSnapshots.count)"
                ]
            )
        }
    }

    private nonisolated static func buildMonthGridCache(_ payload: MonthGridCachePayload) -> MonthGridCacheResult {
        let started = HCBPerformanceTelemetry.timestamp()
        let first = payload.weekStarts.first ?? payload.anchorDate
        let last = payload.weekStarts.last.flatMap {
            payload.calendar.date(byAdding: .day, value: 6, to: $0)
        } ?? first
        let filteredResult = filteredEvents(payload: payload, from: first, to: last)
        let filtered = filteredResult.events
        let byDay = eventsByDayKey(
            filtered,
            from: first,
            to: last,
            calendar: payload.calendar
        )
        let bandsByWeek = buildBandsByWeek(
            weekStarts: payload.weekStarts,
            byDay: byDay,
            calendar: payload.calendar
        )
        let tasksByDay = buildTasksByDay(payload: payload, from: first, to: last)
        let weekSnapshots = buildWeekSnapshots(
            weekStarts: payload.weekStarts,
            byDay: byDay,
            bandsByWeek: bandsByWeek,
            tasksByDay: tasksByDay,
            calendar: payload.calendar,
            laneHeight: payload.laneHeight,
            laneSpacing: payload.laneSpacing,
            maxVisibleLanes: payload.maxVisibleLanes,
            calendarColorHexByID: payload.calendarColorHexByID
        )
        return MonthGridCacheResult(
            key: payload.key,
            weekSnapshots: weekSnapshots,
            eventSearchTextByID: filteredResult.eventSearchTextByID,
            eventSearchRevision: payload.dataRevision,
            visibleEventCount: filtered.count,
            visibleTaskCount: tasksByDay.values.reduce(0) { $0 + $1.count },
            buildDurationMilliseconds: HCBPerformanceTelemetry.elapsedMilliseconds(since: started)
        )
    }

    private nonisolated static func filteredEvents(
        payload: MonthGridCachePayload,
        from rangeStart: Date,
        to rangeEnd: Date
    ) -> (events: [CalendarEventMirror], eventSearchTextByID: [CalendarEventMirror.ID: String]) {
        let q = normalizedSearchText(payload.searchQuery.trimmingCharacters(in: .whitespacesAndNewlines))
        var eventSearchTextByID = payload.eventSearchTextByID

        var seen: Set<CalendarEventMirror.ID> = []
        var out: [CalendarEventMirror] = []
        var cursor = payload.calendar.startOfDay(for: rangeStart)
        let last = payload.calendar.startOfDay(for: rangeEnd)
        while cursor <= last {
            let key = cursor.timeIntervalSinceReferenceDate
            for eventID in payload.visibleRange.eventsByDay[key] ?? [] where seen.insert(eventID).inserted {
                guard let event = payload.visibleRange.eventByID[eventID],
                      payload.selectedCalendarIDs.contains(event.calendarID) else { continue }
                guard payload.eventViewFilter.allows(event) else { continue }
                let matchesSearch: Bool
                if q.isEmpty {
                    matchesSearch = true
                } else {
                    let searchText = eventSearchTextByID[event.id]
                        ?? payload.visibleRange.eventSearchTextByID[event.id].map(normalizedSearchText)
                        ?? normalizedEventSearchText(event)
                    eventSearchTextByID[event.id] = searchText
                    matchesSearch = searchText.contains(q)
                }
                if matchesSearch {
                    out.append(event)
                }
            }
            guard let next = payload.calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
        }

        let sorted = out.sorted { lhs, rhs in
            if lhs.startDate != rhs.startDate { return lhs.startDate < rhs.startDate }
            return lhs.id < rhs.id
        }
        return (sorted, eventSearchTextByID)
    }

    private nonisolated static func normalizedEventSearchText(_ event: CalendarEventMirror) -> String {
        normalizedSearchText("\(event.summary)\n\(event.details)\n\(event.location)")
    }

    private nonisolated static func normalizedSearchText(_ text: String) -> String {
        text
            .folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
            .lowercased()
    }

    private nonisolated static func eventsByDayKey(
        _ events: [CalendarEventMirror],
        from rangeStart: Date,
        to rangeEnd: Date,
        calendar: Calendar
    ) -> [TimeInterval: [CalendarEventMirror]] {
        let startOfRange = calendar.startOfDay(for: rangeStart)
        let endOfRange = calendar.startOfDay(for: rangeEnd)
        var bucket: [TimeInterval: [CalendarEventMirror]] = [:]

        for event in events where event.status != .cancelled {
            let eventStartDay = calendar.startOfDay(for: event.startDate)
            let inclusiveEndDay = CalendarGridLayout.eventEndDay(event: event, calendar: calendar)
            let firstDay = max(eventStartDay, startOfRange)
            let lastDay = min(inclusiveEndDay, endOfRange)
            guard firstDay <= lastDay else { continue }

            var cursor = firstDay
            while cursor <= lastDay {
                bucket[dayKey(for: cursor, calendar: calendar), default: []].append(event)
                guard let next = calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
                cursor = next
            }
        }

        for key in bucket.keys {
            bucket[key]?.sort { lhs, rhs in
                if lhs.isAllDay != rhs.isAllDay { return lhs.isAllDay && rhs.isAllDay == false }
                return lhs.startDate < rhs.startDate
            }
        }
        return bucket
    }

    private nonisolated static func buildBandsByWeek(
        weekStarts: [Date],
        byDay: [TimeInterval: [CalendarEventMirror]],
        calendar: Calendar
    ) -> [Date: [CalendarGridLayout.MonthBand]] {
        var bandsByWeek: [Date: [CalendarGridLayout.MonthBand]] = [:]
        for weekStart in weekStarts {
            let days = weekDays(startingAt: weekStart, calendar: calendar)
            var seen: Set<CalendarEventMirror.ID> = []
            var weekEvents: [CalendarEventMirror] = []
            for day in days {
                let dayStartKey = dayKey(for: day, calendar: calendar)
                for event in byDay[dayStartKey] ?? [] where seen.insert(event.id).inserted {
                    weekEvents.append(event)
                }
            }
            bandsByWeek[weekStart] = CalendarGridLayout.monthBands(for: days, events: weekEvents, calendar: calendar)
        }
        return bandsByWeek
    }

    private nonisolated static func buildWeekSnapshots(
        weekStarts: [Date],
        byDay: [TimeInterval: [CalendarEventMirror]],
        bandsByWeek: [Date: [CalendarGridLayout.MonthBand]],
        tasksByDay: [TimeInterval: [TaskMirror]],
        calendar: Calendar,
        laneHeight: CGFloat,
        laneSpacing: CGFloat,
        maxVisibleLanes: Int,
        calendarColorHexByID: [CalendarListMirror.ID: String]
    ) -> [MonthWeekSnapshot] {
        weekStarts.map { weekStart in
            let snapshots = weekDays(startingAt: weekStart, calendar: calendar).map { day in
                let dayStart = calendar.startOfDay(for: day)
                let dayStartKey = dayKey(for: dayStart, calendar: calendar)
                let allEvents = byDay[dayStartKey] ?? []
                return MonthDaySnapshot(
                    day: day,
                    dayStart: dayStart,
                    dayStartKey: dayStartKey,
                    dayLabel: dayLabel(for: day, calendar: calendar),
                    accessibilityDateLabel: accessibilityDateLabel(for: day),
                    isToday: calendar.isDateInToday(day),
                    allEvents: allEvents,
                    timedEvents: allEvents.filter { CalendarGridLayout.isBandEvent($0, calendar: calendar) == false },
                    tasks: tasksByDay[dayStartKey] ?? [],
                    eventLabels: Dictionary(
                        uniqueKeysWithValues: allEvents.map { event in
                            (event.id, eventLabel(event, in: day, calendar: calendar))
                        }
                    ),
                    eventAccessibilityLabels: Dictionary(
                        uniqueKeysWithValues: allEvents.map { event in
                            (event.id, "\(event.summary) on \(accessibilityDateLabel(for: day))")
                        }
                    ),
                    eventColorHexes: Dictionary(
                        uniqueKeysWithValues: allEvents.compactMap { event in
                            resolvedEventColorHex(event, calendarColorHexByID: calendarColorHexByID)
                                .map { (event.id, $0) }
                        }
                    ),
                    taskAccessibilityLabels: Dictionary(
                        uniqueKeysWithValues: (tasksByDay[dayStartKey] ?? []).map { task in
                            let state = task.isCompleted ? "completed task" : "task"
                            return (task.id, "\(state): \(TagExtractor.stripped(from: task.title))")
                        }
                    )
                )
            }
            let bands = bandsByWeek[weekStart] ?? []
            return MonthWeekSnapshot(
                weekStart: weekStart,
                days: snapshots,
                bands: bands,
                bandReserves: bandReserves(
                    for: bands,
                    laneHeight: laneHeight,
                    laneSpacing: laneSpacing,
                    maxVisibleLanes: maxVisibleLanes
                ),
                bandAccessibilityLabels: bandAccessibilityLabels(for: bands),
                bandColorHexes: Dictionary(
                    uniqueKeysWithValues: bands.compactMap { band in
                        resolvedEventColorHex(band.event, calendarColorHexByID: calendarColorHexByID)
                            .map { (band.id, $0) }
                    }
                )
            )
        }
    }

    private nonisolated static func resolvedEventColorHex(
        _ event: CalendarEventMirror,
        calendarColorHexByID: [CalendarListMirror.ID: String]
    ) -> String? {
        CalendarEventColor.from(colorId: event.colorId).hex ?? calendarColorHexByID[event.calendarID]
    }

    private nonisolated static func bandAccessibilityLabels(for bands: [CalendarGridLayout.MonthBand]) -> [String: String] {
        Dictionary(uniqueKeysWithValues: bands.map { band in
            (
                band.id,
                "\(band.event.summary) \(band.event.startDate.formatted(date: .abbreviated, time: .omitted)) - \(band.event.endDate.formatted(date: .abbreviated, time: .omitted))"
            )
        })
    }

    private nonisolated static func bandReserves(
        for bands: [CalendarGridLayout.MonthBand],
        laneHeight: CGFloat,
        laneSpacing: CGFloat,
        maxVisibleLanes: Int
    ) -> [CGFloat] {
        var highestVisibleLanes = Array(repeating: -1, count: 7)
        for band in bands where band.lane < maxVisibleLanes {
            let lower = max(0, band.startColumn)
            let upper = min(6, band.endColumn)
            guard lower <= upper else { continue }
            for column in lower...upper {
                highestVisibleLanes[column] = max(highestVisibleLanes[column], band.lane)
            }
        }
        return highestVisibleLanes.map { lane in
            guard lane >= 0 else { return 0 }
            return CGFloat(lane + 1) * laneHeight
                + CGFloat(lane) * laneSpacing
                + 4
        }
    }

    private nonisolated static func buildTasksByDay(
        payload: MonthGridCachePayload,
        from rangeStart: Date,
        to rangeEnd: Date
    ) -> [TimeInterval: [TaskMirror]] {
        var tasksByDay: [TimeInterval: [TaskMirror]] = [:]
        var cursor = payload.calendar.startOfDay(for: rangeStart)
        let last = payload.calendar.startOfDay(for: rangeEnd)
        while cursor <= last {
            let key = dayKey(for: cursor, calendar: payload.calendar)
            let tasks = (payload.visibleRange.tasksByDueDate[key] ?? [])
                .compactMap { payload.visibleRange.taskByID[$0] }
                .filter { payload.visibleTaskListIDs.contains($0.taskListID) }
            if tasks.isEmpty == false {
                tasksByDay[key] = tasks
            }
            guard let next = payload.calendar.date(byAdding: .day, value: 1, to: cursor) else { break }
            cursor = next
        }
        return tasksByDay
    }

    private nonisolated static func weekDays(startingAt weekStart: Date, calendar: Calendar) -> [Date] {
        (0..<7).compactMap { calendar.date(byAdding: .day, value: $0, to: weekStart) }
    }

    private nonisolated static func dayKey(for date: Date, calendar: Calendar) -> TimeInterval {
        calendar.startOfDay(for: date).timeIntervalSinceReferenceDate
    }

    private nonisolated static func eventLabel(_ event: CalendarEventMirror, in day: Date, calendar: Calendar) -> String {
        if event.isAllDay { return event.summary }
        let start = calendar.startOfDay(for: event.startDate)
        let dayStart = calendar.startOfDay(for: day)
        if start < dayStart {
            return "... \(event.summary)"
        }
        return "\(event.startDate.formatted(.dateTime.hour().minute())) \(event.summary)"
    }

    private nonisolated static func dayLabel(for day: Date, calendar: Calendar) -> String {
        if calendar.component(.day, from: day) == 1 {
            return day.formatted(.dateTime.month(.abbreviated).day().year())
        }
        return "\(calendar.component(.day, from: day))"
    }

    private nonisolated static func accessibilityDateLabel(for day: Date) -> String {
        day.formatted(.dateTime.weekday(.wide).month(.wide).day().year())
    }

    private func monthKey(for date: Date) -> String {
        let comps = calendar.dateComponents([.year, .month], from: date)
        return "\(comps.year ?? 0)-\(comps.month ?? 0)"
    }

    // MARK: - Window / scroll plumbing

    // Returns the start-of-week (locale-aware) for a given date.
    private func startOfWeek(for date: Date) -> Date {
        calendar.dateInterval(of: .weekOfYear, for: date)?.start
            ?? calendar.startOfDay(for: date)
    }

    // Rebuilds the rolling window of week-start dates around the given date.
    // Past/future month counts come from local UI preferences; the default
    // preserves a bounded current-month start while keeping a year ahead ready.
    private func buildWindow(centeredOn date: Date) {
        let anchorMonth = monthStart(containing: date)
        let firstMonth = calendar.date(
            byAdding: .month,
            value: -CalendarMonthScrollWindow.clampedPast(configuredPastMonths),
            to: anchorMonth
        ) ?? anchorMonth
        let lastMonth = calendar.date(
            byAdding: .month,
            value: CalendarMonthScrollWindow.clampedFuture(configuredFutureMonths),
            to: anchorMonth
        ) ?? anchorMonth
        setWindow(firstMonth: firstMonth, lastMonth: lastMonth)
    }

    private func firstVisibleWeekOfMonth(containing date: Date) -> Date {
        startOfWeek(for: monthStart(containing: date))
    }

    private func lastVisibleWeekOfMonth(containing date: Date) -> Date {
        let start = monthStart(containing: date)
        let nextMonth = calendar.date(byAdding: .month, value: 1, to: start) ?? start
        let lastDay = calendar.date(byAdding: .day, value: -1, to: nextMonth) ?? start
        return startOfWeek(for: lastDay)
    }

    private func monthStart(containing date: Date) -> Date {
        let comps = calendar.dateComponents([.year, .month], from: date)
        return calendar.date(from: DateComponents(year: comps.year, month: comps.month, day: 1)) ?? calendar.startOfDay(for: date)
    }

    private func setWindow(firstMonth: Date, lastMonth: Date) {
        let normalizedFirst = monthStart(containing: firstMonth)
        let normalizedLast = max(monthStart(containing: lastMonth), normalizedFirst)
        windowFirstMonthStart = normalizedFirst
        windowLastMonthStart = normalizedLast
        weekStarts = weeks(from: normalizedFirst, through: normalizedLast)
        windowStart = weekStarts.first ?? firstVisibleWeekOfMonth(containing: normalizedFirst)
    }

    private func weeks(from firstMonth: Date, through lastMonth: Date) -> [Date] {
        let firstWeek = firstVisibleWeekOfMonth(containing: firstMonth)
        let lastWeek = lastVisibleWeekOfMonth(containing: lastMonth)
        var out: [Date] = []
        var cursor = firstWeek
        while cursor <= lastWeek {
            out.append(cursor)
            guard let next = calendar.date(byAdding: .day, value: 7, to: cursor) else { break }
            cursor = next
        }
        return out
    }

    private func weekDays(startingAt weekStart: Date) -> [Date] {
        (0..<7).compactMap { calendar.date(byAdding: .day, value: $0, to: weekStart) }
    }

    private func isDateInWindow(_ date: Date) -> Bool {
        guard let first = weekStarts.first, let last = weekStarts.last else { return false }
        let endExclusive = calendar.date(byAdding: .day, value: 7, to: last) ?? last
        return date >= first && date < endExclusive
    }

    // Programmatic jumps in month mode open at the first visible week of the
    // selected month. The month/year context now lives inside the grid, so the
    // top chrome does not need to chase the scroll position.
    private func scrollToAnchor(proxy: ScrollViewProxy, animated: Bool) {
        let target = firstVisibleWeekOfMonth(containing: anchorDate)
        guard weekStarts.contains(target) else { return }
        if animated {
            HCBMotion.perform(reduceMotion: calendarGridReduceMotion, animation: .easeOut(duration: 0.16)) {
                proxy.scrollTo(target, anchor: .top)
            }
        } else {
            proxy.scrollTo(target, anchor: .top)
        }
    }

    private func requestAnchorScroll(animated: Bool, completesInitialScroll: Bool = false) {
        pendingAnchorScroll = PendingAnchorScroll(
            animated: animated,
            completesInitialScroll: completesInitialScroll
        )
    }

    private func flushPendingAnchorScroll(proxy: ScrollViewProxy) {
        guard let pendingAnchorScroll else { return }
        let target = firstVisibleWeekOfMonth(containing: anchorDate)
        guard cachedWeekSnapshots.contains(where: { $0.weekStart == target }) else { return }
        self.pendingAnchorScroll = nil
        scheduleScrollToAnchor(proxy: proxy, animated: pendingAnchorScroll.animated) {
            if pendingAnchorScroll.completesInitialScroll {
                didPerformInitialScroll = true
            }
        }
    }

    private func scheduleScrollToAnchor(
        proxy: ScrollViewProxy,
        animated: Bool,
        completion: (() -> Void)? = nil
    ) {
        Task { @MainActor in
            await Task.yield()
            scrollToAnchor(proxy: proxy, animated: animated)
            completion?()
        }
    }

    private func scheduleScroll(
        proxy: ScrollViewProxy,
        to target: Date,
        anchor: UnitPoint,
        animated: Bool
    ) {
        Task { @MainActor in
            await Task.yield()
            if animated {
                HCBMotion.perform(reduceMotion: calendarGridReduceMotion, animation: .easeOut(duration: 0.16)) {
                    proxy.scrollTo(target, anchor: anchor)
                }
            } else {
                proxy.scrollTo(target, anchor: anchor)
            }
        }
    }

    private func loadPreviousMonth(proxy: ScrollViewProxy) {
        guard didPerformInitialScroll, isLoadingPreviousMonth == false, let firstWeek = weekStarts.first else { return }
        isLoadingPreviousMonth = true
        Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(90))
            let previousMonth = calendar.date(byAdding: .month, value: -1, to: windowFirstMonthStart) ?? windowFirstMonthStart
            let newWeeks = weeks(from: previousMonth, through: previousMonth).filter { $0 < firstWeek }
            if newWeeks.isEmpty == false {
                weekStarts = newWeeks + weekStarts
                windowFirstMonthStart = monthStart(containing: previousMonth)
                windowStart = weekStarts.first ?? windowStart
                rebuildGridCacheIfNeeded()
                scheduleScroll(proxy: proxy, to: firstWeek, anchor: .bottom, animated: false)
            }
            isLoadingPreviousMonth = false
        }
    }

    private func loadNextMonth(proxy: ScrollViewProxy) {
        guard didPerformInitialScroll, isLoadingNextMonth == false, let lastWeek = weekStarts.last else { return }
        isLoadingNextMonth = true
        Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(90))
            let nextMonth = calendar.date(byAdding: .month, value: 1, to: windowLastMonthStart) ?? windowLastMonthStart
            let newWeeks = weeks(from: nextMonth, through: nextMonth).filter { $0 > lastWeek }
            if newWeeks.isEmpty == false {
                weekStarts.append(contentsOf: newWeeks)
                windowLastMonthStart = monthStart(containing: nextMonth)
                rebuildGridCacheIfNeeded()
            }
            isLoadingNextMonth = false
        }
    }

    // Reverse-sync: as the user scrolls, figure out which week sits at the
    // visible-center and push its month into anchorDate. skipNextScrollSync
    // short-circuits the resulting onChange(of: anchorDate) so programmatic
    // scroll doesn't fight live scrolling.
    private func updateAnchorFromScroll(centerY: CGFloat) {
        // Suppress until the initial scrollTo has positioned the viewport.
        // Without this, the first preference-change callback (fired at the
        // moment the ScrollView lays out at offset 0) runs before onAppear's
        // scrollTo and stomps anchorDate to the top of the window.
        guard didPerformInitialScroll else { return }
        guard fixedRowHeight > 0 else { return }
        let gridY = centerY - previousMonthLoaderHeight
        guard gridY >= 0 else { return }
        let index = Int((gridY / fixedRowHeight).rounded(.down))
        guard weekStarts.indices.contains(index) else { return }
        let weekStart = weekStarts[index]
        // Use Thursday (offset 3) as the "representative" day — ISO week
        // numbering treats the Thursday as the canonical week, which matches
        // how users intuitively assign a week to a month.
        let midDay = calendar.date(byAdding: .day, value: 3, to: weekStart) ?? weekStart
        if monthKey(for: midDay) != monthKey(for: anchorDate) {
            skipNextScrollSync = true
            anchorDate = midDay
            HCBPerformanceTelemetry.debug(
                "month scroll anchor changed",
                metadata: [
                    "anchor": monthKey(for: midDay),
                    "weekIndex": "\(index)",
                    "weeks": "\(weekStarts.count)"
                ]
            )
        }
    }

    private var weekdayHeader: some View {
        HStack(spacing: 0) {
            ForEach(weekdaySymbols.indices, id: \.self) { index in
                Text(weekdaySymbols[index])
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
            }
        }
        .hcbScaledPadding(.vertical, 8)
        .background {
            if usesReadableMonthBackings {
                Rectangle()
                    .fill(.regularMaterial)
                    .overlay(AppColor.cream.opacity(0.20))
            }
        }
    }

    private func weekRow(snapshot: MonthWeekSnapshot) -> some View {
        let days = snapshot.days.map(\.day)
        return GeometryReader { geo in
            let cellWidth = geo.size.width / CGFloat(max(days.count, 1))
            ZStack(alignment: .topLeading) {
                HStack(spacing: 0) {
                    ForEach(Array(snapshot.days.enumerated()), id: \.element.id) { col, daySnapshot in
                        let cellBandReserve = snapshot.bandReserves.indices.contains(col) ? snapshot.bandReserves[col] : 0
                        monthCell(snapshot: daySnapshot, bandReserve: cellBandReserve)
                            .frame(maxWidth: .infinity, minHeight: fixedRowHeight, maxHeight: fixedRowHeight, alignment: .top)
                            .clipped() // stop per-cell VStack overflow (day number + bandReserve + 2 events + 2 tasks + "+N more" can exceed fixedRowHeight) from bleeding into the next week row
                    }
                }
                bandOverlay(
                    bands: snapshot.bands,
                    accessibilityLabels: snapshot.bandAccessibilityLabels,
                    colorHexes: snapshot.bandColorHexes,
                    cellWidth: cellWidth
                )
                monthBoundaryOverlay(days: days, cellWidth: cellWidth)
            }
        }
        .frame(height: fixedRowHeight)
        .clipped() // defense-in-depth: also clip the row itself so band overlays can't leak out
    }

    private func previousMonthLoader(proxy: ScrollViewProxy) -> some View {
        HStack(spacing: 8) {
            if isLoadingPreviousMonth {
                ProgressView()
                    .controlSize(.small)
                Text("Loading previous month…")
                    .hcbFont(.caption, weight: .medium)
            } else {
                Image(systemName: "chevron.up")
                    .hcbFont(.caption, weight: .semibold)
                Text("Scroll up to load previous month")
                    .hcbFont(.caption, weight: .medium)
            }
        }
        .foregroundStyle(.secondary)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.regularMaterial.opacity(0.35))
        .onAppear {
            loadPreviousMonth(proxy: proxy)
        }
    }

    private func nextMonthLoader(proxy: ScrollViewProxy) -> some View {
        HStack(spacing: 8) {
            if isLoadingNextMonth {
                ProgressView()
                    .controlSize(.small)
                Text("Loading next month…")
                    .hcbFont(.caption, weight: .medium)
            } else {
                Text("Scroll down to load next month")
                    .hcbFont(.caption, weight: .medium)
                Image(systemName: "chevron.down")
                    .hcbFont(.caption, weight: .semibold)
            }
        }
        .foregroundStyle(.secondary)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.regularMaterial.opacity(0.35))
        .onAppear {
            loadNextMonth(proxy: proxy)
        }
    }

    // Resolves a gesture point — given in the "monthGridContent" named
    // coordinate space, i.e. relative to the LazyVStack's top-leading — into
    // a concrete date. Row derives from y / fixedRowHeight; column from
    // x / (contentWidth / 7). Clamped so a drag that overshoots the grid
    // doesn't cause the selection to disappear mid-gesture.
    private func dateAtContentPoint(_ point: CGPoint) -> Date? {
        guard fixedRowHeight > 0, weekStarts.isEmpty == false else { return nil }
        let gridY = point.y - previousMonthLoaderHeight
        guard gridY >= 0 else { return nil }
        let rowIndex = max(0, min(weekStarts.count - 1, Int(gridY / fixedRowHeight)))
        let cellWidth = visibleGridWidth / 7
        guard cellWidth > 0 else { return nil }
        let col = max(0, min(6, Int(point.x / cellWidth)))
        return calendar.date(byAdding: .day, value: col, to: weekStarts[rowIndex])
    }

    // Drag highlight overlay, drawn over the LazyVStack in the same named
    // coordinate space as the gesture. Each week that intersects the
    // selection contributes one highlight rectangle at its correct row offset
    // — enables the multi-week drag-create UX across partial-month bounds.
    @ViewBuilder
    private var dragHighlightOverlay: some View {
        if let sel = dragSelection, visibleGridWidth > 0 {
            let cellWidth = visibleGridWidth / 7
            let (from, to) = sel.normalized
            let fromStart = calendar.startOfDay(for: from)
            let toStart = calendar.startOfDay(for: to)
            let highlightRows = dragHighlightRowRange(fromStart: fromStart, toStart: toStart)
            ZStack(alignment: .topLeading) {
                ForEach(Array(highlightRows), id: \.self) { index in
                    let weekStart = weekStarts[index]
                    let weekDays = (0..<7).compactMap { calendar.date(byAdding: .day, value: $0, to: weekStart) }
                    if let (colStart, colEnd) = rowRange(weekDays, fromStart: fromStart, toStart: toStart) {
                        let left = CGFloat(colStart) * cellWidth
                        let width = CGFloat(colEnd - colStart + 1) * cellWidth
                        RoundedRectangle(cornerRadius: 6)
                            .fill(AppColor.ember.opacity(0.2))
                            .overlay(
                                RoundedRectangle(cornerRadius: 6)
                                    .strokeBorder(AppColor.ember.opacity(0.6), lineWidth: 1.2)
                            )
                            .frame(width: max(width - 4, 4), height: fixedRowHeight - 4)
                            .offset(x: left + 2, y: previousMonthLoaderHeight + CGFloat(index) * fixedRowHeight + 2)
                            .allowsHitTesting(false)
                    }
                }
            }
        }
    }

    private func dragHighlightRowRange(fromStart: Date, toStart: Date) -> Range<Int> {
        guard let firstWeek = weekStarts.first, weekStarts.isEmpty == false else { return 0..<0 }
        let firstSelectionWeek = startOfWeek(for: fromStart)
        let lastSelectionWeek = startOfWeek(for: toStart)
        let firstIndex = calendar.dateComponents([.day], from: firstWeek, to: firstSelectionWeek).day.map { $0 / 7 } ?? 0
        let lastIndex = calendar.dateComponents([.day], from: firstWeek, to: lastSelectionWeek).day.map { $0 / 7 } ?? 0
        let lower = max(0, min(firstIndex, lastIndex))
        let upper = min(weekStarts.count - 1, max(firstIndex, lastIndex))
        guard lower <= upper else { return 0..<0 }
        return lower..<(upper + 1)
    }

    private func monthBoundaryOverlay(days: [Date], cellWidth: CGFloat) -> some View {
        ZStack(alignment: .topLeading) {
            ForEach(Array(days.enumerated()), id: \.element) { column, day in
                if calendar.component(.day, from: day) == 1 {
                    let x = CGFloat(column) * cellWidth
                    let line = monthBoundaryLineColor
                    ZStack(alignment: .topLeading) {
                        Rectangle()
                            .fill(line)
                            .frame(width: cellWidth * CGFloat(7 - column), height: 1.5)
                            .offset(x: x, y: 0)

                        if column > 0 {
                            Rectangle()
                                .fill(line)
                                .frame(width: 1.5, height: fixedRowHeight)
                                .offset(x: x, y: 0)

                            Rectangle()
                                .fill(line)
                                .frame(width: x, height: 1.5)
                                .offset(x: 0, y: fixedRowHeight - 1.5)
                        }
                    }
                    .allowsHitTesting(false)
                }
            }
        }
    }

    private var monthBoundaryLineColor: Color {
        AppColor.ember.opacity(0.55)
    }

    // Backing visible-width cache for dateAtContentPoint, written by the
    // overlay GeometryReader. Using @State here avoids forcing a full
    // GeometryReader around the gesture — the DragGesture already receives
    // absolute content coords via the named coordinate space, we only need
    // to know the row width to resolve the column.
    @State private var visibleGridWidth: CGFloat = 0

    // Intersects a row's days with the inclusive [fromStart, toStart] range
    // and returns the contiguous column span, or nil when the row is fully
    // outside the selection.
    private func rowRange(
        _ row: [Date],
        fromStart: Date,
        toStart: Date
    ) -> (Int, Int)? {
        var first: Int?
        var last: Int?
        for (idx, day) in row.enumerated() {
            let d = calendar.startOfDay(for: day)
            if d >= fromStart && d <= toStart {
                if first == nil { first = idx }
                last = idx
            }
        }
        if let first, let last { return (first, last) }
        return nil
    }

    private func bandOverlay(
        bands: [CalendarGridLayout.MonthBand],
        accessibilityLabels: [String: String],
        colorHexes: [String: String],
        cellWidth: CGFloat
    ) -> some View {
        let dayNumberReserve: CGFloat = 24 // matches the hcbScaledPadding(6) + day-number row height
        let renderItems = monthBandRenderItems(
            bands: bands,
            accessibilityLabels: accessibilityLabels,
            colorHexes: colorHexes,
            cellWidth: cellWidth,
            dayNumberReserve: dayNumberReserve
        )
        return ZStack(alignment: .topLeading) {
            ForEach(renderItems) { item in
                monthBandTile(item)
            }
        }
        .frame(maxWidth: .infinity, alignment: .topLeading)
    }

    private func monthBandRenderItems(
        bands: [CalendarGridLayout.MonthBand],
        accessibilityLabels: [String: String],
        colorHexes: [String: String],
        cellWidth: CGFloat,
        dayNumberReserve: CGFloat
    ) -> [MonthBandRenderItem] {
        bands.compactMap { band in
            guard band.lane < maxVisibleLanes else { return nil }
            let durationMinutes = Int(max(band.event.endDate.timeIntervalSince(band.event.startDate) / 60, 15))
            return MonthBandRenderItem(
                id: band.id,
                band: band,
                accessibilityLabel: accessibilityLabels[band.id] ?? band.event.summary,
                colorHex: colorHexes[band.id],
                frameWidth: cellWidth * CGFloat(band.endColumn - band.startColumn + 1) - 4,
                xOffset: cellWidth * CGFloat(band.startColumn) + 2,
                yOffset: dayNumberReserve + CGFloat(band.lane) * (laneHeight + laneSpacing),
                dragPayload: DraggedEvent(
                    eventID: band.event.id,
                    calendarID: band.event.calendarID,
                    durationMinutes: durationMinutes,
                    isAllDay: band.event.isAllDay
                )
            )
        }
    }

    private func monthBandTile(_ item: MonthBandRenderItem) -> some View {
        let fillColor = calendarColor(hex: item.colorHex)
        return CalendarEventPreviewButton(event: item.band.event) {
            monthBandLabel(title: item.band.event.summary, fillColor: fillColor)
        }
        .accessibilityLabel(item.accessibilityLabel)
        .frame(width: item.frameWidth)
        .offset(x: item.xOffset, y: item.yOffset)
        .draggable(item.dragPayload) {
            monthBandDragPreview(title: item.band.event.summary, fillColor: fillColor)
        }
    }

    private func monthBandLabel(title: String, fillColor: Color) -> some View {
        Text(title)
            .hcbFont(.caption2)
            .lineLimit(1)
            .hcbScaledPadding(.horizontal, 6)
            .hcbScaledPadding(.vertical, 2)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(
                RoundedRectangle(cornerRadius: 4)
                    .fill(fillColor.opacity(monthEventFillOpacity))
            )
            .foregroundStyle(AppColor.ink)
    }

    private func monthBandDragPreview(title: String, fillColor: Color) -> some View {
        Text(title)
            .hcbFont(.caption)
            .hcbScaledPadding(.horizontal, 8)
            .hcbScaledPadding(.vertical, 4)
            .background(Capsule().fill(fillColor.opacity(0.35)))
    }

    private func monthCell(snapshot: MonthDaySnapshot, bandReserve: CGFloat) -> some View {
        let day = snapshot.day
        let dayStart = snapshot.dayStart
        let allEventsToday = snapshot.allEvents
        let events = snapshot.timedEvents
        let tasks = snapshot.tasks
        let isDenseMonth = allEventsToday.count + tasks.count >= CalendarDensityRendering.denseMonthItemThreshold
        // +N more counts the hidden band events too so users see them accounted for.
        let hiddenBandEvents = max(0, allEventsToday.count - events.count - visibleBandCount(for: dayStart, in: day, allEventsToday: allEventsToday))
        let rowStride = laneHeight + laneSpacing
        let dayNumberReserve: CGFloat = 24
        let verticalPadding: CGFloat = 8
        let availableRowArea = max(0, fixedRowHeight - verticalPadding - dayNumberReserve - bandReserve)
        let availableRows = max(0, Int(floor((availableRowArea + laneSpacing) / rowStride)))
        let needsMoreButton = isDenseMonth || hiddenBandEvents > 0 || events.count + tasks.count > availableRows
        let visibleItemRows = needsMoreButton ? max(0, availableRows - 1) : availableRows
        let visibleEventCount = isDenseMonth ? 0 : min(events.count, visibleItemRows)
        let visibleTaskCount = isDenseMonth ? 0 : min(tasks.count, max(0, visibleItemRows - visibleEventCount))
        let hiddenEvents = isDenseMonth ? allEventsToday.count : max(0, events.count - visibleEventCount) + hiddenBandEvents
        let hiddenTasks = isDenseMonth ? tasks.count : max(0, tasks.count - visibleTaskCount)
        return VStack(alignment: .leading, spacing: 3) {
            HStack {
                Text(snapshot.dayLabel)
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(dayNumberColor(isToday: snapshot.isToday))
                    .lineLimit(1)
                    .hcbScaledPadding(.horizontal, 6)
                    .hcbScaledPadding(.vertical, 2)
                    .background(
                        Capsule()
                            .fill(snapshot.isToday ? AppColor.ember.opacity(0.25) : .clear)
                    )
                Spacer(minLength: 0)
            }
            if bandReserve > 0 {
                Color.clear.frame(height: bandReserve)
            }
            if isDenseMonth {
                denseMonthSummary(events: allEventsToday, tasks: tasks)
            } else {
                ForEach(events.prefix(visibleEventCount), id: \.id) { event in
                    CalendarEventPreviewButton(event: event) {
                        Text(snapshot.eventLabels[event.id] ?? event.summary)
                            .hcbFont(.caption2)
                            .lineLimit(1)
                            .hcbScaledPadding(.horizontal, 6)
                            .hcbScaledPadding(.vertical, 2)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .background(
                                RoundedRectangle(cornerRadius: 4)
                                    .fill(calendarColor(for: event, in: snapshot).opacity(monthEventFillOpacity))
                            )
                            .foregroundStyle(AppColor.ink)
                    }
                    .accessibilityLabel(snapshot.eventAccessibilityLabels[event.id] ?? event.summary)
                    .draggable(DraggedEvent(
                        eventID: event.id,
                        calendarID: event.calendarID,
                        durationMinutes: Int(max(event.endDate.timeIntervalSince(event.startDate) / 60, 15)),
                        isAllDay: event.isAllDay
                    )) {
                        Text(event.summary)
                            .hcbFont(.caption)
                            .hcbScaledPadding(.horizontal, 8)
                            .hcbScaledPadding(.vertical, 4)
                            .background(Capsule().fill(calendarColor(for: event, in: snapshot).opacity(0.35)))
                    }
                }
                ForEach(tasks.prefix(visibleTaskCount)) { task in
                    HStack(spacing: 3) {
                        CalendarTaskCheckbox(task: task, size: 9)
                        CalendarTaskPreviewButton(task: task) {
                            Text(task.title)
                                .hcbFont(.caption2)
                                .lineLimit(1)
                                .frame(maxWidth: .infinity, alignment: .leading)
                                .contentShape(Rectangle())
                        }
                    }
                    .hcbScaledPadding(.horizontal, 6)
                    .hcbScaledPadding(.vertical, 2)
                    .background(
                        RoundedRectangle(cornerRadius: 4)
                            .fill(AppColor.ember.opacity(monthTaskFillOpacity))
                    )
                    .overlay(
                        RoundedRectangle(cornerRadius: 4)
                            .strokeBorder(AppColor.ember.opacity(monthTaskStrokeOpacity), lineWidth: 0.6)
                    )
                    .foregroundStyle(AppColor.ink)
                    .strikethrough(task.isCompleted, color: .secondary)
                    .opacity(task.isCompleted ? 0.55 : 1.0)
                }
            }
            Spacer(minLength: 0)
        }
        // Horizontal cell padding matches the bandOverlay's 2pt offset so
        // timed-event tiles line up cell-edge to cell-edge with multi-day
        // bands. Vertical padding kept at 4pt for breathing room around
        // the day number and stack of tiles.
        .hcbScaledPadding(.horizontal, 2)
        .hcbScaledPadding(.vertical, 4)
        .frame(maxWidth: .infinity, alignment: .topLeading)
        .background {
            if usesReadableMonthBackings {
                RoundedRectangle(cornerRadius: 6, style: .continuous)
                    .fill(.thinMaterial)
            }
        }
        .background(
            RoundedRectangle(cornerRadius: 6, style: .continuous)
                .fill(AppColor.cream.opacity(monthCellBackingOpacity))
        )
        .accessibilityElement(children: .combine)
        .accessibilityLabel(monthCellAccessibilityLabel(
            snapshot: snapshot,
            visibleEvents: Array(events.prefix(visibleEventCount)),
            visibleTasks: Array(tasks.prefix(visibleTaskCount)),
            hiddenEvents: hiddenEvents,
            hiddenTasks: hiddenTasks
        ))
        .overlay(
            RoundedRectangle(cornerRadius: 6, style: .continuous)
                .strokeBorder(AppColor.cardStroke.opacity(monthCellStrokeOpacity), lineWidth: 0.5)
        )
        .overlay(alignment: .bottomLeading) {
            if hiddenEvents + hiddenTasks > 0 {
                MonthMoreButton(
                    count: hiddenEvents + hiddenTasks,
                    day: dayStart,
                    events: allEventsToday,
                    tasks: tasks,
                    calendarColor: { event in calendarColor(for: event, in: snapshot) }
                )
                .padding(.horizontal, 4)
                .padding(.bottom, 3)
            }
        }
        .contentShape(Rectangle())
        .dropDestination(for: DraggedEvent.self) { items, _ in
            guard let dropped = items.first else { return false }
            Task {
                await rescheduleDroppedEvent(dropped, to: dayStart)
            }
            return true
        }
        .dropDestination(for: DraggedTask.self) { items, _ in
            guard let dropped = items.first else { return false }
            Task {
                await retargetDroppedTask(dropped, to: dayStart)
            }
            return true
        }
    }

    private func denseMonthSummary(events: [CalendarEventMirror], tasks: [TaskMirror]) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            if events.isEmpty == false {
                Label("\(events.count) events", systemImage: "square.grid.3x3.fill")
                    .hcbFont(.caption2, weight: .semibold)
                    .lineLimit(1)
            }
            if tasks.isEmpty == false {
                Label("\(tasks.count) tasks", systemImage: "checklist")
                    .hcbFont(.caption2, weight: .semibold)
                    .lineLimit(1)
            }
        }
        .foregroundStyle(AppColor.ink)
        .hcbScaledPadding(.horizontal, 6)
        .hcbScaledPadding(.vertical, 4)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 4)
                .fill(AppColor.blue.opacity(usesReadableMonthBackings ? 0.24 : 0.14))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 4)
                .strokeBorder(AppColor.blue.opacity(0.35), lineWidth: 0.6)
        )
        .accessibilityHidden(true)
    }

    // How many band events for this specific day are visible in the overlay
    // (counted against maxVisibleLanes). Used to compute +N more accurately.
    private func visibleBandCount(for dayStart: Date, in day: Date, allEventsToday: [CalendarEventMirror]) -> Int {
        let bandEventsToday = allEventsToday.filter { CalendarGridLayout.isBandEvent($0, calendar: calendar) }
        // Overlay caps to maxVisibleLanes per week — approximate here by capping to maxVisibleLanes per day.
        return min(bandEventsToday.count, maxVisibleLanes)
    }

    private func rescheduleDroppedEvent(_ dropped: DraggedEvent, to dayStart: Date) async {
        guard let event = calendarStore.event(id: dropped.eventID) else { return }
        let newStart: Date
        let newEnd: Date
        if event.isAllDay {
            newStart = dayStart
            let duration = calendar.dateComponents([.day], from: calendar.startOfDay(for: event.startDate), to: event.endDate).day ?? 0
            newEnd = calendar.date(byAdding: .day, value: max(duration, 1), to: dayStart) ?? dayStart
        } else {
            let startComponents = calendar.dateComponents([.hour, .minute, .second], from: event.startDate)
            let duration = event.endDate.timeIntervalSince(event.startDate)
            newStart = calendar.date(bySettingHour: startComponents.hour ?? 9, minute: startComponents.minute ?? 0, second: startComponents.second ?? 0, of: dayStart) ?? dayStart
            newEnd = newStart.addingTimeInterval(duration)
        }
        if event.startDate == newStart && event.endDate == newEnd { return }
        let inclusiveEnd = event.isAllDay
            ? (calendar.date(byAdding: .day, value: -1, to: newEnd) ?? newEnd)
            : newEnd
        _ = await model.updateEvent(
            event,
            summary: event.summary,
            details: event.details,
            startDate: newStart,
            endDate: inclusiveEnd,
            isAllDay: event.isAllDay,
            reminderMinutes: event.reminderMinutes.first,
            calendarID: event.calendarID,
            location: event.location,
            recurrence: event.recurrence,
            attendeeEmails: event.attendeeEmails,
            notifyGuests: false
        )
    }

    private func retargetDroppedTask(_ dropped: DraggedTask, to dayStart: Date) async {
        guard let task = calendarStore.task(id: dropped.taskID) else { return }
        _ = await model.updateTask(
            task,
            title: task.title,
            notes: task.notes,
            dueDate: dayStart
        )
    }

    private func eventLabel(_ event: CalendarEventMirror, in day: Date) -> String {
        Self.eventLabel(event, in: day, calendar: calendar)
    }

    private func monthCellAccessibilityLabel(
        snapshot: MonthDaySnapshot,
        visibleEvents: [CalendarEventMirror],
        visibleTasks: [TaskMirror],
        hiddenEvents: Int,
        hiddenTasks: Int
    ) -> String {
        var parts = [snapshot.accessibilityDateLabel]
        if snapshot.isToday {
            parts.append("Today")
        }
        for event in visibleEvents {
            parts.append("Event: \(snapshot.eventLabels[event.id] ?? eventLabel(event, in: snapshot.day))")
        }
        for task in visibleTasks {
            parts.append(snapshot.taskAccessibilityLabels[task.id] ?? task.title)
        }
        if hiddenEvents > 0 {
            parts.append("\(hiddenEvents) more event\(hiddenEvents == 1 ? "" : "s")")
        }
        if hiddenTasks > 0 {
            parts.append("\(hiddenTasks) more task\(hiddenTasks == 1 ? "" : "s")")
        }
        if parts.count == 1 {
            parts.append("No visible events or tasks")
        }
        return parts.joined(separator: ", ")
    }

    private func dayNumberColor(isToday: Bool) -> Color {
        if isToday { return AppColor.ember }
        return AppColor.ink
    }

    private func calendarColor(for event: CalendarEventMirror, in snapshot: MonthDaySnapshot) -> Color {
        calendarColor(hex: snapshot.eventColorHexes[event.id])
    }

    private func calendarColor(hex: String?) -> Color {
        guard let hex else { return AppColor.blue }
        return Color(hex: hex)
    }
}

// PreferenceKey carrying the current scroll-offset (in points, measured
// downward from the top of the content). Used by the continuous-scroll
// month grid to reverse-sync the visible center week into anchorDate.
private struct ScrollOffsetKey: PreferenceKey {
    static var defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = nextValue()
    }
}
