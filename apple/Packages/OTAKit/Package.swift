// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "OTAKit",
    platforms: [.iOS(.v26), .tvOS(.v26), .macOS(.v26)],
    products: [
        .library(name: "OTAKit", targets: ["OTAKit"]),
    ],
    targets: [
        .target(name: "OTAKit"),
        .testTarget(name: "OTAKitTests", dependencies: ["OTAKit"], resources: [.copy("Fixtures")]),
    ]
)
