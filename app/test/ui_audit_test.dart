import 'dart:io';
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/rendering.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/main.dart';

import 'support/ui_fixtures.dart';
import 'support/visual_test_binding.dart';

const _frame = Key('audit-frame');

void main() {
  NagaVisualTestBinding();
  for (final (width, height, scale) in [
    (1440.0, 900.0, 1.0),
    (1024.0, 560.0, 1.0),
    (390.0, 844.0, 1.0),
    (390.0, 680.0, 2.0),
  ]) {
    testWidgets('all sections at $width x $height text $scale', (tester) async {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = Size(width, height);
      addTearDown(tester.view.reset);
      await tester.runAsync(loadAuditFonts);
      Widget dashboard({bool empty = false}) => RepaintBoundary(
        key: _frame,
        child: MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: nagaTheme(),
          builder: (context, child) => MediaQuery(
            data: MediaQuery.of(context)
                .copyWith(textScaler: TextScaler.linear(scale)),
            child: child!,
          ),
          home: DashboardPage(controlPlane: auditClient(empty: empty)),
        ),
      );
      await tester.pumpWidget(dashboard());
      await tester.pumpAndSettle();
      await tester.runAsync(() async {
        final context = tester.element(find.byKey(_frame));
        for (final asset in [
          'naga-mark-red',
          'mascot-cutout',
          'mascot-welcome',
          for (final mood in NagaMascotMood.values) 'states/${mood.name}',
        ]) {
          await precacheImage(AssetImage('assets/brand/$asset.png'), context);
        }
      });
      await tester.pump();

      Future<void> record(String name) async {
        expect(tester.takeException(), isNull, reason: name);
        final root = Platform.environment['NAGA_AUDIT_SHOTS'];
        if (root == null) return;
        final boundary = tester.renderObject<RenderRepaintBoundary>(
          find.byKey(_frame),
        );
        final path = '$root/$name-${width.toInt()}-${scale.toInt()}x.png';
        await tester.runAsync(() async {
          final img = await boundary.toImage(pixelRatio: 1);
          final data = await img.toByteData(format: ui.ImageByteFormat.png);
          await File(path).parent.create(recursive: true);
          await File(path).writeAsBytes(data!.buffer.asUint8List());
          img.dispose();
        });
      }

      Future<void> navigate(String label) async {
        final tooltip = find.byTooltip(label);
        final target = tooltip.evaluate().isNotEmpty
            ? tooltip.first
            : find.text(label).first;
        await tester.ensureVisible(target);
        await tester.tap(target);
        await tester.pumpAndSettle();
      }

      await record('home');
      final serverPicker = find.byKey(const Key('naga-server-picker'));
      if (width >= 1440 && scale == 1) {
        final connection = tester.getRect(
          find.byKey(const Key('naga-connection-hero')),
        );
        final server = tester.getRect(serverPicker);
        final subscription = tester.getRect(
          find.byKey(const Key('naga-home-subscription')),
        );
        expect(server.top, closeTo(connection.top, 1));
        expect(subscription.bottom, closeTo(connection.bottom, 1));
      }
      if (width == 390 && scale == 1) {
        // Сервер доступен на первом экране телефона, над нижней навигацией.
        expect(
          tester.getBottomLeft(serverPicker).dy,
          lessThanOrEqualTo(tester.getTopLeft(find.byType(NavigationBar)).dy),
        );
      }
      await tester.ensureVisible(serverPicker);
      await tester.tap(serverPicker);
      await tester.pumpAndSettle();
      expect(find.byType(NagaNodesView), findsOneWidget);
      Navigator.of(tester.element(find.byType(NagaNodesView))).pop();
      await tester.pumpAndSettle();
      for (final (name, label) in [
        ('profiles', 'Профили'),
        ('nodes', 'Серверы'),
        ('statistics', 'Статистика'),
        ('settings', 'Настройки'),
      ]) {
        await navigate(label);
        await record(name);
        if (name == 'statistics') {
          // Missing session timestamps must not contradict connected runtime.
          expect(find.text('Не подключено'), findsNothing);
          expect(find.text('Время сессии недоступно'), findsOneWidget);
        }
      }
      await navigate('Какие приложения через VPN');
      await record('routing');
      await navigate('Добавить приложение');
      await record('add-application');
      await navigate('Отмена');
      await navigate('Настройки');
      await navigate('Как выбирать сервер');
      await record('server-choice');
      await navigate('Настройки');
      await navigate('Диагностика');
      await record('diagnostics');
      await navigate('Настройки');
      await navigate('Помощь');
      await record('help');
      await navigate('Профили');
      await navigate('Импорт по ссылке');
      await record('import');
      tester.view.viewInsets = const FakeViewPadding(bottom: 280);
      await tester.pumpAndSettle();
      await record('import-keyboard');
      tester.view.resetViewInsets();
      // Closing through the route also exercises the bottom sheet on compact.
      Navigator.of(tester.element(find.byType(TextField).first)).pop();
      await tester.pumpAndSettle();
      await tester.pumpWidget(
        auditShell(
          const NagaShareProfileDialog(
            profileName: 'Naga Network',
            url: 'https://example.invalid/sub/synthetic-audit.json',
          ),
          scale: scale,
        ),
      );
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull, reason: 'QR dialog');
      await tester.pumpWidget(dashboard(empty: true));
      await tester.pumpAndSettle();
      await record('welcome');
      await navigate('Профили');
      await record('profiles-empty');
      await tester.pumpWidget(const SizedBox.shrink());
      await tester.pumpAndSettle();
    });
  }
}
