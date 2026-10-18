import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:naga_network/app_update.dart';
import 'package:naga_network/control_plane_client.dart';
import 'package:naga_network/macos_backend.dart';
import 'package:naga_network/macos_diagnostics.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  final messenger =
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
  late ControlPlaneClient client;
  late List<String> calls;

  Map<String, dynamic> result({
    String outcome = 'pass',
    String cleanup = 'pass',
  }) => {
    'schema_version': 1,
    'diagnostic_version': 'integrated-1',
    'outcome': outcome,
    'system': {
      'macos': '14.8.9',
      'architecture': 'arm64',
      'path': '/Users/private',
    },
    'app': {'version': embeddedAppVersion, 'build': '14'},
    'checks': [
      {
        'id': 'service_connection',
        'status': 'pass',
        'effective_uid': 0,
        'token': 'secret',
      },
    ],
    'cleanup': {
      'status': cleanup,
      'registration': cleanup == 'pass' ? 'not_registered' : 'enabled',
    },
    'token': 'secret',
    'logs': ['secret'],
  };

  void native(Future<Object?> Function(MethodCall) handler) {
    messenger.setMockMethodCallHandler(macOSNativeChannel, (call) {
      calls.add(call.method);
      return handler(call);
    });
  }

  setUp(() {
    calls = [];
    client = ControlPlaneClient(
      token: 'secret-token',
      dataDirectory: '/Users/private',
      client: MockClient((request) async {
        expect(request.url.path, '/v1/health');
        return http.Response(
          jsonEncode({'status': 'ok', 'version': embeddedAppVersion}),
          200,
        );
      }),
    );
  });
  tearDown(() {
    client.close();
    messenger.setMockMethodCallHandler(macOSNativeChannel, null);
    messenger.setMockMethodCallHandler(SystemChannels.platform, null);
  });

  testWidgets(
    'one click starts a check and copies a private result after cleanup',
    (tester) async {
      native((call) async {
        expect(call.method, 'runMacOSCheck');
        return result();
      });
      String? clipboard;
      messenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
        if (call.method == 'Clipboard.setData')
          clipboard = (call.arguments as Map)['text'] as String;
        return null;
      });
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: MacOSCheckBanner(client: client, notice: 'Тестовая сборка'),
          ),
        ),
      );
      await tester.tap(find.text('Проверить Mac'));
      await tester.pumpAndSettle();
      expect(find.text('Проверка пройдена'), findsOneWidget);
      expect(calls, ['runMacOSCheck']);
      expect(clipboard, isNull);
      await tester.tap(find.text('Скопировать отчёт'));
      await tester.pumpAndSettle();
      expect(clipboard, isNotNull);
      expect(clipboard, isNot(contains('secret')));
      expect(clipboard, isNot(contains('/Users')));
      expect(jsonDecode(clipboard!)['vpn_tested'], false);
      expect(jsonDecode(clipboard!)['overall_result'], 'pass');
      expect(find.text('Отчёт скопирован'), findsOneWidget);
      await tester.tap(find.text('Закрыть'));
      await tester.pumpAndSettle();
    },
  );

  testWidgets(
    'permission progress is automatic; cancelling waits for native cleanup',
    (tester) async {
      final pending = Completer<Object?>();
      native((call) async {
        switch (call.method) {
          case 'runMacOSCheck':
            return pending.future;
          case 'macOSCheckState':
            return {'stage': 'waiting_permission', 'running': true};
          case 'cancelMacOSCheck':
            return null;
          default:
            fail('unexpected ${call.method}');
        }
      });
      await tester.pumpWidget(
        MaterialApp(home: MacOSCheckDialog(client: client)),
      );
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 500));
      await tester.pump();
      expect(find.textContaining('продолжится автоматически'), findsOneWidget);
      await tester.tap(find.text('Отменить'));
      await tester.pump();
      expect(find.text('Закрыть'), findsNothing);
      pending.complete(result(outcome: 'cancelled'));
      await tester.pumpAndSettle();
      expect(find.text('Проверка отменена'), findsOneWidget);
      expect(find.text('Закрыть'), findsOneWidget);
      expect(calls, contains('cancelMacOSCheck'));
    },
  );

  testWidgets(
    'failed cleanup cannot pass; retry removes service without rerunning check',
    (tester) async {
      native(
        (call) async =>
            call.method == 'runMacOSCheck' ? result(cleanup: 'fail') : result(),
      );
      final controller = MacOSCheckController(client);
      addTearDown(controller.dispose);
      await controller.run();
      expect(controller.passed, false);
      expect(controller.cleanupFailed, true);
      await controller.run();
      expect(calls, ['runMacOSCheck']);
      await controller.retryCleanup();
      expect(controller.passed, true);
      expect(calls, ['runMacOSCheck', 'cleanupMacOSCheck']);
    },
  );

  testWidgets(
    'channel errors request cleanup and never copy exception details',
    (tester) async {
      native((call) async {
        if (call.method == 'runMacOSCheck')
          throw PlatformException(
            code: 'failure',
            message: '/Users/private secret',
          );
        return null;
      });
      final controller = MacOSCheckController(client);
      addTearDown(controller.dispose);
      await controller.run();
      expect(calls, ['runMacOSCheck', 'cancelMacOSCheck']);
      expect(controller.passed, false);
      expect(controller.cleanupFailed, true);
      expect(controller.reportText, isNot(contains('secret')));
      expect(controller.reportText, isNot(contains('/Users')));
    },
  );

  testWidgets(
    'missing permission is a useful result rather than a successful check',
    (tester) async {
      native((_) async => result(outcome: 'permission_required'));
      final controller = MacOSCheckController(client);
      addTearDown(controller.dispose);
      await controller.run();
      expect(controller.title, 'Нужно разрешение macOS');
      expect(controller.passed, false);
      expect(controller.cleanupFailed, false);
    },
  );

  testWidgets('closing the app view cancels an outstanding native attempt', (
    tester,
  ) async {
    final pending = Completer<Object?>();
    native(
      (call) async => call.method == 'runMacOSCheck' ? pending.future : null,
    );
    await tester.pumpWidget(
      MaterialApp(home: MacOSCheckDialog(client: client)),
    );
    await tester.pump();
    await tester.pumpWidget(const SizedBox());
    await tester.pump();
    expect(calls, contains('cancelMacOSCheck'));
    pending.complete(result(outcome: 'cancelled'));
    await tester.pump();
  });
}
