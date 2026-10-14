import Foundation
import Testing
@testable import NagaControlClient

struct ModelTests {
    @Test func optionalMetadataDoesNotBreakProfilesOrNodes() throws {
        let decoder = JSONDecoder()
        let profiles = try decoder.decode([StoredProfileSummary].self, from: Data(#"[{"id":"p","name":"Test","engine":"sing-box","can_connect":true}]"#.utf8))
        #expect(profiles.first?.hasSourceUrl == true && profiles.first?.updateIntervalSeconds == 86_400)
        let nodes = try decoder.decode([UnifiedNode].self, from: Data(#"[{"id":"n","profile_id":"p","tag":"CZ","runtime_tag":"p::CZ","protocol":"VLESS"}]"#.utf8))
        #expect(nodes.first?.latencyMs == nil && nodes.first?.protocolName == "VLESS")
    }

    @Test func runtimeIncludesTrafficAndFailoverMetadata() throws {
        let fixture = #"{"status":"connected","engine":"sing-box","profile_id":"p","upload_bytes":2147483648,"download_bytes":4294967296,"upload_rate_bytes":1024,"download_rate_bytes":2048,"session_started_at":"2026-10-11T09:00:00Z","last_traffic_update":"2026-10-11T09:00:01.125Z","traffic_available":true,"active_node":"CZ","active_country":"CZ","active_protocol":"VLESS","active_latency_ms":42,"active_node_status":"alive","failover_message":"Выбран запасной сервер"}"#
        let value = try JSONDecoder().decode(RuntimeSnapshot.self, from: Data(fixture.utf8))
        #expect(value.downloadBytes == 4_294_967_296 && value.uploadRateBytes == 1_024)
        #expect(value.trafficAvailable && value.activeLatencyMs == 42 && value.activeNodeStatus == "alive")
        #expect(value.failoverMessage == "Выбран запасной сервер")
        #expect(NagaFormat.date(value.lastTrafficUpdate) != nil)
    }

    @Test func profileDeletionIsBlockedDuringAnyActiveTransition() {
        for state in ["starting", "connected", "stopping"] { #expect(RuntimeSnapshot(status: state).blocksProfileDelete) }
        for state in ["stopped", "failed"] { #expect(!RuntimeSnapshot(status: state).blocksProfileDelete) }
    }

    @Test func subscriptionLimitsAndExpiryPreventConnection() throws {
        let now = try #require(NagaFormat.date("2026-10-11T09:00:00Z"))
        #expect(!StoredProfileSummary(canConnect: false).allowsConnection(at: now))
        #expect(!StoredProfileSummary(expireUtc: "2026-10-10T09:00:00Z", canConnect: true).allowsConnection(at: now))
        #expect(!StoredProfileSummary(uploadBytes: 30, downloadBytes: 70, totalBytes: 100, canConnect: true).allowsConnection(at: now))
        #expect(StoredProfileSummary(totalBytes: 100, unlimited: true, canConnect: true).allowsConnection(at: now))
    }

    @Test func olcRTCIsHiddenWithoutDiscardingOtherNodes() {
        let nodes = [UnifiedNode(protocolName: "VLESS"), UnifiedNode(protocolName: "olcRTC"), UnifiedNode(protocolName: "AmneziaWG")]
        #expect(nodes.filter(\.availableOnIOS).map(\.protocolName) == ["VLESS", "AmneziaWG"])
    }

    @Test func legacyRoutingFlagsUseTheSameFallbacksAsDart() throws {
        let policy = try JSONDecoder().decode(RoutingPolicy.self, from: Data(#"{"mode":"all_vpn","ru_direct":false}"#.utf8))
        #expect(!policy.builtinRu && !policy.providerRules && !policy.ruDirect)
        #expect(policy.builtinPrivate && policy.apps.isEmpty)
    }

    @Test func largeSubscriptionCountersDoNotOverflow() {
        let profile = StoredProfileSummary(uploadBytes: Int64.max, downloadBytes: 1,
                                           totalBytes: Int64.max, canConnect: true)
        #expect(profile.usedBytes == Int64.max && !profile.allowsConnection())
    }

    @Test func byteAndSessionFormattingMatchesFlutter() throws {
        #expect(NagaFormat.bytes(0) == "0 Б")
        #expect(NagaFormat.bytes(1_024) == "1.0 КБ")
        #expect(NagaFormat.bytes(1_073_741_824) == "1.00 ГБ")
        let now = try #require(NagaFormat.date("2026-10-11T09:01:30Z"))
        #expect(NagaFormat.session("2026-10-11T09:00:00Z", now: now) == "01:30")
        #expect(NagaFormat.session(nil, now: now) == "—")
    }
}
