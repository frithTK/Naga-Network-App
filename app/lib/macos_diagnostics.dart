import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'app_update.dart';
import 'control_plane_client.dart';
import 'macos_backend.dart';

/// Owns one diagnostic attempt; never exports backend credentials or raw errors.
class MacOSCheckController extends ChangeNotifier {
  MacOSCheckController(this.client, {this.channel = macOSNativeChannel});

  final ControlPlaneClient client;
  final MethodChannel channel;
  bool busy = false;
  bool _disposed = false;
  bool _polling = false;
  bool _cancelRequested = false;
  Timer? _timer;
  String stage = 'starting';
  Map<String, dynamic>? report;
  Map<String, dynamic> _health = {};

  bool get cleanupFailed => (report?['cleanup'] as Map?)?['status'] == 'fail';
  bool get passed =>
      report?['outcome'] == 'pass' &&
      _health['status'] == 'pass' &&
      (report?['cleanup'] as Map?)?['status'] == 'pass';

  String get title {
    if (busy) return 'Проверяем Mac';
    if (cleanupFailed) return 'Нужно завершить проверку';
    if (passed) return 'Проверка пройдена';
    return switch (report?['outcome']) {
      'install_required' => 'Перенесите Naga в Программы',
      'permission_required' => 'Нужно разрешение macOS',
      'cancelled' => 'Проверка отменена',
      _ => 'Проверка не пройдена',
    };
  }

  String get message {
    if (busy) {
      return switch (stage) {
        'waiting_permission' => 'В открывшихся настройках macOS разрешите Naga работать в фоне. Проверка продолжится автоматически.',
        'cleaning' => 'Завершаем проверку и удаляем временную службу…',
        _ => 'Проверяем приложение и доступ к системной службе…',
      };
    }
    if (cleanupFailed) {
      return 'Не удалось удалить временную службу. Нажмите «Завершить проверку», чтобы повторить удаление.';
    }
    if (passed) {
      return 'Приложение и доступ к системной службе работают. Временная служба удалена. VPN пока не проверялся.';
    }
    return switch (report?['outcome']) {
      'install_required' => 'Перетащите Naga Network из DMG в Программы и откройте установленную копию.',
      'permission_required' => 'Разрешение не получено. Можно повторить проверку и разрешить Naga работать в фоне в настройках macOS.',
      'cancelled' => 'Проверка остановлена. Её можно повторить позже.',
      _ => 'Скопируйте отчёт и передайте его разработчику. Профили, токены и личные пути в него не включаются.',
    };
  }

  String get reportText => const JsonEncoder.withIndent('  ').convert({
    ...?report,
    'local_backend': _health,
    'overall_result': passed ? 'pass' : 'incomplete',
    'vpn_tested': false,
  });

  void _changed() {
    if (!_disposed) notifyListeners();
  }

  Future<void> _poll() async {
    if (_polling || !busy || _disposed) return;
    _polling = true;
    try {
      final value = await channel.invokeMapMethod<String, dynamic>(
        'macOSCheckState',
      );
      if (busy && !_disposed && value?['stage'] is String) {
        stage = value!['stage'] as String;
        _changed();
      }
    } catch (_) {
      // The attempt itself reports channel failures, without raw exception text.
    } finally {
      _polling = false;
    }
  }

  Future<void> run() async {
    if (busy || _disposed || cleanupFailed) return;
    busy = true;
    _cancelRequested = false;
    stage = 'starting';
    report = null;
    _changed();
    try {
      final health = await client.healthInfo().timeout(
        const Duration(seconds: 5),
      );
      _health = {
        'status': health.ok && health.version == embeddedAppVersion
            ? 'pass'
            : 'fail',
        if (RegExp(r'^\d+\.\d+\.\d+$').hasMatch(health.version))
          'version': health.version,
      };
    } catch (_) {
      _health = {'status': 'fail'};
    }
    if (_disposed) return;
    if (_cancelRequested) {
      report = {
        'outcome': 'cancelled',
        'cleanup': {'status': 'not_needed'},
      };
      busy = false;
      _changed();
      return;
    }
    _timer = Timer.periodic(const Duration(milliseconds: 500), (_) {
      unawaited(_poll());
    });
    try {
      final pending = channel.invokeMapMethod<dynamic, dynamic>(
        'runMacOSCheck',
      );
      Map<dynamic, dynamic>? value;
      try {
        value = await pending.timeout(const Duration(seconds: 90));
      } on TimeoutException {
        await cancel();
        // Cancellation returns immediately; the original call waits for cleanup.
        value = await pending.timeout(const Duration(seconds: 8));
      }
      report = _safeReport(value ?? {});
    } catch (_) {
      await cancel();
      report = {
        'outcome': 'channel_error',
        'cleanup': {'status': 'fail'},
      };
    } finally {
      _timer?.cancel();
      busy = false;
      _changed();
    }
  }

