#if canImport(SwiftUI)
import Foundation
import SwiftUI
import NagaControlClient

struct TrafficSample: Identifiable {
    let id: Date
    let upload: Int64
    let download: Int64
}

@MainActor
public final class NagaAppModel: ObservableObject {
    @Published var controlReachable: Bool?
    @Published var health: ControlHealth?
    @Published var profiles: [StoredProfileSummary] = []
    @Published var preview: ProfilePreview?
    @Published var nodes: [UnifiedNode] = []
    @Published var runtime: RuntimeSnapshot?
    @Published var policy: ConnectionPolicy?
    @Published var selectedProfileId: String?
    @Published var selectedNodeId: String?
    @Published var lastError: String?
    @Published var notice: String?
    @Published var busy = false
    @Published var samples: [TrafficSample] = []
    @Published var logs: [DiagnosticLogEntry] = []
    @Published var autoUpdate: Bool {
        didSet { preferences.set(autoUpdate, forKey: "naga.autoUpdateProfiles") }
    }

    let client: any NagaControling
    private let preferences: UserDefaults
    private var polling: Task<Void, Never>?
    private var refreshing = false
    private var runtimeMisses = 0
    private var nextProfileUpdate = Date.distantFuture

    public init(client: any NagaControling, preferences: UserDefaults = .standard) {
        self.client = client
        self.preferences = preferences
        autoUpdate = preferences.object(forKey: "naga.autoUpdateProfiles") as? Bool ?? true
        selectedProfileId = preferences.string(forKey: "naga.selectedProfile")
    }

    var selectedProfile: StoredProfileSummary? { profiles.first { $0.id == selectedProfileId } }
    var connected: Bool { runtime?.status == "connected" }
    var connectionBusy: Bool { ["starting", "stopping"].contains(runtime?.status ?? "") }
    var blockedProfile: Bool { selectedProfile.map { !$0.allowsConnection() } ?? false }
    var selectedNode: UnifiedNode? { nodes.first { $0.id == selectedNodeId } }
    var statusLabel: String {
        if controlReachable == false { return "Статус неизвестен" }
        if let profile = selectedProfile, profile.isExpired() { return "Подписка истекла" }
        if blockedProfile { return "Подключение недоступно" }
        switch runtime?.status {
        case "starting": return "Подключаемся…"
        case "stopping": return "Отключаемся…"
        case "connected": return "VPN подключён"
        case "failed": return "Не удалось подключиться"
        default: return "Готов к подключению"
        }
    }
    var mascot: String {
        if controlReachable == false { return "offline" }
        if profiles.isEmpty { return "welcome" }
        switch runtime?.status {
        case "connected": return "connected"
        case "starting", "stopping": return "connecting"
        case "failed": return "error"
        default: return "idle"
        }
    }

    func activate(dataDirectory: URL) {
        guard polling == nil else { return }
        polling = Task { [weak self] in
            guard let self else { return }
            await waitForToken(in: dataDirectory)
            guard !Task.isCancelled else { return }
            await reload()
            while !Task.isCancelled {
                do { try await Task.sleep(for: .seconds(2)) } catch { break }
                await poll()
            }
        }
    }

    func deactivate() {
        polling?.cancel()
        polling = nil
    }

