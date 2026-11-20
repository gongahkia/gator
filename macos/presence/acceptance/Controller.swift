// fixture-only, targeted accessibility actions. No keyboard or pointer injection.
import AppKit
import ApplicationServices
import Foundation

@MainActor func attr(_ e: AXUIElement, _ key: String) -> CFTypeRef? { var v: CFTypeRef?; AXUIElementCopyAttributeValue(e, key as CFString, &v); return v }
@MainActor func string(_ e: AXUIElement, _ key: String) -> String { attr(e, key) as? String ?? "" }
@MainActor func children(_ e: AXUIElement, _ key: String = "AXChildren") -> [AXUIElement] { attr(e, key) as? [AXUIElement] ?? [] }
@MainActor func element(_ e: AXUIElement, _ key: String) -> AXUIElement? { guard let v = attr(e, key), CFGetTypeID(v) == AXUIElementGetTypeID() else { return nil }; return unsafeDowncast(v, to: AXUIElement.self) }
@MainActor func summary(_ e: AXUIElement?) -> [String: String] { guard let e else { return [:] }; return ["role": string(e,"AXRole"), "id": string(e,"AXIdentifier"), "title": string(e,"AXTitle"), "description": string(e,"AXDescription")] }
@MainActor func matches(_ e: AXUIElement, _ name: String) -> Bool { ["AXIdentifier", "AXDescription", "AXTitle", "AXValue"].contains { string(e,$0) == name } }
@MainActor func find(_ e: AXUIElement, _ name: String, _ depth: Int = 0) -> AXUIElement? { if matches(e,name) { return e }; if depth > 25 { return nil }; for c in children(e) { if let r = find(c,name,depth+1) { return r } }; return nil }
@MainActor func tree(_ e: AXUIElement, _ depth: Int = 0) -> [[String:String]] { if depth>23 { return [] }; var row=summary(e); row["value"] = String(string(e,"AXValue").prefix(240)); row["enabled"] = String(describing: attr(e,"AXEnabled")); row["depth"] = "\(depth)"; return [row] + children(e).flatMap { tree($0,depth+1) } }
@MainActor func wait(_ condition: () -> Bool, seconds: Double = 3) -> Bool { let end=Date().addingTimeInterval(seconds); repeat { if condition() { return true }; RunLoop.current.run(until: Date().addingTimeInterval(0.025)) } while Date()<end; return condition() }
// passive provenance only; records event kinds/PIDs, never keys or characters.
private final class InputAudit {
 let output: FileHandle
 init(_ path: String) { FileManager.default.createFile(atPath:path,contents:nil);output=FileHandle(forWritingAtPath:path)! }
 func record(_ value:[String:Any]) { let data=try! JSONSerialization.data(withJSONObject:value,options:[.sortedKeys]);output.write(data);output.write(Data([10])) }
}
private func startInputAudit() {
 Thread {
  guard let path=ProcessInfo.processInfo.environment["GATOR_PRESENCE_INPUT_AUDIT"] else { return }
  let audit=InputAudit(path)
  guard CGPreflightListenEventAccess() else { audit.record(["available":false,"reason":"listen permission unavailable; no prompt"]);return }
  let types:[CGEventType] = [.keyDown,.keyUp,.leftMouseDown,.leftMouseUp,.rightMouseDown,.rightMouseUp,.scrollWheel]
  let mask=types.reduce(CGEventMask(0)) { $0 | (1 << $1.rawValue) }
  guard let tap=CGEvent.tapCreate(tap:.cgSessionEventTap,place:.headInsertEventTap,options:.listenOnly,eventsOfInterest:mask,callback:{ _,type,event,context in
   let audit=Unmanaged<InputAudit>.fromOpaque(context!).takeUnretainedValue()
   audit.record(["epoch":Date().timeIntervalSince1970,"type":type.rawValue,"sourcePID":event.getIntegerValueField(.eventSourceUnixProcessID)])
   return Unmanaged.passUnretained(event)
  },userInfo:Unmanaged.passUnretained(audit).toOpaque()) else { audit.record(["available":false,"reason":"tap unavailable"]);return }
  audit.record(["available":true,"controllerPID":getpid()])
  CFRunLoopAddSource(CFRunLoopGetCurrent(),CFMachPortCreateRunLoopSource(kCFAllocatorDefault,tap,0),.defaultMode)
  withExtendedLifetime(audit) { CFRunLoopRun() }
 }.start()
}
@main struct Controller {
 @MainActor static func main() {
  _ = NSApplication.shared
  startInputAudit()
  while let line=readLine() {
   let start=Date();var result:[String:Any]=["controllerPID":getpid(),"started":start.timeIntervalSince1970]
   do {
    let request=try JSONSerialization.jsonObject(with:Data(line.utf8)) as! [String:Any]
    let pid=Int32(request["pid"] as! Int), action=request["action"] as! String
    guard let running=NSRunningApplication(processIdentifier:pid), running.bundleIdentifier=="dev.gator.presence.r1", running.bundleURL?.resolvingSymlinksInPath().path.hasPrefix(URL(fileURLWithPath:"/tmp/gator-native-test-presence-r1-").resolvingSymlinksInPath().path)==true else { throw Failure("refuse nonfixture: \(NSRunningApplication(processIdentifier:pid)?.bundleIdentifier ?? "nil") \(NSRunningApplication(processIdentifier:pid)?.bundleURL?.path ?? "nil")") }
    let app=AXUIElementCreateApplication(pid); AXUIElementSetMessagingTimeout(app,2)
    let windows=children(app,"AXWindows")
    func window(_ name:String)->AXUIElement? { windows.first { matches($0,name) } ?? windows.compactMap { find($0,name) }.first }
    let name=request["window"] as? String ?? "conversation", focus=element(app,"AXFocusedWindow")
    result["pid"]=pid;result["bundle"]=running.bundleIdentifier;result["frontPID"]=NSWorkspace.shared.frontmostApplication?.processIdentifier ?? -1;result["focusedWindow"]=summary(focus);result["focusedElement"]=summary(element(app,"AXFocusedUIElement"));result["action"]=action;result["window"]=name
    if action=="activate" { running.activate(options:[.activateAllWindows]);guard wait({NSWorkspace.shared.frontmostApplication?.processIdentifier==pid}) else { throw Failure("activation failed") } }
    else if action=="tree" { result["tree"]=tree(name=="menu" ? element(app,"AXMenuBar")! : window(name) ?? app) }
    else if action=="state" { result["windows"]=windows.map(summary);result["capture"]=((CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]]) ?? []).filter { $0[kCGWindowOwnerPID as String] as? Int32==pid } }
    else if action=="focus" { guard let w=window(name) else { throw Failure("missing focus window") };running.activate(options:[.activateAllWindows]);AXUIElementPerformAction(w,"AXRaise" as CFString);AXUIElementSetAttributeValue(w,"AXMain" as CFString,kCFBooleanTrue);AXUIElementSetAttributeValue(w,"AXFocused" as CFString,kCFBooleanTrue);guard wait({element(app,"AXFocusedWindow").map { matches($0,name) }==true}) else { throw Failure("focus failed") } }
    else {
     guard NSWorkspace.shared.frontmostApplication?.processIdentifier==pid, let focus, matches(focus,request["expectedWindow"] as? String ?? name) else { throw Failure("wrong foreground/window; abort input") }
     // require the expected focused role/identity observed by the caller immediately before input.
     guard let expected=request["expectedFocus"] as? [String:String], expected==summary(element(app,"AXFocusedUIElement")) else { throw Failure("focus changed; abort input") }
     if action=="press" {
      let target=request["target"] as! String
      let allowed:[String] = ["Open Original Task","Review Pending Action","Send message","An action needs your review","Back to Conversation","Agent Presence…","Settings…","Conversation","Window","Gator","gator.presence.mode","Emoji only","Emoji + GIF reactions","Off","gator.mascot.animated","gator.mascot.shown","gator.desktopPet.shown","gator.desktopPet.tuck","Quit and Keep Retained Work"]
      guard allowed.contains(target) else { throw Failure("target not in nondecision allowlist") }
      let scope=name=="menu" ? element(app,"AXMenuBar") : window(name)
      guard let scope, let targetElement=find(scope,target) else { throw Failure("missing target \(target)") }
      result["target"]=summary(targetElement);let code=AXUIElementPerformAction(targetElement,"AXPress" as CFString);guard code == .success else { throw Failure("AXPress \(code.rawValue)") }
     } else if action=="close" { guard let w=window(name),let close=element(w,"AXCloseButton") else { throw Failure("missing close") };guard AXUIElementPerformAction(close,"AXPress" as CFString) == .success else { throw Failure("close failed") } }
     else if action=="quit" { result["requested"]=running.terminate() }
     else { throw Failure("unsupported action") }
    }
    result["ok"]=true
   } catch { result["ok"]=false;result["error"]=String(describing:error) }
   result["elapsed"]=Date().timeIntervalSince(start)
   let data=try! JSONSerialization.data(withJSONObject:result,options:[.sortedKeys]);FileHandle.standardOutput.write(data);FileHandle.standardOutput.write(Data([10]))
  }
 }
 struct Failure: Error, CustomStringConvertible { let description:String;init(_ value:String){description=value} }
}
