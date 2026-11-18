import Foundation
import Testing
@testable import GatorCore

func searchFixture(_ texts: [String], conversation: String = "A") -> [BridgeMessage] {
    BridgeSnapshot(.object(["conversation_id": .string(conversation), "messages": .array(texts.enumerated().map { offset, text in
        .object(["role": .string(offset.isMultiple(of: 2) ? "You" : "Gator"), "text": .string(text)])
    })])).messages
}
let searchScope = NativeTranscriptSearch.Scope(context: "conversation:A", generation: UUID(uuidString: "00000000-0000-0000-0000-000000000001")!)

@Test func transcriptSearchExactCaseAndRepeatedMatchesPreserveMessageIdentityAndOrder() {
    let messages = searchFixture(["Needle needle", "the NEEDLE reply", "absent"])
    let index = NativeTranscriptSearch(scope: searchScope, messages: messages)
    let result = index.search("needle")
    #expect(result.matches.map(\.messageID) == [messages[0].id, messages[0].id, messages[1].id])
    #expect(result.matches.map(\.range.location) == [0, 7, 4])
    #expect(result.matches.map(\.ordinal) == [1, 1, 2])
    #expect(result.matches.map(\.role) == ["You", "You", "Gator"])
    #expect(Set(result.matches.map(\.id)).count == 3)
    #expect(result.matches.allSatisfy { index.resolve($0) == $0.messageID })
    #expect(index.search("absent").matches.count == 1)
    #expect(!result.limited)
}

@Test func transcriptSearchUnicodeUsesComposedBoundariesAndLiteralCaseInsensitiveMatching() throws {
    let index = NativeTranscriptSearch(scope: searchScope, messages: searchFixture(["👩🏽‍💻 Cafe\u{301} CAFÉ Straße 東京 𝒜", "[a+b].*"]))
    #expect(index.search("café").matches.count == 2)
    #expect(index.search("STRASSE").matches.count == 1)
    #expect(index.search("東京").matches.count == 1)
    #expect(index.search("[a+b].*").matches.count == 1)
    let match = try #require(index.search("cafe").matches.first)
    #expect(match.excerpt.contains("👩🏽‍💻"))
    #expect(match.range.length == 5)
}

