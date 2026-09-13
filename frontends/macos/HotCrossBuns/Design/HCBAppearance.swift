import SwiftUI

enum HCBBaseColorSchemePreference: String, CaseIterable, Identifiable {
    static let storageKey = "hcb.baseColorSchemePreference"

    case dark
    case light
    case system

    var id: String { rawValue }

    var title: String {
        switch self {
        case .dark: "Dark"
        case .light: "Light"
        case .system: "Adapt to system"
        }
    }

    var preferredColorScheme: ColorScheme? {
        switch self {
        case .dark: .dark
        case .light: .light
        case .system: nil
        }
    }

    static func fallback(for settings: AppSettings) -> HCBBaseColorSchemePreference {
        fallback(colorSchemeID: settings.colorSchemeID, customSchemes: settings.customColorSchemes)
    }

    static func fallback(colorSchemeID: String, customSchemes: [HCBCustomColorScheme]) -> HCBBaseColorSchemePreference {
        HCBColorScheme.scheme(id: colorSchemeID, customSchemes: customSchemes)?.isDark == true ? .dark : .light
    }
}

enum HCBTextSize {
    static let minPoints: Double = 9
    static let maxPoints: Double = 24
    static let defaultPoints: Double = 13
    static let stepPoints: Double = 1

    static func clamp(_ points: Double) -> Double {
        max(minPoints, min(points, maxPoints))
    }

    static func dynamicTypeScale(for size: DynamicTypeSize) -> CGFloat {
        switch size {
        case .xSmall: 0.82
        case .small: 0.90
        case .medium: 0.96
        case .large: 1.00
        case .xLarge: 1.12
        case .xxLarge: 1.24
        case .xxxLarge: 1.36
        case .accessibility1: 1.52
        case .accessibility2: 1.68
        case .accessibility3: 1.84
        case .accessibility4: 2.00
        case .accessibility5: 2.18
        @unknown default: 1.00
        }
    }
}

enum HCBMotion {
    static func animation(_ animation: Animation?, reduceMotion: Bool) -> Animation? {
        reduceMotion ? nil : animation
    }

    static func transition(_ transition: AnyTransition, reduceMotion: Bool) -> AnyTransition {
        reduceMotion ? .opacity : transition
    }

    static func perform<Result>(
        reduceMotion: Bool,
        animation: Animation?,
        _ updates: () throws -> Result
    ) rethrows -> Result {
        if reduceMotion {
            return try updates()
        }
        return try withAnimation(animation, updates)
    }
}

private struct HCBLayoutScaleKey: EnvironmentKey {
    static let defaultValue: CGFloat = 1.0
}

private struct HCBFontFamilyKey: EnvironmentKey {
    static let defaultValue: String? = nil
}

struct HCBAppBackgroundConfiguration: Equatable {
    var isTranslucent: Bool
    var opacity: Double
    var customImagePath: String?

    static let standard = HCBAppBackgroundConfiguration(
        isTranslucent: false,
        opacity: 1.0,
        customImagePath: nil
    )

    init(isTranslucent: Bool, opacity: Double, customImagePath: String?) {
        self.isTranslucent = isTranslucent
        self.opacity = max(0.35, min(1.0, opacity))
        self.customImagePath = customImagePath
    }

    init(settings: AppSettings) {
        let usesRichPresentation = settings.performanceMode == .rich
        self.init(
            isTranslucent: usesRichPresentation && settings.appBackgroundTranslucencyEnabled,
            opacity: settings.appBackgroundOpacity,
            customImagePath: usesRichPresentation ? settings.customBackgroundImagePath : nil
        )
    }
}

private struct HCBAppBackgroundConfigurationKey: EnvironmentKey {
    static let defaultValue: HCBAppBackgroundConfiguration = .standard
}

extension EnvironmentValues {
    var hcbLayoutScale: CGFloat {
        get { self[HCBLayoutScaleKey.self] }
        set { self[HCBLayoutScaleKey.self] = newValue }
    }

    var hcbFontFamily: String? {
        get { self[HCBFontFamilyKey.self] }
        set { self[HCBFontFamilyKey.self] = newValue }
    }

