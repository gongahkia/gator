import AppKit
import SwiftUI
import Observation
import GatorCore

// Geometry uses the display's reported camera gap, including external-display origins.
struct NotchGeometry: Equatable {
    let frame: NSRect
    let cameraWidth: CGFloat
    let cameraHeight: CGFloat

    static func detect(frame: NSRect, safeTop: CGFloat, left: NSRect?, right: NSRect?) -> Self? {
        guard safeTop > 0, let left, let right, right.minX > left.maxX,
              left.minX >= frame.minX, right.maxX <= frame.maxX else { return nil }
        let gap = right.minX - left.maxX
        guard gap < frame.width / 2 else { return nil }
        return Self(frame: frame, cameraWidth: gap, cameraHeight: safeTop)
    }
    func panelFrame(expanded: Bool) -> NSRect {
        let width = min(frame.width - 24, expanded ? max(420, cameraWidth + 96) : cameraWidth + 64)
        let height = cameraHeight + (expanded ? 214 : 6)
        return NSRect(x: frame.midX - width / 2, y: frame.maxY - height, width: width, height: height)
    }
}

@MainActor @Observable
final class NotchQuickAccess {
    private(set) var geometry: NotchGeometry?
    private(set) var expanded = false
    private(set) var keyboardActive = false
    private(set) var enabled = true
    var canShow: Bool { enabled && geometry != nil }
    private var panel: QuickAccessPanel?
    private var pending: Task<Void, Never>?
    private var observers: [NSObjectProtocol] = []
    private weak var previousWindow: NSWindow?
    private let sessions: ManagedSessionCoordinator
    private let openWindow: () -> Void

    init(sessions: ManagedSessionCoordinator, openWindow: @escaping () -> Void) {
        self.sessions = sessions; self.openWindow = openWindow
        for name in [NSApplication.didChangeScreenParametersNotification, UserDefaults.didChangeNotification] {
            observers.append(NotificationCenter.default.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                Task { @MainActor in self?.configure() }
            })
        }
        configure()
    }
    func stop() {
        pending?.cancel(); pending = nil; panel?.close(); panel = nil
        for observer in observers { NotificationCenter.default.removeObserver(observer) }
        observers.removeAll()
    }
    private func configure() {
        let next = NSScreen.screens.compactMap { NotchGeometry.detect(frame: $0.frame, safeTop: $0.safeAreaInsets.top, left: $0.auxiliaryTopLeftArea, right: $0.auxiliaryTopRightArea) }.first
        enabled = UserDefaults.standard.object(forKey: "gator.notch.enabled") as? Bool ?? true
        guard enabled, let next else {
            pending?.cancel(); expanded = false; keyboardActive = false; panel?.acceptsKeyboard = false; panel?.orderOut(nil); geometry = next; return
        }
        let changed = geometry != next
        geometry = next
        if panel == nil {
            let window = QuickAccessPanel(contentRect: next.panelFrame(expanded: false), styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
            window.backgroundColor = .clear; window.isOpaque = false; window.hasShadow = false
            window.isReleasedWhenClosed = false; window.isMovable = false; window.hidesOnDeactivate = false
            window.level = .mainMenu + 1
            window.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .stationary, .ignoresCycle]
            window.setAccessibilityLabel("Gator quick access")
            observers.append(NotificationCenter.default.addObserver(forName: NSWindow.didResignKeyNotification, object: window, queue: .main) { [weak self] _ in
                Task { @MainActor in self?.dismiss(restoreFocus: false) }
            })
            window.contentView = NSHostingView(rootView: NotchQuickAccessView(presentation: self, sessions: sessions))
            panel = window
        }
        if changed || panel?.isVisible != true { panel?.setFrame(next.panelFrame(expanded: expanded), display: true); panel?.orderFrontRegardless() }
    }
    func hover(_ inside: Bool) {
        guard panel?.acceptsKeyboard != true else { return }
        pending?.cancel()
        pending = Task { [weak self] in
            do { try await Task.sleep(for: .milliseconds(inside ? 160 : 320)) } catch { return }
            guard let self else { return }; self.setExpanded(inside)
        }
    }
    func show() {
        guard canShow, panel?.isVisible == true else { return }
        pending?.cancel(); previousWindow = NSApp.keyWindow
        setExpanded(true); keyboardActive = true; panel?.acceptsKeyboard = true
        NSApp.activate(ignoringOtherApps: true); panel?.makeKeyAndOrderFront(nil)
    }
    private func setExpanded(_ value: Bool) {
        guard let geometry, expanded != value else { return }
        expanded = value
        panel?.setFrame(geometry.panelFrame(expanded: value), display: true)
    }
    func dismiss(restoreFocus: Bool = true) {
        pending?.cancel(); keyboardActive = false; panel?.acceptsKeyboard = false; setExpanded(false)
        if restoreFocus, let previousWindow, previousWindow.isVisible { previousWindow.makeKey() }
        previousWindow = nil
    }
    func open(_ item: NativeChatActivity? = nil) {
        dismiss(restoreFocus: false)
        Task {
            if let item {
                await sessions.openConversation(item.id)
                // Navigation grants no authority. Only open the original owner's current review.
                guard sessions.current.conversationID == item.id else { return }
                if sessions.current.hasReviewableApproval {
                    sessions.closeTaskInspection(); sessions.current.openApprovalReview()
                } else { sessions.inspectTask(item.id) }
            } else { sessions.focusComposer() }
            openWindow()
        }
    }
}

