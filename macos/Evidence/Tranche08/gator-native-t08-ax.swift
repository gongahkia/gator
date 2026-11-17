import AppKit
import ApplicationServices
let arguments = CommandLine.arguments
guard arguments.count >= 3, let pid = Int32(arguments[1]) else { fatalError("pid command [value]") }
let root = AXUIElementCreateApplication(pid)
AXUIElementSetMessagingTimeout(root, 3)
func attribute(_ element: AXUIElement, _ name: String) -> CFTypeRef? {
    var result: CFTypeRef?; let code = AXUIElementCopyAttributeValue(element, name as CFString, &result)
    return code == .success ? result : nil
}
func text(_ element: AXUIElement, _ name: String) -> String { attribute(element, name) as? String ?? "" }
var elements: [AXUIElement] = []
func visit(_ element: AXUIElement, _ depth: Int) {
    guard depth < 15, elements.count < 1600, !elements.contains(where: { CFEqual($0, element) }) else { return }; elements.append(element)
    for child in attribute(element, kAXChildrenAttribute) as? [AXUIElement] ?? [] { visit(child, depth + 1) }
}
let command = arguments[2]
elements.append(root)
if ["scan", "press", "draft", "enter", "shiftenter", "enabled", "scroll-review", "select-account", "field", "scroll-transcript", "drag", "drop-from", "focus-composer"].contains(command) {
    for window in attribute(root, kAXWindowsAttribute) as? [AXUIElement] ?? [] { visit(window, 0) }
}
if ["scan", "actions"].contains(command), let extras = attribute(root, "AXExtrasMenuBar") { visit(unsafeBitCast(extras, to: AXUIElement.self), 0) }
switch command {
case "focus-composer":
    let windows=attribute(root,kAXWindowsAttribute) as? [AXUIElement] ?? []
    if let full=windows.first(where:{text($0,kAXTitleAttribute).hasPrefix("Gator")}){AXUIElementPerformAction(full,kAXRaiseAction as CFString)}
    elements.removeAll()
    if let full=windows.first(where:{text($0,kAXTitleAttribute).hasPrefix("Gator")}){visit(full,0)}
    guard let target=elements.first(where:{text($0,kAXRoleAttribute)=="AXTextArea"}),let p=attribute(target,kAXPositionAttribute) else{fatalError("No composer")}
    var point=CGPoint.zero;AXValueGetValue(unsafeBitCast(p,to:AXValue.self),.cgPoint,&point);point.x+=20;point.y+=20
    for t in [CGEventType.leftMouseDown,.leftMouseUp]{CGEvent(mouseEventSource:nil,mouseType:t,mouseCursorPosition:point,mouseButton:.left)?.post(tap:.cghidEventTap)}
    RunLoop.current.run(until:Date().addingTimeInterval(0.2))
case "paste-key":
    NSRunningApplication(processIdentifier:pid)?.activate(options:[.activateAllWindows]);RunLoop.current.run(until:Date().addingTimeInterval(0.15))
    for down in [true,false] {let e=CGEvent(keyboardEventSource:nil,virtualKey:9,keyDown:down);e?.flags = .maskCommand;var u:[UniChar]=[118];e?.keyboardSetUnicodeString(stringLength:1,unicodeString:&u);e?.postToPid(pid)}
case "global-key":
    NSRunningApplication(processIdentifier:pid)?.activate(options:[.activateAllWindows]); RunLoop.current.run(until:Date().addingTimeInterval(0.1))
    for down in [true,false] {
        let e=CGEvent(keyboardEventSource:nil,virtualKey:UInt16(arguments[3])!,keyDown:down)
        let m=arguments[4]; var flags:CGEventFlags=[]
        if m.contains("command"){flags.insert(.maskCommand)};if m.contains("shift"){flags.insert(.maskShift)}
        e?.flags=flags;e?.post(tap:.cghidEventTap)
    }
    RunLoop.current.run(until:Date().addingTimeInterval(0.2))
case "scroll-transcript":
    guard let bar=elements.first(where:{text($0,kAXRoleAttribute)=="AXScrollBar"}) else{fatalError("No transcript bar")}
    print(AXUIElementSetAttributeValue(bar,kAXValueAttribute as CFString,NSNumber(value:1)).rawValue)
case "drag":
    elements.removeAll()
    if let full=(attribute(root,kAXWindowsAttribute) as? [AXUIElement])?.first(where:{text($0,kAXTitleAttribute).hasPrefix("Gator")}) { visit(full,0) }
    guard let target=elements.first(where:{text($0,kAXIdentifierAttribute)=="gator.composer.text" || text($0,kAXRoleAttribute)=="AXTextArea"}),let p=attribute(target,kAXPositionAttribute),let z=attribute(target,kAXSizeAttribute) else{fatalError("No composer")}
    var point=CGPoint.zero,size=CGSize.zero;AXValueGetValue(unsafeBitCast(p,to:AXValue.self),.cgPoint,&point);AXValueGetValue(unsafeBitCast(z,to:AXValue.self),.cgSize,&size)
    let start=CGPoint(x:Double(arguments[3])!,y:Double(arguments[4])!),end=CGPoint(x:point.x+size.width/2,y:point.y+min(20,size.height/2))
    CGEvent(mouseEventSource:nil,mouseType:.leftMouseDown,mouseCursorPosition:start,mouseButton:.left)?.post(tap:.cghidEventTap)
    for i in 1...30 {let f=Double(i)/30;let q=CGPoint(x:start.x+(end.x-start.x)*f,y:start.y+(end.y-start.y)*f);CGEvent(mouseEventSource:nil,mouseType:.leftMouseDragged,mouseCursorPosition:q,mouseButton:.left)?.post(tap:.cghidEventTap);RunLoop.current.run(until:Date().addingTimeInterval(0.04))}
    CGEvent(mouseEventSource:nil,mouseType:.leftMouseUp,mouseCursorPosition:end,mouseButton:.left)?.post(tap:.cghidEventTap)
case "drop-from":
    elements.removeAll()
    if let full=(attribute(root,kAXWindowsAttribute) as? [AXUIElement])?.first(where:{text($0,kAXTitleAttribute).hasPrefix("Gator")}) { visit(full,0) }
    guard let target=elements.first(where:{text($0,kAXIdentifierAttribute)=="gator.composer.text" || text($0,kAXRoleAttribute)=="AXTextArea"}),let p=attribute(target,kAXPositionAttribute),let z=attribute(target,kAXSizeAttribute) else{fatalError("No composer")}
    var point=CGPoint.zero,size=CGSize.zero;AXValueGetValue(unsafeBitCast(p,to:AXValue.self),.cgPoint,&point);AXValueGetValue(unsafeBitCast(z,to:AXValue.self),.cgSize,&size)
    let sourcePid=Int32(arguments[3])!
    let windows=CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]] ?? []
    guard let source=windows.first(where:{$0[kCGWindowOwnerPID as String] as? Int32 == sourcePid}),let bounds=source[kCGWindowBounds as String] as? [String:CGFloat] else{fatalError("Source missing")}
    let start=CGPoint(x:bounds["X"]!+130,y:bounds["Y"]!+75),end=CGPoint(x:point.x+size.width/2,y:point.y+min(20,size.height/2))
    CGEvent(mouseEventSource:nil,mouseType:.leftMouseDown,mouseCursorPosition:start,mouseButton:.left)?.post(tap:.cghidEventTap)
    for i in 1...30 {let f=Double(i)/30;let q=CGPoint(x:start.x+(end.x-start.x)*f,y:start.y+(end.y-start.y)*f);CGEvent(mouseEventSource:nil,mouseType:.leftMouseDragged,mouseCursorPosition:q,mouseButton:.left)?.post(tap:.cghidEventTap);RunLoop.current.run(until:Date().addingTimeInterval(0.04))}
    CGEvent(mouseEventSource:nil,mouseType:.leftMouseUp,mouseCursorPosition:end,mouseButton:.left)?.post(tap:.cghidEventTap)
