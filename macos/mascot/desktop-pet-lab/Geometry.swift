import Foundation
import CoreGraphics

struct PetScreen: Equatable, Sendable {
    let id: String
    let frame: CGRect
    let visible: CGRect
    var safeTop: CGFloat = 0
    var safeLeft: CGFloat = 0
    var safeRight: CGFloat = 0
    var safeBottom: CGFloat = 0
    var left: CGRect?
    var right: CGRect?
    var builtIn = false
    var scale: CGFloat = 1
    var cameraGap: CGFloat? {
        guard safeTop > 0, let left, let right, right.minX > left.maxX,
              left.minX >= frame.minX, right.maxX <= frame.maxX,
              right.minX-left.maxX < frame.width/2 else { return nil }
        return right.minX-left.maxX
    }
    var usable: CGRect {
        let safe=CGRect(x:frame.minX+safeLeft,y:frame.minY+safeBottom,
                        width:max(0,frame.width-safeLeft-safeRight),height:max(0,frame.height-safeTop-safeBottom))
        return visible.intersection(safe)
    }
    // mirrors the existing expanded quick-access envelope for the experiment;
    // production NotchQuickAccess is neither compiled nor modified here.
    var quickAccess: CGRect? {
        guard let gap=cameraGap else{return nil}
        let width=min(frame.width-24,max(420,gap+96)),height=safeTop+214
        return CGRect(x:frame.midX-width/2,y:frame.maxY-height,width:width,height:height)
    }
}
enum PetPlacement: String, CaseIterable, Sendable {
    case menu = "A · Below menu bar", reserved = "B · Clear quick access", offset = "C · Offset on notch"
}
enum PetGeometry {
    static func select(_ screens:[PetScreen],preferred:String?) -> PetScreen? {
        screens.first{$0.id==preferred} ?? screens.first
    }
    static func position(on screen:PetScreen,placement:PetPlacement,size:CGFloat=72) -> CGRect? {
        let area=screen.usable
        guard !area.isNull,area.width>=size+24,area.height>=size+24 else{return nil}
        var x=area.midX-size/2,top=area.maxY-12
        if let occupied=screen.quickAccess {
            switch placement {
            case .menu: break
            case .reserved: top=min(top,occupied.minY-12)
            case .offset: x=occupied.maxX+12
            }
        }
        var result=CGRect(x:x,y:top-size,width:size,height:size)
        result.origin.x=min(max(result.minX,area.minX+12),area.maxX-size-12)
        result.origin.y=min(max(result.minY,area.minY+12),area.maxY-size-12)
        if placement != .menu,let occupied=screen.quickAccess,result.intersects(occupied) {
            result.origin.y=occupied.minY-12-size
            guard area.contains(result) else{return nil}
        }
        return result
    }
    static func clampedDrag(_ origin:CGPoint,on screen:PetScreen,size:CGFloat=72)->CGRect? {
        guard var result=position(on:screen,placement:.reserved,size:size) else{return nil}
        let area=screen.usable
        result.origin=CGPoint(x:min(max(origin.x,area.minX+12),area.maxX-size-12),y:min(max(origin.y,area.minY+12),area.maxY-size-12))
        if let occupied=screen.quickAccess,result.intersects(occupied){result.origin.y=occupied.minY-size-12}
        return area.contains(result) ? result:nil
    }
}

enum PetInteraction: String, CaseIterable, Sendable {case passive="Passive",interactive="Interactive"}
struct PetPolicy: Codable, Equatable, Sendable {
    var enabled=false
    var inChat=true
    var animated=true
    var reduceMotion=false
    var temporarilySuppressed=false
    var shouldShow: Bool {enabled && !temporarilySuppressed}
    var shouldAnimate: Bool {shouldShow && animated && !reduceMotion}
    mutating func tuck(){enabled=false}
    // no implicit enable on activity, wake, display change or relaunch reconciliation
    mutating func reconcile(){ }
}
