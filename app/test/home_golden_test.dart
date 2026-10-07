import 'dart:io' show Platform;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/main.dart';

import 'support/ui_fixtures.dart';
import 'support/visual_test_binding.dart';

// Эталоны Flutter 3.47.1 / Linux. Изменять только явным --update-goldens
// после визуальной проверки; обычный запуск сравнивает, а не перезаписывает.
void main() {
  NagaVisualTestBinding();
  for (final (name, size, offline) in [
    ('connected-desktop', const Size(1120, 800), false),
    ('connected-compact', const Size(390, 900), false),
    ('offline-compact', const Size(390, 900), true),
  ]) {
    testWidgets('home golden $name', (tester) async {
      tester.view.devicePixelRatio = 1;
      tester.view.physicalSize = size;
      addTearDown(tester.view.reset);
      await tester.runAsync(loadAuditFonts);
      await tester.pumpWidget(
        RepaintBoundary(
          key: const Key('home-golden'),
          child: auditShell(auditHome(offline: offline), reducedMotion: true),
        ),
      );
      await tester.runAsync(() async {
        final context = tester.element(find.byKey(const Key('home-golden')));
        for (final asset in [
          'naga-mark-red',
          'mascot-cutout',
          for (final mood in NagaMascotMood.values) 'states/${mood.name}',
        ]) {
          await precacheImage(AssetImage('assets/brand/$asset.png'), context);
        }
      });
      await tester.pumpAndSettle();
      await expectLater(
        find.byKey(const Key('home-golden')),
        matchesGoldenFile('goldens/home-$name.png'),
      );
    }, skip: !Platform.isLinux); // эталоны Linux; на Windows шум ClearType 1–4%
  }
}
