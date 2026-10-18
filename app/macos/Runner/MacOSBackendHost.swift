import Cocoa
import Darwin
import FlutterMacOS
import UniformTypeIdentifiers

// Owns one unprivileged child. Readiness and credentials travel over private
// pipes; neither health on a fixed port nor an inherited environment token is used.
final class MacOSBackendHost {
  static let shared = MacOSBackendHost()
  weak var channel: FlutterMethodChannel?
  private var child: Process?
  private var input: FileHandle?
  private var ready: [String: Any]?
  private var buffer = Data()
  private var generation = 0
  private var pending: [FlutterResult] = []
  private var stopping: [() -> Void] = []
  private var startupDeadline: DispatchWorkItem?

  var isRunning: Bool { child?.isRunning == true }

  func handle(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    switch call.method {
    case "startBackend": start(result)
    case "stopBackend": stop { result(nil) }
    case "pickConfigFile":
      let panel = NSOpenPanel()
      panel.title = "Naga Network"
      panel.canChooseDirectories = false
      panel.canChooseFiles = true
      panel.allowsMultipleSelection = false
      panel.allowedContentTypes = ["json", "conf", "txt", "yaml", "yml"].compactMap {
        UTType(filenameExtension: $0)
      }
      panel.allowsOtherFileTypes = true
      panel.begin { response in result(response == .OK ? panel.url?.path : nil) }
    default: result(FlutterMethodNotImplemented)
    }
  }

  private func start(_ result: @escaping FlutterResult) {
    if !stopping.isEmpty {
      result(FlutterError(code: "backend_stopping", message: "Backend завершает работу. Повторите запуск через несколько секунд.", details: nil))
      return
    }
    if let ready, isRunning { result(ready); return }
    pending.append(result)
    if isRunning { return }
    let executable = Bundle.main.bundleURL.appendingPathComponent("Contents/Helpers/naga-control")
    guard FileManager.default.isExecutableFile(atPath: executable.path) else {
      fail("В приложении отсутствует naga-control. Используйте полный архив сборки macOS.")
      return
    }
    generation += 1
    let current = generation
    ready = nil
    buffer = Data()
    let process = Process()
    let stdout = Pipe()
    let stderr = Pipe()
    let stdin = Pipe()
    process.executableURL = executable
    process.arguments = ["--app-session"]
    var environment = ProcessInfo.processInfo.environment
    environment["HOME"] = FileManager.default.homeDirectoryForCurrentUser.path
    environment["NAGA_DATA_DIR"] = FileManager.default.homeDirectoryForCurrentUser
      .appendingPathComponent("Library/Application Support/Naga Network").path
    // The packaged app gets its token from its own child, never from a shell.
    environment.removeValue(forKey: "NAGA_CONTROL_TOKEN")
    environment.removeValue(forKey: "NAGA_CONTROL_TOKEN_FILE")
    environment.removeValue(forKey: "NAGA_SINGBOX_PATH")
    environment.removeValue(forKey: "NAGA_NATIVE_LIB_DIR")
    process.environment = environment
    process.standardInput = stdin
    process.standardOutput = stdout
    process.standardError = stderr
    input = stdin.fileHandleForWriting
    child = process
    stdout.fileHandleForReading.readabilityHandler = { [weak self] handle in
      let data = handle.availableData
      if data.isEmpty { handle.readabilityHandler = nil; return }
      DispatchQueue.main.async { self?.consume(data, generation: current) }
    }
    // Drain continuously. Application credentials are never copied into logs.
    stderr.fileHandleForReading.readabilityHandler = { handle in
      if handle.availableData.isEmpty { handle.readabilityHandler = nil }
    }
    process.terminationHandler = { [weak self] process in
      DispatchQueue.main.async {
        stdout.fileHandleForReading.readabilityHandler = nil
        stderr.fileHandleForReading.readabilityHandler = nil
        self?.exited(process, generation: current)
      }
    }
    do {
      try process.run()
      let deadline = DispatchWorkItem { [weak self] in
        guard let self, self.generation == current, self.ready == nil else { return }
        self.fail("Backend не запустился за 15 секунд.")
        self.stop {}
      }
      startupDeadline = deadline
      DispatchQueue.main.asyncAfter(deadline: .now() + 15, execute: deadline)
    } catch {
      stdout.fileHandleForReading.readabilityHandler = nil
      stderr.fileHandleForReading.readabilityHandler = nil
      child = nil
      input = nil
      fail("Не удалось запустить встроенный backend: \(error.localizedDescription)")
    }
  }

  private func consume(_ data: Data, generation current: Int) {
    guard generation == current, ready == nil, !pending.isEmpty else { return }
    buffer.append(data)
    guard buffer.count <= 16 * 1024 else {
      fail("Backend вернул слишком большой ответ запуска.")
      stop {}
      return
    }
    guard let end = buffer.firstIndex(of: 10) else { return }
    let frame = buffer.prefix(upTo: end)
    buffer = Data()
    guard let value = (try? JSONSerialization.jsonObject(with: frame)) as? [String: Any] else {
      fail("Некорректный ответ запуска backend.")
      stop {}
      return
    }
    if value["type"] as? String == "error" {
      fail(value["message"] as? String ?? "Backend отклонил запуск.")
      stop {}
      return
    }
    guard value["type"] as? String == "ready", value["protocol_version"] as? Int == 1,
      let endpoint = value["endpoint"] as? String,
      let url = URLComponents(string: endpoint), url.scheme == "http", url.host == "127.0.0.1",
      let port = url.port, (1...65535).contains(port), url.user == nil, url.password == nil,
      url.query == nil, url.fragment == nil, url.path.isEmpty,
      let token = value["token"] as? String, token.count == 64,
      token.allSatisfy({ "0123456789abcdef".contains($0) }),
      value["version"] as? String == Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String
    else {
      fail("Несовместимая версия или некорректный endpoint встроенного backend.")
      stop {}
      return
    }
    startupDeadline?.cancel()
    ready = value
    let callbacks = pending
    pending = []
    callbacks.forEach { $0(value) }
  }

  private func fail(_ message: String) {
    startupDeadline?.cancel()
    let callbacks = pending
    pending = []
    callbacks.forEach { $0(FlutterError(code: "backend_start_failed", message: message, details: nil)) }
  }

  func stop(completion: @escaping () -> Void) {
    guard let process = child, process.isRunning else { completion(); return }
    stopping.append(completion)
    ready = nil
    try? input?.close()
    input = nil
    let current = generation
    DispatchQueue.main.asyncAfter(deadline: .now() + 8) { [weak self] in
      guard let self, self.generation == current, process.isRunning else { return }
      kill(process.processIdentifier, SIGKILL)
    }
  }

  private func exited(_ process: Process, generation current: Int) {
    guard generation == current else { return }
    let expected = !stopping.isEmpty
    let hadReady = ready != nil
    child = nil
    input = nil
    ready = nil
    buffer = Data()
    fail("Backend завершился, код \(process.terminationStatus).")
    let callbacks = stopping
    stopping = []
    callbacks.forEach { $0() }
    if !expected && hadReady { channel?.invokeMethod("backendExited", arguments: ["exit_code": process.terminationStatus]) }
  }
}
