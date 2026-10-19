import Darwin
import Foundation
import Security

private func readConfig() throws -> Data {
  var data = Data()
  while data.count <= vpnConfigLimit {
    let chunk = try FileHandle.standardInput.read(upToCount: min(65536, vpnConfigLimit + 1 - data.count)) ?? Data()
    guard !chunk.isEmpty else { throw VPNFailure("Не получена конфигурация VPN") }
    if let newline = chunk.firstIndex(of: 10) {
      data.append(chunk.prefix(upTo: newline))
      guard data.count <= vpnConfigLimit else { break }
      return data
    }
    data.append(chunk)
  }
  throw VPNFailure("Конфигурация VPN превышает 8 МБ")
}

func helperCDHash() throws -> String {
  let path = URL(fileURLWithPath: CommandLine.arguments[0]).resolvingSymlinksInPath()
    .deletingLastPathComponent().appendingPathComponent("NagaVPNHelper")
  var code: SecStaticCode?
  guard SecStaticCodeCreateWithPath(path as CFURL, [], &code) == errSecSuccess, let code,
    SecStaticCodeCheckValidity(code, [.strictValidate], nil) == errSecSuccess else { throw VPNFailure("Подпись службы VPN повреждена") }
  var info: CFDictionary?
  guard SecCodeCopySigningInformation(code, SecCSFlags(rawValue: kSecCSSigningInformation), &info) == errSecSuccess,
    let values = info as? [String: Any], let hash = values[kSecCodeInfoUnique as String] as? Data else { throw VPNFailure("Не удалось проверить версию службы VPN") }
  return hash.map { String(format: "%02x", $0) }.joined()
}

@main enum VPNBridgeMain {
  static func main() {
    func emit(_ value: [String: Any]) { print(vpnJSON(value)); fflush(stdout) }
    func fail(_ message: String) -> Never { emit(["type": "error", "message": message]); exit(1) }
    guard geteuid() != 0 else { fail("VPN bridge должен работать от пользователя") }
    let statusOnly = CommandLine.arguments.dropFirst() == ["--status"]
    var data = Data()
    if !statusOnly {
      do { data = try readConfig() } catch { fail("Некорректная конфигурация VPN или превышен размер 8 МБ") }
    }
    let hash: String
    do { hash = try helperCDHash() } catch { fail("Переустановите Naga: не удалось проверить системную службу VPN") }
    let connection = NSXPCConnection(machServiceName: vpnHelperID, options: .privileged)
    connection.setCodeSigningRequirement("identifier \"\(vpnHelperID)\" and cdhash H\"\(hash)\"")
    connection.remoteObjectInterface = NSXPCInterface(with: NagaVPNProtocol.self)
    let died = DispatchSemaphore(value: 0)
    connection.invalidationHandler = { died.signal() }
    connection.interruptionHandler = { died.signal() }
    connection.resume()
    let session = UUID().uuidString
    let lock = NSLock()
    var startResult: [String: Any]?
    let started = DispatchSemaphore(value: 0)
    guard let proxy = connection.remoteObjectProxyWithErrorHandler({ _ in started.signal(); died.signal() }) as? NagaVPNProtocol else { fail("Служба VPN не отвечает") }
    func accept(_ raw: String) -> [String: Any]? {
      guard raw.utf8.count <= 16384, let bytes = raw.data(using: .utf8),
        let value = (try? JSONSerialization.jsonObject(with: bytes)) as? [String: Any],
        value["protocol_version"] as? Int == vpnProtocolVersion, value["build"] as? String == vpnBuild,
        value["euid"] as? Int == 0, connection.effectiveUserIdentifier == 0 else { return nil }
      return value
    }
    let receive: (String) -> Void = { raw in
      lock.lock(); startResult = accept(raw); lock.unlock(); started.signal()
    }
    if statusOnly { proxy.status("", withReply: receive) } else { proxy.start(data, session: session, withReply: receive) }
    guard started.wait(timeout: .now() + 16) == .success else { connection.invalidate(); fail("Истекло время запуска службы VPN") }
    lock.lock(); let first = startResult; lock.unlock()
    guard let first, first["ok"] as? Bool == true else {
      connection.invalidate()
      fail((first?["message"] as? String) ?? "Служба VPN недоступна. Разрешите Naga в настройках фоновых элементов")
    }
    if statusOnly { print(vpnJSON(first)); connection.invalidate(); return }
    guard first["active"] as? Bool == true, first["session"] as? String == session else { connection.invalidate(); fail("Системная служба не подтвердила запуск VPN") }
    emit(["type": "ready", "protocol_version": vpnProtocolVersion])
    let stopped = DispatchSemaphore(value: 0)
    var stopOK = false
    var stopRequested = false
    DispatchQueue.global().async {
      // The backend owns this private pipe. EOF also covers backend/UI death.
      while let chunk = try? FileHandle.standardInput.read(upToCount: 65536), !chunk.isEmpty {}
      lock.lock(); stopRequested = true; lock.unlock()
      proxy.stop(session) { raw in
        let value = accept(raw)
        lock.lock(); stopOK = value?["ok"] as? Bool == true && value?["cleanup"] as? Bool == true; lock.unlock()
        stopped.signal()
      }
    }
    while true {
      if stopped.wait(timeout: .now() + 2) == .success {
        lock.lock(); let ok = stopOK; lock.unlock()
        connection.invalidate()
        if ok { emit(["type": "stopped", "cleanup": true]); return }
        fail("Служба не подтвердила восстановление сети")
      }
      if died.wait(timeout: .now()) == .success { connection.invalidate(); fail("Соединение со службой VPN прервалось") }
      lock.lock(); let isStopping = stopRequested; lock.unlock()
      if isStopping { continue }
      let pulse = DispatchSemaphore(value: 0)
      var alive = false
      proxy.heartbeat(session) { raw in
        let value = accept(raw)
        lock.lock(); alive = value?["ok"] as? Bool == true && value?["active"] as? Bool == true; lock.unlock(); pulse.signal()
      }
      if pulse.wait(timeout: .now() + 4) != .success {
        lock.lock(); let isStopping = stopRequested; lock.unlock()
        if isStopping { continue }
        connection.invalidate(); fail("Служба VPN перестала отвечать")
      }
      lock.lock(); let ok = alive; lock.unlock()
      lock.lock(); let stoppingNow = stopRequested; lock.unlock()
      if stoppingNow { continue }
      guard ok else { connection.invalidate(); fail("VPN-движок завершил работу; временные настройки сети удаляются") }
    }
  }
}
