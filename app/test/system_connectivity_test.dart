import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/system_connectivity.dart';

void main() {
  test('requires a 204 response and retries a transient failure', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    var calls = 0;
    server.listen((request) async {
      calls++;
      request.response.statusCode = calls == 1 ? 503 : 204;
      await request.response.close();
    });
    addTearDown(() => server.close(force: true));
    expect(
      await checkSystemConnectivity(
        endpoint: Uri.parse('http://127.0.0.1:${server.port}/'),
      ),
      isTrue,
    );
    expect(calls, 2);
  });

  test('does not accept redirects as proof of connectivity', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      request.response.statusCode = 302;
      request.response.headers.set('location', '/success');
      await request.response.close();
    });
    addTearDown(() => server.close(force: true));
    expect(
      await checkSystemConnectivity(
        endpoint: Uri.parse('http://127.0.0.1:${server.port}/'),
      ),
      isFalse,
    );
  });
}
