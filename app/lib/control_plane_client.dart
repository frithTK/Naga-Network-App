import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import 'control_plane_token.dart'
    if (dart.library.io) 'control_plane_token_io.dart';
import 'subscription_fetch.dart';
import 'subscription_link.dart';

class ProfilePreview {
  const ProfilePreview({
    this.profileId,
    required this.profileTitle,
    required this.providerName,
    required this.engine,
    required this.uploadBytes,
    required this.downloadBytes,
    required this.totalBytes,
    required this.unlimited,
    required this.expireUtc,
    required this.updateIntervalSeconds,
    required this.canConnect,
    this.olcrtcAvailable = false,
  });

  final String? profileId;
  final String profileTitle;
  final String providerName;
  final String engine;
  final int uploadBytes;
  final int downloadBytes;
  final int totalBytes;
  final bool unlimited;
  final DateTime? expireUtc;
  final int updateIntervalSeconds;
  final bool canConnect;
  final bool olcrtcAvailable;

  factory ProfilePreview.fromJson(Map<String, dynamic> json) {
    final rawExpire = json['expire_utc'] as String?;
    return ProfilePreview(
      profileId: json['profile_id'] as String?,
      profileTitle: json['profile_title'] as String? ?? 'Naga Network',
      providerName: json['provider_name'] as String? ?? '',
      engine: json['engine'] as String? ?? 'unknown',
      uploadBytes: json['upload_bytes'] as int? ?? 0,
      downloadBytes: json['download_bytes'] as int? ?? 0,
      totalBytes: json['total_bytes'] as int? ?? 0,
      unlimited: json['unlimited'] as bool? ?? false,
      expireUtc: rawExpire == null ? null : DateTime.tryParse(rawExpire),
      updateIntervalSeconds: json['update_interval_seconds'] as int? ?? 86400,
      canConnect: json['can_connect'] as bool? ?? false,
      olcrtcAvailable: json['olcrtc_available'] as bool? ?? false,
    );
  }
}

class StoredProfileSummary {
  const StoredProfileSummary({
    required this.id,
    required this.name,
    required this.providerName,
    required this.engine,
    required this.importedAt,
    this.updatedAt,
    required this.uploadBytes,
    required this.downloadBytes,
    required this.totalBytes,
    required this.unlimited,
    required this.updateIntervalSeconds,
    required this.expireUtc,
    required this.canConnect,
    this.hasSourceUrl = true,
    this.olcrtcAvailable = false,
  });

  final String id;
  final String name;
  final String providerName;
  final String engine;
  final DateTime? importedAt;
  final DateTime? updatedAt;
  final int uploadBytes;
  final int downloadBytes;
  final int totalBytes;
  final bool unlimited;
  final int updateIntervalSeconds;
  final DateTime? expireUtc;
  final bool canConnect;
  final bool hasSourceUrl;
  final bool olcrtcAvailable;

  factory StoredProfileSummary.fromJson(Map<String, dynamic> json) {
    return StoredProfileSummary(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? 'Naga Network',
      providerName: json['provider_name'] as String? ?? '',
      engine: json['engine'] as String? ?? 'unknown',
      importedAt: DateTime.tryParse(json['imported_at'] as String? ?? ''),
      updatedAt: DateTime.tryParse(json['updated_at'] as String? ?? ''),
      uploadBytes: _jsonInt(json['upload_bytes']),
      downloadBytes: _jsonInt(json['download_bytes']),
      totalBytes: _jsonInt(json['total_bytes']),
      unlimited: json['unlimited'] as bool? ?? false,
      updateIntervalSeconds: _jsonInt(
        json['update_interval_seconds'],
        fallback: 86400,
      ),
      expireUtc: DateTime.tryParse(json['expire_utc'] as String? ?? ''),
      canConnect: json['can_connect'] as bool? ?? false,
      hasSourceUrl: json['has_source_url'] as bool? ?? true,
      olcrtcAvailable: json['olcrtc_available'] as bool? ?? false,
    );
  }
}

