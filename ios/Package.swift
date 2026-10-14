// swift-tools-version: 6.0

import PackageDescription

let package = Package(
    name: "NagaNetwork",
    platforms: [
        .iOS(.v16),
        .macOS(.v14),
    ],
    products: [
        .library(name: "NagaControlClient", targets: ["NagaControlClient"]),
        .library(
            name: "NagaNetwork",
            targets: ["NagaNetwork"]
        ),
        .library(
            name: "PacketTunnel",
            targets: ["PacketTunnel"]
        ),
    ],
    targets: [
        .target(name: "NagaControlClient"),
        .target(name: "NagaUI", dependencies: ["NagaControlClient"]),
        .target(name: "NagaNetwork", dependencies: ["NagaUI"]),
        .target(name: "NagaKeepText", path: "packaging", exclude: ["ensure-macho-text.py"], publicHeadersPath: "include"),
        .target(name: "PacketTunnel", dependencies: ["NagaKeepText"], linkerSettings: [
            .unsafeFlags(["-Xlinker", "-u", "-Xlinker", "_naga_keep_text"], .when(platforms: [.iOS])),
        ]),
        .testTarget(name: "NagaControlClientTests", dependencies: ["NagaControlClient"],
                    resources: [.copy("Fixtures")]),
    ]
)
