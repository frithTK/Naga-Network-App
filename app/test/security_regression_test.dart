import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:naga_network/app_update.dart';
import 'package:naga_network/control_plane_client.dart';
import 'package:naga_network/main.dart';

import 'support/ui_fixtures.dart';

void main() {
  test(
    'update download cannot overwrite a file outside its temporary directory',
    () async {
      final root = await Directory.systemTemp.createTemp('naga-update-audit-');
      addTearDown(() => root.delete(recursive: true));
      final victim = File('${root.path}/private-config');
      await victim.writeAsString('original');
      final predicted = Directory('${root.path}/naga-update-1234');
      await predicted.create();
      final name = androidApkName('1.2.6');
      await Link('${predicted.path}/$name').create(victim.path);
      final payload = utf8.encode('downloaded executable');
      final updater = AppUpdater(
        kind: UpdateInstallKind.android,
        client: MockClient((_) async => http.Response.bytes(payload, 200)),
        tempDirectory: () => root,
        now: () => DateTime.fromMillisecondsSinceEpoch(1234),
      );
      addTearDown(updater.close);
      final release = GithubRelease(
        tagName: 'v1.2.6',
        assets: [
          ReleaseAsset(
            name: name,
            url: 'https://example.invalid/asset',
            size: payload.length,
            digest: 'sha256:${sha256.convert(payload)}',
          ),
        ],
      );
      await updater.downloadRequired(release, '1.2.6');
      expect(
        await victim.readAsString(),
        'original',
        reason:
            'A pre-created directory/symlink must not redirect update writes',
      );
    },
  );

  test(
    'update rejects missing digest and truncated payload and removes staging',
    () async {
      for (final missingDigest in [true, false]) {
        final root = await Directory.systemTemp.createTemp(
          'naga-download-bounds-',
        );
        addTearDown(() => root.delete(recursive: true));
        final payload = utf8.encode('short');
        final updater = AppUpdater(
          kind: UpdateInstallKind.android,
          tempDirectory: () => root,
          client: MockClient((_) async => http.Response.bytes(payload, 200)),
        );
        addTearDown(updater.close);
        final release = GithubRelease(
          tagName: 'v1.2.6',
          assets: [
            ReleaseAsset(
              name: androidApkName('1.2.6'),
              url: 'https://example.invalid/asset',
              size: payload.length + 1,
              digest: missingDigest ? '' : 'sha256:${sha256.convert(payload)}',
            ),
          ],
        );
        await expectLater(
          updater.downloadRequired(release, '1.2.6'),
          throwsA(isA<AppUpdateException>()),
        );
        expect(await root.list().toList(), isEmpty);
      }
    },
  );

  testWidgets(
    'starting status becomes unknown after control-plane disappears',
    (tester) async {
      var offline = false;
      final client = ControlPlaneClient(
        token: 'audit',
        client: MockClient((request) async {
          final path = request.url.path;
          if (path == '/v1/runtime' && offline)
            throw http.ClientException('offline');
          final Object body = switch (path) {
            '/v1/runtime' => {...auditRuntimeJson, 'status': 'starting'},
            '/v1/profiles' => [auditProfileJson],
            '/v1/health' => {
              'service': 'naga-control',
              'status': 'ok',
              'version': '1.2.5',
            },
            '/v1/policy' => {'mode': 'auto', 'traffic_mode': 'tun'},
            '/v1/routing' => {'mode': 'all_vpn', 'apps': []},
            '/v1/profiles/fixture/nodes' => {
              'profile_id': 'fixture',
              'mode': 'Auto',
              'nodes': [],
            },
            '/v1/nodes' => [],
            '/v1/apps/discovered' => {'apps': []},
            _ => {},
          };
          return http.Response(
            jsonEncode(body),
            200,
            headers: {'content-type': 'application/json; charset=utf-8'},
          );
        }),
      );
      await tester.pumpWidget(
        MaterialApp(
          theme: nagaTheme(),
          home: DashboardPage(controlPlane: client),
        ),
      );
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 200));
      offline = true;
      for (var i = 0; i < 4; i++) {
        await tester.pump(const Duration(seconds: 2));
        await tester.pump();
      }
      try {
        expect(find.text('Статус неизвестен'), findsOneWidget);
      } finally {
        await tester.pumpWidget(const SizedBox.shrink());
      }
    },
  );
}