class ProfileShare {
  const ProfileShare({required this.url, required this.name});

  final String url;
  final String name;

  factory ProfileShare.fromJson(Map<String, dynamic> json) {
    return ProfileShare(
      url: (json['url'] as String? ?? '').trim(),
      name: json['name'] as String? ?? '',
    );
  }
}

class ProfileNode {
  const ProfileNode({
    required this.tag,
    required this.type,
    required this.country,
    required this.protocol,
    required this.selected,
    required this.children,
  });

  final String tag;
  final String type;
  final String country;
  final String protocol;
  final bool selected;
  final List<ProfileNode> children;

  factory ProfileNode.fromJson(Map<String, dynamic> json) {
    return ProfileNode(
      tag: json['tag'] as String? ?? '',
      type: json['type'] as String? ?? 'unknown',
      country: json['country'] as String? ?? '',
      protocol: json['protocol'] as String? ?? '',
      selected: json['selected'] as bool? ?? false,
      children: json['children'] is List
          ? (json['children'] as List)
                .whereType<Map<String, dynamic>>()
                .map(ProfileNode.fromJson)
                .toList()
          : const [],
    );
  }
}

class ProfileNodes {
  const ProfileNodes({
    required this.profileId,
    required this.mode,
    required this.nodes,
  });

  final String profileId;
  final String mode;
  final List<ProfileNode> nodes;

  factory ProfileNodes.fromJson(Map<String, dynamic> json) {
    final rawNodes = json['nodes'];
    return ProfileNodes(
      profileId: json['profile_id'] as String? ?? '',
      mode: json['mode'] as String? ?? '',
      nodes: rawNodes is List
          ? rawNodes
                .whereType<Map<String, dynamic>>()
                .map(ProfileNode.fromJson)
                .toList()
          : const [],
    );
  }
}

class UnifiedNode {
  const UnifiedNode({
    required this.id,
    required this.profileId,
    required this.tag,
    required this.runtimeTag,
    required this.country,
    required this.protocol,
    this.latencyMs,
    this.probeStatus = 'unknown',
  });

  final String id;
  final String profileId;
  final String tag;
  final String runtimeTag;
  final String country;
  final String protocol;
  final int? latencyMs;
  final String probeStatus;

  factory UnifiedNode.fromJson(Map<String, dynamic> json) {
    return UnifiedNode(
      id: json['id'] as String? ?? '',
      profileId: json['profile_id'] as String? ?? '',
      tag: json['tag'] as String? ?? '',
      runtimeTag: json['runtime_tag'] as String? ?? '',
      country: json['country'] as String? ?? '',
      protocol: json['protocol'] as String? ?? '',
      latencyMs: _jsonIntOrNull(json['latency_ms']),
      probeStatus: json['probe_status'] as String? ?? 'unknown',
    );
  }
}

class ConnectionPolicy {
  const ConnectionPolicy({
    required this.mode,
    required this.trafficMode,
    required this.preferredCountry,
    required this.preferredProtocol,
    required this.networkClass,
    required this.tuicFallbackEnabled,
  });

  final String mode;
  final String trafficMode;
  final String preferredCountry;
  final String preferredProtocol;
  final String networkClass;
  final bool tuicFallbackEnabled;

  factory ConnectionPolicy.fromJson(Map<String, dynamic> json) {
    return ConnectionPolicy(
      mode: json['mode'] as String? ?? 'auto',
      trafficMode: json['traffic_mode'] as String? ?? 'tun',
      preferredCountry: json['preferred_country'] as String? ?? '',
      preferredProtocol: json['preferred_protocol'] as String? ?? '',
      networkClass: json['network_class'] as String? ?? 'unknown',
      tuicFallbackEnabled: json['tuic_fallback_enabled'] as bool? ?? true,
    );
  }

  Map<String, dynamic> toJson() => {
    'mode': mode,
    'traffic_mode': trafficMode,
    'preferred_country': preferredCountry,
    'preferred_protocol': preferredProtocol,
    'network_class': networkClass,
    'tuic_fallback_enabled': tuicFallbackEnabled,
  };
}

