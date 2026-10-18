import Foundation

#if INTEGRATED_DIAGNOSTIC
let probeAppID = "eu.nagavpn.nagaNetwork"
let probeHelperID = probeAppID + ".DiagnosticHelper"
let probeBuild = "integrated-1"
let probeProtocolVersion = 1
#else
let probeAppID = "eu.nagavpn.nagaNetwork.ServiceProbe"
let probeHelperID = probeAppID + ".Helper"
#if PROBE_B
let probeBuild = "B"
let probeProtocolVersion = 2
#else
let probeBuild = "A"
let probeProtocolVersion = 1
#endif
#endif

@objc protocol NagaServiceProbeProtocol {
  func status(withReply reply: @escaping (String) -> Void)
}
