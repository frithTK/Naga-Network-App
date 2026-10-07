import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/control_plane_client.dart';
import 'package:naga_network/main.dart';

import 'support/ui_fixtures.dart' as audit;
import 'support/visual_test_binding.dart';

const _snapshotKey = ValueKey('naga-snapshot');

bool get _exportEnabled => Platform.environment['NAGA_UI_SNAPSHOTS'] == '1';

Directory get _outDir {
  final fromEnv = Platform.environment['NAGA_UI_SNAPSHOTS_DIR'];
  if (fromEnv != null && fromEnv.isNotEmpty) {
    return Directory(fromEnv);
  }
  return Directory('${Directory.systemTemp.path}/naga-ui-snapshots');
}

void main() {
  NagaVisualTestBinding();

  if (!_exportEnabled) {
    test('UI snapshots export is opt-in via NAGA_UI_SNAPSHOTS=1', () {});
    return;
  }

  testWidgets('export designer UI snapshots', (tester) async {
    await tester.runAsync(_loadUiFonts);
    final out = _outDir..createSync(recursive: true);
    goldenFileComparator = _WriteAllGoldens(
      Uri.file('${out.absolute.path}/shots.txt'),
    );
    final written = <_Shot>[];

    Future<void> shot(String id, String title, String form) async {
      await expectLater(find.byKey(_snapshotKey), matchesGoldenFile('$id.png'));
      written.add(_Shot(id: id, title: title, form: form));
    }

    await _prepareView(tester, const Size(1440, 900));
    await tester.pumpWidget(_boundary(_desktopShell(home: _welcomeHome())));
    await _pumpFrames(tester);
    await shot('live-welcome-desktop', 'Онбординг / без профиля', 'desktop');

    await tester.pumpWidget(_boundary(_desktopShell(home: _offlineHome())));
    await _pumpFrames(tester);
    await shot('live-offline-desktop', 'Control-plane не запущен', 'desktop');

    await tester.pumpWidget(_boundary(_desktopShell(home: _connectedHome())));
    await _pumpFrames(tester);
    await shot('live-connected-desktop', 'Главная · подключено', 'desktop');

    await tester.pumpWidget(
      _boundary(_desktopShell(home: _disconnectedHome())),
    );
    await _pumpFrames(tester);
    await shot(
      'live-disconnected-desktop',
      'Главная · профиль, VPN выкл',
      'desktop',
    );

    await tester.pumpWidget(_boundary(_desktopShell(home: _errorHome())));
    await _pumpFrames(tester);
    await shot('live-error-desktop', 'Главная · ошибка', 'desktop');

    await _prepareView(tester, const Size(390, 900));
    await tester.pumpWidget(_boundary(_mobileShell(home: _welcomeHome())));
    await _pumpFrames(tester);
    await shot('live-welcome-mobile', 'Онбординг / без профиля', 'mobile');

    await tester.pumpWidget(_boundary(_mobileShell(home: _connectedHome())));
    await _pumpFrames(tester);
    await shot('live-connected-mobile', 'Главная · подключено', 'mobile');

    await tester.pumpWidget(_boundary(_mobileShell(home: _disconnectedHome())));
    await _pumpFrames(tester);
    await shot(
      'live-disconnected-mobile',
      'Главная · профиль, VPN выкл',
      'mobile',
    );

    await tester.pumpWidget(_boundary(_mobileShell(home: _errorHome())));
    await _pumpFrames(tester);
    await shot('live-error-mobile', 'Главная · ошибка', 'mobile');

    for (final size in [const Size(1440, 900), const Size(390, 900)]) {
      await _prepareView(tester, size);
      final form = size.width > 600 ? 'desktop' : 'mobile';
      for (final (id, title, home) in [
        (
          'starting',
          'Подключение',
          audit.auditHome(
            status: ConnectionStatus.connecting,
            runtimeStatus: 'starting',
          ),
        ),
        (
          'stopping',
          'Отключение',
          audit.auditHome(
            status: ConnectionStatus.connecting,
            runtimeStatus: 'stopping',
          ),
        ),
        (
          'expired',
          'Подписка истекла',
          audit.auditHome(
            expired: true,
            status: ConnectionStatus.disconnected,
            runtimeStatus: 'stopped',
          ),
        ),
        (
          'unknown',
          'Нет сведений о подписке',
          audit.auditHome(unknownSubscription: true),
        ),
      ]) {
        await tester.pumpWidget(
          _boundary(
            size.width > 600
                ? _desktopShell(home: home)
                : _mobileShell(home: home),
          ),
        );
        await _pumpFrames(tester);
        await shot('live-$id-$form', title, form);
      }
    }

    await _prepareView(tester, const Size(1440, 2800));
    await tester.pumpWidget(
      _boundary(
        MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: nagaTheme(),
          home: const DesignGalleryPage(),
        ),
      ),
    );
    await _pumpFrames(tester);
    await shot(
      'lab-variants-desktop',
      'UI Lab: пять концепций (ПК + телефон на карточке)',
      'desktop',
    );

    await _prepareView(tester, const Size(390, 3600));
    await tester.pumpWidget(
      _boundary(
        MaterialApp(
          debugShowCheckedModeBanner: false,
          theme: nagaTheme(),
          home: const DesignGalleryPage(),
        ),
      ),
    );
    await _pumpFrames(tester);
    await shot(
      'lab-variants-mobile',
      'UI Lab: пять концепций, узкая ширина',
      'mobile',
    );

    File('${out.path}/index.html').writeAsStringSync(_indexHtml(written));
  }, timeout: const Timeout(Duration(minutes: 3)));
}

