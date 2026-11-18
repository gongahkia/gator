import Foundation
import Testing
@testable import GatorCore
@testable import GatorMac

@MainActor private func findFixture(_ texts: [String], context: String = "A") -> [BridgeMessage] {
    BridgeSnapshot(.object(["conversation_id": .string(context), "messages": .array(texts.map { .object(["role": .string("Gator"), "text": .string($0)]) })])).messages
}
@Test @MainActor func transcriptFindQuerySelectionWrapAndCloseKeepReadingAnchorIntent() throws {
    let find = NativeTranscriptFind(), positions = NativeReadingPositions()
    let scope = NativeTranscriptSearch.Scope(context: "conversation:A", generation: UUID()), messages = findFixture(["target", "target reply"])
    positions.remember("conversation:B", anchor: "B:old", following: false)
    find.open(scope: scope, messages: messages); find.query = "target"
    #expect(find.matches.count == 2); #expect(find.selected?.messageID == messages[0].id)
    find.move(1); #expect(find.selected?.messageID == messages[1].id)
    find.move(1); #expect(find.selected?.messageID == messages[0].id)
    find.move(-1)
    let match = try #require(find.reveal?.match)
    let id = try #require(NativeTranscriptSearch.revealTarget(match, scope: scope, messages: messages))
    positions.remember(scope.context, anchor: id, following: false)
    find.close()
    #expect(!find.isPresented); #expect(find.query.isEmpty); #expect(find.reveal == nil)
    #expect(positions.restored(scope.context, available: messages.map(\.id)) == .init(anchor: id, following: false))
    #expect(positions.restored("conversation:B", available: ["B:old"]).anchor == "B:old")
    positions.remember(scope.context, anchor: nil, following: true)
    #expect(positions.restored(scope.context, available: messages.map(\.id)).following)
}

@Test @MainActor func transcriptFindBackgroundSnapshotsDoNotRequestFocusOrNavigation() throws {
    let find = NativeTranscriptFind(), scope = NativeTranscriptSearch.Scope(context: "conversation:A", generation: UUID())
    find.open(scope: scope, messages: findFixture(["target", "other"]))
    find.query = "target"
    let focus = find.focusRequest, reveal = find.reveal
    find.refresh(scope: scope, messages: findFixture(["target", "other", "target appended"]))
    #expect(find.matches.count == 2); #expect(find.focusRequest == focus); #expect(find.reveal == reveal)
    let stale = try #require(find.selected)
    find.refresh(scope: scope, messages: findFixture(["target changed", "other"]))
    #expect(find.selected == nil); #expect(find.reveal == nil)
    find.select(stale); #expect(find.selected == nil); #expect(find.reveal == nil)
}

@Test @MainActor func transcriptFindConversationSwitchAndReconnectRejectStaleSelections() throws {
    let find = NativeTranscriptFind(), generation = UUID()
    find.open(scope: .init(context: "conversation:A", generation: generation), messages: findFixture(["target"]))
    find.query = "target"
    let stale = try #require(find.selected)
    find.refresh(scope: .init(context: "conversation:B", generation: generation), messages: findFixture(["target"], context: "B"))
    #expect(!find.isPresented); #expect(find.matches.isEmpty)
    find.open(scope: .init(context: "conversation:B", generation: generation), messages: findFixture(["target"], context: "B")); find.query = "target"
    let current = find.selected
    find.select(stale); #expect(find.selected == current)
    find.refresh(scope: .init(context: "conversation:B", generation: UUID()), messages: findFixture(["target"], context: "B"))
    #expect(!find.isPresented); #expect(find.reveal == nil)
}
