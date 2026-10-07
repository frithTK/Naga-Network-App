part of '../main.dart';

class NagaHomeView extends StatelessWidget {
  const NagaHomeView({
    super.key,
    required this.status,
    required this.profileName,
    required this.profileProvider,
    required this.selectedNode,
    required this.runtime,
    required this.trafficSamples,
    required this.preview,
    required this.hasProfile,
    required this.isImporting,
    required this.onImport,
    required this.onPaste,
    required this.onOpenServerPicker,
    required this.onOpenDiagnostics,
    required this.onToggle,
    required this.trafficMode,
    required this.onSelectTrafficMode,
    this.trafficModeError,
    this.controlPlaneReachable,
    this.nodes = const [],
    this.networkClass = 'unknown',
  });

  final ConnectionStatus status;
  final String profileName;
  final String profileProvider;
  final String selectedNode;
  final RuntimeSnapshot? runtime;
  final List<TrafficSample> trafficSamples;
  final ProfilePreview? preview;
  final bool hasProfile;
  final bool isImporting;
  final VoidCallback onImport;
  final VoidCallback onPaste;
  final VoidCallback onOpenServerPicker;
  final VoidCallback onOpenDiagnostics;
  final VoidCallback onToggle;
  final String trafficMode;
  final ValueChanged<String> onSelectTrafficMode;
  final String? trafficModeError;
  final bool? controlPlaneReachable;
  final List<UnifiedNode> nodes;
  final String networkClass;

  @override
  Widget build(BuildContext context) {
    final offline = controlPlaneReachable == false;
    final connected = !offline && status == ConnectionStatus.connected;
    final busy =
        status == ConnectionStatus.connecting || runtime?.status == 'stopping';
    final expired =
        preview?.expireUtc != null &&
        !DateTime.now().isBefore(preview!.expireUtc!);
    final canStart = (preview?.canConnect ?? hasProfile) && !expired;
    final restriction = !connected && !busy && !canStart
        ? expired
              ? 'Обновите подписку, чтобы подключиться'
              : 'Проверьте срок и лимит подписки'
        : null;
    final toggle = !offline && !busy && (connected || canStart)
        ? onToggle
        : null;
    final statusText = offline
        ? 'Статус неизвестен'
        : runtime?.status == 'stopping'
        ? 'Отключаемся…'
        : restriction != null
        ? expired
              ? 'Подписка истекла'
              : 'Подключение недоступно'
        : switch (status) {
            ConnectionStatus.connected => 'VPN подключён',
            ConnectionStatus.connecting => 'Подключаемся…',
            ConnectionStatus.error => 'Не удалось подключиться',
            ConnectionStatus.disconnected => 'Готов к подключению',
          };
    final statusColor = offline || restriction != null
        ? _warning
        : connected
        ? _success
        : status == ConnectionStatus.error
        ? _error
        : _muted;
    final hasTraffic = connected && runtime?.trafficAvailable == true;
    final subscriptionValue = expired
        ? 'Истёк'
        : preview == null
        ? 'Нет данных'
        : preview!.unlimited
        ? 'Безлимит'
        : _formatBytes(preview!.totalBytes);
    final subscriptionDetail = expired
        ? 'Обновите подписку у провайдера'
        : preview == null
        ? 'Сведения о подписке недоступны'
        : preview!.expireUtc == null
        ? 'Дата окончания не указана'
        : 'До ${_formatDate(preview!.expireUtc!)}';

    return LayoutBuilder(
      builder: (context, constraints) {
        final wide =
            constraints.maxWidth >= 760 &&
            MediaQuery.textScalerOf(context).scale(14) <= 21;
        final serverTitle = connected && runtime?.activeNode?.isNotEmpty == true
            ? runtime!.activeNode!
            : selectedNode;
        final hero = _ConnectionHero(
          status: status,
          statusText: statusText,
          statusColor: statusColor,
          connected: connected,
          offline: offline,
          busy: busy,
          compact: !wide,
          errorMessage: _friendlyRuntimeError(runtime?.error),
          restriction: restriction,
          failoverMessage: nagaVisibleFailover(
            runtime?.failoverMessage,
            serverTitle,
          ),
          onToggle: toggle,
          onChooseServer: onOpenServerPicker,
          onOpenDiagnostics: onOpenDiagnostics,
          trafficMode: trafficMode,
          onSelectTrafficMode: onSelectTrafficMode,
          trafficModeError: trafficModeError,
        );
        final server = _ServerCard(
          selectedNode: serverTitle,
          activeLatencyMs: nagaHomeLatencyMs(
            runtimeLatencyMs: runtime?.activeLatencyMs,
            selectedNode: selectedNode,
            activeNode: runtime?.activeNode,
            nodes: nodes,
            networkClass: networkClass,
          ),
          connected: connected,
          compact: !wide,
          onChange: onOpenServerPicker,
        );
        final subscription = _HomeSubscription(
          profileName: profileName,
          value: subscriptionValue,
          detail: subscriptionDetail,
          expired: expired,
        );
        return Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const _DashboardHeading(),
            SizedBox(height: wide ? 32 : 24),
            if (offline) ...[
              _ControlPlaneOfflineCard(onDiagnostics: onOpenDiagnostics),
              if (hasProfile) const SizedBox(height: 16),
            ],
            if (!hasProfile && !offline)
              _WelcomeCard(onImport: onImport, onPaste: onPaste)
            else if (hasProfile) ...[
              if (wide)
                IntrinsicHeight(
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Expanded(flex: 6, child: hero),
                      const SizedBox(width: 16),
                      Expanded(
                        flex: 5,
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          children: [
                            Expanded(child: server),
                            const SizedBox(height: 16),
                            Card(
                              key: const Key('naga-home-subscription'),
                              child: Padding(
                                padding: const EdgeInsets.all(24),
                                child: subscription,
                              ),
                            ),
                          ],
                        ),
                      ),
                    ],
                  ),
                )
              else ...[
                hero,
                const SizedBox(height: 16),
                server,
              ],
              const SizedBox(height: 16),
              _HomeTraffic(
                available: hasTraffic,
                connected: connected,
                offline: offline,
                runtime: runtime,
                wide: wide,
              ),
              if (!wide) ...[
                const SizedBox(height: 16),
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(20),
                    child: subscription,
                  ),
                ),
              ],
            ],
          ],
        );
      },
    );
  }
}

