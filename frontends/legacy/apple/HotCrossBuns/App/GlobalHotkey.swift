import AppKit
import ApplicationServices
import Carbon.HIToolbox

enum GlobalHotkeyRegistrationState: Equatable {
    case disabled
    case ready(String)
    case needsAccessibilityPermission
    case failed(String)

    var message: String {
        switch self {
        case .disabled:
            "The global quick-add hotkey is off."
        case .ready(let label):
            "Ready on this Mac: \(label)"
        case .needsAccessibilityPermission:
            "Accessibility permission is required before the global quick-add hotkey can work."
        case .failed(let message):
            message
        }
    }
}

enum GlobalHotkeyRegistrationError: LocalizedError, Equatable {
    case alreadyInUse
    case system(OSStatus)

    var errorDescription: String? {
        switch self {
        case .alreadyInUse:
            "That shortcut is already in use by macOS or another app."
        case .system(let status):
            "macOS could not register that global hotkey (status \(status))."
        }
    }

    static func from(status: OSStatus) -> GlobalHotkeyRegistrationError {
        if status == eventHotKeyExistsErr {
            return .alreadyInUse
        }
        return .system(status)
    }
}

enum GlobalHotkeyAccessibilityPermission {
    static func isTrusted(prompt: Bool = false) -> Bool {
        let options = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: prompt] as CFDictionary
        return AXIsProcessTrustedWithOptions(options)
    }
}

@MainActor
final class GlobalHotkey {
    private var hotKeyRef: EventHotKeyRef?
    private var eventHandlerRef: EventHandlerRef?
    private static let hotKeyID: UInt32 = 0x48434221 // 'HCB!'

    var action: (() -> Void)?
    var isInstalled: Bool { hotKeyRef != nil }

    func install(binding: GlobalHotkeyBinding) throws {
        uninstall()
        var spec = EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed))
        let selfPointer = Unmanaged.passUnretained(self).toOpaque()
        if eventHandlerRef == nil {
            let status = InstallEventHandler(GetApplicationEventTarget(), { _, event, userData in
                guard let event, let userData else { return noErr }
                var hkID = EventHotKeyID()
                GetEventParameter(
                    event,
                    EventParamName(kEventParamDirectObject),
                    EventParamType(typeEventHotKeyID),
                    nil,
                    MemoryLayout<EventHotKeyID>.size,
                    nil,
                    &hkID
                )
                let center = Unmanaged<GlobalHotkey>.fromOpaque(userData).takeUnretainedValue()
                DispatchQueue.main.async { center.action?() }
                return noErr
            }, 1, &spec, selfPointer, &eventHandlerRef)
            guard status == noErr else {
                throw GlobalHotkeyRegistrationError.from(status: status)
            }
        }

        let hotKeyIDStruct = EventHotKeyID(signature: Self.hotKeyID, id: 1)
        let status = RegisterEventHotKey(
            binding.keyCode,
            binding.carbonModifiers,
            hotKeyIDStruct,
            GetApplicationEventTarget(),
            0,
            &hotKeyRef
        )
        guard status == noErr else {
            hotKeyRef = nil
            throw GlobalHotkeyRegistrationError.from(status: status)
        }
    }

    func uninstall() {
        if let ref = hotKeyRef {
            UnregisterEventHotKey(ref)
            hotKeyRef = nil
        }
        if let handler = eventHandlerRef {
            RemoveEventHandler(handler)
            eventHandlerRef = nil
        }
    }

    deinit {
        if let ref = hotKeyRef {
            UnregisterEventHotKey(ref)
        }
        if let handler = eventHandlerRef {
            RemoveEventHandler(handler)
        }
    }
}
