import AppKit
import SwiftUI
import UniformTypeIdentifiers

struct CustomColorSchemeEditor: View {
    @Environment(\.dismiss) private var dismiss
    @State var draft: HCBCustomColorScheme
    var onSave: (HCBCustomColorScheme) -> Void

    var body: some View {
        Form {
            Section("Details") {
                TextField("Name", text: $draft.title)
                Toggle("Dark theme", isOn: $draft.isDark)
            }

            Section("Palette") {
                palettePicker("Accent", keyPath: \.emberHex)
                palettePicker("Success", keyPath: \.mossHex)
                palettePicker("Link", keyPath: \.blueHex)
                palettePicker("Text", keyPath: \.inkHex)
                palettePicker("Background", keyPath: \.creamHex)
                palettePicker("Borders", keyPath: \.cardStrokeHex)
            }
        }
        .formStyle(.grouped)
        .frame(minWidth: 420, minHeight: 430)
        .toolbar {
            ToolbarItem(placement: .cancellationAction) {
                Button("Cancel") { dismiss() }
            }
            ToolbarItem(placement: .confirmationAction) {
                Button("Save") {
                    onSave(draft)
                    dismiss()
                }
                .keyboardShortcut(.defaultAction)
            }
        }
    }

    private func palettePicker(_ title: String, keyPath: WritableKeyPath<HCBCustomColorScheme, String>) -> some View {
        HStack {
            ColorPicker(
                title,
                selection: Binding(
                    get: { Color(hex: draft[keyPath: keyPath]) },
                    set: { draft[keyPath: keyPath] = $0.hcbHexString }
                ),
                supportsOpacity: false
            )
            Text(draft[keyPath: keyPath])
                .foregroundStyle(.secondary)
                .monospaced()
        }
    }
}

struct HCBColorSchemeDocument: FileDocument {
    static var readableContentTypes: [UTType] { [.json] }

    var scheme: HCBCustomColorScheme

    init(scheme: HCBCustomColorScheme) {
        self.scheme = scheme
    }

    init(configuration: ReadConfiguration) throws {
        guard let data = configuration.file.regularFileContents else {
            throw CocoaError(.fileReadCorruptFile)
        }
        let decoder = JSONDecoder()
        scheme = try decoder.decode(HCBCustomColorScheme.self, from: data)
    }

    func fileWrapper(configuration: WriteConfiguration) throws -> FileWrapper {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.prettyPrinted, .sortedKeys]
        let data = try encoder.encode(scheme)
        return FileWrapper(regularFileWithContents: data)
    }
}

private extension Color {
    var hcbHexString: String {
        let color = NSColor(self).usingColorSpace(.sRGB) ?? .controlAccentColor
        let r = Int((max(0, min(1, color.redComponent)) * 255).rounded())
        let g = Int((max(0, min(1, color.greenComponent)) * 255).rounded())
        let b = Int((max(0, min(1, color.blueComponent)) * 255).rounded())
        return String(format: "#%02X%02X%02X", r, g, b)
    }
}
