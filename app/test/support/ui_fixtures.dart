import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:naga_network/control_plane_client.dart';
import 'package:naga_network/main.dart';

const auditRuntimeJson = <String, dynamic>{
  'status': 'connected',
  'engine': 'sing-box',
  'profile_id': 'fixture',
  'download_bytes': 188743680,
  'upload_bytes': 12582912,
  'download_rate_bytes': 430080,
  'upload_rate_bytes': 32768,
  'traffic_available': true,
  'active_node': 'Нидерланды · Амстердам',
  'active_country': 'Нидерланды',
  'active_protocol': 'Hysteria2',
  'active_latency_ms': 32,
  'active_node_status': 'healthy',
};

const auditProfileJson = <String, dynamic>{
  'id': 'fixture',
  'profile_id': 'fixture',
  'name': 'Naga Network',
  'profile_title': 'Naga Network',
  'provider_name': 'NagaVPN',
  'engine': 'sing-box',
  'unlimited': true,
  'can_connect': true,
  'update_interval_seconds': 86400,
};

ControlPlaneClient auditClient({
  bool empty = false,
  bool offline = false,
  bool Function()? runtimeOffline,
}) => ControlPlaneClient(
  token: 'synthetic-audit-token',
  client: MockClient((request) async {
    if (offline) throw http.ClientException('fixture offline');
    final path = request.url.path;
    if (path == '/v1/runtime' && runtimeOffline?.call() == true) {
      throw http.ClientException('fixture runtime offline');
    }
    final Object response = switch (path) {
      '/v1/profiles' => empty ? [] : [auditProfileJson],
      '/v1/runtime' => empty ? {'status': 'stopped'} : auditRuntimeJson,
      '/v1/health' => {'status': 'ok', 'service': 'naga-control'},
      '/v1/policy' => {'mode': 'auto', 'traffic_mode': 'tun'},
      '/v1/routing' => {
        'mode': 'all_vpn',
        'ru_direct': true,
        'provider_rules': true,
        'builtin_private': true,
        'builtin_ru': true,
        'apps': [],
      },
      '/v1/apps/discovered' => {'apps': []},
      '/v1/profiles/fixture/nodes' => {
        'profile_id': 'fixture',
        'mode': 'Auto',
        'nodes': [
          {'tag': 'Auto', 'type': 'selector', 'selected': true},
        ],
      },
      '/v1/nodes' =>
        empty
            ? []
            : [
                for (final (country, latency) in [
                  ('Нидерланды', 32),
                  ('Германия', 45),
                  ('Финляндия', 52),
                  ('Франция', 64),
                ])
                  {
                    'id': country,
                    'profile_id': 'fixture',
                    'profile_name': 'Naga Network',
                    'tag': country,
                    'type': 'hysteria2',
                    'country': country,
                    'protocol': 'Hysteria2',
                    'latency_ms': latency,
                  },
              ],
      '/v1/diagnostics/logs' => {'entries': []},
      _ => {},
    };
    return http.Response(
      jsonEncode(response),
      200,
      headers: {'content-type': 'application/json; charset=utf-8'},
    );
  }),
);

NagaHomeView auditHome({
  ConnectionStatus status = ConnectionStatus.connected,
  bool offline = false,
  bool hasProfile = true,
  bool expired = false,
  bool unknownSubscription = false,
  bool trafficAvailable = true,
  String trafficMode = 'tun',
  String? trafficModeError,
  ValueChanged<String>? onSelectTrafficMode,
  String runtimeStatus = 'connected',
  VoidCallback? onToggle,
}) => NagaHomeView(
  status: status,
  profileName: 'Naga Network',
  profileProvider: 'NagaVPN',
  selectedNode: 'Автоматически',
  runtime: RuntimeSnapshot.fromJson({
    ...auditRuntimeJson,
    'status': runtimeStatus,
    'traffic_available': trafficAvailable,
  }),
  trafficSamples: const [],
  preview: unknownSubscription
      ? null
      : ProfilePreview.fromJson({
          ...auditProfileJson,
          if (expired) 'expire_utc': '2020-01-01T00:00:00Z',
          if (expired) 'can_connect': false,
        }),
  hasProfile: hasProfile,
  isImporting: false,
  onImport: () {},
  onPaste: () {},
  onOpenServerPicker: () {},
  onOpenDiagnostics: () {},
  onToggle: onToggle ?? () {},
  trafficMode: trafficMode,
  onSelectTrafficMode: onSelectTrafficMode ?? (_) {},
  trafficModeError: trafficModeError,
  controlPlaneReachable: !offline,
);

Widget auditShell(
  Widget child, {
  double scale = 1,
  bool reducedMotion = false,
}) => MaterialApp(
  debugShowCheckedModeBanner: false,
  theme: nagaTheme(),
  builder: (context, widget) => MediaQuery(
    data: MediaQuery.of(context).copyWith(
      textScaler: TextScaler.linear(scale),
      disableAnimations: reducedMotion,
    ),
    child: widget!,
  ),
  home: Scaffold(
    body: SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: child,
    ),
  ),
);

Future<void> loadAuditFonts() async {
  final inter = FontLoader('Inter');
  for (final face in ['Regular', 'Medium', 'SemiBold', 'Bold']) {
    inter.addFont(rootBundle.load('assets/fonts/Inter-$face.ttf'));
  }
  await inter.load();
  await (FontLoader(
    'MaterialIcons',
  )..addFont(rootBundle.load('fonts/MaterialIcons-Regular.otf'))).load();
}