private final class QuickAccessPanel: NSPanel {
    var acceptsKeyboard = false
    override var canBecomeKey: Bool { acceptsKeyboard }
    override var canBecomeMain: Bool { false }
    override func cancelOperation(_ sender: Any?) { (contentView as? NSHostingView<NotchQuickAccessView>)?.rootView.presentation.dismiss() }
}

private struct NotchQuickAccessView: View {
    let presentation: NotchQuickAccess
    let sessions: ManagedSessionCoordinator
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @FocusState private var dismissFocused: Bool
    private var attention: Int { sessions.activity.filter(\.needsAttention).count }
    private var status: String {
        if attention > 0 { return "\(attention) chat\(attention == 1 ? "" : "s") to inspect" }
        if sessions.workingCount > 0 { return "\(sessions.workingCount) task\(sessions.workingCount == 1 ? "" : "s") working" }
        return sessions.current.isConnected ? "Ready when you are" : "Connection unavailable"
    }
    var body: some View {
        VStack(spacing: 0) {
            HStack(spacing: 0) {
                Image(nsImage: GatorBrand.menuBarIcon).resizable().frame(width: 16, height: 16).accessibilityHidden(true)
                Spacer(minLength: presentation.geometry?.cameraWidth ?? 0)
                Image(systemName: attention > 0 ? "exclamationmark.circle.fill" : sessions.workingCount > 0 ? "ellipsis" : "checkmark")
                    .font(.system(size: 11, weight: .semibold)).frame(width: 18).accessibilityHidden(true)
            }
            .padding(.horizontal, presentation.expanded ? 22 : 12)
            .frame(height: presentation.geometry?.cameraHeight ?? 32)
            .contentShape(Rectangle())
            .onTapGesture { presentation.show() }
            .accessibilityElement(children: .ignore).accessibilityLabel("Gator, \(status)")
            .accessibilityAddTraits(.isButton).accessibilityAction { presentation.show() }
            if presentation.expanded {
                VStack(alignment: .leading, spacing: 12) {
                    HStack(alignment: .firstTextBaseline) {
                        VStack(alignment: .leading, spacing: 3) {
                            Text("Gator").font(.headline)
                            Text(status).font(.callout).foregroundStyle(.white.opacity(0.8))
                        }
                        Spacer()
                        Button { presentation.dismiss() } label: { Image(systemName: "xmark").frame(width: 24, height: 24) }
                            .buttonStyle(.plain).focusable().focused($dismissFocused).help("Dismiss quick access · Escape").accessibilityLabel("Dismiss quick access")
                    }
                    if sessions.activity.isEmpty {
                        Text("Your chat stays available while work runs.").font(.callout).foregroundStyle(.white.opacity(0.8)).fixedSize(horizontal: false, vertical: true)
                    } else {
                        ScrollView {
                            VStack(spacing: 4) {
                                ForEach(sessions.activity.sorted { $0.needsAttention && !$1.needsAttention }) { item in
                                    Button { presentation.open(item) } label: {
                                        HStack(spacing: 8) {
                                            Image(systemName: item.needsAttention ? "exclamationmark.circle" : "bubble.left").frame(width: 18)
                                            Text(item.title.isEmpty ? "Conversation" : item.title).lineLimit(1)
                                            Spacer(minLength: 4)
                                            Text(item.status).font(.caption).lineLimit(1)
                                            Image(systemName: "arrow.up.right").font(.caption)
                                        }.padding(.horizontal, 8).padding(.vertical, 6).contentShape(Rectangle())
                                    }.buttonStyle(QuickAccessButtonStyle()).focusable().disabled(!sessions.canNavigate)
                                    .help("Open the original chat and inspect its current work. No action is approved here.")
                                }
                            }
                        }.frame(maxHeight: 76)
                    }
                    HStack {
                        Text("Decisions stay in exact review.").font(.caption).foregroundStyle(.white.opacity(0.8))
                        Spacer(minLength: 8)
                        Button("Open Chat") { presentation.open() }.buttonStyle(.bordered).controlSize(.small).focusable()
                    }
                }.padding(.horizontal, 20).padding(.top, 12).padding(.bottom, 18)
                .transition(.opacity)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
        .foregroundStyle(.white).background(.black, in: UnevenRoundedRectangle(bottomLeadingRadius: presentation.expanded ? 24 : 12, bottomTrailingRadius: presentation.expanded ? 24 : 12))
        .environment(\.colorScheme, .dark)
        .onHover { presentation.hover($0) }
        .onExitCommand { presentation.dismiss() }
        .onChange(of: presentation.keyboardActive) { _, active in dismissFocused = active }
        .animation(reduceMotion ? nil : .easeOut(duration: 0.18), value: presentation.expanded)
    }
}

private struct QuickAccessButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        QuickAccessButtonBody(label: configuration.label, pressed: configuration.isPressed)
    }
}

