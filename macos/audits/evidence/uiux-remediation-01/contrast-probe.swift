import AppKit
import SwiftUI

func rgb(_ color: NSColor, over background: [Double]) -> [Double] {
    let value = color.usingColorSpace(.sRGB)!, alpha = Double(value.alphaComponent)
    return zip([Double(value.redComponent), Double(value.greenComponent), Double(value.blueComponent)], background).map { $0 * alpha + $1 * (1 - alpha) }
}
func luminance(_ values: [Double]) -> Double {
    let v = values.map { $0 <= 0.04045 ? $0 / 12.92 : pow(($0 + 0.055) / 1.055, 2.4) }
    return v[0] * 0.2126 + v[1] * 0.7152 + v[2] * 0.0722
}
var rows: [[String: Any]] = []
for name in [NSAppearance.Name.aqua, .darkAqua, .accessibilityHighContrastAqua, .accessibilityHighContrastDarkAqua] {
    NSAppearance(named: name)!.performAsCurrentDrawingAppearance {
        for (surface, color) in [("window", NSColor.windowBackgroundColor), ("text", .textBackgroundColor), ("control", .controlBackgroundColor)] {
            let background = rgb(color, over: [1, 1, 1]), back = luminance(background)
            for (label, color) in [("before: SwiftUI orange", NSColor(Color.orange)), ("after: semantic label text", .labelColor)] {
                let foreground = rgb(color, over: background), fore = luminance(foreground)
                rows.append(["appearance": name.rawValue, "surface": surface, "treatment": label, "foregroundSRGB": foreground, "backgroundSRGB": background, "contrast": (max(fore, back) + 0.05) / (min(fore, back) + 0.05)])
            }
        }
    }
}
print(String(decoding: try JSONSerialization.data(withJSONObject: rows, options: [.prettyPrinted, .sortedKeys]), as: UTF8.self))