  Future<void> cancel() async {
    _cancelRequested = true;
    try {
      await channel
          .invokeMethod<void>('cancelMacOSCheck')
          .timeout(const Duration(seconds: 8));
    } catch (_) {
      // Keep the result dialog open and offer cleanup again on failure.
    }
  }

  Future<void> retryCleanup() async {
    if (busy || _disposed) return;
    busy = true;
    stage = 'cleaning';
    _changed();
    try {
      final value = await channel
          .invokeMapMethod<dynamic, dynamic>('cleanupMacOSCheck')
          .timeout(const Duration(seconds: 8));
      report = _safeReport(value ?? {});
    } catch (_) {
      report = {
        ...?report,
        'cleanup': {'status': 'fail'},
      };
    } finally {
      busy = false;
      _changed();
    }
  }

  // Whitelist at every level: no arbitrary logs, paths, profiles or IPC payloads.
  static Map<String, dynamic> _safeReport(Map<dynamic, dynamic> raw) {
    Map<String, dynamic> fields(Map value, List<String> keys) => {
      for (final key in keys)
        if (value[key] is num || value[key] is bool || value[key] is String)
          key: value[key],
    };
    return {
      'cleanup': {'status': 'fail'},
      ...fields(raw, [
        'schema_version',
        'diagnostic_version',
        'outcome',
        'registration_before_cleanup',
      ]),
      for (final entry in {
        'system': ['macos', 'architecture'],
        'app': ['version', 'build'],
        'cleanup': ['status', 'registration'],
        'cleanup_error': ['domain', 'code'],
      }.entries)
        if (raw[entry.key] is Map)
          entry.key: fields(raw[entry.key] as Map, entry.value),
      if (raw['checks'] is List)
        'checks': [
          for (final check in (raw['checks'] as List).whereType<Map>())
            fields(check, [
              'id',
              'status',
              'state',
              'error_domain',
              'error_code',
              'effective_uid',
              'protocol_version',
              'helper_build',
            ]),
        ],
    };
  }

  @override
  void dispose() {
    _disposed = true;
    _timer?.cancel();
    if (busy) unawaited(cancel());
    super.dispose();
  }
}

class MacOSCheckBanner extends StatelessWidget {
  const MacOSCheckBanner({
    super.key,
    required this.client,
    required this.notice,
  });

  final ControlPlaneClient client;
  final String notice;

  @override
  Widget build(BuildContext context) => Material(
    color: Theme.of(context).colorScheme.surface,
    child: SafeArea(
      bottom: false,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        child: Row(
          children: [
            Expanded(child: Text(notice, style: const TextStyle(fontSize: 12))),
            const SizedBox(width: 12),
            TextButton.icon(
              icon: const Icon(Icons.fact_check_outlined, size: 18),
              label: const Text('Проверить Mac'),
              onPressed: () => showDialog<void>(
                context: context,
                barrierDismissible: false,
                builder: (_) => MacOSCheckDialog(client: client),
              ),
            ),
          ],
        ),
      ),
    ),
  );
}

class MacOSCheckDialog extends StatefulWidget {
  const MacOSCheckDialog({super.key, required this.client});
  final ControlPlaneClient client;

  @override
  State<MacOSCheckDialog> createState() => _MacOSCheckDialogState();
}

class _MacOSCheckDialogState extends State<MacOSCheckDialog> {
  late final MacOSCheckController controller;
  bool copied = false;

  @override
  void initState() {
    super.initState();
    controller = MacOSCheckController(widget.client);
    unawaited(controller.run());
  }

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: controller,
    builder: (context, _) => PopScope(
      canPop: !controller.busy,
      child: AlertDialog(
        title: Text(controller.title),
        content: SizedBox(
          width: 420,
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (controller.busy) ...[
                const LinearProgressIndicator(),
                const SizedBox(height: 16),
              ],
              Text(controller.message),
            ],
          ),
        ),
        actions: controller.busy
            ? [
                TextButton(
                  onPressed: controller.cancel,
                  child: const Text('Отменить'),
                ),
              ]
            : [
                if (controller.cleanupFailed)
                  TextButton(
                    onPressed: controller.retryCleanup,
                    child: const Text('Завершить проверку'),
                  )
                else if (!controller.passed)
                  TextButton(
                    onPressed: () {
                      copied = false;
                      controller.run();
                    },
                    child: const Text('Повторить'),
                  ),
                TextButton(
                  onPressed: () async {
                    await Clipboard.setData(
                      ClipboardData(text: controller.reportText),
                    );
                    if (mounted) setState(() => copied = true);
                  },
                  child: Text(
                    copied ? 'Отчёт скопирован' : 'Скопировать отчёт',
                  ),
                ),
                TextButton(
                  onPressed: () => Navigator.of(context).pop(),
                  child: const Text('Закрыть'),
                ),
              ],
      ),
    ),
  );
}
