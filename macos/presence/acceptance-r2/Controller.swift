// fixture-only, targeted accessibility actions. No keyboard or pointer injection.
import AppKit
import ApplicationServices
import Foundation
import CryptoKit

@MainActor func attr(_ e: AXUIElement, _ key: String) -> CFTypeRef? { var v: CFTypeRef?; AXUIElementCopyAttributeValue(e, key as CFString, &v); return v }
@MainActor func string(_ e: AXUIElement, _ key: String) -> String {
    if ["AXValue","AXSelectedText"].contains(key), attr(e,"AXSubrole") as? String == "AXSecureTextField" { return "[secure input omitted]" }
    return attr(e, key) as? String ?? ""
}
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
  var preflight: (id: String, pid: Int32, window: AXUIElement?, element: AXUIElement?, front: Int32)?
  var fixtureSubmitted: Set<Int32> = []
  while let line=readLine() {
   let start=Date();var result:[String:Any]=["controllerPID":getpid(),"started":start.timeIntervalSince1970]
   do {
    let request=try JSONSerialization.jsonObject(with:Data(line.utf8)) as! [String:Any]
    let pid=Int32(request["pid"] as! Int), action=request["action"] as! String
    let manifestURL=URL(fileURLWithPath:request["manifest"] as! String).resolvingSymlinksInPath()
    let manifest=try JSONSerialization.jsonObject(with:Data(contentsOf:manifestURL)) as! [String:Any]
    let expectedPath=URL(fileURLWithPath:manifest["app"] as! String).appendingPathComponent("Contents/MacOS/GatorMac").resolvingSymlinksInPath().path
    guard manifest["pid"] as? Int == Int(pid), let running=NSRunningApplication(processIdentifier:pid),
          running.bundleIdentifier==manifest["bundle"] as? String,
          running.bundleIdentifier?.hasPrefix("dev.gator.presence.r2.")==true,
          manifestURL.path.hasPrefix(URL(fileURLWithPath:"/tmp/gator-native-test-presence-r2-").resolvingSymlinksInPath().path),
          URL(fileURLWithPath:manifest["app"] as! String).deletingLastPathComponent().resolvingSymlinksInPath().path==manifestURL.deletingLastPathComponent().path,
          running.executableURL?.resolvingSymlinksInPath().path==expectedPath else { throw Failure("refuse nonfixture: PID/bundle/executable mismatch; actualBundle=\(NSRunningApplication(processIdentifier:pid)?.bundleIdentifier ?? "nil") actualExecutable=\(NSRunningApplication(processIdentifier:pid)?.executableURL?.path ?? "nil") expectedExecutable=\(expectedPath) manifest=\(manifestURL.path)") }
    let digest=manifest["binarySHA256"] as! String
    let actual=SHA256.hash(data:try Data(contentsOf:URL(fileURLWithPath:expectedPath))).map { String(format:"%02x",$0) }.joined()
    guard actual==digest else { throw Failure("refuse nonfixture: executable digest mismatch") }
    result["executable"]=expectedPath;result["binarySHA256"]=digest
    let app=AXUIElementCreateApplication(pid); AXUIElementSetMessagingTimeout(app,2)
    let windows=children(app,"AXWindows")
    func window(_ name:String)->AXUIElement? { windows.first { matches($0,name) } ?? windows.compactMap { find($0,name) }.first }
    let name=request["window"] as? String ?? "conversation", focus=element(app,"AXFocusedWindow")
    result["pid"]=pid;result["bundle"]=running.bundleIdentifier;result["frontPID"]=NSWorkspace.shared.frontmostApplication?.processIdentifier ?? -1;result["focusedWindow"]=summary(focus);result["focusedElement"]=summary(element(app,"AXFocusedUIElement"));result["action"]=action;result["window"]=name
    if action=="activate" { running.activate(options:[.activateAllWindows]);guard wait({NSWorkspace.shared.frontmostApplication?.processIdentifier==pid}) else { throw Failure("activation failed") } }
    else if action=="tree" { result["tree"]=tree(name=="menu" ? element(app,"AXMenuBar")! : window(name) ?? app) }
    else if action=="state" {
     let id=UUID().uuidString
     preflight=(id,pid,focus,element(app,"AXFocusedUIElement"),NSWorkspace.shared.frontmostApplication?.processIdentifier ?? -1)
     result["preflightID"]=id;result["windows"]=windows.map(summary);result["capture"]=((CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]]) ?? []).filter { $0[kCGWindowOwnerPID as String] as? Int32==pid } }
    else if action=="focus" {
     guard let verified=preflight, verified.pid==pid, verified.id==request["preflightID"] as? String,
           verified.front==pid,
           verified.front==NSWorkspace.shared.frontmostApplication?.processIdentifier,
           verified.window.map({ old in focus.map { CFEqual(old,$0) } ?? false })==true,
           verified.element.map({ old in element(app,"AXFocusedUIElement").map { CFEqual(old,$0) } ?? false })==true
     else { throw Failure("stale preflight/focus; abort focus input") }
     preflight=nil
     guard let w=window(name) else { throw Failure("missing focus window") };running.activate(options:[.activateAllWindows]);AXUIElementPerformAction(w,"AXRaise" as CFString);AXUIElementSetAttributeValue(w,"AXMain" as CFString,kCFBooleanTrue);AXUIElementSetAttributeValue(w,"AXFocused" as CFString,kCFBooleanTrue);guard wait({element(app,"AXFocusedWindow").map { matches($0,name) }==true}) else { throw Failure("focus failed") } }
    else {
     guard let verified=preflight, verified.pid==pid, verified.id==request["preflightID"] as? String,
           verified.front==pid,
           verified.front==NSWorkspace.shared.frontmostApplication?.processIdentifier,
           verified.window.map({ old in focus.map { CFEqual(old,$0) } ?? false })==true,
           verified.element.map({ old in element(app,"AXFocusedUIElement").map { CFEqual(old,$0) } ?? false })==true
     else { throw Failure("stale preflight/focus; abort input") }
     preflight=nil
     guard NSWorkspace.shared.frontmostApplication?.processIdentifier==pid, let focus, matches(focus,request["expectedWindow"] as? String ?? name) else { throw Failure("wrong foreground/window; abort input") }
     // require the expected focused role/identity observed by the caller immediately before input.
     guard let expected=request["expectedFocus"] as? [String:String], expected==summary(element(app,"AXFocusedUIElement")) else { throw Failure("focus changed; abort input") }
     if action=="press" {
      let target=request["target"] as! String
      let allowed:[String] = ["Open Original Task","Review Pending Action","An action needs your review","Back to Conversation","Agent Presence…","Settings…","Conversation","Window","Gator","gator.presence.mode","Emoji only","Emoji + GIF reactions","Off","gator.mascot.animated","gator.mascot.shown","gator.desktopPet.shown","gator.desktopPet.tuck","Quit and Keep Retained Work"]
      let setupSend=target=="Send message" && request["positiveCase"] as? String=="create-inert-review" && !fixtureSubmitted.contains(pid)
      guard allowed.contains(target) || setupSend else { throw Failure("target not in nondecision allowlist") }
      let scope=name=="menu" ? element(app,"AXMenuBar") : window(name)
      guard let scope else { throw Failure("missing scope") }
      let wantedRole=request["role"] as! String
      func collect(_ e:AXUIElement, _ depth:Int=0)->[AXUIElement] {
       if depth>25 { return [] }
       let match=string(e,"AXRole")==wantedRole && ["AXIdentifier","AXDescription","AXTitle"].contains { string(e,$0)==target }
       return (match ? [e] : []) + children(e).flatMap { collect($0,depth+1) }
      }
      let candidates=collect(scope)
      guard candidates.count==1,let targetElement=candidates.first else { throw Failure("target missing or ambiguous: \(target), \(wantedRole), count \(candidates.count)") }
      guard attr(targetElement,"AXEnabled") as? Bool != false else { throw Failure("disabled control") }
      if setupSend {
       guard let field=find(scope,"gator.composer.text"),string(field,"AXValue")=="Review mascot" else { throw Failure("fixture draft mismatch") }
       fixtureSubmitted.insert(pid)
      }
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
