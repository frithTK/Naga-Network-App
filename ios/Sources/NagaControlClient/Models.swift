import Foundation

// Wire keys intentionally match core/control/server.go and the Dart client.

public struct ProfilePreview: Codable, Sendable, Equatable {
    public var profileId: String?
    public var profileTitle: String
    public var providerName: String
    public var engine: String
    public var uploadBytes: Int64
    public var downloadBytes: Int64
    public var totalBytes: Int64
    public var unlimited: Bool
    public var expireUtc: String?
    public var updateIntervalSeconds: Int
    public var canConnect: Bool
    public var olcrtcAvailable: Bool

    public init(
        profileId: String? = nil,
        profileTitle: String = "Naga Network",
        providerName: String = "",
        engine: String = "unknown",
        uploadBytes: Int64 = 0,
        downloadBytes: Int64 = 0,
        totalBytes: Int64 = 0,
        unlimited: Bool = false,
        expireUtc: String? = nil,
        updateIntervalSeconds: Int = 86400,
        canConnect: Bool = false,
        olcrtcAvailable: Bool = false
    ) {
        self.profileId = profileId
        self.profileTitle = profileTitle
        self.providerName = providerName
        self.engine = engine
        self.uploadBytes = uploadBytes
        self.downloadBytes = downloadBytes
        self.totalBytes = totalBytes
        self.unlimited = unlimited
        self.expireUtc = expireUtc
        self.updateIntervalSeconds = updateIntervalSeconds
        self.canConnect = canConnect
        self.olcrtcAvailable = olcrtcAvailable
    }

    enum CodingKeys: String, CodingKey {
        case profileId = "profile_id"
        case profileTitle = "profile_title"
        case providerName = "provider_name"
        case engine
        case uploadBytes = "upload_bytes"
        case downloadBytes = "download_bytes"
        case totalBytes = "total_bytes"
        case unlimited
        case expireUtc = "expire_utc"
        case updateIntervalSeconds = "update_interval_seconds"
        case canConnect = "can_connect"
        case olcrtcAvailable = "olcrtc_available"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        profileId = try c.decodeIfPresent(String.self, forKey: .profileId)
        profileTitle = try c.decodeIfPresent(String.self, forKey: .profileTitle) ?? "Naga Network"
        providerName = try c.decodeIfPresent(String.self, forKey: .providerName) ?? ""
        engine = try c.decodeIfPresent(String.self, forKey: .engine) ?? "unknown"
        uploadBytes = try c.decodeIfPresent(Int64.self, forKey: .uploadBytes) ?? 0
        downloadBytes = try c.decodeIfPresent(Int64.self, forKey: .downloadBytes) ?? 0
        totalBytes = try c.decodeIfPresent(Int64.self, forKey: .totalBytes) ?? 0
        unlimited = try c.decodeIfPresent(Bool.self, forKey: .unlimited) ?? false
        expireUtc = try c.decodeIfPresent(String.self, forKey: .expireUtc)
        updateIntervalSeconds = try c.decodeIfPresent(Int.self, forKey: .updateIntervalSeconds) ?? 86400
        canConnect = try c.decodeIfPresent(Bool.self, forKey: .canConnect) ?? false
        olcrtcAvailable = try c.decodeIfPresent(Bool.self, forKey: .olcrtcAvailable) ?? false
    }
}

public struct StoredProfileSummary: Codable, Sendable, Equatable, Identifiable {
    public var id: String
    public var name: String
    public var providerName: String
    public var engine: String
    public var importedAt: String?
    public var updatedAt: String?
    public var uploadBytes: Int64
    public var downloadBytes: Int64
    public var totalBytes: Int64
    public var unlimited: Bool
    public var updateIntervalSeconds: Int
    public var expireUtc: String?
    public var canConnect: Bool
    public var hasSourceUrl: Bool
    public var olcrtcAvailable: Bool

