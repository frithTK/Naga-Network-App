import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/control_plane_token_io.dart';

void main() {
  test('prefers NAGA_CONTROL_TOKEN environment value', () {
    final token = loadControlPlaneToken(
      environment: const {
        'NAGA_CONTROL_TOKEN': '  env-token  ',
        'NAGA_CONTROL_TOKEN_FILE': '/tmp/should-not-read',
      },
    );
    expect(token, 'env-token');
  });

  test('reads trimmed token file and ignores empty file', () {
    final dir = Directory.systemTemp.createTempSync('naga-token-');
    addTearDown(() => dir.deleteSync(recursive: true));
    final file = File('${dir.path}/control.token');
    file.writeAsStringSync('  file-token \n');

    expect(
      loadControlPlaneToken(
        environment: {'NAGA_CONTROL_TOKEN_FILE': file.path},
      ),
      'file-token',
    );

    file.writeAsStringSync('   \n');
    expect(
      loadControlPlaneToken(
        environment: {'NAGA_CONTROL_TOKEN_FILE': file.path},
      ),
      isNull,
    );
  });

  test('missing token file returns null', () {
    expect(
      loadControlPlaneToken(
        environment: const {
          'NAGA_CONTROL_TOKEN_FILE': '/tmp/naga-missing-control.token',
        },
      ),
      isNull,
    );
  });

  test('uses NAGA_DATA_DIR token path', () {
    final dir = Directory.systemTemp.createTempSync('naga-data-');
    addTearDown(() => dir.deleteSync(recursive: true));
    final file = File('${dir.path}${Platform.pathSeparator}control.token');
    file.writeAsStringSync('data-dir-token');

    expect(
      loadControlPlaneToken(environment: {'NAGA_DATA_DIR': dir.path}),
      'data-dir-token',
    );
  });

  test('windows default path is LOCALAPPDATA/naga-network/control.token', () {
    final dir = Directory.systemTemp.createTempSync('naga-win-');
    addTearDown(() => dir.deleteSync(recursive: true));
    final tokenDir = Directory(
      '${dir.path}${Platform.pathSeparator}naga-network',
    )..createSync();
    File('${tokenDir.path}${Platform.pathSeparator}control.token')
        .writeAsStringSync('windows-token');

    expect(
      controlPlaneTokenFilePath(
        environment: {'LOCALAPPDATA': dir.path},
        isWindows: true,
      ),
      '${dir.path}${Platform.pathSeparator}naga-network${Platform.pathSeparator}control.token',
    );
    expect(
      loadControlPlaneToken(
        environment: {'LOCALAPPDATA': dir.path},
        isWindows: true,
      ),
      'windows-token',
    );
  });
}