    private func waitForToken(in directory: URL) async {
        let file = directory.appendingPathComponent("control.token")
        for _ in 0..<40 {
            guard !Task.isCancelled else { return }
            if let value = try? String(contentsOf: file, encoding: .utf8),
               !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                await client.updateToken(value)
                return
            }
            do { try await Task.sleep(for: .milliseconds(250)) } catch { return }
        }
    }

    func reload() async {
        guard !refreshing else { return }
        refreshing = true
        defer { refreshing = false }
        do {
            let info = try await client.healthInfo()
            health = info
            guard info.ok else { controlReachable = false; return }
            async let fetchedProfiles = client.listProfiles()
            async let fetchedNodes = client.listUnifiedNodes()
            async let fetchedPolicy = client.connectionPolicy()
            async let fetchedRuntime = client.runtimeStatus()
            let result = try await (fetchedProfiles, fetchedNodes, fetchedPolicy, fetchedRuntime)
            guard !Task.isCancelled else { return }
            profiles = result.0
            nodes = result.1.filter(\.availableOnIOS)
            policy = result.2
            apply(result.3)
            if selectedProfile == nil {
                selectProfile(profiles.first(where: { $0.id == runtime?.profileId }) ?? profiles.first)
            }
            if selectedNode == nil { selectedNodeId = nil }
            controlReachable = true
            runtimeMisses = 0
            scheduleProfileUpdate()
        } catch is CancellationError { }
        catch { controlReachable = false; show(error) }
    }

    private func poll() async {
        guard !refreshing, !busy else { return }
        refreshing = true
        defer { refreshing = false }
        do {
            if controlReachable != true {
                let info = try await client.healthInfo()
                guard info.ok else { return }
                health = info
                // Bootstrap workspace after the service becomes available.
                refreshing = false
                await reload()
                return
            }
            async let snapshot = client.runtimeStatus()
            async let fetchedNodes = client.listUnifiedNodes()
            let result = try await (snapshot, fetchedNodes)
            guard !Task.isCancelled else { return }
            apply(result.0)
            nodes = result.1.filter(\.availableOnIOS)
            runtimeMisses = 0
            controlReachable = true
            if autoUpdate && Date() >= nextProfileUpdate && !connected && !connectionBusy {
                scheduleProfileUpdate()
                for profile in profiles where profile.hasSourceUrl {
                    _ = try await client.refreshProfile(profile.id)
                }
                profiles = try await client.listProfiles()
                scheduleProfileUpdate()
            }
        } catch is CancellationError { }
        catch {
            runtimeMisses += 1
            if runtimeMisses >= 3 { controlReachable = false }
        }
    }

    private func apply(_ snapshot: RuntimeSnapshot) {
        runtime = snapshot
        if !connected { samples = [] }
        else if snapshot.trafficAvailable, let timestamp = NagaFormat.date(snapshot.lastTrafficUpdate),
                samples.last?.id != timestamp {
            samples.append(TrafficSample(id: timestamp, upload: snapshot.uploadRateBytes, download: snapshot.downloadRateBytes))
            if samples.count > 90 { samples.removeFirst(samples.count - 90) }
        }
    }

    private func scheduleProfileUpdate() {
        let intervals = profiles.filter(\.hasSourceUrl).map { $0.updateIntervalSeconds > 0 ? $0.updateIntervalSeconds : 86_400 }
        nextProfileUpdate = intervals.min().map { Date().addingTimeInterval(Double($0)) } ?? .distantFuture
    }

    func selectProfile(_ profile: StoredProfileSummary?) {
        guard !connected, !connectionBusy else { return }
        selectedProfileId = profile?.id
        selectedNodeId = nil
        preferences.set(selectedProfileId, forKey: "naga.selectedProfile")
    }

    func connect() {
        if controlReachable != true {
            lastError = "VPN-ядро ещё не собрано для этого устройства."
        } else if blockedProfile {
            lastError = "Обновите подписку, чтобы подключиться. Проверьте срок и лимит подписки."
        } else if selectedProfile == nil {
            lastError = "Сначала импортируй профиль."
        } else {
            // No NETunnelProviderManager or /runtime/start in the free-account shell.
            lastError = "VPN-ядро ещё не собрано для этого устройства."
        }
    }

    func importURL(_ url: String) async -> Bool {
        await perform {
            let saved = try await self.client.importProfile(url)
            self.preview = saved
            await self.reload()
            if let profile = self.profiles.first(where: { $0.id == saved.profileId }) { self.selectProfile(profile) }
            self.notice = "Профиль добавлен."
        }
    }

    func importConfig(_ config: String, name: String?) async -> Bool {
        await perform {
            let saved = try await self.client.importConfig(config, name: name)
            self.preview = saved
            await self.reload()
            if let profile = self.profiles.first(where: { $0.id == saved.profileId }) { self.selectProfile(profile) }
            self.notice = "Профиль добавлен."
        }
    }

    func previewURL(_ url: String) async -> Bool {
        preview = nil
        return await perform { self.preview = try await self.client.preview(url) }
    }

    func previewConfig(_ config: String, name: String?) async -> Bool {
        preview = nil
        return await perform { self.preview = try await self.client.previewConfig(config, name: name) }
    }

    func refreshProfile(_ profile: StoredProfileSummary) async {
        _ = await perform {
            _ = try await self.client.refreshProfile(profile.id)
            await self.reload()
            self.notice = "Профиль обновлён."
        }
    }

    func deleteProfile(_ profile: StoredProfileSummary) async {
        guard runtime?.blocksProfileDelete != true else {
            lastError = "Сначала отключите VPN, чтобы удалить профиль."
            return
        }
        _ = await perform {
            try await self.client.deleteProfile(profile.id)
            await self.reload()
            self.notice = "Профиль удалён."
        }
    }

    func chooseNode(_ node: UnifiedNode?) async {
        // The phase-2 shell only persists selection policy, never starts an engine.
        guard let profile = node.flatMap({ item in profiles.first { $0.id == item.profileId } }) ?? selectedProfile else { return }
        if let node, !node.availableOnIOS { lastError = "olcRTC недоступен на iPhone."; return }
        _ = await perform {
            var policy = self.policy ?? ConnectionPolicy()
            policy.mode = node == nil ? "auto" : "manual_fallback"
            policy.preferredCountry = node?.country ?? ""
            policy.preferredProtocol = node?.protocolName ?? ""
            policy.trafficMode = "tun"
            self.policy = try await self.client.saveConnectionPolicy(policy)
            self.selectProfile(profile)
            self.selectedNodeId = node?.id
            self.notice = node == nil ? "Включён автоматический выбор." : "Сервер выбран."
        }
    }

    func savePolicy(mode: String) async {
        _ = await perform {
            var value = self.policy ?? ConnectionPolicy()
            value.mode = mode
            value.trafficMode = "tun"
            self.policy = try await self.client.saveConnectionPolicy(value)
        }
    }

    func loadDiagnostics() async {
        do { logs = try await client.diagnosticLogs(limit: 200) }
        catch is CancellationError { }
        catch { show(error) }
    }

    private func perform(_ action: () async throws -> Void) async -> Bool {
        guard !busy else { return false }
        busy = true
        defer { busy = false }
        do { try await action(); return true }
        catch is CancellationError { return false }
        catch { show(error); return false }
    }

    func show(_ error: any Error) {
        lastError = (error as? NagaAPIError)?.errorDescription ?? "Не удалось выполнить действие. Повторите попытку."
    }
}
#endif
