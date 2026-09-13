import AppKit
import SwiftUI

struct GlobalHotkeySection: View {
    @Environment(AppModel.self) private var model
    @Environment(\.globalHotkeyConfigurator) private var globalHotkeyConfigurator
    @State private var isRecording = false
    @State private var permissionPrimer: PermissionPrimer?
    @State private var shouldEnableAfterAccessibilityGrant = false

    var body: some View {
        Section("Global hotkey") {
            Toggle("Global quick-add hotkey", isOn: enabledBinding)
            HStack(alignment: .center, spacing: 10) {
                Text("Shortcut")
                Spacer()
                if isRecording {
                    GlobalHotkeyRecorderView { captured in
                        capture(captured)
                    }
                    .frame(width: 120, height: 28)
                    .background(
                        RoundedRectangle(cornerRadius: 8)
                            .fill(Color.accentColor.opacity(0.18))
                    )
                    .overlay(
                        RoundedRectangle(cornerRadius: 8)
                            .strokeBorder(Color.accentColor, lineWidth: 1)
                    )
                    Button("Cancel") {
                        isRecording = false
                    }
                    .buttonStyle(.borderless)
                    .keyboardShortcut(.cancelAction)
                } else {
                    Button(model.settings.globalHotkeyBinding.displayLabel) {
                        isRecording = true
                    }
                    .buttonStyle(.bordered)
                    .monospaced()

                    if model.settings.globalHotkeyBinding != .defaultQuickAdd {
                        Button("Reset") {
                            apply(binding: .defaultQuickAdd)
                        }
                        .buttonStyle(.borderless)
                    }
                }
            }
            Label(model.globalHotkeyRegistrationState.message, systemImage: statusIcon)
                .hcbFont(.footnote)
                .foregroundStyle(statusColor)
            Text("This is a system-wide shortcut for the floating quick-capture panel. If the combo is already reserved, Hot Cross Buns keeps your previous working shortcut instead of silently dropping it.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .onAppear(perform: refreshRegistrationState)
        .sheet(item: $permissionPrimer) { primer in
            PermissionPrimerView(primer: primer) {
                permissionPrimer = nil
                _ = GlobalHotkeyAccessibilityPermission.isTrusted(prompt: true)
                HotCrossBunsSystemSettings.open(HotCrossBunsSystemSettings.accessibilityURL)
            } onCancel: {
                shouldEnableAfterAccessibilityGrant = false
                permissionPrimer = nil
            }
        }
        .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
            enableIfAccessibilityWasGranted()
        }
    }

    private var enabledBinding: Binding<Bool> {
        Binding(
            get: { model.settings.enableGlobalHotkey },
            set: { newValue in
                if newValue {
                    requestEnableGlobalHotkey()
                } else {
                    shouldEnableAfterAccessibilityGrant = false
                    model.setEnableGlobalHotkey(false)
                    let state = configure(enabled: false, binding: model.settings.globalHotkeyBinding)
                    model.setGlobalHotkeyRegistrationState(state)
                }
            }
        )
    }

    private var statusIcon: String {
        switch model.globalHotkeyRegistrationState {
        case .disabled:
            "keyboard"
        case .ready:
            "checkmark.circle"
        case .needsAccessibilityPermission:
            "lock.trianglebadge.exclamationmark"
        case .failed:
            "exclamationmark.triangle"
        }
    }

    private var statusColor: Color {
        switch model.globalHotkeyRegistrationState {
        case .disabled:
            .secondary
        case .ready:
            AppColor.moss
        case .needsAccessibilityPermission, .failed:
            AppColor.ember
        }
    }

    private func capture(_ binding: GlobalHotkeyBinding?) {
        defer { isRecording = false }
        guard let binding else { return }
        apply(binding: binding)
    }

    private func apply(binding: GlobalHotkeyBinding) {
        let previous = model.settings.globalHotkeyBinding
        let state = configure(enabled: model.settings.enableGlobalHotkey, binding: binding)
        switch state {
        case .disabled:
            model.setGlobalHotkeyBinding(binding)
            model.setGlobalHotkeyRegistrationState(.disabled)
        case .ready:
            model.setGlobalHotkeyBinding(binding)
            model.setGlobalHotkeyRegistrationState(state)
        case .needsAccessibilityPermission:
            shouldEnableAfterAccessibilityGrant = true
            permissionPrimer = .accessibility
            model.setEnableGlobalHotkey(false)
            model.setGlobalHotkeyRegistrationState(.needsAccessibilityPermission)
        case .failed(let message):
            if model.settings.enableGlobalHotkey {
                _ = configure(enabled: true, binding: previous)
                model.setGlobalHotkeyRegistrationState(.failed("\(message) Current hotkey remains \(previous.displayLabel)."))
            } else {
                model.setGlobalHotkeyBinding(binding)
                model.setGlobalHotkeyRegistrationState(.failed(message))
            }
        }
    }

    private func refreshRegistrationState() {
        let state = configure(
            enabled: model.settings.enableGlobalHotkey,
            binding: model.settings.globalHotkeyBinding
        )
        model.setGlobalHotkeyRegistrationState(state)
        if state == .needsAccessibilityPermission {
            model.setEnableGlobalHotkey(false)
        }
    }

    private func configure(enabled: Bool, binding: GlobalHotkeyBinding) -> GlobalHotkeyRegistrationState {
        globalHotkeyConfigurator(enabled: enabled, binding: binding, model: model)
    }

    private func requestEnableGlobalHotkey() {
        guard GlobalHotkeyAccessibilityPermission.isTrusted(prompt: false) else {
            shouldEnableAfterAccessibilityGrant = true
            permissionPrimer = .accessibility
            model.setEnableGlobalHotkey(false)
            model.setGlobalHotkeyRegistrationState(.needsAccessibilityPermission)
            return
        }

        let state = configure(enabled: true, binding: model.settings.globalHotkeyBinding)
        switch state {
        case .ready:
            model.setEnableGlobalHotkey(true)
        case .disabled, .needsAccessibilityPermission, .failed:
            model.setEnableGlobalHotkey(false)
        }
        model.setGlobalHotkeyRegistrationState(state)
    }

    private func enableIfAccessibilityWasGranted() {
        guard shouldEnableAfterAccessibilityGrant,
              GlobalHotkeyAccessibilityPermission.isTrusted(prompt: false)
        else { return }
        shouldEnableAfterAccessibilityGrant = false
        requestEnableGlobalHotkey()
    }
}

struct GlobalHotkeyRecorderView: NSViewRepresentable {
    let onCapture: (GlobalHotkeyBinding?) -> Void

    func makeNSView(context: Context) -> GlobalHotkeyRecorderNSView {
        let view = GlobalHotkeyRecorderNSView()
        view.onCapture = onCapture
        return view
    }

    func updateNSView(_ nsView: GlobalHotkeyRecorderNSView, context: Context) {}
}

final class GlobalHotkeyRecorderNSView: NSView {
    var onCapture: ((GlobalHotkeyBinding?) -> Void)?

    override var acceptsFirstResponder: Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        DispatchQueue.main.async { [weak self] in
            self?.window?.makeFirstResponder(self)
        }
    }

    override func keyDown(with event: NSEvent) {
        if event.keyCode == 53 {
            onCapture?(nil)
            return
        }
        guard let binding = GlobalHotkeyBinding.binding(from: event) else {
            NSSound.beep()
            return
        }
        onCapture?(binding)
    }
}