    var hcbAppBackgroundConfiguration: HCBAppBackgroundConfiguration {
        get { self[HCBAppBackgroundConfigurationKey.self] }
        set { self[HCBAppBackgroundConfigurationKey.self] = newValue }
    }
}

private struct HCBTextSizePointsKey: EnvironmentKey {
    static let defaultValue: Double = HCBTextSize.defaultPoints
}

private struct HCBReduceMotionKey: EnvironmentKey {
    static let defaultValue = false
}

extension EnvironmentValues {
    var hcbTextSizePoints: Double {
        get { self[HCBTextSizePointsKey.self] }
        set { self[HCBTextSizePointsKey.self] = newValue }
    }

    var hcbReduceMotion: Bool {
        get { self[HCBReduceMotionKey.self] }
        set { self[HCBReduceMotionKey.self] = newValue }
    }
}

struct HCBAppearanceModifier: ViewModifier {
    @Environment(\.accessibilityReduceMotion) private var systemReduceMotion
    let layoutScale: CGFloat
    let textSizePoints: Double
    let fontName: String?
    let backgroundConfiguration: HCBAppBackgroundConfiguration
    let disableAnimations: Bool

    func body(content: Content) -> some View {
        content
            .environment(\.hcbLayoutScale, layoutScale)
            .environment(\.hcbFontFamily, fontName)
            .environment(\.hcbTextSizePoints, textSizePoints)
            .environment(\.hcbAppBackgroundConfiguration, backgroundConfiguration)
            .environment(\.hcbReduceMotion, systemReduceMotion || disableAnimations)
    }
}

struct HCBPreferredColorSchemeModifier: ViewModifier {
    let colorSchemeID: String
    let customColorSchemes: [HCBCustomColorScheme]
    @AppStorage(HCBBaseColorSchemePreference.storageKey) private var storedPreference: String = ""

    func body(content: Content) -> some View {
        content.preferredColorScheme(resolvedPreference.preferredColorScheme)
    }

    private var resolvedPreference: HCBBaseColorSchemePreference {
        HCBBaseColorSchemePreference(rawValue: storedPreference)
            ?? HCBBaseColorSchemePreference.fallback(colorSchemeID: colorSchemeID, customSchemes: customColorSchemes)
    }
}

extension View {
    // Applies the app-wide appearance (layout scale + text size + font family)
    // to a subtree. Use at the shell root AND at every out-of-tree presentation
    // site (sheets, popovers, menubar panels).
    func withHCBAppearance(_ settings: AppSettings) -> some View {
        modifier(HCBAppearanceModifier(
            layoutScale: CGFloat(settings.uiLayoutScale),
            textSizePoints: HCBTextSize.clamp(settings.uiTextSizePoints),
            fontName: settings.uiFontName,
            backgroundConfiguration: HCBAppBackgroundConfiguration(settings: settings),
            disableAnimations: settings.disableAnimations
        ))
    }

    func hcbPreferredColorScheme(_ settings: AppSettings) -> some View {
        modifier(HCBPreferredColorSchemeModifier(
            colorSchemeID: settings.colorSchemeID,
            customColorSchemes: settings.customColorSchemes
        ))
    }

    func withHCBAppearance(_ settings: ShellChromeSettings) -> some View {
        modifier(HCBAppearanceModifier(
            layoutScale: CGFloat(settings.uiLayoutScale),
            textSizePoints: HCBTextSize.clamp(settings.uiTextSizePoints),
            fontName: settings.uiFontName,
            backgroundConfiguration: HCBAppBackgroundConfiguration(
                isTranslucent: settings.performanceMode == .rich && settings.appBackgroundTranslucencyEnabled,
                opacity: settings.appBackgroundOpacity,
                customImagePath: settings.performanceMode == .rich ? settings.customBackgroundImagePath : nil
            ),
            disableAnimations: settings.disableAnimations
        ))
    }

    func hcbPreferredColorScheme(_ settings: ShellChromeSettings) -> some View {
        modifier(HCBPreferredColorSchemeModifier(
            colorSchemeID: settings.colorSchemeID,
            customColorSchemes: settings.customColorSchemes
        ))
    }

