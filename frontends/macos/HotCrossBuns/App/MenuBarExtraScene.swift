import AppKit
import SwiftUI

extension View {
    func hcbMenuBarStatusController(_ controller: HCBMenuBarStatusController, model: AppModel) -> some View {
        background(HCBMenuBarStatusControllerHost(controller: controller, model: model))
    }
}

private struct HCBMenuBarStatusControllerHost: View {
    @Environment(\.openWindow) private var openWindow

    let controller: HCBMenuBarStatusController
    let model: AppModel

    var body: some View {
        Color.clear
            .frame(width: 0, height: 0)
            .accessibilityHidden(true)
            .onAppear {
                HCBMainWindowPresenter.shared.configure(openWindow: openWindow)
                controller.configure(model: model)
            }
            .onChange(of: model.menuBarStore.projection.settings.showMenuBarExtra) { _, _ in
                controller.configure(model: model)
            }
            .onChange(of: model.menuBarStore.projection.settings.menuBarStyle) { _, _ in
                controller.configure(model: model)
            }
            .onChange(of: model.menuBarStore.projection.settings.menuBarAdaptiveStatusSource) { _, _ in
                controller.refreshStatusItemImage()
                controller.refreshContent()
            }
            .onChange(of: model.menuBarStore.projection.settings.menuBarAdaptiveEmptyBehavior) { _, _ in
                controller.refreshStatusItemImage()
            }
            .onChange(of: model.menuBarStore.projection.settings.menuBarAdaptivePanelContent) { _, _ in
                controller.refreshContent()
            }
            .onChange(of: model.menuBarStore.projection.settings.showMenuBarBadge) { _, _ in
                controller.refreshStatusItemImage()
            }
            .onChange(of: model.menuBarStore.projection.settings.menuBarIcon) { _, _ in
                controller.refreshStatusItemImage()
            }
            .onChange(of: model.menuBarStore.projection.settings.colorSchemeID) { _, _ in
                controller.configure(model: model)
            }
            .onChange(of: model.menuBarStore.projection.settings.appLanguage) { _, _ in
                controller.refreshContent()
            }
            .onChange(of: model.menuBarStore.projection.activeAccountID) { _, _ in
                controller.refreshContent()
            }
            .onChange(of: model.menuBarStore.projection.connectedAccounts.map(\.id)) { _, _ in
                controller.refreshContent()
            }
            .onChange(of: model.menuBarStore.projection.overdueCount) { _, _ in
                controller.refreshStatusItemImage()
            }
            .onChange(of: model.menuBarStore.projection.displayRevision) { _, _ in
                controller.refreshContent()
            }
    }
}

@MainActor
final class HCBMenuBarStatusController: NSObject, NSWindowDelegate, NSMenuDelegate {
    private weak var model: AppModel?
    private var statusItem: NSStatusItem?
    private var panel: HCBMenuBarPanelWindow?
    private var detailPanel: HCBMenuBarAttachedPanelWindow?
    private var hostingController: NSHostingController<AnyView>?
    private var detailHostingController: NSHostingController<AnyView>?
    private var selectedWeekDetailDay: Date?
    private var isPinned = false
    private var adaptiveStatusTimer: Timer?

    func configure(model: AppModel) {
        self.model = model
        if model.menuBarStore.projection.settings.showMenuBarExtra {
            installStatusItemIfNeeded()
            refreshContent()
            updateAdaptiveStatusTimer()
        } else {
            uninstallStatusItem()
        }
    }

    func refreshContent() {
        guard let model, statusItem != nil else { return }
        let projection = model.menuBarStore.projection
        let root = AnyView(
            MenuBarExtraContent { [weak self] day in
                self?.showWeekDayDetail(day)
            }
                .environment(model)
                .environment(\.locale, projection.settings.appLanguage.locale)
        )

        if let hostingController {
            hostingController.rootView = root
        } else {
            let hostingController = NSHostingController(rootView: root)
            hostingController.view.wantsLayer = true
            hostingController.view.layer?.backgroundColor = NSColor.clear.cgColor
            self.hostingController = hostingController
        }

        if projection.settings.menuBarStyle != .weekly {
            hideWeekDayDetail()
        }

        refreshStatusItemImage()

        if panel?.isVisible == true {
            resizePanelToFitContent()
            positionPanel()
            refreshWeekDayDetailIfNeeded()
        }
    }

    private func installStatusItemIfNeeded() {
        guard statusItem == nil else { return }
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        item.autosaveName = "HotCrossBunsStatusItem"
        item.button?.target = self
        item.button?.action = #selector(statusItemClicked(_:))
        item.button?.sendAction(on: [.leftMouseDown, .rightMouseDown])
        item.button?.imagePosition = .imageOnly
        item.button?.imageScaling = .scaleProportionallyDown
        statusItem = item
        refreshStatusItemImage()
    }

    private func uninstallStatusItem() {
        invalidateAdaptiveStatusTimer()
        hidePanel()
        if let statusItem {
            NSStatusBar.system.removeStatusItem(statusItem)
        }
        statusItem = nil
        hostingController = nil
        detailHostingController = nil
        selectedWeekDetailDay = nil
    }

    func refreshStatusItemImage() {
        guard let model, let statusItem, let button = statusItem.button else { return }
        let projection = model.menuBarStore.projection
        let count = projection.settings.showMenuBarBadge ? projection.overdueCount : 0
        button.image = statusImage(badgeCount: count)
        button.imageScaling = .scaleProportionallyDown
        button.font = NSFont.menuBarFont(ofSize: 0)

        if projection.settings.menuBarStyle == .adaptive {
            let adaptiveStatus = projection.adaptiveStatus()
            if let label = adaptiveStatus.label {
                statusItem.length = NSStatusItem.variableLength
                button.imagePosition = .imageLeft
                // Keep icon and status text separated without rendering the old vertical divider.
                button.title = " \(label)"
                button.setAccessibilityLabel(accessibilityLabel(base: adaptiveStatus.accessibilityLabel, overdueCount: count))
                return
            }
        }

        statusItem.length = NSStatusItem.squareLength
        button.imagePosition = .imageOnly
        button.title = ""
        button.setAccessibilityLabel(accessibilityLabel(base: "Hot Cross Buns", overdueCount: count))
    }

    private func accessibilityLabel(base: String, overdueCount count: Int) -> String {
        guard count > 0 else { return base }
        return "\(base), \(count) overdue task\(count == 1 ? "" : "s")"
    }

    private func statusImage(badgeCount: Int) -> NSImage? {
        let selectedIcon = model?.menuBarStore.projection.settings.menuBarIcon ?? .buns
        guard let image = selectedIcon.nsStatusImage()
            ?? NSImage(named: "MenuBarIcon")
            ?? NSImage(systemSymbolName: "calendar", accessibilityDescription: "Hot Cross Buns")
        else { return nil }

        guard badgeCount > 0 else {
            let copy = image.copy() as? NSImage ?? image
            copy.isTemplate = true
            copy.size = NSSize(width: 18, height: 18)
            return copy
        }

        return image.hcbMenuBarBadgedImage(count: badgeCount)
    }

    private func updateAdaptiveStatusTimer() {
        invalidateAdaptiveStatusTimer()
        guard let model,
              model.menuBarStore.projection.settings.showMenuBarExtra,
              model.menuBarStore.projection.settings.menuBarStyle == .adaptive,
              statusItem != nil
        else {
            return
        }

        let timer = Timer(timeInterval: 60, repeats: true) { [weak self] _ in
            Task { @MainActor in
                guard let self else { return }
                self.refreshStatusItemImage()
                if self.panel?.isVisible == true {
                    self.refreshContent()
                }
            }
        }
        RunLoop.main.add(timer, forMode: .common)
        adaptiveStatusTimer = timer
    }

    private func invalidateAdaptiveStatusTimer() {
        adaptiveStatusTimer?.invalidate()
        adaptiveStatusTimer = nil
    }

    @objc private func statusItemClicked(_ sender: Any?) {
        if NSApp.currentEvent?.type == .rightMouseDown {
            showContextMenu()
            return
        }

        if panel?.isVisible == true {
            hidePanel()
        } else {
            showPanel()
        }
    }

    private func showPanel() {
        guard statusItem?.button != nil else { return }
        refreshContent()
        guard let hostingView = hostingController?.view else { return }

        let panel = panel ?? HCBMenuBarPanelWindow()
        panel.delegate = self
        panel.setHostedView(hostingView)
        self.panel = panel

        resizePanelToFitContent()
        positionPanel()
        NSApp.unhideWithoutActivation()
        panel.makeKeyAndOrderFront(nil)
    }

    private func hidePanel() {
        hideWeekDayDetail()
        panel?.orderOut(nil)
    }

    private func resizePanelToFitContent() {
        guard let panel, let hostingView = hostingController?.view else { return }
        hostingView.layoutSubtreeIfNeeded()
        var fitting = hostingView.fittingSize
        if fitting.width <= 1 { fitting.width = 320 }
        if fitting.height <= 1 { fitting.height = 360 }

        let screen = statusItemScreen()
        let maxHeight = max(240, screen.visibleFrame.height - 48)
        let contentSize = NSSize(
            width: min(max(fitting.width, 300), 420),
            height: min(max(fitting.height, 80), maxHeight)
        )
        panel.setContentBodySize(contentSize)
    }

