// acceptance-only controller; refuses production and unrelated applications.
import AppKit
import ApplicationServices
import Foundation
@main struct FixtureControl {
@MainActor static func main() {
let args = CommandLine.arguments
if args.count == 2 && args[1] == "permissions" {
    print("AXTrusted=\(AXIsProcessTrusted()) screenCapture=\(CGPreflightScreenCaptureAccess()) postEvents=\(CGPreflightPostEventAccess())"); exit(0)
}
guard args.count>=3,let pid=Int32(args[1]) else{exit(2)}
let action=args[2]
guard let bundle = NSRunningApplication(processIdentifier: pid)?.bundleIdentifier, ["dev.gator.pet.b1.lab","dev.gator.pet.focus-fixture","dev.gator.pet.b1.gator-fixture"].contains(bundle) else { print("refuse_nonfixture"); exit(2) }
guard NSRunningApplication(processIdentifier:pid)?.bundleURL?.resolvingSymlinksInPath().path.hasPrefix(URL(fileURLWithPath:"/tmp/").resolvingSymlinksInPath().path) == true else { exit(2) }
let app = AXUIElementCreateApplication(pid)
func attr(_ e: AXUIElement, _ key: String) -> CFTypeRef? { var v: CFTypeRef?; AXUIElementCopyAttributeValue(e,key as CFString,&v); return v }
func str(_ e: AXUIElement, _ key: String) -> String {
    if ["AXValue", "AXSelectedText"].contains(key), attr(e,"AXSubrole") as? String == "AXSecureTextField" { return "[secure input omitted]" }
    return attr(e,key) as? String ?? ""
}
func elements(_ e: AXUIElement, _ key: String = "AXChildren") -> [AXUIElement] { attr(e,key) as? [AXUIElement] ?? [] }
func element(_ e: AXUIElement, _ key: String) -> AXUIElement? { guard let v=attr(e,key) else { return nil };return unsafeDowncast(v,to:AXUIElement.self) }
func matches(_ e: AXUIElement, _ name: String) -> Bool {
    if name.contains("::") { let p=name.components(separatedBy:"::");return str(e,"AXRole")==p[0] && matches(e,p[1]) }
    return ["AXIdentifier","AXDescription","AXTitle","AXValue"].contains { str(e,$0)==name }
}
func find(_ e: AXUIElement, _ name: String, _ depth: Int = 0) -> AXUIElement? {
    if matches(e,name) { return e };if depth>25 { return nil }
    for c in elements(e) { if let r=find(c,name,depth+1) { return r } };return nil
}
func window(_ id: String) -> AXUIElement? {
    let windows=elements(app,"AXWindows")
    if id=="compact" {return windows.first {str($0,"AXIdentifier").isEmpty && str($0,"AXTitle").isEmpty && abs(size($0).width-440)<2}}
    if let w=windows.first(where:{str($0,"AXIdentifier")==id || str($0,"AXTitle")==id}) {return w}
    for w in windows {if let sheet=find(w,id),str(sheet,"AXRole")=="AXSheet" {return sheet}}
    return nil
}
func scope(_ id: String) -> AXUIElement? {
    if id=="menu" { return element(app,"AXMenuBar") }
    if id=="app" { return app }
    return window(id)
}
func until(_ check: ()->Bool, seconds: Double = 4) -> Bool {
    let limit=Date().addingTimeInterval(seconds)
    repeat { if check() { return true }; RunLoop.current.run(until:Date().addingTimeInterval(0.02)) } while Date()<limit
    return check()
}
func state() {
    let fw=element(app,"AXFocusedWindow"),fe=element(app,"AXFocusedUIElement")
    print("controller=\(getpid()) frontName=\(NSWorkspace.shared.frontmostApplication?.localizedName ?? "nil") frontBundle=\(NSWorkspace.shared.frontmostApplication?.bundleIdentifier ?? "nil") front=\(NSWorkspace.shared.frontmostApplication?.processIdentifier ?? -1) target=\(pid) window=\(fw.map{str($0,"AXIdentifier")} ?? "nil") focused=\(fe.map{str($0,"AXRole")+":"+str($0,"AXDescription")+":"+str($0,"AXIdentifier")} ?? "nil")")
}
func requireFocus(_ win: String, _ name: String? = nil) {
    guard NSWorkspace.shared.frontmostApplication?.processIdentifier==pid,
          let fw=element(app,"AXFocusedWindow"), (win=="compact" ? str(fw,"AXIdentifier").isEmpty && abs(size(fw).width-440)<2 : matches(fw,win)) else { state(); print("refuse_input_wrong_window");exit(3) }
    if let name { guard let focused=element(app,"AXFocusedUIElement"),matches(focused,name) else { state(); print("refuse_input_wrong_element");exit(3) } }
}
func key(_ code: UInt16, flags: CGEventFlags = []) {
    for down in [true,false] { let e=CGEvent(keyboardEventSource:nil,virtualKey:code,keyDown:down)!;e.flags=flags;e.post(tap:.cghidEventTap) }
}
func dump(_ e: AXUIElement,_ depth: Int = 0) {
    if depth>23 { return }
    print(String(repeating:" ",count:depth)+str(e,"AXRole")+" | "+str(e,"AXTitle")+" | "+str(e,"AXDescription")+" | "+String(str(e,"AXValue").prefix(220))+" | "+str(e,"AXIdentifier"))
    for c in elements(e) { dump(c,depth+1) }
}
func position(_ e: AXUIElement) -> CGPoint {var p=CGPoint.zero;if let v=attr(e,"AXPosition"){AXValueGetValue(unsafeDowncast(v,to:AXValue.self),.cgPoint,&p)};return p}
func size(_ e: AXUIElement) -> CGSize {var p=CGSize.zero;if let v=attr(e,"AXSize"){AXValueGetValue(unsafeDowncast(v,to:AXValue.self),.cgSize,&p)};return p}
switch action {
case "quit":
    guard let running=NSRunningApplication(processIdentifier:pid) else{exit(2)}
    let requested=running.terminate();let done=until({running.isTerminated},seconds:8)
    print("quit_requested=\(requested) terminated=\(done)");if !done{exit(3)}
case "state": state()
case "focus":
    _=NSRunningApplication(processIdentifier:pid)?.activate(options:[.activateAllWindows])
    _=until{window(args[3]) != nil}
    guard let w=window(args[3]) else {print("missing_window");exit(2)}
    let activated=NSRunningApplication(processIdentifier:pid)?.activate(options:[.activateAllWindows])
    let raised=AXUIElementPerformAction(w,"AXRaise" as CFString)
    let main=AXUIElementSetAttributeValue(w,"AXMain" as CFString,kCFBooleanTrue)
    let focused=AXUIElementSetAttributeValue(w,"AXFocused" as CFString,kCFBooleanTrue)
    let ready=until { NSWorkspace.shared.frontmostApplication?.processIdentifier==pid && element(app,"AXFocusedWindow").map{matches($0,args[3])}==true }
    print("activated=\(activated as Any) raised=\(raised.rawValue) main=\(main.rawValue) focus=\(focused.rawValue) ready=\(ready)");state();if !ready{exit(3)}
case "wait-window":
    let ready=until { window(args[3]) != nil };print("ready=\(ready)");if !ready{exit(2)}
case "wait-gone":
    let ready=until { window(args[3]) == nil };print("gone=\(ready)");if !ready{exit(2)}
case "wait":
    let ready=until { scope(args[3]).flatMap{find($0,args[4])} != nil };print("ready=\(ready)");if !ready{exit(2)}
case "tree": guard let s=scope(args[3]) else{exit(2)};dump(s)
case "press":
    guard let s=scope(args[3]),let e=find(s,args[4]) else{print("missing");exit(2)}
    let result=AXUIElementPerformAction(e,"AXPress" as CFString);print("press=\(result.rawValue)");if result != .success{exit(2)}
case "focus-element":
    guard let s=scope(args[3]),let e=find(s,args[4]) else{print("missing");exit(2)}
    let result=AXUIElementSetAttributeValue(e,"AXFocused" as CFString,kCFBooleanTrue)
    let ready=until{element(app,"AXFocusedUIElement").map{matches($0,args[4])}==true};print("set=\(result.rawValue) focused=\(ready)");state();if !ready{exit(3)}
case "click":
    requireFocus(args[3]);guard let w=window(args[3]) else{exit(2)};let origin=position(w),extent=size(w)
    let p=CGPoint(x:origin.x+Double(args[4])!,y:origin.y+Double(args[5])!)
    guard CGRect(origin:origin,size:extent).contains(p) else{exit(2)}
    for type in [CGEventType.leftMouseDown,.leftMouseUp] {CGEvent(mouseEventSource:nil,mouseType:type,mouseCursorPosition:p,mouseButton:.left)?.post(tap:.cghidEventTap)}
    print("clicked_owned_window_at=\(p)")
case "select-all":
    requireFocus(args[3],args[4]);guard let e=element(app,"AXFocusedUIElement") else{exit(2)}
    let value=str(e,"AXValue");key(0,flags:.maskCommand)
    let ready=until{str(e,"AXSelectedText")==value};print("all_selected=\(ready) characters=\(value.count)");if !ready{exit(3)}
case "key-focus":
    requireFocus(args[3]);key(UInt16(args[4])!,flags:CGEventFlags(rawValue:UInt64(args[5])!))
    let ready=until {element(app,"AXFocusedUIElement").map{matches($0,args[6])}==true}
    print("key=\(args[4]) focused_expected=\(ready)");state();if !ready{exit(3)}
case "key": requireFocus(args[3],args.count>6 ? args[6]:nil);key(UInt16(args[4])!,flags:CGEventFlags(rawValue:UInt64(args[5])!));print("key=\(args[4]) target=\(pid)")
case "type":
    requireFocus(args[3],args[4]);let chars=Array(args[5].utf16)
    for down in [true,false]{let e=CGEvent(keyboardEventSource:nil,virtualKey:0,keyDown:down)!;e.flags=[];e.keyboardSetUnicodeString(stringLength:chars.count,unicodeString:chars);e.post(tap:.cghidEventTap)}
    let ready=until {element(app,"AXFocusedUIElement").map{str($0,"AXValue").contains(args[5])}==true};print("typed_to=\(args[4]) observed_value=\(ready)");if !ready{exit(3)}
case "inspect":
    guard let s=scope(args[3]),let e=find(s,args[4]) else{print("missing");exit(2)}
    print("role=\(str(e,"AXRole")) title=\(str(e,"AXTitle")) desc=\(str(e,"AXDescription")) id=\(str(e,"AXIdentifier")) focused=\(attr(e,"AXFocused") as Any) enabled=\(attr(e,"AXEnabled") as Any) pos=\(position(e)) size=\(size(e)) value=\(str(e,"AXValue"))")
case "move":
    guard let w=window(args[3]) else{exit(2)};var p=CGPoint(x:Double(args[4])!,y:Double(args[5])!);print("move=\(AXUIElementSetAttributeValue(w,"AXPosition" as CFString,AXValueCreate(.cgPoint,&p)!).rawValue)")
case "resize":
    guard let w=window(args[3]) else{exit(2)};var s=CGSize(width:Double(args[4])!,height:Double(args[5])!);print("resize=\(AXUIElementSetAttributeValue(w,"AXSize" as CFString,AXValueCreate(.cgSize,&s)!).rawValue)")
case "frames":
    for w in elements(app,"AXWindows"){print("window=\(str(w,"AXIdentifier")) title=\(str(w,"AXTitle")) pos=\(position(w)) size=\(size(w)) main=\(attr(w,"AXMain") as Any) focused=\(attr(w,"AXFocused") as Any)")}
    for w in CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]] ?? [] where w[kCGWindowOwnerPID as String] as? Int32==pid {print("captureID=\(w[kCGWindowNumber as String]!) bounds=\(w[kCGWindowBounds as String]!)")}
case "close":
    guard let w=window(args[3]),let e=element(w,"AXCloseButton") else{exit(2)};print("close=\(AXUIElementPerformAction(e,"AXPress" as CFString).rawValue)")
default: exit(2)
}

}
}
