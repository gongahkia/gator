import AppKit
import SwiftUI

// A named palette for the app. Every HCBColorScheme provides the full set
// of semantic colors AppColor depends on. One palette is active at a time
// (HCBColorSchemeStore.current); changing it updates every AppColor.X
// accessor via a .id() flip at the shell root.
struct HCBColorScheme: Identifiable, Hashable, Sendable {
    let id: String
    let title: String
    let isDark: Bool
    let ember: RGB // accent / primary CTA
    let moss: RGB // success
    let blue: RGB // info / link
    let ink: RGB // primary text
    let cream: RGB // background
    let cardStroke: RGB // card border / divider

    // Card surface one step elevated from the cream background. Dark schemes
    // lighten cream so cards stand out against the backdrop; light schemes
    // darken it slightly for the same effect. Derived rather than stored so
    // every palette — including user-added ones in the future — gets a
    // readable kanban/inspector card without hand-authoring a 3rd color.
    var cardSurface: RGB {
        let delta: Double = isDark ? 0.08 : -0.04
        return RGB(
            red: clamp(cream.red + delta),
            green: clamp(cream.green + delta),
            blue: clamp(cream.blue + delta),
            alpha: 1.0
        )
    }

    private func clamp(_ v: Double) -> Double {
        min(1.0, max(0.0, v))
    }

    struct RGB: Hashable, Sendable {
        let red: Double
        let green: Double
        let blue: Double
        let alpha: Double

        init(_ hex: String, alpha: Double = 1.0) {
            let normalized = hex.trimmingCharacters(in: CharacterSet(charactersIn: "#"))
            var value: UInt64 = 0
            Scanner(string: normalized).scanHexInt64(&value)
            self.red = Double((value >> 16) & 0xff) / 255.0
            self.green = Double((value >> 8) & 0xff) / 255.0
            self.blue = Double(value & 0xff) / 255.0
            self.alpha = alpha
        }

        init(red: Double, green: Double, blue: Double, alpha: Double = 1.0) {
            self.red = red
            self.green = green
            self.blue = blue
            self.alpha = alpha
        }

        var nsColor: NSColor {
            NSColor(red: red, green: green, blue: blue, alpha: alpha)
        }

        var swiftColor: Color {
            Color(red: red, green: green, blue: blue).opacity(alpha)
        }

        var hexString: String {
            let r = Int((max(0, min(1, red)) * 255).rounded())
            let g = Int((max(0, min(1, green)) * 255).rounded())
            let b = Int((max(0, min(1, blue)) * 255).rounded())
            return String(format: "#%02X%02X%02X", r, g, b)
        }
    }
}

struct HCBCustomColorScheme: Identifiable, Hashable, Codable, Sendable {
    var id: String
    var title: String
    var isDark: Bool
    var emberHex: String
    var mossHex: String
    var blueHex: String
    var inkHex: String
    var creamHex: String
    var cardStrokeHex: String

    init(
        id: String = "custom:\(UUID().uuidString)",
        title: String,
        isDark: Bool,
        emberHex: String,
        mossHex: String,
        blueHex: String,
        inkHex: String,
        creamHex: String,
        cardStrokeHex: String
    ) {
        self.id = id.hasPrefix("custom:") ? id : "custom:\(id)"
        self.title = title.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? "Custom theme" : title
        self.isDark = isDark
        self.emberHex = Self.normalizedHex(emberHex, fallback: "#E9552F")
        self.mossHex = Self.normalizedHex(mossHex, fallback: "#3C7255")
        self.blueHex = Self.normalizedHex(blueHex, fallback: "#1677FF")
        self.inkHex = Self.normalizedHex(inkHex, fallback: isDark ? "#F4F1EC" : "#1B1E24")
        self.creamHex = Self.normalizedHex(creamHex, fallback: isDark ? "#171A21" : "#FCF4E4")
        self.cardStrokeHex = Self.normalizedHex(cardStrokeHex, fallback: isDark ? "#30343D" : "#DFD3BF")
    }

    init(copying scheme: HCBColorScheme, title: String? = nil) {
        self.init(
            title: title ?? "\(scheme.title) copy",
            isDark: scheme.isDark,
            emberHex: scheme.ember.hexString,
            mossHex: scheme.moss.hexString,
            blueHex: scheme.blue.hexString,
            inkHex: scheme.ink.hexString,
            creamHex: scheme.cream.hexString,
            cardStrokeHex: scheme.cardStroke.hexString
        )
    }

