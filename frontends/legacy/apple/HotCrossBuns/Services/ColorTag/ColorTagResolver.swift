import Foundation

// Pure logic for the event color-tag feature. Given an event title and the
// user's bindings, returns which Google colorId should auto-apply (if any)
// and which `#tag` token should be stripped from the title before the POST
// goes to Google.
//
// Only the *winning* tag is stripped — losing tags (other #words that also
// happened to bind colors but lost under the match policy, or tags that
// don't bind anything at all) stay in the title as plain text. This keeps
// the feature ephemeral and non-destructive: the tag is a shortcut, not a
// stored concept.
enum ColorTagResolver {
    struct Resolution: Equatable {
        let colorId: String   // Google Calendar colorId ("1".."11")
        let matchedTag: String // the tag spelling that won (for stripping)
    }

    // Build a lowercase tag → colorId lookup from the user's bindings.
    // `bindings` is stored as colorId → tag; multiple colors pointing at
    // the same tag (user error) collapse: last one wins. Empty / blank
    // tag strings are skipped so an unconfigured row never matches.
    static func buildIndex(bindings: [String: String]) -> [String: String] {
        var index: [String: String] = [:]
        for (colorId, rawTag) in bindings {
            let tag = rawTag.trimmingCharacters(in: .whitespacesAndNewlines)
            guard tag.isEmpty == false else { continue }
            index[tag.lowercased()] = colorId
        }
        return index
    }

    // Returns the winning (colorId, matched-tag) under the given policy,
    // or nil if no #tag in the title binds a color. `matched-tag` carries
    // the case as typed (not lowercased) so the caller can strip the
    // exact literal.
    static func resolve(title: String, bindings: [String: String], policy: ColorTagMatchPolicy) -> Resolution? {
        let index = buildIndex(bindings: bindings)
        guard index.isEmpty == false else { return nil }
        let tags = TagExtractor.tags(in: title)
        guard tags.isEmpty == false else { return nil }
        let ordered = policy == .firstMatch ? tags : Array(tags.reversed())
        for tag in ordered {
            if let colorId = index[tag.lowercased()] {
                return Resolution(colorId: colorId, matchedTag: tag)
            }
        }
        return nil
    }

    // Strip exactly one occurrence of `#<matchedTag>` from `title`,
    // case-insensitively. Preserves whatever surrounded the token, then
    // collapses runs of whitespace so we don't leave "Meeting  with X".
    static func stripTag(_ matchedTag: String, from title: String) -> String {
        let escaped = NSRegularExpression.escapedPattern(for: matchedTag)
        // Match `#<tag>` with a word boundary after so "#work" doesn't eat "#workflow".
        guard let regex = try? NSRegularExpression(pattern: "#\(escaped)(?![A-Za-z0-9_-])", options: [.caseInsensitive]) else {
            return title
        }
        let range = NSRange(title.startIndex..., in: title)
        guard let first = regex.firstMatch(in: title, options: [], range: range),
              let swiftRange = Range(first.range, in: title)
        else {
            return title
        }
        var stripped = title
        stripped.replaceSubrange(swiftRange, with: "")
        return stripped
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }
}

// Routes event quick-create text to a Google calendar when a #tag matches
// the calendar name. This lets users model custom colour schemes the way
// Google Calendar does: create separate calendars, give them colours, then
// use tags like #work or #school while capturing events.
enum CalendarTagResolver {
    struct Resolution: Equatable {
        let calendar: CalendarListMirror
        let matchedTag: String
    }

    static func resolve(title: String, calendars: [CalendarListMirror]) -> Resolution? {
        let tags = TagExtractor.tags(in: title)
        guard tags.isEmpty == false else { return nil }

        let index = calendars.reduce(into: [String: CalendarListMirror]()) { result, calendar in
            let key = tagKey(for: calendar.summary)
            guard key.isEmpty == false, result[key] == nil else { return }
            result[key] = calendar
        }
        guard index.isEmpty == false else { return nil }

        for tag in tags {
            if let calendar = index[tagKey(for: tag)] {
                return Resolution(calendar: calendar, matchedTag: tag)
            }
        }
        return nil
    }

    static func tagKey(for value: String) -> String {
        value
            .folding(options: [.caseInsensitive, .diacriticInsensitive], locale: .current)
            .unicodeScalars
            .filter { CharacterSet.alphanumerics.contains($0) }
            .map(String.init)
            .joined()
    }
}
