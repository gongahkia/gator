import Foundation
import Testing
@testable import GatorCore

@Test func framingHandlesSplitUTF8AndCoalescedFrames() throws {
    let data = Data("{\"version\":1,\"type\":\"state\",\"state\":{\"title\":\"Hi 🐊 海\"}}\n{\"version\":1,\"type\":\"action_applied\",\"nonce\":\"one\"}\n".utf8)
    var framer = BridgeFramer(), events: [BridgeEvent] = []
    for byte in data { events += try framer.append(Data([byte])) }
    #expect(events.count == 2)
    guard case .state(let state) = events[0] else { Issue.record("Expected state"); return }
    #expect(state.title == "Hi 🐊 海")
    #expect(events[1] == .acknowledged("one"))
    var combined = BridgeFramer(); #expect(try combined.append(data) == events)
    try framer.finish()
}

@Test func framingRejectsMalformedOversizedUnsupportedAndPartialEOF() throws {
    for text in ["nope\n", "{\"version\":2,\"type\":\"state\",\"state\":{}}\n", "{\"version\":1,\"type\":\"state\",\"state\":[]}\n", "{\"version\":1,\"type\":\"other\"}\n", "{\"version\":1,\"type\":\"action_applied\"}\n"] {
        var framer = BridgeFramer()
        #expect(throws: (any Error).self) { try framer.append(Data(text.utf8)) }
    }
    var bounded = BridgeFramer(limit: 5)
    #expect(throws: (any Error).self) { try bounded.append(Data(repeating: 1, count: 6)) }
    var partial = BridgeFramer(); _ = try partial.append(Data("{\"version\":".utf8))
    #expect(throws: (any Error).self) { try partial.finish() }
}

@Test func outboundSubmissionUsesAdmissionContractAndBoundedText() throws {
    let bytes = try BridgeCommand.submission(id: "original", context: "conversation:a", text: "Hello 🐊").data(nonce: "transport")
    let value = try JSONDecoder().decode(JSONValue.self, from: bytes)
    #expect(value["version"].int == 1)
    #expect(value["upload"]["request_id"].string == "original")
    #expect(value["upload"]["files"] == .array([]))
    #expect(value["nonce"].string == "transport")
    #expect(throws: (any Error).self) { try BridgeCommand.submission(id: "x", context: "a", text: String(repeating: "海", count: 50_000)).data(nonce: "x") }
}

@Test func structuredNumbersAndContentRemainLiteralData() throws {
    let data = Data("{\"id\":1234567890123456789,\"body\":\"[click](file:///private)\"}".utf8)
    let value = try JSONDecoder().decode(JSONValue.self, from: data)
    #expect(value.pretty.contains("1234567890123456789"))
    #expect(value["body"].string == "[click](file:///private)")
}

@Test func retainedDialogueIsVisibleWithoutChangingApprovalMessageIndexes() {
    let state = sampleState(["messages": .array([.object(["role": .string("Gator"), "result": .object(["kind": .string("approval")])])]), "dialogue": .array([.object(["id": .string("retained-turn"), "sequence": .number(1), "text": .string("Earlier question"), "response": .string("Earlier answer")])])])
    #expect(state.messages.count == 1)
    #expect(state.messages[0].result["kind"].string == "approval")
    #expect(state.transcript.contains { $0.text == "Earlier question" })
    #expect(state.transcript.contains { $0.text == "Earlier answer" })
    #expect(Set(state.transcript.map(\.id)).count == state.transcript.count)
}
