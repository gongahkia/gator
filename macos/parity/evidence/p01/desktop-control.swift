import AppKit
import ApplicationServices
import Foundation
let pid = Int32(CommandLine.arguments[1])!, action = CommandLine.arguments[2]
// audit-only: refuse to operate a production or unrelated process.
let allowed = ["dev.gator.native.p01-disposable"]
guard let bundle = NSRunningApplication(processIdentifier: pid)?.bundleIdentifier, allowed.contains(bundle) else {
    print("Use only an isolated Gator UI/UX fixture process."); exit(1)
}
let app = AXUIElementCreateApplication(pid)
func attribute(_ e: AXUIElement, _ name: String) -> CFTypeRef? { var value: CFTypeRef?; AXUIElementCopyAttributeValue(e, name as CFString, &value); return value }
func str(_ e: AXUIElement, _ name: String) -> String { attribute(e,name) as? String ?? "" }
func children(_ e: AXUIElement) -> [AXUIElement] { attribute(e,"AXChildren") as? [AXUIElement] ?? [] }
func matches(_ e:AXUIElement,_ name:String)->Bool { if name.contains("::") { let parts=name.components(separatedBy:"::"); return str(e,"AXRole")==parts[0] && matches(e,parts[1]) }; return ["AXTitle","AXDescription","AXIdentifier","AXValue"].contains { str(e,$0) == name } }
func find(_ e:AXUIElement,_ name:String,_ depth:Int=0)->AXUIElement? { if matches(e,name) { return e }; if depth>30 { return nil }; for child in children(e) { if let result=find(child,name,depth+1) { return result } }; return nil }
func dump(_ e:AXUIElement,_ depth:Int=0) { if depth>25 {return};let role=str(e,"AXRole"); let desc=str(e,"AXDescription"),title=str(e,"AXTitle"),value=str(e,"AXValue"),identifier=str(e,"AXIdentifier"); print(String(repeating:" ",count:depth)+role+" | "+title+" | "+desc+" | "+String(value.prefix(160))+" | "+identifier);for child in children(e){dump(child,depth+1)} }
func point(_ e:AXUIElement)->CGPoint { var point=CGPoint.zero; if let x=attribute(e,"AXPosition") { AXValueGetValue(unsafeBitCast(x,to:AXValue.self),.cgPoint,&point) };return point }
func size(_ e:AXUIElement)->CGSize { var value=CGSize.zero; if let x=attribute(e,"AXSize") { AXValueGetValue(unsafeBitCast(x,to:AXValue.self),.cgSize,&value) };return value }
func focus() { NSRunningApplication(processIdentifier:pid)?.activate(options:[]);usleep(150_000) }
switch action {
case "guarded-key":
 guard NSWorkspace.shared.frontmostApplication?.processIdentifier==pid else { print("fixture_not_frontmost");exit(1) }
 let code=UInt16(CommandLine.arguments[3])!,flags=CGEventFlags(rawValue:UInt64(CommandLine.arguments[4])!)
 for down in [true,false] { let e=CGEvent(keyboardEventSource:nil,virtualKey:code,keyDown:down)!;e.flags=down ? flags : [];e.post(tap:.cghidEventTap) }
 print("guarded_key_to_fixture=\(code)")
case "native-key":
 let point=CGPoint(x:Double(CommandLine.arguments[3])!,y:Double(CommandLine.arguments[4])!)
 for type in [CGEventType.leftMouseDown,.leftMouseUp] { let e=CGEvent(mouseEventSource:nil,mouseType:type,mouseCursorPosition:point,mouseButton:.left)!;e.flags=[];e.post(tap:.cghidEventTap) }
 usleep(150_000)
 guard NSWorkspace.shared.frontmostApplication?.processIdentifier==pid else { print("fixture_not_frontmost");exit(1) }
 let code=UInt16(CommandLine.arguments[5])!, flags=CGEventFlags(rawValue:UInt64(CommandLine.arguments[6])!)
 for down in [true,false] { let e=CGEvent(keyboardEventSource:nil,virtualKey:code,keyDown:down)!;e.flags=down ? flags : [];e.post(tap:.cghidEventTap) }
 print("native_key_to_fixture=\(code)")
case "actions":
 guard let e=find(app,CommandLine.arguments[3]) else { print("missing");exit(1) }; var names:CFArray?;AXUIElementCopyActionNames(e,&names);print(names as Any)
case "open":
 guard let e=find(app,CommandLine.arguments[3]) else { print("missing");exit(1) }; print(AXUIElementPerformAction(e,"AXOpen" as CFString).rawValue)
case "confirm":
 guard let e=find(app,CommandLine.arguments[3]) else { print("missing");exit(1) };print(AXUIElementPerformAction(e,"AXConfirm" as CFString).rawValue)
case "front": print(NSWorkspace.shared.frontmostApplication?.processIdentifier as Any)
case "click":
 let point=CGPoint(x:Double(CommandLine.arguments[3])!,y:Double(CommandLine.arguments[4])!); for type in [CGEventType.leftMouseDown,.leftMouseUp] { let e=CGEvent(mouseEventSource:nil,mouseType:type,mouseCursorPosition:point,mouseButton:.left)!;e.flags=[];e.post(tap:.cghidEventTap) }
case "tree": dump(app)
case "press":
 let target=CommandLine.arguments[3];guard let e=find(app,target) else { fatalError("Missing element: "+target) };let result=AXUIElementPerformAction(e,"AXPress" as CFString);print("press \(target): \(result.rawValue)")
case "set":
 guard let e=find(app,CommandLine.arguments[3]) else { fatalError("Missing text") };print(AXUIElementSetAttributeValue(e,"AXValue" as CFString,CommandLine.arguments[4] as CFString).rawValue)
case "key":
 focus();let code=UInt16(CommandLine.arguments[3])!;let flags=CGEventFlags(rawValue:UInt64(CommandLine.arguments[4])!);for down in [true,false] {let e=CGEvent(keyboardEventSource:nil,virtualKey:code,keyDown:down)!;e.flags=down ? flags : [];e.postToPid(pid)}
case "type":
 focus();for down in [true,false] { let event=CGEvent(keyboardEventSource:nil,virtualKey:0,keyDown:down)!;event.flags=[];let text=Array(CommandLine.arguments[3].utf16);event.keyboardSetUnicodeString(stringLength:text.count,unicodeString:text);event.postToPid(pid) }
case "scroll":
 focus();let x=Double(CommandLine.arguments[3])!,y=Double(CommandLine.arguments[4])!;CGEvent(mouseEventSource:nil,mouseType:.mouseMoved,mouseCursorPosition:CGPoint(x:x,y:y),mouseButton:.left)?.post(tap:.cghidEventTap);usleep(100_000);CGEvent(scrollWheelEvent2Source:nil,units:.pixel,wheelCount:1,wheel1:Int32(CommandLine.arguments[5])!,wheel2:0,wheel3:0)?.post(tap:.cghidEventTap)
case "resize":
 guard let window=(attribute(app,"AXWindows") as? [AXUIElement])?.first(where: { str($0,"AXIdentifier")=="conversation" }) else { fatalError("No window") };var value=CGSize(width:Double(CommandLine.arguments[3])!,height:Double(CommandLine.arguments[4])!);print(AXUIElementSetAttributeValue(window,"AXSize" as CFString,AXValueCreate(.cgSize,&value)!).rawValue)
case "inspect":
 guard let e=find(app,CommandLine.arguments[3]) else { fatalError("Missing element") }; print("focused=\(attribute(e,"AXFocused") as Any) enabled=\(attribute(e,"AXEnabled") as Any) position=\(point(e)) size=\(size(e)) value=\(str(e,"AXValue"))")
case "scrollbars":
 func visit(_ e:AXUIElement,_ depth:Int=0) { if depth>25{return}; if str(e,"AXRole")=="AXScrollBar" { print("value=\(attribute(e,"AXValue") as Any) position=\(point(e)) size=\(size(e))") };for child in children(e){visit(child,depth+1)} };visit(app)
case "heads":
 func visit(_ e:AXUIElement,_ depth:Int=0) { if depth>25{return}; if str(e,"AXRole")=="AXHeading" || str(e,"AXDescription")=="Jump to Latest" { print("\(str(e,"AXDescription")) at \(point(e)) size \(size(e))") };for child in children(e){visit(child,depth+1)} };visit(app)
case "close-window":
 guard let window=(attribute(app,"AXWindows") as? [AXUIElement])?.first(where: { str($0,"AXIdentifier")=="conversation" }), let button=attribute(window,"AXCloseButton") else { fatalError("No conversation close button") };print(AXUIElementPerformAction(unsafeBitCast(button,to:AXUIElement.self),"AXPress" as CFString).rawValue)
case "frames":
 for w in attribute(app,"AXWindows") as? [AXUIElement] ?? [] {print("window \(str(w,"AXTitle")) \(point(w)) \(size(w))")};for w in CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]] ?? [] where w[kCGWindowOwnerPID as String] as? Int32 == pid {print(w[kCGWindowNumber as String]!,w[kCGWindowBounds as String]!)}
default: fatalError("Unknown action")
}