class DiscoveredApp {
  const DiscoveredApp({
    required this.name,
    required this.process,
    required this.processPath,
  });

  final String name;
  final String process;
  final String processPath;

  factory DiscoveredApp.fromJson(Map<String, dynamic> json) {
    return DiscoveredApp(
      name: json['name'] as String? ?? '',
      process: json['process'] as String? ?? '',
      processPath: json['process_path'] as String? ?? '',
    );
  }

  String get ruleId {
    if (processPath.contains(r'\')) {
      return process.isNotEmpty ? process : name;
    }
    if (processPath.contains('/')) {
      return processPath;
    }
    return process.isNotEmpty ? process : name;
  }

  String get ruleProcessId {
    if (processPath.isNotEmpty) {
      return processPath;
    }
    return process.isNotEmpty ? process : name;
  }
}

class AppRoute {
  const AppRoute({
    required this.id,
    required this.displayName,
    required this.platform,
    required this.packageOrProcessId,
    required this.route,
    required this.enabled,
  });

  final String id;
  final String displayName;
  final String platform;
  final String packageOrProcessId;
  final String route;
  final bool enabled;

  factory AppRoute.fromJson(Map<String, dynamic> json) {
    return AppRoute(
      id: json['id'] as String? ?? '',
      displayName: json['display_name'] as String? ?? '',
      platform: json['platform'] as String? ?? 'linux',
      packageOrProcessId: json['package_name_or_process_id'] as String? ?? '',
      route: json['route'] as String? ?? 'vpn',
      enabled: json['enabled'] as bool? ?? true,
    );
  }

  Map<String, dynamic> toJson() => {
    'id': id,
    'display_name': displayName,
    'platform': platform,
    'package_name_or_process_id': packageOrProcessId,
    'route': route,
    'enabled': enabled,
  };
}

class RoutingPolicy {
  const RoutingPolicy({
    required this.mode,
    this.ruDirect = true,
    this.providerRules = true,
    this.builtinPrivate = true,
    this.builtinRU = true,
    required this.apps,
  });

  final String mode;
  final bool ruDirect;
  final bool providerRules;
  final bool builtinPrivate;
  final bool builtinRU;
  final List<AppRoute> apps;

  factory RoutingPolicy.fromJson(Map<String, dynamic> json) {
    final rawApps = json['apps'];
    final ruDirect = json['ru_direct'] as bool? ?? true;
    return RoutingPolicy(
      mode: json['mode'] as String? ?? 'all_vpn',
      ruDirect: json['builtin_ru'] as bool? ?? ruDirect,
      providerRules: json['provider_rules'] as bool? ?? ruDirect,
      builtinPrivate: json['builtin_private'] as bool? ?? true,
      builtinRU: json['builtin_ru'] as bool? ?? ruDirect,
      apps: rawApps is List
          ? rawApps
                .whereType<Map<String, dynamic>>()
                .map(AppRoute.fromJson)
                .toList()
          : const [],
    );
  }

  Map<String, dynamic> toJson() => {
    'mode': mode,
    'ru_direct': builtinRU,
    'provider_rules': providerRules,
    'builtin_private': builtinPrivate,
    'builtin_ru': builtinRU,
    'apps': apps.map((app) => app.toJson()).toList(),
  };

  RoutingPolicy copyWith({
    String? mode,
    bool? providerRules,
    bool? builtinPrivate,
    bool? builtinRU,
    List<AppRoute>? apps,
  }) {
    final nextRU = builtinRU ?? this.builtinRU;
    return RoutingPolicy(
      mode: mode ?? this.mode,
      ruDirect: nextRU,
      providerRules: providerRules ?? this.providerRules,
      builtinPrivate: builtinPrivate ?? this.builtinPrivate,
      builtinRU: nextRU,
      apps: apps ?? this.apps,
    );
  }
}

class RuntimeSnapshot {
  const RuntimeSnapshot({
    required this.status,
    required this.engine,
    required this.profileId,
    required this.error,
    required this.uploadBytes,
    required this.downloadBytes,
    required this.uploadRateBytes,
    required this.downloadRateBytes,
    required this.sessionStartedAt,
    required this.lastTrafficUpdate,
    required this.trafficAvailable,
    required this.activeNode,
    required this.activeCountry,
    required this.activeProtocol,
    required this.activeLatencyMs,
    required this.activeNodeStatus,
    this.failoverMessage,
  });

