import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/macos_backend.dart';

void main() {
  Map<String, dynamic> ready() => {
    'type': 'ready',
    'protocol_version': 1,
    'endpoint': 'http://127.0.0.1:49167',
    'token': 'a' * 64,
    'version': '1.2.9',
    'data_dir':
        '/Users/Имя с пробелами/Library/Application Support/Naga Network',
  };

  test(
    'accepts a private child endpoint and rejects incompatible handshakes',
    () {
      expect(
        MacOSBackendConnection.fromMap(
          ready(),
          expectedVersion: '1.2.9',
        ).endpoint,
        'http://127.0.0.1:49167',
      );
      for (final override in [
        {'type': 'error'},
        {'protocol_version': 2},
        {'version': '1.2.8'},
        {'token': 'short'},
        {'data_dir': 'relative/path'},
        {'endpoint': 'https://untrusted.example:443'},
        {'endpoint': 'http://127.0.0.1'},
        {'endpoint': 'http://user@127.0.0.1:49167'},
        {'endpoint': 'http://127.0.0.1:49167/token?secret=value'},
      ]) {
        expect(
          () => MacOSBackendConnection.fromMap({
            ...ready(),
            ...override,
          }, expectedVersion: '1.2.9'),
          throwsA(
            isA<FormatException>().having(
              (e) => e.toString(),
              'no token in error',
              isNot(contains('a' * 64)),
            ),
          ),
        );
      }
    },
  );

  testWidgets(
    'dashboard waits for readiness and supports a failed-start retry',
    (tester) async {
      final pending = Completer<Map<dynamic, dynamic>?>();
      var calls = 0;
      var dashboardBuilds = 0;
      tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        macOSNativeChannel,
        (call) async {
          expect(call.method, 'startBackend');
          calls++;
          if (calls == 1) return pending.future;
          throw PlatformException(
            code: 'start_failed',
            message: 'Повторный запуск отклонён',
          );
        },
      );
      addTearDown(
        () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
          macOSNativeChannel,
          null,
        ),
      );
      await tester.pumpWidget(
        MacOSBackendGate(
          builder: (_) {
            dashboardBuilds++;
            return const SizedBox();
          },
        ),
      );
      expect(dashboardBuilds, 0);
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
      pending.completeError(
        PlatformException(code: 'start_failed', message: 'Backend отсутствует'),
      );
      await tester.pumpAndSettle();
      expect(find.text('Backend отсутствует'), findsOneWidget);
      expect(dashboardBuilds, 0);
      await tester.tap(find.text('Повторить запуск'));
      await tester.pumpAndSettle();
      expect(calls, 2);
      expect(dashboardBuilds, 0);
      expect(find.text('Повторный запуск отклонён'), findsOneWidget);
      await tester.pumpWidget(const SizedBox());
    },
  );
}
