import Cocoa
import Darwin
import FlutterMacOS
import ServiceManagement

// A diagnostic service with status only. It never owns a VPN session.
final class MacOSQuickCheck {
  static let shared = MacOSQuickCheck()
  private let service = SMAppService.daemon(plistName: probeHelperID + ".plist")
  private var completion: FlutterResult?
  private var generation = 0
  private var connection: NSXPCConnection?
  private var report: [String: Any] = [:]
  private var checks: [[String: Any]] = []
  private var stage = "idle"
  private var pollDeadline = Date()
  private var finishing = false
  private var shouldCleanup = false

  var isRunning: Bool { completion != nil }
  var state: [String: Any] { ["stage": stage, "running": isRunning] }

  private var registration: String {
    switch service.status {
    case .notRegistered: return "not_registered"
    case .enabled: return "enabled"
    case .requiresApproval: return "requires_approval"
    case .notFound: return "not_found"
    @unknown default: return "unknown"
    }
  }

  // Removes only our diagnostic ID after an interrupted previous run.
  func cleanInterruptedCheck() {
    guard !isRunning, service.status != .notRegistered else { return }
    try? service.unregister()
  }

  func run(result: @escaping FlutterResult) {
    guard !isRunning else {
      result(FlutterError(code: "check_busy", message: "Проверка уже выполняется.", details: nil))
      return
    }
    generation += 1
    completion = result
    finishing = false
    shouldCleanup = false
    stage = "starting"
    checks = []
    let os = ProcessInfo.processInfo.operatingSystemVersion
    report = [
      "schema_version": 1,
      "diagnostic_version": probeBuild,
      "system": ["macos": "\(os.majorVersion).\(os.minorVersion).\(os.patchVersion)",
                 "description": ProcessInfo.processInfo.operatingSystemVersionString,
                 "architecture": "arm64"],
      "app": ["version": Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "",
              "build": Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? ""],
      "vpn_tested": false,
    ]
    let parent = Bundle.main.bundleURL.deletingLastPathComponent().standardizedFileURL.path
    guard parent == "/Applications" else {
      checks.append(["id": "installation", "status": "needs_action"])
      finish("install_required")
      return
    }
    checks.append(["id": "installation", "status": "pass"])
    guard geteuid() != 0 else {
      checks.append(["id": "user_process", "status": "fail"])
      finish("failed")
      return
    }
    checks.append(["id": "user_process", "status": "pass"])
    stage = "registering"
    shouldCleanup = true
    do {
      if service.status != .enabled { try service.register() }
    } catch {
      record(error, phase: "registration")
      finish("failed")
      return
    }
    awaitApproval(generation)
  }