    func colorScheme() -> HCBColorScheme {
        HCBColorScheme(
            id: id,
            title: title,
            isDark: isDark,
            ember: .init(emberHex),
            moss: .init(mossHex),
            blue: .init(blueHex),
            ink: .init(inkHex),
            cream: .init(creamHex),
            cardStroke: .init(cardStrokeHex)
        )
    }

    static func normalizedHex(_ value: String, fallback: String) -> String {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        let raw = trimmed.hasPrefix("#") ? String(trimmed.dropFirst()) : trimmed
        guard raw.count == 6, raw.allSatisfy(\.isHexDigit) else { return fallback }
        return "#\(raw.uppercased())"
    }
}

extension HCBColorScheme {
    static let notion = HCBColorScheme(
        id: "notion",
        title: "Notion",
        isDark: false,
        ember: .init("2383E2"),
        moss: .init("448361"),
        blue: .init("2383E2"),
        ink: .init("37352F"),
        cream: .init("FFFFFF"),
        cardStroke: .init("E5E5E5")
    )

    static let hotCrossBuns = HCBColorScheme(
        id: "hotcrossbuns",
        title: "Hot Cross Buns",
        isDark: false,
        ember: .init(red: 0.965, green: 0.420, blue: 0.231),
        moss: .init(red: 0.235, green: 0.447, blue: 0.333),
        blue: .init(red: 0.086, green: 0.467, blue: 1.000),
        ink: .init(red: 0.106, green: 0.118, blue: 0.145),
        cream: .init(red: 0.988, green: 0.957, blue: 0.894),
        cardStroke: .init(red: 0, green: 0, blue: 0, alpha: 0.08)
    )

    static let dracula = HCBColorScheme(
        id: "dracula",
        title: "Dracula",
        isDark: true,
        ember: .init("FF79C6"),
        moss: .init("50FA7B"),
        blue: .init("8BE9FD"),
        ink: .init("F8F8F2"),
        cream: .init("282A36"),
        cardStroke: .init("44475A")
    )

    static let oneDarkPro = HCBColorScheme(
        id: "oneDarkPro",
        title: "One Dark Pro",
        isDark: true,
        ember: .init("E06C75"),
        moss: .init("98C379"),
        blue: .init("61AFEF"),
        ink: .init("ABB2BF"),
        cream: .init("282C34"),
        cardStroke: .init("3E4451")
    )

    static let solarizedLight = HCBColorScheme(
        id: "solarizedLight",
        title: "Solarized Light",
        isDark: false,
        ember: .init("CB4B16"),
        moss: .init("859900"),
        blue: .init("268BD2"),
        ink: .init("586E75"),
        cream: .init("FDF6E3"),
        cardStroke: .init("EEE8D5")
    )

    static let solarizedDark = HCBColorScheme(
        id: "solarizedDark",
        title: "Solarized Dark",
        isDark: true,
        ember: .init("CB4B16"),
        moss: .init("859900"),
        blue: .init("268BD2"),
        ink: .init("839496"),
        cream: .init("002B36"),
        cardStroke: .init("073642")
    )

    static let nord = HCBColorScheme(
        id: "nord",
        title: "Nord",
        isDark: true,
        ember: .init("88C0D0"),
        moss: .init("A3BE8C"),
        blue: .init("81A1C1"),
        ink: .init("D8DEE9"),
        cream: .init("2E3440"),
        cardStroke: .init("3B4252")
    )

    static let gruvboxDark = HCBColorScheme(
        id: "gruvboxDark",
        title: "Gruvbox Dark",
        isDark: true,
        ember: .init("FE8019"),
        moss: .init("B8BB26"),
        blue: .init("83A598"),
        ink: .init("EBDBB2"),
        cream: .init("282828"),
        cardStroke: .init("3C3836")
    )

    static let gruvboxLight = HCBColorScheme(
        id: "gruvboxLight",
        title: "Gruvbox Light",
        isDark: false,
        ember: .init("D65D0E"),
        moss: .init("98971A"),
        blue: .init("458588"),
        ink: .init("3C3836"),
        cream: .init("FBF1C7"),
        cardStroke: .init("D5C4A1")
    )

    static let tokyoNight = HCBColorScheme(
        id: "tokyoNight",
        title: "Tokyo Night",
        isDark: true,
        ember: .init("BB9AF7"),
        moss: .init("9ECE6A"),
        blue: .init("7DCFFF"),
        ink: .init("C0CAF5"),
        cream: .init("1A1B26"),
        cardStroke: .init("414868")
    )

