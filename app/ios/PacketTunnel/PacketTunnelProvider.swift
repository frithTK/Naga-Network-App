import NetworkExtension

/// Packet tunnel datapath. Until libbox is linked, startTunnel fails closed
/// so iOS never leaves a dead VPN interface up.
/// Linux-сборка живёт в ios/Sources/PacketTunnel (xtool), не в этом файле.
final class PacketTunnelProvider: NEPacketTunnelProvider {
  override func startTunnel(
    options: [String: NSObject]?,
    completionHandler: @escaping (Error?) -> Void
  ) {
    completionHandler(
      NSError(
        domain: "eu.nagavpn.naga_network.PacketTunnel",
        code: 1,
        userInfo: [
          NSLocalizedDescriptionKey: "VPN-ядро ещё не собрано для этого устройства.",
        ]
      )
    )
  }

  override func stopTunnel(
    with reason: NEProviderStopReason,
    completionHandler: @escaping () -> Void
  ) {
    completionHandler()
  }
}
