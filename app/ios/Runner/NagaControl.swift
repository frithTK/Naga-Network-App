import Foundation

enum NagaAppIdentity {
  static let appGroup = "group.eu.nagavpn.naga_network"
  static let tunnelBundleID = "eu.nagavpn.naga_network.PacketTunnel"
  static let displayName = "Naga Network"
}

enum NagaControl {
  static func dataDirectory() -> URL {
    if let group = FileManager.default.containerURL(
      forSecurityApplicationGroupIdentifier: NagaAppIdentity.appGroup
    ) {
      return group
    }
    return FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
      .appendingPathComponent("naga-network", isDirectory: true)
  }

  static func start() {
    let dir = dataDirectory()
    try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    // Linux-сборка — ios/ через xtool. Этот Runner — запасной Flutter/Xcode путь.
  }
}
