// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "SpellcheckMac",
    platforms: [.macOS(.v14)],
    products: [
        .library(name: "SpellcheckCore", targets: ["SpellcheckCore"]),
        .executable(name: "SpellcheckMac", targets: ["SpellcheckMac"]),
    ],
    targets: [
        .target(
            name: "SpellcheckCore",
            path: "Sources/SpellcheckCore"
        ),
        .executableTarget(
            name: "SpellcheckMac",
            dependencies: ["SpellcheckCore"],
            path: "Sources/SpellcheckMac",
            linkerSettings: [
                .linkedFramework("AppKit"),
                .linkedFramework("Carbon"),
                .linkedFramework("ApplicationServices"),
            ]
        ),
    ]
)