    func withHCBAppearance(_ settings: MenuBarSettingsSnapshot) -> some View {
        modifier(HCBAppearanceModifier(
            layoutScale: CGFloat(settings.uiLayoutScale),
            textSizePoints: HCBTextSize.clamp(settings.uiTextSizePoints),
            fontName: settings.uiFontName,
            backgroundConfiguration: HCBAppBackgroundConfiguration(
                isTranslucent: settings.performanceMode == .rich && settings.appBackgroundTranslucencyEnabled,
                opacity: settings.appBackgroundOpacity,
                customImagePath: settings.performanceMode == .rich ? settings.customBackgroundImagePath : nil
            ),
            disableAnimations: settings.disableAnimations
        ))
    }

    func hcbPreferredColorScheme(_ settings: MenuBarSettingsSnapshot) -> some View {
        modifier(HCBPreferredColorSchemeModifier(
            colorSchemeID: settings.colorSchemeID,
            customColorSchemes: settings.customColorSchemes
        ))
    }
}

// Layout helpers that multiply by the ambient hcbLayoutScale. Use these
// anywhere the app hard-codes a numeric padding / frame / icon size. Text
// sizes are NOT scaled — DynamicType + hcbFontFamily handle those.

extension View {
    func hcbScaledPadding(_ length: CGFloat) -> some View {
        modifier(HCBScaledPaddingModifier(length: length, edges: .all))
    }

    func hcbScaledPadding(_ edges: Edge.Set, _ length: CGFloat) -> some View {
        modifier(HCBScaledPaddingModifier(length: length, edges: edges))
    }

    func hcbScaledFrame(
        width: CGFloat? = nil,
        height: CGFloat? = nil,
        alignment: Alignment = .center
    ) -> some View {
        modifier(HCBScaledFrameModifier(width: width, height: height, alignment: alignment))
    }

    func hcbScaledFrame(
        minWidth: CGFloat? = nil,
        idealWidth: CGFloat? = nil,
        maxWidth: CGFloat? = nil,
        minHeight: CGFloat? = nil,
        idealHeight: CGFloat? = nil,
        maxHeight: CGFloat? = nil,
        alignment: Alignment = .center
    ) -> some View {
        modifier(HCBScaledFlexibleFrameModifier(
            minWidth: minWidth,
            idealWidth: idealWidth,
            maxWidth: maxWidth,
            minHeight: minHeight,
            idealHeight: idealHeight,
            maxHeight: maxHeight,
            alignment: alignment
        ))
    }

    // Applies a semantic text style that honors the custom UI font family.
    // If no custom family is set, falls back to SwiftUI's system font for
    // that style. DynamicType still scales text size independently.
    func hcbFont(_ style: HCBFontStyle) -> some View {
        modifier(HCBFontModifier(style: style, weight: nil))
    }

    // Weight-qualified variant — matches call sites like .font(.body.weight(.semibold)).
    func hcbFont(_ style: HCBFontStyle, weight: Font.Weight) -> some View {
        modifier(HCBFontModifier(style: style, weight: weight))
    }

    // Explicit-size call site that still honors the custom family. Size
    // stays fixed (no layoutScale) — text isn't part of the zoom.
    func hcbFontSystem(size: CGFloat, weight: Font.Weight = .regular, design: Font.Design = .default) -> some View {
        modifier(HCBFontSystemModifier(size: size, weight: weight, design: design))
    }

    func hcbMotionAnimation<Value: Equatable>(_ animation: Animation?, value: Value) -> some View {
        modifier(HCBMotionAnimationModifier(animation: animation, value: value))
    }

    func hcbMotionTransition(_ transition: AnyTransition) -> some View {
        modifier(HCBMotionTransitionModifier(transition: transition))
    }

    func hcbDebugBodyProbe(_ name: String) -> some View {
        modifier(HCBDebugBodyProbeModifier(name: name))
    }
}

enum HCBFontStyle {
    case largeTitle, title, title2, title3
    case headline, subheadline
    case body, callout, footnote
    case caption, caption2

    var systemFont: Font {
        switch self {
        case .largeTitle: return .largeTitle
        case .title: return .title
        case .title2: return .title2
        case .title3: return .title3
        case .headline: return .headline
        case .subheadline: return .subheadline
        case .body: return .body
        case .callout: return .callout
        case .footnote: return .footnote
        case .caption: return .caption
        case .caption2: return .caption2
        }
    }

