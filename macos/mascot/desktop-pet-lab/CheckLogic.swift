import Foundation
@main struct CheckPetLogic {
    static func main() throws {
        var count=0
        func check(_ value:Bool,_ description:String)throws{guard value else{throw NSError(domain:"PetLogic",code:count,userInfo:[NSLocalizedDescriptionKey:description])};count+=1}
        let plain=PetScreen(id:"primary",frame:CGRect(x:0,y:0,width:1920,height:1080),visible:CGRect(x:0,y:40,width:1920,height:1016))
        let notched=PetScreen(id:"built",frame:CGRect(x:0,y:0,width:1512,height:982),visible:CGRect(x:0,y:40,width:1512,height:904),safeTop:38,left:CGRect(x:0,y:944,width:680,height:38),right:CGRect(x:832,y:944,width:680,height:38),builtIn:true,scale:2)
        let negative=PetScreen(id:"left",frame:CGRect(x:-1920,y:-200,width:1920,height:1080),visible:CGRect(x:-1920,y:-160,width:1920,height:1016),scale:1)
        let above=PetScreen(id:"above",frame:CGRect(x:100,y:1080,width:1080,height:1920),visible:CGRect(x:100,y:1110,width:1080,height:1866),scale:2)
        for screen in [plain,notched,negative,above] {
            for placement in PetPlacement.allCases {
                let r=try unwrap(PetGeometry.position(on:screen,placement:placement))
                try check(screen.usable.contains(r),"within usable screen, including negative/rotated origins")
                try check(r.maxY<=screen.frame.maxY-screen.safeTop-12,"below menu/camera safe edge")
                if placement != .offset || screen.cameraGap == nil{try check(abs(r.midX-screen.usable.midX)<0.001,"top center")}
                if placement != .menu,let occupied=screen.quickAccess{try check(!r.intersects(occupied),"does not compete with expanded quick access")}
            }
            let clamped=try unwrap(PetGeometry.clampedDrag(CGPoint(x:-99999,y:99999),on:screen));try check(screen.usable.contains(clamped),"drag clamped")
        }
        try check(notched.cameraGap==152 && notched.quickAccess?.height==252,"matches current notch envelope")
        try check(PetGeometry.select([plain,notched],preferred:"built")==notched,"fixed user choice")
        try check(PetGeometry.select([plain,negative],preferred:"removed")==plain,"disconnected chosen display falls back")
        try check(PetGeometry.select([],preferred:"built")==nil,"no screen safe suppression")
        let tiny=PetScreen(id:"tiny",frame:CGRect(x:0,y:0,width:80,height:80),visible:CGRect(x:0,y:0,width:80,height:80))
        try check(PetGeometry.position(on:tiny,placement:.reserved)==nil,"cannot fit safely")
        var missingCamera=notched;missingCamera.left=nil;try check(missingCamera.cameraGap==nil,"do not invent camera geometry")
        var policy=PetPolicy();try check(!policy.enabled && policy.inChat,"pet defaults off independently")
        for desktop in [false,true]{for chat in [false,true]{policy.enabled=desktop;policy.inChat=chat;try check(policy.shouldShow==desktop && policy.inChat==chat,"four independent configurations")}}
        policy.enabled=true;policy.animated=false;try check(!policy.shouldAnimate && policy.shouldShow,"static")
        policy.animated=true;policy.reduceMotion=true;try check(!policy.shouldAnimate,"Reduce Motion wins")
        policy.reduceMotion=false;policy.temporarilySuppressed=true;try check(!policy.shouldShow,"sleep/temporary suppression")
        policy.temporarilySuppressed=false;policy.tuck();for _ in 0..<100{policy.reconcile()};try check(!policy.shouldShow,"activity/wake cannot re-enable")
        let decoded=try JSONDecoder().decode(PetPolicy.self,from:JSONEncoder().encode(policy));try check(!decoded.enabled,"proposed persistence keeps tuck, without creating settings")
        try check(NativeMascotStyle.allCases.map(\.rawValue)==["rendered","classic"],"reuse only accepted styles")
        var state=NativeMascotStateMachine();state.receive(.init(owner:"A",connected:true));state.receive(.init(owner:"A",connected:true,working:true));try check(state.expression == .working,"real public working")
        state.receive(.init(owner:"A",connected:true,working:true,publicTextRevision:4));try check(state.expression == .responding,"public text only")
        state.receive(.init(owner:"A",connected:true,working:true,attention:true));try check(state.expression == .attention,"review precedence")
        state.receive(.init(owner:"A",connected:false,attention:true));try check(state.expression == .disconnected,"disconnect precedence")
        state.receive(.init(owner:"B",connected:true,completedRun:"old"));try check(state.expression == .neutral,"selected-owner identity reset")
        print("{\"checks\":\(count),\"passed\":true,\"windows\":0,\"backendProcesses\":0}")
    }
    static func unwrap<T>(_ value:T?)throws->T{guard let value else{throw NSError(domain:"PetGeometryMissing",code:1)};return value}
}
