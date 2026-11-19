import AppKit
@main struct PointerProbe {
    @MainActor static func main(){do{try run()}catch{print("probe refused: \(error)");exit(1)}}
    @MainActor static func run() throws{
        let a=CommandLine.arguments
        print("own=\(getpid()) front=\(NSWorkspace.shared.frontmostApplication?.processIdentifier ?? -1) post=\(CGPreflightPostEventAccess()) targetBundles=\(NSRunningApplication(processIdentifier:Int32(a[1])!)?.bundleIdentifier ?? "nil"),\(NSRunningApplication(processIdentifier:Int32(a[2])!)?.bundleIdentifier ?? "nil")")
        guard a.count>=5,let petPID=Int32(a[1]),let peerPID=Int32(a[2]),
              NSRunningApplication(processIdentifier:petPID)?.bundleIdentifier=="dev.gator.pet.b1.lab",
              NSRunningApplication(processIdentifier:peerPID)?.bundleIdentifier=="dev.gator.pet.focus-fixture",
              NSWorkspace.shared.frontmostApplication?.processIdentifier==peerPID,CGPreflightPostEventAccess() else{throw NSError(domain:"RefuseWrongFocusOrFixture",code:1)}
        let windows=CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]] ?? []
        func frame(_ pid:Int32,pet:Bool)->CGRect?{windows.first{($0[kCGWindowOwnerPID as String] as? NSNumber)?.int32Value==pid && (pet ? (($0[kCGWindowBounds as String] as? [String:Double])?["Width"]==72) : (($0[kCGWindowBounds as String] as? [String:Double])?["Width"] ?? 0)>=400)}.flatMap{($0[kCGWindowBounds as String] as? NSDictionary).flatMap{CGRect(dictionaryRepresentation:$0)}}}
        guard let panel=frame(petPID,pet:true),let peer=frame(peerPID,pet:false) else{throw NSError(domain:"NoOwnedVisibleWindows",code:1)}
        let p=CGPoint(x:panel.minX+(Double(a[3]) ?? 0.5)*panel.width,y:panel.minY+(Double(a[4]) ?? 0.5)*panel.height)
        print("panel=\(panel) peer=\(peer) point=\(p)")
        guard panel.insetBy(dx:-12,dy:-12).contains(p),peer.contains(p) else{throw NSError(domain:"RefuseOutsideFixture",code:1)}
        func post(_ kind:CGEventType,_ point:CGPoint){CGEvent(mouseEventSource:nil,mouseType:kind,mouseCursorPosition:point,mouseButton:.left)?.post(tap:.cghidEventTap)}
        post(.leftMouseDown,p)
        if a.count>5,a[5]=="drag"{for step in 1...12{let q=CGPoint(x:p.x+CGFloat(step)*6,y:p.y);guard peer.contains(q) else{throw NSError(domain:"DragLeavesFixture",code:1)};post(.leftMouseDragged,q)};post(.leftMouseUp,CGPoint(x:p.x+72,y:p.y))}
        else if a.count>5,a[5]=="scroll"{post(.leftMouseUp,p);CGEvent(scrollWheelEvent2Source:nil,units:.pixel,wheelCount:1,wheel1:32,wheel2:0,wheel3:0)?.post(tap:.cghidEventTap)}
        else{post(.leftMouseUp,p)}
        print("posted only within owned fixture; maximum pet hit rectangle=\(panel)")
    }
}
