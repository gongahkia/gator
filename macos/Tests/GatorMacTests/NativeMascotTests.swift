import AppKit
import SwiftUI
import Testing
@testable import GatorMac
@testable import GatorCore

private var mascotAssetRoot: URL {
    URL(fileURLWithPath: #filePath).deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("mascot/2d/assets")
}
@Test func mascotOnlyOffersTheTwoSelectedAppearances() {
    #expect(NativeMascotStyle.allCases.map(\.rawValue) == ["rendered","classic"])
    #expect(NativeMascotStyle.allCases.map(\.label) == ["3D-rendered","Classic 2D"])
    #expect(NativeMascotPreference.appearance("live-3d") == .rendered)
}
@Test @MainActor func mascotPreferencesPersistUsingNativeAppStorage() throws {
    let name="gator.mascot.test."+UUID().uuidString
    let defaults=try #require(UserDefaults(suiteName:name)); defer{defaults.removePersistentDomain(forName:name)}
    struct Preferences {
        @AppStorage var shown: Bool
        @AppStorage var animated: Bool
        @AppStorage var style: String
        init(_ defaults: UserDefaults) {
            _shown=AppStorage(wrappedValue:true,NativeMascotPreference.shown,store:defaults)
            _animated=AppStorage(wrappedValue:true,NativeMascotPreference.animated,store:defaults)
            _style=AppStorage(wrappedValue:NativeMascotStyle.rendered.rawValue,NativeMascotPreference.style,store:defaults)
        }
    }
    let first=Preferences(defaults);#expect(first.shown && first.animated)
    first.shown=false;first.animated=false;first.style="classic"
    let reopened=Preferences(defaults)
    #expect(!reopened.shown && !reopened.animated && reopened.style=="classic")
    #expect((defaults.persistentDomain(forName:name) ?? [:]).keys.filter{$0.hasPrefix("gator.")}.sorted()==[NativeMascotPreference.animated,NativeMascotPreference.shown,NativeMascotPreference.style].sorted())
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
@Test @MainActor func mascotReadingAnchorRemainsExactAcrossAppearanceChanges() {
    let positions=NativeReadingPositions();positions.remember("A",anchor:"A:17",following:false)
    for style in NativeMascotStyle.allCases {
        _=NativeMascotRequest(owner:"A",style:style,expression:.working,animate:false)
        #expect(positions.restored("A",available:["A:17","A:18"]).anchor=="A:17")
        #expect(!positions.restored("A",available:["A:17"]).following)
    }
}
@Test func mascotFrameCacheIsBoundedAndStaticDoesNotDecodeTheClip() async throws {
    let store=NativeMascotFrameStore(root:mascotAssetRoot)
    let still=try await store.load(style:.rendered,name:"working",animate:false,ticket:1)
    #expect(still.images.count==1)
    let motion=try await store.load(style:.classic,name:"working",animate:true,ticket:2)
    #expect(motion.images.count==12 && motion.bytes<=NativeMascotFrameStore.maximumBytes)
    #expect(await store.bytes==motion.bytes)
    await store.clear(ticket:3);#expect(await store.bytes==0)
    await #expect(throws:CancellationError.self){try await store.load(style:.rendered,name:"blink",animate:true,ticket:2)}
}
@Test func mascotExportManifestMatchesFinitePlaybackSpecifications() throws {
    let manifest=try JSONDecoder().decode(JSONValue.self,from:Data(contentsOf:mascotAssetRoot.appendingPathComponent("manifest.json")))
    #expect(manifest["canonicalImportedMaterials"].int==26 && manifest["clips"].array.count==14)
    for clip in manifest["clips"].array {
        let style=try #require(NativeMascotStyle(rawValue:clip["style"].string))
        let spec=try #require(NativeMascotClip.spec(style:style,name:clip["state"].string))
        #expect(spec.count==clip["frames"].int)
        #expect(abs(spec.duration-(Double(clip["duration"].pretty) ?? -1))<0.001)
    }
}
@Test @MainActor func mascotAnimationUsesFiniteNativeKeyframes() async throws {
    let frames=try await NativeMascotFrameStore(root:mascotAssetRoot).load(style:.rendered,name:"responding",animate:true,ticket:1)
    let animation=NativeMascotPlayer.keyframes(frames)
    #expect(animation.values?.count==9 && animation.duration==0.55 && animation.repeatCount==0)
    #expect(animation.calculationMode == .discrete && animation.keyTimes?.first==0 && animation.keyTimes?.last==1)
}
@Test @MainActor func mascotHiddenAndReduceMotionCancelPlaybackAndReleaseFrames() async {
    let player=NativeMascotPlayer(root:mascotAssetRoot)
    player.configure(.init(owner:"A",style:.rendered,expression:.working));await player.settled()
    player.configure(.init(owner:"A",style:.classic,expression:.attention,reduceMotion:true));await player.settled()
    #expect(player.layer.animationKeys()==nil && player.layer.contents != nil)
    player.stop();await player.settled()
    #expect(player.layer.contents==nil && player.layer.animationKeys()==nil && player.activeFrameBytes==0)
    #expect(await player.store.bytes==0)
}
@Test @MainActor func mascotStaleDecodeAndHiddenResponsesCannotReplay() async {
    let player=NativeMascotPlayer(root:mascotAssetRoot)
    player.configure(.init(owner:"A",style:.rendered,expression:.working))
    player.configure(.init(owner:"B",style:.classic,expression:.disconnected,animate:false));await player.settled()
    #expect(player.current?.owner=="B" && player.displayedStyle == .classic && player.animationStarts==0)
    player.configure(.init(owner:"B",style:.classic,expression:.responding,revision:1,visible:false));await player.settled()
    player.configure(.init(owner:"B",style:.classic,expression:.responding,revision:1));await player.settled()
    #expect(player.animationStarts==0)
}
@Test @MainActor func mascotMissingAssetsRemainNonfatalAndDoNotLoop() async {
    let player=NativeMascotPlayer(root:mascotAssetRoot.appendingPathComponent("missing"))
    player.configure(.init(owner:"test",style:.rendered,expression:.working));await player.settled()
    #expect(player.layer.contents==nil && player.layer.animationKeys()==nil && player.animationStarts==0)
    #expect(await player.store.bytes==0)
}
@Test func mascotProductionSourceHasNoLive3DOrExecutionDependency() throws {
    let sources=mascotAssetRoot.deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent().appendingPathComponent("Sources/GatorMac")
    for name in ["NativeMascotModel.swift","NativeMascotPlayback.swift","NativeMascotView.swift","NativeMascotPreferences.swift"] {
        let text=try String(contentsOf:sources.appendingPathComponent(name),encoding:.utf8)
        for forbidden in ["import RealityKit","import SceneKit","import WebKit",".send(",".newConversation(",".respondToApproval("] { #expect(!text.contains(forbidden)) }
    }
}