    private func positionPanel() {
        guard let panel, let button = statusItem?.button, let buttonWindow = button.window else { return }
        var statusFrame = buttonWindow.convertToScreen(button.frame)
        let screen = statusItemScreen()

        statusFrame.origin.y = min(statusFrame.origin.y, screen.frame.maxY)
        panel.position(relativeTo: statusFrame, in: screen)
        positionWeekDayDetail()
    }

    private func statusItemScreen() -> NSScreen {
        guard let button = statusItem?.button, let window = button.window else {
            return NSScreen.main ?? NSScreen.screens.first!
        }

        let frame = window.convertToScreen(button.frame)
        var testPoint = frame.origin
        testPoint.y -= 100
        return NSScreen.screens.first(where: { $0.frame.contains(testPoint) })
            ?? window.screen
            ?? NSScreen.main
            ?? NSScreen.screens.first
            ?? NSScreen.screens[0]
    }

    private func showContextMenu() {
        guard let button = statusItem?.button else { return }
        let menu = NSMenu()
        menu.delegate = self

        let openItem = menu.addItem(withTitle: "Open Hot Cross Buns", action: #selector(openMainWindow), keyEquivalent: "")
        openItem.target = self

        let settingsItem = menu.addItem(withTitle: "Settings…", action: #selector(openSettings), keyEquivalent: "")
        settingsItem.target = self

        let refreshItem = menu.addItem(withTitle: "Refresh", action: #selector(refresh), keyEquivalent: "")
        refreshItem.target = self
        refreshItem.isEnabled = model?.account != nil

        menu.addItem(.separator())

        let pinItem = menu.addItem(withTitle: "Keep Panel Open", action: #selector(togglePin), keyEquivalent: "")
        pinItem.target = self
        pinItem.state = isPinned ? .on : .off

        menu.addItem(.separator())

        let quitItem = menu.addItem(withTitle: "Quit Hot Cross Buns", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "")
        quitItem.target = NSApp

        if let event = NSApp.currentEvent {
            NSMenu.popUpContextMenu(menu, with: event, for: button)
        } else {
            menu.popUp(positioning: nil, at: NSPoint(x: 0, y: button.bounds.minY), in: button)
        }
    }

    @objc private func openMainWindow() {
        hidePanel()
        HCBMainWindowPresenter.shared.show()
    }

    @objc private func openSettings() {
        hidePanel()
        NSApp.activate(ignoringOtherApps: true)
        if NSApp.sendAction(Selector(("showSettingsWindow:")), to: nil, from: nil) == false {
            NSApp.sendAction(Selector(("showPreferencesWindow:")), to: nil, from: nil)
        }
    }

    @objc private func refresh() {
        guard let model else { return }
        Task { await model.refreshNow() }
    }

    @objc private func togglePin() {
        isPinned.toggle()
    }

    private func showWeekDayDetail(_ day: Date) {
        guard model != nil, panel?.isVisible == true else { return }
        selectedWeekDetailDay = Calendar.current.startOfDay(for: day)
        refreshWeekDayDetailIfNeeded()
        positionWeekDayDetail()
        detailPanel?.orderFront(nil)
    }

    private func refreshWeekDayDetailIfNeeded() {
        guard let model, let selectedWeekDetailDay, panel?.isVisible == true else { return }
        let projection = model.menuBarStore.projection
        let root = AnyView(
            WeeklyMenuBarDayDetailPanel(day: selectedWeekDetailDay)
                .withHCBAppearance(projection.settings)
                .hcbPreferredColorScheme(projection.settings)
                .hcbSurface(.menuBar)
                .environment(model)
                .environment(\.locale, projection.settings.appLanguage.locale)
        )

        if let detailHostingController {
            detailHostingController.rootView = root
        } else {
            let detailHostingController = NSHostingController(rootView: root)
            detailHostingController.view.wantsLayer = true
            detailHostingController.view.layer?.backgroundColor = NSColor.clear.cgColor
            self.detailHostingController = detailHostingController
        }

        let detailPanel = detailPanel ?? HCBMenuBarAttachedPanelWindow()
        detailPanel.delegate = self
        guard let detailView = detailHostingController?.view else { return }
        detailPanel.setHostedView(detailView)
        self.detailPanel = detailPanel
        resizeWeekDayDetail()
    }

    private func resizeWeekDayDetail() {
        guard let detailPanel, let model, let selectedWeekDetailDay else { return }
        let projection = model.menuBarStore.projection
        let eventCount = projection.events(on: selectedWeekDetailDay).count
        let taskCount = projection.tasks(on: selectedWeekDetailDay).count
        let rowCount = eventCount + taskCount
        let screen = statusItemScreen()
        let maxHeight = max(220, screen.visibleFrame.height - 72)
        let preferredHeight = CGFloat(112 + min(rowCount, 8) * 44)
        detailPanel.setContentBodySize(NSSize(
            width: 300,
            height: min(max(preferredHeight, 160), min(maxHeight, 420))
        ))
    }

    private func positionWeekDayDetail() {
        guard let panel, let detailPanel else { return }
        let screen = statusItemScreen()
        detailPanel.position(attachedTo: panel.frame, in: screen)
    }

    private func hideWeekDayDetail() {
        detailPanel?.orderOut(nil)
        selectedWeekDetailDay = nil
    }

    func windowDidResignKey(_ notification: Notification) {
        let keyWindow = NSApp.keyWindow
        if keyWindow === panel || keyWindow === detailPanel { return }
        guard isPinned == false else { return }
        hidePanel()
    }

    func windowDidResize(_ notification: Notification) {
        guard notification.object as? NSWindow === panel else { return }
        positionPanel()
    }

    func menuDidClose(_ menu: NSMenu) {
        statusItem?.menu = nil
    }
}

private extension AppSettings.MenuBarIcon {
    func nsStatusImage() -> NSImage? {
        if let systemImageName {
            return NSImage(systemSymbolName: systemImageName, accessibilityDescription: title)
        }
        return NSImage(named: "MenuBarIcon")
    }
}

private extension NSImage {
    func hcbMenuBarBadgedImage(count: Int) -> NSImage {
        let displayCount = count > 99 ? "99+" : "\(count)"
        let canvasSize = NSSize(width: 24, height: 22)
        let iconRect = NSRect(x: 0, y: 1, width: 21, height: 21)
        let image = NSImage(size: canvasSize, flipped: false) { _ in
            if let cgImage = self.cgImage(forProposedRect: nil, context: nil, hints: nil),
               let context = NSGraphicsContext.current?.cgContext {
                context.saveGState()
                context.clip(to: iconRect, mask: cgImage)
                context.setAlpha(1)
                NSColor.labelColor.setFill()
                context.fill(iconRect)
                context.restoreGState()
            } else {
                self.draw(in: iconRect, from: .zero, operation: .sourceOver, fraction: 1)
            }

            let font = NSFont.monospacedDigitSystemFont(ofSize: displayCount.count > 2 ? 7 : 8, weight: .bold)
            let attributes: [NSAttributedString.Key: Any] = [
                .font: font,
                .foregroundColor: NSColor.controlBackgroundColor
            ]
            let textSize = displayCount.size(withAttributes: attributes)
            let badgeHeight: CGFloat = 11
            let badgeWidth = max(badgeHeight, ceil(textSize.width) + 6)
            let badgeRect = NSRect(
                x: canvasSize.width - badgeWidth,
                y: 0,
                width: badgeWidth,
                height: badgeHeight
            )
            NSColor.controlBackgroundColor.setFill()
            NSBezierPath(roundedRect: badgeRect, xRadius: badgeHeight / 2, yRadius: badgeHeight / 2).fill()
            NSColor.labelColor.setFill()
            NSBezierPath(roundedRect: badgeRect, xRadius: badgeHeight / 2, yRadius: badgeHeight / 2).fill()

            let textRect = NSRect(
                x: badgeRect.midX - textSize.width / 2,
                y: badgeRect.midY - textSize.height / 2,
                width: textSize.width,
                height: textSize.height
            )
            displayCount.draw(in: textRect, withAttributes: attributes)
            return true
        }
        image.isTemplate = false
        image.size = canvasSize
        return image
    }
}

private final class HCBMenuBarPanelWindow: NSPanel {
    private let frameView = HCBMenuBarPanelFrameView()

    init() {
        super.init(
            contentRect: NSRect(x: 0, y: 0, width: 320, height: 420),
            styleMask: [.nonactivatingPanel],
            backing: .buffered,
            defer: false
        )
        backgroundColor = .clear
        isOpaque = false
        hasShadow = true
        level = .mainMenu
        collectionBehavior = [.moveToActiveSpace, .transient]
        animationBehavior = .utilityWindow
        isMovableByWindowBackground = false
        super.contentView = frameView
    }

    override var canBecomeMain: Bool { false }
    override var canBecomeKey: Bool { true }

    func setHostedView(_ hostedView: NSView) {
        frameView.setHostedView(hostedView)
    }

    func setContentBodySize(_ contentSize: NSSize) {
        let frameSize = frameView.outerSize(for: contentSize)
        setFrame(NSRect(origin: frame.origin, size: frameSize), display: true)
    }

    func position(relativeTo statusFrame: NSRect, in screen: NSScreen) {
        let screenMaxX = screen.frame.maxX
        let margin: CGFloat = 10
        var x = round(statusFrame.midX - frame.width / 2)
        let y = statusFrame.minY - 2
        if x + frame.width + margin > screenMaxX {
            x = screenMaxX - frame.width - margin
        }
        x = max(screen.frame.minX + margin, x)

        setFrameTopLeftPoint(NSPoint(x: x, y: y))
        frameView.arrowMidX = statusFrame.midX - frame.minX
        frameView.needsDisplay = true
        invalidateShadow()
    }

    override func cancelOperation(_ sender: Any?) {
        orderOut(nil)
    }
}

private final class HCBMenuBarPanelFrameView: NSView {
    var arrowMidX: CGFloat = 0

    private let arrowHeight: CGFloat = 8
    private let cornerRadius: CGFloat = 12
    private let borderWidth: CGFloat = 1
    private let sideInset: CGFloat = 1
    private let bottomInset: CGFloat = 1
    private var topInset: CGFloat { arrowHeight + borderWidth + 3 }
    private weak var hostedView: NSView?

    func outerSize(for contentSize: NSSize) -> NSSize {
        NSSize(
            width: contentSize.width + sideInset * 2,
            height: contentSize.height + topInset + bottomInset
        )
    }

    func setHostedView(_ view: NSView) {
        guard hostedView !== view else { return }
        hostedView?.removeFromSuperview()
        hostedView = view
        view.translatesAutoresizingMaskIntoConstraints = false
        addSubview(view)
        NSLayoutConstraint.activate([
            view.leadingAnchor.constraint(equalTo: leadingAnchor, constant: sideInset),
            view.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -sideInset),
            view.topAnchor.constraint(equalTo: topAnchor, constant: topInset),
            view.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -bottomInset)
        ])
    }

    override func draw(_ dirtyRect: NSRect) {
        NSColor.clear.setFill()
        dirtyRect.fill()

        let bodyRect = NSRect(
            x: borderWidth,
            y: borderWidth,
            width: bounds.width - borderWidth * 2,
            height: bounds.height - arrowHeight - borderWidth * 2
        )

        let path = NSBezierPath(roundedRect: bodyRect, xRadius: cornerRadius, yRadius: cornerRadius)
        let clampedArrowMidX = min(max(arrowMidX, bodyRect.minX + cornerRadius + arrowHeight), bodyRect.maxX - cornerRadius - arrowHeight)
        let arrowBaseY = bodyRect.maxY - 1
        let arrow = NSBezierPath()
        arrow.move(to: NSPoint(x: clampedArrowMidX - arrowHeight, y: arrowBaseY))
        arrow.curve(
            to: NSPoint(x: clampedArrowMidX, y: arrowBaseY + arrowHeight),
            controlPoint1: NSPoint(x: clampedArrowMidX - 4, y: arrowBaseY),
            controlPoint2: NSPoint(x: clampedArrowMidX - 4, y: arrowBaseY + arrowHeight)
        )
        arrow.curve(
            to: NSPoint(x: clampedArrowMidX + arrowHeight, y: arrowBaseY),
            controlPoint1: NSPoint(x: clampedArrowMidX + 4, y: arrowBaseY + arrowHeight),
            controlPoint2: NSPoint(x: clampedArrowMidX + 4, y: arrowBaseY)
        )
        path.append(arrow)

        NSColor.controlBackgroundColor.setFill()
        path.fill()
        NSColor.separatorColor.withAlphaComponent(0.45).setStroke()
        path.lineWidth = borderWidth
        path.stroke()
    }
}

private final class HCBMenuBarAttachedPanelWindow: NSPanel {
    private let frameView = HCBMenuBarAttachedPanelFrameView()

    init() {
        super.init(
            contentRect: NSRect(x: 0, y: 0, width: 300, height: 320),
            styleMask: [.nonactivatingPanel],
            backing: .buffered,
            defer: false
        )
        backgroundColor = .clear
        isOpaque = false
        hasShadow = true
        level = .mainMenu
        collectionBehavior = [.moveToActiveSpace, .transient]
        animationBehavior = .utilityWindow
        isMovableByWindowBackground = false
        super.contentView = frameView
    }

    override var canBecomeMain: Bool { false }
    override var canBecomeKey: Bool { true }

    func setHostedView(_ hostedView: NSView) {
        frameView.setHostedView(hostedView)
    }

    func setContentBodySize(_ contentSize: NSSize) {
        let frameSize = frameView.outerSize(for: contentSize)
        setFrame(NSRect(origin: frame.origin, size: frameSize), display: true)
    }

    func position(attachedTo mainFrame: NSRect, in screen: NSScreen) {
        let visibleFrame = screen.visibleFrame
        let gap: CGFloat = 8
        let margin: CGFloat = 8

        let fitsRight = mainFrame.maxX + gap + frame.width <= visibleFrame.maxX - margin
        let proposedX = fitsRight
            ? mainFrame.maxX + gap
            : mainFrame.minX - gap - frame.width
        let x = min(max(proposedX, visibleFrame.minX + margin), visibleFrame.maxX - frame.width - margin)

        let proposedTopY = mainFrame.maxY
        let minTopY = visibleFrame.minY + frame.height + margin
        let maxTopY = visibleFrame.maxY - margin
        let y = min(max(proposedTopY, minTopY), maxTopY)

        setFrameTopLeftPoint(NSPoint(x: round(x), y: round(y)))
        invalidateShadow()
    }

    override func cancelOperation(_ sender: Any?) {
        orderOut(nil)
    }
}

private final class HCBMenuBarAttachedPanelFrameView: NSView {
    private let cornerRadius: CGFloat = 12
    private let borderWidth: CGFloat = 1
    private let inset: CGFloat = 1
    private weak var hostedView: NSView?

    func outerSize(for contentSize: NSSize) -> NSSize {
        NSSize(width: contentSize.width + inset * 2, height: contentSize.height + inset * 2)
    }

    func setHostedView(_ view: NSView) {
        guard hostedView !== view else { return }
        hostedView?.removeFromSuperview()
        hostedView = view
        view.translatesAutoresizingMaskIntoConstraints = false
        addSubview(view)
        NSLayoutConstraint.activate([
            view.leadingAnchor.constraint(equalTo: leadingAnchor, constant: inset),
            view.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -inset),
            view.topAnchor.constraint(equalTo: topAnchor, constant: inset),
            view.bottomAnchor.constraint(equalTo: bottomAnchor, constant: -inset)
        ])
    }

    override func draw(_ dirtyRect: NSRect) {
        NSColor.clear.setFill()
        dirtyRect.fill()

        let bodyRect = bounds.insetBy(dx: borderWidth, dy: borderWidth)
        let path = NSBezierPath(roundedRect: bodyRect, xRadius: cornerRadius, yRadius: cornerRadius)
        NSColor.controlBackgroundColor.setFill()
        path.fill()
        NSColor.separatorColor.withAlphaComponent(0.45).setStroke()
        path.lineWidth = borderWidth
        path.stroke()
    }
}

struct MenuBarExtraContent: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    let onWeekDaySelected: (Date) -> Void

