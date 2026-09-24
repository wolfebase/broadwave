// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "BroadwaveUI",
    platforms: [.iOS(.v26), .tvOS(.v26), .macOS(.v26)],
    products: [
        .library(name: "BroadwaveUI", targets: ["BroadwaveUI"]),
    ],
    dependencies: [
        .package(path: "../BroadwaveKit"),
    ],
    targets: [
        .target(name: "BroadwaveUI", dependencies: ["BroadwaveKit"]),
    ]
)