@Test func transcriptSearchEmptyWhitespaceAndAbsentQueriesHaveNoResults() {
    let index = NativeTranscriptSearch(scope: searchScope, messages: searchFixture(["ordinary message"]))
    for query in ["", " \n\t", "unfindable"] { #expect(index.search(query).matches.isEmpty) }
    #expect(index.search(String(repeating: "a", count: 257)).limited)
}

@Test func transcriptSearchExcludesPrivateFieldsCollapsedDetailsAndUnrenderedStructuredText() {
    let values: [JSONValue] = [
        .object(["role": .string("Gator"), "text": .string("visible answer"), "reasoning": .string("private-secret"), "activity": .object(["entries": .array([.string("activity-secret")])])]),
        .object(["role": .string("Gator"), "text": .string("unrendered-secret"), "result": .object(["title": .string("Public result heading"), "summary": .string("collapsed-secret"), "token": .string("credential-secret")])]),
        .object(["role": .string("analysis"), "text": .string("private-secret")]),
        .object(["role": .string("assistant"), "channel": .string("analysis"), "text": .string("private-secret")]),
        .object(["role": .string("assistant"), "hidden": .bool(true), "text": .string("private-secret")]),
        .object(["role": .string("tool"), "text": .string("private-secret")]),
        .object(["role": .string("You"), "text": .string("user message"), "input_ids": .array([.string("attachment-secret")])])
    ]
    let messages = BridgeSnapshot(.object(["conversation_id": .string("A"), "messages": .array(values)])).messages
    let index = NativeTranscriptSearch(scope: searchScope, messages: messages)
    #expect(index.search("secret").matches.isEmpty)
    #expect(index.search("visible answer").matches.count == 1)
    #expect(index.search("public result").matches.map(\.messageID) == [messages[1].id])
    #expect(index.search("user message").matches.count == 1)
}

@Test func transcriptSearchIncludesProjectedPublicDialogueWithoutInventingOlderHistory() {
    let snapshot = BridgeSnapshot(.object(["conversation_id": .string("A"), "dialogue_more": .bool(true), "dialogue": .array([
        .object(["id": .string("turn-1"), "sequence": .number(1), "text": .string("public question"), "response": .string("public response")])
    ])]))
    let index = NativeTranscriptSearch(scope: searchScope, messages: snapshot.transcript)
    #expect(index.search("public").matches.map(\.messageID) == ["conversation:A:dialogue:turn-1:user", "conversation:A:dialogue:turn-1:response"])
    #expect(index.search("older").matches.isEmpty)
}

@Test func transcriptSearchLongMessagesAreCompleteWithinExplicitBoundsAndExcerptsStaySmall() throws {
    let text = String(repeating: "a", count: 1_000_000) + " target " + String(repeating: "b", count: 1_000_000)
    let index = NativeTranscriptSearch(scope: searchScope, messages: searchFixture([text]))
    let match = try #require(index.search("target").matches.first)
    #expect(match.range.location == 1_000_001)
    #expect(match.excerpt.utf16.count < 200)
    #expect(index.resolve(match) == "conversation:A:0")
    #expect(!index.limited)
    let bounded = NativeTranscriptSearch(scope: searchScope, messages: searchFixture([String(repeating: "x", count: NativeTranscriptSearch.maximumTextBytes + 1), "target"]))
    #expect(bounded.limited)
    #expect(bounded.search("target").matches.count == 1)
    let repeated = NativeTranscriptSearch(scope: searchScope, messages: searchFixture([String(repeating: "match ", count: 501)])).search("match")
    #expect(repeated.matches.count == 500); #expect(repeated.limited)
}

@Test func transcriptSearchRejectsStaleGenerationConversationDeletionAndChangedContent() throws {
    let messages = searchFixture(["original target", "reply"])
    let index = NativeTranscriptSearch(scope: searchScope, messages: messages)
    let match = try #require(index.search("target").matches.first)
    #expect(NativeTranscriptSearch.revealTarget(match, scope: searchScope, messages: messages) == messages[0].id)
    #expect(NativeTranscriptSearch.revealTarget(match, scope: .init(context: searchScope.context, generation: UUID()), messages: messages) == nil)
    #expect(NativeTranscriptSearch.revealTarget(match, scope: .init(context: "conversation:B", generation: searchScope.generation), messages: searchFixture(["original target"], conversation: "B")) == nil)
    #expect(NativeTranscriptSearch.revealTarget(match, scope: searchScope, messages: Array(messages.dropFirst())) == nil)
    #expect(NativeTranscriptSearch.revealTarget(match, scope: searchScope, messages: searchFixture(["changed target", "reply"])) == nil)
    #expect(NativeTranscriptSearch(scope: searchScope, messages: searchFixture(["changed target"])).resolve(match) == nil)
    #expect(NativeTranscriptSearch(scope: searchScope, messages: messages + searchFixture(["append"], conversation: "extra")).resolve(match) == messages[0].id)
}

@Test func transcriptSearchBenchmarks() throws {
    let clock = ContinuousClock()
    for count in [50, 500, 2_000] {
        let messages = searchFixture((0..<count).map { "Message \($0)\n" + String(repeating: "Deterministic ordinary prose. ", count: 30) + ($0.isMultiple(of: 20) ? " unique target" : "") })
        var index = NativeTranscriptSearch(scope: searchScope, messages: [])
        let construction = clock.measure { index = NativeTranscriptSearch(scope: searchScope, messages: messages) }
        var results = index.search("")
        let query = clock.measure { for _ in 0..<20 { results = index.search("target") } }
        let absent = clock.measure { for _ in 0..<20 { _ = index.search("not found anywhere") } }
        #expect(results.matches.count == (count + 19) / 20)
        let match = try #require(results.matches.last)
        let selection = clock.measure { for _ in 0..<1_000 { #expect(index.resolve(match) == match.messageID) } }
        let liveSelection = clock.measure { for _ in 0..<100 { #expect(NativeTranscriptSearch.revealTarget(match, scope: searchScope, messages: messages) == match.messageID) } }
        print("GATOR_P01_BENCH messages=\(count) build=\(construction) query20=\(query) absent20=\(absent) resolve1000=\(selection) liveResolve100=\(liveSelection)")
    }
}
