import 'dart:io';

import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/app_update.dart';
import 'package:path/path.dart' as p;

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();

  Future<void> checkUpdate({required bool deny, bool symlink = false}) async {
    final root = Directory.systemTemp.createTempSync('naga-single-image-');
    addTearDown(() => root.deleteSync(recursive: true));
    final current = File(p.join(root.path, linuxAppImageName('1.2.4')))
      ..writeAsStringSync('current image');
    final destination = File(p.join(root.path, linuxAppImageName('1.2.5')));
    final sentinel = File(p.join(root.path, 'sentinel'))
      ..writeAsStringSync('unchanged');
    if (symlink) {
      Link(destination.path).createSync(sentinel.path);
    } else if (deny) {
      destination.writeAsStringSync('previous retry image');
    }
    final downloads = Directory(p.join(root.path, 'downloads'))..createSync();
    final incoming = File(p.join(downloads.path, linuxAppImageName('1.2.5')))
      ..writeAsStringSync('new image with runtime');
    final processes = <String>[];
    final launches = <String>[];
    final messenger =
        TestDefaultBinaryMessengerBinding.instance.defaultBinaryMessenger;
    messenger.setMockMethodCallHandler(const MethodChannel(updateChannelName), (
      call,
    ) async {
      expect(call.method, 'launchAndQuit');
      launches.add((call.arguments as Map)['path'] as String);
      return null;
    });
    addTearDown(
      () => messenger.setMockMethodCallHandler(
        const MethodChannel(updateChannelName),
        null,
      ),
    );
    final updater = AppUpdater(
      kind: UpdateInstallKind.linuxAppImage,
      environment: {'APPIMAGE': current.path},
      runProcess: (executable, args) async {
        processes.add(executable);
        if (executable == 'chmod') return ProcessResult(1, 0, '', '');
        expect(executable, destination.path);
        expect(args, ['--prepare-runtime']);
        expect(destination.readAsStringSync(), 'new image with runtime');
        return ProcessResult(1, deny ? 126 : 0, '', '');
      },
    );
    addTearDown(updater.close);
    final operation = updater.applyDownloaded(
      [incoming],
      version: '1.2.5',
      disconnectIfNeeded: () async => true,
      waitForControlVersion: (version) async => version == '1.2.5',
    );
    if (deny || symlink) {
      await expectLater(operation, throwsA(isA<AppUpdateException>()));
      expect(launches, isEmpty);
      if (deny && !symlink)
        expect(destination.readAsStringSync(), 'previous retry image');
      if (symlink) expect(processes, isEmpty);
    } else {
      await operation;
      expect(processes, ['chmod', destination.path]);
      expect(launches, [destination.path]);
    }
    expect(sentinel.readAsStringSync(), 'unchanged');
    expect(current.readAsStringSync(), 'current image');
    expect(
      root.listSync().where(
        (f) => p.basename(f.path).startsWith('.naga-update-'),
      ),
      isEmpty,
    );
  }

  test(
    'Linux update installs runtime from its only downloaded asset',
    () => checkUpdate(deny: false),
  );
  test(
    'Linux installer cancellation preserves images and does not launch',
    () => checkUpdate(deny: true),
  );
  test(
    'Linux update cannot overwrite a symlink target',
    () => checkUpdate(deny: false, symlink: true),
  );
}