  final String status;
  final String engine;
  final String? profileId;
  final String? error;
  final int uploadBytes;
  final int downloadBytes;
  final int uploadRateBytes;
  final int downloadRateBytes;
  final DateTime? sessionStartedAt;
  final DateTime? lastTrafficUpdate;
  final bool trafficAvailable;
  final String? activeNode;
  final String? activeCountry;
  final String? activeProtocol;
  final int? activeLatencyMs;
  final String? activeNodeStatus;
  final String? failoverMessage;

  factory RuntimeSnapshot.fromJson(Map<String, dynamic> json) {
    return RuntimeSnapshot(
      status: json['status'] as String? ?? 'stopped',
      engine: json['engine'] as String? ?? 'sing-box',
      profileId: json['profile_id'] as String?,
      error: json['error'] as String?,
      uploadBytes: json['upload_bytes'] as int? ?? 0,
      downloadBytes: json['download_bytes'] as int? ?? 0,
      uploadRateBytes: _jsonInt(json['upload_rate_bytes']),
      downloadRateBytes: _jsonInt(json['download_rate_bytes']),
      sessionStartedAt: DateTime.tryParse(
        json['session_started_at'] as String? ?? '',
      ),
      lastTrafficUpdate: DateTime.tryParse(
        json['last_traffic_update'] as String? ?? '',
      ),
      trafficAvailable: json['traffic_available'] as bool? ?? false,
      activeNode: json['active_node'] as String?,
      activeCountry: json['active_country'] as String?,
      activeProtocol: json['active_protocol'] as String?,
      activeLatencyMs: _jsonIntOrNull(json['active_latency_ms']),
      activeNodeStatus: json['active_node_status'] as String?,
      failoverMessage: json['failover_message'] as String?,
    );
  }
}

class DiagnosticLogEntry {
  const DiagnosticLogEntry({
    required this.timestamp,
    required this.level,
    required this.component,
    required this.event,
    required this.message,
    required this.fields,
  });

  final DateTime? timestamp;
  final String level;
  final String component;
  final String event;
  final String message;
  final Map<String, dynamic> fields;

  factory DiagnosticLogEntry.fromJson(Map<String, dynamic> json) {
    final rawFields = json['fields'];
    return DiagnosticLogEntry(
      timestamp: DateTime.tryParse(json['timestamp'] as String? ?? ''),
      level: json['level'] as String? ?? 'info',
      component: json['component'] as String? ?? 'app',
      event: json['event'] as String? ?? 'event',
      message: json['message'] as String? ?? '',
      fields: rawFields is Map<String, dynamic> ? rawFields : const {},
    );
  }
}

class ControlHealth {
  const ControlHealth({
    required this.ok,
    this.version = '',
    this.runtimeReady = false,
    this.olcrtcReady = false,
  });

  final bool ok;
  final String version;
  final bool runtimeReady;
  final bool olcrtcReady;

  factory ControlHealth.fromJson(Map<String, dynamic> json) {
    return ControlHealth(
      ok: json['status'] == 'ok',
      version: json['version'] as String? ?? '',
      runtimeReady: json['runtime_ready'] as bool? ?? false,
      olcrtcReady: json['olcrtc_ready'] as bool? ?? false,
    );
  }
}

class ControlPlaneException implements Exception {
  const ControlPlaneException(this.message);

  final String message;

