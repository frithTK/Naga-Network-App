import Flutter
import Foundation
import NetworkExtension

enum NagaVpnPlugin {
  static func register(messenger: FlutterBinaryMessenger) {
    let vpn = FlutterMethodChannel(
      name: "eu.nagavpn.naga_network/vpn",
      binaryMessenger: messenger
    )
    let update = FlutterMethodChannel(
      name: "eu.nagavpn.naga_network/update",
      binaryMessenger: messenger
    )
    vpn.setMethodCallHandler(handleVpn)
    update.setMethodCallHandler(handleUpdate)
  }

  private static func handleUpdate(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    switch call.method {
    case "dataDir":
      result(NagaControl.dataDirectory().path)
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  private static func handleVpn(_ call: FlutterMethodCall, result: @escaping FlutterResult) {
    switch call.method {
    case "prepare":
      prepare(result: result)
    case "start":
      startTunnel(result: result)
    case "stop":
      stopTunnel(result: result)
    case "listPackages":
      result([])
    default:
      result(FlutterMethodNotImplemented)
    }
  }

  private static func prepare(result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, error in
      if let error {
        result(
          FlutterError(code: "vpn_prepare", message: error.localizedDescription, details: nil)
        )
        return
      }
      let manager = managers?.first { existing in
        guard let proto = existing.protocolConfiguration as? NETunnelProviderProtocol else {
          return false
        }
        return proto.providerBundleIdentifier == NagaAppIdentity.tunnelBundleID
      } ?? NETunnelProviderManager()
      let proto = NETunnelProviderProtocol()
      proto.providerBundleIdentifier = NagaAppIdentity.tunnelBundleID
      proto.serverAddress = NagaAppIdentity.displayName
      proto.excludeLocalNetworks = true
      if #available(iOS 16.4, *) {
        proto.excludeAPNs = true
        proto.excludeCellularServices = true
      }
      manager.localizedDescription = NagaAppIdentity.displayName
      manager.protocolConfiguration = proto
      manager.isEnabled = true
      manager.saveToPreferences { saveError in
        if let saveError {
          result(
            FlutterError(
              code: "vpn_prepare",
              message: saveError.localizedDescription,
              details: nil
            )
          )
          return
        }
        result(true)
      }
    }
  }

  private static func startTunnel(result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, error in
      if let error {
        result(
          FlutterError(code: "vpn_start", message: error.localizedDescription, details: nil)
        )
        return
      }
      guard let manager = managers?.first(where: { existing in
        (existing.protocolConfiguration as? NETunnelProviderProtocol)?
          .providerBundleIdentifier == NagaAppIdentity.tunnelBundleID
      }) else {
        result(FlutterError(code: "vpn_start", message: "VPN configuration missing", details: nil))
        return
      }
      do {
        try manager.connection.startVPNTunnel()
        result(true)
      } catch {
        result(
          FlutterError(code: "vpn_start", message: error.localizedDescription, details: nil)
        )
      }
    }
  }

  private static func stopTunnel(result: @escaping FlutterResult) {
    NETunnelProviderManager.loadAllFromPreferences { managers, _ in
      managers?.forEach { manager in
        let proto = manager.protocolConfiguration as? NETunnelProviderProtocol
        if proto?.providerBundleIdentifier == NagaAppIdentity.tunnelBundleID {
          manager.connection.stopVPNTunnel()
        }
      }
      result(true)
    }
  }
}
