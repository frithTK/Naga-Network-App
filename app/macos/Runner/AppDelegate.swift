import Cocoa
import FlutterMacOS

@main
class AppDelegate: FlutterAppDelegate {
  override func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
    let decision = super.applicationShouldTerminate(sender)
    guard decision == .terminateNow else { return decision }
    guard MacOSBackendHost.shared.isRunning || MacOSQuickCheck.shared.isRunning else { return .terminateNow }
    MacOSQuickCheck.shared.prepareToQuit {
      MacOSBackendHost.shared.stop { sender.reply(toApplicationShouldTerminate: true) }
    }
    return .terminateLater
  }

  override func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
    return true
  }

  override func applicationSupportsSecureRestorableState(_ app: NSApplication) -> Bool {
    return true
  }
}