  @override
  String toString() => message;
}

class ControlPlaneClient {
  ControlPlaneClient({
    String baseUrl = const String.fromEnvironment(
      'NAGA_CONTROL_URL',
      defaultValue: 'http://127.0.0.1:8765',
    ),
    String? token,
    http.Client? client,
  }) : _baseUrl = baseUrl.replaceFirst(RegExp(r'/$'), ''),
       _token = token == null || token.trim().isEmpty
           ? loadControlPlaneToken()
           : token.trim(),
       _client = client ?? http.Client();

  final String _baseUrl;
  final String? _token;
  final http.Client _client;

  Map<String, String> _headers([Map<String, String>? extra]) {
    return {if (_token != null) 'Authorization': 'Bearer $_token', ...?extra};
  }

  Future<ProfilePreview> preview(String subscriptionUrl) async {
    return _post('/v1/subscriptions/preview', subscriptionUrl);
  }

  Future<ProfilePreview> previewConfig({
    required String config,
    String? name,
    SubscriptionDownload? downloaded,
  }) async {
    return ProfilePreview.fromJson(
      await _postBody(
        '/v1/subscriptions/preview',
        _configBody(config: config, name: name, downloaded: downloaded),
      ),
    );
  }

  Future<ProfilePreview> importProfile(String subscriptionUrl) async {
    return _importSaved(
      await _postJson('/v1/profiles/import', subscriptionUrl),
    );
  }

  Future<ProfilePreview> importConfig({
    required String config,
    String? name,
    SubscriptionDownload? downloaded,
  }) async {
    return _importSaved(
      await _postBody(
        '/v1/profiles/import',
        _configBody(config: config, name: name, downloaded: downloaded),
      ),
    );
  }

  Map<String, String> _configBody({
    required String config,
    String? name,
    SubscriptionDownload? downloaded,
  }) {
    if (downloaded == null) {
      return {
        'config': config,
        if (name != null && name.trim().isNotEmpty) 'name': name.trim(),
      };
    }
    return downloaded.toJson();
  }

  ProfilePreview _importSaved(Map<String, dynamic> payload) {
    final nested = payload['preview'];
    if (nested is! Map<String, dynamic>) {
      throw const ControlPlaneException(
        'Control-plane вернул неполный профиль.',
      );
    }
    return ProfilePreview.fromJson({
      ...nested,
      'profile_id': payload['profile_id'],
    });
  }

  Future<List<StoredProfileSummary>> listProfiles() async {
    late http.Response response;
    try {
      response = await _client
          .get(Uri.parse('$_baseUrl/v1/profiles'), headers: _headers())
          .timeout(const Duration(seconds: 5));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось загрузить сохранённые профили.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw const ControlPlaneException('Профили не удалось загрузить.');
    }
    try {
      final payload = jsonDecode(response.body) as List<dynamic>;
      return payload
          .whereType<Map<String, dynamic>>()
          .map(StoredProfileSummary.fromJson)
          .toList();
    } catch (_) {
      throw const ControlPlaneException('Список профилей повреждён.');
    }
  }

  Future<ProfileShare> shareProfile(String profileId) async {
    final payload = await _getJson(
      '/v1/profiles/${Uri.encodeComponent(profileId)}/share',
    );
    final share = ProfileShare.fromJson(payload);
    if (share.url.isEmpty) {
      throw const ControlPlaneException(
        'У профиля нет ссылки для копирования.',
      );
    }
    return share;
  }

  Future<ProfileNodes> listNodes(String profileId) async {
    final payload = await _getJson(
      '/v1/profiles/${Uri.encodeComponent(profileId)}/nodes',
    );
    return ProfileNodes.fromJson(payload);
  }

  Future<ProfileNodes> selectMode(String profileId, String mode) async {
    final payload = await _postPath(
      '/v1/profiles/${Uri.encodeComponent(profileId)}/mode',
      {'mode': mode},
    );
    return ProfileNodes.fromJson(payload);
  }

