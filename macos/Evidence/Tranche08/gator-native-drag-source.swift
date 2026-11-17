import AppKit
final class SourceView:NSView,NSDraggingSource {
    let url:URL
    init(url:URL){self.url=url;super.init(frame:NSRect(x:0,y:0,width:260,height:120))}
    required init?(coder:NSCoder){fatalError()}
    override func acceptsFirstMouse(for event:NSEvent?)->Bool{true}
    override func draw(_ dirtyRect:NSRect){NSColor.windowBackgroundColor.setFill();bounds.fill();("Drag controlled attachment\n"+url.lastPathComponent).draw(at:NSPoint(x:20,y:40),withAttributes:[.font:NSFont.systemFont(ofSize:15),.foregroundColor:NSColor.labelColor])}
    override func mouseDown(with event:NSEvent){print("DRAG START",event.locationInWindow);fflush(stdout);let item=NSDraggingItem(pasteboardWriter:url as NSURL);item.setDraggingFrame(NSRect(x:20,y:30,width:200,height:60),contents:NSImage(systemSymbolName:"doc",accessibilityDescription:nil)!);beginDraggingSession(with:[item],event:event,source:self)}
    func draggingSession(_ session:NSDraggingSession,sourceOperationMaskFor context:NSDraggingContext)->NSDragOperation{.copy}
    func draggingSession(_ session:NSDraggingSession,endedAt screenPoint:NSPoint,operation:NSDragOperation){print("DRAG END",operation.rawValue);fflush(stdout)}
    func ignoreModifierKeys(for session:NSDraggingSession)->Bool{true}
}
let app=NSApplication.shared;app.setActivationPolicy(.accessory)
app.finishLaunching()
let window=NSWindow(contentRect:NSRect(x:20,y:90,width:260,height:120),styleMask:[.titled,.closable],backing:.buffered,defer:false)
window.level = .floating;window.title="Controlled Gator Drag Source";window.contentView=SourceView(url:URL(fileURLWithPath:CommandLine.arguments[1]));window.orderFrontRegardless();window.makeKeyAndOrderFront(nil);app.activate(ignoringOtherApps:true);app.run()
