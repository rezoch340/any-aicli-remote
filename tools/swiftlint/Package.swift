// swift-tools-version: 5.9
import PackageDescription
let package = Package(
    name: "SwiftLintTooling",
    platforms: [.macOS(.v12)],
    dependencies: [
        .package(url: "https://github.com/SimplyDanny/SwiftLintPlugins.git", exact: "0.65.0")
    ],
    targets: []
)
