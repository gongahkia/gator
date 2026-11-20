import AppKit
import SwiftUI
import Testing
@testable import GatorMac
@testable import GatorCore

private var presenceRoot: URL {
    URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("Resources/Presence")
}
@Test func mascotStatePrecedenceAndOwnerReplacement() {
    var state=NativeMascotStateMachine()
    state.receive(.init(owner:"A",connected:true,completedRun:"old"));#expect(state.expression == .neutral)
    state.receive(.init(owner:"A",connected:true,working:true));#expect(state.expression == .working)
    state.receive(.init(owner:"A",connected:true,working:true,publicTextRevision:12));#expect(state.expression == .responding)
    state.receive(.init(owner:"A",connected:true,working:true,attention:true,publicTextRevision:14));#expect(state.expression == .attention)
    state.receive(.init(owner:"A",connected:false,working:true,attention:true));#expect(state.expression == .disconnected)
    state.receive(.init(owner:"A",connected:true,completedRun:"new-confirmed"));#expect(state.expression == .completed)
    state.receive(.init(owner:"B",connected:true,completedRun:"new-confirmed"));#expect(state.expression == .neutral)
}
@Test func mascotDoesNotCelebrateUnconfirmedOrFailedWork() {
    var state=NativeMascotStateMachine()
    state.receive(.init(owner:"A",connected:true,working:true));state.receive(.init(owner:"A",connected:true))
    #expect(state.expression == .neutral)
    state.receive(.init(owner:"A",connected:true,working:true,publicTextRevision:100))
    state.receive(.init(owner:"A",connected:true,working:true,publicTextRevision:4))
    #expect(state.expression == .responding)
}
@Test func mascotProjectionReadsOnlyPublicSelectedOwnerContent() {
    let snapshot=BridgeSnapshot(.object(["running":.bool(true),"activity":.object(["entries":.array([
        .object(["kind":.string("text"),"text":.string("Public")]),.object(["kind":.string("reasoning"),"text":.string("PRIVATE DO NOT ANIMATE")]),.object(["kind":.string("tool"),"arguments":.string("PRIVATE")])
    ])]),"pending_approval":.object(["token":.string("EXACT DO NOT USE")])]))
    let signal=NativeMascotProjection.signal(snapshot:snapshot,messages:[],owner:"selected",connected:true,reviewing:false)
    #expect(signal.owner=="selected" && signal.working && signal.attention && signal.publicTextRevision==6 && signal.completedRun.isEmpty)
}
@Test func mascotCompletionRequiresCanonicalFinishedOutcome() {
    func signal(_ state:String,_ finished:String)->NativeMascotSignal{
        let message=BridgeMessage(id:"message",value:.object(["run_id":.string("run"),"activity":.object(["state":.string(state),"finished_at":.string(finished)])]))
        return NativeMascotProjection.signal(snapshot:nil,messages:[message],owner:"A",connected:true,reviewing:false)
    }
    #expect(signal("completed","").completedRun.isEmpty)
    #expect(signal("failed","at").completedRun.isEmpty)
    #expect(signal("completed","at").completedRun=="run:at")
}
@Test @MainActor func mascotProjectionDoesNotStartAnOwnerOrMutateItsDraft() {
    let folder=FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    let app=GatorApplicationState(configuration:.init(runtime:URL(fileURLWithPath:"/not/a/runtime"),workspace:folder),nativeDraftDirectory:folder,profile:"mascot-test")
    _=NativeMascotProjection.signal(app)
    #expect(!app.started && app.connectionStatus == .idle && app.draftText.isEmpty && app.pendingSubmission == nil)
    #expect(!FileManager.default.fileExists(atPath:folder.path))
}
@Test func mascotCollapseDoesNotMutateReadingAnchors() {
    #expect(NativeMascotStateMachine.collapsed(reading:true,reviewing:false,compact:false,height:740))
    #expect(NativeMascotStateMachine.collapsed(reading:false,reviewing:true,compact:true,height:620))
    #expect(NativeMascotStateMachine.collapsed(reading:false,reviewing:false,compact:false,height:480))
    #expect(!NativeMascotStateMachine.collapsed(reading:false,reviewing:false,compact:true,height:620))
}

