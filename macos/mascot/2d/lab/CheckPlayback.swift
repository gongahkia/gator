import AppKit

@main @MainActor struct CheckPlayback {
    static func main() async throws {
        _ = NSApplication.shared
        let root = URL(fileURLWithPath: CommandLine.arguments[1])
        var checks = 0
        func check(_ value: Bool, _ label: String) throws {
            guard value else { throw NSError(domain: "PlaybackCheck", code: checks, userInfo: [NSLocalizedDescriptionKey: label]) }; checks += 1
        }
        let store = NativeMascotFrameStore(root: root)
        var ticket = 1
        for style in NativeMascotStyle.allCases {
            for name in ["neutral","blink","working","responding","completed","attention","disconnected"] {
                ticket += 1
                let clip = try await store.load(style: style, name: name, animate: true, ticket: ticket)
                try check(clip.images.count == NativeMascotClip.spec(style:style,name:name)?.count, "exact state frame count")
                try check(clip.bytes <= NativeMascotFrameStore.maximumBytes, "bounded current clip")
                try check(clip.images.allSatisfy { $0.width == 224 && $0.height == 224 }, "bounded raster dimensions")
            }
        }
        await store.clear(ticket: 100); try check(await store.bytes == 0, "explicit cache release")
        var rejected = false
        do { _ = try await store.load(style:.rendered,name:"working",animate:true,ticket:99) } catch { rejected = true }
        try check(rejected,"stale decode cannot repopulate cleared cache")
        let player = NativeMascotPlayer(root:root)
        player.layer.frame = CGRect(x:0,y:0,width:112,height:112)
        player.configure(.init(owner:"A",style:.rendered,expression:.neutral)); await player.settled()
        player.configure(.init(owner:"A",style:.rendered,expression:.responding,revision:1))
        await player.settled()
        let descriptorFrames = try await store.load(style:.rendered,name:"responding",animate:true,ticket:101)
        let animation = NativeMascotPlayer.keyframes(descriptorFrames)
        try check(animation.values?.count == 9 && animation.duration == 0.55 && animation.repeatCount == 0 && animation.calculationMode == .discrete,"finite native animation descriptor")
        try check(player.animationStarts == 1 && descriptorFrames.bytes > 0,"decoded frames submitted; live context tested in native lab")
        player.configure(.init(owner:"B",style:.classic,expression:.attention))
        await player.settled()
        try check(player.displayedStyle == .classic,"new owner and style win")
        player.configure(.init(owner:"B",style:.classic,expression:.working,reduceMotion:true))
        await player.settled()
        try check(player.layer.animationKeys() == nil && player.layer.contents != nil,"Reduce Motion is static")
        player.configure(.init(owner:"B",style:.classic,expression:.working,shown:false))
        await player.settled()
        try check(player.layer.contents == nil && player.layer.animationKeys() == nil && player.activeFrameBytes == 0,"disabled releases presentation")
        try check(await player.store.bytes == 0,"disabled releases cache")
        let before = player.animationStarts
        player.configure(.init(owner:"C",style:.rendered,expression:.responding))
        player.configure(.init(owner:"D",style:.classic,expression:.disconnected,animate:false))
        await player.settled()
        try check(player.current?.owner == "D" && player.displayedStyle == .classic && player.layer.animationKeys() == nil,"cancel pending old-owner decode")
        try check(player.animationStarts == before,"stale decode did not start animation")
        let missing = NativeMascotPlayer(root:root.appendingPathComponent("missing"))
        missing.configure(.init(owner:"fixture",style:.rendered,expression:.working)); await missing.settled()
        try check(missing.layer.contents == nil && missing.layer.animationKeys() == nil,"missing all assets hides decoration")
        var model = NativeMascotStateMachine()
        model.receive(.init(owner:"A",connected:true,completedRun:"old"))
        try check(model.expression == .neutral,"initial retained history is not a completion transition")
        model.receive(.init(owner:"A",connected:true,working:true))
        try check(model.expression == .working,"authoritative working")
        model.receive(.init(owner:"A",connected:true,working:true,publicTextRevision:10))
        try check(model.expression == .responding,"public stream delta")
        model.receive(.init(owner:"A",connected:true,working:true,attention:true,publicTextRevision:20))
        try check(model.expression == .attention,"review priority over stream")
        model.receive(.init(owner:"A",connected:false,working:true,attention:true))
        try check(model.expression == .disconnected,"disconnect priority")
        model.receive(.init(owner:"A",connected:true,completedRun:"confirmed-new"))
        try check(model.expression == .completed,"confirmed new outcome")
        model.receive(.init(owner:"B",connected:true,completedRun:"confirmed-new"))
        try check(model.expression == .neutral,"owner switch resets outcome")
        try check(NativeMascotStateMachine.collapsed(reading:true,reviewing:false,compact:false,height:740),"older reading collapses")
        try check(NativeMascotStateMachine.collapsed(reading:false,reviewing:true,compact:true,height:620),"review collapses")
        try check(NativeMascotStateMachine.collapsed(reading:false,reviewing:false,compact:false,height:480),"minimum layout collapses")
        player.stop(); missing.stop(); await player.settled(); await missing.settled()
        print("{\"checks\":\(checks),\"passed\":true,\"windowsCreated\":0,\"backendProcesses\":0}")
    }
}