struct GatorMenuBarLabel: View {
    let sessions: ManagedSessionCoordinator
    let delegate: AppDelegate
    @Environment(\.openWindow) private var openWindow
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    private var attention: Bool { sessions.activity.contains(where: \.needsAttention) }
    private var status: String {
        if attention { return "A chat needs inspection" }
        if sessions.workingCount > 0 { return "\(sessions.workingCount) task\(sessions.workingCount == 1 ? "" : "s") working" }
        return sessions.current.isConnected ? "Ready" : "Not connected"
    }
    var body: some View {
        HStack(spacing: 3) {
            Image(nsImage: GatorBrand.menuBarIcon)
            if attention || sessions.workingCount > 0 {
                Image(systemName: attention ? "exclamationmark.circle.fill" : "ellipsis")
                    .font(.system(size: 9, weight: .semibold)).transition(.opacity)
            }
        }.help("Gator · \(status)").accessibilityLabel("Gator, \(status)")
        .animation(reduceMotion ? nil : .easeOut(duration: 0.15), value: status)
        .task {
            delegate.application = sessions
            if delegate.quickAccess == nil { delegate.quickAccess = NotchQuickAccess(sessions: sessions) { openWindow(id: "conversation"); NSApp.activate(ignoringOtherApps: true) } }
            await sessions.start()
        }
    }
}

private struct QuickAccessButtonBody<Label: View>: View {
    let label: Label
    let pressed: Bool
    @State private var hovering = false
    var body: some View {
        label.background(.white.opacity(pressed ? 0.24 : hovering ? 0.16 : 0.07), in: RoundedRectangle(cornerRadius: 6))
            .onHover { hovering = $0 }
    }
}
