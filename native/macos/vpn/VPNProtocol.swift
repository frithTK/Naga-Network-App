import Foundation

let vpnHelperID = "eu.nagavpn.nagaNetwork.VPNHelper"
let vpnBridgeID = "eu.nagavpn.nagaNetwork.VPNBridge"
let vpnProtocolVersion = 1
let vpnBuild = "vpn-1"
let vpnConfigLimit = 8 * 1024 * 1024

#if os(macOS)
@objc protocol NagaVPNProtocol {
  func start(_ config: Data, session: String, withReply reply: @escaping (String) -> Void)
  func stop(_ session: String, withReply reply: @escaping (String) -> Void)
  func status(_ session: String, withReply reply: @escaping (String) -> Void)
  func heartbeat(_ session: String, withReply reply: @escaping (String) -> Void)
}
#endif

func vpnJSON(_ object: [String: Any]) -> String {
  guard let data = try? JSONSerialization.data(withJSONObject: object, options: [.sortedKeys]) else { return "{}" }
  return String(decoding: data, as: UTF8.self)
}

struct VPNFailure: Error, CustomStringConvertible {
  let description: String
  init(_ description: String) { self.description = description }
}
