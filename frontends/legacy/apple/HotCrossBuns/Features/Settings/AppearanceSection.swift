import SwiftUI
import UniformTypeIdentifiers

struct AppearanceSection: View {
    @Environment(AppModel.self) private var model
    @Environment(\.colorScheme) private var systemColorScheme
    @AppStorage(HCBBaseColorSchemePreference.storageKey) private var baseColorSchemePreference: String = ""
    @State private var fontQuery: String = ""
    @State private var availableFonts: [String] = []
    @State private var editingCustomScheme: HCBCustomColorScheme?
    @State private var exportedScheme: HCBColorSchemeDocument?
    @State private var isImportingScheme = false
    @State private var isImportingBackground = false
    @State private var backgroundImportError: String?
    @State private var derivedBackgroundScheme: HCBCustomColorScheme?

    var body: some View {
        Section("Appearance") {
            colourPanel
            performanceRow
            backgroundPanel
            motionRow
            layoutScaleRow
            textSizeRow
            fontRow
            Text("Layout scale resizes UI chrome (sidebar, icons, padding). Text size, font, and color scheme are controlled independently. System dialogs (alerts, confirmation dialogs) follow macOS display settings and aren't affected.")
                .hcbFont(.footnote)
                .foregroundStyle(.secondary)
        }
        .task {
            if availableFonts.isEmpty {
                availableFonts = HCBInstalledFonts.available()
            }
            coerceThemeToBaseColourScheme()
        }
        .sheet(item: $editingCustomScheme) { scheme in
            CustomColorSchemeEditor(draft: scheme) { updated in
                saveCustomScheme(updated)
            }
            .withHCBAppearance(model.settings)
            .hcbPreferredColorScheme(model.settings)
        }
        .sheet(item: $derivedBackgroundScheme) { scheme in
            DerivedBackgroundThemeSheet(
                scheme: scheme,
                onSkip: {
                    derivedBackgroundScheme = nil
                },
                onSave: {
                    saveCustomScheme(scheme)
                    derivedBackgroundScheme = nil
                },
                onTweak: {
                    derivedBackgroundScheme = nil
                    Task { @MainActor in
                        await Task.yield()
                        editingCustomScheme = scheme
                    }
                }
            )
            .withHCBAppearance(model.settings)
            .hcbPreferredColorScheme(model.settings)
        }
        .fileExporter(
            isPresented: Binding(
                get: { exportedScheme != nil },
                set: { if $0 == false { exportedScheme = nil } }
            ),
            document: exportedScheme ?? HCBColorSchemeDocument(scheme: HCBCustomColorScheme(copying: .notion)),
            contentType: .json,
            defaultFilename: exportedScheme?.scheme.title.replacingOccurrences(of: " ", with: "-").lowercased()
        ) { _ in
            exportedScheme = nil
        }
        .fileImporter(isPresented: $isImportingScheme, allowedContentTypes: [.json]) { result in
            guard case .success(let url) = result else { return }
            let shouldStopAccess = url.startAccessingSecurityScopedResource()
            defer {
                if shouldStopAccess {
                    url.stopAccessingSecurityScopedResource()
                }
            }
            if let data = try? Data(contentsOf: url),
               let scheme = try? JSONDecoder().decode(HCBCustomColorScheme.self, from: data) {
                model.upsertCustomColorScheme(scheme)
            }
        }
        .fileImporter(isPresented: $isImportingBackground, allowedContentTypes: [.image]) { result in
            guard case .success(let url) = result else { return }
            do {
                try model.importCustomBackground(from: url)
                backgroundImportError = nil
                deriveThemeFromCurrentBackground()
            } catch {
                backgroundImportError = error.localizedDescription
            }
        }
        .onChange(of: effectiveThemeIsDark) { _, _ in
            coerceThemeToBaseColourScheme()
        }
    }

