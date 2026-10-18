import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/config_file_picker.dart';

void main() {
  test('file import preserves Unicode names and refuses oversized files before reading', () async {
    final root = Directory.systemTemp.createTempSync('naga-config-');
    addTearDown(() => root.deleteSync(recursive: true));
    final file = File('${root.path}/Профиль с пробелами.json');
    await file.writeAsString('{"outbounds":[]}');
    final picked = await readNagaConfigFile(file);
    expect(picked.fileName, 'Профиль с пробелами.json');
    expect(picked.contents, '{"outbounds":[]}');
    final handle = await file.open(mode: FileMode.write);
    await handle.truncate(nagaMaxConfigFileBytes + 1);
    await handle.close();
    await expectLater(
      readNagaConfigFile(file),
      throwsA(isA<FormatException>()),
    );
    await file.writeAsBytes([0xff]);
    await expectLater(
      readNagaConfigFile(file),
      throwsA(isA<FormatException>()),
    );
  });
  test('rejects oversized config files', () {
    expect(
      () => nagaPickedConfigFromBytes(
        Uint8List(nagaMaxConfigFileBytes + 1),
        'big.json',
      ),
      throwsA(isA<FormatException>()),
    );
  });

  test('decodes utf8 config files', () {
    final picked = nagaPickedConfigFromBytes(
      Uint8List.fromList(utf8.encode('{"outbounds":[]}')),
      'naga.json',
    );
    expect(picked.fileName, 'naga.json');
    expect(picked.contents, '{"outbounds":[]}');
  });

  test('windows file dialog is allowed to take the foreground', () {
    final args = windowsConfigPickerArguments(windowsConfigPickerScript);
    expect(args, isNot(contains('Hidden')));
    expect(args, contains('-STA'));
    expect(windowsConfigPickerScript, contains('TopMost = \$true'));
    expect(windowsConfigPickerScript, contains('ShowDialog(\$owner)'));
  });
}