@Test func presenceModesAndEmojiAreNativeAndDoNotOfferLegacyArt() {
    #expect(NativePresenceMode.allCases.map(\.label) == ["Emoji only", "Emoji + GIF reactions", "Off"])
    for obsolete in ["rendered", "classic", "live-3d", "invalid"] { #expect(NativeMascotPreference.presence(obsolete) == .emoji) }
    #expect(NativeMascotExpression.neutral.emoji == "🐊")
    #expect(NativeMascotExpression.working.emoji == "⚙️")
    #expect(NativeMascotExpression.responding.emoji == "📝")
    #expect(NativeMascotExpression.failed.emoji == "⚠️")
    #expect(NativeMascotExpression.attention.emoji == "👀")
    #expect(NativeMascotExpression.completed.emoji == "✅")
    #expect(NativeMascotExpression.disconnected.emoji == "🔌")
}

@Test @MainActor func presenceMigrationPreservesOffAnimationTuckAndObsoleteStyle() throws {
    let name = "gator.presence.test." + UUID().uuidString
    let defaults = try #require(UserDefaults(suiteName: name)); defer { defaults.removePersistentDomain(forName: name) }
    defaults.set(false, forKey: NativeMascotPreference.shown)
    defaults.set(false, forKey: NativeMascotPreference.animated)
    defaults.set(false, forKey: NativeDesktopPetPreference.shown)
    defaults.set("classic", forKey: "gator.mascot.style")
    let before = defaults.persistentDomain(forName: name)
    #expect(NativeMascotPreference.presence(defaults.string(forKey: NativeMascotPreference.mode)) == .emoji)
    #expect(defaults.persistentDomain(forName: name) as NSDictionary? == before as NSDictionary?)
    struct Preferences {
        @AppStorage var mode: String
        @AppStorage var chat: Bool
        @AppStorage var animated: Bool
        init(_ defaults: UserDefaults) {
            _mode = AppStorage(wrappedValue: NativePresenceMode.emoji.rawValue, NativeMascotPreference.mode, store: defaults)
            _chat = AppStorage(wrappedValue: true, NativeMascotPreference.shown, store: defaults)
            _animated = AppStorage(wrappedValue: true, NativeMascotPreference.animated, store: defaults)
        }
    }
    let migrated = Preferences(defaults)
    #expect(!migrated.chat && !migrated.animated && migrated.mode == "emoji")
    for mode in NativePresenceMode.allCases {
        migrated.mode = mode.rawValue
        #expect(Preferences(try #require(UserDefaults(suiteName: name))).mode == mode.rawValue)
        #expect(!defaults.bool(forKey: NativeDesktopPetPreference.shown))
    }
    #expect(defaults.string(forKey: "gator.mascot.style") == "classic")
}

@Test func presenceUsesLatestConfirmedFailureInsteadOfOlderSuccess() {
    func message(_ state: String, _ run: String, _ date: String) -> BridgeMessage {
        .init(id: run, value: .object(["run_id": .string(run), "activity": .object(["state": .string(state), "finished_at": .string(date)])]))
    }
    let signal = NativeMascotProjection.signal(snapshot: nil, messages: [message("completed", "old", "a"), message("failed", "new", "b")], owner: "A", connected: true, reviewing: false)
    #expect(signal.completedRun.isEmpty && signal.failedRun == "new:b")
    var state = NativeMascotStateMachine(); state.receive(signal)
    #expect(state.expression == .failed)
    state.receive(.init(owner: "A", connected: true, working: true, failedRun: "new:b")); #expect(state.expression == .working)
    state.receive(.init(owner: "A", connected: true, attention: true, failedRun: "new:b")); #expect(state.expression == .attention)
    let unconfirmed = NativeMascotProjection.signal(snapshot: nil, messages: [message("completed", "", "date"), message("failed", "new", "")], owner: "A", connected: true, reviewing: false)
    #expect(unconfirmed.completedRun.isEmpty && unconfirmed.failedRun.isEmpty)
}

@Test func presenceReactionRevisionRequiresNewSelectedOwnerEvidence() {
    var state = NativeMascotStateMachine()
    state.receive(.init(owner: "A", connected: true, attention: true)); #expect(state.revision == 0)
    state.receive(.init(owner: "A", connected: true))
    state.receive(.init(owner: "A", connected: true, attention: true)); #expect(state.revision == 1)
    state.receive(.init(owner: "B", connected: true, completedRun: "historical")); #expect(state.revision == 0 && state.expression == .neutral)
    state.receive(.init(owner: "B", connected: true, completedRun: "confirmed-new")); #expect(state.revision == 1 && state.expression == .completed)
    state.receive(.init(owner: "B", connected: true, completedRun: "confirmed-new")); #expect(state.revision == 1)
}

@Test @MainActor func presenceSmallStableSlotsPreserveExactReadingAnchors() {
    let positions = NativeReadingPositions(); positions.remember("A", anchor: "A:17", following: false)
    for compact in [false, true] {
        let normal = NativePresenceLayout(compact: compact, collapsed: false)
        let reading = NativePresenceLayout(compact: compact, collapsed: true)
        #expect(normal.slot == reading.slot && normal.slot <= 48 && reading.size < normal.size)
        for _ in NativePresenceMode.allCases {
            #expect(positions.restored("A", available: ["A:17", "A:18"]).anchor == "A:17")
            #expect(!positions.restored("A", available: ["A:17"]).following)
        }
    }
}

@Test func presenceGIFBoundsCacheAndAlpha() async throws {
    let store = NativeMascotFrameStore(root: presenceRoot)
    for (index, name) in ["completed", "attention"].enumerated() {
        let frames = try await store.load(name: name, ticket: index)
        #expect(frames.images.count == 12 && frames.bytes <= NativeMascotFrameStore.maximumBytes)
        #expect(abs(frames.duration - 0.72) < 0.001)
        #expect(frames.images.allSatisfy { $0.width == 96 && $0.height == 96 && $0.alphaInfo != .none })
        #expect(await store.bytes == frames.bytes)
        let count = await store.decodeCount
        _ = try await store.load(name: name, ticket: index)
        #expect(await store.decodeCount == count)
    }
    await store.clear(ticket: 3); #expect(await store.bytes == 0)
    await #expect(throws: CancellationError.self) { try await store.load(name: "completed", ticket: 2) }
    await #expect(throws: (any Error).self) { try await store.load(name: "../Mascot", ticket: 4) }
}

@Test @MainActor func presenceGIFIsFiniteAndCancellationClearsEveryFrame() async throws {
    let frames = try await NativeMascotFrameStore(root: presenceRoot).load(name: "completed", ticket: 1)
    let animation = NativeMascotPlayer.keyframes(frames)
    #expect(animation.repeatCount == 0 && animation.values?.count == 12 && animation.keyTimes?.first == 0 && animation.keyTimes?.last == 1)
    var installed: CAKeyframeAnimation?
    let player = NativeMascotPlayer(root: presenceRoot, installAnimation: { layer, animation in
        installed = animation; layer.add(animation, forKey: "gator.expression")
    })
    player.configure(.init(owner: "A", mode: .reactions, expression: .working)); await player.settled()
    player.configure(.init(owner: "A", mode: .reactions, expression: .completed, revision: 1)); await player.settled()
    // unattached headless layers can discard their animations at transaction commit.
    #expect(player.animationStarts == 1 && player.activeFrameBytes > 0)
    #expect(installed?.values?.count == 12 && installed?.repeatCount == 0 && abs((installed?.duration ?? 0) - 0.72) < 0.001)
    player.stop(); await player.settled()
    #expect(player.layer.animationKeys() == nil && player.layer.contents == nil && player.activeFrameBytes == 0)
    #expect(await player.store.bytes == 0)
    player.configure(.init(owner: "A", mode: .reactions, expression: .completed, revision: 1)); await player.settled()
    #expect(player.animationStarts == 1)
}

@Test @MainActor func presenceStaticReduceMotionAndOffNeverDecodeAnAsset() async {
    for request in [NativeMascotRequest(owner: "A", expression: .completed), .init(owner: "A", mode: .off, expression: .attention), .init(owner: "A", mode: .reactions, expression: .attention, reduceMotion: true), .init(owner: "A", mode: .reactions, expression: .completed, animate: false)] {
        let player = NativeMascotPlayer(root: presenceRoot)
        var old = request; old.expression = .working; player.configure(old); await player.settled()
        var fresh = request; fresh.revision = 1; player.configure(fresh); await player.settled()
        #expect(player.layer.animationKeys() == nil && player.animationStarts == 0)
        #expect(await player.store.decodeCount == 0 && player.activeFrameBytes == 0)
    }
}

@Test @MainActor func presenceOwnerSwitchStateChangeAndStaleDecodeCannotReplay() async {
    let player = NativeMascotPlayer(root: presenceRoot)
    player.configure(.init(owner: "A", mode: .reactions, expression: .working)); await player.settled()
    player.configure(.init(owner: "A", mode: .reactions, expression: .completed, revision: 1))
    player.configure(.init(owner: "B", mode: .reactions, expression: .attention, revision: 10)); await player.settled()
    #expect(player.current?.owner == "B" && player.layer.animationKeys() == nil && player.activeFrameBytes == 0)
    #expect(await player.store.bytes == 0)
}

@Test @MainActor func presenceMissingAndEmptyGIFsKeepNativeEmojiFallback() async throws {
    let root = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true); defer { try? FileManager.default.removeItem(at: root) }
    for content in [Data(), Data("invalid gif".utf8), Data(repeating: 0, count: NativeMascotFrameStore.maximumFileBytes + 1)] {
        try content.write(to: root.appendingPathComponent("completed.gif"))
        let player = NativeMascotPlayer(root: root)
        player.configure(.init(owner: "A", mode: .reactions, expression: .working)); await player.settled()
        player.configure(.init(owner: "A", mode: .reactions, expression: .completed, revision: 1)); await player.settled()
        #expect(player.current?.expression.emoji == "✅" && player.animationStarts == 0 && player.layer.animationKeys() == nil)
        #expect(await player.store.bytes == 0)
    }
}