class _Shot {
  const _Shot({required this.id, required this.title, required this.form});

  final String id;
  final String title;
  final String form;
}

Widget _boundary(Widget child) {
  return RepaintBoundary(key: _snapshotKey, child: child);
}

Future<void> _prepareView(WidgetTester tester, Size logical) async {
  tester.view.devicePixelRatio = 1.25;
  tester.view.physicalSize = Size(logical.width * 1.25, logical.height * 1.25);
  addTearDown(tester.view.reset);
}

Future<void> _pumpFrames(WidgetTester tester) async {
  await tester.pump();
  await tester.runAsync(() async {
    final context = tester.element(find.byKey(_snapshotKey));
    for (final asset in [
      'assets/brand/naga-mark-red.png',
      'assets/brand/mascot-cutout.png',
      'assets/brand/mascot-welcome.png',
      for (final mood in NagaMascotMood.values)
        'assets/brand/states/${mood.name}.png',
    ]) {
      await precacheImage(AssetImage(asset), context);
    }
  });
  await tester.pump();
  // Сцены сменяются в одном дереве, поэтому кросс-фейд позы маскота нужно
  // доиграть до конца — иначе слепок покажет позу предыдущего состояния.
  await tester.pump(const Duration(milliseconds: 400));
}

class _WriteAllGoldens extends LocalFileComparator {
  _WriteAllGoldens(super.testFile);

  @override
  Future<bool> compare(Uint8List imageBytes, Uri golden) async {
    await update(golden, imageBytes);
    return true;
  }
}

Future<void> _loadUiFonts() async {
  final inter = FontLoader('Inter');
  var hasFace = false;
  for (final path in [
    'assets/fonts/Inter-Regular.ttf',
    'assets/fonts/Inter-Medium.ttf',
    'assets/fonts/Inter-SemiBold.ttf',
    'assets/fonts/Inter-Bold.ttf',
  ]) {
    if (File(path).existsSync()) {
      inter.addFont(_fontBytes(path));
      hasFace = true;
    }
  }
  if (hasFace) {
    await inter.load();
  }

  final flutterRoot = Platform.environment['FLUTTER_ROOT'];
  if (flutterRoot == null || flutterRoot.isEmpty) {
    return;
  }
  final icons =
      '$flutterRoot/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf';
  if (File(icons).existsSync()) {
    await (FontLoader('MaterialIcons')..addFont(_fontBytes(icons))).load();
  }
}

Future<ByteData> _fontBytes(String path) async {
  final bytes = await File(path).readAsBytes();
  return ByteData.view(Uint8List.fromList(bytes).buffer);
}

const _connectedRuntime = RuntimeSnapshot(
  status: 'connected',
  engine: 'sing-box',
  profileId: 'profile',
  error: null,
  uploadBytes: 12 * 1024 * 1024,
  downloadBytes: 180 * 1024 * 1024,
  uploadRateBytes: 32 * 1024,
  downloadRateBytes: 420 * 1024,
  sessionStartedAt: null,
  lastTrafficUpdate: null,
  trafficAvailable: true,
  activeNode: 'Estonia (EE) Hysteria2',
  activeCountry: 'Estonia',
  activeProtocol: 'Hysteria2',
  activeLatencyMs: 32,
  activeNodeStatus: 'healthy',
);

const _preview = ProfilePreview(
  profileId: 'profile',
  profileTitle: 'Naga Network',
  providerName: 'NagaVPN',
  engine: 'sing-box',
  uploadBytes: 0,
  downloadBytes: 0,
  totalBytes: 0,
  unlimited: true,
  expireUtc: null,
  updateIntervalSeconds: 86400,
  canConnect: true,
);