    private var colourPanel: some View {
        VStack(spacing: 0) {
            baseColourSchemeRow
            Divider()
                .hcbScaledPadding(.vertical, 10)
            themeRow
            Divider()
                .hcbScaledPadding(.vertical, 10)
            customThemeRow
        }
    }

    private var backgroundPanel: some View {
        VStack(alignment: .leading, spacing: 10) {
            Divider()
                .hcbScaledPadding(.vertical, 10)
            HStack(alignment: .center, spacing: 18) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Background")
                        .hcbFont(.body, weight: .semibold)
                    Text("Make the window translucent or place a local image behind app surfaces.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 16)
                Toggle("Translucent", isOn: Binding(
                    get: { model.settings.appBackgroundTranslucencyEnabled },
                    set: { model.setAppBackgroundTranslucencyEnabled($0) }
                ))
                .toggleStyle(.switch)
            }

            if model.settings.appBackgroundTranslucencyEnabled || model.settings.customBackgroundImagePath != nil {
                VStack(alignment: .leading, spacing: 6) {
                    // Presets were removed so opacity has one direct control instead of two competing controls.
                    HStack {
                        Text("App surface opacity")
                        Spacer()
                        Text("\(Int(model.settings.appBackgroundOpacity * 100))%")
                            .foregroundStyle(.secondary)
                            .monospacedDigit()
                    }
                    Slider(
                        value: Binding(
                            get: { model.settings.appBackgroundOpacity },
                            set: { model.setAppBackgroundOpacity($0) }
                        ),
                        in: 0.35...1.0,
                        step: 0.05
                    )
                }
            }

            HStack(spacing: 8) {
                Button {
                    isImportingBackground = true
                } label: {
                    Label("Choose Image", systemImage: "photo")
                }

                if model.settings.customBackgroundImagePath != nil {
                    Button {
                        deriveThemeFromCurrentBackground()
                    } label: {
                        Label("Derive Theme", systemImage: "paintpalette")
                    }

                    Button("Remove Image", role: .destructive) {
                        model.clearCustomBackground()
                    }
                }
            }

            if let path = model.settings.customBackgroundImagePath {
                Text(URL(filePath: path).lastPathComponent)
                    .hcbFont(.caption2)
                    .foregroundStyle(.secondary)
                    .lineLimit(1)
            }

            if let backgroundImportError {
                Text(backgroundImportError)
                    .hcbFont(.caption2)
                    .foregroundStyle(AppColor.ember)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private var performanceRow: some View {
        VStack(spacing: 0) {
            Divider()
                .hcbScaledPadding(.vertical, 10)
            HStack(alignment: .center, spacing: 18) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Performance")
                        .hcbFont(.body, weight: .semibold)
                    Text(model.settings.performanceMode.summary)
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 16)
                Picker("Performance", selection: Binding(
                    get: { model.settings.performanceMode },
                    set: { mode in
                        var next = model.settings
                        next.performanceMode = mode
                        model.updateSettings(next)
                    }
                )) {
                    ForEach(AppPerformanceMode.allCases) { mode in
                        Text(mode.title).tag(mode)
                    }
                }
                .labelsHidden()
                .pickerStyle(.segmented)
                .frame(width: 190)
            }
        }
    }

    private var motionRow: some View {
        VStack(spacing: 0) {
            Divider()
                .hcbScaledPadding(.vertical, 10)
            HStack(alignment: .center, spacing: 18) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Animations")
                        .hcbFont(.body, weight: .semibold)
                    Text("Turn off app-driven transitions, panel motion, loading animation, and animated state changes.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 16)
                Toggle("Disable animations", isOn: Binding(
                    get: { model.settings.disableAnimations },
                    set: { isDisabled in
                        var next = model.settings
                        next.disableAnimations = isDisabled
                        model.updateSettings(next)
                    }
                ))
                .toggleStyle(.switch)
            }
        }
    }

    private var baseColourSchemeRow: some View {
        HStack(alignment: .center, spacing: 18) {
            VStack(alignment: .leading, spacing: 4) {
                Text("Base colour scheme")
                    .hcbFont(.body, weight: .semibold)
                Text("Choose whether app chrome resolves as dark, light, or follows macOS.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 16)
            Picker("Base colour scheme", selection: baseColourSchemeBinding) {
                ForEach(HCBBaseColorSchemePreference.allCases) { preference in
                    Text(preference.title).tag(preference)
                }
            }
            .labelsHidden()
            .pickerStyle(.menu)
            .frame(width: 170)
        }
    }

    private var themeRow: some View {
        HStack(alignment: .center, spacing: 18) {
            VStack(alignment: .leading, spacing: 4) {
                Text("Themes")
                    .hcbFont(.body, weight: .semibold)
                Text("Choose the Hot Cross Buns palette used by cards, text, and app surfaces.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 16)
            HStack(spacing: 8) {
                Button("Reset to \(defaultTheme.title)") { model.setColorSchemeID(defaultTheme.id) }
                    .buttonStyle(.borderless)
                    .hcbFont(.caption)
                    .disabled(model.settings.colorSchemeID == defaultTheme.id)
                Picker("Theme", selection: Binding(
                    get: { model.settings.colorSchemeID },
                    set: { newID in
                        guard filteredThemes.contains(where: { $0.id == newID }) else { return }
                        model.setColorSchemeID(newID)
                    }
                )) {
                    ForEach(filteredThemes) { scheme in
                        HStack(spacing: 8) {
                            ColorSchemeSwatch(scheme: scheme)
                            Text(scheme.title)
                        }
                        .tag(scheme.id)
                    }
                }
                .pickerStyle(.menu)
                .frame(width: 190)
            }
        }
    }

    private var customThemeRow: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .center, spacing: 18) {
                VStack(alignment: .leading, spacing: 4) {
                    Text("Custom themes")
                        .hcbFont(.body, weight: .semibold)
                    Text("Create palettes with the editor, import a JSON theme, or export one to share.")
                        .hcbFont(.footnote)
                        .foregroundStyle(.secondary)
                }
                Spacer(minLength: 16)
                Button("Import") { isImportingScheme = true }
                Button("New from current") {
                    editingCustomScheme = model.duplicateCurrentColorSchemeForEditing()
                }
            }

            if model.settings.customColorSchemes.isEmpty {
                Text("No custom themes yet.")
                    .hcbFont(.caption)
                    .foregroundStyle(.secondary)
            } else {
                ForEach(model.settings.customColorSchemes) { custom in
                    HStack(spacing: 10) {
                        ColorSchemeSwatch(scheme: custom.colorScheme())
                        VStack(alignment: .leading, spacing: 2) {
                            Text(custom.title)
                            Text(custom.isDark ? "Dark" : "Light")
                                .hcbFont(.caption2)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                        Button("Edit") { editingCustomScheme = custom }
                        Button("Export") { exportedScheme = HCBColorSchemeDocument(scheme: custom) }
                        Button("Delete", role: .destructive) { model.deleteCustomColorScheme(id: custom.id) }
                    }
                }
            }
        }
    }

    private var baseColourSchemeBinding: Binding<HCBBaseColorSchemePreference> {
        Binding(
            get: {
                HCBBaseColorSchemePreference(rawValue: baseColorSchemePreference) ?? HCBBaseColorSchemePreference.fallback(for: model.settings)
            },
            set: { preference in
                baseColorSchemePreference = preference.rawValue
                coerceThemeToBaseColourScheme(for: preference)
            }
        )
    }

    private var resolvedBaseColourScheme: HCBBaseColorSchemePreference {
        HCBBaseColorSchemePreference(rawValue: baseColorSchemePreference) ?? HCBBaseColorSchemePreference.fallback(for: model.settings)
    }

    private var effectiveThemeIsDark: Bool {
        switch resolvedBaseColourScheme {
        case .dark:
            true
        case .light:
            false
        case .system:
            systemColorScheme == .dark
        }
    }

    private var filteredThemes: [HCBColorScheme] {
        HCBColorScheme.all(including: model.settings.customColorSchemes).filter { $0.isDark == effectiveThemeIsDark }
    }

    private var defaultTheme: HCBColorScheme {
        filteredThemes.first ?? .notion
    }

    private func coerceThemeToBaseColourScheme(for preference: HCBBaseColorSchemePreference? = nil) {
        let targetIsDark: Bool
        switch preference ?? resolvedBaseColourScheme {
        case .dark:
            targetIsDark = true
        case .light:
            targetIsDark = false
        case .system:
            targetIsDark = systemColorScheme == .dark
        }

        guard HCBColorScheme.scheme(id: model.settings.colorSchemeID, customSchemes: model.settings.customColorSchemes)?.isDark != targetIsDark else { return }
        let replacement = HCBColorScheme.all.first { $0.isDark == targetIsDark } ?? HCBColorScheme.notion
        model.setColorSchemeID(replacement.id)
    }

    private func deriveThemeFromCurrentBackground() {
        guard let path = model.settings.customBackgroundImagePath else { return }
        let url = URL(filePath: path)
        let title = "\(url.deletingPathExtension().lastPathComponent) theme"
        guard let scheme = BackgroundImageColorSchemeDeriver.derive(from: url, suggestedTitle: title) else {
            backgroundImportError = "Could not derive a color scheme from that image."
            return
        }
        backgroundImportError = nil
        derivedBackgroundScheme = scheme
    }

    private func saveCustomScheme(_ scheme: HCBCustomColorScheme) {
        baseColorSchemePreference = scheme.isDark ? HCBBaseColorSchemePreference.dark.rawValue : HCBBaseColorSchemePreference.light.rawValue
        model.upsertCustomColorScheme(scheme)
    }

    private var layoutScaleRow: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("Layout scale")
                Spacer()
                Text("\(Int(model.settings.uiLayoutScale * 100))%")
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
            }
            Slider(
                value: Binding(
                    get: { model.settings.uiLayoutScale },
                    set: { model.setUILayoutScale($0) }
                ),
                in: 0.80...1.50,
                step: 0.05
            )
            HStack {
                Button("Reset") { model.setUILayoutScale(1.0) }
                    .buttonStyle(.borderless)
                    .hcbFont(.caption)
                Spacer()
            }
        }
    }

