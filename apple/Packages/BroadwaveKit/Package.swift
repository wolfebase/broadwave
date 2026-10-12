// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "BroadwaveKit",
    defaultLocalization: "en",
    platforms: [.iOS(.v26), .tvOS(.v26), .macOS(.v26)],
    products: [
        .library(name: "BroadwaveKit", targets: ["BroadwaveKit"]),
    ],
    targets: [
        // Named, because SwiftPM before 6.4 does not treat a String Catalog as a resource on its own.
        .target(name: "BroadwaveKit", resources: [.process("Localizable.xcstrings")]),
        .testTarget(name: "BroadwaveKitTests", dependencies: ["BroadwaveKit"]),
    ]
)
