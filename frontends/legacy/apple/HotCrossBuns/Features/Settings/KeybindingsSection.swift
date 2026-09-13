import AppKit
import SwiftUI

struct KeybindingsSection: View {
    @Environment(AppModel.self) private var model
    @State private var selectedGroup: HCBShortcutGroup?
    @State private var query: String = ""
    @State private var recording: HCBShortcutCommand?
    @State private var conflictState: HCBShortcutConflictState?

    var body: some View {
        HStack(alignment: .top, spacing: 0) {
            sidebar
                .frame(width: 190, alignment: .topLeading)
                .hcbScaledPadding(.trailing, 16)

            Divider()

            VStack(alignment: .leading, spacing: 14) {
                header
                shortcutList
                if let conflictState {
                    conflictRow(conflictState)
                }
                footer
            }
            .hcbScaledPadding(.leading, 22)
            .frame(maxWidth: .infinity, alignment: .topLeading)
        }
        .hcbScaledPadding(18)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 12))
        .overlay {
            RoundedRectangle(cornerRadius: 12)
                .strokeBorder(.separator.opacity(0.8), lineWidth: 1)
        }
    }

    private var sidebar: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 8) {
                Text("Options")
                    .hcbFont(.caption2, weight: .bold)
                    .foregroundStyle(.secondary)
                sidebarButton(
                    title: "All hotkeys",
                    systemImage: "keyboard",
                    isSelected: selectedGroup == nil
                ) {
                    selectedGroup = nil
                }
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("Groups")
                    .hcbFont(.caption2, weight: .bold)
                    .foregroundStyle(.secondary)
                ForEach(HCBShortcutGroup.allCases) { group in
                    sidebarButton(
                        title: group.title,
                        systemImage: group.systemImage,
                        isSelected: selectedGroup == group
                    ) {
                        selectedGroup = group
                    }
                }
            }

            Spacer(minLength: 0)
        }
    }

    private var header: some View {
        HStack(alignment: .top, spacing: 16) {
            VStack(alignment: .leading, spacing: 4) {
                Text("Search hotkeys")
                    .hcbFont(.title3, weight: .semibold)
                    .foregroundStyle(.primary)
                Text("Showing \(filteredCommands.count) hotkeys.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 12)
            searchField
        }
    }

    private var searchField: some View {
        HStack(spacing: 7) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(.secondary)
            TextField("Filter...", text: $query)
                .textFieldStyle(.plain)
            Image(systemName: "keyboard")
                .foregroundStyle(.secondary)
        }
        .hcbFont(.body)
        .hcbScaledPadding(.horizontal, 9)
        .hcbScaledPadding(.vertical, 7)
        .background(.quaternary.opacity(0.35), in: RoundedRectangle(cornerRadius: 8))
        .overlay {
            RoundedRectangle(cornerRadius: 8)
                .strokeBorder(Color.accentColor.opacity(query.isEmpty ? 0.35 : 0.9), lineWidth: 1)
        }
        .frame(width: 260)
    }

    private var shortcutList: some View {
        LazyVStack(spacing: 0) {
            ForEach(filteredCommands) { command in
                shortcutRow(for: command)
                if command != filteredCommands.last {
                    Divider()
                }
            }
        }
        .background(.quaternary.opacity(0.16), in: RoundedRectangle(cornerRadius: 10))
        .overlay {
            RoundedRectangle(cornerRadius: 10)
                .strokeBorder(.separator.opacity(0.65), lineWidth: 1)
        }
    }

    private func sidebarButton(title: String, systemImage: String, isSelected: Bool, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            HStack(spacing: 9) {
                Image(systemName: systemImage)
                    .frame(width: 18)
                Text(title)
                    .lineLimit(1)
                Spacer(minLength: 0)
                Text("\(count(for: title))")
                    .hcbFont(.caption)
                    .foregroundStyle(isSelected ? .primary : .secondary)
            }
            .hcbFont(.body, weight: isSelected ? .semibold : .regular)
            .foregroundStyle(.primary)
            .hcbScaledPadding(.horizontal, 10)
            .hcbScaledPadding(.vertical, 8)
            .background(isSelected ? Color.accentColor.opacity(0.18) : Color.clear, in: RoundedRectangle(cornerRadius: 8))
        }
        .buttonStyle(.plain)
    }

    @ViewBuilder
    private func shortcutRow(for command: HCBShortcutCommand) -> some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 3) {
                Text(command.title)
                    .foregroundStyle(.primary)
                    .lineLimit(1)
                if selectedGroup == nil {
                    Text(command.group.title)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            Spacer(minLength: 12)
            shortcutField(for: command)
        }
        .hcbFont(.body)
        .hcbScaledPadding(.horizontal, 12)
        .hcbScaledPadding(.vertical, 10)
    }

    @ViewBuilder
    private func shortcutField(for command: HCBShortcutCommand) -> some View {
        let effective = model.settings.shortcutOverrides[command.rawValue] ?? command.defaultBinding
        let isCustom = model.settings.shortcutOverrides[command.rawValue] != nil

        HStack(spacing: 6) {
            if recording == command {
                KeyRecorderView { newBinding in
                    capture(newBinding, for: command)
                    recording = nil
                }
                .frame(width: 110, height: 26)
                .background(
                    RoundedRectangle(cornerRadius: 8)
                        .fill(Color.accentColor.opacity(0.18))
                )
                .overlay(
                    RoundedRectangle(cornerRadius: 8)
                        .strokeBorder(Color.accentColor, lineWidth: 1)
                )
                Text("Press keys…")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
                Button {
                    recording = nil
                } label: {
                    Image(systemName: "xmark.circle")
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .keyboardShortcut(.cancelAction)
                .help("Cancel recording")
            } else {
                Button {
                    recording = command
                } label: {
                    Text(effective.displayLabel.isEmpty ? "Blank" : effective.displayLabel)
                        .hcbFont(.body, weight: .medium)
                        .monospaced()
                        .foregroundStyle(isCustom ? Color.accentColor : .primary)
                        .frame(minWidth: 74, alignment: .center)
                        .hcbScaledPadding(.horizontal, 8)
                        .hcbScaledPadding(.vertical, 5)
                        .background(
                            Capsule()
                                .fill(.quaternary.opacity(0.42))
                        )
                }
                .buttonStyle(.plain)
                .help("Click to record a new shortcut")

                Button {
                    recording = command
                } label: {
                    Image(systemName: "plus.circle")
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
                .help("Record shortcut")

                if isCustom {
                    Button {
                        model.setShortcutBinding(command, binding: nil)
                    } label: {
                        Image(systemName: "xmark.circle")
                            .foregroundStyle(.secondary)
                    }
                    .buttonStyle(.plain)
                    .help("Reset to default (\(command.defaultBinding.displayLabel))")
                }
            }
        }
    }

    private var footer: some View {
        HStack {
            Text("Click a shortcut and press the new key combo. Escape cancels. Modifier keys (⌘⇧⌥⌃) are required except for function keys and arrows.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
            Spacer()
            Button("Reset all to defaults") {
                model.resetAllShortcutBindings()
            }
            .buttonStyle(.borderless)
            .hcbFont(.caption)
            .disabled(model.settings.shortcutOverrides.isEmpty)
        }
    }

    private func conflictRow(_ state: HCBShortcutConflictState) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(state.message, systemImage: "exclamationmark.triangle")
                .hcbFont(.footnote)
                .foregroundStyle(.red)
            ForEach(state.conflictingCommands) { command in
                HStack(spacing: 8) {
                    Text(command.title)
                        .hcbFont(.caption)
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                    Spacer(minLength: 8)
                    Button("Show") {
                        showConflictingCommand(command)
                    }
                    .buttonStyle(.borderless)
                    Button("Reset conflicting shortcut") {
                        resetConflictingShortcut(command, for: state)
                    }
                    .buttonStyle(.borderless)
                    .disabled(model.settings.shortcutOverrides[command.rawValue] == nil)
                    .help(model.settings.shortcutOverrides[command.rawValue] == nil
                        ? "This command is using its default shortcut."
                        : "Reset \(command.title) to its default shortcut.")
                }
            }
        }
        .hcbScaledPadding(10)
        .background(AppColor.ember.opacity(0.10), in: RoundedRectangle(cornerRadius: 8))
    }

    private var filteredCommands: [HCBShortcutCommand] {
        let scoped = HCBShortcutCommand.allCases.filter { command in
            guard let selectedGroup else { return true }
            return command.group == selectedGroup
        }
        let trimmedQuery = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmedQuery.isEmpty == false else { return scoped }
        return scoped.filter { command in
            let effective = model.settings.shortcutOverrides[command.rawValue] ?? command.defaultBinding
            return command.title.localizedCaseInsensitiveContains(trimmedQuery)
                || command.group.title.localizedCaseInsensitiveContains(trimmedQuery)
                || effective.displayLabel.localizedCaseInsensitiveContains(trimmedQuery)
        }
    }

    private func count(for title: String) -> Int {
        if title == "All hotkeys" {
            return HCBShortcutCommand.allCases.count
        }
        return HCBShortcutCommand.allCases.filter { $0.group.title == title }.count
    }

    private func capture(_ newBinding: HCBKeyBinding?, for command: HCBShortcutCommand) {
        guard let newBinding else { return }
        let conflicts = hcbConflictingCommands(
            proposed: newBinding,
            for: command,
            overrides: model.settings.shortcutOverrides
        )
        if conflicts.isEmpty == false {
            conflictState = HCBShortcutConflictState(
                proposedBinding: newBinding,
                targetCommand: command,
                conflictingCommands: conflicts
            )
        } else {
            conflictState = nil
            model.setShortcutBinding(command, binding: newBinding)
        }
    }

    private func showConflictingCommand(_ command: HCBShortcutCommand) {
        selectedGroup = command.group
        query = command.title
    }

    private func resetConflictingShortcut(_ command: HCBShortcutCommand, for state: HCBShortcutConflictState) {
        guard model.settings.shortcutOverrides[command.rawValue] != nil else { return }
        model.setShortcutBinding(command, binding: nil)
        let remaining = hcbConflictingCommands(
            proposed: state.proposedBinding,
            for: state.targetCommand,
            overrides: model.settings.shortcutOverrides
        )
        conflictState = remaining.isEmpty ? nil : HCBShortcutConflictState(
            proposedBinding: state.proposedBinding,
            targetCommand: state.targetCommand,
            conflictingCommands: remaining
        )
    }
}

