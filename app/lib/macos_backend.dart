import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'app_update.dart';
import 'control_plane_client.dart';

const macOSNativeChannel = MethodChannel('eu.nagavpn.naga_network/macos');

class MacOSBackendConnection {
  const MacOSBackendConnection._(
    this.endpoint,
    this._token,
    this.dataDirectory,
  );

  final String endpoint;
  final String _token;
  final String dataDirectory;

  factory MacOSBackendConnection.fromMap(
    Map<dynamic, dynamic> value, {
    String expectedVersion = embeddedAppVersion,
  }) {
    final endpoint = value['endpoint'];
    final token = value['token'];
    final dataDirectory = value['data_dir'];
    final uri = endpoint is String ? Uri.tryParse(endpoint) : null;
    if (value['type'] != 'ready' ||
        value['protocol_version'] != 1 ||
        value['version'] != expectedVersion ||
        uri == null ||
        uri.scheme != 'http' ||
        uri.host != '127.0.0.1' ||
        !uri.hasPort ||
        uri.port < 1 ||
        uri.port > 65535 ||
        uri.userInfo.isNotEmpty ||
        uri.path.isNotEmpty ||
        uri.hasQuery ||
        uri.hasFragment ||
        dataDirectory is! String ||
        !dataDirectory.startsWith('/') ||
        token is! String ||
        !RegExp(r'^[0-9a-f]{64}$').hasMatch(token)) {
      throw const FormatException(
        'Некорректный или несовместимый ответ запуска backend.',
      );
    }
    return MacOSBackendConnection._(endpoint as String, token, dataDirectory);
  }

  ControlPlaneClient createClient() => ControlPlaneClient(
    baseUrl: endpoint,
    token: _token,
    dataDirectory: dataDirectory,
  );
}

/// The dashboard cannot issue requests until its own native child is ready.
class MacOSBackendGate extends StatefulWidget {
  const MacOSBackendGate({super.key, required this.builder});

  final Widget Function(ControlPlaneClient client) builder;

  @override
  State<MacOSBackendGate> createState() => _MacOSBackendGateState();
}

class _MacOSBackendGateState extends State<MacOSBackendGate> {
  ControlPlaneClient? _client;
  String? _error;
  bool _busy = false;
  int _generation = 0;

  @override
  void initState() {
    super.initState();
    macOSNativeChannel.setMethodCallHandler((call) async {
      if (call.method == 'backendExited' && mounted) {
        _generation++;
        _client?.close();
        setState(() {
          _client = null;
          _busy = false;
          _error = 'Локальная служба приложения завершилась. Профили сохранены; можно повторить запуск.';
        });
      }
    });
    unawaited(_start());
  }

  Future<void> _start() async {
    if (_busy) return;
    final current = ++_generation;
    setState(() {
      _busy = true;
      _error = null;
    });
    ControlPlaneClient? client;
    try {
      final raw = await macOSNativeChannel
          .invokeMapMethod<dynamic, dynamic>('startBackend')
          .timeout(const Duration(seconds: 20));
      final connection = MacOSBackendConnection.fromMap(raw ?? const {});
      client = connection.createClient();
      final health = await client.healthInfo();
      if (!health.ok || health.version != embeddedAppVersion) {
        throw const FormatException(
          'Встроенная служба вернула несовместимую версию.',
        );
      }
      if (!mounted || current != _generation) {
        client.close();
        return;
      }
      setState(() {
        _client = client;
        _busy = false;
      });
    } catch (error) {
      client?.close();
      if (!mounted || current != _generation) return;
      setState(() {
        _busy = false;
        _error = switch (error) {
          PlatformException() =>
            error.message ?? 'Не удалось запустить локальную службу.',
          FormatException() => error.message,
          TimeoutException() => 'Истекло время запуска локальной службы.',
          _ => 'Не удалось связаться со встроенной службой приложения.',
        };
      });
    }
  }

  @override
  void dispose() {
    _generation++;
    macOSNativeChannel.setMethodCallHandler(null);
    _client?.close();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final client = _client;
    if (client != null) return widget.builder(client);
    return MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: ThemeData.dark(),
      home: Scaffold(
        backgroundColor: const Color(0xFF080808),
        body: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 520),
            child: Padding(
              padding: const EdgeInsets.all(32),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Text('Naga Network', style: TextStyle(fontSize: 28)),
                  const SizedBox(height: 24),
                  if (_busy)
                    const CircularProgressIndicator(color: Color(0xFFF72F38)),
                  const SizedBox(height: 16),
                  Text(
                    _error ?? 'Запускаем приложение…',
                    textAlign: TextAlign.center,
                  ),
                  if (!_busy) ...[
                    const SizedBox(height: 20),
                    FilledButton(
                      onPressed: _start,
                      child: const Text('Повторить запуск'),
                    ),
                  ],
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}
