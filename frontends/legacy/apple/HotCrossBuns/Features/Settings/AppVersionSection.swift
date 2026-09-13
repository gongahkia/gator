import AppKit
import SwiftUI

struct AppVersionSection: View {
    private let info: AppVersionInfo
    @State private var didCopy = false

    init(info: AppVersionInfo = .current) {
        self.info = info
    }

    var body: some View {
        Section("About") {
            LabeledContent("App", value: info.appName)
            LabeledContent("Version", value: info.version)
            LabeledContent("Build", value: info.build)
            LabeledContent("Bundle ID", value: info.bundleIdentifier)

            Button {
                copySupportSummary()
            } label: {
                Label(didCopy ? "Copied version info" : "Copy version info", systemImage: didCopy ? "checkmark" : "doc.on.doc")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(.plain)
        }
    }

    private func copySupportSummary() {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()
        pasteboard.setString(info.supportSummary, forType: .string)
        didCopy = true
    }
}

struct AppVersionInfo: Equatable {
    var appName: String
    var version: String
    var build: String
    var bundleIdentifier: String

    static var current: AppVersionInfo {
        let bundle = Bundle.main
        return AppVersionInfo(
            appName: bundle.object(forInfoDictionaryKey: "CFBundleDisplayName") as? String
                ?? bundle.object(forInfoDictionaryKey: "CFBundleName") as? String
                ?? "Hot Cross Buns",
            version: bundle.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "Unknown",
            build: bundle.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "Unknown",
            bundleIdentifier: bundle.bundleIdentifier ?? "Unknown"
        )
    }

    var supportSummary: String {
        "\(appName) \(version) (\(build)); \(bundleIdentifier)"
    }
}

#Preview {
    Form {
        AppVersionSection(
            info: AppVersionInfo(
                appName: "Hot Cross Buns",
                version: "0.2.5",
                build: "8",
                bundleIdentifier: "com.gongahkia.hotcrossbuns.mac"
            )
        )
    }
    .formStyle(.grouped)
}
