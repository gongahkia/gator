import Foundation

// Shared fuzzy matcher used by the command palette (commands) and the quick
// switcher (entities). Keeps ranking consistent across both surfaces so muscle
// memory behaves the same way.
//
// Algorithm: subsequence match with bonuses.
//  - Every character of `query` must appear in `label`, in order, case-insensitive.
//  - Score starts at 0 and accumulates bonuses: consecutive-match, word-start,
//    prefix-of-label, whole-word-match.
//  - No-match returns nil (caller drops the candidate).
//  - Keywords: tried after the primary label; the best (highest) score wins.
//
// Not a production fzf — intentionally small. Good enough for a few hundred
// entities, which is the realistic ceiling per-user.
enum FuzzySearcher {
    struct Match: Equatable {
        let score: Double
        let matchedRanges: [Range<String.Index>]
    }

    // Returns nil if the query is not a subsequence of any searchable field.
    static func match(label: String, keywords: [String] = [], query: String) -> Match? {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard q.isEmpty == false else {
            return Match(score: 0, matchedRanges: [])
        }

        if let primary = score(label: label, query: q) {
            // Try keywords too — sometimes a synonym ranks higher, but we only
            // use the keyword score for tie-breaking against the label score.
            let keywordBest = keywords
                .compactMap { score(label: $0, query: q)?.score }
                .max() ?? -.infinity
            if keywordBest > primary.score {
                return Match(score: keywordBest, matchedRanges: [])
            }
            return primary
        }

        // No label match — accept a keyword match (no highlights).
        if let kw = keywords.compactMap({ score(label: $0, query: q) }).max(by: { $0.score < $1.score }) {
            return Match(score: kw.score, matchedRanges: [])
        }
        return nil
    }

    static func rank<T>(
        _ items: [T],
        query: String,
        labelForItem: (T) -> String,
        keywordsForItem: (T) -> [String] = { _ in [] },
        limit: Int = 50
    ) -> [(item: T, score: Double)] {
        let trimmed = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.isEmpty == false else {
            // Empty query — return everything up to `limit`, in insertion order.
            return Array(items.prefix(limit)).map { ($0, 0) }
        }
        var results: [(T, Double)] = []
        for item in items {
            if let m = match(label: labelForItem(item), keywords: keywordsForItem(item), query: trimmed) {
                results.append((item, m.score))
            }
        }
        results.sort { $0.1 > $1.1 }
        return Array(results.prefix(limit))
    }

    // MARK: - internals

    // Returns nil when `query` isn't a subsequence of `label`.
    private static func score(label: String, query: String) -> Match? {
        let lowerLabel = label.lowercased()
        let lowerQuery = query.lowercased()
        guard lowerQuery.isEmpty == false else {
            return Match(score: 0, matchedRanges: [])
        }

        var labelIdx = lowerLabel.startIndex
        var queryIdx = lowerQuery.startIndex
        var matchedRanges: [Range<String.Index>] = []
        var rawScore: Double = 0
        var consecutive = 0
        let labelEnd = lowerLabel.endIndex

        // Prefix bonus — the first query char matches position 0.
        if let firstChar = lowerLabel.first, firstChar == lowerQuery.first {
            rawScore += 4
        }

        while labelIdx < labelEnd, queryIdx < lowerQuery.endIndex {
            if lowerLabel[labelIdx] == lowerQuery[queryIdx] {
                // Map indices into the original `label` for highlight ranges.
                let labelOffset = lowerLabel.distance(from: lowerLabel.startIndex, to: labelIdx)
                if let labelRangeStart = label.index(label.startIndex, offsetBy: labelOffset, limitedBy: label.endIndex),
                   let labelRangeEnd = label.index(labelRangeStart, offsetBy: 1, limitedBy: label.endIndex) {
                    if let last = matchedRanges.last, last.upperBound == labelRangeStart {
                        matchedRanges[matchedRanges.count - 1] = last.lowerBound ..< labelRangeEnd
                    } else {
                        matchedRanges.append(labelRangeStart ..< labelRangeEnd)
                    }
                }

                // Bonuses
                if labelIdx == lowerLabel.startIndex {
                    rawScore += 3 // first char of label
                } else if let prev = lowerLabel.index(labelIdx, offsetBy: -1, limitedBy: lowerLabel.startIndex) {
                    let prevChar = lowerLabel[prev]
                    if prevChar == " " || prevChar == "-" || prevChar == "_" || prevChar == "/" {
                        rawScore += 2 // word start
                    }
                }
                consecutive += 1
                rawScore += 1 + Double(consecutive) * 0.5 // consecutive matches grow non-linearly

                queryIdx = lowerQuery.index(after: queryIdx)
            } else {
                consecutive = 0
            }
            labelIdx = lowerLabel.index(after: labelIdx)
        }

        guard queryIdx == lowerQuery.endIndex else { return nil }

        // Full-string match bonus
        if lowerLabel == lowerQuery { rawScore += 8 }
        // Prefix-of-label bonus (query starts the label)
        else if lowerLabel.hasPrefix(lowerQuery) { rawScore += 5 }
        // Whole-word bonus (label contains query surrounded by boundaries)
        else if let wordRange = lowerLabel.range(of: lowerQuery),
                isWordBoundary(in: lowerLabel, range: wordRange) {
            rawScore += 3
        }

        return Match(score: rawScore, matchedRanges: matchedRanges)
    }

    private static func isWordBoundary(in source: String, range: Range<String.Index>) -> Bool {
        let isBoundary: (Character) -> Bool = { c in
            c == " " || c == "-" || c == "_" || c == "/" || c == "." || c == "\t"
        }
        let leading: Bool = range.lowerBound == source.startIndex
            || isBoundary(source[source.index(before: range.lowerBound)])
        let trailing: Bool = range.upperBound == source.endIndex
            || isBoundary(source[range.upperBound])
        return leading && trailing
    }
}
