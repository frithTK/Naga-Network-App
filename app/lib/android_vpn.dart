import 'dart:io';

import 'package:flutter/foundation.dart';
import 'package:flutter/services.dart';

import 'app_update.dart';
import 'control_plane_client.dart';

const vpnChannelName = 'eu.nagavpn.naga_network/vpn';

bool get nagaRunsOnAndroid => !kIsWeb && Platform.isAndroid;

class AndroidVpn {
  AndroidVpn({
    MethodChannel? vpnChannel,
    MethodChannel? updateChannel,
  }) : _vpn = vpnChannel ?? const MethodChannel(vpnChannelName),
       _update = updateChannel ?? const MethodChannel(updateChannelName);

  final MethodChannel _vpn;
  final MethodChannel _update;

  static final AndroidVpn instance = AndroidVpn();

  Future<String?> waitForControlToken({
    int attempts = 40,
    Duration delay = const Duration(milliseconds: 250),
  }) async {
    for (var i = 0; i < attempts; i++) {
      try {
        final dir = await _update.invokeMethod<String>('dataDir');
        if (dir != null && dir.isNotEmpty) {
          final file = File('$dir${Platform.pathSeparator}control.token');
          if (file.existsSync()) {
            final token = file.readAsStringSync().trim();
            if (token.isNotEmpty) return token;
          }
        }
      } on MissingPluginException {
        return null;
      } on PlatformException {
        // Go may still be writing the token.
      } on FileSystemException {
        // Token file appears after NagaStart.
      }
      await Future<void>.delayed(delay);
    }
    return null;
  }

  Future<bool> waitUntilHealthy(
    ControlPlaneClient client, {
    int attempts = 40,
    Duration delay = const Duration(milliseconds: 250),
  }) async {
    for (var i = 0; i < attempts; i++) {
      try {
        if (await client.health()) return true;
      } on ControlPlaneException {
        // Control-plane starts from Application.onCreate.
      }
      await Future<void>.delayed(delay);
    }
    return false;
  }

  Future<bool> prepare() async {
    try {
      return await _vpn.invokeMethod<bool>('prepare') ?? false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  Future<bool> start({
    required String engine,
    required String routing,
    required List<String> packages,
  }) async {
    try {
      return await _vpn.invokeMethod<bool>('start', {
            'engine': engine,
            'routing': routing,
            'packages': packages,
          }) ??
          false;
    } on MissingPluginException {
      return false;
    } on PlatformException {
      return false;
    }
  }

  Future<void> stop() async {
    try {
      await _vpn.invokeMethod<bool>('stop');
    } on MissingPluginException {
      // Widget tests and desktop hosts have no VpnService.
    } on PlatformException {
      // Disconnect must still proceed in Dart.
    }
  }

  Future<List<DiscoveredApp>> listPackages() async {
    try {
      final raw = await _vpn.invokeMethod<List<dynamic>>('listPackages');
      if (raw == null) return const [];
      return raw
          .whereType<Map>()
          .map(
            (item) => DiscoveredApp(
              name: item['name'] as String? ?? '',
              process: item['process'] as String? ?? '',
              processPath: item['process_path'] as String? ?? '',
            ),
          )
          .where((app) => app.process.isNotEmpty)
          .toList(growable: false);
    } on MissingPluginException {
      return const [];
    } on PlatformException {
      return const [];
    }
  }
}
