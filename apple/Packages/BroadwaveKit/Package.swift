// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "BroadwaveKit",
    platforms: [.iOS(.v26), .tvOS(.v26), .macOS(.v26)],
    products: [
        .library(name: "BroadwaveKit", targets: ["BroadwaveKit"]),
    ],
    targets: [
        .target(name: "BroadwaveKit", resources: [.copy("DemoMedia")]),
        .testTarget(name: "BroadwaveKitTests", dependencies: ["BroadwaveKit"]),
    ]
)