    static let catppuccinMocha = HCBColorScheme(
        id: "catppuccinMocha",
        title: "Catppuccin Mocha",
        isDark: true,
        ember: .init("F5C2E7"),
        moss: .init("A6E3A1"),
        blue: .init("89B4FA"),
        ink: .init("CDD6F4"),
        cream: .init("1E1E2E"),
        cardStroke: .init("313244")
    )

    static let catppuccinLatte = HCBColorScheme(
        id: "catppuccinLatte",
        title: "Catppuccin Latte",
        isDark: false,
        ember: .init("EA76CB"),
        moss: .init("40A02B"),
        blue: .init("1E66F5"),
        ink: .init("4C4F69"),
        cream: .init("EFF1F5"),
        cardStroke: .init("BCC0CC")
    )

    static let githubLight = HCBColorScheme(
        id: "githubLight",
        title: "GitHub Light",
        isDark: false,
        ember: .init("0969DA"),
        moss: .init("1A7F37"),
        blue: .init("0969DA"),
        ink: .init("24292F"),
        cream: .init("FFFFFF"),
        cardStroke: .init("D0D7DE")
    )

    static let githubDark = HCBColorScheme(
        id: "githubDark",
        title: "GitHub Dark",
        isDark: true,
        ember: .init("58A6FF"),
        moss: .init("3FB950"),
        blue: .init("58A6FF"),
        ink: .init("C9D1D9"),
        cream: .init("0D1117"),
        cardStroke: .init("30363D")
    )

    static let ayuLight = HCBColorScheme(
        id: "ayuLight",
        title: "Ayu Light",
        isDark: false,
        ember: .init("FF6A00"),
        moss: .init("86B300"),
        blue: .init("36A3D9"),
        ink: .init("5C6773"),
        cream: .init("FAFAFA"),
        cardStroke: .init("E6E1CF")
    )

    static let ayuDark = HCBColorScheme(
        id: "ayuDark",
        title: "Ayu Dark",
        isDark: true,
        ember: .init("FFB454"),
        moss: .init("AAD94C"),
        blue: .init("59C2FF"),
        ink: .init("B3B1AD"),
        cream: .init("0F1419"),
        cardStroke: .init("1F2430")
    )

    static let materialPalenight = HCBColorScheme(
        id: "materialPalenight",
        title: "Material Palenight",
        isDark: true,
        ember: .init("C792EA"),
        moss: .init("C3E88D"),
        blue: .init("82AAFF"),
        ink: .init("A6ACCD"),
        cream: .init("292D3E"),
        cardStroke: .init("444267")
    )

    static let rosePine = HCBColorScheme(
        id: "rosePine",
        title: "Rosé Pine",
        isDark: true,
        ember: .init("EBBCBA"),
        moss: .init("9CCFD8"),
        blue: .init("C4A7E7"),
        ink: .init("E0DEF4"),
        cream: .init("191724"),
        cardStroke: .init("26233A")
    )

    static let nightOwl = HCBColorScheme(
        id: "nightOwl",
        title: "Night Owl",
        isDark: true,
        ember: .init("82AAFF"),
        moss: .init("ADDB67"),
        blue: .init("7FDBCA"),
        ink: .init("D6DEEB"),
        cream: .init("011627"),
        cardStroke: .init("1D3B53")
    )

    static let monokai = HCBColorScheme(
        id: "monokai",
        title: "Monokai",
        isDark: true,
        ember: .init("F92672"),
        moss: .init("A6E22E"),
        blue: .init("66D9EF"),
        ink: .init("F8F8F2"),
        cream: .init("272822"),
        cardStroke: .init("3E3D32")
    )

    static let oneLight = HCBColorScheme(
        id: "oneLight",
        title: "One Light",
        isDark: false,
        ember: .init("4078F2"),
        moss: .init("50A14F"),
        blue: .init("4078F2"),
        ink: .init("383A42"),
        cream: .init("FAFAFA"),
        cardStroke: .init("E5E5E6")
    )

    static let ayuMirage = HCBColorScheme(
        id: "ayuMirage",
        title: "Ayu Mirage",
        isDark: true,
        ember: .init("FFCC66"),
        moss: .init("87D96C"),
        blue: .init("73D0FF"),
        ink: .init("CBCCC6"),
        cream: .init("1F2430"),
        cardStroke: .init("171B24")
    )

