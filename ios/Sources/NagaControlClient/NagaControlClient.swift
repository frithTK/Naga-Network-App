import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public enum NagaAPIError: Error, LocalizedError, Sendable, Equatable {
    case invalidEndpoint, missingToken, invalidSubscription, emptyShare, invalidResponse
    case server(status: Int, message: String)
    case unavailable

    public var errorDescription: String? {
        switch self {
        case .invalidEndpoint: return "Сервис Naga должен работать на локальном адресе."
        case .missingToken: return "Сервис Naga ещё не готов. Повторите попытку позже."
        case .invalidSubscription: return "Вставьте HTTPS-ссылку подписки."
        case .emptyShare: return "У профиля нет ссылки для копирования."
        case .invalidResponse: return "Сервис Naga вернул неполный ответ."
        case .unavailable: return "Нет связи с сервисом Naga. Повторите попытку позже."
        case .server(_, let message):
            // Preserve Russian core errors; do not show raw English engine output.
            return message.range(of: "[А-Яа-яЁё]", options: .regularExpression) != nil
                ? message : "Не удалось выполнить запрос. Проверьте профиль и повторите попытку."
        }
    }
}

public protocol HTTPTransport: Sendable {
    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse)
}

private final class LocalOnlyDelegate: NSObject, URLSessionTaskDelegate, Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask,
                    willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest,
                    completionHandler: @escaping (URLRequest?) -> Void) {
        // The local API never redirects. Never forward its Bearer to a remote URL.
        completionHandler(nil)
    }
}

public struct URLSessionTransport: HTTPTransport {
    private let session: URLSession

    public init() {
        let configuration = URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForResource = 210
        configuration.urlCache = nil
        configuration.httpCookieStorage = nil
        session = URLSession(configuration: configuration, delegate: LocalOnlyDelegate(), delegateQueue: nil)
    }

    public func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse else { throw NagaAPIError.invalidResponse }
        return (data, response)
    }
}

public protocol NagaControling: Sendable {
    func updateToken(_ token: String?) async
    func health() async throws -> Bool
    func healthInfo() async throws -> ControlHealth
    func preview(_ url: String) async throws -> ProfilePreview
    func previewConfig(_ config: String, name: String?) async throws -> ProfilePreview
    func importProfile(_ url: String) async throws -> ProfilePreview
    func importConfig(_ config: String, name: String?) async throws -> ProfilePreview
    func listProfiles() async throws -> [StoredProfileSummary]
    func shareProfile(_ id: String) async throws -> ProfileShare
    func listNodes(_ id: String) async throws -> ProfileNodes
    func selectMode(_ id: String, mode: String) async throws -> ProfileNodes
    func deleteProfile(_ id: String) async throws
    func refreshProfile(_ id: String) async throws -> ProfilePreview
    func listUnifiedNodes() async throws -> [UnifiedNode]
    func probeUnifiedNodes() async throws -> [UnifiedNode]
    func runtimeStatus() async throws -> RuntimeSnapshot
    func startRuntime(_ profileId: String) async throws -> RuntimeSnapshot
    func selectRuntime(_ profileId: String, outbound: String) async throws -> RuntimeSnapshot
    func selectRuntimeAuto() async throws -> RuntimeSnapshot
    func stopRuntime() async throws -> RuntimeSnapshot
    func connectionPolicy() async throws -> ConnectionPolicy
    func saveConnectionPolicy(_ policy: ConnectionPolicy) async throws -> ConnectionPolicy
    func routingPolicy() async throws -> RoutingPolicy
    func saveRoutingPolicy(_ policy: RoutingPolicy) async throws -> RoutingPolicy
    func diagnosticLogs(limit: Int) async throws -> [DiagnosticLogEntry]
    func recordClientEvent(event: String, message: String) async throws
}

