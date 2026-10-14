import 'dart:async';
import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:naga_network/control_plane_client.dart';
import 'package:naga_network/main.dart';
import 'package:naga_network/node_probe_controller.dart';

const jobId = '0123456789abcdef0123456789abcdef';
Map<String, dynamic> node(String id, String phase, {int? latency}) => {
  'id': id,
  'profile_id': 'profile',
  'tag': id,
  'runtime_tag': id,
  'protocol': id == 'fast' ? 'VLESS' : 'TUIC',
  'country': 'Test',
  'phase': phase,
  'probe_status': phase == 'healthy' ? 'healthy' : 'unknown',
  if (latency != null) 'latency_ms': latency,
  if (latency != null) 'checked_at': '2026-10-14T10:00:00Z',
};
http.Response snapshot(
  String state,
  int revision,
  List<Map<String, dynamic>> nodes, {
  int completed = 0,
  int status = 200,
}) => http.Response(
  jsonEncode({
    'job_id': jobId,
    'state': state,
    'revision': revision,
    'total': 2,
    'completed': completed,
    'nodes': nodes,
  }),
  status,
);
http.Response health({bool supported = true}) => http.Response(
  jsonEncode({
    'status': 'ok',
    'capabilities': {'node_probe_jobs': supported},
  }),
  200,
);

Future<void> pumpUntil(WidgetTester tester, bool Function() ready) async {
  for (var i = 0; i < 200 && !ready(); i++) {
    await tester.pump(const Duration(milliseconds: 10));
  }
  expect(
    ready(),
    isTrue,
    reason: 'Expected asynchronous state within two seconds',
  );
}