    static let everforestDark = HCBColorScheme(
        id: "everforestDark",
        title: "Everforest Dark",
        isDark: true,
        ember: .init("E67E80"),
        moss: .init("A7C080"),
        blue: .init("7FBBB3"),
        ink: .init("D3C6AA"),
        cream: .init("2D353B"),
        cardStroke: .init("3D484D")
    )

    static let everforestLight = HCBColorScheme(
        id: "everforestLight",
        title: "Everforest Light",
        isDark: false,
        ember: .init("F85552"),
        moss: .init("8DA101"),
        blue: .init("3A94C5"),
        ink: .init("5C6A72"),
        cream: .init("FDF6E3"),
        cardStroke: .init("EFEBD4")
    )

    static let kanagawa = HCBColorScheme(
        id: "kanagawa",
        title: "Kanagawa",
        isDark: true,
        ember: .init("FFA066"),
        moss: .init("98BB6C"),
        blue: .init("7E9CD8"),
        ink: .init("DCD7BA"),
        cream: .init("1F1F28"),
        cardStroke: .init("2A2A37")
    )

    static let rosePineMoon = HCBColorScheme(
        id: "rosePineMoon",
        title: "Rosé Pine Moon",
        isDark: true,
        ember: .init("EA9A97"),
        moss: .init("9CCFD8"),
        blue: .init("C4A7E7"),
        ink: .init("E0DEF4"),
        cream: .init("232136"),
        cardStroke: .init("393552")
    )

    static let rosePineDawn = HCBColorScheme(
        id: "rosePineDawn",
        title: "Rosé Pine Dawn",
        isDark: false,
        ember: .init("D7827E"),
        moss: .init("56949F"),
        blue: .init("286983"),
        ink: .init("575279"),
        cream: .init("FAF4ED"),
        cardStroke: .init("DFDAD9")
    )

    static let moonlight = HCBColorScheme(
        id: "moonlight",
        title: "Moonlight",
        isDark: true,
        ember: .init("C099FF"),
        moss: .init("C3E88D"),
        blue: .init("86E1FC"),
        ink: .init("C8D3F5"),
        cream: .init("212337"),
        cardStroke: .init("2F334D")
    )

    static let cobalt2 = HCBColorScheme(
        id: "cobalt2",
        title: "Cobalt2",
        isDark: true,
        ember: .init("FFC600"),
        moss: .init("3AD900"),
        blue: .init("9EFFFF"),
        ink: .init("FFFFFF"),
        cream: .init("193549"),
        cardStroke: .init("1F4662")
    )

    static let synthwave84 = HCBColorScheme(
        id: "synthwave84",
        title: "SynthWave '84",
        isDark: true,
        ember: .init("FF7EDB"),
        moss: .init("72F1B8"),
        blue: .init("36F9F6"),
        ink: .init("F1F1F0"),
        cream: .init("241B2F"),
        cardStroke: .init("34294F")
    )

    static let shadesOfPurple = HCBColorScheme(
        id: "shadesOfPurple",
        title: "Shades of Purple",
        isDark: true,
        ember: .init("FAD000"),
        moss: .init("3AD900"),
        blue: .init("9EFFFF"),
        ink: .init("F5F5F5"),
        cream: .init("2D2B55"),
        cardStroke: .init("1E1E3F")
    )

    static let oceanicNext = HCBColorScheme(
        id: "oceanicNext",
        title: "Oceanic Next",
        isDark: true,
        ember: .init("F99157"),
        moss: .init("99C794"),
        blue: .init("6699CC"),
        ink: .init("D8DEE9"),
        cream: .init("1B2B34"),
        cardStroke: .init("343D46")
    )

    static let tomorrowNight = HCBColorScheme(
        id: "tomorrowNight",
        title: "Tomorrow Night",
        isDark: true,
        ember: .init("CC6666"),
        moss: .init("B5BD68"),
        blue: .init("81A2BE"),
        ink: .init("C5C8C6"),
        cream: .init("1D1F21"),
        cardStroke: .init("373B41")
    )

    static let zenburn = HCBColorScheme(
        id: "zenburn",
        title: "Zenburn",
        isDark: true,
        ember: .init("F0DFAF"),
        moss: .init("7F9F7F"),
        blue: .init("8CD0D3"),
        ink: .init("DCDCCC"),
        cream: .init("3F3F3F"),
        cardStroke: .init("4F4F4F")
    )

    static let horizon = HCBColorScheme(
        id: "horizon",
        title: "Horizon",
        isDark: true,
        ember: .init("E95678"),
        moss: .init("29D398"),
        blue: .init("26BBD9"),
        ink: .init("C7C7C7"),
        cream: .init("1C1E26"),
        cardStroke: .init("2E303E")
    )