  Future<List<UnifiedNode>> listUnifiedNodes() async {
    late http.Response response;
    try {
      response = await _client
          .get(Uri.parse('$_baseUrl/v1/nodes'), headers: _headers())
          .timeout(const Duration(seconds: 5));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось загрузить общий список узлов.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw const ControlPlaneException(
        'Общий список узлов не удалось загрузить.',
      );
    }
    try {
      final payload = jsonDecode(response.body) as List<dynamic>;
      return payload
          .whereType<Map<String, dynamic>>()
          .map(UnifiedNode.fromJson)
          .toList();
    } catch (_) {
      throw const ControlPlaneException('Общий список узлов повреждён.');
    }
  }

  Future<ProfilePreview> refreshWithConfig(
    String profileId,
    SubscriptionDownload downloaded,
  ) async {
    return _importSaved(
      await _postBody(
        '/v1/profiles/${Uri.encodeComponent(profileId)}/refresh',
        downloaded.toJson(includeUrl: false),
      ),
    );
  }

  Future<ProfilePreview> refreshProfile(String profileId) async {
    final payload = await _postPath(
      '/v1/profiles/${Uri.encodeComponent(profileId)}/refresh',
      const {},
    );
    final nested = payload['preview'];
    if (nested is! Map<String, dynamic>) {
      throw const ControlPlaneException(
        'Control-plane вернул неполный профиль.',
      );
    }
    return ProfilePreview.fromJson({
      ...nested,
      'profile_id': payload['profile_id'],
    });
  }