    // Reference size (macOS) used when a custom font family is applied.
    // DynamicType scales these because Font.custom(_:size:relativeTo:) opts in.
    var referenceSize: CGFloat {
        switch self {
        case .largeTitle: return 26
        case .title: return 22
        case .title2: return 17
        case .title3: return 15
        case .headline: return 13
        case .subheadline: return 11
        case .body: return 13
        case .callout: return 12
        case .footnote: return 10
        case .caption: return 10
        case .caption2: return 10
        }
    }

    var relativeTextStyle: Font.TextStyle {
        switch self {
        case .largeTitle: return .largeTitle
        case .title: return .title
        case .title2: return .title2
        case .title3: return .title3
        case .headline: return .headline
        case .subheadline: return .subheadline
        case .body: return .body
        case .callout: return .callout
        case .footnote: return .footnote
        case .caption: return .caption
        case .caption2: return .caption2
        }
    }
}

private struct HCBScaledPaddingModifier: ViewModifier {
    @Environment(\.hcbLayoutScale) private var scale
    let length: CGFloat
    let edges: Edge.Set

    func body(content: Content) -> some View {
        content.padding(edges, length * scale)
    }
}

private struct HCBScaledFrameModifier: ViewModifier {
    @Environment(\.hcbLayoutScale) private var scale
    let width: CGFloat?
    let height: CGFloat?
    let alignment: Alignment

    func body(content: Content) -> some View {
        content.frame(
            width: width.map { $0 * scale },
            height: height.map { $0 * scale },
            alignment: alignment
        )
    }
}

private struct HCBScaledFlexibleFrameModifier: ViewModifier {
    @Environment(\.hcbLayoutScale) private var scale
    let minWidth: CGFloat?
    let idealWidth: CGFloat?
    let maxWidth: CGFloat?
    let minHeight: CGFloat?
    let idealHeight: CGFloat?
    let maxHeight: CGFloat?
    let alignment: Alignment

    func body(content: Content) -> some View {
        content.frame(
            minWidth: minWidth.map { $0 * scale },
            idealWidth: idealWidth.map { $0 * scale },
            maxWidth: maxWidth.map { $0 * scale },
            minHeight: minHeight.map { $0 * scale },
            idealHeight: idealHeight.map { $0 * scale },
            maxHeight: maxHeight.map { $0 * scale },
            alignment: alignment
        )
    }
}

private struct HCBFontModifier: ViewModifier {
    @Environment(\.hcbFontFamily) private var family
    @Environment(\.hcbTextSizePoints) private var basePoints
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let style: HCBFontStyle
    let weight: Font.Weight?

    func body(content: Content) -> some View {
        content.font(resolved)
    }

    // Literal size: style's reference size is scaled by (userBody / 13).
    // Picking 16 as body multiplies every semantic style by 16/13 ≈ 1.23.
    private var resolved: Font {
        let scale = CGFloat(basePoints / HCBTextSize.defaultPoints)
        let size = style.referenceSize * scale * HCBTextSize.dynamicTypeScale(for: dynamicTypeSize)
        let base: Font
        if let family, family.isEmpty == false {
            base = .custom(family, fixedSize: size)
        } else {
            base = .system(size: size)
        }
        if let weight {
            return base.weight(weight)
        }
        return base
    }
}

private struct HCBFontSystemModifier: ViewModifier {
    @Environment(\.hcbFontFamily) private var family
    @Environment(\.hcbTextSizePoints) private var basePoints
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    let size: CGFloat
    let weight: Font.Weight
    let design: Font.Design

    func body(content: Content) -> some View {
        content.font(resolved)
    }

    // System-font call sites (e.g., explicit size) are rarer; still scale
    // them by the same body-size ratio so headings + icons follow the
    // user's chosen text size proportionally.
    private var resolved: Font {
        let scale = CGFloat(basePoints / HCBTextSize.defaultPoints)
        let scaledSize = size * scale * HCBTextSize.dynamicTypeScale(for: dynamicTypeSize)
        if let family, family.isEmpty == false {
            return Font.custom(family, fixedSize: scaledSize).weight(weight)
        }
        return .system(size: scaledSize, weight: weight, design: design)
    }
}

