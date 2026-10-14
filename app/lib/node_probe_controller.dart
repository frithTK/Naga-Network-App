import 'dart:async';

import 'package:flutter/foundation.dart';

import 'control_plane_client.dart';

/// One controller per application; screens only subscribe to its existing job.
class NodeProbeController extends ChangeNotifier {
  NodeProbeController(
    this.client, {
    this.pollInterval = const Duration(milliseconds: 300),
  });
  final ControlPlaneClient client;
  final Duration pollInterval;
  NodeProbeJob? job;
  String? progressError;
  final Map<String, UnifiedNode> _nodes = {};
  List<UnifiedNode> get nodes => _nodes.values.toList(growable: false);
  bool _starting = false;
  bool _legacy = false;
  bool _foreground = true;
  bool _disposed = false;
  bool _requestInFlight = false;
  bool _cancelRequested = false;
  int _consumers = 0;
  int _epoch = 0;
  bool _fullSnapshot = false;
  Timer? _timer;
  Completer<List<UnifiedNode>>? _completion;
  bool get busy => _starting || _legacy || (job != null && !job!.terminal);

  void attach() {
    _consumers++;
    _schedule(Duration.zero);
  }

  void detach() {
    if (_consumers > 0) _consumers--;
    if (_consumers == 0) _timer?.cancel();
  }

  void setForeground(bool value) {
    _foreground = value;
    if (!value) {
      _timer?.cancel();
    } else {
      _schedule(Duration.zero);
    }
  }

  void _emit() {
    if (!_disposed) notifyListeners();
  }

  void _schedule(Duration delay) {
    _timer?.cancel();
    if (_disposed || !_foreground || _consumers == 0 || _legacy || _starting)
      return;
    if (job?.terminal == true) return;
    _timer = Timer(delay, () => unawaited(_poll()));
  }

  Future<void> _poll() async {
    if (_requestInFlight || _disposed || _starting) return;
    final epoch = _epoch;
    _requestInFlight = true;
    try {
      if (job == null && !(await client.healthInfo()).nodeProbeJobs) return;
      final update = await client.nodeProbeJob(
        id: job?.id ?? 'active',
        afterRevision: _fullSnapshot ? 0 : job?.revision ?? 0,
      );
      if (_disposed || epoch != _epoch) return;
      final hadError = progressError != null;
      progressError = null;
      if (update != null) {
        final changed = _accept(update, reset: _fullSnapshot);
        if (hadError && !changed) _emit();
        _fullSnapshot = false;
      } else if (hadError) {
        _emit();
      }
    } on ControlPlaneException catch (error) {
      if (!_disposed && epoch == _epoch) {
        progressError = error.message;
        if (error.fullSnapshotRequired) _fullSnapshot = true;
        if (error.statusCode == 404 && job != null) {
          job = null;
          _nodes.clear();
          final completion = _completion;
          _completion = null;
          if (completion != null && !completion.isCompleted)
            completion.completeError(error);
        }
        _emit();
      }
    } finally {
      _requestInFlight = false;
      // Failed progress reads retry the same ID; they never submit a new job.
      if (job != null || progressError != null)
        _schedule(
          progressError == null ? pollInterval : const Duration(seconds: 1),
        );
    }
  }

  bool _accept(NodeProbeJob update, {bool reset = false}) {
    if (!reset &&
        job != null &&
        update.id == job!.id &&
        update.revision <= job!.revision)
      return false;
    if (reset || job?.id != update.id) _nodes.clear();
    job = update;
    if (update.invalidated) {
      _nodes.clear();
    } else {
      for (final node in update.nodes) {
        _nodes[node.id] = node;
      }
    }
    _emit();
    if (update.terminal) {
      _timer?.cancel();
      final completion = _completion;
      _completion = null;
      if (completion != null && !completion.isCompleted) {
        if (update.state == 'failed') {
          completion.completeError(
            ControlPlaneException(
              update.error ?? 'Не удалось проверить серверы.',
            ),
          );
        } else {
          completion.complete(nodes);
        }
      }
    }
    return true;
  }

  Future<List<UnifiedNode>> start() async {
    if (_completion != null) return _completion!.future;
    if (job != null && !job!.terminal) {
      _completion = Completer<List<UnifiedNode>>();
      _schedule(Duration.zero);
      return _completion!.future;
    }
    job = null;
    _nodes.clear();
    final completion = Completer<List<UnifiedNode>>();
    // Attach an error handler immediately, including registration failures.
    final result = completion.future;
    result.ignore();
    _completion = completion;
    _epoch++;
    _starting = true;
    _cancelRequested = false;
    progressError = null;
    _emit();
    try {
      final health = await client.healthInfo();
      if (!health.nodeProbeJobs) {
        _legacy = true;
        final list = await client.probeUnifiedNodes();
        _nodes.clear();
        for (final node in list) {
          _nodes[node.id] = node;
        }
        completion.complete(nodes);
        _completion = null;
      } else {
        _accept(await client.startNodeProbeJob());
        if (_cancelRequested && job != null && !job!.terminal) {
          _accept(await client.cancelNodeProbeJob(job!.id));
        }
      }
    } catch (error, stack) {
      if (!completion.isCompleted) completion.completeError(error, stack);
      _completion = null;
      // Registration might have succeeded before a network failure. Recover
      // through the active endpoint on the next poll, without another POST.
      if (error is ControlPlaneException) progressError = error.message;
    } finally {
      _starting = false;
      _legacy = false;
      _emit();
      _schedule(Duration.zero);
    }
    return result;
  }

  Future<void> cancel() async {
    if (_legacy) return;
    _cancelRequested = true;
    if (job == null || job!.terminal) return;
    final id = job!.id;
    _epoch++;
    try {
      final update = await client.cancelNodeProbeJob(id);
      if (!_disposed && job?.id == id) {
        _epoch++;
        _accept(update);
      }
    } on ControlPlaneException catch (error) {
      if (!_disposed && job?.id == id) {
        progressError = error.message;
        _emit();
      }
    }
    _schedule(Duration.zero);
  }

  @override
  void dispose() {
    _disposed = true;
    _timer?.cancel();
    // Closing a screen/app does not stop a control-owned job.
    if (_completion != null && !_completion!.isCompleted)
      _completion!.complete(nodes);
    _completion = null;
    super.dispose();
  }
}