/// This client talks only to the local control-plane. Subscription fetch stays in Go.
public actor NagaControlClient: NagaControling {
    private let baseURL: URL
    private var token: String?
    private let transport: any HTTPTransport

    public init(baseURL: URL = URL(string: "http://127.0.0.1:8765")!,
                token: String? = nil, transport: any HTTPTransport = URLSessionTransport()) throws {
        guard baseURL.scheme == "http", ["127.0.0.1", "::1", "[::1]"].contains(baseURL.host ?? ""),
              baseURL.user == nil, baseURL.password == nil, baseURL.query == nil, baseURL.fragment == nil,
              baseURL.path.isEmpty || baseURL.path == "/" else { throw NagaAPIError.invalidEndpoint }
        self.baseURL = baseURL
        self.token = Self.trimmedToken(token)
        self.transport = transport
    }

    public func updateToken(_ token: String?) { self.token = Self.trimmedToken(token) }

    private static func trimmedToken(_ token: String?) -> String? {
        guard let token = token?.trimmingCharacters(in: .whitespacesAndNewlines), !token.isEmpty else { return nil }
        return token
    }

    private func request(_ path: String, method: String = "GET", body: Data? = nil,
                         timeout: TimeInterval = 10) async throws -> Data {
        guard let token else { throw NagaAPIError.missingToken }
        let root = baseURL.absoluteString.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        guard let url = URL(string: root + path) else { throw NagaAPIError.invalidEndpoint }
        var request = URLRequest(url: url, timeoutInterval: timeout)
        request.httpMethod = method
        request.httpBody = body
        request.cachePolicy = .reloadIgnoringLocalCacheData
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if body != nil { request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        let data: Data
        let response: HTTPURLResponse
        do { (data, response) = try await transport.send(request) }
        catch is CancellationError { throw CancellationError() }
        catch let error as URLError where error.code == .cancelled { throw CancellationError() }
        catch { throw NagaAPIError.unavailable }
        guard (200..<300).contains(response.statusCode) else {
            let message = (try? JSONDecoder().decode(APIErrorBody.self, from: data))?.error ?? ""
            throw NagaAPIError.server(status: response.statusCode, message: message)
        }
        return data
    }

    private func decode<T: Decodable & Sendable>(_ type: T.Type, _ path: String,
        method: String = "GET", body: Data? = nil, timeout: TimeInterval = 10) async throws -> T {
        let data = try await request(path, method: method, body: body, timeout: timeout)
        do { return try JSONDecoder().decode(type, from: data) }
        catch { throw NagaAPIError.invalidResponse }
    }

    private func post<T: Decodable & Sendable, B: Encodable>(_ type: T.Type, _ path: String,
        _ body: B, method: String = "POST", timeout: TimeInterval = 35) async throws -> T {
        try await decode(type, path, method: method, body: JSONEncoder().encode(body), timeout: timeout)
    }

    private func profilePath(_ id: String, _ action: String = "") throws -> String {
        var allowed = CharacterSet.alphanumerics
        allowed.insert(charactersIn: "-_.")
        guard !id.isEmpty, id != ".", id != "..",
              let id = id.addingPercentEncoding(withAllowedCharacters: allowed) else { throw NagaAPIError.invalidResponse }
        return "/v1/profiles/\(id)" + (action.isEmpty ? "" : "/\(action)")
    }

    public func health() async throws -> Bool { try await healthInfo().ok }
    public func healthInfo() async throws -> ControlHealth {
        try await decode(ControlHealth.self, "/v1/health", timeout: 5)
    }
    public func preview(_ url: String) async throws -> ProfilePreview {
        guard let url = SubscriptionLink.resolve(url) else { throw NagaAPIError.invalidSubscription }
        return try await post(ProfilePreview.self, "/v1/subscriptions/preview", ["url": url])
    }
    public func previewConfig(_ config: String, name: String? = nil) async throws -> ProfilePreview {
        try await post(ProfilePreview.self, "/v1/subscriptions/preview", configBody(config, name))
    }
    public func importProfile(_ url: String) async throws -> ProfilePreview {
        guard let url = SubscriptionLink.resolve(url) else { throw NagaAPIError.invalidSubscription }
        let result = try await post(ImportResponse.self, "/v1/profiles/import", ["url": url])
        return try result.savedPreview()
    }
    public func importConfig(_ config: String, name: String? = nil) async throws -> ProfilePreview {
        let result = try await post(ImportResponse.self, "/v1/profiles/import", configBody(config, name))
        return try result.savedPreview()
    }
    private func configBody(_ config: String, _ name: String?) -> [String: String] {
        var body = ["config": config]
        if let name = name?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty { body["name"] = name }
        return body
    }
    public func listProfiles() async throws -> [StoredProfileSummary] {
        try await decode([StoredProfileSummary].self, "/v1/profiles", timeout: 5)
    }
    public func shareProfile(_ id: String) async throws -> ProfileShare {
        let share = try await decode(ProfileShare.self, profilePath(id, "share"))
        guard !share.url.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { throw NagaAPIError.emptyShare }
        return share
    }
    public func listNodes(_ id: String) async throws -> ProfileNodes {
        try await decode(ProfileNodes.self, profilePath(id, "nodes"))
    }
    public func selectMode(_ id: String, mode: String) async throws -> ProfileNodes {
        try await post(ProfileNodes.self, profilePath(id, "mode"), ["mode": mode])
    }
    public func deleteProfile(_ id: String) async throws {
        _ = try await request(profilePath(id), method: "DELETE")
    }
    public func refreshProfile(_ id: String) async throws -> ProfilePreview {
        let result = try await post(ImportResponse.self, profilePath(id, "refresh"), [String: String]())
        return try result.savedPreview()
    }
    public func listUnifiedNodes() async throws -> [UnifiedNode] {
        try await decode([UnifiedNode].self, "/v1/nodes", timeout: 5)
    }
    public func probeUnifiedNodes() async throws -> [UnifiedNode] {
        try await post([UnifiedNode].self, "/v1/nodes/probe", [String: String](), timeout: 200)
    }
    public func runtimeStatus() async throws -> RuntimeSnapshot {
        try await decode(RuntimeSnapshot.self, "/v1/runtime", timeout: 5)
    }
    // Present for API parity. SwiftUI Connect must use PacketTunnel in phase 4.
    public func startRuntime(_ profileId: String) async throws -> RuntimeSnapshot {
        try await post(RuntimeSnapshot.self, "/v1/runtime/start", ["profile_id": profileId], timeout: 90)
    }
    public func selectRuntime(_ profileId: String, outbound: String) async throws -> RuntimeSnapshot {
        try await post(RuntimeSnapshot.self, "/v1/runtime/select", ["profile_id": profileId, "outbound": outbound], timeout: 20)
    }
    public func selectRuntimeAuto() async throws -> RuntimeSnapshot {
        try await post(RuntimeSnapshot.self, "/v1/runtime/select", ["auto": true], timeout: 20)
    }
    public func stopRuntime() async throws -> RuntimeSnapshot {
        try await post(RuntimeSnapshot.self, "/v1/runtime/stop", [String: String](), timeout: 20)
    }
    public func connectionPolicy() async throws -> ConnectionPolicy {
        try await decode(ConnectionPolicy.self, "/v1/policy")
    }
    public func saveConnectionPolicy(_ policy: ConnectionPolicy) async throws -> ConnectionPolicy {
        try await post(ConnectionPolicy.self, "/v1/policy", policy, method: "PUT", timeout: 10)
    }
    public func routingPolicy() async throws -> RoutingPolicy {
        try await decode(RoutingPolicy.self, "/v1/routing")
    }
    public func saveRoutingPolicy(_ policy: RoutingPolicy) async throws -> RoutingPolicy {
        try await post(RoutingPolicy.self, "/v1/routing", policy, method: "PUT", timeout: 10)
    }
    public func diagnosticLogs(limit: Int = 200) async throws -> [DiagnosticLogEntry] {
        let result = try await decode(LogResponse.self, "/v1/diagnostics/logs?limit=\(min(max(limit, 1), 1000))")
        return result.entries
    }
    public func recordClientEvent(event: String, message: String) async throws {
        _ = try await request("/v1/diagnostics/client", method: "POST",
            body: JSONEncoder().encode(["event": event, "message": message, "level": "error"]), timeout: 5)
    }
}

private struct APIErrorBody: Decodable { let error: String }
private struct LogResponse: Decodable, Sendable { let entries: [DiagnosticLogEntry] }
private struct ImportResponse: Decodable, Sendable {
    let profile_id: String
    let preview: ProfilePreview
    func savedPreview() throws -> ProfilePreview {
        guard !profile_id.isEmpty else { throw NagaAPIError.invalidResponse }
        var preview = preview
        preview.profileId = profile_id
        return preview
    }
}
