import Foundation

enum TagExtractor {
    private static let pattern = #"(?<![A-Za-z0-9_])#([A-Za-z0-9_\-]{1,40})(?![A-Za-z0-9_\-])"#
    private static let tagRegex = try! NSRegularExpression(pattern: pattern)

    struct ExtractionProfile: Sendable {
        var regexNanoseconds: UInt64 = 0
        var singleHashFastPathNanoseconds: UInt64 = 0
    }

    private struct Match {
        let tag: String
        let range: Range<String.Index>
    }

    static func tags(in text: String) -> [String] {
        matches(in: text).map(\.tag)
    }

    static func tagsProfiled(in text: String) -> (tags: [String], profile: ExtractionProfile) {
        var profile = ExtractionProfile()
        let tags = matches(in: text, profile: &profile).map(\.tag)
        return (tags, profile)
    }

    static func firstTag(in text: String) -> (tag: String, range: Range<String.Index>)? {
        guard let match = matches(in: text).first else { return nil }
        return (match.tag, match.range)
    }

    static func stripped(from text: String) -> String {
        let matches = matches(in: text)
        guard matches.isEmpty == false else { return text }
        var cleaned = text
        for match in matches.reversed() {
            cleaned.removeSubrange(match.range)
        }
        return cleaned
            .replacingOccurrences(of: "\\s+", with: " ", options: .regularExpression)
            .trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func matches(in text: String) -> [Match] {
        matches(in: text, profile: nil)
    }

    private static func matches(
        in text: String,
        profile: UnsafeMutablePointer<ExtractionProfile>?
    ) -> [Match] {
        guard text.contains("#") else { return [] }
        if let hashIndex = text.firstIndex(of: "#"),
           text[text.index(after: hashIndex)...].contains("#") == false {
            guard let profile else {
                return singleHashMatch(in: text, hashIndex: hashIndex).map { [$0] } ?? []
            }
            let start = DispatchTime.now().uptimeNanoseconds
            let result = singleHashMatch(in: text, hashIndex: hashIndex).map { [$0] } ?? []
            profile.pointee.singleHashFastPathNanoseconds += DispatchTime.now().uptimeNanoseconds - start
            return result
        }

        let regexStart = profile.map { _ in DispatchTime.now().uptimeNanoseconds } ?? 0
        let range = NSRange(text.startIndex..., in: text)
        let regexMatches = tagRegex.matches(in: text, range: range)
        if let profile {
            profile.pointee.regexNanoseconds += DispatchTime.now().uptimeNanoseconds - regexStart
        }
        return regexMatches.compactMap { match -> Match? in
            guard match.numberOfRanges >= 2,
                  let tagRange = Range(match.range(at: 1), in: text),
                  let fullRange = Range(match.range, in: text)
            else {
                return nil
            }
            let tag = String(text[tagRange])
            guard isUserTag(tag) else { return nil }
            return Match(tag: tag, range: fullRange)
        }
    }

    private static func singleHashMatch(in text: String, hashIndex: String.Index) -> Match? {
        if hashIndex > text.startIndex {
            let previousIndex = text.index(before: hashIndex)
            guard isTagPrefixBoundary(text[previousIndex]) else { return nil }
        }

        let tagStart = text.index(after: hashIndex)
        guard tagStart < text.endIndex else { return nil }

        var cursor = tagStart
        var length = 0
        while cursor < text.endIndex, isTagBodyCharacter(text[cursor]) {
            length += 1
            guard length <= 40 else { return nil }
            cursor = text.index(after: cursor)
        }

        guard length > 0 else { return nil }
        let tag = String(text[tagStart..<cursor])
        guard isUserTag(tag) else { return nil }
        return Match(tag: tag, range: hashIndex..<cursor)
    }

    private static func isTagPrefixBoundary(_ character: Character) -> Bool {
        guard let scalar = onlyUnicodeScalar(in: character) else { return true }
        return isASCIILetterOrDigit(scalar) == false && scalar.value != 95
    }

    private static func isTagBodyCharacter(_ character: Character) -> Bool {
        guard let scalar = onlyUnicodeScalar(in: character) else { return false }
        return isASCIILetterOrDigit(scalar) || scalar.value == 95 || scalar.value == 45
    }

    private static func onlyUnicodeScalar(in character: Character) -> Unicode.Scalar? {
        var iterator = character.unicodeScalars.makeIterator()
        guard let first = iterator.next(), iterator.next() == nil else { return nil }
        return first
    }

    private static func isASCIILetterOrDigit(_ scalar: Unicode.Scalar) -> Bool {
        switch scalar.value {
        case 48...57, 65...90, 97...122:
            return true
        default:
            return false
        }
    }

    private static func isUserTag(_ value: String) -> Bool {
        isNumericDashTag(value) == false
    }

    private static func isNumericDashTag(_ value: String) -> Bool {
        var sawDash = false
        var hasDigitInCurrentGroup = false
        for scalar in value.unicodeScalars {
            switch scalar.value {
            case 48...57:
                hasDigitInCurrentGroup = true
            case 45:
                guard hasDigitInCurrentGroup else { return false }
                sawDash = true
                hasDigitInCurrentGroup = false
            default:
                return false
            }
        }
        return sawDash && hasDigitInCurrentGroup
    }
}
