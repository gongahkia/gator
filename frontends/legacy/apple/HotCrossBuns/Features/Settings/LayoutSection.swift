import SwiftUI

struct LayoutSection: View {
    @Environment(AppModel.self) private var model
    @AppStorage(CalendarMonthScrollWindow.pastMonthsKey) private var monthScrollPastMonths = CalendarMonthScrollWindow.defaultPastMonths
    @AppStorage(CalendarMonthScrollWindow.futureMonthsKey) private var monthScrollFutureMonths = CalendarMonthScrollWindow.defaultFutureMonths

    var body: some View {
        Section("Layout") {
            navigationPlacementBlock
            Divider()
            sidebarTabsBlock
            Divider()
            calendarViewsBlock
            Divider()
            monthScrollBlock
            Divider()
            quickCreateBlock
            Divider()
            windowStateBlock
        }
    }

    private var windowStateBlock: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Windows")
                .hcbFont(.subheadline, weight: .medium)
            Toggle(isOn: restoreWindowStateBinding) {
                Label("Restore previous session", systemImage: "macwindow.on.rectangle")
            }
            Text("Reopens Help, History, Sync Issues, and Diagnostics after relaunch, and restores saved window positions.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var restoreWindowStateBinding: Binding<Bool> {
        Binding(
            get: { model.settings.restoreWindowStateEnabled },
            set: { newValue in
                var next = model.settings
                next.restoreWindowStateEnabled = newValue
                model.updateSettings(next)
                if newValue == false {
                    WindowRestorationStore.shared.clearOpenWindows()
                }
            }
        )
    }

    private var quickCreateBlock: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Quick create popover")
                .hcbFont(.subheadline, weight: .medium)
            Toggle(isOn: quickCreateExpandedBinding) {
                Label("Show all fields by default", systemImage: "rectangle.expand.vertical")
            }
            Text("Off: the popover starts compact and hides optional fields behind \"More\". On: the popover opens detailed — matches Apple Calendar's inline expansion.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var quickCreateExpandedBinding: Binding<Bool> {
        Binding(
            get: { model.settings.quickCreateExpandedByDefault },
            set: { newValue in
                var next = model.settings
                next.quickCreateExpandedByDefault = newValue
                model.updateSettings(next)
            }
        )
    }

    private var navigationPlacementBlock: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Navigation placement")
                .hcbFont(.subheadline, weight: .medium)
            Picker("Navigation placement", selection: navigationPlacementBinding) {
                ForEach(NavigationSurfacePlacement.allCases) { placement in
                    Label(placement.title, systemImage: placement.systemImage)
                        .tag(placement)
                }
            }
            .pickerStyle(.segmented)
            Text("Left keeps the native macOS sidebar. Right, top, and bottom move the same tabs around the current view.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var navigationPlacementBinding: Binding<NavigationSurfacePlacement> {
        Binding(
            get: { model.settings.sidebarPlacement },
            set: { model.setSidebarPlacement($0) }
        )
    }

    private var sidebarTabsBlock: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Navigation tabs")
                .hcbFont(.subheadline, weight: .medium)
            ForEach(SidebarItem.allCases.filter(\.isHideable)) { item in
                Toggle(isOn: sidebarItemVisibleBinding(item)) {
                    Label(item.title, systemImage: item.systemImage)
                }
                .disabled(isLastVisibleSidebarItem(item))
            }
            Text("Hidden tabs disappear from every navigation placement. Keyboard shortcuts for a hidden tab are ignored until it's re-enabled.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var calendarViewsBlock: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Calendar view modes")
                .hcbFont(.subheadline, weight: .medium)
            ForEach(CalendarGridMode.allCases, id: \.self) { mode in
                Toggle(isOn: calendarModeVisibleBinding(mode)) {
                    Label(mode.title, systemImage: mode.systemImage)
                }
                .disabled(isLastVisibleMode(mode))
            }
            Text("Hidden modes disappear from the Calendar view picker. At least one mode stays visible.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private var monthScrollBlock: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Month scroll range")
                .hcbFont(.subheadline, weight: .medium)
            monthScrollRangeRow(
                title: "Past months",
                systemImage: "arrow.up.to.line",
                value: monthScrollPastBinding,
                bounds: CalendarMonthScrollWindow.pastRange
            )
            monthScrollRangeRow(
                title: "Future months",
                systemImage: "arrow.down.to.line",
                value: monthScrollFutureBinding,
                bounds: CalendarMonthScrollWindow.futureRange
            )
            Text("Month view opens with this many months loaded around the selected month. Scrolling to either boundary loads one more month.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
    }

    private func monthScrollRangeRow(
        title: String,
        systemImage: String,
        value: Binding<Int>,
        bounds: ClosedRange<Int>
    ) -> some View {
        HStack(spacing: 8) {
            Label(title, systemImage: systemImage)
                .lineLimit(1)
                .frame(maxWidth: .infinity, alignment: .leading)
            TextField(
                "",
                value: value,
                format: .number.precision(.integerLength(1...2))
            )
            .textFieldStyle(.roundedBorder)
            .monospacedDigit()
            .multilineTextAlignment(.trailing)
            .frame(width: 62)
            .accessibilityLabel(title)
            Stepper(value: value, in: bounds) {
                EmptyView()
            }
                .labelsHidden()
        }
    }

    private func sidebarItemVisibleBinding(_ item: SidebarItem) -> Binding<Bool> {
        Binding(
            get: { model.settings.hiddenSidebarItems.contains(item.rawValue) == false },
            set: { isVisible in model.setSidebarItemHidden(item, hidden: isVisible == false) }
        )
    }

    private func isLastVisibleSidebarItem(_ item: SidebarItem) -> Bool {
        guard model.settings.hiddenSidebarItems.contains(item.rawValue) == false else { return false }
        let visibleCount = SidebarItem.allCases.reduce(0) { acc, sidebarItem in
            acc + (model.settings.hiddenSidebarItems.contains(sidebarItem.rawValue) ? 0 : 1)
        }
        return visibleCount <= 1
    }

    private func calendarModeVisibleBinding(_ mode: CalendarGridMode) -> Binding<Bool> {
        Binding(
            get: { model.settings.hiddenCalendarViewModes.contains(mode.rawValue) == false },
            set: { isVisible in model.setCalendarViewModeHidden(mode, hidden: isVisible == false) }
        )
    }

    // Disables the toggle for the only remaining visible mode, so the UI
    // matches the setter's guard (which also refuses to hide the last mode).
    private func isLastVisibleMode(_ mode: CalendarGridMode) -> Bool {
        guard model.settings.hiddenCalendarViewModes.contains(mode.rawValue) == false else { return false }
        let visibleCount = CalendarGridMode.allCases.reduce(0) { acc, m in
            acc + (model.settings.hiddenCalendarViewModes.contains(m.rawValue) ? 0 : 1)
        }
        return visibleCount <= 1
    }

    private var monthScrollPastBinding: Binding<Int> {
        Binding(
            get: { CalendarMonthScrollWindow.clampedPast(monthScrollPastMonths) },
            set: { monthScrollPastMonths = CalendarMonthScrollWindow.clampedPast($0) }
        )
    }

    private var monthScrollFutureBinding: Binding<Int> {
        Binding(
            get: { CalendarMonthScrollWindow.clampedFuture(monthScrollFutureMonths) },
            set: { monthScrollFutureMonths = CalendarMonthScrollWindow.clampedFuture($0) }
        )
    }

}