    public init(
        id: String = "",
        name: String = "Naga Network",
        providerName: String = "",
        engine: String = "unknown",
        importedAt: String? = nil,
        updatedAt: String? = nil,
        uploadBytes: Int64 = 0,
        downloadBytes: Int64 = 0,
        totalBytes: Int64 = 0,
        unlimited: Bool = false,
        updateIntervalSeconds: Int = 86400,
        expireUtc: String? = nil,
        canConnect: Bool = false,
        hasSourceUrl: Bool = true,
        olcrtcAvailable: Bool = false
    ) {
        self.id = id
        self.name = name
        self.providerName = providerName
        self.engine = engine
        self.importedAt = importedAt
        self.updatedAt = updatedAt
        self.uploadBytes = uploadBytes
        self.downloadBytes = downloadBytes
        self.totalBytes = totalBytes
        self.unlimited = unlimited
        self.updateIntervalSeconds = updateIntervalSeconds
        self.expireUtc = expireUtc
        self.canConnect = canConnect
        self.hasSourceUrl = hasSourceUrl
        self.olcrtcAvailable = olcrtcAvailable
    }

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case providerName = "provider_name"
        case engine
        case importedAt = "imported_at"
        case updatedAt = "updated_at"
        case uploadBytes = "upload_bytes"
        case downloadBytes = "download_bytes"
        case totalBytes = "total_bytes"
        case unlimited
        case updateIntervalSeconds = "update_interval_seconds"
        case expireUtc = "expire_utc"
        case canConnect = "can_connect"
        case hasSourceUrl = "has_source_url"
        case olcrtcAvailable = "olcrtc_available"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? "Naga Network"
        providerName = try c.decodeIfPresent(String.self, forKey: .providerName) ?? ""
        engine = try c.decodeIfPresent(String.self, forKey: .engine) ?? "unknown"
        importedAt = try c.decodeIfPresent(String.self, forKey: .importedAt)
        updatedAt = try c.decodeIfPresent(String.self, forKey: .updatedAt)
        uploadBytes = try c.decodeIfPresent(Int64.self, forKey: .uploadBytes) ?? 0
        downloadBytes = try c.decodeIfPresent(Int64.self, forKey: .downloadBytes) ?? 0
        totalBytes = try c.decodeIfPresent(Int64.self, forKey: .totalBytes) ?? 0
        unlimited = try c.decodeIfPresent(Bool.self, forKey: .unlimited) ?? false
        updateIntervalSeconds = try c.decodeIfPresent(Int.self, forKey: .updateIntervalSeconds) ?? 86400
        expireUtc = try c.decodeIfPresent(String.self, forKey: .expireUtc)
        canConnect = try c.decodeIfPresent(Bool.self, forKey: .canConnect) ?? false
        hasSourceUrl = try c.decodeIfPresent(Bool.self, forKey: .hasSourceUrl) ?? true
        olcrtcAvailable = try c.decodeIfPresent(Bool.self, forKey: .olcrtcAvailable) ?? false
    }
}

public struct ProfileShare: Codable, Sendable, Equatable {
    public var url: String
    public var name: String

    public init(
        url: String = "",
        name: String = ""
    ) {
        self.url = url
        self.name = name
    }

    enum CodingKeys: String, CodingKey {
        case url
        case name
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        url = try c.decodeIfPresent(String.self, forKey: .url) ?? ""
        name = try c.decodeIfPresent(String.self, forKey: .name) ?? ""
    }
}

public struct ProfileNode: Codable, Sendable, Equatable {
    public var tag: String
    public var type: String
    public var country: String
    public var protocolName: String
    public var selected: Bool
    public var children: [ProfileNode]

    public init(
        tag: String = "",
        type: String = "unknown",
        country: String = "",
        protocolName: String = "",
        selected: Bool = false,
        children: [ProfileNode] = []
    ) {
        self.tag = tag
        self.type = type
        self.country = country
        self.protocolName = protocolName
        self.selected = selected
        self.children = children
    }

    enum CodingKeys: String, CodingKey {
        case tag
        case type
        case country
        case protocolName = "protocol"
        case selected
        case children
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        tag = try c.decodeIfPresent(String.self, forKey: .tag) ?? ""
        type = try c.decodeIfPresent(String.self, forKey: .type) ?? "unknown"
        country = try c.decodeIfPresent(String.self, forKey: .country) ?? ""
        protocolName = try c.decodeIfPresent(String.self, forKey: .protocolName) ?? ""
        selected = try c.decodeIfPresent(Bool.self, forKey: .selected) ?? false
        children = try c.decodeIfPresent([ProfileNode].self, forKey: .children) ?? []
    }
}

public struct ProfileNodes: Codable, Sendable, Equatable {
    public var profileId: String
    public var mode: String
    public var nodes: [ProfileNode]

    public init(
        profileId: String = "",
        mode: String = "",
        nodes: [ProfileNode] = []
    ) {
        self.profileId = profileId
        self.mode = mode
        self.nodes = nodes
    }

    enum CodingKeys: String, CodingKey {
        case profileId = "profile_id"
        case mode
        case nodes
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        profileId = try c.decodeIfPresent(String.self, forKey: .profileId) ?? ""
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? ""
        nodes = try c.decodeIfPresent([ProfileNode].self, forKey: .nodes) ?? []
    }
}