@Test @MainActor func presenceRepeatedLifetimesAndCanvasNeverTakeFocus() async {
    for _ in 0..<30 {
        weak var probe: NativeMascotPlayer?
        do {
            let canvas = NativeMascotCanvas(root: presenceRoot); probe = canvas.player
            #expect(!canvas.acceptsFirstResponder && !canvas.emoji.isSelectable)
            canvas.request = .init(owner: "A", mode: .reactions, expression: .working)
            #expect(canvas.emoji.stringValue.isEmpty)
            canvas.detach(); await canvas.player.settled()
            #expect(await canvas.player.store.bytes == 0)
        }
        #expect(probe == nil)
    }
}

@Test func presenceProductionHasNoOldArtLoadingOrExecutionCapability() throws {
    let root = presenceRoot.deletingLastPathComponent().deletingLastPathComponent()
    for name in ["NativeMascotModel.swift", "NativeMascotPlayback.swift", "NativeMascotView.swift", "NativeMascotPreferences.swift", "NativeDesktopPet.swift"] {
        let source = try String(contentsOf: root.appendingPathComponent("Sources/GatorMac/" + name), encoding: .utf8)
        for forbidden in ["import RealityKit", "import SceneKit", "import WebKit", ".send(", ".newConversation(", ".respondToApproval(", "00.png", "NativeMascotStyle", "appendingPathComponent(\"Mascot\")"] { #expect(!source.contains(forbidden)) }
    }
    let build = try String(contentsOf: root.appendingPathComponent("scripts/build-dev-app.sh"), encoding: .utf8)
    #expect(!build.contains("mascot/2d/assets") && build.contains("Resources/Presence"))
}

@Test @MainActor func presenceReduceMotionStyleOffAndStateChangesCancelAnInFlightReaction() async {
    for change in ["reduce", "off", "state", "hide", "animation"] {
        let player = NativeMascotPlayer(root: presenceRoot)
        player.configure(.init(owner: "A", mode: .reactions, expression: .working)); await player.settled()
        var request = NativeMascotRequest(owner: "A", mode: .reactions, expression: .completed, revision: 1)
        player.configure(request); await player.settled(); #expect(player.activeFrameBytes > 0)
        switch change {
        case "reduce": request.reduceMotion = true
        case "off": request.mode = .off
        case "state": request.expression = .working
        case "hide": request.visible = false
        default: request.animate = false
        }
        player.configure(request); await player.settled()
        #expect(player.layer.animationKeys() == nil && player.activeFrameBytes == 0 && player.layer.contents == nil)
        #expect(await player.store.bytes == 0)
    }
}