NagaHomeView _welcomeHome() {
  return NagaHomeView(
    status: ConnectionStatus.disconnected,
    profileName: 'Профиль не импортирован',
    profileProvider: '',
    selectedNode: 'Auto',
    runtime: null,
    trafficSamples: const [],
    preview: null,
    hasProfile: false,
    isImporting: false,
    onImport: () {},
    onPaste: () {},
    onOpenServerPicker: () {},
    onOpenDiagnostics: () {},
    onToggle: () {},
    trafficMode: 'tun',
    onSelectTrafficMode: (_) {},
    controlPlaneReachable: true,
  );
}

NagaHomeView _offlineHome() {
  return NagaHomeView(
    status: ConnectionStatus.disconnected,
    profileName: 'Профиль не импортирован',
    profileProvider: '',
    selectedNode: 'Auto',
    runtime: null,
    trafficSamples: const [],
    preview: null,
    hasProfile: false,
    isImporting: false,
    onImport: () {},
    onPaste: () {},
    onOpenServerPicker: () {},
    onOpenDiagnostics: () {},
    onToggle: () {},
    trafficMode: 'tun',
    onSelectTrafficMode: (_) {},
    controlPlaneReachable: false,
  );
}

NagaHomeView _connectedHome() {
  return NagaHomeView(
    status: ConnectionStatus.connected,
    profileName: 'Naga Network',
    profileProvider: 'NagaVPN',
    selectedNode: 'Auto',
    runtime: _connectedRuntime,
    trafficSamples: const [],
    preview: _preview,
    hasProfile: true,
    isImporting: false,
    onImport: () {},
    onPaste: () {},
    onOpenServerPicker: () {},
    onOpenDiagnostics: () {},
    onToggle: () {},
    trafficMode: 'tun',
    onSelectTrafficMode: (_) {},
    controlPlaneReachable: true,
  );
}

NagaHomeView _disconnectedHome() {
  return NagaHomeView(
    status: ConnectionStatus.disconnected,
    profileName: 'Naga Network',
    profileProvider: 'NagaVPN',
    selectedNode: 'Auto',
    runtime: null,
    trafficSamples: const [],
    preview: _preview,
    hasProfile: true,
    isImporting: false,
    onImport: () {},
    onPaste: () {},
    onOpenServerPicker: () {},
    onOpenDiagnostics: () {},
    onToggle: () {},
    trafficMode: 'tun',
    onSelectTrafficMode: (_) {},
    controlPlaneReachable: true,
  );
}

NagaHomeView _errorHome() {
  return NagaHomeView(
    status: ConnectionStatus.error,
    profileName: 'Naga Network',
    profileProvider: 'NagaVPN',
    selectedNode: 'Auto',
    runtime: const RuntimeSnapshot(
      status: 'error',
      engine: 'sing-box',
      profileId: 'profile',
      error: 'Ни один VPN-узел не прошёл проверку доступности',
      uploadBytes: 0,
      downloadBytes: 0,
      uploadRateBytes: 0,
      downloadRateBytes: 0,
      sessionStartedAt: null,
      lastTrafficUpdate: null,
      trafficAvailable: false,
      activeNode: null,
      activeCountry: null,
      activeProtocol: null,
      activeLatencyMs: null,
      activeNodeStatus: null,
    ),
    trafficSamples: const [],
    preview: _preview,
    hasProfile: true,
    isImporting: false,
    onImport: () {},
    onPaste: () {},
    onOpenServerPicker: () {},
    onOpenDiagnostics: () {},
    onToggle: () {},
    trafficMode: 'tun',
    onSelectTrafficMode: (_) {},
    controlPlaneReachable: true,
  );
}

