import AppKit
import SwiftUI
func rgb(_ color: NSColor) -> [Double] { let c = color.usingColorSpace(.deviceRGB)!; return [Double(c.redComponent),Double(c.greenComponent),Double(c.blueComponent)] }
func luminance(_ color: NSColor) -> Double { let c=rgb(color).map { $0 <= 0.04045 ? $0/12.92 : pow(($0+0.055)/1.055,2.4) }; return c[0]*0.2126+c[1]*0.7152+c[2]*0.0722 }
var rows: [[String: Any]]=[]
for name in [NSAppearance.Name.aqua,.darkAqua,.accessibilityHighContrastAqua,.accessibilityHighContrastDarkAqua] {
 let appearance=NSAppearance(named:name)!
 appearance.performAsCurrentDrawingAppearance {
  for (label,background) in [("window",NSColor.windowBackgroundColor),("control",NSColor.controlBackgroundColor),("text",NSColor.textBackgroundColor)] {
   let foreground=NSColor(Color.orange);let a=luminance(foreground),b=luminance(background)
   rows.append(["appearance":name.rawValue,"foreground":"SwiftUI Color.orange","background":label,"foregroundRGB":rgb(foreground),"backgroundRGB":rgb(background),"contrast":(max(a,b)+0.05)/(min(a,b)+0.05)])
  }
 }
}
print(String(decoding:try JSONSerialization.data(withJSONObject:rows,options:[.prettyPrinted,.sortedKeys]),as:UTF8.self))
