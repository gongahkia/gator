import Foundation
import UniformTypeIdentifiers

enum CompletionSoundLibrary {
    static let builtInSoundNames: [String] = [
        "Glass",
        "Pop",
        "Ping",
        "Tink",
        "Hero",
        "Bottle",
        "Frog",
        "Funk",
        "Basso",
        "Blow",
        "Purr",
        "Submarine",
        "Sosumi",
        "Morse"
    ]

    static var supportedAudioTypes: [UTType] {
        [
            .audio,
            .mp3,
            .wav,
            .aiff,
            .midi,
            .mpeg4Audio
        ]
    }

    private static let supportedExtensions: Set<String> = [
        "aif",
        "aiff",
        "aifc",
        "wav",
        "caf",
        "m4a",
        "mp3"
    ]

    static func importSound(from sourceURL: URL) throws -> CompletionSoundAsset {
        guard let directoryURL = soundsDirectoryURL() else {
            throw CompletionSoundLibraryError.directoryUnavailable
        }

        let ext = sourceURL.pathExtension.lowercased()
        guard supportedExtensions.contains(ext) else {
            throw CompletionSoundLibraryError.unsupportedType
        }

        let accessingScopedResource = sourceURL.startAccessingSecurityScopedResource()
        defer {
            if accessingScopedResource {
                sourceURL.stopAccessingSecurityScopedResource()
            }
        }

        try FileManager.default.createDirectory(
            at: directoryURL,
            withIntermediateDirectories: true
        )

        let id = UUID()
        let filename = "\(id.uuidString).\(ext)"
        let destinationURL = directoryURL.appending(path: filename)
        if FileManager.default.fileExists(atPath: destinationURL.path) {
            try FileManager.default.removeItem(at: destinationURL)
        }
        try FileManager.default.copyItem(at: sourceURL, to: destinationURL)

        let baseName = sourceURL.deletingPathExtension().lastPathComponent
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let displayName = baseName.isEmpty ? "Imported Sound" : baseName
        return CompletionSoundAsset(
            id: id,
            displayName: displayName,
            storedFilename: filename,
            importedAt: Date()
        )
    }

    static func delete(_ asset: CompletionSoundAsset) {
        guard let url = url(for: asset) else { return }
        try? FileManager.default.removeItem(at: url)
    }

    static func url(for asset: CompletionSoundAsset) -> URL? {
        soundsDirectoryURL()?.appending(path: asset.storedFilename)
    }

    private static func soundsDirectoryURL() -> URL? {
        guard let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first else {
            return nil
        }
        let bundleID = Bundle.main.bundleIdentifier ?? "com.gongahkia.hotcrossbuns.mac"
        return base.appending(path: bundleID).appending(path: "CompletionSounds")
    }
}

enum CompletionSoundLibraryError: LocalizedError {
    case directoryUnavailable
    case unsupportedType

    var errorDescription: String? {
        switch self {
        case .directoryUnavailable:
            "Hot Cross Buns couldn't create its local sound library."
        case .unsupportedType:
            "Choose an AIFF, WAV, CAF, M4A, or MP3 sound file."
        }
    }
}
