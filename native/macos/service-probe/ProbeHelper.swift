import Darwin
import Foundation

final class ProbeEndpoint: NSObject, NagaServiceProbeProtocol {
  func status(withReply reply: @escaping (String) -> Void) {
    let result: [String: Any] = [
      "build": probeBuild,
      "protocol_version": probeProtocolVersion,
      "euid": geteuid(),
      "pid": getpid(),
      "scope": "diagnostic status only",
    ]
    let data = try! JSONSerialization.data(withJSONObject: result, options: [.sortedKeys])
    reply(String(decoding: data, as: UTF8.self))
  }
}

final class ProbeListener: NSObject, NSXPCListenerDelegate {
  func listener(_ listener: NSXPCListener, shouldAcceptNewConnection connection: NSXPCConnection) -> Bool {
    // This deliberately limited ad-hoc prototype has no privileged mutation API.
    // A production Team ID/bootstrap authority policy is a separate M00.05 task.
    connection.setCodeSigningRequirement("identifier \"\(probeAppID)\"")
    connection.exportedInterface = NSXPCInterface(with: NagaServiceProbeProtocol.self)
    connection.exportedObject = ProbeEndpoint()
    connection.resume()
    return true
  }
}

@main enum ProbeHelperMain {
  static func main() {
    if CommandLine.arguments.contains("--self-check") {
      ProbeEndpoint().status { print($0) }
      return
    }
    let delegate = ProbeListener()
#if INTEGRATED_DIAGNOSTIC
    // A killed test UI must not leave a permanently running diagnostic process.
    // Registration is cleaned by the app on completion, cancellation or next launch.
    DispatchQueue.global().asyncAfter(deadline: .now() + 60) { exit(0) }
#endif
    let listener = NSXPCListener(machServiceName: probeHelperID)
    listener.delegate = delegate
    listener.resume()
    withExtendedLifetime(delegate) { RunLoop.main.run() }
  }
}