Widget _desktopShell({required Widget home}) {
  return MaterialApp(
    debugShowCheckedModeBanner: false,
    theme: nagaTheme(),
    home: Scaffold(
      backgroundColor: const Color(0xFF080808),
      body: Column(
        children: [
          const NagaTitleBar(),
          Expanded(
            child: Row(
              children: [
                NagaNavigationRail(
                  selectedIndex: 0,
                  onSelect: (_) {},
                  compact: false,
                  activeProfile: 'Naga Network',
                ),
                Expanded(
                  child: ColoredBox(
                    color: const Color(0xFF080808),
                    child: SingleChildScrollView(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 32,
                        vertical: 28,
                      ),
                      child: ConstrainedBox(
                        constraints: const BoxConstraints(maxWidth: 1120),
                        child: home,
                      ),
                    ),
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

Widget _mobileShell({required Widget home}) {
  return MaterialApp(
    debugShowCheckedModeBanner: false,
    theme: nagaTheme(),
    home: Scaffold(
      backgroundColor: const Color(0xFF080808),
      body: Column(
        children: [
          const NagaTitleBar(),
          Expanded(
            child: SafeArea(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(
                  horizontal: 16,
                  vertical: 24,
                ),
                child: home,
              ),
            ),
          ),
        ],
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: 0,
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.home_outlined),
            selectedIcon: Icon(Icons.home_rounded),
            label: 'Главная',
          ),
          NavigationDestination(
            icon: Icon(Icons.person_outline_rounded),
            selectedIcon: Icon(Icons.person_rounded),
            label: 'Профили',
          ),
          NavigationDestination(
            icon: Icon(Icons.public_outlined),
            selectedIcon: Icon(Icons.public_rounded),
            label: 'Серверы',
          ),
          NavigationDestination(
            icon: Icon(Icons.bar_chart_outlined),
            selectedIcon: Icon(Icons.bar_chart_rounded),
            label: 'Статистика',
          ),
          NavigationDestination(
            icon: Icon(Icons.settings_outlined),
            selectedIcon: Icon(Icons.settings_rounded),
            label: 'Настройки',
          ),
        ],
      ),
    ),
  );
}

String _indexHtml(List<_Shot> shots) {
  final pairs = <String, Map<String, _Shot>>{};
  for (final shot in shots) {
    final key = shot.title;
    pairs.putIfAbsent(key, () => {});
    pairs[key]![shot.form] = shot;
  }

  final cards = StringBuffer();
  for (final entry in pairs.entries) {
    final desktop = entry.value['desktop'];
    final mobile = entry.value['mobile'];
    cards.writeln('''
    <section class="pair">
      <h2>${_esc(entry.key)}</h2>
      <div class="grid">
        ${_figure(desktop, 'ПК · 1440')}
        ${_figure(mobile, 'Телефон · 390')}
      </div>
    </section>''');
  }

  return '''
<!DOCTYPE html>
<html lang="ru">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Naga Network — UI snapshots</title>
  <style>
    :root {
      --bg: #080808;
      --surface: #121316;
      --line: #24262C;
      --text: #FBFBF9;
      --muted: #A6A8B0;
      --accent: #F72F38;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--text);
      font: 14px/1.45 Inter, "Noto Sans", sans-serif;
    }
    header {
      padding: 32px 40px 16px;
      border-bottom: 1px solid var(--line);
    }
    header p { color: var(--muted); max-width: 720px; }
    .accent { color: var(--accent); letter-spacing: 0.16em; font-size: 11px; font-weight: 700; }
    .pair { padding: 28px 40px; border-bottom: 1px solid var(--line); }
    h2 { font-size: 18px; margin: 0 0 16px; }
    .grid {
      display: grid;
      grid-template-columns: minmax(0, 1.6fr) minmax(180px, 0.55fr);
      gap: 20px;
      align-items: start;
    }
    figure {
      margin: 0;
      background: var(--surface);
      border: 1px solid var(--line);
      border-radius: 12px;
      overflow: hidden;
    }
    figure img { display: block; width: 100%; height: auto; }
    figcaption {
      padding: 10px 12px;
      color: var(--muted);
      font-size: 12px;
      border-top: 1px solid var(--line);
    }
    @media (max-width: 900px) {
      header, .pair { padding: 20px 16px; }
      .grid { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <header>
    <div class="accent">NAGA NETWORK · UI SNAPSHOTS</div>
    <h1>Слепки интерфейса для дизайнера</h1>
    <p>
      Текущий Flutter-клиент: desktop 1440 и mobile 390.
      Блок «UI Lab» — пять концепций (Orbit / Signal / Monolith / Route / Pulse),
      на каждой карточке сразу ПК и телефон.
    </p>
  </header>
  ${cards.toString()}
</body>
</html>
''';
}

String _figure(_Shot? shot, String caption) {
  if (shot == null) {
    return '<p class="muted">нет кадра</p>';
  }
  return '''
        <figure>
          <img src="${shot.id}.png" alt="${_esc(shot.title)}">
          <figcaption>${_esc(caption)}</figcaption>
        </figure>''';
}

String _esc(String value) {
  return value
      .replaceAll('&', '&amp;')
      .replaceAll('<', '&lt;')
      .replaceAll('>', '&gt;');
}