public struct UnifiedNode: Codable, Sendable, Equatable, Identifiable {
    public var id: String
    public var profileId: String
    public var tag: String
    public var runtimeTag: String
    public var country: String
    public var protocolName: String
    public var latencyMs: Int?
    public var probeStatus: String

    public init(
        id: String = "",
        profileId: String = "",
        tag: String = "",
        runtimeTag: String = "",
        country: String = "",
        protocolName: String = "",
        latencyMs: Int? = nil,
        probeStatus: String = "unknown"
    ) {
        self.id = id
        self.profileId = profileId
        self.tag = tag
        self.runtimeTag = runtimeTag
        self.country = country
        self.protocolName = protocolName
        self.latencyMs = latencyMs
        self.probeStatus = probeStatus
    }

    enum CodingKeys: String, CodingKey {
        case id
        case profileId = "profile_id"
        case tag
        case runtimeTag = "runtime_tag"
        case country
        case protocolName = "protocol"
        case latencyMs = "latency_ms"
        case probeStatus = "probe_status"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        profileId = try c.decodeIfPresent(String.self, forKey: .profileId) ?? ""
        tag = try c.decodeIfPresent(String.self, forKey: .tag) ?? ""
        runtimeTag = try c.decodeIfPresent(String.self, forKey: .runtimeTag) ?? ""
        country = try c.decodeIfPresent(String.self, forKey: .country) ?? ""
        protocolName = try c.decodeIfPresent(String.self, forKey: .protocolName) ?? ""
        latencyMs = try c.decodeIfPresent(Int.self, forKey: .latencyMs)
        probeStatus = try c.decodeIfPresent(String.self, forKey: .probeStatus) ?? "unknown"
    }
}

public struct RuntimeSnapshot: Codable, Sendable, Equatable {
    public var status: String
    public var engine: String
    public var profileId: String?
    public var error: String?
    public var uploadBytes: Int64
    public var downloadBytes: Int64
    public var uploadRateBytes: Int64
    public var downloadRateBytes: Int64
    public var sessionStartedAt: String?
    public var lastTrafficUpdate: String?
    public var trafficAvailable: Bool
    public var activeNode: String?
    public var activeCountry: String?
    public var activeProtocol: String?
    public var activeLatencyMs: Int?
    public var activeNodeStatus: String?
    public var failoverMessage: String?

    public init(
        status: String = "stopped",
        engine: String = "sing-box",
        profileId: String? = nil,
        error: String? = nil,
        uploadBytes: Int64 = 0,
        downloadBytes: Int64 = 0,
        uploadRateBytes: Int64 = 0,
        downloadRateBytes: Int64 = 0,
        sessionStartedAt: String? = nil,
        lastTrafficUpdate: String? = nil,
        trafficAvailable: Bool = false,
        activeNode: String? = nil,
        activeCountry: String? = nil,
        activeProtocol: String? = nil,
        activeLatencyMs: Int? = nil,
        activeNodeStatus: String? = nil,
        failoverMessage: String? = nil
    ) {
        self.status = status
        self.engine = engine
        self.profileId = profileId
        self.error = error
        self.uploadBytes = uploadBytes
        self.downloadBytes = downloadBytes
        self.uploadRateBytes = uploadRateBytes
        self.downloadRateBytes = downloadRateBytes
        self.sessionStartedAt = sessionStartedAt
        self.lastTrafficUpdate = lastTrafficUpdate
        self.trafficAvailable = trafficAvailable
        self.activeNode = activeNode
        self.activeCountry = activeCountry
        self.activeProtocol = activeProtocol
        self.activeLatencyMs = activeLatencyMs
        self.activeNodeStatus = activeNodeStatus
        self.failoverMessage = failoverMessage
    }

