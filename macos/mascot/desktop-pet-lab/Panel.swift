import AppKit

@MainActor final class PetPanel:NSPanel {
    override var canBecomeKey:Bool{false}
    override var canBecomeMain:Bool{false}
}
@MainActor final class PetHitView:NSView {
    var open:(()->Void)?
    var tuck:(()->Void)?
    var move:((CGPoint)->Void)?
    private var press:CGPoint?
    private var offset=CGPoint.zero
    private var dragged=false
    override var acceptsFirstResponder:Bool{false}
    override func acceptsFirstMouse(for event:NSEvent?)->Bool{true}
    override func hitTest(_ point:NSPoint)->NSView?{bounds.contains(point) ? self:nil}
    override func accessibilityPerformPress()->Bool{open?();return true}
    override func mouseDown(with event:NSEvent){
        guard let window else{return}
        press=window.convertPoint(toScreen:event.locationInWindow);offset=event.locationInWindow;dragged=false
    }
    override func mouseDragged(with event:NSEvent){
        guard let start=press,let window else{return}
        let point=window.convertPoint(toScreen:event.locationInWindow)
        if hypot(point.x-start.x,point.y-start.y)>4{dragged=true}
        if dragged{move?(CGPoint(x:point.x-offset.x,y:point.y-offset.y))}
    }
    override func mouseUp(with event:NSEvent){
        let activate=press != nil && !dragged;press=nil
        if activate{open?()}
    }
    override func menu(for event:NSEvent)->NSMenu?{
        let menu=NSMenu();let item=NSMenuItem(title:"Tuck Away",action:#selector(tuckPet),keyEquivalent:"");item.target=self;menu.addItem(item);return menu
    }
    @objc private func tuckPet(){tuck?()}
}
@MainActor final class PetPanelController {
    private(set) var panel:PetPanel?
    private(set) var canvas:NativeMascotCanvas?
    private(set) var hit:PetHitView?
    var screen:PetScreen?
    var placement=PetPlacement.reserved
    var interaction=PetInteraction.passive{didSet{if oldValue != interaction{applyInteraction()}}}
    var open:(()->Void)?
    var tuck:(()->Void)?
    var record:((String)->Void)?
    let assets:URL
    init(assets:URL){self.assets=assets}
    func show(_ request:NativeMascotRequest) {
        guard let screen,let frame=PetGeometry.position(on:screen,placement:placement) else{hide();return}
        if panel == nil{
            let window=PetPanel(contentRect:frame,styleMask:[.borderless,.nonactivatingPanel],backing:.buffered,defer:false)
            window.title="Gator Desktop Pet — isolated";window.identifier=NSUserInterfaceItemIdentifier("pet")
            window.backgroundColor = .clear;window.isOpaque=false;window.hasShadow=false;window.isRestorable=false;window.animationBehavior = .none
            window.hidesOnDeactivate=false;window.isReleasedWhenClosed=false;window.isMovable=false
            window.level = .floating;window.isFloatingPanel=true;window.becomesKeyOnlyIfNeeded=true;window.worksWhenModal=false
            window.collectionBehavior=[.canJoinAllSpaces,.transient,.ignoresCycle,.fullScreenNone]
            let root=NSView(frame:CGRect(origin:.zero,size:frame.size));let picture=NativeMascotCanvas(root:assets);picture.frame=root.bounds;root.addSubview(picture)
            let target=PetHitView(frame:root.bounds);target.open={ [weak self] in self?.record?("intentional-click-open-fixture");self?.open?() }
            target.tuck={ [weak self] in self?.tuck?() }
            target.move={ [weak self] origin in
                guard let self,let screen=self.screen,let bounds=PetGeometry.clampedDrag(origin,on:screen) else{return}
                self.panel?.setFrame(bounds,display:true);self.record?("drag-without-activation")
            }
            target.setAccessibilityLabel("Open Gator chat — isolated fixture");target.setAccessibilityRole(.button)
            root.addSubview(target);window.contentView=root;panel=window;canvas=picture;hit=target
        }
        applyInteraction();panel?.setFrame(frame,display:true)
        canvas?.request=request
        panel?.orderFrontRegardless();canvas?.refreshVisibility()
    }
    func update(_ request:NativeMascotRequest){canvas?.request=request}
    private func applyInteraction(){panel?.ignoresMouseEvents=interaction == .passive;hit?.isHidden=interaction == .passive;panel?.setAccessibilityElement(interaction == .interactive)}
    func hide(){canvas?.player.stop();panel?.orderOut(nil)}
    func destroy() async{
        canvas?.detach();panel?.orderOut(nil)
        await canvas?.player.settled()
        panel?.contentView=nil;canvas=nil;hit=nil;panel?.close();panel=nil
    }
}
