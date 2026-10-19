import CryptoKit
import Darwin
import Foundation
import Security
import SystemConfiguration

private let work = DispatchQueue(label: "eu.nagavpn.vpn-session")

// All mutations are serialized. A session belongs to one authenticated XPC
// connection, never to a client-supplied UID, PID, path or command line.
final class VPNSession {
  static let shared = VPNSession()
  private var owner: UUID?
  private var session = ""
  private var uid: uid_t = 0
  private var child: Process?
  private var interface = ""
  private var dns: SCDynamicStore?
  private var heartbeatAt = Date()
  private var directory: URL?
  private var lastCleanup = true
  private var stopping = false
  private var timer: DispatchSourceTimer?
  private let root = URL(fileURLWithPath: "/Library/PrivilegedHelperTools/eu.nagavpn.nagaNetwork.runtime", isDirectory: true)
  private let routes = [(false, "0.0.0.0/1"), (false, "128.0.0.0/1"), (true, "::/1"), (true, "8000::/1")]

  func watch() {
    let timer = DispatchSource.makeTimerSource(queue: work)
    timer.schedule(deadline: .now() + 2, repeating: 2)
    timer.setEventHandler { [weak self] in
      guard let self, self.owner != nil else { return }
      var consoleUID: uid_t = 0
      _ = SCDynamicStoreCopyConsoleUser(nil, &consoleUID, nil)
      if Date().timeIntervalSince(self.heartbeatAt) > 10 || consoleUID != self.uid {
        _ = self.cleanup()
      }
    }
    self.timer = timer
    timer.resume()
  }

  func response(_ ok: Bool = true, message: String? = nil) -> String {
    var value: [String: Any] = ["ok": ok, "protocol_version": vpnProtocolVersion, "build": vpnBuild,
      "euid": geteuid(), "active": owner != nil && child?.isRunning == true, "cleanup": lastCleanup]
    if let message { value["message"] = message }
    if !session.isEmpty { value["session"] = session }
    return vpnJSON(value)
  }