    enum CodingKeys: String, CodingKey {
        case status
        case engine
        case profileId = "profile_id"
        case error
        case uploadBytes = "upload_bytes"
        case downloadBytes = "download_bytes"
        case uploadRateBytes = "upload_rate_bytes"
        case downloadRateBytes = "download_rate_bytes"
        case sessionStartedAt = "session_started_at"
        case lastTrafficUpdate = "last_traffic_update"
        case trafficAvailable = "traffic_available"
        case activeNode = "active_node"
        case activeCountry = "active_country"
        case activeProtocol = "active_protocol"
        case activeLatencyMs = "active_latency_ms"
        case activeNodeStatus = "active_node_status"
        case failoverMessage = "failover_message"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        status = try c.decodeIfPresent(String.self, forKey: .status) ?? "stopped"
        engine = try c.decodeIfPresent(String.self, forKey: .engine) ?? "sing-box"
        profileId = try c.decodeIfPresent(String.self, forKey: .profileId)
        error = try c.decodeIfPresent(String.self, forKey: .error)
        uploadBytes = try c.decodeIfPresent(Int64.self, forKey: .uploadBytes) ?? 0
        downloadBytes = try c.decodeIfPresent(Int64.self, forKey: .downloadBytes) ?? 0
        uploadRateBytes = try c.decodeIfPresent(Int64.self, forKey: .uploadRateBytes) ?? 0
        downloadRateBytes = try c.decodeIfPresent(Int64.self, forKey: .downloadRateBytes) ?? 0
        sessionStartedAt = try c.decodeIfPresent(String.self, forKey: .sessionStartedAt)
        lastTrafficUpdate = try c.decodeIfPresent(String.self, forKey: .lastTrafficUpdate)
        trafficAvailable = try c.decodeIfPresent(Bool.self, forKey: .trafficAvailable) ?? false
        activeNode = try c.decodeIfPresent(String.self, forKey: .activeNode)
        activeCountry = try c.decodeIfPresent(String.self, forKey: .activeCountry)
        activeProtocol = try c.decodeIfPresent(String.self, forKey: .activeProtocol)
        activeLatencyMs = try c.decodeIfPresent(Int.self, forKey: .activeLatencyMs)
        activeNodeStatus = try c.decodeIfPresent(String.self, forKey: .activeNodeStatus)
        failoverMessage = try c.decodeIfPresent(String.self, forKey: .failoverMessage)
    }
}

public struct ConnectionPolicy: Codable, Sendable, Equatable {
    public var schemaVersion: Int
    public var revision: Int64
    public var mode: String
    public var trafficMode: String
    public var preferredCountry: String
    public var preferredProtocol: String
    public var networkClass: String
    public var tuicFallbackEnabled: Bool

    public init(
        schemaVersion: Int = 1,
        revision: Int64 = 0,
        mode: String = "auto",
        trafficMode: String = "tun",
        preferredCountry: String = "",
        preferredProtocol: String = "",
        networkClass: String = "unknown",
        tuicFallbackEnabled: Bool = true
    ) {
        self.schemaVersion = schemaVersion
        self.revision = revision
        self.mode = mode
        self.trafficMode = trafficMode
        self.preferredCountry = preferredCountry
        self.preferredProtocol = preferredProtocol
        self.networkClass = networkClass
        self.tuicFallbackEnabled = tuicFallbackEnabled
    }

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case revision
        case mode
        case trafficMode = "traffic_mode"
        case preferredCountry = "preferred_country"
        case preferredProtocol = "preferred_protocol"
        case networkClass = "network_class"
        case tuicFallbackEnabled = "tuic_fallback_enabled"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        schemaVersion = try c.decodeIfPresent(Int.self, forKey: .schemaVersion) ?? 1
        revision = try c.decodeIfPresent(Int64.self, forKey: .revision) ?? 0
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? "auto"
        trafficMode = try c.decodeIfPresent(String.self, forKey: .trafficMode) ?? "tun"
        preferredCountry = try c.decodeIfPresent(String.self, forKey: .preferredCountry) ?? ""
        preferredProtocol = try c.decodeIfPresent(String.self, forKey: .preferredProtocol) ?? ""
        networkClass = try c.decodeIfPresent(String.self, forKey: .networkClass) ?? "unknown"
        tuicFallbackEnabled = try c.decodeIfPresent(Bool.self, forKey: .tuicFallbackEnabled) ?? true
    }
}

public struct AppRoute: Codable, Sendable, Equatable, Identifiable {
    public var id: String
    public var displayName: String
    public var platform: String
    public var packageOrProcessId: String
    public var route: String
    public var enabled: Bool

    public init(
        id: String = "",
        displayName: String = "",
        platform: String = "ios",
        packageOrProcessId: String = "",
        route: String = "vpn",
        enabled: Bool = true
    ) {
        self.id = id
        self.displayName = displayName
        self.platform = platform
        self.packageOrProcessId = packageOrProcessId
        self.route = route
        self.enabled = enabled
    }

    enum CodingKeys: String, CodingKey {
        case id
        case displayName = "display_name"
        case platform
        case packageOrProcessId = "package_name_or_process_id"
        case route
        case enabled
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decodeIfPresent(String.self, forKey: .id) ?? ""
        displayName = try c.decodeIfPresent(String.self, forKey: .displayName) ?? ""
        platform = try c.decodeIfPresent(String.self, forKey: .platform) ?? "ios"
        packageOrProcessId = try c.decodeIfPresent(String.self, forKey: .packageOrProcessId) ?? ""
        route = try c.decodeIfPresent(String.self, forKey: .route) ?? "vpn"
        enabled = try c.decodeIfPresent(Bool.self, forKey: .enabled) ?? true
    }
}