    var body: some View {
        Group {
            switch projection.settings.menuBarStyle {
            case .detailed: DetailedMenuBarPanel()
            case .weekly: WeeklyMenuBarPanel(onDaySelected: onWeekDaySelected)
            case .adaptive: AdaptiveMenuBarPanel()
            case .compact: CompactMenuBarPanel()
            }
        }
        .id(projection.settings.colorSchemeID)
        .environment(\.locale, projection.settings.appLanguage.locale)
        .withHCBAppearance(projection.settings)
        .hcbPreferredColorScheme(projection.settings)
        .hcbSurface(.menuBar) // §6.11 per-surface font override
        .onChange(of: projection.settings.colorSchemeID, initial: true) { _, newID in
            HCBColorSchemeStore.current = HCBColorScheme.scheme(id: newID, customSchemes: projection.settings.customColorSchemes) ?? .notion
        }
    }
}

struct MenuBarAdaptiveStatus: Equatable, Sendable {
    enum Kind: Equatable, Sendable {
        case currentEvent(CalendarEventMirror.ID)
        case nextEvent(CalendarEventMirror.ID)
        case task(TaskMirror.ID)
        case clear
        case iconOnly
    }

    var label: String?
    var accessibilityLabel: String
    var kind: Kind
    var sortDate: Date
}

enum MenuBarAdaptiveStatusResolver {
    static func status(
        now: Date,
        events: [CalendarEventMirror],
        tasks: [TaskMirror],
        source: AppSettings.MenuBarAdaptiveStatusSource,
        emptyBehavior: AppSettings.MenuBarAdaptiveEmptyBehavior,
        calendar: Calendar = .current
    ) -> MenuBarAdaptiveStatus {
        if let primary = bestStatus(now: now, events: events, tasks: tasks, source: source, calendar: calendar) {
            return primary
        }

        if emptyBehavior == .nextCommitment,
           source != .eventsAndTasks,
           let fallback = bestStatus(now: now, events: events, tasks: tasks, source: .eventsAndTasks, calendar: calendar)
        {
            return fallback
        }

        switch emptyBehavior {
        case .iconOnly, .nextCommitment:
            return MenuBarAdaptiveStatus(
                label: nil,
                accessibilityLabel: "Hot Cross Buns",
                kind: .iconOnly,
                sortDate: .distantFuture
            )
        case .clear:
            return MenuBarAdaptiveStatus(
                label: "Clear",
                accessibilityLabel: "Hot Cross Buns, Clear",
                kind: .clear,
                sortDate: .distantFuture
            )
        }
    }