case "field":
    guard let field = elements.last(where: { text($0,kAXRoleAttribute)=="AXTextField" || text($0,kAXRoleAttribute)=="AXComboBox" }) else { fatalError("No field") }
    print(AXUIElementSetAttributeValue(field,kAXValueAttribute as CFString,arguments[3] as CFString).rawValue)
case "clipboard-image":
    let bytes=try Data(contentsOf: URL(fileURLWithPath:arguments[3])); NSPasteboard.general.clearContents(); print(NSPasteboard.general.setData(bytes,forType:.png))
case "clipboard-text":
    NSPasteboard.general.clearContents(); print(NSPasteboard.general.setString(arguments[3],forType:.string))
case "resize":
    guard let window=(attribute(root,kAXWindowsAttribute) as? [AXUIElement])?.first(where:{text($0,kAXTitleAttribute).hasPrefix("Gator")}) else{fatalError("Window missing")}
    var size=CGSize(width:Double(arguments[3])!,height:Double(arguments[4])!);let value=AXValueCreate(.cgSize,&size)!
    print(AXUIElementSetAttributeValue(window,kAXSizeAttribute as CFString,value).rawValue)
case "capture-compact":
    let windows=CGWindowListCopyWindowInfo(.optionOnScreenOnly,kCGNullWindowID) as? [[String:Any]] ?? []
    guard let w=windows.first(where:{v in guard v[kCGWindowOwnerPID as String] as? Int32 == pid,let b=v[kCGWindowBounds as String] as? [String:CGFloat] else{return false};return b["Width"]==440 && (b["Height"] ?? 0)>300}),let id=w[kCGWindowNumber as String] as? Int else{fatalError("Compact window missing")}
    let p=Process();p.executableURL=URL(fileURLWithPath:"/usr/sbin/screencapture");p.arguments=["-x","-l",String(id),arguments[3]];try p.run();p.waitUntilExit()
