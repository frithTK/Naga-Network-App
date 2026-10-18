import Foundation

let probeAppID = "eu.nagavpn.nagaNetwork.ServiceProbe"
let probeHelperID = probeAppID + ".Helper"

#if PROBE_B
let probeBuild = "B"
let probeProtocolVersion = 2
#else
let probeBuild = "A"
let probeProtocolVersion = 1
#endif

@objc protocol NagaServiceProbeProtocol {
  func status(withReply reply: @escaping (String) -> Void)
}