class _ControlPlaneOfflineCard extends StatelessWidget {
  const _ControlPlaneOfflineCard({required this.onDiagnostics});
  final VoidCallback onDiagnostics;

  @override
  Widget build(BuildContext context) => Card(
    child: Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Icon(Icons.cloud_off_rounded, color: _warning, size: 22),
              SizedBox(width: 12),
              Expanded(
                child: Text(
                  'Нет связи с сервисом Naga',
                  style: TextStyle(fontWeight: FontWeight.w600, color: _text),
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          const Text(
            'Сейчас не удаётся проверить состояние VPN. Соединение могло остаться активным. Попробуйте перезапустить приложение или откройте диагностику.',
            style: TextStyle(color: _muted, height: 1.5),
          ),
          const SizedBox(height: 8),
          TextButton.icon(
            onPressed: onDiagnostics,
            icon: const Icon(Icons.health_and_safety_outlined, size: 18),
            label: const Text('Открыть диагностику'),
          ),
        ],
      ),
    ),
  );
}

class _WelcomeCard extends StatelessWidget {
  const _WelcomeCard({required this.onImport, required this.onPaste});
  final VoidCallback onImport, onPaste;

  @override
  Widget build(BuildContext context) => Card(
    child: _NagaBackdrop(
      imageKey: const Key('naga-welcome-backdrop'),
      mood: NagaMascotMood.welcome,
      child: Padding(
        padding: const EdgeInsets.all(28),
        child: Align(
          alignment: Alignment.centerLeft,
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 520),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const SizedBox(height: 24),
                const Text(
                  'Подключите свой VPN',
                  style: TextStyle(
                    color: _text,
                    fontSize: 28,
                    height: 1.2,
                    fontWeight: FontWeight.w500,
                    letterSpacing: -0.6,
                  ),
                ),
                const SizedBox(height: 16),
                const Text(
                  'Добавьте ссылку на подписку или файл конфига. Naga проверит профиль и подготовит подключение.',
                  style: TextStyle(color: _muted, fontSize: 14, height: 1.6),
                ),
                const SizedBox(height: 28),
                FilledButton.icon(
                  onPressed: onImport,
                  icon: const Icon(Icons.add_link_rounded, size: 18),
                  label: const Text('Добавить профиль'),
                ),
                const SizedBox(height: 8),
                TextButton.icon(
                  onPressed: onPaste,
                  icon: const Icon(Icons.content_paste_rounded, size: 18),
                  label: const Text('Вставить из буфера'),
                ),
                const SizedBox(height: 24),
              ],
            ),
          ),
        ),
      ),
    ),
  );
}