  private func awaitApproval(_ current: Int) {
    guard current == generation, isRunning, !finishing else { return }
    switch service.status {
    case .enabled:
      checks.append(["id": "registration", "status": "pass"])
      ping(current)
    case .requiresApproval:
      if stage != "waiting_permission" {
        stage = "waiting_permission"
        pollDeadline = Date().addingTimeInterval(60)
        SMAppService.openSystemSettingsLoginItems()
      }
      if Date() >= pollDeadline {
        checks.append(["id": "registration", "status": "needs_action"])
        finish("permission_required")
      } else {
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in self?.awaitApproval(current) }
      }
    default:
      checks.append(["id": "registration", "status": "fail", "state": registration])
      finish("failed")
    }
  }

  private func ping(_ current: Int) {
    stage = "checking_service"
    let ipc = NSXPCConnection(machServiceName: probeHelperID, options: .privileged)
    connection = ipc
    ipc.remoteObjectInterface = NSXPCInterface(with: NagaServiceProbeProtocol.self)
    ipc.setCodeSigningRequirement("identifier \"\(probeHelperID)\"")
    ipc.resume()
    let proxy = ipc.remoteObjectProxyWithErrorHandler { [weak self] error in
      DispatchQueue.main.async {
        guard let self, self.generation == current, self.isRunning, !self.finishing else { return }
        self.record(error, phase: "service_connection")
        self.finish("failed")
      }
    } as? NagaServiceProbeProtocol
    guard let proxy else { finish("failed"); return }
    DispatchQueue.main.asyncAfter(deadline: .now() + 5) { [weak self] in
      guard let self, self.generation == current, self.isRunning, !self.finishing else { return }
      self.checks.append(["id": "service_connection", "status": "timeout"])
      self.finish("failed")
    }
    proxy.status { [weak self] response in
      DispatchQueue.main.async {
        guard let self, self.generation == current, self.isRunning, !self.finishing else { return }
        guard response.utf8.count <= 4096,
          let value = (try? JSONSerialization.jsonObject(with: Data(response.utf8))) as? [String: Any],
          ipc.effectiveUserIdentifier == 0, value["euid"] as? Int == 0,
          value["protocol_version"] as? Int == probeProtocolVersion, value["build"] as? String == probeBuild
        else {
          self.checks.append(["id": "service_connection", "status": "invalid_reply"])
          self.finish("failed")
          return
        }
        self.checks.append(["id": "service_connection", "status": "pass", "effective_uid": 0,
                            "protocol_version": probeProtocolVersion, "helper_build": probeBuild])
        self.finish("pass")
      }
    }
  }

  // Reports only error domain/code; userInfo/localizedDescription may contain paths.
  private func record(_ error: Error, phase: String) {
    let value = error as NSError
    checks.append(["id": phase, "status": "fail", "error_domain": value.domain, "error_code": value.code])
  }

  private func finish(_ outcome: String) {
    guard isRunning, !finishing else { return }
    finishing = true
    stage = "cleaning"
    connection?.invalidate()
    connection = nil
    report["outcome"] = outcome
    report["checks"] = checks
    report["registration_before_cleanup"] = registration
    if !shouldCleanup {
      report["cleanup"] = ["status": "not_needed", "registration": registration]
      stage = "complete"
      let result = completion
      completion = nil
      finishing = false
      result?(report)
      return
    }
    do {
      if service.status != .notRegistered { try service.unregister() }
    } catch {
      let value = error as NSError
      report["cleanup_error"] = ["domain": value.domain, "code": value.code]
    }
    awaitCleanup(generation, until: Date().addingTimeInterval(5))
  }

  private func awaitCleanup(_ current: Int, until deadline: Date) {
    guard current == generation, isRunning else { return }
    if service.status != .notRegistered && Date() < deadline {
      DispatchQueue.main.asyncAfter(deadline: .now() + 0.25) { [weak self] in self?.awaitCleanup(current, until: deadline) }
      return
    }
    report["cleanup"] = ["status": service.status == .notRegistered ? "pass" : "fail", "registration": registration]
    stage = "complete"
    let result = completion
    completion = nil
    finishing = false
    result?(report)
  }

  func cancel(result: @escaping FlutterResult) {
    guard isRunning else {
      cleanInterruptedCheck()
      result(nil)
      return
    }
    finish("cancelled")
    result(nil)
  }

  func retryCleanup(result: @escaping FlutterResult) {
    guard !isRunning else {
      result(FlutterError(code: "check_busy", message: "Проверка ещё выполняется.", details: nil))
      return
    }
    generation += 1
    completion = result
    finishing = true
    stage = "cleaning"
    report.removeValue(forKey: "cleanup_error")
    do {
      if service.status != .notRegistered { try service.unregister() }
    } catch {
      let value = error as NSError
      report["cleanup_error"] = ["domain": value.domain, "code": value.code]
    }
    awaitCleanup(generation, until: Date().addingTimeInterval(5))
  }

  func prepareToQuit(completion done: @escaping () -> Void) {
    guard isRunning else { cleanInterruptedCheck(); done(); return }
    let previous = completion
    completion = { value in previous?(value); done() }
    finish("cancelled")
  }
}
