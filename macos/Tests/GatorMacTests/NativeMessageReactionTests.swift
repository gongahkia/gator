import AppKit
import SwiftUI
import Testing
@testable import GatorCore
@testable import GatorMac

private func reactionTurn(id: String = "turn-A", conversation: String = "chat-A", kind: String = "appreciation", overrides: [String: JSONValue] = [:], metadata: [String: JSONValue] = [:]) -> JSONValue {
    let greeting = kind == "greeting"
    var reaction: [String: JSONValue] = ["version": .number(1), "conversation_id": .string(conversation), "turn_id": .string(id), "revision": .number(3), "kind": .string(kind), "emoji": .string(greeting ? "👋" : "❤️"), "level": .string(greeting ? "expressive" : "subtle")]
    reaction.merge(metadata) { _, new in new }
    var value: [String: JSONValue] = ["id": .string(id), "revision": .number(3), "sequence": .number(1), "text": .string(greeting ? "Hello!" : "Thanks!"), "response": .string(greeting ? "Hello!" : "You're welcome."), "state": .string("answered"), "reaction": .object(reaction), "created_at": .string("2026-11-20T00:00:00Z")]
    value.merge(overrides) { _, new in new }; return .object(value)
}
private func reactionSnapshot(_ turns: [JSONValue], conversation: String = "chat-A") -> BridgeSnapshot {
    .init(.object(["conversation_id": .string(conversation), "dialogue": .array(turns)]))
}