  func start(_ data: Data, id: String, connection: UUID, uid: uid_t) -> String {
    guard UUID(uuidString: id) != nil else { return response(false, message: "Некорректная сессия VPN") }
    if owner != nil {
      return owner == connection && session == id ? response() : response(false, message: "VPN уже занят другой сессией Naga")
    }
    do {
      guard let name = (123...254).map({ "utun\($0)" }).first(where: { if_nametoindex($0) == 0 }) else { throw VPNFailure("Нет свободного VPN-интерфейса") }
      let config = try VPNConfig.prepare(data, interface: name)
      try secureDirectory(root)
      let engine = try snapshotEngine()
      let dir = root.appendingPathComponent(id, isDirectory: true)
      try secureDirectory(dir)
      directory = dir
      let path = dir.appendingPathComponent("config.json")
      try writePrivate(config, path)
      // The journal precedes engine/utun/route/DNS creation. The fixed runtime
      // has its own parent watchdog; interface routes die with its utun FD.
      try writePrivate(try JSONSerialization.data(withJSONObject: ["session": id, "interface": name,
        "engine_sha256": vpnEngineSHA256, "phase": "prepared"]), dir.appendingPathComponent("journal.json"))
      owner = connection; session = id; self.uid = uid; interface = name
      heartbeatAt = Date(); lastCleanup = false
      let process = Process()
      process.executableURL = engine
      process.arguments = ["run", "-c", path.path]
      process.currentDirectoryURL = dir
      process.environment = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "HOME": dir.path, "TMPDIR": dir.path,
        "NAGA_VPN_PARENT_PID": String(getpid()), "ENABLE_DEPRECATED_LEGACY_DNS_SERVERS": "true",
        "ENABLE_DEPRECATED_OUTBOUND_DNS_RULE_ITEM": "true", "ENABLE_DEPRECATED_MISSING_DOMAIN_RESOLVER": "true"]
      process.standardInput = FileHandle.nullDevice
      process.standardOutput = FileHandle.nullDevice
      // Root logs must not grow without bound or expose profile credentials.
      // Runtime/API failures are reported by the user backend's diagnostics.
      process.standardError = FileHandle.nullDevice
      process.terminationHandler = { [weak self] process in
        work.async { if self?.child === process { _ = self?.cleanup() } }
      }
      child = process
      try process.run()
      let deadline = Date().addingTimeInterval(8)
      while if_nametoindex(name) == 0 && process.isRunning && Date() < deadline { Thread.sleep(forTimeInterval: 0.05) }
      guard process.isRunning, if_nametoindex(name) != 0 else { throw VPNFailure("VPN-движок не создал utun. Проверьте профиль и повторите подключение") }
      // Refuse an existing split-default route; never replace another VPN's
      // routes. A failed partial start rolls back only our own interface.
      for (v6, prefix) in routes {
        var arguments = ["-n", "add"]
        if v6 { arguments.append("-inet6") }
        arguments += ["-net", prefix, "-interface", name]
        let result = try command("/sbin/route", arguments)
        guard result.status == 0 else { throw VPNFailure("Маршруты заняты другой VPN или недоступны. Отключите другую VPN и повторите подключение") }
      }
      let options = [kSCDynamicStoreUseSessionKeys as String: true] as CFDictionary
      guard let store = SCDynamicStoreCreateWithOptions(nil, "Naga VPN" as CFString, options, nil, nil) else { throw VPNFailure("Не удалось открыть системную сессию DNS") }
      dns = store
      // This is a new, session-owned key. Existing Wi-Fi/Ethernet DNS settings
      // are untouched; configd removes our key even if this helper is killed.
      let key = "State:/Network/Service/Naga-\(id)/DNS" as CFString
      let resolver: [String: Any] = ["ServerAddresses": ["172.19.0.2"], "SupplementalMatchDomains": [""],
        "SupplementalMatchOrders": [0], "SupplementalMatchDomainsNoSearch": true]
      guard SCDynamicStoreSetValue(store, key, resolver as CFDictionary) else { throw VPNFailure("Не удалось подключить DNS к VPN") }
      try writePrivate(try JSONSerialization.data(withJSONObject: ["session": id, "interface": name,
        "phase": "connected", "pid": process.processIdentifier]), dir.appendingPathComponent("journal.json"), replace: true)
      heartbeatAt = Date()
      return response()
    } catch {
      _ = cleanup()
      // Do not return raw engine/config/filesystem errors across the boundary.
      return response(false, message: (error as? VPNFailure)?.description ?? "Не удалось запустить системную службу VPN")
    }
  }

  func heartbeat(_ id: String, connection: UUID) -> String {
    guard owner == connection, session == id, child?.isRunning == true else { return response(false, message: "Сессия VPN завершилась") }
    heartbeatAt = Date()
    return response()
  }

  func stop(_ id: String, connection: UUID) -> String {
    guard owner != nil else { return response(lastCleanup) }
    guard owner == connection, session == id else { return response(false, message: "Сессия VPN принадлежит другому подключению") }
    let ok = cleanup()
    return response(ok, message: ok ? nil : "Не удалось подтвердить остановку VPN")
  }

  func disconnected(_ connection: UUID) { if owner == connection { _ = cleanup() } }

  @discardableResult private func cleanup() -> Bool {
    if stopping { return lastCleanup }
    stopping = true
    defer { stopping = false }
    dns = nil // Releases the per-session DNS key, including on partial start.
    if !interface.isEmpty {
      for (v6, prefix) in routes {
        var query = ["-n", "get"]
        if v6 { query.append("-inet6") }
        query += ["-net", prefix]
        if let found = try? command("/sbin/route", query), found.status == 0,
          found.output.split(separator: "\n").contains(where: { $0.trimmingCharacters(in: .whitespaces) == "interface: \(interface)" }) {
          var args = ["-n", "delete"]
          if v6 { args.append("-inet6") }
          args += ["-net", prefix, "-interface", interface]
          _ = try? command("/sbin/route", args)
        }
      }
    }
    if let process = child, process.isRunning {
      process.interrupt()
      let grace = Date().addingTimeInterval(3)
      while process.isRunning && Date() < grace { Thread.sleep(forTimeInterval: 0.05) }
      if process.isRunning {
        // This is still our unreaped Process object, not a remembered PID.
        kill(process.processIdentifier, SIGKILL)
        let deadline = Date().addingTimeInterval(2)
        while process.isRunning && Date() < deadline { Thread.sleep(forTimeInterval: 0.05) }
      }
    }
    lastCleanup = child?.isRunning != true && (interface.isEmpty || if_nametoindex(interface) == 0)
    if lastCleanup {
      if let directory { try? FileManager.default.removeItem(at: directory) }
      child = nil; owner = nil; session = ""; interface = ""; directory = nil
    }
    return lastCleanup
  }

  private func snapshotEngine() throws -> URL {
    let target = root.appendingPathComponent("sing-box-" + vpnEngineSHA256)
    if let data = try? Data(contentsOf: target), SHA256.hash(data: data).map({ String(format: "%02x", $0) }).joined() == vpnEngineSHA256 {
      var st = stat()
      guard lstat(target.path, &st) == 0, st.st_uid == 0, st.st_mode & 0o777 == 0o700 else { throw VPNFailure("Небезопасные права VPN-движка") }
      return target
    }
    let source = URL(fileURLWithPath: CommandLine.arguments[0]).resolvingSymlinksInPath().deletingLastPathComponent().appendingPathComponent("sing-box")
    let handle = try FileHandle(forReadingFrom: source)
    defer { try? handle.close() }
    let bytes = try handle.read(upToCount: 100 * 1024 * 1024 + 1) ?? Data()
    guard bytes.count <= 100 * 1024 * 1024,
      SHA256.hash(data: bytes).map({ String(format: "%02x", $0) }).joined() == vpnEngineSHA256 else { throw VPNFailure("VPN-движок повреждён. Переустановите Naga") }
    try writePrivate(bytes, target, mode: 0o700)
    return target
  }

  private func secureDirectory(_ url: URL) throws {
    if mkdir(url.path, 0o700) != 0 && errno != EEXIST { throw VPNFailure("Не удалось создать защищённый каталог VPN") }
    var st = stat()
    guard lstat(url.path, &st) == 0, st.st_uid == 0, st.st_mode & S_IFMT == S_IFDIR,
      st.st_mode & 0o077 == 0 else { throw VPNFailure("Небезопасные права системного каталога VPN") }
  }

  private func writePrivate(_ bytes: Data, _ path: URL, mode: mode_t = 0o600, replace: Bool = false) throws {
    if replace { guard unlink(path.path) == 0 || errno == ENOENT else { throw VPNFailure("Ошибка журнала VPN") } }
    let fd = open(path.path, O_WRONLY | O_CREAT | O_EXCL | O_NOFOLLOW | O_CLOEXEC, mode)
    guard fd >= 0 else { throw VPNFailure("Не удалось сохранить защищённую сессию VPN") }
    let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
    try handle.write(contentsOf: bytes)
    try handle.synchronize()
    try handle.close()
  }

  private func command(_ executable: String, _ args: [String]) throws -> (status: Int32, output: String) {
    let process = Process()
    process.executableURL = URL(fileURLWithPath: executable); process.arguments = args
    process.environment = ["PATH": "/usr/bin:/bin:/usr/sbin:/sbin"]
    let pipe = Pipe(); process.standardOutput = pipe; process.standardError = FileHandle.nullDevice
    try process.run()
    let deadline = Date().addingTimeInterval(2)
    while process.isRunning && Date() < deadline { Thread.sleep(forTimeInterval: 0.02) }
    if process.isRunning { process.terminate(); throw VPNFailure("Система не ответила на изменение маршрутов VPN") }
    let data = try pipe.fileHandleForReading.read(upToCount: 16384) ?? Data()
    return (process.terminationStatus, String(decoding: data, as: UTF8.self))
  }
}

