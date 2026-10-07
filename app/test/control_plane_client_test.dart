import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:naga_network/control_plane_client.dart';

void main() {
  test('listProfiles maps 401 to ControlPlaneException', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        expect(request.headers['Authorization'], 'Bearer secret-token');
        return http.Response(
          '{"error":"control-plane authorization required"}',
          401,
        );
      }),
    );

    expect(
      client.listProfiles(),
      throwsA(
        isA<ControlPlaneException>().having(
          (error) => error.message,
          'message',
          isNot(contains('secret-token')),
        ),
      ),
    );
  });

  test('listProfiles maps offline errors without leaking URLs', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        throw http.ClientException('Connection refused', request.url);
      }),
    );

    expect(
      client.listProfiles(),
      throwsA(
        isA<ControlPlaneException>().having(
          (error) => error.message,
          'message',
          allOf(
            contains('Не удалось загрузить сохранённые профили'),
            isNot(contains('secret-token')),
            isNot(contains('http://')),
          ),
        ),
      ),
    );
  });

  test('healthInfo reads control-plane version', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        expect(request.url.path, '/v1/health');
        return http.Response(
          '{"service":"naga-control","status":"ok","version":"1.2.4","runtime_ready":true}',
          200,
        );
      }),
    );
    final health = await client.healthInfo();
    expect(health.ok, isTrue);
    expect(health.version, '1.2.4');
    expect(health.runtimeReady, isTrue);
  });

  test(
    'shareProfile returns URL without leaking bearer token on error',
    () async {
      final client = ControlPlaneClient(
        baseUrl: 'http://127.0.0.1:8765',
        token: 'secret-token',
        client: MockClient((request) async {
          expect(request.method, 'GET');
          expect(request.url.path, '/v1/profiles/profile-1/share');
          expect(request.headers['Authorization'], 'Bearer secret-token');
          return http.Response(
            '{"url":"https://example.invalid/sub.json","name":"Naga"}',
            200,
          );
        }),
      );

      final share = await client.shareProfile('profile-1');
      expect(share.url, 'https://example.invalid/sub.json');
      expect(share.name, 'Naga');
    },
  );

  test('shareProfile maps empty URL as a copy error', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        return http.Response('{"url":"  ","name":"Naga"}', 200);
      }),
    );

    expect(
      client.shareProfile('profile-1'),
      throwsA(
        isA<ControlPlaneException>().having(
          (error) => error.message,
          'message',
          'У профиля нет ссылки для копирования.',
        ),
      ),
    );
  });

  test('importProfile unwraps nagavpn deep-link before POST', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/profiles/import');
        expect(
          request.body,
          contains('"url":"https://nagavpn.example/bundles/nagavpn/uuid.json"'),
        );
        expect(request.body, isNot(contains('nagavpn://')));
        return http.Response(
          '{"profile_id":"p1","preview":{"profile_title":"Naga","provider_name":"NagaVPN","engine":"sing-box","upload_bytes":0,"download_bytes":0,"total_bytes":0,"unlimited":true,"expire_utc":"","update_interval_seconds":86400,"can_connect":true}}',
          200,
        );
      }),
    );

    final saved = await client.importProfile(
      'nagavpn://install-config?url=https%3A%2F%2Fnagavpn.example%2Fbundles%2Fnagavpn%2Fuuid.json',
    );
    expect(saved.profileId, 'p1');
    expect(saved.engine, 'sing-box');
  });

  test('importConfig posts raw profile bytes without a URL', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        expect(request.method, 'POST');
        expect(request.url.path, '/v1/profiles/import');
        expect(request.body, contains('"config":"{}"'));
        expect(request.body, contains('"name":"estonia.json"'));
        expect(request.body, isNot(contains('"url"')));
        return http.Response(
          '{"profile_id":"p-file","preview":{"profile_title":"estonia","provider_name":"","engine":"sing-box","upload_bytes":0,"download_bytes":0,"total_bytes":0,"unlimited":true,"expire_utc":"","update_interval_seconds":0,"can_connect":true}}',
          200,
        );
      }),
    );

    final saved = await client.importConfig(config: '{}', name: 'estonia.json');
    expect(saved.profileId, 'p-file');
    expect(saved.profileTitle, 'estonia');
  });

  test('ruleId keeps Linux paths and uses basename for Windows paths', () {
    const windowsApp = DiscoveredApp(
      name: 'Telegram',
      process: 'Telegram.exe',
      processPath: r'C:\Program Files\Telegram Desktop\Telegram.exe',
    );
    expect(windowsApp.ruleId, 'Telegram.exe');
    expect(
      windowsApp.ruleProcessId,
      r'C:\Program Files\Telegram Desktop\Telegram.exe',
    );

    const linuxApp = DiscoveredApp(
      name: 'firefox',
      process: 'firefox',
      processPath: '/usr/bin/firefox',
    );
    expect(linuxApp.ruleId, '/usr/bin/firefox');
    expect(linuxApp.ruleProcessId, '/usr/bin/firefox');
  });

  test('startRuntime keeps Windows TUN leftover error', () async {
    final client = ControlPlaneClient(
      baseUrl: 'http://127.0.0.1:8765',
      token: 'secret-token',
      client: MockClient((request) async {
        return http.Response(
          '{"error":"Не удалось создать сетевой адаптер VPN: старый TUN ещё занят. Закрой другие VPN и повтори подключение, при необходимости перезагрузи Windows."}',
          409,
          headers: const {'content-type': 'application/json; charset=utf-8'},
        );
      }),
    );

    expect(
      client.startRuntime('profile-1'),
      throwsA(
        isA<ControlPlaneException>().having(
          (error) => error.message,
          'message',
          allOf(
            contains('старый TUN ещё занят'),
            isNot(contains('CAP_NET_ADMIN')),
            isNot(contains('secret-token')),
          ),
        ),
      ),
    );
  });

  test('RoutingPolicy migrates legacy ru_direct', () {
    final parsed = RoutingPolicy.fromJson({
      'mode': 'all_vpn',
      'ru_direct': false,
      'apps': [],
    });
    expect(parsed.providerRules, isFalse);
    expect(parsed.builtinRU, isFalse);
    expect(parsed.builtinPrivate, isTrue);
    expect(parsed.toJson()['provider_rules'], isFalse);
    expect(parsed.toJson()['builtin_ru'], isFalse);
  });

  test('RoutingPolicy defaults new toggles to enabled', () {
    final parsed = RoutingPolicy.fromJson({'mode': 'all_vpn'});
    expect(parsed.providerRules, isTrue);
    expect(parsed.builtinRU, isTrue);
    expect(parsed.builtinPrivate, isTrue);
  });
}
