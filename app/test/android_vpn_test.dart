import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/android_vpn.dart';

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  test('host flutter tests are not Android', () {
    expect(nagaRunsOnAndroid, isFalse);
    expect(Platform.isAndroid, isFalse);
  });

  test('waitForControlToken reads dataDir file', () async {
    final dir = Directory.systemTemp.createTempSync('naga-android-token-');
    addTearDown(() => dir.deleteSync(recursive: true));
    File('${dir.path}${Platform.pathSeparator}control.token')
        .writeAsStringSync('  android-token \n');

    final vpn = AndroidVpn(
      vpnChannel: const MethodChannel(vpnChannelName),
      updateChannel: const MethodChannel('eu.nagavpn.naga_network/update'),
    );
    TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
        .setMockMethodCallHandler(
      const MethodChannel('eu.nagavpn.naga_network/update'),
      (call) async {
        if (call.method == 'dataDir') return dir.path;
        return null;
      },
    );
    addTearDown(() {
      TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger
          .setMockMethodCallHandler(
        const MethodChannel('eu.nagavpn.naga_network/update'),
        null,
      );
    });

    expect(await vpn.waitForControlToken(attempts: 2, delay: Duration.zero), 'android-token');
  });
}
