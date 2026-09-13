import Foundation

// Top-level layout mode for the Notes tab. Kept independent from Tasks
// since notes have their own defaults and set of
// reasonable modes (no by-due-date column — notes are undated).
enum NotesViewMode: String, CaseIterable, Hashable, Codable, Sendable {
    case grid       // flat Trello-style card grid (default, pre-refactor behavior)
    case grouped    // simple toggle: same grid with section headers by list
    case kanban     // columns per list / per tag (by-list / by-tag)

    var title: String {
        switch self {
        case .grid: "Grid"
        case .grouped: "Grouped"
        case .kanban: "Kanban"
        }
    }

    var systemImage: String {
        switch self {
        case .grid: "square.grid.2x2"
        case .grouped: "list.bullet.rectangle"
        case .kanban: "rectangle.3.group"
        }
    }
}
