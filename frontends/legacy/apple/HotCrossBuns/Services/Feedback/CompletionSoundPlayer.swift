import AppKit

enum CompletionSoundKind: Sendable {
    case taskCompleted
    case eventDismissed

    fileprivate var fallbackChoice: CompletionSoundChoice {
        switch self {
        case .taskCompleted:
            .defaultTask
        case .eventDismissed:
            .defaultEvent
        }
    }

    fileprivate func isEnabled(in settings: AppSettings) -> Bool {
        switch self {
        case .taskCompleted:
            settings.enableTaskCompletionSound
        case .eventDismissed:
            settings.enableEventCompletionSound
        }
    }

    fileprivate func configuredChoice(in settings: AppSettings) -> CompletionSoundChoice {
        switch self {
        case .taskCompleted:
            settings.taskCompletionSoundChoice
        case .eventDismissed:
            settings.eventCompletionSoundChoice
        }
    }
}

@MainActor
enum CompletionSoundPlayer {
    private static var cachedNamedSounds: [String: NSSound] = [:]
    private static var cachedFileSounds: [String: NSSound] = [:]

    static func play(_ kind: CompletionSoundKind, settings: AppSettings) {
        guard kind.isEnabled(in: settings) else { return }
        let choice = kind.configuredChoice(in: settings)
        guard
            let sound = resolvedSound(for: choice, customAssets: settings.customCompletionSounds)
                ?? resolvedSound(for: kind.fallbackChoice, customAssets: settings.customCompletionSounds)
        else {
            AppLogger.debug("completion sound unavailable", category: .ui, metadata: [
                "kind": String(describing: kind)
            ])
            return
        }

        if sound.isPlaying {
            sound.stop()
        }

        let didPlay = sound.play()
        if didPlay == false {
            AppLogger.warn("completion sound failed to play", category: .ui, metadata: [
                "kind": String(describing: kind),
                "sound": sound.name ?? "unknown"
            ])
        }
    }

    static func preview(_ choice: CompletionSoundChoice, customAssets: [CompletionSoundAsset]) {
        guard let sound = resolvedSound(for: choice, customAssets: customAssets) else { return }
        if sound.isPlaying {
            sound.stop()
        }
        _ = sound.play()
    }

    private static func resolvedSound(
        for choice: CompletionSoundChoice,
        customAssets: [CompletionSoundAsset]
    ) -> NSSound? {
        switch choice.source {
        case .system:
            let name = choice.identifier
            if let cached = cachedNamedSounds[name] {
                return cached
            }
            if let sound = NSSound(named: NSSound.Name(name)) {
                cachedNamedSounds[name] = sound
                return sound
            }
        case .custom:
            guard
                let assetID = choice.customAssetID,
                let asset = customAssets.first(where: { $0.id == assetID }),
                let url = CompletionSoundLibrary.url(for: asset)
            else {
                return nil
            }
            if let cached = cachedFileSounds[url.path] {
                return cached
            }
            if let sound = NSSound(contentsOf: url, byReference: false) {
                cachedFileSounds[url.path] = sound
                return sound
            }
        }
        return nil
    }
}
