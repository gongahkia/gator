import SwiftUI

// "+N more" affordance + the popover that opens when tapped. Mirrors Google
// Calendar's per-day agenda popover: header with weekday + day number, then
// a scrollable list of every event and task on that day. Tapping a row
// routes to the existing edit sheet via router.present(...).
//
// Hosted in a child View struct (not as a function on MonthGridView) so the
// popover state can live as @State, and so each day's "+N more" gets its
// own independent isPresented flag.
struct MonthMoreButton: View {
    let count: Int
    let day: Date
    let events: [CalendarEventMirror]   // full day's events (band + timed)
    let tasks: [TaskMirror]              // full day's tasks
    let calendarColor: (CalendarEventMirror) -> Color
    @State private var isPresented = false

    var body: some View {
        Button {
            isPresented = true
        } label: {
            Text("+\(count) more")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
                .hcbScaledPadding(.leading, 6)
                .frame(maxWidth: .infinity, minHeight: 18, alignment: .leading)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help("Show all events and tasks for this day")
        .popover(isPresented: $isPresented, arrowEdge: .top) {
            DayAgendaPopover(
                day: day,
                events: events,
                tasks: tasks,
                calendarColor: calendarColor
            )
        }
    }
}

struct DayAgendaPopover: View {
    @Environment(\.routerPath) private var router
    @Environment(\.dismiss) private var dismiss
    let day: Date
    let events: [CalendarEventMirror]
    let tasks: [TaskMirror]
    let calendarColor: (CalendarEventMirror) -> Color

    private let calendar = Calendar.current
    private let rowControlColumnWidth: CGFloat = 18
    private let rowControlSize: CGFloat = 12
    private let rowSpacing: CGFloat = 8

    var body: some View {
        VStack(spacing: 0) {
            header
            Divider()
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 4) {
                    if sortedEvents.isEmpty && tasks.isEmpty {
                        Text("No events or tasks")
                            .hcbFont(.caption)
                            .foregroundStyle(.secondary)
                            .frame(maxWidth: .infinity, alignment: .center)
                            .padding(.vertical, 24)
                    } else {
                        ForEach(sortedEvents, id: \.id) { event in
                            eventRow(event)
                        }
                        ForEach(tasks) { task in
                            taskRow(task)
                        }
                    }
                }
                .hcbScaledPadding(12)
            }
        }
        .frame(width: 300, height: estimatedHeight)
    }

    // MARK: - Header

    private var header: some View {
        HStack(alignment: .center, spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(weekdayLabel)
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                Text("\(calendar.component(.day, from: day))")
                    .hcbFontSystem(size: 22, weight: .semibold)
                    .foregroundStyle(calendar.isDateInToday(day) ? AppColor.ember : AppColor.ink)
            }
            Spacer()
            Button {
                dismiss()
            } label: {
                Image(systemName: "xmark")
                    .hcbFont(.body)
                    .foregroundStyle(.secondary)
            }
            .buttonStyle(.plain)
            .help("Close")
        }
        .hcbScaledPadding(12)
    }

    // MARK: - Rows

    @ViewBuilder
    private func eventRow(_ event: CalendarEventMirror) -> some View {
        HStack(alignment: .top, spacing: rowSpacing) {
            ZStack {
                CalendarEventDismissButton(event: event, size: rowControlSize)
            }
            .frame(width: rowControlColumnWidth)
            .padding(.top, 3)

            Button {
                isPresented_dismiss()
                router?.present(.editEvent(event.id))
            } label: {
                HStack(alignment: .top, spacing: 8) {
                    Circle()
                        .fill(calendarColor(event))
                        .frame(width: 8, height: 8)
                        .padding(.top, 5)
                    if event.isAllDay {
                        Text(event.summary)
                            .hcbFont(.caption)
                            .foregroundStyle(AppColor.ink)
                            .lineLimit(2)
                            .multilineTextAlignment(.leading)
                    } else {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(timeLabel(event))
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                            Text(event.summary)
                                .hcbFont(.caption)
                                .foregroundStyle(AppColor.ink)
                                .lineLimit(2)
                                .multilineTextAlignment(.leading)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .contentShape(Rectangle())
        .padding(.vertical, 4)
    }

    @ViewBuilder
    private func taskRow(_ task: TaskMirror) -> some View {
        HStack(alignment: .top, spacing: rowSpacing) {
            ZStack {
                CalendarTaskCheckbox(task: task, size: rowControlSize)
            }
            .frame(width: rowControlColumnWidth)
            .padding(.top, 3)

            Button {
                isPresented_dismiss()
                router?.present(.editTask(task.id))
            } label: {
                Text(task.title)
                    .hcbFont(.caption)
                    .foregroundStyle(AppColor.ink)
                    .strikethrough(task.isCompleted, color: .secondary)
                    .lineLimit(2)
                    .multilineTextAlignment(.leading)
            }
            .buttonStyle(.plain)
            .frame(maxWidth: .infinity, alignment: .leading)

            Spacer(minLength: 0)
        }
        .contentShape(Rectangle())
        .padding(.vertical, 4)
    }

    // MARK: - Helpers

    private var weekdayLabel: String {
        let formatter = DateFormatter()
        formatter.dateFormat = "EEE"
        return formatter.string(from: day).uppercased()
    }

    // All-day events first (alphabetical-stable by start), then timed events
    // by start time. Matches the visual order users expect from Calendar UIs.
    private var sortedEvents: [CalendarEventMirror] {
        events.sorted { lhs, rhs in
            if lhs.isAllDay != rhs.isAllDay {
                return lhs.isAllDay && !rhs.isAllDay
            }
            return lhs.startDate < rhs.startDate
        }
    }

    private func timeLabel(_ event: CalendarEventMirror) -> String {
        event.startDate.formatted(.dateTime.hour().minute())
    }

    // Caps at 460 to avoid an absurdly tall popover; otherwise sizes to
    // content. 60pt for header + ~32pt per row.
    private var estimatedHeight: CGFloat {
        let rows = sortedEvents.count + tasks.count
        let raw = CGFloat(74 + rows * 36)
        return min(460, max(140, raw))
    }

    // SwiftUI's @Environment(\.dismiss) on a popover dismisses the popover
    // when called. Wrapping in a no-arg helper keeps the call site readable
    // and lets us add side-effects (e.g. ordering: dismiss, then present
    // the edit sheet) without scattering the dismiss() call across rows.
    private func isPresented_dismiss() {
        dismiss()
    }
}