    private var textSizeRow: some View {
        Stepper(
            value: Binding(
                get: { model.settings.uiTextSizePoints },
                set: { model.setUITextSizePoints($0) }
            ),
            in: HCBTextSize.minPoints...HCBTextSize.maxPoints,
            step: HCBTextSize.stepPoints
        ) {
            HStack {
                Text("Text size")
                Spacer()
                Text("\(Int(model.settings.uiTextSizePoints)) pt")
                    .foregroundStyle(.secondary)
                    .monospacedDigit()
                if model.settings.uiTextSizePoints != HCBTextSize.defaultPoints {
                    Button("Reset") { model.setUITextSizePoints(HCBTextSize.defaultPoints) }
                        .buttonStyle(.borderless)
                        .hcbFont(.caption)
                }
            }
        }
    }

    private var fontRow: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack {
                Text("UI font")
                Spacer()
                Button("System default") { model.setUIFontName(nil) }
                    .buttonStyle(.borderless)
                    .hcbFont(.caption)
                    .disabled(model.settings.uiFontName == nil)
            }
            HStack(spacing: 8) {
                TextField("Search fonts", text: $fontQuery)
                    .textFieldStyle(.roundedBorder)
                Menu {
                    if filteredFonts.isEmpty {
                        Text("No fonts match").foregroundStyle(.secondary)
                    } else {
                        ForEach(filteredFonts.prefix(200), id: \.self) { family in
                            Button(family) { model.setUIFontName(family) }
                        }
                        if filteredFonts.count > 200 {
                            Text("Showing first 200 — refine the search").foregroundStyle(.secondary)
                        }
                    }
                } label: {
                    Label(model.settings.uiFontName ?? "System", systemImage: "textformat")
                        .lineLimit(1)
                }
                .menuStyle(.borderlessButton)
                .fixedSize()
            }
            Text("Custom font applies to all text rendered via the app's semantic text styles. Calls that use system design variants (monospaced digits, rounded, serif) keep the system font on purpose.")
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
        }
    }

    private var filteredFonts: [String] {
        let q = fontQuery.trimmingCharacters(in: .whitespaces)
        guard q.isEmpty == false else { return availableFonts }
        return availableFonts.filter { $0.localizedCaseInsensitiveContains(q) }
    }
}