private extension HCBShortcutGroup {
    var systemImage: String {
        switch self {
        case .app: "app.dashed"
        case .navigation: "arrow.left.arrow.right"
        case .store: "checklist"
        case .calendar: "calendar"
        case .taskInspector: "sidebar.right"
        }
    }
}

// NSView-based key recorder. Becomes first responder on appear and reports
// the next keyDown as an HCBKeyBinding (or nil on Escape / invalid combos).
struct KeyRecorderView: NSViewRepresentable {
    let onCapture: (HCBKeyBinding?) -> Void

    func makeNSView(context: Context) -> KeyRecorderNSView {
        let view = KeyRecorderNSView()
        view.onCapture = onCapture
        return view
    }

    func updateNSView(_ nsView: KeyRecorderNSView, context: Context) {}
}

final class KeyRecorderNSView: NSView {
    var onCapture: ((HCBKeyBinding?) -> Void)?

    override var acceptsFirstResponder: Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        DispatchQueue.main.async { [weak self] in
            self?.window?.makeFirstResponder(self)
        }
    }

    override func keyDown(with event: NSEvent) {
        if event.keyCode == 53 { // Escape
            onCapture?(nil)
            return
        }
        guard let binding = Self.binding(from: event) else {
            NSSound.beep()
            return
        }
        onCapture?(binding)
    }

    static func binding(from event: NSEvent) -> HCBKeyBinding? {
        var mods: HCBModifierSet = []
        if event.modifierFlags.contains(.command) { mods.insert(.command) }
        if event.modifierFlags.contains(.shift) { mods.insert(.shift) }
        if event.modifierFlags.contains(.option) { mods.insert(.option) }
        if event.modifierFlags.contains(.control) { mods.insert(.control) }

        let key: HCBKey
        switch event.keyCode {
        case 36: key = .returnKey // Return
        case 76: key = .returnKey // Enter (numpad)
        case 51: key = .delete
        case 117: key = .delete // forward delete
        case 48: key = .tab
        case 49: key = .space
        case 123: key = .leftArrow
        case 124: key = .rightArrow
        case 125: key = .downArrow
        case 126: key = .upArrow
        default:
            // Require a letter/number/punctuation character AND at least one
            // modifier (otherwise the user can't type normal text anywhere).
            guard mods.isEmpty == false else { return nil }
            guard let chars = event.charactersIgnoringModifiers, chars.isEmpty == false else { return nil }
            key = .char(String(chars.lowercased().first!))
        }
        return HCBKeyBinding(key: key, modifiers: mods)
    }
}