case "quit":
    print(NSRunningApplication(processIdentifier: pid)?.terminate() as Any)
case "actions":
    print("Trusted", AXIsProcessTrusted(), "Post", CGPreflightPostEventAccess())
    for element in elements where text(element, kAXRoleAttribute) == "AXMenuBarItem" {
        var actions: CFArray?; print(text(element, kAXTitleAttribute), AXUIElementCopyActionNames(element, &actions).rawValue, actions as Any)
        print("press", AXUIElementPerformAction(element, kAXPressAction as CFString).rawValue)
    }
case "status":
    let windows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
    guard let status = windows.first(where: { $0[kCGWindowOwnerPID as String] as? Int32 == pid && $0[kCGWindowLayer as String] as? Int == 25 }),
          let rect = status[kCGWindowBounds as String] as? [String: CGFloat], let x = rect["X"], let y = rect["Y"], let width = rect["Width"], let height = rect["Height"] else { fatalError("Status item not found") }
    let point = CGPoint(x: x + width / 2, y: y + height / 2)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: point, mouseButton: .left)?.post(tap: .cghidEventTap)
    CGEvent(mouseEventSource: nil, mouseType: .leftMouseUp, mouseCursorPosition: point, mouseButton: .left)?.post(tap: .cghidEventTap)
case "scan":
    for element in elements {
        var values: CFArray?
        let keys = [kAXRoleAttribute, kAXTitleAttribute, kAXDescriptionAttribute, kAXValueAttribute] as CFArray
        _ = AXUIElementCopyMultipleAttributeValues(element, keys, AXCopyMultipleAttributeOptions(rawValue: 0), &values)
        let strings = (values as? [Any] ?? []).map { $0 as? String ?? "" }
        guard strings.count == 4 else { continue }
        let role = strings[0], title = strings[1], description = strings[2], value = strings[3]
        if !role.isEmpty { print([role, title, description, value].joined(separator: " | ").prefix(16000)) }
    }
case "enabled":
    guard let target = elements.first(where: { text($0, kAXTitleAttribute) == arguments[3] || text($0, kAXDescriptionAttribute) == arguments[3] }) else { fatalError("Control missing") }
    print(attribute(target, kAXEnabledAttribute) as? Bool ?? false)
case "scroll-review":
    guard let bar = elements.last(where: { text($0, kAXRoleAttribute) == "AXScrollBar" }) else { fatalError("Review scrollbar missing") }
    print(AXUIElementSetAttributeValue(bar, kAXValueAttribute as CFString, NSNumber(value: 1.0)).rawValue)
    RunLoop.current.run(until: Date().addingTimeInterval(0.2))
case "focused":
    if let value = attribute(root, kAXFocusedUIElementAttribute) {
        let element = unsafeBitCast(value, to: AXUIElement.self)
        print(text(element, kAXRoleAttribute), text(element, kAXTitleAttribute), text(element, kAXDescriptionAttribute), text(element, kAXValueAttribute))
    }
