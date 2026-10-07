import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/main.dart';

import 'support/ui_fixtures.dart';

void main() {
  testWidgets('profiles endpoint alone cannot establish VPN status', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        theme: nagaTheme(),
        home: DashboardPage(
          controlPlane: auditClient(runtimeOffline: () => true),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('Статус неизвестен'), findsOneWidget);
    expect(find.text('VPN отключён'), findsNothing);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('failed poll clears confirmed status and recovers on next poll', (
    tester,
  ) async {
    var offline = false;
    await tester.pumpWidget(
      MaterialApp(
        theme: nagaTheme(),
        home: DashboardPage(
          controlPlane: auditClient(runtimeOffline: () => offline),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('VPN подключён'), findsOneWidget);
    offline = true;
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.text('VPN подключён'), findsOneWidget);
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.text('Статус неизвестен'), findsOneWidget);
    expect(find.text('VPN подключён'), findsNothing);
    offline = false;
    await tester.pump(const Duration(seconds: 2));
    await tester.pumpAndSettle();
    expect(find.text('VPN подключён'), findsOneWidget);
    await tester.pumpWidget(const SizedBox.shrink());
  });

  testWidgets('clipboard failure is explained without false success', (
    tester,
  ) async {
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          throw PlatformException(code: 'unavailable');
        }
        return null;
      },
    );
    addTearDown(
      () => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
        SystemChannels.platform,
        null,
      ),
    );
    await tester.pumpWidget(
      auditShell(
        const NagaShareProfileDialog(
          profileName: 'Naga Network',
          url: 'https://example.invalid/synthetic',
        ),
      ),
    );
    await tester.ensureVisible(find.text('Скопировать'));
    await tester.tap(find.text('Скопировать'));
    await tester.pumpAndSettle();
    expect(find.text('Ссылка скопирована'), findsNothing);
    expect(
      find.text('Не удалось скопировать ссылку. Попробуйте ещё раз.'),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });

  testWidgets(
    'offline never presents cached connection as current protection',
    (tester) async {
      var calls = 0;
      await tester.pumpWidget(
        auditShell(auditHome(offline: true, onToggle: () => calls++)),
      );
      expect(find.text('Статус неизвестен'), findsOneWidget);
      expect(find.text('VPN подключён'), findsNothing);
      expect(find.text('Соединение активно'), findsNothing);
      expect(find.text('Отключиться'), findsNothing);
      await tester.tap(find.byKey(const Key('naga-power-target')));
      expect(calls, 0);
    },
  );

  testWidgets('unknown subscription is not presented as unlimited', (
    tester,
  ) async {
    await tester.pumpWidget(auditShell(auditHome(unknownSubscription: true)));
    expect(find.text('Безлимит'), findsNothing);
    expect(find.text('Сведения о подписке недоступны'), findsOneWidget);
  });

  testWidgets('background portrait leaves connection controls accessible', (
    tester,
  ) async {
    var calls = 0;
    tester.view.devicePixelRatio = 1;
    tester.view.physicalSize = const Size(390, 844);
    addTearDown(tester.view.reset);
    await tester.pumpWidget(auditShell(auditHome(onToggle: () => calls++)));
    await tester.pumpAndSettle();
    expect(find.byKey(const Key('naga-home-backdrop')), findsOneWidget);
    expect(find.text('Приятного сёрфинга!'), findsNothing);
    await tester.tap(find.byKey(const Key('naga-power-target')));
    await tester.tap(find.text('Отключиться'));
    expect(calls, 2);
    expect(tester.takeException(), isNull);
  });

  testWidgets('missing traffic does not contradict a connected VPN', (
    tester,
  ) async {
    await tester.pumpWidget(auditShell(auditHome(trafficAvailable: false)));
    expect(find.text('VPN подключён'), findsOneWidget);
    expect(
      find.text('Сервис пока не передаёт данные о трафике'),
      findsOneWidget,
    );
    expect(find.text('Показатели появятся после подключения'), findsNothing);
    expect(find.text('420.0 КБ/с'), findsNothing);
  });

  testWidgets('expired profile cannot reconnect after a previous failure', (
    tester,
  ) async {
    var calls = 0;
    await tester.pumpWidget(
      auditShell(
        auditHome(
          expired: true,
          status: ConnectionStatus.error,
          runtimeStatus: 'failed',
          onToggle: () => calls++,
        ),
      ),
    );
    await tester.tap(find.text('Повторить'));
    expect(calls, 0);
    expect(find.text('Подписка истекла'), findsOneWidget);
    expect(find.text('Готов к подключению'), findsNothing);
  });

  testWidgets('power button can be activated with a keyboard', (tester) async {
    var calls = 0;
    await tester.pumpWidget(auditShell(auditHome(onToggle: () => calls++)));
    await tester.sendKeyEvent(LogicalKeyboardKey.tab);
    await tester.pump();
    await tester.sendKeyEvent(LogicalKeyboardKey.enter);
    await tester.pump();
    expect(calls, 1);
  });

  testWidgets('reduced motion stops continuous connection animations', (
    tester,
  ) async {
    await tester.pumpWidget(
      auditShell(
        auditHome(
          status: ConnectionStatus.connecting,
          runtimeStatus: 'starting',
        ),
        reducedMotion: true,
      ),
    );
    await tester.pumpAndSettle();
    expect(find.byType(CircularProgressIndicator), findsNothing);
    expect(tester.binding.hasScheduledFrame, isFalse);
  });

  testWidgets('stopping is distinct from connecting', (tester) async {
    await tester.pumpWidget(
      auditShell(
        auditHome(
          status: ConnectionStatus.connecting,
          runtimeStatus: 'stopping',
        ),
        reducedMotion: true,
      ),
    );
    expect(find.text('Отключаемся…'), findsNWidgets(2));
    expect(find.text('Подключаемся…'), findsNothing);
  });

  testWidgets('traffic dialog selects system proxy', (tester) async {
    var selected = 'tun';
    await tester.pumpWidget(
      auditShell(auditHome(onSelectTrafficMode: (value) => selected = value)),
    );
    await tester.tap(find.byKey(const Key('naga-mode-settings')));
    await tester.pumpAndSettle();
    expect(find.text('Как направлять трафик'), findsOneWidget);
    await tester.tap(find.text('Системный прокси'));
    await tester.pumpAndSettle();
    expect(selected, 'system_proxy');
  });

  testWidgets('tun dialog keeps leftover errors without a proxy prefix', (
    tester,
  ) async {
    await tester.pumpWidget(
      auditShell(
        auditHome(
          trafficModeError: 'Не удалось связаться с локальным control-plane.',
        ),
      ),
    );
    await tester.tap(find.byKey(const Key('naga-mode-settings')));
    await tester.pumpAndSettle();
    expect(
      find.text('Не удалось связаться с локальным control-plane.'),
      findsWidgets,
    );
    expect(
      find.textContaining('Не удалось включить системный прокси'),
      findsNothing,
    );
  });

  testWidgets('system proxy does not claim protection of all device traffic', (
    tester,
  ) async {
    await tester.pumpWidget(auditShell(auditHome(trafficMode: 'system_proxy')));
    expect(
      find.text('Для приложений, использующих системный прокси'),
      findsOneWidget,
    );
    expect(find.textContaining('Весь системный трафик'), findsNothing);
  });

  for (final width in [320.0, 390.0, 768.0, 1024.0, 1440.0]) {
    for (final scale in [1.0, 2.0]) {
      testWidgets('home and welcome fit width $width text $scale', (
        tester,
      ) async {
        tester.view.devicePixelRatio = 1;
        tester.view.physicalSize = Size(width, 680);
        addTearDown(tester.view.reset);
        for (final home in [auditHome(), auditHome(hasProfile: false)]) {
          await tester.pumpWidget(auditShell(home, scale: scale));
          await tester.pumpAndSettle();
          expect(tester.takeException(), isNull);
        }
      });
    }
  }

  test('filled action text and secondary text meet contrast targets', () {
    final theme = nagaTheme();
    double ratio(Color a, Color b) {
      final x = a.computeLuminance(), y = b.computeLuminance();
      return x > y ? (x + .05) / (y + .05) : (y + .05) / (x + .05);
    }

    final style = theme.filledButtonTheme.style!;
    expect(
      ratio(
        style.foregroundColor!.resolve({})!,
        style.backgroundColor!.resolve({})!,
      ),
      greaterThanOrEqualTo(4.5),
    );
    expect(
      ratio(theme.textTheme.bodyMedium!.color!, theme.colorScheme.surface),
      greaterThanOrEqualTo(4.5),
    );
    // Даже белый участок фонового портрета при заданной прозрачности
    // сохраняет контраст вторичного текста; затемняющий градиент лишь улучшает его.
    final brightestBackdrop = Color.alphaBlend(
      Colors.white.withValues(alpha: 0.16),
      const Color(0xFF111215),
    );
    expect(
      ratio(theme.textTheme.bodyMedium!.color!, brightestBackdrop),
      greaterThanOrEqualTo(4.5),
    );
  });
}