  Future<void> deleteProfile(String profileId) async {
    late http.Response response;
    try {
      response = await _client
          .delete(
            Uri.parse(
              '$_baseUrl/v1/profiles/${Uri.encodeComponent(profileId)}',
            ),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      try {
        final payload = jsonDecode(response.body) as Map<String, dynamic>;
        throw ControlPlaneException(
          payload['error'] as String? ?? 'Профиль не удалось удалить.',
        );
      } catch (error) {
        if (error is ControlPlaneException) rethrow;
        throw const ControlPlaneException('Профиль не удалось удалить.');
      }
    }
  }

  Future<RuntimeSnapshot> runtimeStatus() async {
    final payload = await _getJson('/v1/runtime');
    return RuntimeSnapshot.fromJson(payload);
  }

  Future<bool> health() async {
    return (await healthInfo()).ok;
  }

  Future<ControlHealth> healthInfo() async {
    final payload = await _getJson('/v1/health');
    return ControlHealth.fromJson(payload);
  }

  Future<List<DiagnosticLogEntry>> diagnosticLogs({int limit = 200}) async {
    final safeLimit = limit.clamp(1, 1000);
    final payload = await _getJson('/v1/diagnostics/logs?limit=$safeLimit');
    final rawEntries = payload['entries'];
    if (rawEntries is! List) return const [];
    return rawEntries
        .whereType<Map<String, dynamic>>()
        .map(DiagnosticLogEntry.fromJson)
        .toList(growable: false);
  }

  /// Persists a UI-side event into the control-plane journal so failures that
  /// never reach another endpoint (subscription download timeout, cancelled
  /// file dialog) still appear in the diagnostics log the user exports.
  Future<void> recordClientEvent({
    required String event,
    required String message,
    String level = 'error',
  }) async {
    try {
      await _client
          .post(
            Uri.parse('$_baseUrl/v1/diagnostics/client'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: jsonEncode({
              'level': level,
              'event': event,
              'message': message,
            }),
          )
          .timeout(const Duration(seconds: 5));
    } catch (_) {
      // Diagnostics logging must never mask the original user-facing error.
    }
  }

  Future<RuntimeSnapshot> startRuntime(String profileId) async {
    late http.Response response;
    try {
      response = await _client
          .post(
            Uri.parse('$_baseUrl/v1/runtime/start'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: jsonEncode({'profile_id': profileId}),
          )
          .timeout(const Duration(seconds: 90));
    } on TimeoutException {
      throw const ControlPlaneException(
        'Запуск VPN занял слишком много времени. Сервис Naga ещё работает — подожди или открой диагностику.',
      );
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    return RuntimeSnapshot.fromJson(_decodeResponse(response));
  }

  Future<RuntimeSnapshot> selectRuntime(
    String outbound, {
    String? profileId,
  }) async {
    final payload = await _runtimePost('/v1/runtime/select', {
      if (profileId != null && profileId.isNotEmpty) 'profile_id': profileId,
      'outbound': outbound,
    });
    return RuntimeSnapshot.fromJson(payload);
  }

  Future<RuntimeSnapshot> selectRuntimeAuto() async {
    final payload = await _runtimePost('/v1/runtime/select', {'auto': true});
    return RuntimeSnapshot.fromJson(payload);
  }

  Future<List<UnifiedNode>> probeUnifiedNodes() async {
    late http.Response response;
    try {
      response = await _client
          .post(
            Uri.parse('$_baseUrl/v1/nodes/probe'),
            headers: _headers(const {'Content-Type': 'application/json'}),
          )
          .timeout(const Duration(seconds: 110));
    } on TimeoutException {
      throw const ControlPlaneException(
        'Проверка серверов заняла слишком много времени.',
      );
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось проверить доступность серверов.',
      );
    }
    if (response.statusCode == 409) {
      throw const ControlPlaneException('Включи VPN, чтобы измерить задержку.');
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ControlPlaneException(
        _jsonErrorMessage(response.body) ??
            'Не удалось проверить доступность серверов.',
      );
    }
    try {
      final payload = jsonDecode(response.body) as List<dynamic>;
      return payload
          .whereType<Map<String, dynamic>>()
          .map(UnifiedNode.fromJson)
          .toList();
    } catch (_) {
      throw const ControlPlaneException('Общий список узлов повреждён.');
    }
  }

  Future<List<DiscoveredApp>> listDiscoveredApps() async {
    final payload = await _getJson('/v1/apps/discovered');
    final rawApps = payload['apps'];
    if (rawApps is List) {
      return rawApps
          .whereType<Map<String, dynamic>>()
          .map(DiscoveredApp.fromJson)
          .toList();
    }
    return const [];
  }

  Future<RuntimeSnapshot> stopRuntime() async {
    final payload = await _runtimePost('/v1/runtime/stop', null);
    return RuntimeSnapshot.fromJson(payload);
  }

  Future<ConnectionPolicy> connectionPolicy() async {
    return ConnectionPolicy.fromJson(await _getJson('/v1/policy'));
  }

  Future<ConnectionPolicy> saveConnectionPolicy(ConnectionPolicy policy) async {
    return ConnectionPolicy.fromJson(
      await _putJson('/v1/policy', policy.toJson()),
    );
  }

  Future<RoutingPolicy> routingPolicy() async {
    return RoutingPolicy.fromJson(await _getJson('/v1/routing'));
  }

  Future<RoutingPolicy> saveRoutingPolicy(RoutingPolicy policy) async {
    return RoutingPolicy.fromJson(
      await _putJson('/v1/routing', policy.toJson()),
    );
  }

  Future<List<AppRoute>> listApps() async {
    final payload = await _getJson('/v1/apps');
    final rawApps = payload['apps'];
    if (rawApps is List) {
      return rawApps
          .whereType<Map<String, dynamic>>()
          .map(AppRoute.fromJson)
          .toList();
    }
    // The endpoint wraps the array so the response remains a JSON object,
    // matching the rest of the control-plane client contract.
    return const [];
  }

  Future<AppRoute> saveApp(AppRoute app) async {
    final payload = await _putJson(
      '/v1/apps/${Uri.encodeComponent(app.id)}',
      app.toJson(),
    );
    return AppRoute.fromJson(payload);
  }

  Future<void> deleteApp(String id) async {
    late http.Response response;
    try {
      response = await _client
          .delete(
            Uri.parse('$_baseUrl/v1/apps/${Uri.encodeComponent(id)}'),
            headers: _headers(),
          )
          .timeout(const Duration(seconds: 10));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ControlPlaneException('Приложение не удалось удалить.');
    }
  }

  Future<Map<String, dynamic>> _getJson(String path) async {
    late http.Response response;
    try {
      response = await _client
          .get(Uri.parse('$_baseUrl$path'), headers: _headers())
          .timeout(const Duration(seconds: 5));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    return _decodeResponse(response);
  }

  Future<Map<String, dynamic>> _runtimePost(
    String path,
    Map<String, dynamic>? body,
  ) async {
    late http.Response response;
    try {
      response = await _client
          .post(
            Uri.parse('$_baseUrl$path'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: body == null ? null : jsonEncode(body),
          )
          .timeout(const Duration(seconds: 20));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    return _decodeResponse(response);
  }

  Future<Map<String, dynamic>> _putJson(
    String path,
    Map<String, dynamic> body,
  ) async {
    late http.Response response;
    try {
      response = await _client
          .put(
            Uri.parse('$_baseUrl$path'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 10));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    return _decodeResponse(response);
  }

  Future<Map<String, dynamic>> _postPath(
    String path,
    Map<String, dynamic> body,
  ) async {
    late http.Response response;
    try {
      response = await _client
          .post(
            Uri.parse('$_baseUrl$path'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 10));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane.',
      );
    }
    return _decodeResponse(response);
  }

  Map<String, dynamic> _decodeResponse(http.Response response) {
    Map<String, dynamic> payload;
    try {
      payload = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      throw const ControlPlaneException(
        'Control-plane вернул некорректный ответ.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ControlPlaneException(
        payload['error'] as String? ?? 'Операция не выполнена.',
      );
    }
    return payload;
  }

  Future<ProfilePreview> _post(String path, String subscriptionUrl) async {
    return ProfilePreview.fromJson(await _postJson(path, subscriptionUrl));
  }

  Future<Map<String, dynamic>> _postJson(
    String path,
    String subscriptionUrl,
  ) async {
    final resolved = subscriptionFetchURL(subscriptionUrl);
    if (resolved == null) {
      throw const ControlPlaneException(
        'Нужна HTTPS-ссылка профиля или nagavpn://install-config.',
      );
    }
    late http.Response response;
    try {
      response = await _client
          .post(
            Uri.parse('$_baseUrl$path'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: jsonEncode({'url': resolved}),
          )
          .timeout(const Duration(seconds: 35));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane. Запусти naga-control.',
      );
    }

    Map<String, dynamic> payload;
    try {
      payload = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      throw const ControlPlaneException(
        'Control-plane вернул некорректный ответ.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ControlPlaneException(
        payload['error'] as String? ?? 'Профиль не удалось проверить.',
      );
    }
    return payload;
  }

  Future<Map<String, dynamic>> _postBody(
    String path,
    Map<String, String> body,
  ) async {
    late http.Response response;
    try {
      response = await _client
          .post(
            Uri.parse('$_baseUrl$path'),
            headers: _headers(const {'Content-Type': 'application/json'}),
            body: jsonEncode(body),
          )
          .timeout(const Duration(seconds: 35));
    } catch (_) {
      throw const ControlPlaneException(
        'Не удалось связаться с локальным control-plane. Запусти naga-control.',
      );
    }

    Map<String, dynamic> payload;
    try {
      payload = jsonDecode(response.body) as Map<String, dynamic>;
    } catch (_) {
      throw const ControlPlaneException(
        'Control-plane вернул некорректный ответ.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw ControlPlaneException(
        payload['error'] as String? ?? 'Профиль не удалось проверить.',
      );
    }
    return payload;
  }
}

int _jsonInt(dynamic value, {int fallback = 0}) {
  if (value is int) return value;
  if (value is num) return value.toInt();
  return fallback;
}

int? _jsonIntOrNull(dynamic value) {
  if (value is int) return value;
  if (value is num) return value.toInt();
  return int.tryParse(value?.toString() ?? '');
}

String? _jsonErrorMessage(String body) {
  try {
    final decoded = jsonDecode(body);
    if (decoded is Map<String, dynamic>) {
      final error = decoded['error'];
      if (error is String && error.trim().isNotEmpty) {
        return error;
      }
    }
  } catch (_) {}
  return null;
}
