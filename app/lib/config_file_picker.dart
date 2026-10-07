import 'dart:convert';
import 'dart:io';

import 'package:flutter/services.dart';

const nagaMaxConfigFileBytes = 16 * 1024 * 1024;

class NagaPickedConfig {
  const NagaPickedConfig({required this.contents, required this.fileName});

  final String contents;
  final String fileName;
}

NagaPickedConfig nagaPickedConfigFromBytes(Uint8List bytes, String fileName) {
  if (bytes.length > nagaMaxConfigFileBytes) {
    throw const FormatException('config file is too large');
  }
  return NagaPickedConfig(contents: utf8.decode(bytes), fileName: fileName);
}

Future<NagaPickedConfig?> pickNagaConfigFile() async {
  final path = await _pickConfigPath();
  if (path == null || path.trim().isEmpty) {
    return null;
  }
  final file = File(path);
  if (!file.existsSync()) {
    return null;
  }
  final bytes = await file.readAsBytes();
  return nagaPickedConfigFromBytes(
    Uint8List.fromList(bytes),
    file.uri.pathSegments.last,
  );
}

Future<String?> _pickConfigPath() async {
  if (Platform.isWindows) {
    return _pickWindowsPath();
  }
  if (Platform.isLinux) {
    return _pickLinuxPath();
  }
  return null;
}

List<String> windowsConfigPickerArguments(String script) {
  return <String>[
    '-NoProfile',
    '-STA',
    '-ExecutionPolicy',
    'Bypass',
    '-Command',
    script,
  ];
}

const windowsConfigPickerScript = r'''
Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form
$owner.TopMost = $true
$owner.ShowInTaskbar = $false
$owner.FormBorderStyle = [System.Windows.Forms.FormBorderStyle]::None
$owner.Opacity = 0
$owner.StartPosition = 'CenterScreen'
$owner.Show()
$owner.Activate()
$dialog = New-Object System.Windows.Forms.OpenFileDialog
$dialog.Filter = 'VPN config (*.json;*.conf;*.txt)|*.json;*.conf;*.txt|All files (*.*)|*.*'
$dialog.Title = 'Naga Network'
$dialog.CheckFileExists = $true
$dialog.Multiselect = $false
$result = $dialog.ShowDialog($owner)
$owner.Close()
if ($result -ne [System.Windows.Forms.DialogResult]::OK) { exit 1 }
[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::Out.Write($dialog.FileName)
''';

const configFileChannel = MethodChannel('eu.nagavpn.naga_network/tray');

Future<String?> _pickWindowsPath() async {
  try {
    final path = await configFileChannel.invokeMethod<String>('pickConfigFile');
    final trimmed = path?.trim() ?? '';
    if (trimmed.isNotEmpty) {
      return trimmed;
    }
    return null;
  } on MissingPluginException {
    // Tests and non-Windows hosts keep the PowerShell dialog.
  }
  final result = await Process.run(
    'powershell.exe',
    windowsConfigPickerArguments(windowsConfigPickerScript),
    stdoutEncoding: utf8,
    stderrEncoding: utf8,
  );
  if (result.exitCode != 0) {
    final error = result.stderr.toString().trim();
    if (error.isNotEmpty) {
      throw FormatException(error);
    }
    return null;
  }
  final path = result.stdout.toString().trim();
  return path.isEmpty ? null : path;
}

Future<String?> _pickLinuxPath() async {
  final commands = <List<String>>[
    <String>[
      'zenity',
      '--file-selection',
      '--title=Naga Network',
      '--file-filter=VPN config | *.json *.conf *.txt',
    ],
    <String>['kdialog', '--getopenfilename', '.', '*.json *.conf *.txt'],
  ];
  for (final command in commands) {
    try {
      final result = await Process.run(
        command.first,
        command.sublist(1),
        stdoutEncoding: utf8,
        stderrEncoding: utf8,
      );
      if (result.exitCode != 0) {
        continue;
      }
      final path = result.stdout.toString().trim();
      if (path.isNotEmpty) {
        return path;
      }
    } on ProcessException {
      continue;
    }
  }
  return null;
}