    static func durationText(from start: Date, to end: Date) -> String {
        let seconds = max(0, end.timeIntervalSince(start))
        let minutes = max(1, Int(ceil(seconds / 60)))
        if minutes < 60 {
            return "\(minutes)m"
        }

        let hours = minutes / 60
        let remainingMinutes = minutes % 60
        if hours < 24 {
            return remainingMinutes == 0 ? "\(hours)h" : "\(hours)h \(remainingMinutes)m"
        }

        let days = hours / 24
        let remainingHours = hours % 24
        return remainingHours == 0 ? "\(days)d" : "\(days)d \(remainingHours)h"
    }

    private static func bestStatus(
        now: Date,
        events: [CalendarEventMirror],
        tasks: [TaskMirror],
        source: AppSettings.MenuBarAdaptiveStatusSource,
        calendar: Calendar
    ) -> MenuBarAdaptiveStatus? {
        let eventStatus: MenuBarAdaptiveStatus?
        switch source {
        case .events, .eventsAndTasks:
            eventStatus = bestEventStatus(now: now, events: events)
        case .tasks:
            eventStatus = nil
        }

        let taskStatus: MenuBarAdaptiveStatus?
        switch source {
        case .tasks, .eventsAndTasks:
            taskStatus = bestTaskStatus(now: now, tasks: tasks, calendar: calendar)
        case .events:
            taskStatus = nil
        }

        if let eventStatus, case .currentEvent = eventStatus.kind {
            return eventStatus
        }

        switch (eventStatus, taskStatus) {
        case let (event?, task?):
            return statusSortDate(event) <= statusSortDate(task) ? event : task
        case let (event?, nil):
            return event
        case let (nil, task?):
            return task
        case (nil, nil):
            return nil
        }
    }

    private static func bestEventStatus(now: Date, events: [CalendarEventMirror]) -> MenuBarAdaptiveStatus? {
        var currentEvent: CalendarEventMirror?
        var nextEvent: CalendarEventMirror?

        for event in events where event.status != .cancelled && event.isAllDay == false && event.endDate > now {
            if event.startDate <= now {
                if let existing = currentEvent {
                    if eventSortsBeforeCurrentStatus(event, existing) {
                        currentEvent = event
                    }
                } else {
                    currentEvent = event
                }
            } else if let existing = nextEvent {
                if eventSortsBeforeNextStatus(event, existing) {
                    nextEvent = event
                }
            } else {
                nextEvent = event
            }
        }

        if let current = currentEvent {
            let label = statusLabel(
                title: current.summary,
                detail: "\(durationText(from: now, to: current.endDate)) left"
            )
            return MenuBarAdaptiveStatus(
                label: label,
                accessibilityLabel: "Hot Cross Buns, \(label)",
                kind: .currentEvent(current.id),
                sortDate: .distantPast
            )
        }

        guard let next = nextEvent else { return nil }
        let label = statusLabel(
            title: next.summary,
            detail: "in \(durationText(from: now, to: next.startDate))"
        )
        return MenuBarAdaptiveStatus(
            label: label,
            accessibilityLabel: "Hot Cross Buns, \(label)",
            kind: .nextEvent(next.id),
            sortDate: next.startDate
        )
    }

    private static func bestTaskStatus(now: Date, tasks: [TaskMirror], calendar: Calendar) -> MenuBarAdaptiveStatus? {
        var bestTask: TaskMirror?
        var bestDueDate: Date?

        for task in tasks where task.isCompleted == false && task.isDeleted == false && task.isHidden == false {
            guard let dueDate = task.dueDate else { continue }
            if let existingDueDate = bestDueDate, let existing = bestTask {
                if dueDate < existingDueDate
                    || (dueDate == existingDueDate && task.title.localizedCaseInsensitiveCompare(existing.title) == .orderedAscending) {
                    bestTask = task
                    bestDueDate = dueDate
                }
            } else {
                bestTask = task
                bestDueDate = dueDate
            }
        }

        guard let task = bestTask, let dueDate = bestDueDate else {
            return nil
        }

        let label = statusLabel(
            title: task.title,
            detail: taskDueText(now: now, dueDate: dueDate, calendar: calendar)
        )
        return MenuBarAdaptiveStatus(
            label: label,
            accessibilityLabel: "Hot Cross Buns, \(label)",
            kind: .task(task.id),
            sortDate: dueDate
        )
    }

    private static func eventSortsBeforeCurrentStatus(_ lhs: CalendarEventMirror, _ rhs: CalendarEventMirror) -> Bool {
        if lhs.endDate == rhs.endDate {
            return lhs.summary.localizedCaseInsensitiveCompare(rhs.summary) == .orderedAscending
        }
        return lhs.endDate < rhs.endDate
    }

    private static func eventSortsBeforeNextStatus(_ lhs: CalendarEventMirror, _ rhs: CalendarEventMirror) -> Bool {
        if lhs.startDate == rhs.startDate {
            return lhs.summary.localizedCaseInsensitiveCompare(rhs.summary) == .orderedAscending
        }
        return lhs.startDate < rhs.startDate
    }

    private static func taskDueText(now: Date, dueDate: Date, calendar: Calendar) -> String {
        let today = calendar.startOfDay(for: now)
        let dueDay = calendar.startOfDay(for: dueDate)
        let dayDelta = calendar.dateComponents([.day], from: today, to: dueDay).day ?? 0

        if dayDelta < 0 {
            return "overdue"
        }
        if dayDelta == 0 {
            return "due today"
        }
        if dayDelta == 1 {
            return "due tomorrow"
        }
        return "due in \(dayDelta)d"
    }

    private static func statusSortDate(_ status: MenuBarAdaptiveStatus) -> Date {
        status.sortDate
    }

    private static func statusLabel(title rawTitle: String, detail: String) -> String {
        let title = shortTitle(rawTitle)
        // Parenthesize timing only when the title is truncated, matching the requested compact menu-bar format.
        if title.isTruncated {
            return "\(title.text) (\(detail))"
        }
        return "\(title.text) - \(detail)"
    }

    private struct ShortTitle {
        var text: String
        var isTruncated: Bool
    }

    private static func shortTitle(_ raw: String, maxCharacters: Int = 28) -> ShortTitle {
        let trimmed = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        let title = trimmed.isEmpty ? "Untitled" : trimmed
        guard title.count > maxCharacters else {
            return ShortTitle(text: title, isTruncated: false)
        }
        let prefixCount = max(1, maxCharacters - 3)
        return ShortTitle(text: String(title.prefix(prefixCount)) + "...", isTruncated: true)
    }
}