private struct ColorSchemeSwatch: View {
    let scheme: HCBColorScheme

    var body: some View {
        ZStack {
            Circle().fill(scheme.cream.swiftColor)
            Circle()
                .fill(scheme.ember.swiftColor)
                .frame(width: 8, height: 8)
                .offset(x: -3, y: -3)
            Circle()
                .fill(scheme.blue.swiftColor)
                .frame(width: 8, height: 8)
                .offset(x: 3, y: 3)
        }
        .frame(width: 22, height: 22)
        .overlay(Circle().strokeBorder(scheme.cardStroke.swiftColor, lineWidth: 0.5))
    }
}

private struct DerivedBackgroundThemeSheet: View {
    let scheme: HCBCustomColorScheme
    var onSkip: () -> Void
    var onSave: () -> Void
    var onTweak: () -> Void

    private var colorScheme: HCBColorScheme {
        scheme.colorScheme()
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 6) {
                Text("Create a theme from this image?")
                    .hcbFont(.title3, weight: .semibold)
                Text("Hot Cross Buns derived a custom palette from the selected background. You can save it now or tweak the values first.")
                    .hcbFont(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }

            VStack(alignment: .leading, spacing: 10) {
                HStack(spacing: 10) {
                    ColorSchemeSwatch(scheme: colorScheme)
                    VStack(alignment: .leading, spacing: 2) {
                        Text(scheme.title)
                            .hcbFont(.body, weight: .semibold)
                        Text(scheme.isDark ? "Dark theme" : "Light theme")
                            .hcbFont(.caption)
                            .foregroundStyle(.secondary)
                    }
                }

                HStack(spacing: 8) {
                    paletteChip("Accent", scheme.emberHex)
                    paletteChip("Success", scheme.mossHex)
                    paletteChip("Link", scheme.blueHex)
                    paletteChip("Text", scheme.inkHex)
                    paletteChip("Surface", scheme.creamHex)
                    paletteChip("Border", scheme.cardStrokeHex)
                }
            }
            .hcbScaledPadding(14)
            .background(
                RoundedRectangle(cornerRadius: 12)
                    .fill(colorScheme.cardSurface.swiftColor)
            )
            .overlay(
                RoundedRectangle(cornerRadius: 12)
                    .strokeBorder(colorScheme.cardStroke.swiftColor, lineWidth: 1)
            )

            HStack {
                Button("Not Now") { onSkip() }
                Spacer()
                Button("Tweak & Save") { onTweak() }
                Button("Save Theme") { onSave() }
                    .keyboardShortcut(.defaultAction)
                    .buttonStyle(.borderedProminent)
            }
        }
        .hcbScaledPadding(20)
        .frame(width: 560)
    }

    private func paletteChip(_ title: String, _ hex: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            RoundedRectangle(cornerRadius: 4)
                .fill(Color(hex: hex))
                .frame(height: 18)
                .overlay(
                    RoundedRectangle(cornerRadius: 4)
                        .strokeBorder(colorScheme.cardStroke.swiftColor.opacity(0.7), lineWidth: 0.6)
                )
            Text(title)
                .hcbFont(.caption2, weight: .medium)
                .foregroundStyle(colorScheme.ink.swiftColor)
                .lineLimit(1)
            Text(hex)
                .hcbFont(.caption2)
                .foregroundStyle(.secondary)
                .monospaced()
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
