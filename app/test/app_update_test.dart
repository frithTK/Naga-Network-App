import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:naga_network/app_update.dart';
import 'package:path/path.dart' as p;

void main() {
  test('Semver strips v prefix and build number', () {
    expect(Semver.tryParse('v1.2.4+9'), Semver(1, 2, 4));
    expect(Semver.tryParse('1.2.4'), Semver(1, 2, 4));
    expect(Semver.tryParse('1.3.0')! > Semver.tryParse('1.2.4')!, isTrue);
    expect(Semver.tryParse('1.2.4')! >= Semver.tryParse('v1.2.4')!, isTrue);
    expect(Semver.tryParse('dev'), isNull);
  });

  test('controlPlaneBehind only when both parse and UI is newer', () {
    expect(controlPlaneBehind('1.2.5', '1.2.4'), isTrue);
    expect(controlPlaneBehind('1.2.4', '1.2.4'), isFalse);
    expect(controlPlaneBehind('1.2.4', '1.2.5'), isFalse);
    expect(controlPlaneBehind('1.2.4', 'dev'), isFalse);
  });

  test('parseSha256Digest accepts GitHub digest field', () {
    const hex = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
    expect(parseSha256Digest('sha256:$hex'), hex);
    expect(parseSha256Digest(hex.toUpperCase()), hex);
    expect(parseSha256Digest('md5:abc'), isNull);
  });

  test('required Linux assets include naga-control', () {
    expect(
      requiredAssetNames(UpdateInstallKind.linuxAppImage, '1.2.5'),
      [
        'NagaNetwork-1.2.5-x86_64.AppImage',
        'naga-control-linux-x86_64',
      ],
    );
    expect(
      requiredAssetNames(UpdateInstallKind.android, '1.2.5'),
      ['NagaNetwork-1.2.5-arm64-v8a.apk'],
    );
  });

  test('detectInstallKind treats APPIMAGE as packaged Linux', () {
    expect(
      detectInstallKind(
        isLinux: true,
        isWindows: false,
        isAndroid: false,
        environment: const {'APPIMAGE': '/home/me/NagaNetwork-1.2.4-x86_64.AppImage'},
      ),
      UpdateInstallKind.linuxAppImage,
    );
    expect(
      detectInstallKind(
        isLinux: true,
        isWindows: false,
        isAndroid: false,
        environment: const {},
      ),
      UpdateInstallKind.linuxDev,
    );
  });

  test('GithubRelease.fromJson maps assets and tag', () {
    final release = GithubRelease.fromJson({
      'tag_name': 'v1.2.5',
      'assets': [
        {
          'name': 'naga-control-linux-x86_64',
          'browser_download_url': 'https://example.test/control',
          'digest': 'sha256:${'a' * 64}',
          'size': 12,
        },
      ],
    });
    expect(release.version, '1.2.5');
    expect(release.assetNamed('naga-control-linux-x86_64')?.url, contains('control'));
  });

  test('checkLatest sends User-Agent and finds a newer release', () async {
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.linuxAppImage,
      client: MockClient((request) async {
        expect(request.url.path, endsWith('/releases/latest'));
        expect(request.headers['user-agent'], 'Naga-Network-App/1.2.4');
        expect(request.headers['accept'], contains('github+json'));
        return http.Response(
          jsonEncode({
            'tag_name': 'v1.2.5',
            'assets': [
              {
                'name': 'NagaNetwork-1.2.5-x86_64.AppImage',
                'browser_download_url': 'https://example.test/ui',
              },
              {
                'name': 'naga-control-linux-x86_64',
                'browser_download_url': 'https://example.test/control',
              },
            ],
          }),
          200,
          headers: {'etag': '"abc"'},
        );
      }),
    );
    addTearDown(updater.close);
    final result = await updater.checkLatest();
    expect(result.newerAvailable, isTrue);
    expect(result.latestVersion, '1.2.5');
    expect(result.missingAssets, isEmpty);
  });

  test('checkLatest does not offer a downgrade', () async {
    final updater = AppUpdater(
      currentVersion: '1.3.0',
      kind: UpdateInstallKind.windowsPortable,
      client: MockClient((request) async {
        return http.Response(jsonEncode({'tag_name': 'v1.2.5', 'assets': []}), 200);
      }),
    );
    addTearDown(updater.close);
    final result = await updater.checkLatest();
    expect(result.newerAvailable, isFalse);
  });

  test('checkLatest maps GitHub rate limit', () async {
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.windowsPortable,
      client: MockClient((request) async => http.Response('rate limited', 403)),
    );
    addTearDown(updater.close);
    expect(
      updater.checkLatest(),
      throwsA(
        isA<AppUpdateException>().having(
          (error) => error.message,
          'message',
          contains('ограничил запросы'),
        ),
      ),
    );
  });

  test('checkLatest reports missing linux runtime asset', () async {
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.linuxAppImage,
      client: MockClient((request) async {
        return http.Response(
          jsonEncode({
            'tag_name': 'v1.2.5',
            'assets': [
              {
                'name': 'NagaNetwork-1.2.5-x86_64.AppImage',
                'browser_download_url': 'https://example.test/ui',
              },
            ],
          }),
          200,
        );
      }),
    );
    addTearDown(updater.close);
    final result = await updater.checkLatest();
    expect(result.newerAvailable, isTrue);
    expect(result.missingAssets, ['naga-control-linux-x86_64']);
  });

  test('checkLatest reuses ETag cache on 304', () async {
    var calls = 0;
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.windowsPortable,
      client: MockClient((request) async {
        calls += 1;
        if (calls == 1) {
          return http.Response(
            jsonEncode({
              'tag_name': 'v1.2.5',
              'assets': [
                {
                  'name': 'NagaNetwork-1.2.5-windows-x64.zip',
                  'browser_download_url': 'https://example.test/zip',
                },
              ],
            }),
            200,
            headers: {'etag': '"rel"'},
          );
        }
        expect(request.headers['if-none-match'], '"rel"');
        return http.Response('', 304);
      }),
    );
    addTearDown(updater.close);
    expect((await updater.checkLatest()).latestVersion, '1.2.5');
    expect((await updater.checkLatest()).latestVersion, '1.2.5');
    expect(calls, 2);
  });

  test('downloadRequired verifies sha256 and refuses linux-dev apply', () async {
    const payload = 'naga';
    final digest = sha256.convert(utf8.encode(payload)).toString();
    final temp = Directory.systemTemp.createTempSync('naga-update-test');
    addTearDown(() => temp.deleteSync(recursive: true));
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.android,
      tempDirectory: () => temp,
      client: MockClient((request) async {
        if (request.url.path.contains('releases/latest')) {
          return http.Response('no', 500);
        }
        return http.Response(
          payload,
          200,
          headers: {'content-type': 'application/octet-stream'},
        );
      }),
    );
    addTearDown(updater.close);
    final files = await updater.downloadRequired(
      GithubRelease.fromJson({
        'tag_name': 'v1.2.5',
        'assets': [
          {
            'name': 'NagaNetwork-1.2.5-arm64-v8a.apk',
            'browser_download_url': 'https://example.test/app.apk',
            'digest': 'sha256:$digest',
            'size': payload.length,
          },
        ],
      }),
      '1.2.5',
    );
    expect(files, hasLength(1));
    expect(files.single.readAsStringSync(), payload);

    final dev = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.linuxDev,
      client: MockClient((request) async => http.Response('{}', 200)),
    );
    addTearDown(dev.close);
    expect(
      dev.downloadRequired(
        GithubRelease.fromJson({'tag_name': 'v1.2.5', 'assets': []}),
        '1.2.5',
      ),
      throwsA(isA<AppUpdateException>().having(
        (error) => error.message,
        'message',
        contains('AppImage'),
      )),
    );
  });

  test('downloadRequired rejects digest mismatch', () async {
    final temp = Directory.systemTemp.createTempSync('naga-update-bad');
    addTearDown(() => temp.deleteSync(recursive: true));
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.android,
      tempDirectory: () => temp,
      client: MockClient((request) async => http.Response('hello', 200)),
    );
    addTearDown(updater.close);
    expect(
      updater.downloadRequired(
        GithubRelease.fromJson({
          'tag_name': 'v1.2.5',
          'assets': [
            {
              'name': 'NagaNetwork-1.2.5-arm64-v8a.apk',
              'browser_download_url': 'https://example.test/app.apk',
              'digest': 'sha256:${'0' * 64}',
              'size': 5,
            },
          ],
        }),
        '1.2.5',
      ),
      throwsA(isA<AppUpdateException>().having(
        (error) => error.message,
        'message',
        contains('Контрольная сумма'),
      )),
    );
  });

  testWidgets('applyAndroid uses MethodChannel', (tester) async {
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(
      const MethodChannel(updateChannelName),
      (call) async {
        expect(call.method, 'applyAndroid');
        expect((call.arguments as Map)['apkPath'], contains('.apk'));
        return null;
      },
    );
    final apk = File(p.join(Directory.systemTemp.path, 'NagaNetwork-1.2.5-arm64-v8a.apk'));
    apk.writeAsStringSync('apk');
    addTearDown(() {
      if (apk.existsSync()) apk.deleteSync();
    });
    final updater = AppUpdater(
      currentVersion: '1.2.4',
      kind: UpdateInstallKind.android,
      client: MockClient((request) async => http.Response('{}', 200)),
    );
    addTearDown(updater.close);
    await updater.applyDownloaded(
      [apk],
      version: '1.2.5',
      disconnectIfNeeded: () async => true,
      waitForControlVersion: (_) async => true,
    );
  });

  test('canAutoApplyUpdate requires toggle, assets and packaged install', () {
    const result = UpdateCheckResult(
      currentVersion: '1.2.4',
      latestVersion: '1.2.5',
      newerAvailable: true,
    );
    expect(
      canAutoApplyUpdate(
        result: result,
        kind: UpdateInstallKind.linuxAppImage,
        autoInstall: true,
      ),
      isTrue,
    );
    expect(
      canAutoApplyUpdate(
        result: result,
        kind: UpdateInstallKind.linuxDev,
        autoInstall: true,
      ),
      isFalse,
    );
    expect(
      canAutoApplyUpdate(
        result: result,
        kind: UpdateInstallKind.linuxAppImage,
        autoInstall: false,
      ),
      isFalse,
    );
    expect(
      canAutoApplyUpdate(
        result: UpdateCheckResult(
          currentVersion: '1.2.4',
          latestVersion: '1.2.5',
          newerAvailable: true,
          missingAssets: const ['naga-control-linux-x86_64'],
        ),
        kind: UpdateInstallKind.linuxAppImage,
        autoInstall: true,
      ),
      isFalse,
    );
  });

  test('AppUpdatePrefs dueForCheck uses 12 hour interval', () {
    final now = DateTime.utc(2026, 10, 7, 15);
    expect(const AppUpdatePrefs().dueForCheck(now), isTrue);
    expect(
      AppUpdatePrefs(
        lastCheckUtc: now.subtract(const Duration(hours: 11)),
      ).dueForCheck(now),
      isFalse,
    );
    expect(
      AppUpdatePrefs(
        lastCheckUtc: now.subtract(const Duration(hours: 12)),
      ).dueForCheck(now),
      isTrue,
    );
  });

  test('AppUpdatePrefs round-trip on disk', () {
    final file = File(
      p.join(Directory.systemTemp.createTempSync('naga-prefs').path, 'app-update.json'),
    );
    addTearDown(() => file.parent.deleteSync(recursive: true));
    final original = AppUpdatePrefs(
      autoInstall: false,
      lastCheckUtc: DateTime.utc(2026, 10, 7, 12),
      lastLatestVersion: '1.2.5',
    );
    saveAppUpdatePrefs(original, path: file.path);
    final loaded = loadAppUpdatePrefs(path: file.path);
    expect(loaded.autoInstall, isFalse);
    expect(loaded.lastLatestVersion, '1.2.5');
    expect(loaded.lastCheckUtc!.toUtc(), DateTime.utc(2026, 10, 7, 12));
  });
}
