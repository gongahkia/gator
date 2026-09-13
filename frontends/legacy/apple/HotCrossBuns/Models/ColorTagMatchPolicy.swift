import Foundation

// When multiple `#tag` tokens in an event title match color bindings, this
// policy picks which one wins. The loser tags are left alone and don't get
// stripped from the title on submit — only the winning tag is stripped.
enum ColorTagMatchPolicy: String, CaseIterable, Hashable, Codable, Sendable {
    case firstMatch
    case lastMatch

    var title: String {
        switch self {
        case .firstMatch: "First match"
        case .lastMatch: "Last match"
        }
    }

    var subtitle: String {
        switch self {
        case .firstMatch: "Leftmost matching tag in the title wins."
        case .lastMatch: "Rightmost matching tag in the title wins."
        }
    }
}
