import SwiftUI

// §7.01 Phase D3 — year overview. 4 columns × 3 rows of mini-months.
// Each day cell shades by event-count heatmap; clicking a day jumps to
// Day view via onPickDay. Reuses the calendar surface store and the calendar-
// selection pipeline, so year heatmap updates do not observe unrelated app state.
struct YearGridView: View {
    @Environment(AppModel.self) private var model
    @Environment(\.hcbAppBackgroundConfiguration) private var backgroundConfiguration
    @Environment(\.calendarEventViewFilter) private var calendarEventViewFilter
    @Binding var anchorDate: Date
    let searchQuery: String
    let onPickDay: (Date) -> Void
    @State private var preparedYearSnapshot: CalendarYearDisplaySnapshot?
    @State private var yearSnapshotBuildTask: Task<Void, Never>?
    @State private var yearSnapshotPrewarmTask: Task<Void, Never>?

    private let calendar = Calendar.current
    private var calendarStore: CalendarStore { model.calendarStore }
    private var usesReadableMonthBackings: Bool {
        backgroundConfiguration.customImagePath != nil || backgroundConfiguration.isTranslucent
    }

    private var year: Int {
        calendar.component(.year, from: anchorDate)
    }

    var body: some View {
        Group {
            if let snapshot = preparedYearSnapshot, snapshot.key == yearSnapshotKey, calendarStore.isRebuildingDerivedSnapshots == false {
                GeometryReader { proxy in
                    let outerPadding: CGFloat = 16
                    let gridSpacing: CGFloat = 16
                    let availableHeight = max(0, proxy.size.height - outerPadding * 2 - gridSpacing * 2)
                    let monthHeight = max(210, availableHeight / 3)
                    let dayCellHeight = max(18, (monthHeight - 58) / 7)

                    ScrollView {
                        LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: gridSpacing), count: 4), spacing: gridSpacing) {
                            ForEach(snapshot.months) { month in
                                miniMonth(
                                    month,
                                    counts: snapshot.countsByDay,
                                    maxCount: snapshot.maxCount,
                                    monthHeight: monthHeight,
                                    dayCellHeight: dayCellHeight
                                )
                            }
                        }
                        .hcbScaledPadding(outerPadding)
                    }
                }
            } else {
                PreparedSnapshotOverlay(
                    title: "Preparing year...",
                    message: "Building the yearly event heatmap before enabling navigation."
                )
                .onAppear { rebuildYearSnapshotIfNeeded() }
            }
        }
        .onAppear { rebuildYearSnapshotIfNeeded() }
        .onChange(of: yearSnapshotKey) { _, _ in rebuildYearSnapshotIfNeeded() }
        .onDisappear {
            yearSnapshotBuildTask?.cancel()
            yearSnapshotPrewarmTask?.cancel()
        }
    }

    private var yearSnapshotKey: PreparedSnapshotKey {
        yearSnapshotKey(for: anchorDate)
    }

    private func yearSnapshotKey(for date: Date) -> PreparedSnapshotKey {
        PreparedSnapshotKeys.calendar(
            mode: .year,
            dataRevision: calendarStore.calendarDisplayRevisionKey(forYearContaining: date, calendar: calendar),
            selectedCalendarIDs: calendarStore.calendarSnapshot.selectedCalendarIDs,
            visibleTaskListIDs: calendarStore.visibleTaskListIDs,
            filterKey: calendarEventViewFilter.cacheKey,
            searchQuery: searchQuery,
            rangeKey: PreparedSnapshotKeys.yearKey(date, calendar: calendar),
            settings: calendarStore.settings
        )
    }

    private func miniMonth(
        _ month: CalendarYearDisplaySnapshot.Month,
        counts: [TimeInterval: Int],
        maxCount: Int,
        monthHeight: CGFloat,
        dayCellHeight: CGFloat
    ) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(month.monthName)
                .hcbFont(.subheadline, weight: .semibold)
                .foregroundStyle(AppColor.ink)
            LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 1), count: 7), spacing: 2) {
                ForEach(weekdayHeaders.indices, id: \.self) { index in
                    Text(weekdayHeaders[index])
                        .hcbFont(.caption2, weight: .medium)
                        .foregroundStyle(.secondary)
                }
                ForEach(month.cells, id: \.self) { day in
                    dayCell(
                        day: day,
                        monthNumber: month.monthNumber,
                        counts: counts,
                        maxCount: maxCount,
                        dayCellHeight: dayCellHeight
                    )
                }
            }
            Spacer(minLength: 0)
        }
        .frame(minHeight: monthHeight, alignment: .top)
        .hcbScaledPadding(10)
        .background(
            RoundedRectangle(cornerRadius: 10)
                .fill(usesReadableMonthBackings ? AppColor.cardSurface.opacity(0.86) : AppColor.cream.opacity(0.25))
                .overlay(
                    RoundedRectangle(cornerRadius: 10)
                        .fill(AppColor.cream.opacity(usesReadableMonthBackings ? 0.14 : 0))
                )
        )
        .overlay(
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(AppColor.cardStroke.opacity(usesReadableMonthBackings ? 0.9 : 1), lineWidth: 0.5)
        )
    }

    private func dayCell(
        day: Date,
        monthNumber: Int,
        counts: [TimeInterval: Int],
        maxCount: Int,
        dayCellHeight: CGFloat
    ) -> some View {
        let startOfDay = calendar.startOfDay(for: day)
        let isInMonth = calendar.component(.month, from: day) == monthNumber
        let isToday = calendar.isDateInToday(day)
        let key = CalendarDisplaySnapshotBuilder.dayKey(startOfDay, calendar: calendar)
        let count = counts[key] ?? 0
        let shade: Double = maxCount > 0 ? min(0.7, Double(count) / Double(max(maxCount, 1)) * 0.7) : 0
        return Button {
            onPickDay(startOfDay)
        } label: {
            Text("\(calendar.component(.day, from: day))")
                .hcbFontSystem(size: 9, weight: isToday ? .bold : .regular)
                .foregroundStyle(isInMonth ? AppColor.ink : .secondary.opacity(0.5))
                .frame(maxWidth: .infinity, minHeight: dayCellHeight)
                .background(
                    RoundedRectangle(cornerRadius: 3)
                        .fill(isToday ? AppColor.ember.opacity(0.35) : AppColor.ember.opacity(shade))
                )
        }
        .buttonStyle(.plain)
        .opacity(isInMonth ? 1.0 : 0.4)
        .help(count > 0 ? "\(count) event\(count == 1 ? "" : "s") on \(day.formatted(.dateTime.month(.abbreviated).day().year()))" : "\(day.formatted(.dateTime.month(.abbreviated).day().year()))")
    }

    private func rebuildYearSnapshotIfNeeded() {
        let key = yearSnapshotKey
        guard preparedYearSnapshot?.key != key else { return }
        if let snapshot = calendarStore.cachedCalendarYearSnapshot(for: key) {
            preparedYearSnapshot = snapshot
            prewarmAdjacentYearSnapshots(around: anchorDate)
            return
        }
        yearSnapshotBuildTask?.cancel()
        yearSnapshotBuildTask = Task { @MainActor in
            let started = HCBPerformanceTelemetry.timestamp()
            let input = await model.calendarDisplayInputFromQuery(
                key: key,
                kind: .year,
                anchorDate: anchorDate,
                eventViewFilter: calendarEventViewFilter,
                searchQuery: searchQuery,
                referenceDate: Date(),
                calendar: calendar
            )
            let snapshot = await Task.detached(priority: .utility) {
                CalendarDisplaySnapshotBuilder.yearSnapshot(input)
            }.value
            guard Task.isCancelled == false, snapshot.key == yearSnapshotKey else { return }
            calendarStore.storeCalendarYearSnapshot(snapshot)
            preparedYearSnapshot = snapshot
            prewarmAdjacentYearSnapshots(around: anchorDate)
            HCBPerformanceTelemetry.debug(
                "calendar year snapshot built",
                metadata: [
                    "buildMs": HCBPerformanceTelemetry.elapsedMilliseconds(since: started),
                    "maxDayCount": "\(snapshot.maxCount)"
                ]
            )
        }
    }

    private func prewarmAdjacentYearSnapshots(around date: Date) {
        guard calendarStore.settings.performanceMode == .snappy else { return }
        let adjacentYears = [-1, 1].compactMap { calendar.date(byAdding: .year, value: $0, to: date) }
        guard adjacentYears.isEmpty == false else { return }
        yearSnapshotPrewarmTask?.cancel()
        yearSnapshotPrewarmTask = Task { @MainActor in
            for prewarmDate in adjacentYears {
                let key = yearSnapshotKey(for: prewarmDate)
                if calendarStore.cachedCalendarYearSnapshot(for: key) != nil { continue }
                let input = await model.calendarDisplayInputFromQuery(
                    key: key,
                    kind: .year,
                    anchorDate: prewarmDate,
                    eventViewFilter: calendarEventViewFilter,
                    searchQuery: searchQuery,
                    referenceDate: Date(),
                    calendar: calendar
                )
                let snapshot = await Task.detached(priority: .utility) {
                    CalendarDisplaySnapshotBuilder.yearSnapshot(input)
                }.value
                guard Task.isCancelled == false, snapshot.key == key else { return }
                calendarStore.storeCalendarYearSnapshot(snapshot)
            }
        }
    }

    private var weekdayHeaders: [String] {
        let symbols = calendar.veryShortStandaloneWeekdaySymbols
        let first = calendar.firstWeekday - 1
        return (0..<7).map { symbols[(first + $0) % 7] }
    }
}
