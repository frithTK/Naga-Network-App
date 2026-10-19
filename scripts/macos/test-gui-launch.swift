import AppKit
import CoreGraphics
import Darwin
import Foundation

// Exercises LaunchServices, not just the executable or its signature. Never
// clicks the privileged diagnostic button or registers a service.
let bundle = URL(fileURLWithPath: CommandLine.arguments[1]).standardizedFileURL
let destination = URL(fileURLWithPath: CommandLine.arguments[2])
var report: [String: Any] = ["schema_version": 1, "vpn_tested": false,
                           "quarantine_acceptance": "UNAVAILABLE"]
var launched: NSRunningApplication?
var launchError: NSError?
var replied = false

func waitUntil(_ seconds: TimeInterval, _ predicate: () -> Bool) -> Bool {
  let deadline = Date().addingTimeInterval(seconds)
  while Date() < deadline {
    if predicate() { return true }
    RunLoop.current.run(until: Date().addingTimeInterval(0.1))
  }
  return predicate()
}

func backendPIDs(_ parent: pid_t) -> [pid_t] {
  let process = Process()
  process.executableURL = URL(fileURLWithPath: "/bin/ps")
  process.arguments = ["-axo", "pid=,ppid=,comm="]
  let pipe = Pipe()
  process.standardOutput = pipe
  do { try process.run() } catch { return [] }
  let data = pipe.fileHandleForReading.readDataToEndOfFile()
  process.waitUntilExit()
  return String(decoding: data, as: UTF8.self).split(separator: "\n").compactMap { line in
    let fields = line.split(maxSplits: 2, omittingEmptySubsequences: true, whereSeparator: { $0 == " " || $0 == "\t" })
    guard fields.count == 3, Int32(fields[1]) == parent,
      URL(fileURLWithPath: String(fields[2])).lastPathComponent == "naga-control" else { return nil }
    return Int32(fields[0])
  }
}

func visibleWindow(_ pid: pid_t) -> Bool {
  let list = CGWindowListCopyWindowInfo([.optionAll, .excludeDesktopElements], kCGNullWindowID) as? [[String: Any]] ?? []
  return list.contains { value in
    guard value[kCGWindowOwnerPID as String] as? Int32 == pid,
      value[kCGWindowLayer as String] as? Int == 0,
      let bounds = value[kCGWindowBounds as String] as? [String: Any],
      let width = bounds["Width"] as? Double,
      let height = bounds["Height"] as? Double else { return false }
    return width >= 400 && height >= 300
  }
}

let configuration = NSWorkspace.OpenConfiguration()
configuration.createsNewApplicationInstance = true
configuration.activates = true
configuration.promptsUserIfNeeded = false
NSWorkspace.shared.openApplication(at: bundle, configuration: configuration) { application, error in
  DispatchQueue.main.async {
    launched = application
    launchError = error as NSError?
    replied = true
  }
}
var success = false
if !waitUntil(20, { replied }) {
  report["launch_services"] = "timeout"
} else if let error = launchError {
  report["launch_services"] = "fail"
  report["error"] = ["domain": error.domain, "code": error.code,
                     "description": error.localizedDescription]
} else if let app = launched {
  report["launch_services"] = "pass"
  let pid = app.processIdentifier
  let ready = waitUntil(25) { app.isTerminated || (app.isFinishedLaunching && visibleWindow(pid) && !backendPIDs(pid).isEmpty) }
  let children = backendPIDs(pid)
  report["finished_launching"] = app.isFinishedLaunching
  report["window"] = visibleWindow(pid)
  report["user_backend"] = !children.isEmpty
  report["terminated_early"] = app.isTerminated
  if ready && !app.isTerminated && report["window"] as? Bool == true && !children.isEmpty {
    // A process that crashes just after presenting its window must still fail.
    let earlyExit = waitUntil(5) { app.isTerminated }
    success = !earlyExit
    report["stable_for_five_seconds"] = !earlyExit
  }
  app.terminate()
  let cleanExit = waitUntil(15) { app.isTerminated }
  if !cleanExit { app.forceTerminate(); _ = waitUntil(3) { app.isTerminated } }
  let childrenStopped = waitUntil(10) { children.allSatisfy { kill($0, 0) != 0 && errno == ESRCH } }
  report["graceful_exit"] = cleanExit
  report["backend_stopped"] = childrenStopped
  success = success && cleanExit && childrenStopped
}
report["result"] = success ? "PASS" : "FAIL"
let data = try JSONSerialization.data(withJSONObject: report, options: [.prettyPrinted, .sortedKeys])
try data.write(to: destination)
print(String(decoding: data, as: UTF8.self))
exit(success ? 0 : 1)
