import Testing
@testable import HotCrossBunsMac

// Migrated from XCTest to Swift Testing.
// Parameterized the score-ordering suite so adding a new ranking scenario
// only needs a row in the table, not a whole new test method.

struct FuzzySearcherTests {

    // MARK: - match / score

    @Test func emptyQueryMatchesWithZeroScore() {
        let m = FuzzySearcher.match(label: "Anything", query: "")
        #expect(m != nil)
        #expect(m?.score == 0)
    }

    @Test func nonSubsequenceReturnsNil() {
        #expect(FuzzySearcher.match(label: "Hello world", query: "zzz") == nil)
    }

    @Test(arguments: [
        ("Refresh Sync", "REFRESH"),
        ("REFRESH SYNC", "refresh"),
    ])
    func caseInsensitive(label: String, query: String) {
        #expect(FuzzySearcher.match(label: label, query: query) != nil)
    }

    @Test func keywordsAreConsulted() {
        // Label alone doesn't contain 'sync', but the keyword does.
        let m = FuzzySearcher.match(label: "Refresh", keywords: ["sync", "reload"], query: "sync")
        #expect(m != nil)
    }

    @Test func highlightRangesCoverMatches() throws {
        let m = try #require(FuzzySearcher.match(label: "refresh sync", query: "rsh"))
        let matched = m.matchedRanges.map { "refresh sync"[$0] }.joined()
        #expect(String(matched) == "rsh")
    }

    // Parameterized ordering table: (description, higher-scoring input, lower-scoring input, shared query).
    // Each row asserts that the first input scores strictly higher than the second.
    @Test(arguments: [
        ("exact beats prefix", "refresh", "refresh sync", "refresh"),
        ("prefix beats fuzzy", "refresh sync", "my refresh thingy", "refresh"),
        ("word-start beats mid-word", "New Task", "Snoring Theatrics", "nt"),
        ("consecutive beats scattered", "buy milk", "b u y", "buy"),
    ])
    func scoreOrderingMatrix(_ description: String, higher: String, lower: String, query: String) throws {
        let high = try #require(FuzzySearcher.match(label: higher, query: query), Comment(rawValue: description))
        let low = try #require(FuzzySearcher.match(label: lower, query: query), Comment(rawValue: description))
        #expect(high.score > low.score, Comment(rawValue: description))
    }

    // MARK: - rank

    @Test func rankReturnsHighestScoringFirst() {
        let labels = ["New Task", "Refresh Sync", "Open Diagnostics", "Force Full Resync"]
        let result = FuzzySearcher.rank(labels, query: "refresh", labelForItem: { $0 })
        #expect(result.first?.item == "Refresh Sync")
    }

    @Test func rankDropsNonMatches() {
        let labels = ["alpha", "beta", "gamma"]
        let result = FuzzySearcher.rank(labels, query: "zzz", labelForItem: { $0 })
        #expect(result.isEmpty)
    }

    @Test func rankHonorsLimit() {
        let many = (1...100).map { "item\($0)" }
        let result = FuzzySearcher.rank(many, query: "item", labelForItem: { $0 }, limit: 5)
        #expect(result.count == 5)
    }

    @Test func rankWithEmptyQueryReturnsPrefix() {
        let labels = ["a", "b", "c", "d", "e"]
        let result = FuzzySearcher.rank(labels, query: "", labelForItem: { $0 }, limit: 3)
        #expect(result.map(\.item) == ["a", "b", "c"])
    }

    @Test func rankUsesKeywordsForFallback() {
        struct Cmd { let title: String; let keywords: [String] }
        let cmds = [
            Cmd(title: "Refresh", keywords: ["sync", "reload"]),
            Cmd(title: "Compile", keywords: [])
        ]
        let result = FuzzySearcher.rank(
            cmds,
            query: "sync",
            labelForItem: { $0.title },
            keywordsForItem: { $0.keywords }
        )
        #expect(result.first?.item.title == "Refresh")
    }
}