private struct AdaptiveMenuBarPanel: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    @State private var completingTaskIDs: Set<TaskMirror.ID> = []
    @State private var snoozeCustomTask: TaskMirror?
    @State private var pendingDeleteEvent: CalendarEventMirror?

    private enum AgendaItem: Identifiable {
        case event(CalendarEventMirror)
        case task(TaskMirror)

        var id: String {
            switch self {
            case .event(let event): "event-\(event.id)"
            case .task(let task): "task-\(task.id)"
            }
        }

        var sortDate: Date {
            switch self {
            case .event(let event): event.isAllDay ? .distantPast : event.startDate
            case .task(let task): task.dueDate ?? .distantFuture
            }
        }

        var isAllDayEvent: Bool {
            switch self {
            case .event(let event): event.isAllDay
            case .task: false
            }
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            if let currentEvent {
                currentEventSection(currentEvent)
            }
            agendaSection(title: "Today", day: Date())
            agendaSection(title: "Tomorrow", day: tomorrow)
            if todayItems.isEmpty && tomorrowItems.isEmpty && currentEvent == nil {
                emptyAgenda
            }
            Divider()
            MenuBarAccountSwitcher()
            MenuBarQuickActions()
        }
        .hcbScaledPadding(14)
        .hcbScaledFrame(width: 340)
        .sheet(item: $snoozeCustomTask) { task in
            SnoozePickerSheet(task: task) { newDate in
                Task { await snooze(task, to: newDate) }
            }
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
    }

    private var header: some View {
        HStack(alignment: .firstTextBaseline) {
            Text("Agenda")
                .hcbFont(.headline)
            Spacer()
            Text(projection.syncState.title)
                .hcbFont(.caption, weight: .medium)
                .foregroundStyle(.secondary)
        }
    }

    private func currentEventSection(_ event: CalendarEventMirror) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline) {
                Text("Now")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                Spacer()
                Text("\(MenuBarAdaptiveStatusResolver.durationText(from: Date(), to: event.endDate)) left")
                    .font(.caption.monospacedDigit())
                    .foregroundStyle(.secondary)
            }
            eventRow(event, isCurrent: true)
        }
    }

    @ViewBuilder
    private func agendaSection(title: String, day: Date) -> some View {
        let items = agendaItems(on: day)
        if items.isEmpty == false {
            VStack(alignment: .leading, spacing: 6) {
                HStack(alignment: .firstTextBaseline) {
                    Text(title)
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                    Spacer()
                    Text("\(items.count)")
                        .font(.caption2.monospacedDigit().weight(.semibold))
                        .foregroundStyle(.tertiary)
                }
                VStack(alignment: .leading, spacing: 3) {
                    ForEach(items) { item in
                        switch item {
                        case .event(let event):
                            eventRow(event)
                        case .task(let task):
                            taskRow(task)
                        }
                    }
                }
            }
        }
    }

    private var emptyAgenda: some View {
        Text("No visible agenda items.")
            .hcbFont(.footnote)
            .foregroundStyle(.secondary)
            .frame(maxWidth: .infinity, alignment: .leading)
            .hcbScaledPadding(.vertical, 8)
    }

    private func eventRow(_ event: CalendarEventMirror, isCurrent: Bool = false) -> some View {
        HStack(alignment: .center, spacing: 8) {
            Image(systemName: isCurrent ? "record.circle.fill" : "calendar")
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(isCurrent ? AppColor.ember : .secondary)
                .hcbScaledFrame(width: 16)
            VStack(alignment: .leading, spacing: 1) {
                Text(event.summary.isEmpty ? "Untitled event" : event.summary)
                    .hcbFont(.subheadline, weight: isCurrent ? .semibold : .regular)
                    .lineLimit(1)
                Text(eventSubtitle(for: event))
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
            if let url = meetURL(for: event) {
                Button {
                    NSWorkspace.shared.open(url)
                } label: {
                    Label("Join meeting", systemImage: "video.fill")
                        .labelStyle(.titleAndIcon)
                }
                .buttonStyle(.borderless)
                .help("Join meeting")
            }
        }
        .hcbScaledPadding(.horizontal, isCurrent ? 7 : 4)
        .hcbScaledPadding(.vertical, isCurrent ? 6 : 4)
        .background {
            if isCurrent {
                RoundedRectangle(cornerRadius: 7)
                    .fill(Color.secondary.opacity(0.10))
            }
        }
        .contentShape(Rectangle())
        .contextMenu {
            EventContextMenu(
                event: event,
                onOpen: { open(event) },
                onDelete: { pendingDeleteEvent = event }
            )
        }
    }

    private func taskRow(_ task: TaskMirror) -> some View {
        HStack(alignment: .center, spacing: 8) {
            Image(systemName: "checkmark.circle")
                .hcbFont(.caption, weight: .semibold)
                .foregroundStyle(.secondary)
                .hcbScaledFrame(width: 16)
            VStack(alignment: .leading, spacing: 1) {
                Text(task.title.isEmpty ? "Untitled task" : task.title)
                    .hcbFont(.subheadline)
                    .lineLimit(1)
                Text(taskSubtitle(for: task))
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
            Button {
                complete(task)
            } label: {
                if completingTaskIDs.contains(task.id) {
                    ProgressView()
                        .controlSize(.small)
                } else {
                    Image(systemName: "checkmark")
                }
            }
            .buttonStyle(.borderless)
            .disabled(completingTaskIDs.contains(task.id))
            .help("Mark complete")
        }
        .hcbScaledPadding(.horizontal, 4)
        .hcbScaledPadding(.vertical, 4)
        .contentShape(Rectangle())
        .contextMenu {
            TaskContextMenu(
                task: task,
                onOpen: { open(task) },
                onCustomSnooze: { snoozeCustomTask = task },
                onDelete: {
                    Task { _ = await model.deleteTask(task) }
                }
            )
        }
    }

    private var includesEvents: Bool {
        switch projection.settings.menuBarAdaptivePanelContent {
        case .events, .eventsAndTasks: true
        case .tasks: false
        }
    }

    private var includesTasks: Bool {
        switch projection.settings.menuBarAdaptivePanelContent {
        case .tasks, .eventsAndTasks: true
        case .events: false
        }
    }

    private var currentEvent: CalendarEventMirror? {
        guard includesEvents else { return nil }
        let now = Date()
        return projection.adaptiveEvents
            .filter { event in
                event.status != .cancelled
                    && event.isAllDay == false
                    && event.startDate <= now
                    && event.endDate > now
            }
            .sorted { lhs, rhs in
                if lhs.endDate == rhs.endDate {
                    return lhs.summary.localizedCaseInsensitiveCompare(rhs.summary) == .orderedAscending
                }
                return lhs.endDate < rhs.endDate
            }
            .first
    }

    private var tomorrow: Date {
        Calendar.current.date(byAdding: .day, value: 1, to: Date()) ?? Date()
    }

    private var todayItems: [AgendaItem] {
        agendaItems(on: Date())
    }

    private var tomorrowItems: [AgendaItem] {
        agendaItems(on: tomorrow)
    }

    private func agendaItems(on day: Date) -> [AgendaItem] {
        var items: [AgendaItem] = []
        if includesEvents {
            let currentID = currentEvent?.id
            items.append(contentsOf: projection.events(on: day)
                .filter { $0.id != currentID }
                .map(AgendaItem.event))
        }
        if includesTasks {
            items.append(contentsOf: projection.tasks(on: day).map(AgendaItem.task))
        }
        return items.sorted { lhs, rhs in
            if lhs.isAllDayEvent != rhs.isAllDayEvent {
                return lhs.isAllDayEvent
            }
            if lhs.sortDate == rhs.sortDate {
                return lhs.id < rhs.id
            }
            return lhs.sortDate < rhs.sortDate
        }
    }

    private func eventSubtitle(for event: CalendarEventMirror) -> String {
        let calendarTitle = projection.calendarTitle(for: event.calendarID, fallback: "Calendar")
        if event.isAllDay {
            return "All day - \(calendarTitle)"
        }
        return "\(event.startDate.formatted(date: .omitted, time: .shortened))-\(event.endDate.formatted(date: .omitted, time: .shortened)) - \(calendarTitle)"
    }

    private func taskSubtitle(for task: TaskMirror) -> String {
        let listTitle = projection.taskListTitle(for: task.taskListID, fallback: "Tasks")
        guard let dueDate = task.dueDate else { return listTitle }
        let calendar = Calendar.current
        if dueDate < calendar.startOfDay(for: Date()) {
            return "Overdue - \(listTitle)"
        }
        if calendar.isDateInToday(dueDate) {
            return "Due today - \(listTitle)"
        }
        if calendar.isDateInTomorrow(dueDate) {
            return "Due tomorrow - \(listTitle)"
        }
        return "Due \(dueDate.formatted(.dateTime.weekday(.abbreviated).month().day())) - \(listTitle)"
    }

    private func meetURL(for event: CalendarEventMirror) -> URL? {
        let trimmed = event.meetLink.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else { return nil }
        return URL(string: trimmed)
    }

    private func open(_ event: CalendarEventMirror) {
        NSApp.activate(ignoringOtherApps: true)
        NotificationCenter.default.post(name: .hcbRevealEventInCalendar, object: event.id)
    }

    private func open(_ task: TaskMirror) {
        NSApp.activate(ignoringOtherApps: true)
        NotificationCenter.default.post(name: .hcbRevealTaskInStore, object: task.id)
    }

    private func complete(_ task: TaskMirror) {
        guard completingTaskIDs.contains(task.id) == false else { return }
        completingTaskIDs.insert(task.id)
        Task {
            _ = await model.setTaskCompleted(true, task: task)
            completingTaskIDs.remove(task.id)
        }
    }

    private func snooze(_ task: TaskMirror, to newDate: Date?) async {
        _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: newDate)
    }
}

private struct DetailedMenuBarPanel: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    @State private var selectedDay = Calendar.current.startOfDay(for: Date())
    @State private var snoozeCustomTask: TaskMirror?
    @State private var pendingDeleteEvent: CalendarEventMirror?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            MenuBarMonthCalendar(selectedDay: $selectedDay)
            Divider()
            selectedDayHeader
            agenda
            MenuBarQuickAddRow()
            Divider()
            MenuBarAccountSwitcher()
            MenuBarQuickActions()
        }
        .hcbScaledPadding(12)
        .hcbScaledFrame(width: 320)
        .sheet(item: $snoozeCustomTask) { task in
            SnoozePickerSheet(task: task) { newDate in
                Task { await snooze(task, to: newDate) }
            }
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
    }

    private var selectedDayHeader: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(selectedDay.formatted(.dateTime.weekday(.wide).month().day()))
                .hcbFont(.subheadline, weight: .semibold)
            Spacer()
            let eventCount = eventsForSelectedDay.count
            let taskCount = tasksForSelectedDay.count
            Text("\(eventCount) event\(eventCount == 1 ? "" : "s") · \(taskCount) task\(taskCount == 1 ? "" : "s")")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    // Renders a focused list for just the selected day instead of a 14-day
    // horizon. The previous horizon-based agenda was silently empty in
    // practice because the ScrollView collapsed between DatePicker and
    // QuickAddRow — this tight per-day list fills predictably.
    private var agenda: some View {
        let events = eventsForSelectedDay
        let tasks = tasksForSelectedDay
        return Group {
            if events.isEmpty && tasks.isEmpty {
                Text("Nothing scheduled for this day.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .hcbScaledPadding(.vertical, 6)
            } else {
                ScrollView {
                    LazyVStack(alignment: .leading, spacing: 6) {
                        ForEach(events, id: \.id) { event in
                            eventDayItemRow(event)
                        }
                        ForEach(tasks, id: \.id) { task in
                            taskDayItemRow(task)
                        }
                    }
                    .hcbScaledPadding(.top, 2)
                }
                .hcbScaledFrame(maxHeight: 200)
            }
        }
    }

    private func dayItemRow(title: String, subtitle: String, symbol: String) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: symbol)
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
                .hcbScaledFrame(width: 16)
            VStack(alignment: .leading, spacing: 1) {
                Text(title)
                    .hcbFont(.subheadline)
                    .lineLimit(1)
                Text(subtitle)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }
            Spacer(minLength: 0)
        }
    }

    private func taskDayItemRow(_ task: TaskMirror) -> some View {
        dayItemRow(
            title: task.title,
            subtitle: projection.taskListTitle(for: task.taskListID, fallback: "Tasks"),
            symbol: "checkmark.circle"
        )
        .contextMenu {
            TaskContextMenu(
                task: task,
                onOpen: { open(task) },
                onCustomSnooze: { snoozeCustomTask = task },
                onDelete: {
                    Task { _ = await model.deleteTask(task) }
                }
            )
        }
    }

    private func open(_ task: TaskMirror) {
        NSApp.activate(ignoringOtherApps: true)
        NotificationCenter.default.post(name: .hcbRevealTaskInStore, object: task.id)
    }

    private func eventDayItemRow(_ event: CalendarEventMirror) -> some View {
        dayItemRow(
            title: event.summary,
            subtitle: event.isAllDay
                ? "All day"
                : event.startDate.formatted(.dateTime.hour().minute()),
            symbol: "calendar"
        )
        .contextMenu {
            EventContextMenu(
                event: event,
                onOpen: { open(event) },
                onDelete: { pendingDeleteEvent = event }
            )
        }
    }

    private func open(_ event: CalendarEventMirror) {
        NSApp.activate(ignoringOtherApps: true)
        NotificationCenter.default.post(name: .hcbRevealEventInCalendar, object: event.id)
    }

    private var eventsForSelectedDay: [CalendarEventMirror] {
        projection.events(on: selectedDay)
    }

    private var tasksForSelectedDay: [TaskMirror] {
        projection.tasks(on: selectedDay)
    }

    private func snooze(_ task: TaskMirror, to newDate: Date?) async {
        _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: newDate)
    }

}