public struct RoutingPolicy: Codable, Sendable, Equatable {
    public var schemaVersion: Int
    public var revision: Int64
    public var mode: String
    public var ruDirect: Bool
    public var providerRules: Bool
    public var builtinPrivate: Bool
    public var builtinRu: Bool
    public var apps: [AppRoute]

    public init(
        schemaVersion: Int = 2,
        revision: Int64 = 0,
        mode: String = "all_vpn",
        ruDirect: Bool = true,
        providerRules: Bool = true,
        builtinPrivate: Bool = true,
        builtinRu: Bool = true,
        apps: [AppRoute] = []
    ) {
        self.schemaVersion = schemaVersion
        self.revision = revision
        self.mode = mode
        self.ruDirect = ruDirect
        self.providerRules = providerRules
        self.builtinPrivate = builtinPrivate
        self.builtinRu = builtinRu
        self.apps = apps
    }

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case revision
        case mode
        case ruDirect = "ru_direct"
        case providerRules = "provider_rules"
        case builtinPrivate = "builtin_private"
        case builtinRu = "builtin_ru"
        case apps
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        schemaVersion = try c.decodeIfPresent(Int.self, forKey: .schemaVersion) ?? 2
        revision = try c.decodeIfPresent(Int64.self, forKey: .revision) ?? 0
        mode = try c.decodeIfPresent(String.self, forKey: .mode) ?? "all_vpn"
        ruDirect = try c.decodeIfPresent(Bool.self, forKey: .ruDirect) ?? true
        providerRules = try c.decodeIfPresent(Bool.self, forKey: .providerRules) ?? ruDirect
        builtinPrivate = try c.decodeIfPresent(Bool.self, forKey: .builtinPrivate) ?? true
        builtinRu = try c.decodeIfPresent(Bool.self, forKey: .builtinRu) ?? ruDirect
        ruDirect = builtinRu
        apps = try c.decodeIfPresent([AppRoute].self, forKey: .apps) ?? []
    }
}

public struct DiagnosticLogEntry: Codable, Sendable, Equatable {
    public var timestamp: String?
    public var level: String
    public var component: String
    public var event: String
    public var message: String
    public var fields: [String: JSONValue]

    public init(
        timestamp: String? = nil,
        level: String = "info",
        component: String = "app",
        event: String = "event",
        message: String = "",
        fields: [String: JSONValue] = [:]
    ) {
        self.timestamp = timestamp
        self.level = level
        self.component = component
        self.event = event
        self.message = message
        self.fields = fields
    }

    enum CodingKeys: String, CodingKey {
        case timestamp
        case level
        case component
        case event
        case message
        case fields
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        timestamp = try c.decodeIfPresent(String.self, forKey: .timestamp)
        level = try c.decodeIfPresent(String.self, forKey: .level) ?? "info"
        component = try c.decodeIfPresent(String.self, forKey: .component) ?? "app"
        event = try c.decodeIfPresent(String.self, forKey: .event) ?? "event"
        message = try c.decodeIfPresent(String.self, forKey: .message) ?? ""
        fields = try c.decodeIfPresent([String: JSONValue].self, forKey: .fields) ?? [:]
    }
}

public struct ControlHealth: Codable, Sendable, Equatable {
    public var service: String
    public var status: String
    public var version: String
    public var runtimeReady: Bool
    public var olcrtcReady: Bool

    public init(
        service: String = "",
        status: String = "",
        version: String = "",
        runtimeReady: Bool = false,
        olcrtcReady: Bool = false
    ) {
        self.service = service
        self.status = status
        self.version = version
        self.runtimeReady = runtimeReady
        self.olcrtcReady = olcrtcReady
    }

    enum CodingKeys: String, CodingKey {
        case service
        case status
        case version
        case runtimeReady = "runtime_ready"
        case olcrtcReady = "olcrtc_ready"
    }

    public init(from decoder: any Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        service = try c.decodeIfPresent(String.self, forKey: .service) ?? ""
        status = try c.decodeIfPresent(String.self, forKey: .status) ?? ""
        version = try c.decodeIfPresent(String.self, forKey: .version) ?? ""
        runtimeReady = try c.decodeIfPresent(Bool.self, forKey: .runtimeReady) ?? false
        olcrtcReady = try c.decodeIfPresent(Bool.self, forKey: .olcrtcReady) ?? false
    }
}
