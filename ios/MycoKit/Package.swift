// swift-tools-version: 6.0
import PackageDescription

// The iOS app's code, split by responsibility. Dependencies point down only:
//
//   MycoFeatures  pages, their view models' wiring, navigation
//     ├─ MycoUI     reusable views; plain values in, no network, no storage
//     └─ MycoCore   conversation and run state, sync, naming rules
//          ├─ MycoAPI     the hub's JSON contract and client protocol
//          ├─ MycoCrypto  end-to-end encryption envelopes
//          └─ MycoStore   keychain, drafts, the list of workspaces
//   MycoFake      an in-memory workspace behind MycoAPI's protocol
let package = Package(
    name: "MycoKit",
    platforms: [.iOS(.v17)],
    products: [
        .library(name: "MycoFeatures", targets: ["MycoFeatures"]),
        .library(name: "MycoCore", targets: ["MycoCore"]),
        .library(name: "MycoFake", targets: ["MycoFake"]),
    ],
    targets: [
        .target(name: "MycoAPI"),
        .target(name: "MycoCrypto"),
        .target(name: "MycoStore"),
        .target(name: "MycoCore", dependencies: ["MycoAPI", "MycoCrypto", "MycoStore"]),
        .target(name: "MycoFake", dependencies: ["MycoAPI"]),
        .target(name: "MycoUI"),
        .target(name: "MycoFeatures", dependencies: ["MycoCore", "MycoUI", "MycoAPI"]),
        .testTarget(name: "MycoAPITests", dependencies: ["MycoAPI"]),
        .testTarget(name: "MycoCoreTests", dependencies: ["MycoCore", "MycoFake"]),
        .testTarget(name: "MycoUITests", dependencies: ["MycoUI"], resources: [.copy("Fixtures")]),
    ]
)
