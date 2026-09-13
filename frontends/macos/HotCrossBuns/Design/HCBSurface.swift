import SwiftUI

// Discrete rendering surfaces that can receive a font override. Each surface
// is orthogonal: changing the markdown editor font doesn't affect the task
// list, and vice versa. Kept deliberately small — adding more surfaces is
// trivial but each additional one expands the Settings matrix.
enum HCBSurface: String, CaseIterable, Hashable, Codable, Sendable {
    case editor      // markdown editor (task notes, event details)
    case sidebar     // MacSidebarShell list rows
    case calendarGrid // week/day/month/year/agenda body text
    case taskList    // Store list rows
    case inspector   // task / event detail panes
    case menuBar     // menu-bar extra popover

    var title: String {
        switch self {
        case .editor: "Markdown editor"
        case .sidebar: "Sidebar"
        case .calendarGrid: "Calendar grid"
        case .taskList: "Task list"
        case .inspector: "Inspector"
        case .menuBar: "Menu bar"
        }
    }

    var systemImage: String {
        switch self {
        case .editor: "text.alignleft"
        case .sidebar: "sidebar.left"
        case .calendarGrid: "calendar"
        case .taskList: "list.bullet"
        case .inspector: "sidebar.right"
        case .menuBar: "menubar.dock.rectangle"
        }
    }
}

// Per-surface font override. `nil` fields inherit the global Appearance
// (uiFontName / uiTextSizePoints). Only non-nil fields override.
struct HCBSurfaceFontOverride: Codable, Hashable, Sendable {
    var fontName: String?   // PostScript family, nil = system default
    var pointSize: Double?  // nil = global size

    static let empty = HCBSurfaceFontOverride(fontName: nil, pointSize: nil)

    var isEmpty: Bool {
        fontName == nil && pointSize == nil
    }
}

extension AppSettings {
    // Resolves the effective font for a surface given the global defaults.
    // Point size is clamped to the HCBTextSize range so a corrupt user
    // override can't produce a 1pt or 999pt font.
    func resolvedFont(for surface: HCBSurface, baseSize: CGFloat) -> Font {
        let override = perSurfaceFontOverrides[surface.rawValue] ?? .empty
        let size = override.pointSize.map { CGFloat(HCBTextSize.clamp($0)) } ?? baseSize
        if let name = override.fontName, name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            return Font.custom(name, size: size)
        }
        if let globalName = uiFontName, globalName.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false {
            return Font.custom(globalName, size: size)
        }
        return Font.system(size: size)
    }
}
