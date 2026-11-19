import AppKit
import Foundation

@main struct CheckImages {
    static func main() throws {
        guard let first = NSBitmapImageRep(data: try Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[1]))),
              let second = NSBitmapImageRep(data: try Data(contentsOf: URL(fileURLWithPath: CommandLine.arguments[2]))),
              first.pixelsWide == second.pixelsWide, first.pixelsHigh == second.pixelsHigh else { throw NSError(domain: "ImageDimensionsDiffer", code: 1) }
        var changed = 0, maximum = 0.0, sum = 0.0, transparent = 0, visible = 0, partial = 0
        for y in 0..<first.pixelsHigh { for x in 0..<first.pixelsWide {
            guard let a = first.colorAt(x: x, y: y)?.usingColorSpace(.sRGB),
                  let b = second.colorAt(x: x, y: y)?.usingColorSpace(.sRGB) else { throw NSError(domain: "ImageColorUnavailable", code: 1) }
            if b.alphaComponent == 0 { transparent += 1 } else { visible += 1 }
            if b.alphaComponent > 0 && b.alphaComponent < 1 { partial += 1 }
            let deltas = zip([a.redComponent,a.greenComponent,a.blueComponent,a.alphaComponent], [b.redComponent,b.greenComponent,b.blueComponent,b.alphaComponent]).map { abs(Double($0 - $1)) * 255 }
            let delta = deltas.max() ?? 0
            if delta > 0.01 { changed += 1 }
            maximum = max(maximum, delta); sum += deltas.reduce(0,+)
        } }
        let result: [String: Any] = ["width": first.pixelsWide, "height": first.pixelsHigh, "changedPixels": changed,
                                   "maximumChannelDelta8Bit": maximum, "meanChannelDelta8Bit": sum / Double(first.pixelsWide * first.pixelsHigh * 4),
                                   "transparentPixels": transparent, "visiblePixels": visible, "partialAlphaPixels": partial]
        FileHandle.standardOutput.write(try JSONSerialization.data(withJSONObject: result, options: [.sortedKeys, .prettyPrinted]))
        guard maximum <= 1.01, transparent > 0, visible > 0 else { throw NSError(domain: "VisualFidelityOrAlphaFailed", code: 1) }
    }
}
