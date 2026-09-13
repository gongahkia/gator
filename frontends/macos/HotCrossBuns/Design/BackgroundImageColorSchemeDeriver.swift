import AppKit

enum BackgroundImageColorSchemeDeriver {
    static func derive(from imageURL: URL, suggestedTitle: String? = nil) -> HCBCustomColorScheme? {
        guard let image = NSImage(contentsOf: imageURL),
              let pixels = sampledPixels(from: image),
              pixels.isEmpty == false else {
            return nil
        }

        let average = averageColor(pixels)
        let isDark = relativeLuminance(average) < 0.46
        let accent = dominantAccent(from: pixels) ?? average
        let moss = color(nearHue: 0.34, in: pixels) ?? adjustedHue(from: accent, hue: 0.36)
        let blue = color(nearHue: 0.58, in: pixels) ?? adjustedHue(from: accent, hue: 0.58)
        let cream = isDark
            ? mix(average, RGB(red: 0.06, green: 0.07, blue: 0.09), amount: 0.68)
            : mix(average, RGB(red: 0.98, green: 0.96, blue: 0.91), amount: 0.72)
        let ink = isDark ? RGB(red: 0.95, green: 0.93, blue: 0.88) : RGB(red: 0.12, green: 0.12, blue: 0.14)
        let stroke = isDark ? lighten(cream, amount: 0.18) : darken(cream, amount: 0.16)
        let title = suggestedTitle?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty == false
            ? suggestedTitle!
            : "Image theme"

        return HCBCustomColorScheme(
            title: title,
            isDark: isDark,
            emberHex: accent.hexString,
            mossHex: moss.hexString,
            blueHex: blue.hexString,
            inkHex: ink.hexString,
            creamHex: cream.hexString,
            cardStrokeHex: stroke.hexString
        )
    }

    private struct RGB {
        var red: Double
        var green: Double
        var blue: Double

        var hexString: String {
            let r = Int((clamp(red) * 255).rounded())
            let g = Int((clamp(green) * 255).rounded())
            let b = Int((clamp(blue) * 255).rounded())
            return String(format: "#%02X%02X%02X", r, g, b)
        }
    }

    private static func sampledPixels(from image: NSImage) -> [RGB]? {
        let size = 56
        guard let bitmap = NSBitmapImageRep(
            bitmapDataPlanes: nil,
            pixelsWide: size,
            pixelsHigh: size,
            bitsPerSample: 8,
            samplesPerPixel: 4,
            hasAlpha: true,
            isPlanar: false,
            colorSpaceName: .deviceRGB,
            bytesPerRow: 0,
            bitsPerPixel: 0
        ) else {
            return nil
        }

        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: bitmap)
        NSColor.clear.setFill()
        NSRect(x: 0, y: 0, width: size, height: size).fill()
        image.draw(in: NSRect(x: 0, y: 0, width: size, height: size), from: .zero, operation: .copy, fraction: 1)
        NSGraphicsContext.restoreGraphicsState()

        var pixels: [RGB] = []
        pixels.reserveCapacity(size * size)
        for y in 0..<size {
            for x in 0..<size {
                guard let color = bitmap.colorAt(x: x, y: y)?.usingColorSpace(.sRGB),
                      color.alphaComponent > 0.4 else { continue }
                pixels.append(RGB(red: color.redComponent, green: color.greenComponent, blue: color.blueComponent))
            }
        }
        return pixels
    }

    private static func averageColor(_ pixels: [RGB]) -> RGB {
        let total = pixels.reduce(into: RGB(red: 0, green: 0, blue: 0)) { partial, color in
            partial.red += color.red
            partial.green += color.green
            partial.blue += color.blue
        }
        let count = Double(max(pixels.count, 1))
        return RGB(red: total.red / count, green: total.green / count, blue: total.blue / count)
    }

    private static func dominantAccent(from pixels: [RGB]) -> RGB? {
        pixels
            .filter { saturation($0) >= 0.18 && brightness($0) >= 0.18 && brightness($0) <= 0.92 }
            .max { lhs, rhs in
                saturation(lhs) * (0.6 + brightness(lhs)) < saturation(rhs) * (0.6 + brightness(rhs))
            }
            .map { boost($0) }
    }

    private static func color(nearHue targetHue: Double, in pixels: [RGB]) -> RGB? {
        pixels
            .filter { saturation($0) >= 0.14 && brightness($0) >= 0.16 }
            .min { lhs, rhs in
                hueDistance(hue(lhs), targetHue) < hueDistance(hue(rhs), targetHue)
            }
            .map { boost($0) }
    }

    private static func adjustedHue(from color: RGB, hue targetHue: Double) -> RGB {
        let hsb = hsb(color)
        return rgb(hue: targetHue, saturation: max(0.34, hsb.saturation), brightness: max(0.42, hsb.brightness))
    }

    private static func boost(_ color: RGB) -> RGB {
        let hsb = hsb(color)
        return rgb(
            hue: hsb.hue,
            saturation: min(0.88, max(0.36, hsb.saturation * 1.08)),
            brightness: min(0.86, max(0.36, hsb.brightness * 1.02))
        )
    }

    private static func mix(_ lhs: RGB, _ rhs: RGB, amount: Double) -> RGB {
        let t = clamp(amount)
        return RGB(
            red: lhs.red * (1 - t) + rhs.red * t,
            green: lhs.green * (1 - t) + rhs.green * t,
            blue: lhs.blue * (1 - t) + rhs.blue * t
        )
    }

    private static func lighten(_ color: RGB, amount: Double) -> RGB {
        mix(color, RGB(red: 1, green: 1, blue: 1), amount: amount)
    }

    private static func darken(_ color: RGB, amount: Double) -> RGB {
        mix(color, RGB(red: 0, green: 0, blue: 0), amount: amount)
    }

    private static func relativeLuminance(_ color: RGB) -> Double {
        0.2126 * color.red + 0.7152 * color.green + 0.0722 * color.blue
    }

    private static func brightness(_ color: RGB) -> Double {
        max(color.red, color.green, color.blue)
    }

    private static func saturation(_ color: RGB) -> Double {
        let maxValue = max(color.red, color.green, color.blue)
        let minValue = min(color.red, color.green, color.blue)
        guard maxValue > 0 else { return 0 }
        return (maxValue - minValue) / maxValue
    }

    private static func hue(_ color: RGB) -> Double {
        hsb(color).hue
    }

    private static func hsb(_ color: RGB) -> (hue: Double, saturation: Double, brightness: Double) {
        let nsColor = NSColor(red: clamp(color.red), green: clamp(color.green), blue: clamp(color.blue), alpha: 1)
        var hue: CGFloat = 0
        var saturation: CGFloat = 0
        var brightness: CGFloat = 0
        nsColor.getHue(&hue, saturation: &saturation, brightness: &brightness, alpha: nil)
        return (Double(hue), Double(saturation), Double(brightness))
    }

    private static func rgb(hue: Double, saturation: Double, brightness: Double) -> RGB {
        let color = NSColor(
            hue: CGFloat(clamp(hue)),
            saturation: CGFloat(clamp(saturation)),
            brightness: CGFloat(clamp(brightness)),
            alpha: 1
        ).usingColorSpace(.sRGB) ?? .controlAccentColor
        return RGB(red: color.redComponent, green: color.greenComponent, blue: color.blueComponent)
    }

    private static func hueDistance(_ lhs: Double, _ rhs: Double) -> Double {
        let raw = abs(lhs - rhs)
        return min(raw, 1 - raw)
    }

    private static func clamp(_ value: Double) -> Double {
        min(1, max(0, value))
    }
}