case "key":
    NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
    RunLoop.current.run(until: Date().addingTimeInterval(0.15))
    elements.removeAll()
    let windows = attribute(root, kAXWindowsAttribute) as? [AXUIElement] ?? []
    if let full = windows.first(where: { text($0, kAXTitleAttribute).hasPrefix("Gator") }) { visit(full, 0) }
    else { for window in windows { visit(window, 0) } }
    let label = arguments[3]
    if label != "__current", let target = elements.first(where: { text($0, kAXTitleAttribute) == label || text($0, kAXDescriptionAttribute) == label }) {
        print("Focus result", AXUIElementSetAttributeValue(target, kAXFocusedAttribute as CFString, kCFBooleanTrue).rawValue)
    }
    let code = UInt16(arguments[4])!
    let modifiers = arguments[5]
    for down in [true, false] {
        let event = CGEvent(keyboardEventSource: nil, virtualKey: code, keyDown: down)
        var flags: CGEventFlags = []
        if modifiers.contains("command") { flags.insert(.maskCommand) }
        if modifiers.contains("option") { flags.insert(.maskAlternate) }
        if modifiers.contains("control") { flags.insert(.maskControl) }
        if modifiers.contains("shift") { flags.insert(.maskShift) }
        event?.flags = flags; event?.postToPid(pid)
    }
    RunLoop.current.run(until: Date().addingTimeInterval(0.2))
case "escape":
    for down in [true, false] { CGEvent(keyboardEventSource: nil, virtualKey: 53, keyDown: down)?.postToPid(pid) }
case "chat", "inspect":
    let identifier = (command == "chat" ? "gator.chat." : "gator.inspect.") + arguments[3]
    func rebuild() {
        elements.removeAll()
        let windows = attribute(root, kAXWindowsAttribute) as? [AXUIElement] ?? []
        if let full = windows.first(where: { text($0, kAXTitleAttribute).hasPrefix("Gator") }) { visit(full, 0) }
        else { for window in windows { visit(window, 0) } }
    }
    rebuild()
    if !elements.contains(where: { text($0, kAXIdentifierAttribute) == identifier }) {
        guard let disclosure = elements.first(where: { text($0, kAXRoleAttribute) == "AXDisclosureTriangle" && ["Work continues in another chat", "Another chat needs attention", "Open chats and retained tasks"].contains(text($0, kAXDescriptionAttribute)) }) else { fatalError("Chat disclosure missing") }
        AXUIElementPerformAction(disclosure, kAXPressAction as CFString)
        RunLoop.current.run(until: Date().addingTimeInterval(0.3)); rebuild()
    }
    guard let target = elements.first(where: { text($0, kAXIdentifierAttribute) == identifier }) else { fatalError("Chat row missing: " + identifier) }
    print(AXUIElementPerformAction(target, kAXPressAction as CFString).rawValue)
case "select-account":
    let label = arguments[3]
    guard let target = elements.first(where: { text($0, kAXRoleAttribute) == "AXPopUpButton" && text($0, kAXDescriptionAttribute).contains("Configured account for requirement") }) else { fatalError("Account picker missing") }
    print("picker", text(target,kAXDescriptionAttribute), AXUIElementPerformAction(target,kAXPressAction as CFString).rawValue)
    RunLoop.current.run(until: Date().addingTimeInterval(0.4))
    elements.removeAll()
    for window in attribute(root,kAXWindowsAttribute) as? [AXUIElement] ?? [] { visit(window,0) }
    visit(target,0)
    guard let item=elements.first(where: { text($0,kAXRoleAttribute)=="AXMenuItem" && (text($0,kAXTitleAttribute)==label || text($0,kAXDescriptionAttribute)==label) }) else {
        for element in elements where text(element,kAXRoleAttribute).contains("Menu") { print(text(element,kAXRoleAttribute),text(element,kAXTitleAttribute),text(element,kAXDescriptionAttribute)) }
        fatalError("Account choice missing")
    }
    print(AXUIElementPerformAction(item,kAXPressAction as CFString).rawValue)
