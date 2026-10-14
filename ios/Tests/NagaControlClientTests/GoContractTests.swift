import Foundation
import Testing
@testable import NagaControlClient

struct GoContractTests {
    private func fixture(_ name: String) throws -> Data {
        let url = try #require(Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures"))
        return try Data(contentsOf: url)
    }

    @Test func publicResponsesFromGoDecodeInSwift() throws {
        let decoder = JSONDecoder()
        let health = try decoder.decode(ControlHealth.self, from: fixture("health"))
        #expect(health.ok && health.version == "1.2.5" && !health.runtimeReady)
        let profiles = try decoder.decode([StoredProfileSummary].self, from: fixture("profiles"))
        let profile = try #require(profiles.first)
        #expect(profile.name == "Fixture profile" && !profile.hasSourceUrl && profile.canConnect)
        let nodes = try decoder.decode([UnifiedNode].self, from: fixture("nodes"))
        #expect(nodes.contains { $0.profileId == profile.id && $0.protocolName == "VLESS" })
        let tree = try decoder.decode(ProfileNodes.self, from: fixture("profile-nodes"))
        #expect(tree.profileId == profile.id && !tree.nodes.isEmpty)
        let snapshot = try decoder.decode(RuntimeSnapshot.self, from: fixture("runtime"))
        #expect(snapshot.status == "stopped" && !snapshot.trafficAvailable && !snapshot.blocksProfileDelete)
        let policy = try decoder.decode(ConnectionPolicy.self, from: fixture("policy"))
        #expect(policy.mode == "auto" && policy.trafficMode == "tun")
        let routing = try decoder.decode(RoutingPolicy.self, from: fixture("routing"))
        #expect(routing.mode == "all_vpn" && routing.apps.isEmpty)
        let preview = try decoder.decode(ProfilePreview.self, from: fixture("preview"))
        #expect(preview.engine == "sing-box" && preview.canConnect)
    }
}
