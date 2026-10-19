import Cocoa
import FlutterMacOS

class MainFlutterWindow: NSWindow {
  private var backendChannel: FlutterMethodChannel?
  override func awakeFromNib() {
    let flutterViewController = FlutterViewController()
    let windowFrame = self.frame
    self.contentViewController = flutterViewController
    self.setFrame(windowFrame, display: true)

    RegisterGeneratedPlugins(registry: flutterViewController)

    let channel = FlutterMethodChannel(
      name: "eu.nagavpn.naga_network/macos", binaryMessenger: flutterViewController.engine.binaryMessenger)
    backendChannel = channel
    MacOSBackendHost.shared.channel = channel
    MacOSQuickCheck.shared.cleanInterruptedCheck()
    channel.setMethodCallHandler { call, result in
      MacOSBackendHost.shared.handle(call, result: result)
    }

    super.awakeFromNib()
  }
}