case "choose":
    NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
    RunLoop.current.run(until: Date().addingTimeInterval(0.3))
    elements.removeAll()
    for window in attribute(root, kAXWindowsAttribute) as? [AXUIElement] ?? [] { visit(window, 0) }
    let shownWindows = attribute(root, kAXWindowsAttribute) as? [AXUIElement] ?? []
    if let full = shownWindows.first(where: { text($0, kAXTitleAttribute).hasPrefix("Gator") }) { elements.removeAll(); visit(full, 0) }
    let menu = arguments[3], label = arguments[4]
    if !elements.contains(where: { text($0, kAXTitleAttribute) == label }) {
        guard let target = elements.first(where: { text($0, kAXTitleAttribute) == menu || text($0, kAXDescriptionAttribute) == menu }) else { fatalError("Menu missing: " + menu) }
        var point = CGPoint.zero, size = CGSize.zero
        let p = attribute(target, kAXPositionAttribute)!, z = attribute(target, kAXSizeAttribute)!
        AXValueGetValue(unsafeBitCast(p, to: AXValue.self), .cgPoint, &point)
        AXValueGetValue(unsafeBitCast(z, to: AXValue.self), .cgSize, &size)
        point.x += size.width / 2; point.y += size.height / 2
        for kind in [CGEventType.leftMouseDown, CGEventType.leftMouseUp] {
            CGEvent(mouseEventSource: nil, mouseType: kind, mouseCursorPosition: point, mouseButton: .left)?.post(tap: .cghidEventTap)
        }
        RunLoop.current.run(until: Date().addingTimeInterval(0.4))
    }
    elements.removeAll()
    for window in attribute(root, kAXWindowsAttribute) as? [AXUIElement] ?? [] { visit(window, 0) }
    guard let target = elements.first(where: { text($0, kAXTitleAttribute) == label }) else { fatalError("Menu item missing: " + label) }
    print(AXUIElementPerformAction(target, kAXPressAction as CFString).rawValue)
case "press":
    let label = arguments[3]
    guard let target = elements.first(where: { text($0, kAXTitleAttribute) == label || text($0, kAXDescriptionAttribute) == label }) else { fatalError("Control not found: " + label) }
    print(AXUIElementPerformAction(target, kAXPressAction as CFString).rawValue)
case "draft":
    guard let target = elements.first(where: { text($0, kAXRoleAttribute) == "AXTextArea" }) else { fatalError("Composer not found") }
    AXUIElementSetAttributeValue(target, kAXFocusedAttribute as CFString, kCFBooleanTrue)
    print(AXUIElementSetAttributeValue(target, kAXValueAttribute as CFString, arguments[3] as CFString).rawValue)
case "enter", "shiftenter":
    NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
    RunLoop.current.run(until: Date().addingTimeInterval(0.3))
    if let target = elements.first(where: { text($0, kAXRoleAttribute) == "AXTextArea" }) {
        AXUIElementSetAttributeValue(target, kAXFocusedAttribute as CFString, kCFBooleanTrue)
    }
    for down in [true, false] {
        let event = CGEvent(keyboardEventSource: nil, virtualKey: 36, keyDown: down)
        if command == "shiftenter" { event?.flags = .maskShift }
        event?.postToPid(pid)
    }
    RunLoop.current.run(until: Date().addingTimeInterval(0.3))
case "close":
    guard let window = (attribute(root, kAXWindowsAttribute) as? [AXUIElement])?.first(where: { text($0, kAXTitleAttribute).hasPrefix("Gator") }),
          let button = attribute(window, kAXCloseButtonAttribute) else { fatalError("Close button missing") }
    print(AXUIElementPerformAction(unsafeBitCast(button, to: AXUIElement.self), kAXPressAction as CFString).rawValue)
case "visible":
    let windows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
    print(windows.filter { $0[kCGWindowOwnerPID as String] as? Int32 == pid && [0, 101].contains($0[kCGWindowLayer as String] as? Int ?? -1) }.count)
case "capture":
    NSRunningApplication(processIdentifier: pid)?.activate(options: [.activateAllWindows])
    if let full = (attribute(root, kAXWindowsAttribute) as? [AXUIElement])?.first(where: { text($0, kAXTitleAttribute).hasPrefix("Gator") }) { AXUIElementPerformAction(full, kAXRaiseAction as CFString) }
    RunLoop.current.run(until: Date().addingTimeInterval(0.4))
    let windows = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? []
    guard let window = windows.first(where: { $0[kCGWindowOwnerPID as String] as? Int32 == pid && ($0[kCGWindowName as String] as? String ?? "").hasPrefix("Gator") }), let id = window[kCGWindowNumber as String] as? Int else { fatalError("Full window not visible") }
    let p = Process(); p.executableURL = URL(fileURLWithPath: "/usr/sbin/screencapture"); p.arguments = ["-x", "-l", String(id), arguments[3]]; try p.run(); p.waitUntilExit()
case "windows":
    for window in CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as? [[String: Any]] ?? [] {
        if window[kCGWindowOwnerPID as String] as? Int32 == pid { print(window) }
    }
default: fatalError("Unknown command")
}