@Test func messageReactionBindsExactConversationTurnAndRevision() throws {
    let snapshot = reactionSnapshot([reactionTurn()])
    let user = try #require(snapshot.transcript.first)
    let reaction = try #require(user.messageReaction)
    #expect(user.id == "conversation:chat-A:dialogue:turn-A:user")
    #expect(reaction.id == "chat-A:turn-A:3" && reaction.emoji == "❤️")
    #expect(user.text == "Thanks!" && snapshot.transcript[1].text == "You're welcome.")
    #expect(reactionSnapshot([reactionTurn()], conversation: "chat-B").transcript.first?.messageReaction == nil)
    for fields: [String: JSONValue] in [["turn_id": .string("other")], ["conversation_id": .string("other")], ["revision": .number(2)], ["revision": .number(3.5)], ["version": .number(2)], ["kind": .string("approval")], ["emoji": .string("✅")], ["level": .string("unknown")]] {
        #expect(reactionSnapshot([reactionTurn(metadata: fields)]).transcript.first?.messageReaction == nil)
    }
}
@Test func messageReactionRejectsUnconfirmedFailedAndMalformedOutcomes() {
    for fields: [String: JSONValue] in [["state": .string("routing")], ["state": .string("failed")], ["state": .string("received")], ["response": .string("")], ["response": .string("The email was sent.")], ["issue": .string("Unconfirmed")], ["task_chat": .string("child")], ["input_ids": .array([.string("upload")])], ["reply_to_update_id": .string("update")]] {
        let messages = reactionSnapshot([reactionTurn(overrides: fields)]).transcript
        #expect(messages.first?.messageReaction == nil)
        #expect(NativeQuickReactionMode.transcript(messages, mode: .expressive).count == messages.count)
    }
}
@Test func messageReactionModesPreserveFallbackAndStableOriginalIdentity() throws {
    let raw = reactionSnapshot([reactionTurn(), reactionTurn(id: "turn-B", kind: "greeting")]).transcript
    let subtle = NativeQuickReactionMode.transcript(raw, mode: .subtle)
    let expressive = NativeQuickReactionMode.transcript(raw, mode: .expressive)
    #expect(NativeQuickReactionMode.transcript(raw, mode: .off) == raw)
    #expect(subtle.map(\.id) == [raw[0].id, raw[2].id, raw[3].id])
    #expect(expressive.map(\.id) == [raw[0].id, raw[2].id])
    #expect(subtle[0].text == raw[0].text && expressive[0].id == raw[0].id)
    #expect(NativeQuickReactionMode.resolve(nil) == .subtle && NativeQuickReactionMode.resolve("future") == .subtle)
    #expect(try #require(raw[0].messageReaction).accessibilityLabel.contains("appreciation"))
}
@Test @MainActor func messageReactionPreferencePersistsWithoutChangingPresenceOrStreaming() throws {
    let name = "gator.reactions.test." + UUID().uuidString
    let defaults = try #require(UserDefaults(suiteName: name)); defer { defaults.removePersistentDomain(forName: name) }
    for key in [NativeMotionPreference.streaming, NativeMascotPreference.animated, NativeMascotPreference.shown, NativeDesktopPetPreference.shown] { defaults.set(false, forKey: key) }
    for mode in NativeQuickReactionMode.allCases {
        defaults.set(mode.rawValue, forKey: NativeQuickReactionMode.preference)
        let reopened = try #require(UserDefaults(suiteName: name))
        #expect(NativeQuickReactionMode.resolve(reopened.string(forKey: NativeQuickReactionMode.preference)) == mode)
        for key in [NativeMotionPreference.streaming, NativeMascotPreference.animated, NativeMascotPreference.shown, NativeDesktopPetPreference.shown] { #expect(!reopened.bool(forKey: key)) }
    }
}
@Test @MainActor func messageReactionSearchAndReadingAnchorsFollowPresentedMessagesOnly() throws {
    let raw = reactionSnapshot([reactionTurn()]).transcript
    let visible = NativeQuickReactionMode.transcript(raw, mode: .subtle)
    let scope = NativeTranscriptSearch.Scope(context: "conversation:chat-A", generation: UUID())
    let search = NativeTranscriptSearch(scope: scope, messages: visible)
    #expect(search.search("Thanks").matches.count == 1 && search.search("welcome").matches.isEmpty)
    #expect(NativeTranscriptSearch(scope: scope, messages: raw).search("welcome").matches.count == 1)
    let reading = NativeReadingPositions()
    reading.remember(scope.context, anchor: raw[1].id, following: false)
    let saved = reading.restored(scope.context, available: raw.map(\.id))
    let presented = NativeQuickReactionMode.position(saved, raw: raw, mode: .subtle)
    #expect(presented.anchor == raw[0].id && !presented.following)
    #expect(reading.restored(scope.context, available: raw.map(\.id)) == saved)
    #expect(NativeQuickReactionMode.position(saved, raw: raw, mode: .off) == saved)
    let find = NativeTranscriptFind(); find.open(scope: scope, messages: visible); find.query = "Thanks"
    #expect(find.selected?.messageID == raw[0].id)
    find.refresh(scope: scope, messages: reactionSnapshot([reactionTurn()], conversation: "chat-B").transcript)
    #expect(find.selected == nil)
}
@Test @MainActor func messageReactionDoesNotReplayOrMutateAnUnrelatedStream() {
    let presentation = NativeStreamingPresentation()
    let entry = NativePublicText(id: "selected:generation:run:entry", text: "Public", active: true, hasExactIdentity: true)
    presentation.receive(entry, policy: .init(), now: 1)
    presentation.receive(.init(id: entry.id, text: "Public response arriving", active: true, hasExactIdentity: true), policy: .init(), now: 2)
    let before = presentation.cursor.visible
    _ = NativeQuickReactionMode.transcript(reactionSnapshot([reactionTurn()]).transcript, mode: .subtle)
    #expect(presentation.cursor.visible == before && presentation.scheduled)
    presentation.hide()
}
@Test @MainActor func messageReactionNativeChipFitsCompactAndAllNativeAppearances() throws {
    let reaction = try #require(reactionSnapshot([reactionTurn()]).transcript.first?.messageReaction)
    for name in [NSAppearance.Name.aqua, .darkAqua, .accessibilityHighContrastAqua, .accessibilityHighContrastDarkAqua] {
        for reduced in [false, true] {
            let host = NSHostingView(rootView: NativeMessageReactionChip(reaction: reaction).transaction { $0.disablesAnimations = reduced })
            host.appearance = try #require(NSAppearance(named: name))
            let size = host.fittingSize
            #expect(size.width >= 20 && size.width < 60 && size.height >= 16 && size.height < 45)
        }
    }
}

private var reactionEvidence: URL {
    URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("reactions/evidence/p04e-2")
}
@Test @MainActor func messageReactionActualRowLayoutAndHeadlessCaptures() throws {
    let name = "gator.reaction.render." + UUID().uuidString
    let defaults = try #require(UserDefaults(suiteName: name)); defer { defaults.removePersistentDomain(forName: name) }
    defaults.set("subtle", forKey: NativeQuickReactionMode.preference)
    let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let sessions = ManagedSessionCoordinator(configuration: .init(runtime: root.appendingPathComponent("not-a-runtime"), workspace: root), nativeDraftDirectory: root.appendingPathComponent("drafts"), profile: "render")
    let laughter = reactionTurn(id: "turn-B", kind: "laughter", overrides: ["text": .string("Haha that's hilarious"), "response": .string("Glad that made you smile.")], metadata: ["emoji": .string("😂")])
    let messages = NativeQuickReactionMode.transcript(reactionSnapshot([reactionTurn(), laughter]).transcript, mode: .subtle)
    for dark in [false, true] {
        for width: CGFloat in [380, 780] {
            let view = VStack(alignment: .leading, spacing: 16) {
                Text("Gator · isolated reaction rows").font(.headline)
                ForEach(messages) { message in MessageRow(message: message) }
            }.padding(20).frame(width: width).background(Color(nsColor: .windowBackgroundColor))
                .environment(sessions).defaultAppStorage(defaults).environment(\.colorScheme, dark ? .dark : .light)
            let host = NSHostingView(rootView: view)
            host.appearance = try #require(NSAppearance(named: dark ? .darkAqua : .aqua))
            host.frame = CGRect(origin: .zero, size: host.fittingSize); host.layoutSubtreeIfNeeded()
            #expect(host.fittingSize.height < 380 && host.fittingSize.width <= width + 1)
            if ProcessInfo.processInfo.environment["GATOR_REACTION_CAPTURE"] == "1" {
                let bitmap = try #require(host.bitmapImageRepForCachingDisplay(in: host.bounds))
                host.cacheDisplay(in: host.bounds, to: bitmap)
                try #require(bitmap.representation(using: .png, properties: [:])).write(to: reactionEvidence.appendingPathComponent("rows-\(Int(width))-\(dark ? "dark" : "light").png"))
            }
        }
    }
    #expect(sessions.ownerCount == 1 && !sessions.current.started && !FileManager.default.fileExists(atPath: root.path))
}

@MainActor @Observable private final class ReactionResourceInput {
    var messages: [BridgeMessage] = []
    var mode = NativeQuickReactionMode.subtle
    var visible = true
}
private struct ReactionResourceRows: View {
    let input: ReactionResourceInput
    var body: some View {
        ScrollView {
            LazyVStack(spacing: 16) {
                if input.visible {
                    ForEach(NativeQuickReactionMode.transcript(input.messages, mode: input.mode)) { MessageRow(message: $0).equatable() }
                }
            }.padding(20)
        }.frame(width: 540, height: 640)
    }
}
@Test @MainActor func messageReactionResourceTrial() async throws {
    guard let mode = ProcessInfo.processInfo.environment["GATOR_REACTION_BENCH"], ["before", "after"].contains(mode) else { return }
    let domain = "gator.reaction.resources." + UUID().uuidString
    let defaults = try #require(UserDefaults(suiteName: domain)); defer { defaults.removePersistentDomain(forName: domain) }
    defaults.set("subtle", forKey: NativeQuickReactionMode.preference)
    let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let sessions = ManagedSessionCoordinator(configuration: .init(runtime: root.appendingPathComponent("not-a-runtime"), workspace: root), nativeDraftDirectory: root.appendingPathComponent("drafts"), profile: "bench")
    let input = ReactionResourceInput()
    func phase(_ name: String) throws {
        let value: [String: Any] = ["pid": ProcessInfo.processInfo.processIdentifier, "phase": name, "epoch": Date().timeIntervalSince1970]
        let file = reactionEvidence.appendingPathComponent("resource-\(mode)-phases.jsonl")
        if !FileManager.default.fileExists(atPath: file.path) { try Data().write(to: file) }
        let handle = try FileHandle(forWritingTo: file); defer { try? handle.close() }
        try handle.seekToEnd(); try handle.write(contentsOf: JSONSerialization.data(withJSONObject: value) + Data([10]))
    }
    let host = NSHostingView(rootView: ReactionResourceRows(input: input).environment(sessions).defaultAppStorage(defaults))
    host.frame = CGRect(x: 0, y: 0, width: 540, height: 640)
    try phase("empty-host"); try await Task.sleep(for: .seconds(3))
    let turns = (0..<1000).map { index in reactionTurn(id: "turn-\(index)", overrides: ["sequence": .number(Decimal(index + 1))]) }
    let began = ContinuousClock.now
    let raw = reactionSnapshot(turns).transcript
    let reload = ContinuousClock.now - began
    var times: [Double] = []
    for _ in 0..<20 {
        let start = ContinuousClock.now
        #expect(NativeQuickReactionMode.transcript(raw, mode: .subtle).count == 1000)
        let duration = ContinuousClock.now - start; times.append(Double(duration.components.attoseconds) / 1e15 + Double(duration.components.seconds) * 1000)
    }
    let users = raw.filter(\.isUser)
    input.messages = users.map { message in
        guard mode == "before", case .object(var fields) = message.value else { return message }
        fields.removeValue(forKey: "reaction_turn"); fields.removeValue(forKey: "reaction_conversation")
        return BridgeMessage(id: message.id, value: .object(fields))
    }
    let insertion = ContinuousClock.now
    await Task.yield(); host.layoutSubtreeIfNeeded()
    let layout = ContinuousClock.now - insertion
    try phase("visible-static"); try await Task.sleep(for: .seconds(4))
    try phase("preference-switch-stress")
    for index in 0..<12 {
        input.mode = index.isMultiple(of: 2) ? .off : .subtle
        defaults.set(input.mode.rawValue, forKey: NativeQuickReactionMode.preference)
        await Task.yield(); host.layoutSubtreeIfNeeded(); try await Task.sleep(for: .milliseconds(150))
    }
    try phase("settled"); try await Task.sleep(for: .seconds(3))
    input.visible = false; await Task.yield(); host.layoutSubtreeIfNeeded()
    try phase("hidden-component"); try await Task.sleep(for: .seconds(3))
    input.messages = []; await Task.yield()
    try phase("released-content"); try await Task.sleep(for: .seconds(2))
    try phase("end")
    #expect(sessions.ownerCount == 1 && !sessions.current.started)
    func ms(_ duration: Duration) -> Double { Double(duration.components.seconds) * 1000 + Double(duration.components.attoseconds) / 1e15 }
    let metrics: [String: Any] = ["mode": mode, "userRows": 1000, "reloadProjectionMilliseconds": ms(reload), "reactionFilterMedianMilliseconds": times.sorted()[times.count / 2], "hostedInsertionMilliseconds": ms(layout), "additionalGoOwners": 0, "scope": "headless NSHostingView; no compositor/physical scrolling or typing measurement"]
    try JSONSerialization.data(withJSONObject: metrics, options: [.prettyPrinted, .sortedKeys]).write(to: reactionEvidence.appendingPathComponent("resource-\(mode)-layout.json"))
}
