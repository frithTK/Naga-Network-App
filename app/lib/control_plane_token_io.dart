import 'dart:io';

String? loadControlPlaneToken({
  Map<String, String>? environment,
  bool? isWindows,
}) {
  final env = environment ?? Platform.environment;
  final fromEnvironment = env['NAGA_CONTROL_TOKEN']?.trim();
  if (fromEnvironment != null && fromEnvironment.isNotEmpty) {
    return fromEnvironment;
  }

  final path = controlPlaneTokenFilePath(
    environment: env,
    isWindows: isWindows ?? Platform.isWindows,
  );
  if (path == null) return null;

  try {
    final token = File(path).readAsStringSync().trim();
    return token.isEmpty ? null : token;
  } on IOException {
    return null;
  }
}

String? controlPlaneTokenFilePath({
  Map<String, String>? environment,
  bool? isWindows,
}) {
  final env = environment ?? Platform.environment;
  final configuredPath = env['NAGA_CONTROL_TOKEN_FILE']?.trim();
  if (configuredPath != null && configuredPath.isNotEmpty) {
    return configuredPath;
  }

  final dataDir = env['NAGA_DATA_DIR']?.trim();
  if (dataDir != null && dataDir.isNotEmpty) {
    return '$dataDir${Platform.pathSeparator}control.token';
  }

  if (isWindows ?? Platform.isWindows) {
    final localAppData = env['LOCALAPPDATA']?.trim();
    if (localAppData == null || localAppData.isEmpty) {
      return null;
    }
    return '$localAppData${Platform.pathSeparator}naga-network${Platform.pathSeparator}control.token';
  }

  final home = env['HOME']?.trim();
  final dataHome = env['XDG_DATA_HOME']?.trim();
  if (dataHome != null && dataHome.isNotEmpty) {
    return '$dataHome/naga-network/control.token';
  }
  if (home == null || home.isEmpty) {
    return null;
  }
  return '$home/.local/share/naga-network/control.token';
}
