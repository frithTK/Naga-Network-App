#if os(iOS)
import Foundation

enum NagaControl {
    static func dataDirectory() -> URL {
        if let group = FileManager.default.containerURL(
            forSecurityApplicationGroupIdentifier: NagaAppIdentity.appGroup
        ) {
            return group
        }
        return FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("naga-network", isDirectory: true)
    }

    static func start() {
        let dir = dataDirectory()
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        // Go control-plane (cmd/naga-ios) is linked after Darwin SDK + CGO.
        // Until then the UI can import profiles into the App Group, but the
        // PacketTunnel startTunnel path fails closed.
    }
}
#endif
