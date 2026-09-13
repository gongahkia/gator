import SwiftUI

struct GlobalHotkeyConfigurator {
    var configure: @MainActor (Bool, GlobalHotkeyBinding, AppModel) -> GlobalHotkeyRegistrationState

    @MainActor
    func callAsFunction(
        enabled: Bool,
        binding: GlobalHotkeyBinding,
        model: AppModel
    ) -> GlobalHotkeyRegistrationState {
        configure(enabled, binding, model)
    }
}

private struct GlobalHotkeyConfiguratorKey: EnvironmentKey {
    static let defaultValue = GlobalHotkeyConfigurator { _, _, _ in
        .failed("Hotkey registration is unavailable right now.")
    }
}

extension EnvironmentValues {
    var globalHotkeyConfigurator: GlobalHotkeyConfigurator {
        get { self[GlobalHotkeyConfiguratorKey.self] }
        set { self[GlobalHotkeyConfiguratorKey.self] = newValue }
    }
}