void main() {
  testWidgets('late full snapshot cannot reverse acknowledged cancellation', (
    tester,
  ) async {
    final cancelReply = Completer<http.Response>();
    final staleReply = Completer<http.Response>();
    var reads = 0;
    final controller = NodeProbeController(
      ControlPlaneClient(
        client: MockClient((request) async {
          if (request.url.path == '/v1/health') return health();
          if (request.url.path.endsWith('/start'))
            return snapshot('running', 99, [], status: 202);
          if (request.url.path.endsWith('/cancel')) return cancelReply.future;
          reads++;
          if (reads == 1)
            return http.Response(
              '{"error":"invalid revision","full_snapshot_required":true}',
              400,
            );
          if (reads == 2) return staleReply.future;
          return snapshot('cancelled', 101, []);
        }),
      ),
    );
    controller.attach();
    final result = controller.start();
    await pumpUntil(tester, () => controller.progressError != null);
    final cancel = controller.cancel();
    await pumpUntil(tester, () => reads == 2);
    cancelReply.complete(snapshot('cancelling', 100, []));
    await tester.pump();
    await cancel;
    staleReply.complete(snapshot('running', 2, []));
    await tester.pump();
    expect(controller.job?.state, 'cancelling');
    await pumpUntil(tester, () => controller.job?.terminal == true);
    await result;
    controller.detach();
    controller.dispose();
  });

  testWidgets('queued rows do not create hundreds of animation tickers', (
    tester,
  ) async {
    final controller =
        NodeProbeController(
            ControlPlaneClient(
              client: MockClient((_) async => snapshot('running', 1, [])),
            ),
          )
          ..job = const NodeProbeJob(
            id: jobId,
            state: 'running',
            revision: 1,
            nodes: [],
            total: 200,
          );
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: NagaNodesView(
              nodes: [
                for (var i = 0; i < 200; i++)
                  UnifiedNode.fromJson(node('queue$i', 'queued')),
              ],
              connected: false,
              onSelect: (_) {},
              onProbe: controller.start,
              probeController: controller,
            ),
          ),
        ),
      ),
    );
    await tester.pump();
    expect(find.byType(CircularProgressIndicator), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
    controller.dispose();
  });

  testWidgets('unchanged progress does not repeatedly notify list consumers', (
    tester,
  ) async {
    var reads = 0;
    var changes = 0;
    final controller = NodeProbeController(
      ControlPlaneClient(
        client: MockClient((request) async {
          if (request.url.path == '/v1/health') return health();
          if (request.method == 'POST')
            return snapshot('running', 1, [], status: 202);
          reads++;
          return snapshot('running', 2, [
            node('fast', 'healthy', latency: 25),
          ], completed: 1);
        }),
      ),
    );
    controller.addListener(() {
      if (controller.job?.revision == 2) changes++;
    });
    controller.attach();
    controller.start().ignore();
    await pumpUntil(tester, () => reads >= 3);
    expect(changes, 1);
    controller.detach();
    controller.dispose();
  });

  testWidgets('cleanup failure retains the job until its resources close', (
    tester,
  ) async {
    var reads = 0;
    final controller = NodeProbeController(
      ControlPlaneClient(
        client: MockClient((request) async {
          if (request.url.path == '/v1/health') return health();
          if (request.method == 'POST')
            return snapshot('running', 1, [], status: 202);
          reads++;
          return http.Response(
            jsonEncode({
              'job_id': jobId,
              'state': 'failed',
              'revision': reads + 1,
              'cleanup_complete': reads > 1,
              'error': 'Cleanup failed',
              'total': 2,
              'nodes': [],
            }),
            200,
          );
        }),
      ),
    );
    controller.attach();
    final expected = expectLater(
      controller.start(),
      throwsA(isA<ControlPlaneException>()),
    );
    await pumpUntil(tester, () => controller.job?.state == 'failed');
    expect(controller.busy, isTrue);
    expect(controller.job!.terminal, isFalse);
    controller.setForeground(false);
    await tester.pump(const Duration(seconds: 1));
    expect(reads, 1);
    controller.setForeground(true);
    await pumpUntil(tester, () => controller.job?.terminal == true);
    await expected;
    expect(controller.busy, isFalse);
    controller.detach();
    controller.dispose();
  });

  testWidgets(
    'invalid revision requests a full snapshot without a second job',
    (tester) async {
      final queries = <Map<String, String>>[];
      var posts = 0;
      final controller = NodeProbeController(
        ControlPlaneClient(
          client: MockClient((request) async {
            if (request.url.path == '/v1/health') return health();
            if (request.method == 'POST') {
              posts++;
              return snapshot('running', 99, [], status: 202);
            }
            queries.add(request.url.queryParameters);
            if (queries.length == 1)
              return http.Response(
                '{"error":"invalid revision","full_snapshot_required":true}',
                400,
              );
            return snapshot('completed', 2, [
              node('fast', 'healthy', latency: 25),
            ], completed: 2);
          }),
        ),
      );
      controller.attach();
      final result = controller.start();
      await pumpUntil(tester, () => controller.job?.terminal == true);
      expect((await result).single.latencyMs, 25);
      expect(posts, 1);
      expect(queries.first['after_revision'], '99');
      expect(queries.last.containsKey('after_revision'), isFalse);
      controller.detach();
      controller.dispose();
    },
  );

  testWidgets('missing job releases the old wait and recovers through active', (
    tester,
  ) async {
    var posts = 0;
    var activeReads = 0;
    final controller = NodeProbeController(
      ControlPlaneClient(
        client: MockClient((request) async {
          if (request.url.path == '/v1/health') return health();
          if (request.method == 'POST') {
            posts++;
            return snapshot('running', 1, [], status: 202);
          }
          if (request.url.path.endsWith('/active')) {
            activeReads++;
            return http.Response('null', 200);
          }
          return http.Response('{"error":"probe job not found"}', 404);
        }),
      ),
    );
    controller.attach();
    final expected = expectLater(
      controller.start(),
      throwsA(isA<ControlPlaneException>()),
    );
    await pumpUntil(tester, () => controller.progressError != null);
    await expected;
    expect(controller.busy, isFalse);
    await pumpUntil(tester, () => activeReads == 1);
    expect(posts, 1);
    controller.detach();
    controller.dispose();
  });

  test('malformed node metadata is a readable error', () {
    expect(
      () => NodeProbeJob.fromJson({
        'job_id': jobId,
        'state': 'running',
        'revision': 1,
        'nodes': [123],
      }),
      throwsA(isA<ControlPlaneException>()),
    );
  });

  testWidgets(
    'ready latency is visible within 300 ms while another node is pending',
    (tester) async {
      var cancelled = false;
      var posts = 0;
      final client = ControlPlaneClient(
        client: MockClient((request) async {
          if (request.url.path == '/v1/health') return health();
          if (request.url.path.endsWith('/start')) {
            posts++;
            return snapshot('preparing', 1, [], status: 202);
          }
          if (request.url.path.endsWith('/cancel')) {
            cancelled = true;
            return snapshot('cancelling', 3, []);
          }
          return cancelled
              ? snapshot('cancelled', 4, [
                  node('slow', 'cancelled'),
                ], completed: 1)
              : snapshot('running', 2, [
                  node('fast', 'healthy', latency: 25),
                  node('slow', 'checking'),
                ], completed: 1);
        }),
      );
      final controller = NodeProbeController(client);
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: NagaNodesView(
              nodes: [
                UnifiedNode.fromJson(node('fast', 'queued')),
                UnifiedNode.fromJson(node('slow', 'queued')),
              ],
              connected: false,
              onSelect: (_) {},
              onProbe: controller.start,
              probeController: controller,
            ),
          ),
        ),
      );
      await tester.pump();
      await tester.tap(find.text('Проверить'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pump();
      expect(find.text('25 мс'), findsOneWidget);
      expect(find.text('Готово: 1 из 2'), findsOneWidget);
      expect(find.text('Отменить проверку'), findsOneWidget);
      expect(controller.busy, isTrue);
      await tester.tap(find.text('Отменить проверку'));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pump();
      expect(controller.busy, isFalse);
      expect(controller.nodes.firstWhere((n) => n.id == 'fast').latencyMs, 25);
      expect(posts, 1);
      await tester.pumpWidget(const SizedBox());
      controller.dispose();
    },
  );

  testWidgets(
    'slow progress requests do not overlap and detach pauses polling',
    (tester) async {
      final pending = Completer<http.Response>();
      var reads = 0;
      var posts = 0;
      final controller = NodeProbeController(
        ControlPlaneClient(
          client: MockClient((request) async {
            if (request.url.path == '/v1/health') return health();
            if (request.method == 'POST') {
              posts++;
              return snapshot('running', 1, [], status: 202);
            }
            reads++;
            if (reads == 1) return pending.future;
            return snapshot('completed', 3, [
              node('fast', 'healthy', latency: 25),
            ], completed: 2);
          }),
        ),
      );
      controller.attach();
      final result = controller.start();
      await tester.pump();
      await tester.pump(const Duration(seconds: 2));
      expect(reads, 1);
      controller.detach();
      pending.complete(
        snapshot('running', 2, [
          node('fast', 'healthy', latency: 25),
        ], completed: 1),
      );
      await tester.pump();
      await tester.pump(const Duration(seconds: 2));
      expect(reads, 1);
      controller.attach();
      await tester.pump();
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pump();
      expect(controller.job?.state, 'completed');
      expect((await result).single.latencyMs, 25);
      expect(posts, 1);
      controller.detach();
      controller.dispose();
    },
  );

  testWidgets(
    'progress failure retries the existing job without another POST',
    (tester) async {
      var reads = 0;
      var posts = 0;
      final progressRequests = <Uri>[];
      final controller = NodeProbeController(
        ControlPlaneClient(
          client: MockClient((request) async {
            if (request.url.path == '/v1/health') return health();
            if (request.method == 'POST') {
              posts++;
              return snapshot('running', 1, [], status: 202);
            }
            reads++;
            if (reads == 1) throw http.ClientException('offline');
            progressRequests.add(request.url);
            return snapshot('completed', 2, [
              node('fast', 'healthy', latency: 25),
            ], completed: 2);
          }),
        ),
      );
      controller.attach();
      final result = controller.start();
      await tester.pump();
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      await tester.pump();
      await pumpUntil(tester, () => controller.progressError != null);
      expect(controller.progressError, isNotNull);
      await tester.pump(const Duration(seconds: 1));
      await tester.pump();
      await tester.pump(const Duration(seconds: 2));
      await tester.pump();
      expect(
        controller.job?.state,
        'completed',
        reason: 'reads=$reads error=${controller.progressError}',
      );
      expect((await result).single.latencyMs, 25);
      expect(posts, 1);
      expect(
        progressRequests.every((url) => url.path.endsWith(jobId)),
        isTrue,
        reason: progressRequests.toString(),
      );
      expect(
        progressRequests.every(
          (url) => url.queryParameters['after_revision'] == '1',
        ),
        isTrue,
        reason: progressRequests.toString(),
      );
      controller.detach();
      controller.dispose();
    },
  );

  testWidgets(
    'older control uses legacy endpoint, authorization failure does not',
    (tester) async {
      for (final supported in [false, true]) {
        var legacy = 0;
        final controller = NodeProbeController(
          ControlPlaneClient(
            client: MockClient((request) async {
              if (request.url.path == '/v1/health')
                return health(supported: supported);
              if (request.url.path == '/v1/nodes/probe') {
                legacy++;
                return http.Response('[]', 200);
              }
              return http.Response('{"error":"authorization required"}', 401);
            }),
          ),
        );
        if (supported) {
          await expectLater(
            controller.start(),
            throwsA(isA<ControlPlaneException>()),
          );
        } else {
          expect(await controller.start(), isEmpty);
        }
        expect(legacy, supported ? 0 : 1);
        controller.dispose();
      }
    },
  );

  test(
    'stale latency and changed profile revision do not override new data',
    () {
      final old = UnifiedNode.fromJson({
        ...node('fast', 'healthy', latency: 10),
        'profile_revision': 1,
      });
      final fresh = UnifiedNode.fromJson({
        ...node('fast', 'healthy', latency: 50),
        'checked_at': '2026-10-14T10:00:01Z',
        'profile_revision': 1,
      });
      expect(nagaPreferProbedNode(old, fresh).latencyMs, 50);
      final changed = UnifiedNode.fromJson({
        ...node('fast', 'queued'),
        'profile_revision': 2,
      });
      expect(nagaPreferProbedNode(changed, fresh).latencyMs, isNull);
    },
  );
}
