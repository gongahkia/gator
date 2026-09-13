import AppKit
import ImageIO
import SwiftUI

// Semantic colors resolve through the active HCBColorScheme. AppColor.X
// accessors stay as static properties so the 185 existing call sites keep
// working; the actual palette comes from HCBColorSchemeStore.current,
// which Settings updates. The MacSidebarShell applies .id(schemeID) so
// views re-render when the scheme changes.
// @MainActor to match HCBColorSchemeStore.current; every caller is already
// a SwiftUI View body / computed, which is @MainActor-bound.
@MainActor
enum AppColor {
    static var ember: Color { HCBColorSchemeStore.current.ember.swiftColor }
    static var moss: Color { HCBColorSchemeStore.current.moss.swiftColor }
    static var blue: Color { HCBColorSchemeStore.current.blue.swiftColor }
    static var ink: Color { HCBColorSchemeStore.current.ink.swiftColor }
    static var cream: Color { HCBColorSchemeStore.current.cream.swiftColor }
    static var cardStroke: Color { HCBColorSchemeStore.current.cardStroke.swiftColor }
    static var cardSurface: Color { HCBColorSchemeStore.current.cardSurface.swiftColor }
}

private struct HCBAppBackgroundDepthKey: EnvironmentKey {
    static let defaultValue: Int = 0
}

extension EnvironmentValues {
    var hcbAppBackgroundDepth: Int {
        get { self[HCBAppBackgroundDepthKey.self] }
        set { self[HCBAppBackgroundDepthKey.self] = newValue }
    }
}

private final class HCBBackgroundImageCache {
    static let shared = HCBBackgroundImageCache()

    private let cache = NSCache<NSString, NSImage>()

    func image(at path: String) async -> NSImage? {
        let key = path as NSString
        if let cached = cache.object(forKey: key) {
            return cached
        }

        let maxPixelSize = max(1800, NSScreen.main.map { Int(max($0.frame.width, $0.frame.height) * $0.backingScaleFactor) } ?? 2200)
        return await Task.detached(priority: .utility) { [cache] in
            let url = URL(filePath: path)
            guard let source = CGImageSourceCreateWithURL(url as CFURL, nil) else { return nil }
            let options: [CFString: Any] = [
                kCGImageSourceCreateThumbnailFromImageAlways: true,
                kCGImageSourceCreateThumbnailWithTransform: true,
                kCGImageSourceThumbnailMaxPixelSize: maxPixelSize
            ]

            guard let cgImage = CGImageSourceCreateThumbnailAtIndex(source, 0, options as CFDictionary) else { return nil }
            let image = NSImage(cgImage: cgImage, size: NSSize(width: cgImage.width, height: cgImage.height))
            cache.setObject(image, forKey: key)
            return image
        }.value
    }
}

private struct HCBBackgroundImage: View {
    let path: String
    @State private var image: NSImage?

    var body: some View {
        Group {
            if let image {
                Image(nsImage: image)
                    .resizable()
                    .scaledToFill()
            } else {
                AppColor.cream
            }
        }
        .task(id: path) {
            image = await HCBBackgroundImageCache.shared.image(at: path)
        }
    }
}

private struct HCBWindowBackgroundConfigurator: NSViewRepresentable {
    let configuration: HCBAppBackgroundConfiguration

    func makeNSView(context: Context) -> NSView {
        NSView(frame: .zero)
    }

    func updateNSView(_ nsView: NSView, context: Context) {
        DispatchQueue.main.async {
            guard let window = nsView.window else { return }
            let wantsClearWindow = configuration.isTranslucent || configuration.customImagePath != nil
            window.isOpaque = wantsClearWindow == false
            window.backgroundColor = wantsClearWindow ? .clear : .windowBackgroundColor
        }
    }
}

struct AppBackground: ViewModifier {
    @Environment(\.hcbAppBackgroundConfiguration) private var configuration
    @Environment(\.hcbAppBackgroundDepth) private var depth

    func body(content: Content) -> some View {
        content
            .background {
                if depth == 0 {
                    ZStack {
                        rootBackground
                    }
                    .ignoresSafeArea()
                } else {
                    nestedBackground
                }
            }
            .background {
                if depth == 0 {
                    HCBWindowBackgroundConfigurator(configuration: configuration)
                        .frame(width: 0, height: 0)
                }
            }
            .environment(\.hcbAppBackgroundDepth, depth + 1)
    }

    @ViewBuilder
    private var rootBackground: some View {
        if let path = configuration.customImagePath {
            HCBBackgroundImage(path: path)
            AppColor.cream.opacity(configuration.opacity)
        } else if configuration.isTranslucent {
            AppColor.cream.opacity(configuration.opacity)
                .background(.ultraThinMaterial)
        } else {
            Color(nsColor: .windowBackgroundColor)
        }
    }

    @ViewBuilder
    private var nestedBackground: some View {
        if let path = configuration.customImagePath {
            HCBBackgroundImage(path: path)
            AppColor.cream.opacity(max(0.18, configuration.opacity * 0.62))
        } else if configuration.isTranslucent {
            AppColor.cream.opacity(max(0.18, configuration.opacity * 0.62))
        } else {
            Color(nsColor: .windowBackgroundColor)
        }
    }
}

struct CardSurface: ViewModifier {
    var cornerRadius: CGFloat = 18

    func body(content: Content) -> some View {
        content
            .padding(18)
            .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: cornerRadius))
            .overlay {
                RoundedRectangle(cornerRadius: cornerRadius)
                    .stroke(AppColor.cardStroke, lineWidth: 1)
            }
    }
}

extension View {
    func appBackground() -> some View {
        modifier(AppBackground())
    }

    func cardSurface(cornerRadius: CGFloat = 18) -> some View {
        modifier(CardSurface(cornerRadius: cornerRadius))
    }
}

extension Color {
    init(hex: String) {
        let normalized = hex.trimmingCharacters(in: CharacterSet(charactersIn: "#"))
        var value: UInt64 = 0
        Scanner(string: normalized).scanHexInt64(&value)
        let red = Double((value >> 16) & 0xff) / 255.0
        let green = Double((value >> 8) & 0xff) / 255.0
        let blue = Double(value & 0xff) / 255.0
        self.init(red: red, green: green, blue: blue)
    }
}