    static let iceberg = HCBColorScheme(
        id: "iceberg",
        title: "Iceberg",
        isDark: true,
        ember: .init("E2A478"),
        moss: .init("B5BF77"),
        blue: .init("84A0C6"),
        ink: .init("C6C8D1"),
        cream: .init("161821"),
        cardStroke: .init("1E2132")
    )

    static let pandaSyntax = HCBColorScheme(
        id: "pandaSyntax",
        title: "Panda Syntax",
        isDark: true,
        ember: .init("FF75B5"),
        moss: .init("19F9D8"),
        blue: .init("45A9F9"),
        ink: .init("E6E6E6"),
        cream: .init("292A2B"),
        cardStroke: .init("3A3B3D")
    )

    static let poimandres = HCBColorScheme(
        id: "poimandres",
        title: "Poimandres",
        isDark: true,
        ember: .init("89DDFF"),
        moss: .init("5DE4C7"),
        blue: .init("91B4D5"),
        ink: .init("A6ACCD"),
        cream: .init("1B1E28"),
        cardStroke: .init("303340")
    )

    static let vitesseDark = HCBColorScheme(
        id: "vitesseDark",
        title: "Vitesse Dark",
        isDark: true,
        ember: .init("4D9375"),
        moss: .init("4D9375"),
        blue: .init("6394BF"),
        ink: .init("DBD7CA"),
        cream: .init("121212"),
        cardStroke: .init("393A34")
    )

    static let vitesseLight = HCBColorScheme(
        id: "vitesseLight",
        title: "Vitesse Light",
        isDark: false,
        ember: .init("1E754F"),
        moss: .init("1E754F"),
        blue: .init("2993A3"),
        ink: .init("393A34"),
        cream: .init("FFFFFF"),
        cardStroke: .init("DBD7CA")
    )

    // Order as presented in Settings: Notion first (default), then sorted
    // roughly by aggregate popularity (VSCode marketplace installs,
    // GitHub stars, cross-editor presence). Hot Cross Buns last — it's
    // the house theme, not a ported palette.
    static let all: [HCBColorScheme] = [
        .notion,
        .oneDarkPro,
        .githubDark,
        .githubLight,
        .dracula,
        .solarizedDark,
        .solarizedLight,
        .monokai,
        .tokyoNight,
        .materialPalenight,
        .nord,
        .gruvboxDark,
        .gruvboxLight,
        .catppuccinMocha,
        .catppuccinLatte,
        .ayuDark,
        .ayuLight,
        .ayuMirage,
        .nightOwl,
        .oneLight,
        .rosePine,
        .rosePineMoon,
        .rosePineDawn,
        .kanagawa,
        .everforestDark,
        .everforestLight,
        .moonlight,
        .cobalt2,
        .synthwave84,
        .shadesOfPurple,
        .oceanicNext,
        .tomorrowNight,
        .zenburn,
        .horizon,
        .iceberg,
        .pandaSyntax,
        .poimandres,
        .vitesseDark,
        .vitesseLight,
        .hotCrossBuns
    ]

    static func scheme(id: String) -> HCBColorScheme? {
        all.first { $0.id == id }
    }

    static func all(including customSchemes: [HCBCustomColorScheme]) -> [HCBColorScheme] {
        all + customSchemes.map { $0.colorScheme() }
    }

    static func scheme(id: String, customSchemes: [HCBCustomColorScheme]) -> HCBColorScheme? {
        all.first { $0.id == id } ?? customSchemes.first { $0.id == id }?.colorScheme()
    }
}

// Mutable current-scheme holder. Read by AppColor accessors. Updated when
// the user picks a new scheme in Settings. SwiftUI view re-evaluation is
// triggered separately via a .id(schemeID) modifier at the shell root.
//
// @MainActor-bound: all writers (AppModel settings-change paths,
// MacSidebarShell / MenuBarExtraScene color-scheme onChange handlers)
// already run on main, and every reader flows through SwiftUI View body /
// computed properties which are @MainActor. Previously nonisolated(unsafe)
// — an escape hatch silencing the compiler rather than reflecting the real
// access pattern. Under Swift 6 strict checking the @MainActor form surfaces
// any future off-main caller as a compile error instead of a latent race.
enum HCBColorSchemeStore {
    @MainActor static var current: HCBColorScheme = .notion
}