private struct HCBMotionAnimationModifier<Value: Equatable>: ViewModifier {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let animation: Animation?
    let value: Value

    func body(content: Content) -> some View {
        content.animation(HCBMotion.animation(animation, reduceMotion: reduceMotion), value: value)
    }
}

private struct HCBMotionTransitionModifier: ViewModifier {
    @Environment(\.hcbReduceMotion) private var reduceMotion
    let transition: AnyTransition

    func body(content: Content) -> some View {
        content.transition(HCBMotion.transition(transition, reduceMotion: reduceMotion))
    }
}

private struct HCBDebugBodyProbeModifier: ViewModifier {
    let name: String

    func body(content: Content) -> some View {
        #if DEBUG
        if ProcessInfo.processInfo.environment["HCB_BODY_PROBE"] == "1" {
            content.background(HCBDebugBodyProbeEmitter(name: name))
        } else {
            content
        }
        #else
        content
        #endif
    }
}

#if DEBUG
struct HCBDebugBodyProbeCount: Identifiable, Equatable, Sendable {
    var name: String
    var count: Int

    var id: String { name }
}

enum HCBDebugBodyProbeRecorder {
    static var isEnabled: Bool {
        ProcessInfo.processInfo.environment["HCB_BODY_PROBE"] == "1"
    }

    private static let queue = DispatchQueue(label: "com.gongahkia.hotcrossbuns.body-probe")
    private nonisolated(unsafe) static var counts: [String: Int] = [:]

    static func record(_ name: String) {
        guard isEnabled else { return }
        queue.async {
            counts[name, default: 0] += 1
        }
    }

    static func snapshot() -> [HCBDebugBodyProbeCount] {
        guard isEnabled else { return [] }
        return queue.sync {
            counts
                .map { HCBDebugBodyProbeCount(name: $0.key, count: $0.value) }
                .sorted {
                    if $0.count == $1.count {
                        return $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending
                    }
                    return $0.count > $1.count
                }
        }
    }

    static func reset() {
        queue.async {
            counts = [:]
        }
    }
}

private struct HCBDebugBodyProbeEmitter: View {
    let name: String

    var body: some View {
        let _ = HCBDebugBodyProbeRecorder.record(name)
        let _ = AppLogger.debug("view body evaluated", category: .perf, metadata: ["view": name])
        Color.clear
            .frame(width: 0, height: 0)
            .accessibilityHidden(true)
    }
}
#endif

// Installed-font enumeration for the Settings picker.
enum HCBInstalledFonts {
    static func available() -> [String] {
        var names = Set<String>()
        for family in NSFontManager.shared.availableFontFamilies {
            names.insert(family)
        }
        return names.sorted { $0.localizedCaseInsensitiveCompare($1) == .orderedAscending }
    }
}

// §6.11: per-surface appearance override. Re-derives the ambient
// hcbFontFamily / hcbTextSizePoints for a subtree so every nested
// `.hcbFont(.role)` call site inherits the surface's family + size without
// touching individual call sites. Unset fields on the override fall back to
// the global Appearance values, so users only see overrides where they've
// explicitly set one.
extension View {
    func hcbSurface(_ surface: HCBSurface) -> some View {
        modifier(HCBSurfaceAppearanceModifier(surface: surface))
    }
}

private struct HCBSurfaceAppearanceModifier: ViewModifier {
    @Environment(AppModel.self) private var model
    @Environment(\.hcbFontFamily) private var ambientFamily
    @Environment(\.hcbTextSizePoints) private var ambientPoints
    let surface: HCBSurface

    func body(content: Content) -> some View {
        let override = model.settings.perSurfaceFontOverrides[surface.rawValue] ?? .empty
        // Family: surface override → ambient (global) → nil (system font).
        let resolvedFamily: String? = {
            if let name = override.fontName, name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
                return name
            }
            return ambientFamily
        }()
        // Size: surface override clamped → ambient base points (already clamped upstream).
        let resolvedPoints: Double = override.pointSize.map { HCBTextSize.clamp($0) } ?? ambientPoints
        return content
            .environment(\.hcbFontFamily, resolvedFamily)
            .environment(\.hcbTextSizePoints, resolvedPoints)
    }
}
