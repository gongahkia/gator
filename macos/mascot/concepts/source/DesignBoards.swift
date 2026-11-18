// Offline layout studies, drawn from the current Gator structure. Not app UI.
import AppKit
import Foundation
@MainActor final class Canvas: NSView {
    var paint: ()->Void = {}
    override var isFlipped:Bool {true}
    override func draw(_ dirtyRect:NSRect){paint()}
}
@MainActor func ink(_ value:UInt32)->NSColor {NSColor(srgbRed:Double((value>>16)&255)/255,green:Double((value>>8)&255)/255,blue:Double(value&255)/255,alpha:1)}
@MainActor func rect(_ r:NSRect,_ c:NSColor,_ radius:CGFloat=0){c.setFill();NSBezierPath(roundedRect:r,xRadius:radius,yRadius:radius).fill()}
@MainActor func text(_ s:String,_ r:NSRect,_ size:CGFloat=15,_ c:NSColor = .labelColor,_ weight:NSFont.Weight = .regular){
    let p=NSMutableParagraphStyle();p.lineBreakMode = .byWordWrapping
    (s as NSString).draw(in:r,withAttributes:[.font:NSFont.systemFont(ofSize:size,weight:weight),.foregroundColor:c,.paragraphStyle:p])
}
@MainActor func centered(_ s:String,_ r:NSRect,_ size:CGFloat,_ c:NSColor,_ weight:NSFont.Weight = .regular){
    let p=NSMutableParagraphStyle();p.alignment = .center
    (s as NSString).draw(in:r,withAttributes:[.font:NSFont.systemFont(ofSize:size,weight:weight),.foregroundColor:c,.paragraphStyle:p])
}
@MainActor func png(_ url:URL,_ r:NSRect){if let im=NSImage(contentsOf:url){im.draw(in:r,from:.zero,operation:.sourceOver,fraction:1,respectFlipped:true,hints:[.interpolation:NSImageInterpolation.high])}}
@MainActor func board(_ size:NSSize,_ path:URL,_ paint:@escaping ()->Void)throws {
    let view=Canvas(frame:NSRect(origin:.zero,size:size));view.paint=paint
    guard let rep=NSBitmapImageRep(bitmapDataPlanes:nil,pixelsWide:Int(size.width),pixelsHigh:Int(size.height),bitsPerSample:8,samplesPerPixel:4,hasAlpha:true,isPlanar:false,colorSpaceName:.deviceRGB,bytesPerRow:Int(size.width)*4,bitsPerPixel:32) else{throw NSError(domain:"Board",code:1)}
    rep.size=size
    view.cacheDisplay(in:view.bounds,to:rep);guard let data=rep.representation(using:.png,properties:[:]) else{throw NSError(domain:"Board",code:2)};try data.write(to:path)
}
struct UIState {let title:String;let compact:Bool;let dark:Bool;let old:Bool;let work:Bool;let approval:Bool;let empty:Bool}
@MainActor func chat(_ root:URL,_ id:String,_ r:NSRect,_ state:UIState,_ option:Int){
    let dark=state.dark,fg=dark ? ink(0xF1F3F0):ink(0x252B27),secondary=dark ? ink(0xAAB3AD):ink(0x56655D)
    let bg=dark ? ink(0x242824):ink(0xFFFFFF),bar=dark ? ink(0x2E332F):ink(0xF2F5F2),line=dark ? ink(0x434B45):ink(0xDFE6E0),accent=dark ? ink(0x6BC79B):ink(0x296E4F)
    rect(r,bg,12);rect(NSRect(x:r.minX,y:r.minY,width:r.width,height:42),bar,12)
    if !state.compact {for (index,c) in [0xFF6057,0xFEBC2E,0x28C840].enumerated(){rect(NSRect(x:r.minX+14+CGFloat(index)*17,y:r.minY+15,width:10,height:10),ink(UInt32(c)),5)}} else {text("More   Open Window",NSRect(x:r.maxX-132,y:r.minY+13,width:122,height:20),11,secondary)}
    text(state.compact ? "Gator":"Gator — Workspace notes",NSRect(x:r.minX+(state.compact ? 14:78),y:r.minY+12,width:r.width-190,height:22),14,fg,.semibold)
    if !state.compact {text("Recent Conversations     Find     •••",NSRect(x:r.maxX-275,y:r.minY+13,width:260,height:20),11,secondary)}
    let workspace=NSRect(x:r.minX+14,y:r.minY+51,width:r.width-28,height:24)
    text("Workspace: Sample Project",NSRect(x:workspace.minX,y:workspace.minY,width:workspace.width*0.64,height:24),11,secondary)
    text("Read-only in Work",NSRect(x:workspace.maxX-114,y:workspace.minY,width:114,height:24),10,secondary)
    var top=r.minY+86
    // A reserved presentation slot: nothing overlays workspace/search/approval.
    let small=state.old || state.approval || state.compact || r.height < 580
    let edge:CGFloat=small ? (state.compact ? 44:28):(state.empty ? 96:72)
    let band:CGFloat=small ? (state.compact ? 50:36):(state.empty ? 110:84)
    let anchor:CGFloat=option==2 ? 0.42:0.5
    let cx=r.minX+r.width*anchor
    if option==3 && !small {rect(NSRect(x:r.minX+14,y:top+27,width:r.width-28,height:1),line)}
    let pose=state.approval ? "front-attention":"quarter-neutral"
    png(root.appendingPathComponent("\(id)/\(pose).png"),NSRect(x:cx-edge/2,y:top,width:edge,height:edge))
    if small {text(state.approval ? "An action needs your review":state.old ? "Reading earlier messages":"Gator",NSRect(x:cx+edge/2+8,y:top+8,width:min(220,r.maxX-cx-edge/2-14),height:20),10,secondary)}
    top += band
    if state.work {text("Background work continues in Trip planning",NSRect(x:r.minX+16,y:top,width:r.width-32,height:26),11,secondary);top+=27}
    NSGraphicsContext.saveGraphicsState()
    let bodyBottom=r.maxY-(state.compact ? 89:112)-18-35
    NSBezierPath(rect:NSRect(x:r.minX+1,y:top,width:r.width-2,height:max(0,bodyBottom-top))).addClip()
    if state.approval {
        text("Review requested action",NSRect(x:r.minX+18,y:top+8,width:r.width-36,height:25),17,fg,.semibold)
        text("Existing exact-review content stays here.\nThe mascot cannot approve or change this action.",NSRect(x:r.minX+18,y:top+47,width:r.width-36,height:65),13,fg)
        rect(NSRect(x:r.minX+18,y:top+130,width:150,height:29),bar,6);text("Existing review controls",NSRect(x:r.minX+25,y:top+137,width:140,height:20),10,fg)
        text("Layout reference only — no live approval",NSRect(x:r.minX+18,y:top+178,width:r.width-36,height:24),10,secondary)
    } else if state.empty {
        centered("What can I help with?",NSRect(x:r.minX+28,y:top+38,width:r.width-56,height:36),state.compact ? 20:26,fg,.medium)
        centered("Ask a question, make a plan, or pick up where you left off.",NSRect(x:r.minX+28,y:top+87,width:r.width-56,height:60),14,secondary)
    } else {
        rect(NSRect(x:r.maxX-min(260,r.width-48),y:top+10,width:min(242,r.width-48),height:51),dark ? ink(0x35483D):ink(0xEDF4ED),10)
        text(state.old ? "Earlier: how should I structure these notes?":"Help me organize the project notes.",NSRect(x:r.maxX-min(247,r.width-61),y:top+24,width:min(220,r.width-70),height:32),12,fg)
        text("Gator",NSRect(x:r.minX+18,y:top+88,width:r.width-36,height:20),12,secondary,.semibold)
        text("A clear starting point",NSRect(x:r.minX+18,y:top+116,width:r.width-36,height:28),18,fg,.semibold)
        text("Group the notes by outcome, then keep the next action beside each group.",NSRect(x:r.minX+18,y:top+154,width:r.width-36,height:54),13,fg)
        if r.height>580 {text("• Decisions and their context\n• Open questions\n• Next steps",NSRect(x:r.minX+18,y:top+216,width:r.width-36,height:84),13,fg)}
    }
    NSGraphicsContext.restoreGraphicsState()
    let composerHeight:CGFloat=state.compact ? 89:112
    let composerY=r.maxY-composerHeight-18
    rect(NSRect(x:r.minX+14,y:composerY-30,width:r.width-28,height:1),line)
    text("● Connected",NSRect(x:r.minX+16,y:composerY-23,width:110,height:20),10,accent)
    text("Model selection",NSRect(x:r.maxX-130,y:composerY-23,width:112,height:20),10,secondary)
    rect(NSRect(x:r.minX+14,y:composerY,width:r.width-28,height:composerHeight),dark ? ink(0x303731):ink(0xF6F8F5),9)
    text("An unsent draft stays here…",NSRect(x:r.minX+26,y:composerY+15,width:r.width-76,height:30),12,fg)
    text("Attach Files     Plan first",NSRect(x:r.minX+26,y:r.maxY-40,width:190,height:20),10,secondary)
    rect(NSRect(x:r.maxX-58,y:r.maxY-52,width:34,height:26),accent,6);text("Send",NSRect(x:r.maxX-53,y:r.maxY-46,width:30,height:17),10,dark ? ink(0x193F2A):.white,.medium)
    if state.old {rect(NSRect(x:r.midX-52,y:composerY-65,width:104,height:24),bar,8);text("Jump to Latest",NSRect(x:r.midX-43,y:composerY-60,width:92,height:20),10,fg)}
}
@main @MainActor struct Boards {
    static func main()throws {
        let root=URL(fileURLWithPath:CommandLine.arguments.count>1 ? CommandLine.arguments[1]:"macos/mascot/concepts")
        let names=["A · Round Gator","B · Soft 3D Gator","C · Expressive Mini"]
        let skins:[UInt32]=[0x4B946A,0x3E835F,0x5EAE92]
        try board(NSSize(width:1536,height:750),root.appendingPathComponent("comparison.png")){
            rect(NSRect(x:0,y:0,width:1536,height:750),ink(0xF4F6F2))
            text("Gator character studies",NSRect(x:54,y:36,width:1200,height:55),36,ink(0x243B2D),.semibold)
            text("Original procedural 3D · three directions for a small in-chat companion",NSRect(x:55,y:96,width:1300,height:30),17,ink(0x56655D))
            for i in 0..<3 {
                let x=CGFloat(i)*492+35,id=["A","B","C"][i]
                png(root.appendingPathComponent("\(id)/quarter-neutral.png"),NSRect(x:x,y:160,width:460,height:460))
                text(names[i],NSRect(x:x+25,y:625,width:440,height:35),25,ink(0x243B2D),.semibold)
                rect(NSRect(x:x+26,y:677,width:16,height:16),ink(skins[i]),8)
                text(["Soft jade · quiet and round","Moss green · broad, blunt snout","Mint green · curious asymmetric lids"][i],NSRect(x:x+52,y:677,width:407,height:25),14,ink(0x56655D))
            }
        }
        for (i,id) in ["A","B","C"].enumerated(){
            try board(NSSize(width:1260,height:1100),root.appendingPathComponent("\(id)/concept-sheet.png")){
                rect(NSRect(x:0,y:0,width:1260,height:1100),ink(0xF6F7F3));text(names[i],NSRect(x:44,y:28,width:1150,height:50),32,ink(0x243B2D),.semibold)
                for (j,file) in ["front-neutral","quarter-neutral","front-open","quarter-open"].enumerated(){
                    let x=CGFloat(j%2)*470+150,y=CGFloat(j/2)*390+84
                    png(root.appendingPathComponent("\(id)/\(file).png"),NSRect(x:x,y:y,width:360,height:360));text(["Front / neutral","Three-quarter / neutral","Front / jaw open","Three-quarter / jaw open"][j],NSRect(x:x+20,y:y+350,width:370,height:28),16,ink(0x41594A),.medium)
                }
                for (j,p) in ["blink","attention","concern"].enumerated(){let x=CGFloat(j)*245+280;png(root.appendingPathComponent("\(id)/front-\(p).png"),NSRect(x:x,y:900,width:120,height:120));text(p.capitalized,NSRect(x:x,y:1020,width:200,height:25),14,ink(0x41594A))}
                text("Actual geometry renders. Editable source, named parts, USDA and USDZ accompany this sheet.",NSRect(x:44,y:1065,width:1180,height:25),13,ink(0x56655D))
            }
            try board(NSSize(width:1440,height:950),root.appendingPathComponent("placement-\(id).png")){
                rect(NSRect(x:0,y:0,width:1440,height:950),ink(0xE8EDE8));text(names[i]+" / in-chat placement",NSRect(x:40,y:22,width:1320,height:42),28,ink(0x243B2D),.semibold)
                text("Layout studies drawn from Gator — illustrative content, not application screenshots or implemented UI",NSRect(x:40,y:67,width:1320,height:26),14,ink(0x56655D))
                chat(root,id,NSRect(x:40,y:112,width:820,height:740),UIState(title:"Full",compact:false,dark:false,old:false,work:false,approval:false,empty:i==0),i+1)
                chat(root,id,NSRect(x:928,y:112,width:440,height:620),UIState(title:"Compact",compact:true,dark:true,old:false,work:false,approval:false,empty:false),i+1)
                text(["Option 1: centered, in a reserved header slot.","Option 2: a slight offset, clear of native controls.","Option 3: peeks across its own divider; no control overlap."][i],NSRect(x:40,y:882,width:1290,height:28),17,ink(0x314A3A),.medium)
            }
            try board(NSSize(width:1536,height:1200),root.appendingPathComponent("states-\(id).png")){
                rect(NSRect(x:0,y:0,width:1536,height:1200),ink(0xE8EDE8));text(names[i]+" / conversation states",NSRect(x:32,y:22,width:1460,height:42),28,ink(0x243B2D),.semibold)
                let states=[UIState(title:"Long conversation / light",compact:false,dark:false,old:false,work:false,approval:false,empty:false),UIState(title:"Earlier messages / dark",compact:false,dark:true,old:true,work:false,approval:false,empty:false),UIState(title:"Background work / light",compact:false,dark:false,old:false,work:true,approval:false,empty:false),UIState(title:"Pending approval / dark",compact:false,dark:true,old:false,work:false,approval:true,empty:false)]
                for (j,state) in states.enumerated(){let x=CGFloat(j%2)*752+24,y=CGFloat(j/2)*543+87;text(state.title,NSRect(x:x,y:y-23,width:700,height:22),13,ink(0x41594A),.medium);chat(root,id,NSRect(x:x,y:y,width:736,height:500),state,i+1)}
            }
        }
        try board(NSSize(width:1260,height:700),root.appendingPathComponent("small-size-legibility.png")){
            rect(NSRect(x:0,y:0,width:1260,height:700),ink(0xF5F7F3));text("Small-size legibility",NSRect(x:44,y:26,width:1160,height:48),32,ink(0x243B2D),.semibold)
            text("Actual native render sizes at 1× — inspect this image at 100%. Not every detail survives 16–24 px.",NSRect(x:44,y:82,width:1160,height:28),15,ink(0x56655D))
            for (row,id) in ["A","B","C"].enumerated(){
                let y=CGFloat(row)*177+145;text(names[row],NSRect(x:44,y:y+37,width:260,height:50),19,ink(0x243B2D),.medium)
                for (col,pixels) in [16,24,32,48,64,96].enumerated(){let x=CGFloat(col)*140+340,s=CGFloat(pixels);png(root.appendingPathComponent("\(id)/size-\(pixels).png"),NSRect(x:x+(96-s)/2,y:y+(96-s)/2,width:s,height:s));text("\(pixels) px",NSRect(x:x+12,y:y+108,width:105,height:22),12,ink(0x56655D))}
            }
        }
        print("Rendered comparison, three concept sheets, three placement boards, three state boards and native-size checks")
    }
}