private struct CompactMenuBarPanel: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    @State private var completingTaskIDs: Set<TaskMirror.ID> = []
    @State private var snoozeCustomTask: TaskMirror?

    private enum Lane: Int, CaseIterable, Identifiable {
        case now
        case next
        case later

        var id: Int { rawValue }

        var title: String {
            switch self {
            case .now: "Now"
            case .next: "Next"
            case .later: "Later"
            }
        }

        // SF Symbol rendered in place of the NOW/NEXT/LATER text labels.
        // Reads faster + matches native macOS menu bar apps that prefer
        // glyphs for tight vertical columns.
        var laneSymbol: String {
            switch self {
            case .now: "bolt.fill"
            case .next: "arrow.forward"
            case .later: "clock"
            }
        }

        @MainActor
        var tint: Color {
            switch self {
            case .now: AppColor.ember
            case .next: AppColor.blue
            case .later: AppColor.moss
            }
        }

        var emptyState: String {
            switch self {
            case .now: "You're clear right now."
            case .next: "Nothing queued next."
            case .later: "No later commitments."
            }
        }
    }

    private enum ActionableItem {
        case task(TaskMirror)
        case event(CalendarEventMirror)
    }

    private struct LaneRow: Identifiable {
        let lane: Lane
        let title: String
        let subtitle: String
        let symbol: String
        let color: Color
        let task: TaskMirror?
        let isPlaceholder: Bool

        var id: Int { lane.rawValue }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                Text("Compact")
                    .hcbFont(.headline)
                Spacer()
                Text(projection.syncState.title)
                    .hcbFont(.caption, weight: .medium)
                    .foregroundStyle(.secondary)
            }

            VStack(spacing: 7) {
                ForEach(Lane.allCases) { lane in
                    laneRow(for: row(for: lane))
                }
            }

            MenuBarQuickAddRow()
            Divider()
            MenuBarAccountSwitcher()
            MenuBarQuickActions()
        }
        .hcbScaledPadding(14)
        .hcbScaledFrame(width: 320)
        .sheet(item: $snoozeCustomTask) { task in
            SnoozePickerSheet(task: task) { newDate in
                Task { await snooze(task, to: newDate) }
            }
        }
    }

    @ViewBuilder
    private func laneRow(for row: LaneRow) -> some View {
        HStack(spacing: 8) {
            Image(systemName: row.lane.laneSymbol)
                .hcbFont(.subheadline, weight: .semibold)
                .foregroundStyle(.secondary)
                .hcbScaledFrame(width: 24, alignment: .center)
                .accessibilityLabel(row.lane.title)
                .help(row.lane.title)

            VStack(alignment: .leading, spacing: 1) {
                Text(row.title)
                    .font(.subheadline.weight(row.isPlaceholder ? .regular : .semibold))
                    .lineLimit(1)
                    .foregroundStyle(row.isPlaceholder ? .secondary : .primary)
                Text(row.subtitle)
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            Spacer(minLength: 0)

            if let task = row.task {
                Button {
                    complete(task)
                } label: {
                    if completingTaskIDs.contains(task.id) {
                        ProgressView()
                            .controlSize(.small)
                    } else {
                        Image(systemName: "checkmark.circle")
                            .foregroundStyle(.tint)
                    }
                }
                .buttonStyle(.borderless)
                .disabled(completingTaskIDs.contains(task.id))
            }
        }
        .hcbScaledPadding(.horizontal, 4)
        .hcbScaledPadding(.vertical, 4)
        .contextMenu {
            if let task = row.task {
                TaskContextMenu(
                    task: task,
                    onOpen: { open(task) },
                    onCustomSnooze: { snoozeCustomTask = task },
                    onDelete: {
                        Task { _ = await model.deleteTask(task) }
                    }
                )
            }
        }
    }

    private func open(_ task: TaskMirror) {
        NSApp.activate(ignoringOtherApps: true)
        NotificationCenter.default.post(name: .hcbRevealTaskInStore, object: task.id)
    }

    private var actionable: [ActionableItem] {
        let now = Date()
        let calendar = Calendar.current
        let startOfToday = calendar.startOfDay(for: now)
        let taskPool = projection.adaptiveTasks

        let overdueTasks = taskPool.filter { ($0.dueDate ?? .distantFuture) < startOfToday }
        let dueTodayTasks = taskPool.filter { task in
            guard let dueDate = task.dueDate else { return false }
            return calendar.isDate(dueDate, inSameDayAs: now)
        }
        let futureTasks = taskPool.filter { ($0.dueDate ?? .distantPast) > startOfToday }

        let eventPool = projection.adaptiveEvents
            .filter { event in event.status != .cancelled && event.endDate > now }
            .sorted { $0.startDate < $1.startDate }
        let ongoingEvent = eventPool.first(where: { $0.startDate <= now && $0.endDate > now })
        let upcomingEvents = eventPool.filter { $0.startDate > now }

        var items: [ActionableItem] = []
        var seenTaskIDs: Set<TaskMirror.ID> = []
        var seenEventIDs: Set<CalendarEventMirror.ID> = []

        func addTask(_ task: TaskMirror) {
            guard seenTaskIDs.insert(task.id).inserted else { return }
            items.append(.task(task))
        }

        func addEvent(_ event: CalendarEventMirror) {
            guard seenEventIDs.insert(event.id).inserted else { return }
            items.append(.event(event))
        }

        overdueTasks.prefix(2).forEach(addTask)
        if let ongoingEvent {
            addEvent(ongoingEvent)
        }
        upcomingEvents.prefix(3).forEach(addEvent)
        dueTodayTasks.prefix(3).forEach(addTask)
        futureTasks.prefix(3).forEach(addTask)

        return Array(items.prefix(3))
    }

    private func row(for lane: Lane) -> LaneRow {
        guard actionable.indices.contains(lane.rawValue) else {
            return LaneRow(
                lane: lane,
                title: lane.emptyState,
                subtitle: "No immediate tasks or events",
                symbol: "sparkles",
                color: .secondary,
                task: nil,
                isPlaceholder: true
            )
        }

        switch actionable[lane.rawValue] {
        case .task(let task):
            return LaneRow(
                lane: lane,
                title: task.title,
                subtitle: taskSubtitle(for: task),
                symbol: "checkmark.circle",
                color: AppColor.ember,
                task: task,
                isPlaceholder: false
            )
        case .event(let event):
            return LaneRow(
                lane: lane,
                title: event.summary,
                subtitle: eventSubtitle(for: event),
                symbol: "calendar",
                color: AppColor.blue,
                task: nil,
                isPlaceholder: false
            )
        }
    }

    private func taskSubtitle(for task: TaskMirror) -> String {
        let listTitle = projection.taskListTitle(for: task.taskListID, fallback: "Tasks")
        guard let dueDate = task.dueDate else {
            return listTitle
        }

        let calendar = Calendar.current
        if dueDate < calendar.startOfDay(for: Date()) {
            return "Overdue · \(listTitle)"
        }
        if calendar.isDateInToday(dueDate) {
            return "Due today · \(listTitle)"
        }
        return "Due \(dueDate.formatted(.dateTime.weekday(.abbreviated).month().day())) · \(listTitle)"
    }

    private func eventSubtitle(for event: CalendarEventMirror) -> String {
        let calendarTitle = projection.calendarTitle(for: event.calendarID, fallback: "Calendar")
        if event.isAllDay {
            return "All day · \(calendarTitle)"
        }
        return "\(event.startDate.formatted(date: .omitted, time: .shortened))–\(event.endDate.formatted(date: .omitted, time: .shortened)) · \(calendarTitle)"
    }

    private func complete(_ task: TaskMirror) {
        guard completingTaskIDs.contains(task.id) == false else { return }
        completingTaskIDs.insert(task.id)
        Task {
            _ = await model.setTaskCompleted(true, task: task)
            completingTaskIDs.remove(task.id)
        }
    }

    private func snooze(_ task: TaskMirror, to newDate: Date?) async {
        _ = await model.updateTask(task, title: task.title, notes: task.notes, dueDate: newDate)
    }
}

