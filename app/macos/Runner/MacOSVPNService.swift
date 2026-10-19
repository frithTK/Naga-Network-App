import Cocoa
import FlutterMacOS
import ServiceManagement

// Permission/UI live in the ordinary app. The backend never installs a daemon
// or opens System Settings, and the root service never displays UI.
final class MacOSVPNService {
  static let shared = MacOSVPNService()
  private let service = SMAppService.daemon(plistName: "eu.nagavpn.nagaNetwork.VPNHelper.plist")
  private var pending: [FlutterResult] = []
  private var deadline = Date()
  private var last: [String: Any] = ["ready": false, "registration": "not_checked"]

  func prepare(_ result: @escaping FlutterResult) {
    guard Bundle.main.bundleURL.deletingLastPathComponent().path == "/Applications" else {
      result(FlutterError(code: "vpn_installation", message: "Перенесите Naga Network в «Программы» и откройте оттуда.", details: nil)); return
    }
    pending.append(result)
    guard pending.count == 1 else { return }
    deadline = Date().addingTimeInterval(90)
    do {
      if service.status == .notRegistered || service.status == .notFound { try service.register() }
      if service.status == .requiresApproval { SMAppService.openSystemSettingsLoginItems() }
      poll()
    } catch {
      finish(FlutterError(code: "vpn_registration", message: "Не удалось включить службу Naga. Проверьте разрешение в «Настройки системы → Основные → Объекты входа и расширения».", details: nil))
    }
  }

  private func poll() {
    guard !pending.isEmpty else { return }
    switch service.status {
    case .enabled:
      query { value in
        self.last = value.merging(["registration": "enabled"]) { _, new in new }
        if value["ok"] as? Bool == true {
          self.finish(["ready": true, "registration": "enabled", "protocol_version": 1])
        } else {
          self.finish(FlutterError(code: "vpn_service", message: "Системная служба VPN не отвечает или относится к другой сборке. Закройте Naga, отключите её фоновый элемент в настройках и повторите запуск.", details: nil))
        }
      }
    case .requiresApproval:
      if Date() >= deadline {
        finish(FlutterError(code: "vpn_permission", message: "Разрешите Naga Network в настройках фоновых элементов и повторите подключение.", details: nil))
      } else { DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { self.poll() } }
    default:
      finish(FlutterError(code: "vpn_disabled", message: "Служба VPN отключена. Разрешите Naga в настройках фоновых элементов.", details: nil))
    }
  }

  func status(_ result: @escaping FlutterResult) {
    guard service.status == .enabled else {
      result(["ready": false, "registration": service.status == .requiresApproval ? "requires_approval" : "not_registered"]); return
    }
    query { value in result(value.merging(["registration": "enabled"]) { _, new in new }) }
  }

  private func query(_ completion: @escaping ([String: Any]) -> Void) {
    DispatchQueue.global(qos: .userInitiated).async {
      let process = Process()
      process.executableURL = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers/NagaVPNBridge")
      process.arguments = ["--status"]
      process.environment = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin"]
      let output = Pipe(); process.standardOutput = output; process.standardError = FileHandle.nullDevice
      process.standardInput = FileHandle.nullDevice
      var value: [String: Any] = ["ok": false]
      do {
        try process.run()
        let end = Date().addingTimeInterval(18)
        while process.isRunning && Date() < end { Thread.sleep(forTimeInterval: 0.05) }
        if process.isRunning { process.terminate() }
        else if process.terminationStatus == 0 {
          let data = try output.fileHandleForReading.read(upToCount: 16384) ?? Data()
          if let raw = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any] {
            // A finite diagnostics projection: no profile, path or raw error.
            for key in ["ok", "active", "cleanup", "euid", "protocol_version", "build"] { value[key] = raw[key] }
          }
        }
      } catch {}
      let captured = value
      DispatchQueue.main.async { completion(captured) }
    }
  }

  private func finish(_ value: Any) {
    let callbacks = pending; pending = []
    callbacks.forEach { $0(value) }
  }
}