class _ConnectionHero extends StatelessWidget {
  const _ConnectionHero({
    this.restriction,
    required this.status,
    required this.statusText,
    required this.statusColor,
    required this.connected,
    required this.offline,
    required this.busy,
    required this.compact,
    required this.errorMessage,
    required this.onToggle,
    required this.onChooseServer,
    required this.onOpenDiagnostics,
    required this.trafficMode,
    required this.onSelectTrafficMode,
    this.trafficModeError,
    this.failoverMessage,
  });

  final ConnectionStatus status;
  final String statusText;
  final String? restriction;
  final Color statusColor;
  final bool connected, offline, busy, compact;
  final String? errorMessage, trafficModeError, failoverMessage;
  final VoidCallback? onToggle;
  final VoidCallback onChooseServer, onOpenDiagnostics;
  final String trafficMode;
  final ValueChanged<String> onSelectTrafficMode;

  NagaMascotMood get _mascotMood => offline
      ? NagaMascotMood.offline
      : switch (status) {
          ConnectionStatus.connected => NagaMascotMood.connected,
          ConnectionStatus.connecting => NagaMascotMood.connecting,
          ConnectionStatus.error => NagaMascotMood.error,
          ConnectionStatus.disconnected => NagaMascotMood.idle,
        };

  @override
  Widget build(BuildContext context) {
    final effectiveStatus = offline ? ConnectionStatus.disconnected : status;
    final action = offline
        ? TextButton(
            onPressed: onOpenDiagnostics,
            child: const Text('Диагностика'),
          )
        : busy
        ? TextButton(onPressed: null, child: Text(statusText))
        : TextButton(
            style: TextButton.styleFrom(
              foregroundColor: connected ? _muted : _text,
              minimumSize: const Size(120, 48),
            ),
            onPressed: onToggle,
            child: Text(
              connected
                  ? 'Отключиться'
                  : status == ConnectionStatus.error
                  ? 'Повторить'
                  : 'Подключить',
            ),
          );
    return Card(
      key: const Key('naga-connection-hero'),
      child: _NagaBackdrop(
        imageKey: const Key('naga-home-backdrop'),
        mood: _mascotMood,
        child: Padding(
          padding: EdgeInsets.all(compact ? 20 : 28),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Semantics(
                liveRegion: true,
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.start,
                  children: [
                    Container(
                      width: 7,
                      height: 7,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: statusColor,
                      ),
                    ),
                    const SizedBox(width: 10),
                    Flexible(
                      child: Text(
                        statusText,
                        textAlign: TextAlign.left,
                        style: TextStyle(
                          color: _text,
                          fontSize: compact ? 24 : 26,
                          height: 1.2,
                          fontWeight: FontWeight.w500,
                          letterSpacing: -0.9,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(height: 8),
              Text(
                offline
                    ? 'Проверим подключение, когда сервис станет доступен'
                    : connected
                    ? trafficMode == 'olcrtc_tun'
                          ? 'Трафик идёт через туннель olcRTC'
                          : trafficMode == 'system_proxy'
                          ? 'Для приложений, использующих системный прокси'
                          : 'Трафик направляется по вашим правилам'
                    : busy
                    ? 'Это может занять несколько секунд'
                    : restriction ??
                          (status == ConnectionStatus.error
                              ? 'Можно повторить попытку или сменить сервер'
                              : 'Нажмите кнопку подключения'),
                textAlign: TextAlign.left,
                style: const TextStyle(
                  color: _muted,
                  fontSize: 13,
                  height: 1.5,
                ),
              ),
              const SizedBox(height: 24),
              Center(
                child: _PowerButton(
                  status: effectiveStatus,
                  onPressed: onToggle,
                  diameter: compact ? 152 : 184,
                  label: offline
                      ? 'Состояние VPN неизвестно'
                      : busy
                      ? statusText
                      : null,
                ),
              ),
              const SizedBox(height: 8),
              Center(child: action),
              if (!offline && status == ConnectionStatus.error) ...[
                if (errorMessage?.isNotEmpty == true) ...[
                  const SizedBox(height: 12),
                  Text(
                    errorMessage!,
                    textAlign: TextAlign.center,
                    style: const TextStyle(
                      color: _muted,
                      fontSize: 12,
                      height: 1.5,
                    ),
                  ),
                ],
                Wrap(
                  alignment: WrapAlignment.center,
                  spacing: 8,
                  children: [
                    TextButton(
                      onPressed: onChooseServer,
                      child: const Text('Другой сервер'),
                    ),
                    TextButton(
                      onPressed: onOpenDiagnostics,
                      child: const Text('Диагностика'),
                    ),
                  ],
                ),
              ],
              if (connected && failoverMessage?.isNotEmpty == true) ...[
                const SizedBox(height: 12),
                Text(
                  failoverMessage!,
                  textAlign: TextAlign.center,
                  style: const TextStyle(
                    color: _warning,
                    fontSize: 12,
                    height: 1.5,
                  ),
                ),
              ],
              Center(
                child: TextButton.icon(
                  key: const Key('naga-mode-settings'),
                  onPressed: offline || busy ? null : () => _showMode(context),
                  icon: const Icon(Icons.tune_rounded, size: 15),
                  style: TextButton.styleFrom(foregroundColor: _muted),
                  label: Text(
                    trafficMode == 'olcrtc_tun'
                        ? 'Туннель olcRTC'
                        : trafficMode == 'system_proxy'
                        ? 'Прокси приложений'
                        : 'VPN устройства',
                    style: const TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w400,
                    ),
                  ),
                ),
              ),
              if (trafficModeError != null)
                Text(
                  trafficModeError!,
                  style: const TextStyle(color: _warning, fontSize: 12),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _showMode(BuildContext context) => showDialog<void>(
    context: context,
    builder: (dialogContext) => AlertDialog(
      title: const Text('Как направлять трафик'),
      content: SingleChildScrollView(
        child: SizedBox(
          width: 420,
          child: _TrafficModeSelector(
            value: trafficMode,
            onChanged: (value) {
              Navigator.of(dialogContext).pop();
              onSelectTrafficMode(value);
            },
            errorMessage: trafficModeError,
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(dialogContext).pop(),
          child: const Text('Готово'),
        ),
      ],
    ),
  );
}

class _PowerButton extends StatefulWidget {
  const _PowerButton({
    required this.status,
    required this.onPressed,
    this.diameter = 180,
    this.label,
  });
  final ConnectionStatus status;
  final VoidCallback? onPressed;
  final double diameter;
  final String? label;

  @override
  State<_PowerButton> createState() => _PowerButtonState();
}

class _PowerButtonState extends State<_PowerButton>
    with SingleTickerProviderStateMixin {
  late final AnimationController _pulse;
  bool _focused = false;
  bool _reduceMotion = false;

  @override
  void initState() {
    super.initState();
    _pulse = AnimationController(
      vsync: this,
      duration: NagaMotion.connectPulse,
    );
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduceMotion = MediaQuery.disableAnimationsOf(context);
    _syncPulse();
  }

  @override
  void didUpdateWidget(covariant _PowerButton oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.status != widget.status) _syncPulse();
  }

  void _syncPulse() {
    if (widget.status == ConnectionStatus.connecting &&
        !_reduceMotion &&
        TickerMode.valuesOf(context).enabled) {
      if (!_pulse.isAnimating) _pulse.repeat(reverse: true);
    } else {
      _pulse.stop();
      _pulse.value = 0;
    }
  }

  @override
  void dispose() {
    _pulse.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final connected = widget.status == ConnectionStatus.connected;
    final connecting = widget.status == ConnectionStatus.connecting;
    final enabled = widget.onPressed != null && !connecting;
    final ring = widget.status == ConnectionStatus.error ? _error : _accent;
    final label =
        widget.label ??
        (connecting
            ? 'Подключение'
            : connected
            ? 'Отключить VPN'
            : 'Подключить VPN');
    return RepaintBoundary(
      child: AnimatedBuilder(
        animation: _pulse,
        builder: (context, _) => Container(
          width: widget.diameter,
          height: widget.diameter,
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(
              color: _focused ? _text : ring.withValues(alpha: 0.10),
              width: _focused ? 2 : 1,
            ),
            boxShadow: [
              BoxShadow(
                color: ring.withValues(
                  alpha: connected || connecting
                      ? 0.12 + _pulse.value * 0.04
                      : enabled
                      ? 0.07
                      : 0,
                ),
                blurRadius: 72,
                spreadRadius: 8,
              ),
            ],
          ),
          child: Semantics(
            key: const Key('naga-power-control'),
            label: label,
            button: true,
            enabled: enabled,
            onTap: enabled ? widget.onPressed : null,
            child: ExcludeSemantics(
              child: Container(
                padding: const EdgeInsets.all(3),
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  gradient: LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: enabled || connecting
                        ? [
                            Color.lerp(ring, _text, 0.25)!,
                            ring,
                            Color.lerp(ring, _background, 0.5)!,
                          ]
                        : [_lineStrong, _surface],
                  ),
                  boxShadow: [
                    if (enabled || connecting)
                      BoxShadow(
                        color: ring.withValues(alpha: 0.18),
                        blurRadius: 18,
                      ),
                  ],
                ),
                child: Material(
                  color: _surface,
                  shape: const CircleBorder(),
                  clipBehavior: Clip.antiAlias,
                  child: Ink(
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      border: Border.all(color: _background, width: 3),
                      gradient: const LinearGradient(
                        begin: Alignment.topLeft,
                        end: Alignment.bottomRight,
                        colors: [_surfaceRaised, _surfaceLow],
                      ),
                    ),
                    child: InkWell(
                      key: const Key('naga-power-target'),
                      customBorder: const CircleBorder(),
                      onTap: enabled ? widget.onPressed : null,
                      onFocusChange: (value) =>
                          setState(() => _focused = value),
                      hoverColor: _accent.withValues(alpha: 0.08),
                      focusColor: _accent.withValues(alpha: 0.10),
                      child: Center(
                        child: connecting && !_reduceMotion
                            ? const SizedBox(
                                width: 36,
                                height: 36,
                                child: CircularProgressIndicator(
                                  strokeWidth: 2,
                                  color: _accent,
                                ),
                              )
                            : Icon(
                                connecting
                                    ? Icons.more_horiz_rounded
                                    : Icons.power_settings_new_rounded,
                                size: widget.diameter * 0.26,
                                color: enabled ? _text : _muted,
                              ),
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _ServerCard extends StatelessWidget {
  const _ServerCard({
    required this.selectedNode,
    required this.activeLatencyMs,
    required this.connected,
    required this.compact,
    required this.onChange,
  });
  final String selectedNode;
  final int? activeLatencyMs;
  final bool connected, compact;
  final VoidCallback onChange;

  @override
  Widget build(BuildContext context) {
    final automatic =
        selectedNode.isEmpty || selectedNode.toLowerCase() == 'auto';
    final latency = activeLatencyMs != null && activeLatencyMs! > 0
        ? '${activeLatencyMs!} мс'
        : null;
    final title = automatic ? 'Автоматический выбор' : selectedNode;
    return Semantics(
      button: true,
      child: Tooltip(
        message: 'Выбрать сервер',
        child: Material(
          color: _surfaceLow,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(NagaRadius.card),
            side: BorderSide(color: _line.withValues(alpha: 0.65)),
          ),
          clipBehavior: Clip.antiAlias,
          child: InkWell(
            key: const Key('naga-server-picker'),
            onTap: onChange,
            hoverColor: _surface,
            child: Padding(
              padding: EdgeInsets.all(compact ? 18 : 28),
              child: compact
                  ? Row(
                      children: [
                        const _SoftIcon(Icons.public_rounded, accent: true),
                        const SizedBox(width: 14),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              const Text(
                                'Выбрать сервер',
                                style: TextStyle(color: _muted, fontSize: 12),
                              ),
                              const SizedBox(height: 5),
                              Text(
                                title,
                                style: const TextStyle(
                                  color: _text,
                                  fontSize: 16,
                                  height: 1.4,
                                  fontWeight: FontWeight.w500,
                                ),
                              ),
                              if (latency != null) ...[
                                const SizedBox(height: 4),
                                Text(
                                  latency,
                                  style: const TextStyle(
                                    color: _muted,
                                    fontSize: 12,
                                  ),
                                ),
                              ],
                            ],
                          ),
                        ),
                        const SizedBox(width: 10),
                        const Icon(
                          Icons.chevron_right_rounded,
                          color: _muted,
                          size: 20,
                        ),
                      ],
                    )
                  : Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Row(
                          children: [
                            const _SoftIcon(
                              Icons.public_rounded,
                              accent: true,
                              size: 44,
                            ),
                            const SizedBox(width: 14),
                            Expanded(
                              child: Text(
                                connected
                                    ? 'Текущий сервер'
                                    : 'Выбранный сервер',
                                style: const TextStyle(
                                  color: _muted,
                                  fontSize: 13,
                                ),
                              ),
                            ),
                            if (latency != null)
                              Text(
                                latency,
                                style: const TextStyle(
                                  color: _muted,
                                  fontSize: 13,
                                ),
                              ),
                          ],
                        ),
                        const SizedBox(height: 28),
                        Text(
                          title,
                          style: const TextStyle(
                            color: _text,
                            fontSize: 28,
                            height: 1.25,
                            letterSpacing: -0.7,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                        if (automatic) ...[
                          const SizedBox(height: 10),
                          const Text(
                            'Naga найдёт подходящий сервер',
                            style: TextStyle(
                              color: _muted,
                              fontSize: 13,
                              height: 1.5,
                            ),
                          ),
                        ],
                        const SizedBox(height: 24),
                        const Spacer(),
                        const Divider(height: 1),
                        const SizedBox(height: 18),
                        const Row(
                          children: [
                            Expanded(
                              child: Text(
                                'Выбрать сервер',
                                style: TextStyle(
                                  color: _text,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w500,
                                ),
                              ),
                            ),
                            Icon(
                              Icons.arrow_forward_rounded,
                              color: _accent,
                              size: 20,
                            ),
                          ],
                        ),
                      ],
                    ),
            ),
          ),
        ),
      ),
    );
  }
}

class _HomeSubscription extends StatelessWidget {
  const _HomeSubscription({
    required this.profileName,
    required this.value,
    required this.detail,
    required this.expired,
  });
  final String profileName, value, detail;
  final bool expired;

  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.symmetric(horizontal: 4),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Wrap(
          spacing: 12,
          runSpacing: 8,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            const Text(
              'Подписка',
              style: TextStyle(color: _muted, fontSize: 13),
            ),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 4),
              decoration: BoxDecoration(
                color: _surface,
                borderRadius: BorderRadius.circular(6),
              ),
              child: Text(
                value,
                style: TextStyle(
                  color: expired ? _warning : _text,
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        Text(
          profileName,
          style: const TextStyle(
            color: _text,
            fontSize: 15,
            fontWeight: FontWeight.w500,
          ),
        ),
        const SizedBox(height: 5),
        Text(
          detail,
          style: const TextStyle(color: _muted, fontSize: 12, height: 1.5),
        ),
      ],
    ),
  );
}

class _HomeTraffic extends StatelessWidget {
  const _HomeTraffic({
    required this.available,
    required this.connected,
    required this.offline,
    required this.runtime,
    required this.wide,
  });
  final bool available, connected, offline, wide;
  final RuntimeSnapshot? runtime;

  @override
  Widget build(BuildContext context) {
    final cells = [
      _TrafficValue(
        icon: Icons.south_rounded,
        label: 'Скорость загрузки',
        value: available ? _formatRate(runtime!.downloadRateBytes) : '—',
      ),
      _TrafficValue(
        icon: Icons.north_rounded,
        label: 'Скорость отправки',
        value: available ? _formatRate(runtime!.uploadRateBytes) : '—',
      ),
      if (wide)
        _TrafficValue(
          icon: Icons.data_usage_rounded,
          label: 'Получено за сессию',
          value: available ? _formatBytes(runtime!.downloadBytes) : '—',
        ),
    ];
    return Container(
      padding: EdgeInsets.all(wide ? 24 : 18),
      decoration: BoxDecoration(
        color: _surfaceLow,
        borderRadius: BorderRadius.circular(NagaRadius.card),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (MediaQuery.textScalerOf(context).scale(14) > 21)
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (var i = 0; i < cells.length; i++) ...[
                  if (i > 0) const SizedBox(height: 20),
                  cells[i],
                ],
              ],
            )
          else
            Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (var i = 0; i < cells.length; i++) ...[
                  if (i > 0) const SizedBox(width: 16),
                  Expanded(child: cells[i]),
                ],
              ],
            ),
          if (!available) ...[
            const SizedBox(height: 14),
            Text(
              offline
                  ? 'Нет связи с локальным сервисом'
                  : connected
                  ? 'Сервис пока не передаёт данные о трафике'
                  : 'Показатели появятся после подключения',
              style: const TextStyle(color: _muted, fontSize: 12, height: 1.5),
            ),
          ],
        ],
      ),
    );
  }
}

class _TrafficValue extends StatelessWidget {
  const _TrafficValue({
    required this.icon,
    required this.label,
    required this.value,
  });
  final IconData icon;
  final String label, value;

  @override
  Widget build(BuildContext context) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    children: [
      Row(
        children: [
          Icon(icon, size: 14, color: _muted),
          const SizedBox(width: 6),
          Expanded(
            child: Text(
              label,
              style: const TextStyle(color: _muted, fontSize: 12),
            ),
          ),
        ],
      ),
      const SizedBox(height: 10),
      Text(
        value,
        style: const TextStyle(
          color: _text,
          fontSize: 24,
          height: 1.2,
          fontWeight: FontWeight.w500,
          letterSpacing: -0.7,
          fontFeatures: [FontFeature.tabularFigures()],
        ),
      ),
    ],
  );
}

class _HomeMetricCard extends StatelessWidget {
  const _HomeMetricCard({
    required this.icon,
    required this.label,
    required this.value,
    required this.detail,
  });
  final IconData icon;
  final String label, value, detail;

  @override
  Widget build(BuildContext context) {
    final content = Padding(
      padding: const EdgeInsets.all(22),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, color: _muted, size: 17),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  label,
                  style: const TextStyle(color: _muted, fontSize: 12),
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          Text(
            value,
            style: const TextStyle(
              color: _text,
              fontSize: 22,
              height: 1.2,
              letterSpacing: -0.6,
              fontWeight: FontWeight.w600,
              fontFeatures: [FontFeature.tabularFigures()],
            ),
          ),
          const SizedBox(height: 8),
          Text(
            detail,
            style: const TextStyle(color: _muted, fontSize: 12, height: 1.5),
          ),
        ],
      ),
    );
    return Card(child: content);
  }
}
