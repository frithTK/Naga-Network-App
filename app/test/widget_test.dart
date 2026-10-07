import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:qr_flutter/qr_flutter.dart';

import 'package:naga_network/control_plane_client.dart';
import 'package:naga_network/main.dart';

void main() {
  test('failover note is hidden when it names another server', () {
    const canada =
        'Переключили на 🇨🇦 Canada (CA) Hysteria2 — предыдущий не ответил';
    expect(nagaVisibleFailover(canada, '🇪🇪 Estonia (EE) Hysteria2'), isNull);
    expect(nagaVisibleFailover(canada, '🇨🇦 Canada (CA) Hysteria2'), canada);
  });

  test('profile card shows when the profile was updated', () {
    final updated = DateTime.utc(2026, 9, 25, 23, 39);
    final imported = DateTime.utc(2026, 9, 1, 12);
    final local = updated.toLocal();
    final day = local.day.toString().padLeft(2, '0');
    final month = local.month.toString().padLeft(2, '0');
    final clock =
        '${local.hour.toString().padLeft(2, '0')}:${local.minute.toString().padLeft(2, '0')}';
    expect(
      nagaProfileUpdatedLabel(updated, imported),
      'Обновлён $day.$month.${local.year}, $clock',
    );
    expect(nagaProfileUpdatedLabel(null, imported), isNotEmpty);
    expect(nagaProfileUpdatedLabel(null, null), isEmpty);
  });

  test('processBaseName understands Windows and Linux paths', () {
    expect(processBaseName(r'C:\Program Files\Telegram.exe'), 'Telegram.exe');
    expect(processBaseName('/usr/bin/firefox'), 'firefox');
    expect(processBaseName('Telegram.exe'), 'Telegram.exe');
  });

  testWidgets('renders the Naga Network dashboard', (tester) async {
    await tester.pumpWidget(const NagaNetworkApp());

    expect(find.byType(NagaHomeView), findsOneWidget);
    expect(find.text('Главная'), findsAtLeastNWidgets(1));
    expect(find.text('Подключите свой VPN'), findsOneWidget);
    expect(find.text('Добавить профиль'), findsOneWidget);
    expect(find.text('Вставить из буфера'), findsOneWidget);
  });

  testWidgets('requires a profile before starting runtime', (tester) async {
    await tester.pumpWidget(const NagaNetworkApp());

    expect(find.text('Подключить'), findsNothing);
    await tester.tap(find.text('Добавить профиль'));
    await tester.pumpAndSettle();
    expect(find.text('Добавить VPN'), findsOneWidget);
  });

  testWidgets('opens the profiles flow from mobile navigation', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(const NagaNetworkApp());

    await tester.tap(find.text('Профили'));
    await tester.pump();

    expect(find.text('Импорт по ссылке'), findsOneWidget);
    expect(find.text('Здесь пока пусто'), findsOneWidget);
  });

  testWidgets('uses a mobile bottom sheet for profile import', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(const NagaNetworkApp());
    await tester.tap(find.text('Добавить профиль').first);
    await tester.pumpAndSettle();

    expect(find.text('Добавить VPN'), findsOneWidget);
    expect(find.text('Вставить из буфера'), findsAtLeastNWidgets(1));
    expect(find.text('Выбрать файл'), findsOneWidget);
    expect(find.text('Продолжить'), findsOneWidget);
  });

  testWidgets('settings controls provide immediate feedback', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(const NagaNetworkApp());

    await tester.tap(find.text('Настройки'));
    await tester.pump();
    expect(find.text('По расписанию подписки'), findsOneWidget);
    expect(find.text('Проверить обновление'), findsOneWidget);
    expect(find.text('Автообновление приложения'), findsOneWidget);

    await tester.ensureVisible(find.byType(SwitchListTile).first);
    await tester.tap(find.byType(SwitchListTile).first);
    await tester.pump();
    expect(find.text('Вручную'), findsOneWidget);
  });

  testWidgets('opens nodes, routing and applications from mobile navigation', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(const NagaNetworkApp());

    await tester.tap(find.text('Серверы'));
    await tester.pump();
    expect(find.text('Серверы'), findsNWidgets(2));

    await tester.tap(find.text('Настройки'));
    await tester.pump();
    await tester.tap(find.text('Какие приложения через VPN'));
    await tester.pump();
    expect(find.text('Локальная сеть напрямую'), findsOneWidget);
    expect(find.text('Российские домены напрямую'), findsOneWidget);
    expect(find.text('Использовать правила VPN-провайдера'), findsOneWidget);
    expect(find.text('Список приложений пуст'), findsOneWidget);

    await tester.tap(find.text('Настройки'));
    await tester.pump();
    await tester.tap(find.text('Как выбирать сервер'));
    await tester.pump();
    expect(find.text('Автоматически'), findsOneWidget);
  });

  testWidgets('design gallery fits mobile preview', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(const MaterialApp(home: DesignGalleryPage()));
    await tester.pumpAndSettle();

    expect(find.text('Пять направлений для Naga Network'), findsOneWidget);
    expect(find.text('Orbit'), findsOneWidget);
    expect(find.text('Pulse'), findsOneWidget);
  });

  testWidgets('home with a profile hides import CTA and profile card', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: NagaHomeView(
            status: ConnectionStatus.connected,
            profileName: 'Naga Network',
            profileProvider: 'NagaVPN',
            selectedNode: 'Auto',
            runtime: const RuntimeSnapshot(
              status: 'connected',
              engine: 'sing-box',
              profileId: 'profile',
              error: null,
              uploadBytes: 0,
              downloadBytes: 0,
              uploadRateBytes: 0,
              downloadRateBytes: 0,
              sessionStartedAt: null,
              lastTrafficUpdate: null,
              trafficAvailable: false,
              activeNode: 'Estonia (EE) Hysteria2',
              activeCountry: 'Estonia',
              activeProtocol: 'Hysteria2',
              activeLatencyMs: 32,
              activeNodeStatus: 'healthy',
              failoverMessage: 'Переключили на Estonia (EE) Hysteria2 — предыдущий не ответил',
            ),
            trafficSamples: const [],
            preview: null,
            hasProfile: true,
            isImporting: false,
            onImport: () {},
            onPaste: () {},
            onOpenServerPicker: () {},
            onOpenDiagnostics: () {},
            onToggle: () {},
            trafficMode: 'tun',
            onSelectTrafficMode: (_) {},
          ),
        ),
      ),
    );

    expect(find.text('Добавить профиль'), findsNothing);
    expect(find.text('Скорость загрузки'), findsOneWidget);
    expect(find.text('Подписка'), findsOneWidget);
    expect(find.textContaining('предыдущий не ответил'), findsOneWidget);
  });

  testWidgets('sidebar help mascot opens help on desktop', (tester) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    var selected = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: Row(
            children: [
              NagaNavigationRail(
                selectedIndex: selected,
                onSelect: (index) => selected = index,
                compact: false,
                activeProfile: 'Naga Network',
              ),
            ],
          ),
        ),
      ),
    );

    expect(find.byKey(const Key('naga-sidebar-mascot')), findsOneWidget);
    await tester.ensureVisible(find.text('Помощь'));
    await tester.tap(find.text('Помощь'));
    await tester.pump();
    expect(selected, 5);
  });

  testWidgets('compact rail keeps help accessible within 72 px', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(800, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: NagaNavigationRail(
            selectedIndex: 0,
            onSelect: _noopSelect,
            compact: true,
          ),
        ),
      ),
    );

    final rail = tester.getSize(find.byType(NagaNavigationRail));
    expect(rail.width, 72);
    expect(find.byTooltip('Помощь'), findsOneWidget);
  });

  testWidgets('settings expose help on compact layouts', (tester) async {
    tester.view.physicalSize = const Size(390, 844);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(const NagaNetworkApp());
    await tester.tap(find.text('Настройки'));
    await tester.pump();
    await tester.scrollUntilVisible(find.text('Ответы и подсказки'), 120);
    await tester.tap(find.text('Помощь').last);
    await tester.pump();
    expect(find.text('VPN не работает?'), findsOneWidget);
  });

  test('groups unified nodes by country without profile ids', () {
    final groups = groupUnifiedNodes(const [
      UnifiedNode(
        id: 'nl',
        profileId: 'secret-profile',
        tag: 'Amsterdam',
        runtimeTag: 'p::nl',
        country: 'Netherlands',
        protocol: 'VLESS',
        latencyMs: 24,
        probeStatus: 'healthy',
      ),
      UnifiedNode(
        id: 'de',
        profileId: 'secret-profile',
        tag: 'Frankfurt',
        runtimeTag: 'p::de',
        country: 'Germany',
        protocol: 'Hysteria2',
      ),
    ]);
    expect(groups.map((group) => group.country).toList(), [
      'Germany',
      'Netherlands',
    ]);
    expect(groups.first.nodes.single.tag, 'Frankfurt');
    expect(nagaNodePingLabel(groups.last.nodes.single), '24 мс');
    expect(nagaNodePingLabel(groups.first.nodes.single), '—');
    expect(
      nagaNodePingLabel(
        const UnifiedNode(
          id: 'fail',
          profileId: 'secret-profile',
          tag: 'Paris',
          runtimeTag: 'p::fail',
          country: 'France',
          protocol: 'TUIC',
          probeStatus: 'failed',
        ),
      ),
      'нет ответа',
    );
    expect(
      nagaHomeLatencyMs(selectedNode: 'Amsterdam', nodes: groups.last.nodes),
      24,
    );
    expect(
      nagaMergeProbedNodes(groups.first.nodes, [
        UnifiedNode(
          id: groups.first.nodes.single.id,
          profileId: groups.first.nodes.single.profileId,
          tag: groups.first.nodes.single.tag,
          runtimeTag: groups.first.nodes.single.runtimeTag,
          country: groups.first.nodes.single.country,
          protocol: groups.first.nodes.single.protocol,
          latencyMs: 55,
          probeStatus: 'healthy',
        ),
      ]).single.latencyMs,
      55,
    );
    expect(
      nagaMergeProbedNodes(
        [
          UnifiedNode(
            id: groups.first.nodes.single.id,
            profileId: groups.first.nodes.single.profileId,
            tag: groups.first.nodes.single.tag,
            runtimeTag: groups.first.nodes.single.runtimeTag,
            country: groups.first.nodes.single.country,
            protocol: groups.first.nodes.single.protocol,
            probeStatus: 'failed',
          ),
        ],
        [
          UnifiedNode(
            id: groups.first.nodes.single.id,
            profileId: groups.first.nodes.single.profileId,
            tag: groups.first.nodes.single.tag,
            runtimeTag: groups.first.nodes.single.runtimeTag,
            country: groups.first.nodes.single.country,
            protocol: groups.first.nodes.single.protocol,
            latencyMs: 55,
            probeStatus: 'healthy',
          ),
        ],
      ).single.probeStatus,
      'failed',
    );
  });

  test('recommended unified nodes take three best and keep TUIC last', () {
    const nodes = [
      UnifiedNode(
        id: 'tuic',
        profileId: 'secret-profile',
        tag: 'Paris',
        runtimeTag: 'p::tuic',
        country: 'France',
        protocol: 'TUIC',
        latencyMs: 5,
      ),
      UnifiedNode(
        id: 'slow',
        profileId: 'secret-profile',
        tag: 'Berlin',
        runtimeTag: 'p::slow',
        country: 'Germany',
        protocol: 'Hysteria2',
        latencyMs: 90,
      ),
      UnifiedNode(
        id: 'fast',
        profileId: 'secret-profile',
        tag: 'Amsterdam',
        runtimeTag: 'p::fast',
        country: 'Netherlands',
        protocol: 'VLESS',
        latencyMs: 18,
      ),
      UnifiedNode(
        id: 'mid',
        profileId: 'secret-profile',
        tag: 'Frankfurt',
        runtimeTag: 'p::mid',
        country: 'Germany',
        protocol: 'VLESS',
        latencyMs: 32,
      ),
      UnifiedNode(
        id: 'awg',
        profileId: 'secret-profile',
        tag: 'AmneziaWG',
        runtimeTag: 'p::awg',
        country: 'Netherlands',
        protocol: 'AmneziaWG',
        latencyMs: 40,
      ),
    ];
    final recommended = recommendedUnifiedNodes(nodes);
    expect(recommended.map((node) => node.id).toList(), ['fast', 'mid', 'awg']);
    expect(nagaProtocolPriority('TUIC'), 2);
    expect(nagaProtocolPriority('VLESS', network: 'cellular'), 0);
  });

  testWidgets('grouped node list keeps auto on top and hides profile ids', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: NagaNodesView(
              nodes: const [
                UnifiedNode(
                  id: 'de',
                  profileId: 'secret-profile',
                  tag: 'Frankfurt',
                  runtimeTag: 'p::de',
                  country: 'Germany',
                  protocol: 'VLESS',
                ),
              ],
              connected: false,
              onSelect: (_) {},
              onSelectAuto: () {},
              onProbe: () async => const <UnifiedNode>[],
            ),
          ),
        ),
      ),
    );

    expect(find.text('Автоматический выбор'), findsOneWidget);
    expect(
      find.text('Проверит 3 наиболее подходящих сервера и включит лучший.'),
      findsOneWidget,
    );
    expect(find.text('Рекомендуемые'), findsOneWidget);
    expect(find.text('Frankfurt · VLESS'), findsOneWidget);
    expect(find.text('secret-profile'), findsNothing);
    expect(find.text('Включи VPN, чтобы измерить задержку.'), findsNothing);
    expect(find.text('Проверить'), findsOneWidget);
    expect(find.text('—'), findsOneWidget);
  });

  testWidgets('olcRTC variants use the subscription name', (tester) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);
    Future<void> pump(List<UnifiedNode> nodes) async {
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: NagaNodesView(
              nodes: nodes,
              connected: false,
              onSelect: (_) {},
              onSelectAuto: () {},
              onProbe: () async => const <UnifiedNode>[],
            ),
          ),
        ),
      );
    }

    await pump(const [
      UnifiedNode(
        id: 'ee',
        profileId: 'p',
        tag: 'Tallinn',
        runtimeTag: 'p::ee',
        country: 'Estonia',
        protocol: 'VLESS',
      ),
      UnifiedNode(
        id: 'awg',
        profileId: 'p',
        tag: 'AmneziaWG',
        runtimeTag: 'p::awg',
        country: 'Netherlands',
        protocol: 'AmneziaWG',
      ),
      UnifiedNode(
        id: 'olc-pl',
        profileId: 'p',
        tag: '🇵🇱 Poland (PL) Jitsi',
        runtimeTag: 'p::pl-jitsi',
        country: 'Poland',
        protocol: 'olcRTC',
      ),
      UnifiedNode(
        id: 'olc-cz',
        profileId: 'p',
        tag: '🇨🇿 Czechia (CZ) Jitsi',
        runtimeTag: 'p::cz-jitsi',
        country: 'Czechia',
        protocol: 'olcRTC',
      ),
    ]);
    expect(find.text('Tallinn · VLESS'), findsOneWidget);
    expect(find.text('AmneziaWG · AmneziaWG'), findsOneWidget);
    expect(find.text('🇵🇱 Poland (PL) Jitsi · olcRTC'), findsOneWidget);
    expect(find.text('🇨🇿 Czechia (CZ) Jitsi · olcRTC'), findsOneWidget);
    expect(find.textContaining('olcRTC (beta)'), findsNothing);

    await pump(const [
      UnifiedNode(
        id: 'ee',
        profileId: 'p',
        tag: 'Tallinn',
        runtimeTag: 'p::ee',
        country: 'Estonia',
        protocol: 'VLESS',
      ),
    ]);
    expect(find.textContaining('olcRTC'), findsNothing);
  });

  testWidgets('recommended nodes are excluded from country groups', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: NagaNodesView(
              nodes: const [
                UnifiedNode(
                  id: 'fast',
                  profileId: 'secret-profile',
                  tag: 'Amsterdam',
                  runtimeTag: 'p::fast',
                  country: 'Netherlands',
                  protocol: 'VLESS',
                  latencyMs: 10,
                ),
                UnifiedNode(
                  id: 'mid',
                  profileId: 'secret-profile',
                  tag: 'Frankfurt',
                  runtimeTag: 'p::mid',
                  country: 'Germany',
                  protocol: 'Hysteria2',
                  latencyMs: 20,
                ),
                UnifiedNode(
                  id: 'awg',
                  profileId: 'secret-profile',
                  tag: 'AmneziaWG',
                  runtimeTag: 'p::awg',
                  country: 'Netherlands',
                  protocol: 'AmneziaWG',
                  latencyMs: 30,
                ),
                UnifiedNode(
                  id: 'slow',
                  profileId: 'secret-profile',
                  tag: 'Berlin',
                  runtimeTag: 'p::slow',
                  country: 'Germany',
                  protocol: 'Trojan',
                  latencyMs: 80,
                ),
              ],
              connected: true,
              onSelect: (_) {},
              onSelectAuto: () {},
            ),
          ),
        ),
      ),
    );

    expect(find.text('Рекомендуемые'), findsOneWidget);
    expect(find.text('Amsterdam · VLESS'), findsOneWidget);
    expect(find.text('Frankfurt · Hysteria2'), findsOneWidget);
    expect(find.text('AmneziaWG · AmneziaWG'), findsOneWidget);
    expect(find.text('Berlin · Trojan'), findsOneWidget);
    expect(find.text('Germany'), findsOneWidget);
    expect(find.text('Netherlands'), findsNothing);
  });

  testWidgets(
    'expired subscription is visible and connect stays available to parent',
    (tester) async {
      tester.view.physicalSize = const Size(1280, 800);
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.reset);

      var taps = 0;
      final expireUtc = DateTime.utc(2020, 1, 1);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: NagaHomeView(
              status: ConnectionStatus.disconnected,
              profileName: 'Naga Network',
              profileProvider: 'NagaVPN',
              selectedNode: 'Auto',
              runtime: null,
              trafficSamples: const [],
              preview: ProfilePreview(
                profileTitle: 'Naga Network',
                providerName: 'NagaVPN',
                engine: 'sing-box',
                uploadBytes: 0,
                downloadBytes: 0,
                totalBytes: 0,
                unlimited: true,
                expireUtc: expireUtc,
                updateIntervalSeconds: 86400,
                canConnect: false,
              ),
              hasProfile: true,
              isImporting: false,
              onImport: () {},
              onPaste: () {},
              onOpenServerPicker: () {},
              onOpenDiagnostics: () {},
              onToggle: () => taps++,
              trafficMode: 'tun',
              onSelectTrafficMode: (_) {},
            ),
          ),
        ),
      );

      expect(find.text('Истёк'), findsOneWidget);
      expect(find.text('Подключить'), findsOneWidget);
      await tester.tap(find.text('Подключить'));
      await tester.pump();
      expect(taps, 0);
    },
  );

  testWidgets('connect button is disabled while connecting', (tester) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    var taps = 0;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: NagaHomeView(
            status: ConnectionStatus.connecting,
            profileName: 'Naga Network',
            profileProvider: 'NagaVPN',
            selectedNode: 'Auto',
            runtime: null,
            trafficSamples: const [],
            preview: null,
            hasProfile: true,
            isImporting: false,
            onImport: () {},
            onPaste: () {},
            onOpenServerPicker: () {},
            onOpenDiagnostics: () {},
            onToggle: () => taps++,
            trafficMode: 'tun',
            onSelectTrafficMode: (_) {},
          ),
        ),
      ),
    );

    final buttons = tester
        .widgetList<TextButton>(
          find.widgetWithText(TextButton, 'Подключаемся…'),
        )
        .toList();
    expect(buttons, isNotEmpty);
    expect(buttons.every((button) => button.onPressed == null), isTrue);
    expect(taps, 0);
  });

  testWidgets('home shows control-plane offline banner instead of welcome', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: NagaHomeView(
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
          ),
        ),
      ),
    );

    expect(find.text('Нет связи с сервисом Naga'), findsOneWidget);
    expect(find.text('Подключите свой VPN'), findsNothing);
    expect(find.text('Добавить профиль'), findsNothing);
  });

  testWidgets('diagnostics status card shows active node and failover', (
    tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: DiagnosticsStatusCard(
            serviceHealthy: true,
            runtime: RuntimeSnapshot(
              status: 'connected',
              engine: 'sing-box',
              profileId: 'profile',
              error: null,
              uploadBytes: 0,
              downloadBytes: 0,
              uploadRateBytes: 0,
              downloadRateBytes: 0,
              sessionStartedAt: null,
              lastTrafficUpdate: null,
              trafficAvailable: false,
              activeNode: 'Estonia (EE) Hysteria2',
              activeCountry: 'Estonia',
              activeProtocol: 'Hysteria2',
              activeLatencyMs: 32,
              activeNodeStatus: 'healthy',
              failoverMessage: 'Переключили на Estonia (EE) Hysteria2 — предыдущий не ответил',
            ),
          ),
        ),
      ),
    );

    expect(find.text('Estonia (EE) Hysteria2'), findsWidgets);
    expect(find.text('healthy'), findsOneWidget);
    expect(find.textContaining('предыдущий не ответил'), findsOneWidget);
  });

  test('runtimeBlocksProfileDelete covers live VPN statuses', () {
    expect(runtimeBlocksProfileDelete('starting'), isTrue);
    expect(runtimeBlocksProfileDelete('connected'), isTrue);
    expect(runtimeBlocksProfileDelete('stopping'), isTrue);
    expect(runtimeBlocksProfileDelete(null), isFalse);
    expect(runtimeBlocksProfileDelete('stopped'), isFalse);
    expect(runtimeBlocksProfileDelete('failed'), isFalse);
  });

  testWidgets('Windows chrome shows titlebar buttons at any width', (
    tester,
  ) async {
    Future<void> pumpAt(Size size) async {
      tester.view.physicalSize = size;
      tester.view.devicePixelRatio = 1;
      await tester.pumpWidget(const NagaNetworkApp());
      await tester.pump();
    }

    addTearDown(tester.view.reset);

    await pumpAt(const Size(1440, 900));
    if (nagaUsesWindowsChrome) {
      expect(find.byKey(const Key('naga-window-titlebar')), findsOneWidget);
      expect(find.byKey(const Key('naga-window-minimize')), findsOneWidget);
      expect(find.byKey(const Key('naga-window-maximize')), findsOneWidget);
      expect(find.byKey(const Key('naga-window-close')), findsOneWidget);
    } else {
      expect(find.byKey(const Key('naga-window-titlebar')), findsNothing);
    }

    await pumpAt(const Size(NagaBreakpoints.minWindowWidth, 844));
    if (nagaUsesWindowsChrome) {
      expect(find.byKey(const Key('naga-window-titlebar')), findsOneWidget);
      expect(find.byKey(const Key('naga-window-close')), findsOneWidget);
    } else {
      expect(find.byKey(const Key('naga-window-titlebar')), findsNothing);
    }
  });

  testWidgets('share profile dialog shows QR and copies the URL', (
    tester,
  ) async {
    const url = 'https://example.invalid/sub/secret-token.json';
    String? copied;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(
      SystemChannels.platform,
      (call) async {
        if (call.method == 'Clipboard.setData') {
          final args = call.arguments;
          if (args is Map && args['text'] is String) {
            copied = args['text'] as String;
          }
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

    tester.view.physicalSize = const Size(1280, 800);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.reset);

    await tester.pumpWidget(
      MaterialApp(
        theme: nagaTheme(),
        home: const Scaffold(
          body: NagaShareProfileDialog(profileName: 'Naga Network', url: url),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Скопировать профиль'), findsOneWidget);
    expect(find.text('Naga Network'), findsOneWidget);
    expect(find.byType(QrImageView), findsOneWidget);
    expect(
      find.text('Ссылка — секрет, как пароль. Не отправляй её в чат.'),
      findsOneWidget,
    );
    expect(find.text(url), findsOneWidget);

    final copyButton = find.widgetWithText(FilledButton, 'Скопировать');
    await tester.ensureVisible(copyButton);
    await tester.tap(copyButton);
    await tester.pumpAndSettle();
    expect(copied, url);
    expect(find.text('Ссылка скопирована'), findsOneWidget);
  });
}

void _noopSelect(int index) {}
