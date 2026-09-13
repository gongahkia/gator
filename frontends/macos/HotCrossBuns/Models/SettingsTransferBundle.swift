import Foundation

struct SettingsTransferBundle: Codable, Equatable, Sendable {
    static let currentFormatVersion = 1

    var formatVersion: Int
    var exportedAt: Date
    var appVersion: String
    var settings: AppSettings
    var excludedFields: [String]

    init(
        settings: AppSettings,
        exportedAt: Date = Date(),
        appVersion: String = Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "unknown"
    ) {
        self.formatVersion = Self.currentFormatVersion
        self.exportedAt = exportedAt
        self.appVersion = appVersion
        self.settings = settings
        self.excludedFields = [
            "Google account tokens",
            "cached Google Tasks and Calendar data",
            "cache encryption key or passphrase",
            "imported completion-sound audio files"
        ]
    }

    static func encode(_ bundle: SettingsTransferBundle) throws -> Data {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        encoder.dateEncodingStrategy = .iso8601
        return try encoder.encode(bundle)
    }

    static func decode(_ data: Data) throws -> SettingsTransferBundle {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        let bundle = try decoder.decode(SettingsTransferBundle.self, from: data)
        guard bundle.formatVersion == currentFormatVersion else {
            throw SettingsTransferError.unsupportedVersion(bundle.formatVersion)
        }
        return bundle
    }
}

struct SettingsImportPreview: Identifiable, Equatable, Sendable {
    let id = UUID()
    var changeCount: Int
    var summaries: [String]
    var excludedFields: [String]

    var message: String {
        var lines = summaries.isEmpty
            ? ["No settings changes detected."]
            : summaries
        if excludedFields.isEmpty == false {
            lines.append("Not imported: \(excludedFields.joined(separator: ", ")).")
        }
        return lines.joined(separator: "\n\n")
    }
}

enum SettingsTransferError: LocalizedError, Equatable {
    case unsupportedVersion(Int)

    var errorDescription: String? {
        switch self {
        case .unsupportedVersion(let version):
            return "This settings file uses format version \(version). This app supports version \(SettingsTransferBundle.currentFormatVersion)."
        }
    }
}