private struct MenuBarQuickAddRow: View {
    @Environment(AppModel.self) private var model
    private var taskStore: TaskStore { model.taskStore }
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    @State private var input: String = ""
    @State private var isSubmitting = false
    @State private var errorMessage: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            HStack(spacing: 8) {
                TextField("Add a task — tmr 9am #work", text: $input)
                    .textFieldStyle(.roundedBorder)
                    .hcbFont(.subheadline)
                    .onSubmit { Task { await submit() } }
                if isSubmitting {
                    ProgressView().controlSize(.small)
                }
            }
            if let errorMessage {
                Text(errorMessage)
                    .hcbFont(.caption2)
                    .foregroundStyle(AppColor.ember)
            } else if projection.account == nil {
                Text("Connect Google in Settings before adding tasks.")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            } else if taskStore.taskLists.isEmpty {
                Text("Refresh to load your task lists.")
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
            }
        }
    }

    private func submit() async {
        let parsed = NaturalLanguageTaskParser().parse(input)
        guard parsed.title.isEmpty == false else { return }
        guard let listID = resolvedListID(hint: parsed.taskListHint) else {
            errorMessage = "Connect Google and pick a task list first."
            return
        }
        isSubmitting = true
        errorMessage = nil
        let created = await model.createTask(
            title: parsed.title,
            notes: "",
            dueDate: parsed.dueDate,
            taskListID: listID
        )
        isSubmitting = false
        if created {
            input = ""
        } else {
            errorMessage = model.lastMutationError ?? "Couldn't add task."
        }
    }

    private func resolvedListID(hint: String?) -> TaskListMirror.ID? {
        if let hint {
            if let exact = taskStore.taskLists.first(where: { $0.title.localizedCaseInsensitiveCompare(hint) == .orderedSame }) {
                return exact.id
            }
            if let fuzzy = taskStore.taskLists.first(where: { $0.title.localizedCaseInsensitiveContains(hint) }) {
                return fuzzy.id
            }
        }
        return taskStore.taskLists.first?.id
    }
}

private struct MenuBarPinnedFilters: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    private var projection: MenuBarProjection { model.menuBarStore.projection }

    // Up to 3 matching tasks are shown inline per pinned filter — enough
    // to be useful at a glance without letting the popover grow unbounded.
    private let previewLimit = 3
    // Cap the whole popover too — 4 pinned filters max in the list.
    private let pinnedLimit = 4

    var body: some View {
        let filters = projection.pinnedFilters
        if filters.isEmpty == false {
            VStack(alignment: .leading, spacing: 10) {
                Divider()
                Text("Pinned filters")
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(filters.prefix(pinnedLimit)) { f in
                        filterRow(for: f)
                    }
                }
            }
        }
    }

    @ViewBuilder
    private func filterRow(for projection: MenuBarPinnedFilterProjection) -> some View {
        let f = projection.definition
        let tasks = projection.matchingTasks
        Button {
            openFilter(f)
        } label: {
            VStack(alignment: .leading, spacing: 4) {
                HStack(spacing: 8) {
                    Image(systemName: f.systemImage)
                        .foregroundStyle(AppColor.ember)
                    Text(f.name)
                        .hcbFont(.subheadline, weight: .semibold)
                    Spacer()
                    Text("\(tasks.count)")
                        .hcbFont(.caption, weight: .semibold)
                        .foregroundStyle(.secondary)
                        .hcbScaledPadding(.horizontal, 6)
                        .hcbScaledPadding(.vertical, 1)
                        .background(Capsule().fill(.quaternary.opacity(0.5)))
                }
                if tasks.isEmpty {
                    Text("No matches")
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                } else {
                    ForEach(tasks.prefix(previewLimit)) { task in
                        HStack(spacing: 6) {
                            Image(systemName: task.isCompleted ? "checkmark.circle.fill" : "circle")
                                .hcbFont(.caption)
                                .foregroundStyle(task.isCompleted ? AppColor.moss : AppColor.ember)
                            Text(TagExtractor.stripped(from: task.title))
                                .hcbFont(.caption)
                                .lineLimit(1)
                            Spacer()
                        }
                    }
                    if tasks.count > previewLimit {
                        Text("+\(tasks.count - previewLimit) more")
                            .hcbFont(.caption2)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            .hcbScaledPadding(.vertical, 4)
            .hcbScaledPadding(.horizontal, 6)
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
    }

    private func openFilter(_ f: CustomFilterDefinition) {
        model.markCustomFilterUsed(f.id)
        NotificationCenter.default.post(name: .hcbOpenStoreTab, object: nil)
        HCBMainWindowPresenter.shared.show(openWindow: openWindow)
    }
}

private struct MenuBarAccountSwitcher: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }

    var body: some View {
        if let activeAccount, displayAccounts.isEmpty == false {
            VStack(alignment: .leading, spacing: 8) {
                Menu {
                    ForEach(displayAccounts) { account in
                        Button {
                            switchAccount(account)
                        } label: {
                            Label(
                                accountMenuTitle(account),
                                systemImage: account.id == activeAccount.id ? "checkmark.circle.fill" : "person.crop.circle"
                            )
                        }
                        .disabled(account.id == activeAccount.id)
                    }
                } label: {
                    HStack(spacing: 8) {
                        Image(systemName: "person.crop.circle")
                            .foregroundStyle(.secondary)
                            .hcbScaledFrame(width: 16)
                        VStack(alignment: .leading, spacing: 1) {
                            Text("Google account")
                                .hcbFont(.caption2, weight: .semibold)
                                .foregroundStyle(.secondary)
                            Text(activeAccount.displayName)
                                .hcbFont(.caption, weight: .semibold)
                                .lineLimit(1)
                            Text(activeAccount.email)
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                                .lineLimit(1)
                        }
                        Spacer(minLength: 0)
                        Image(systemName: "chevron.up.chevron.down")
                            .hcbFont(.caption2, weight: .semibold)
                            .foregroundStyle(.secondary)
                    }
                    .hcbScaledPadding(.vertical, 5)
                    .hcbScaledPadding(.horizontal, 6)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .help("Switch Google account")
                .accessibilityLabel("Google account, \(activeAccount.displayName)")

                Divider()
            }
        }
    }

    private var activeAccount: GoogleAccount? {
        let activeID = projection.activeAccountID ?? projection.account?.id
        if let activeID, let account = displayAccounts.first(where: { $0.id == activeID }) {
            return account
        }
        return projection.account ?? displayAccounts.first
    }

    private var displayAccounts: [GoogleAccount] {
        projection.displayAccounts
    }

    private func accountMenuTitle(_ account: GoogleAccount) -> String {
        if account.email.isEmpty {
            return account.displayName
        }
        return "\(account.displayName) (\(account.email))"
    }

    private func switchAccount(_ account: GoogleAccount) {
        guard account.id != activeAccount?.id else { return }
        Task { await model.switchGoogleAccount(to: account.id) }
    }
}

private struct MenuBarQuickActions: View {
    @Environment(AppModel.self) private var model
    @Environment(\.openWindow) private var openWindow
    private var projection: MenuBarProjection { model.menuBarStore.projection }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Button {
                bringAppToFront()
            } label: {
                Label("Open Hot Cross Buns", systemImage: "arrow.up.right.square")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.borderless)

            Button {
                Task { await model.refreshNow() }
            } label: {
                Label("Refresh", systemImage: "arrow.clockwise")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.borderless)
            .disabled(projection.account == nil)

            Button {
                openWindow(id: "history")
            } label: {
                Label("History…", systemImage: "clock.arrow.circlepath")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.borderless)

            Button {
                NSApp.terminate(nil)
            } label: {
                Label("Quit", systemImage: "power")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.borderless)
        }
    }

    private func bringAppToFront() {
        HCBMainWindowPresenter.shared.show(openWindow: openWindow)
    }
}

private struct MenuBarMonthCalendar: View {
    @Binding var selectedDay: Date
    @State private var displayedMonth: Date = Calendar.current.startOfDay(for: Date())

    private let calendar = Calendar.current
    private let columns = Array(repeating: GridItem(.flexible(minimum: 28), spacing: 4), count: 7)

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            header
            weekdayHeader
            LazyVGrid(columns: columns, spacing: 4) {
                ForEach(monthDays, id: \.self) { day in
                    dayButton(day)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .onAppear { displayedMonth = calendar.startOfMonth(for: selectedDay) }
        .onChange(of: selectedDay) { _, newValue in
            if calendar.isDate(newValue, equalTo: displayedMonth, toGranularity: .month) == false {
                displayedMonth = calendar.startOfMonth(for: newValue)
            }
        }
    }

    private var header: some View {
        HStack(spacing: 8) {
            Text(displayedMonth.formatted(.dateTime.month(.wide).year()))
                .hcbFont(.headline)
                .lineLimit(1)
            Spacer(minLength: 0)
            Button {
                shiftMonth(by: -1)
            } label: {
                Image(systemName: "chevron.left")
                    .frame(width: 20, height: 20)
            }
            .help("Previous month")
            Button {
                let today = calendar.startOfDay(for: Date())
                selectedDay = today
                displayedMonth = calendar.startOfMonth(for: today)
            } label: {
                Image(systemName: "circle.fill")
                    .font(.system(size: 7, weight: .semibold))
                    .frame(width: 20, height: 20)
            }
            .help("Today")
            Button {
                shiftMonth(by: 1)
            } label: {
                Image(systemName: "chevron.right")
                    .frame(width: 20, height: 20)
            }
            .help("Next month")
        }
        .buttonStyle(.plain)
        .foregroundStyle(.secondary)
    }

    private var weekdayHeader: some View {
        LazyVGrid(columns: columns, spacing: 4) {
            ForEach(weekdaySymbols, id: \.self) { symbol in
                Text(symbol)
                    .hcbFont(.caption, weight: .semibold)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity)
            }
        }
    }

