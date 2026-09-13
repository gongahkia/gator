import Foundation

// Leader-key chord bindings. Press the leader (⌘K by
// default) then a sequence of 1-2 keys to run a command. Lives alongside
// single-key shortcuts — nothing from HCBShortcutCommand changes. Chord
// bindings are a pure overlay that calls into the same AppCommandActions.
//
// Scope (v1):
//  - Hardcoded defaults (no user customisation yet — ship defaults, let users
//    rebind in a future pass).
//  - Single leader: ⌘K.
//  - 3s inactivity timeout.
//  - Esc cancels.
//  - No multi-modifier chord keys (plain characters only after the leader).

struct HCBChordBinding: Equatable, Hashable, Sendable {
    // Post-leader key sequence, single lowercase characters. Up to 2 keys
    // in the default set — longer sequences are supported but not recommended.
    let sequence: [String]
    let command: HCBShortcutCommand
    let hint: String // one-line description shown in the HUD

    init(sequence: [String], command: HCBShortcutCommand, hint: String) {
        self.sequence = sequence.map { $0.lowercased() }
        self.command = command
        self.hint = hint
    }
}

enum HCBChordRegistry {
    // Hardcoded default bindings. Grouped by intent so the HUD can show
    // sensible sub-menus: "n" → new …, "g" → go to …
    static let defaults: [HCBChordBinding] = [
        HCBChordBinding(sequence: ["n", "t"], command: .newTask, hint: String(localized: "New Task")),
        HCBChordBinding(sequence: ["n", "n"], command: .newNote, hint: String(localized: "New Note")),
        HCBChordBinding(sequence: ["n", "e"], command: .newEvent, hint: String(localized: "New Event")),
        HCBChordBinding(sequence: ["g", "t"], command: .goToStore, hint: String(localized: "Go to Tasks")),
        HCBChordBinding(sequence: ["g", "n"], command: .goToNotes, hint: String(localized: "Go to Notes")),
        HCBChordBinding(sequence: ["g", "c"], command: .goToCalendar, hint: String(localized: "Go to Calendar")),
        HCBChordBinding(sequence: ["g", "x"], command: .goToSettings, hint: String(localized: "Go to Settings")),
        HCBChordBinding(sequence: ["p"], command: .commandPalette, hint: String(localized: "Command Palette")),
        HCBChordBinding(sequence: ["r"], command: .refresh, hint: String(localized: "Refresh Sync")),
        HCBChordBinding(sequence: ["d"], command: .diagnostics, hint: String(localized: "Diagnostics")),
        HCBChordBinding(sequence: ["h"], command: .help, hint: String(localized: "Help"))
    ]
}

// Pure match logic: given a partial sequence, returns bindings whose first
// `current.count` keys equal `current`. Used by the state machine to decide:
//  - zero matches → cancel, play haptic-ish feedback
//  - one full match (matches.count == 1 && matches[0].sequence == current) → execute
//  - multiple partial → keep collecting, show HUD
enum HCBChordMatcher {
    static func matches(current: [String], in bindings: [HCBChordBinding]) -> [HCBChordBinding] {
        let normalized = current.map { $0.lowercased() }
        return bindings.filter { b in
            guard b.sequence.count >= normalized.count else { return false }
            return Array(b.sequence.prefix(normalized.count)) == normalized
        }
    }

    // True when `current` is a complete sequence that exactly matches one
    // binding and no longer binding starts with it.
    static func isExactTerminal(current: [String], in bindings: [HCBChordBinding]) -> HCBChordBinding? {
        let normalized = current.map { $0.lowercased() }
        let exact = bindings.filter { $0.sequence == normalized }
        // If ANOTHER binding extends this one (e.g. user has both ["n"] and
        // ["n","t"]), we can't execute on the shorter one without robbing
        // the longer one — the caller should keep collecting.
        let extensions = bindings.filter { b in
            b.sequence.count > normalized.count
                && Array(b.sequence.prefix(normalized.count)) == normalized
        }
        if exact.count == 1, extensions.isEmpty { return exact.first }
        return nil
    }

    // Next-key hints for the HUD — the distinct next characters after
    // `current` across all still-viable bindings, paired with a summary
    // label. Bindings with exactly one more char than `current` contribute
    // their hint directly; bindings with more chars contribute their next
    // char with "…" suffix so the HUD signals "press this then more".
    static func hudHints(current: [String], in bindings: [HCBChordBinding]) -> [ChordHudHint] {
        let normalized = current.map { $0.lowercased() }
        let viable = matches(current: normalized, in: bindings)
            .filter { $0.sequence.count > normalized.count }

        var byNext: [String: [HCBChordBinding]] = [:]
        for b in viable {
            let nextKey = b.sequence[normalized.count]
            byNext[nextKey, default: []].append(b)
        }

        return byNext
            .map { next, group -> ChordHudHint in
                if group.count == 1, let only = group.first, only.sequence.count == normalized.count + 1 {
                    return ChordHudHint(key: next, label: only.hint)
                }
                // Multiple bindings share this prefix — show "…" to signal
                // there's another key to press.
                let label = group.count == 1
                    ? "\(group.first!.hint) …"
                    : "\(group.count) actions …"
                return ChordHudHint(key: next, label: label)
            }
            .sorted { $0.key < $1.key }
    }
}

struct ChordHudHint: Equatable {
    let key: String
    let label: String
}
