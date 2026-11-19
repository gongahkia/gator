import AppKit

@MainActor enum PetScreens {
    static func read()->[PetScreen] {
        NSScreen.screens.map{screen in
            let number=(screen.deviceDescription[NSDeviceDescriptionKey("NSScreenNumber")] as? NSNumber)?.uint32Value ?? 0
            let id=CGDisplayCreateUUIDFromDisplayID(number).map{CFUUIDCreateString(nil,$0.takeRetainedValue()) as String} ?? "unavailable-\(number)"
            return PetScreen(id:id,frame:screen.frame,visible:screen.visibleFrame,safeTop:screen.safeAreaInsets.top,
                             safeLeft:screen.safeAreaInsets.left,safeRight:screen.safeAreaInsets.right,safeBottom:screen.safeAreaInsets.bottom,
                             left:screen.auxiliaryTopLeftArea,right:screen.auxiliaryTopRightArea,builtIn:CGDisplayIsBuiltin(number) != 0,scale:screen.backingScaleFactor)
        }
    }
}
