// swift-tools-version: 6.2
import PackageDescription

let package = Package(
    name: "OTAUI",
    platforms: [.iOS(.v26), .tvOS(.v26), .macOS(.v26)],
    products: [
        .library(name: "OTAUI", targets: ["OTAUI"]),
    ],
    dependencies: [
        .package(path: "../OTAKit"),
    ],
    targets: [
        .target(name: "OTAUI", dependencies: ["OTAKit"]),
    ]
)
