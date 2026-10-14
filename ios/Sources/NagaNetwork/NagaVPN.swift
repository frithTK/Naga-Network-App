#if canImport(NetworkExtension)
import Foundation
import NetworkExtension

enum NagaVPNError: LocalizedError {
    case prepareFailed(String)
    case startFailed(String)
    case configurationMissing

    var errorDescription: String? {
        switch self {
        case .prepareFailed(let message), .startFailed(let message):
            return message
        case .configurationMissing:
            return "VPN configuration missing"
        }
    }
}

enum NagaVPN {
    static func loadManager() async throws -> NETunnelProviderManager {
        let managers = try await NETunnelProviderManager.loadAllFromPreferences()
        if let existing = managers.first(where: isNagaTunnel) {
            return existing
        }
        return NETunnelProviderManager()
    }

    static func prepare() async throws {
        let manager = try await loadManager()
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
        do {
            try await manager.saveToPreferences()
        } catch {
            throw NagaVPNError.prepareFailed(error.localizedDescription)
        }
    }

    static func start() async throws {
        let managers = try await NETunnelProviderManager.loadAllFromPreferences()
        guard let manager = managers.first(where: isNagaTunnel) else {
            throw NagaVPNError.configurationMissing
        }
        do {
            try manager.connection.startVPNTunnel()
        } catch {
            throw NagaVPNError.startFailed(error.localizedDescription)
        }
    }

    static func stop() async {
        let managers = (try? await NETunnelProviderManager.loadAllFromPreferences()) ?? []
        for manager in managers where isNagaTunnel(manager) {
            manager.connection.stopVPNTunnel()
        }
    }

    static func status() async -> NEVPNStatus {
        let managers = (try? await NETunnelProviderManager.loadAllFromPreferences()) ?? []
        return managers.first(where: isNagaTunnel)?.connection.status ?? .invalid
    }

    private static func isNagaTunnel(_ manager: NETunnelProviderManager) -> Bool {
        guard let proto = manager.protocolConfiguration as? NETunnelProviderProtocol else {
            return false
        }
        return proto.providerBundleIdentifier == NagaAppIdentity.tunnelBundleID
    }
}
#endif