    private var weekdaySymbols: [String] {
        let symbols = calendar.shortWeekdaySymbols
        let offset = max(calendar.firstWeekday - 1, 0)
        return Array(symbols[offset...]) + Array(symbols[..<offset])
    }

    private var monthDays: [Date] {
        let monthStart = calendar.startOfMonth(for: displayedMonth)
        let firstWeekdayOffset = (calendar.component(.weekday, from: monthStart) - calendar.firstWeekday + 7) % 7
        let gridStart = calendar.date(byAdding: .day, value: -firstWeekdayOffset, to: monthStart) ?? monthStart
        return (0..<42).compactMap { calendar.date(byAdding: .day, value: $0, to: gridStart) }
    }

    private func dayButton(_ day: Date) -> some View {
        let isSelected = calendar.isDate(day, inSameDayAs: selectedDay)
        let isToday = calendar.isDateInToday(day)
        let isPast = calendar.startOfDay(for: day) < calendar.startOfDay(for: Date())

        return Button {
            selectedDay = calendar.startOfDay(for: day)
            displayedMonth = calendar.startOfMonth(for: day)
        } label: {
            Text("\(calendar.component(.day, from: day))")
                .font(.body.monospacedDigit().weight(isToday || isSelected ? .semibold : .regular))
                .foregroundStyle(dayForeground(isSelected: isSelected, isPast: isPast))
                .frame(maxWidth: .infinity, minHeight: 26)
                .background {
                    RoundedRectangle(cornerRadius: 6)
                        .fill(isSelected ? AppColor.blue.opacity(0.95) : isToday ? AppColor.blue.opacity(0.18) : Color.clear)
                }
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help(day.formatted(.dateTime.weekday(.wide).month(.wide).day().year()))
    }

    private func dayForeground(isSelected: Bool, isPast: Bool) -> Color {
        if isSelected { return .white }
        return isPast ? Color.secondary.opacity(0.35) : .primary
    }

    private func shiftMonth(by value: Int) {
        displayedMonth = calendar.date(byAdding: .month, value: value, to: displayedMonth) ?? displayedMonth
    }
}

private extension Calendar {
    func startOfMonth(for date: Date) -> Date {
        self.date(from: dateComponents([.year, .month], from: date)) ?? startOfDay(for: date)
    }
}

private struct StatusLine: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(.secondary)
            Spacer()
            Text(value)
                .font(.body.monospacedDigit())
        }
        .hcbFont(.callout)
    }
}

private struct WeeklyMenuBarDayDetailPanel: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    let day: Date

    private var events: [CalendarEventMirror] {
        projection.events(on: day)
    }

    private var tasks: [TaskMirror] {
        projection.tasks(on: day)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            header
            Divider()
            content
        }
        .hcbScaledPadding(12)
        .hcbScaledFrame(width: 300, alignment: .leading)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(day.formatted(.dateTime.weekday(.wide).month(.abbreviated).day()))
                .hcbFont(.headline)
                .lineLimit(1)
            Text("\(events.count) event\(events.count == 1 ? "" : "s") · \(tasks.count) task\(tasks.count == 1 ? "" : "s")")
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder
    private var content: some View {
        if events.isEmpty && tasks.isEmpty {
            HStack(spacing: 8) {
                Image(systemName: "tray")
                    .foregroundStyle(.secondary)
                Text("Nothing scheduled for this day.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
            .hcbScaledPadding(.vertical, 10)
        } else {
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 10) {
                    if events.isEmpty == false {
                        detailSection(title: "Events") {
                            ForEach(events, id: \.id) { event in
                                detailRow(
                                    symbol: "calendar",
                                    title: event.summary,
                                    subtitle: eventSubtitle(event)
                                )
                            }
                        }
                    }

                    if tasks.isEmpty == false {
                        detailSection(title: "Tasks") {
                            ForEach(tasks, id: \.id) { task in
                                detailRow(
                                    symbol: "checkmark.circle",
                                    title: task.title,
                                    subtitle: taskSubtitle(task)
                                )
                            }
                        }
                    }
                }
                .hcbScaledPadding(.vertical, 1)
            }
            .hcbScaledFrame(maxHeight: 320)
        }
    }

    private func detailSection<Content: View>(
        title: String,
        @ViewBuilder content: () -> Content
    ) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .hcbFont(.caption2, weight: .semibold)
                .foregroundStyle(.secondary)
            content()
        }
    }

    private func detailRow(symbol: String, title: String, subtitle: String) -> some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: symbol)
                .hcbFont(.caption)
                .foregroundStyle(.secondary)
                .hcbScaledFrame(width: 16)
            VStack(alignment: .leading, spacing: 2) {
                Text(title.isEmpty ? "Untitled" : title)
                    .hcbFont(.subheadline, weight: .medium)
                    .lineLimit(2)
                Text(subtitle)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(2)
            }
            Spacer(minLength: 0)
        }
        .hcbScaledPadding(.vertical, 2)
    }

    private func eventSubtitle(_ event: CalendarEventMirror) -> String {
        var parts: [String] = [eventTimeLabel(event)]
        let calendar = projection.calendarTitle(for: event.calendarID, fallback: "")
        if calendar.isEmpty == false {
            parts.append(calendar)
        }
        if event.location.isEmpty == false {
            parts.append(event.location)
        }
        return parts.joined(separator: " · ")
    }

    private func taskSubtitle(_ task: TaskMirror) -> String {
        var parts: [String] = []
        if let due = task.dueDate {
            parts.append(due.formatted(.dateTime.hour().minute()))
        }
        parts.append(projection.taskListTitle(for: task.taskListID, fallback: "Tasks"))
        if task.notes.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            parts.append(task.notes.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        return parts.joined(separator: " · ")
    }

    private func eventTimeLabel(_ event: CalendarEventMirror) -> String {
        if event.isAllDay { return "All day" }
        return "\(event.startDate.formatted(.dateTime.hour().minute()))-\(event.endDate.formatted(.dateTime.hour().minute()))"
    }
}

private struct WeeklyMenuBarPanel: View {
    @Environment(AppModel.self) private var model
    private var projection: MenuBarProjection { model.menuBarStore.projection }
    let onDaySelected: (Date) -> Void

    private var days: [Date] {
        let cal = Calendar.current
        let today = cal.startOfDay(for: Date())
        return (0..<7).compactMap { cal.date(byAdding: .day, value: $0, to: today) }
    }

    private func eventsOn(_ day: Date) -> [CalendarEventMirror] {
        projection.events(on: day)
    }

    private func tasksOn(_ day: Date) -> [TaskMirror] {
        projection.tasks(on: day)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                Text("Next 7 days")
                    .hcbFont(.headline)
                Spacer()
                Text(projection.syncState.title)
                    .hcbFont(.caption, weight: .medium)
                    .foregroundStyle(.secondary)
            }
            VStack(spacing: 6) {
                ForEach(days, id: \.self) { day in
                    dayRow(day)
                }
            }
            MenuBarQuickAddRow()
            Divider()
            MenuBarAccountSwitcher()
            MenuBarQuickActions()
        }
        .hcbScaledPadding(14)
        .hcbScaledFrame(width: 320)
    }

    private func dayRow(_ day: Date) -> some View {
        let events = eventsOn(day)
        let tasks = tasksOn(day)
        let cal = Calendar.current
        let isToday = cal.isDateInToday(day)
        // Monochrome: weekday + day number use .primary / .secondary only.
        // Today is signalled via a bolder day-number weight, not color — in
        // line with native macOS menu bar apps.
        return Button {
            onDaySelected(day)
        } label: {
            HStack(spacing: 10) {
                VStack(alignment: .leading, spacing: 0) {
                    Text(day.formatted(.dateTime.weekday(.abbreviated)))
                        .hcbFont(.caption2, weight: .semibold)
                        .foregroundStyle(.secondary)
                    Text("\(cal.component(.day, from: day))")
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(isToday ? .primary : .secondary)
                }
                .hcbScaledFrame(width: 38)
                Divider().hcbScaledFrame(height: 28)
                HStack(spacing: 6) {
                    countChip(symbol: "calendar", count: events.count)
                    countChip(symbol: "checkmark.circle", count: tasks.count)
                }
                Spacer(minLength: 0)
                if let first = events.first {
                    Text(first.isAllDay ? "All day" : first.startDate.formatted(.dateTime.hour().minute()))
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                }
            }
            .hcbScaledPadding(.vertical, 4)
            .hcbScaledPadding(.horizontal, 6)
            .background(isToday ? Color.secondary.opacity(0.10) : Color.clear)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .help("Show \(day.formatted(.dateTime.weekday(.wide).month(.wide).day()))")
    }

    private func countChip(symbol: String, count: Int) -> some View {
        HStack(spacing: 3) {
            Image(systemName: symbol)
                .hcbFont(.caption2)
            Text("\(count)")
                .font(.caption2.monospacedDigit())
        }
        .foregroundStyle(count == 0 ? .tertiary : .secondary)
    }
}