final class VPNEndpoint: NSObject, NagaVPNProtocol {
  let id = UUID()
  let uid: uid_t
  init(uid: uid_t) { self.uid = uid }
  func start(_ config: Data, session: String, withReply reply: @escaping (String) -> Void) {
    guard config.count <= vpnConfigLimit else { reply(vpnJSON(["ok": false, "message": "Конфигурация превышает 8 МБ"])); return }
    work.async { reply(VPNSession.shared.start(config, id: session, connection: self.id, uid: self.uid)) }
  }
  func stop(_ session: String, withReply reply: @escaping (String) -> Void) { work.async { reply(VPNSession.shared.stop(session, connection: self.id)) } }
  func status(_ session: String, withReply reply: @escaping (String) -> Void) { work.async { reply(VPNSession.shared.response()) } }
  func heartbeat(_ session: String, withReply reply: @escaping (String) -> Void) { work.async { reply(VPNSession.shared.heartbeat(session, connection: self.id)) } }
}

final class VPNListener: NSObject, NSXPCListenerDelegate {
  func listener(_ listener: NSXPCListener, shouldAcceptNewConnection connection: NSXPCConnection) -> Bool {
    var consoleUID: uid_t = 0
    _ = SCDynamicStoreCopyConsoleUser(nil, &consoleUID, nil)
    guard consoleUID != 0, connection.effectiveUserIdentifier == consoleUID else { return false }
    // Exact build hash, not an identifier-only ad-hoc authority. The bridge
    // permits only the fixed API/schema above, with no exec/path parameters.
    connection.setCodeSigningRequirement("identifier \"\(vpnBridgeID)\" and cdhash H\"\(vpnBridgeCDHash)\"")
    let endpoint = VPNEndpoint(uid: consoleUID)
    connection.exportedInterface = NSXPCInterface(with: NagaVPNProtocol.self)
    connection.exportedObject = endpoint
    connection.invalidationHandler = { work.async { VPNSession.shared.disconnected(endpoint.id) } }
    connection.interruptionHandler = { work.async { VPNSession.shared.disconnected(endpoint.id) } }
    connection.resume()
    return true
  }
}

@main enum VPNHelperMain {
  static func main() {
    guard geteuid() == 0 && !CommandLine.arguments.contains("--self-check") else {
      print(vpnJSON(["protocol_version": vpnProtocolVersion, "build": vpnBuild, "euid": geteuid(),
        "bridge_cdhash": vpnBridgeCDHash, "engine_sha256": vpnEngineSHA256,
        "scope": "vpn helper self-check; no network changes"]))
      return
    }
    let delegate = VPNListener()
    let listener = NSXPCListener(machServiceName: vpnHelperID)
    listener.delegate = delegate
    work.async { VPNSession.shared.watch() }
    listener.resume()
    withExtendedLifetime(delegate) { RunLoop.main.run() }
  }
}
