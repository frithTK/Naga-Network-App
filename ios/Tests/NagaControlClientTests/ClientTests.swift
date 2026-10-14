import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif
import Testing
@testable import NagaControlClient

private actor StubTransport: HTTPTransport {
    private var requests: [URLRequest] = []
    let body: String
    let status: Int
    init(_ body: String = "{}", status: Int = 200) { self.body = body; self.status = status }
    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        requests.append(request)
        return (Data(body.utf8), HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: [:])!)
    }
    func captured() -> [URLRequest] { requests }
}

struct ClientTests {
    @Test func healthUsesBearerAndTokenCanBeReplaced() async throws {
        let transport = StubTransport(#"{"service":"naga-control","status":"ok","version":"1.2.5","runtime_ready":false,"olcrtc_ready":false}"#)
        let client = try NagaControlClient(token: " first \n", transport: transport)
        let info = try await client.healthInfo()
        #expect(info.ok && !info.runtimeReady && info.version == "1.2.5")
        await client.updateToken("second")
        #expect(try await client.health())
        let requests = await transport.captured()
        #expect(requests.map { $0.value(forHTTPHeaderField: "Authorization") } == ["Bearer first", "Bearer second"])
        #expect(requests.allSatisfy { $0.url?.path == "/v1/health" && $0.timeoutInterval == 5 })
    }

    @Test func unauthenticatedClientDoesNotSendRequests() async throws {
        let transport = StubTransport()
        let client = try NagaControlClient(token: "  ", transport: transport)
        await #expect(throws: NagaAPIError.missingToken) { try await client.healthInfo() }
        #expect(await transport.captured().isEmpty)
    }

    @Test func clientRejectsRemoteEndpoints() {
        for endpoint in ["https://example.com", "http://192.168.0.1:8765", "http://user@127.0.0.1:8765", "http://127.0.0.1:8765/remote"] {
            #expect(throws: NagaAPIError.invalidEndpoint) { try NagaControlClient(baseURL: URL(string: endpoint)!) }
        }
    }

    @Test func subscriptionIsPostedToLoopbackNotFetchedBySwift() async throws {
        let transport = StubTransport(#"{"profile_id":"a","preview":{"profile_title":"Naga","engine":"sing-box","can_connect":true}}"#)
        let client = try NagaControlClient(token: "token", transport: transport)
        let preview = try await client.importProfile("nagavpn://install-config?url=https%3A%2F%2Fexample.com%2Fprofile.json")
        #expect(preview.profileId == "a" && preview.profileTitle == "Naga" && preview.canConnect)
        let request = try #require(await transport.captured().first)
        #expect(request.url?.host == "127.0.0.1" && request.url?.path == "/v1/profiles/import")
        #expect(request.httpMethod == "POST" && request.timeoutInterval == 35)
        let body = try JSONDecoder().decode([String: String].self, from: #require(request.httpBody))
        #expect(body == ["url": "https://example.com/profile.json"])
    }

    @Test func invalidLinksNeverReachTransport() async throws {
        let transport = StubTransport()
        let client = try NagaControlClient(token: "token", transport: transport)
        for link in ["http://example.com", "olcrtc://room", "karing://install-config?url=https://example.com", "nagavpn://wrong?url=https://example.com"] {
            await #expect(throws: NagaAPIError.invalidSubscription) { try await client.importProfile(link) }
        }
        #expect(await transport.captured().isEmpty)
    }

    @Test func filesArePostedAsConfigAndNamesAreTrimmed() async throws {
        let transport = StubTransport(#"{"profile_id":"b","preview":{"engine":"amneziawg"}}"#)
        let client = try NagaControlClient(token: "token", transport: transport)
        let text = "[Interface]\nPrivateKey = fixture\n"
        _ = try await client.importConfig(text, name: "  profile.conf  ")
        let request = try #require(await transport.captured().first)
        let body = try JSONDecoder().decode([String: String].self, from: #require(request.httpBody))
        #expect(body == ["config": text, "name": "profile.conf"])
    }

    @Test func serverErrorsPreserveRussianTextAndHaveSafeEnglishFallback() async throws {
        let transport = StubTransport(#"{"error":"Профиль не найден"}"#, status: 404)
        let client = try NagaControlClient(token: "token", transport: transport)
        await #expect(throws: NagaAPIError.server(status: 404, message: "Профиль не найден")) { try await client.shareProfile("missing") }
        #expect(NagaAPIError.server(status: 404, message: "Профиль не найден").errorDescription == "Профиль не найден")
        #expect(NagaAPIError.server(status: 500, message: "engine: secret upstream failed").errorDescription?.contains("secret") == false)
    }

    @Test func incompleteImportResponseDoesNotCreateSuccess() async throws {
        let transport = StubTransport(#"{"profile_id":"a"}"#)
        let client = try NagaControlClient(token: "token", transport: transport)
        await #expect(throws: NagaAPIError.invalidResponse) { try await client.importProfile("https://example.com") }
    }

    @Test func profileActionsUseExistingRoutesAndEscapeIDs() async throws {
        let transport = StubTransport()
        let client = try NagaControlClient(token: "token", transport: transport)
        _ = try await client.listNodes("a/b?#")
        _ = try await client.selectMode("abc", mode: "sing-box")
        try await client.deleteProfile("abc")
        let requests = await transport.captured()
        #expect(requests[0].url?.absoluteString.hasSuffix("/v1/profiles/a%2Fb%3F%23/nodes") == true)
        #expect(requests[1].url?.path == "/v1/profiles/abc/mode" && requests[1].httpMethod == "POST")
        #expect(requests[2].url?.path == "/v1/profiles/abc" && requests[2].httpMethod == "DELETE")
    }

    @Test func runtimeRequestsMatchDartContract() async throws {
        let transport = StubTransport()
        let client = try NagaControlClient(token: "token", transport: transport)
        _ = try await client.startRuntime("abc")
        _ = try await client.selectRuntime("abc", outbound: "node")
        _ = try await client.selectRuntimeAuto()
        _ = try await client.stopRuntime()
        let requests = await transport.captured()
        #expect(requests.map { $0.url!.path } == ["/v1/runtime/start", "/v1/runtime/select", "/v1/runtime/select", "/v1/runtime/stop"])
        #expect(requests.map(\.timeoutInterval) == [90, 20, 20, 20])
        let auto = try JSONDecoder().decode([String: Bool].self, from: #require(requests[2].httpBody))
        #expect(auto == ["auto": true])
    }

    @Test func policySaveUsesSnakeCaseAndPut() async throws {
        let transport = StubTransport(#"{"mode":"manual_strict","traffic_mode":"tun","revision":8}"#)
        let client = try NagaControlClient(token: "token", transport: transport)
        let result = try await client.saveConnectionPolicy(ConnectionPolicy(mode: "manual_strict", preferredCountry: "CZ"))
        #expect(result.mode == "manual_strict" && result.revision == 8)
        let request = try #require(await transport.captured().first)
        #expect(request.httpMethod == "PUT" && request.url?.path == "/v1/policy")
        let body = try JSONSerialization.jsonObject(with: #require(request.httpBody)) as! [String: Any]
        #expect(body["preferred_country"] as? String == "CZ" && body["traffic_mode"] as? String == "tun")
        #expect(body["preferredCountry"] == nil)
    }

    @Test func diagnosticsDecodeNestedFieldsAndClampLimit() async throws {
        let transport = StubTransport(#"{"entries":[{"timestamp":"2026-10-11T09:00:00.123Z","level":"info","component":"runtime","event":"start","message":"Готово","fields":{"ok":true,"duration_ms":12,"nested":{"nodes":["a",null]}}}]}"#)
        let client = try NagaControlClient(token: "token", transport: transport)
        let logs = try await client.diagnosticLogs(limit: 9999)
        #expect(logs.first?.fields["ok"] == .bool(true))
        #expect(logs.first?.fields["nested"] == .object(["nodes": .array([.string("a"), .null])]))
        #expect(await transport.captured().first?.url?.query == "limit=1000")
    }

    @Test func diagnosticsEventHasOnlyFieldsAcceptedByGo() async throws {
        let transport = StubTransport("", status: 204)
        let client = try NagaControlClient(token: "token", transport: transport)
        try await client.recordClientEvent(event: "import_failed", message: "Ошибка импорта")
        let request = try #require(await transport.captured().first)
        let body = try JSONDecoder().decode([String: String].self, from: #require(request.httpBody))
        #expect(body == ["event": "import_failed", "message": "Ошибка импорта", "level": "error"])
        #expect(request.url?.path == "/v1/diagnostics/client" && request.httpMethod == "POST")
    }
}
