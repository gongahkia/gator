// swift-tools-version: 6.1
import PackageDescription

let package = Package(
    name: "GatorNative",
    platforms: [.macOS(.v15)],
    products: [.executable(name: "GatorMac", targets: ["GatorMac"])],
    targets: [
        .target(name: "GatorCore"),
        .executableTarget(name: "GatorMac", dependencies: ["GatorCore"]),
        .testTarget(name: "GatorCoreTests", dependencies: ["GatorCore"], resources: [.copy("Fixtures")])
    ]
)
