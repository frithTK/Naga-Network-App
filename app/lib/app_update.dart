import 'dart:convert';
import 'dart:io';

import 'package:crypto/crypto.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:path/path.dart' as p;

const updateChannelName = 'eu.nagavpn.naga_network/update';
const githubReleasesRepo = 'frithTK/Naga-Network-App';
const embeddedAppVersion = String.fromEnvironment(
  'NAGA_VERSION',
  defaultValue: '1.2.5',
);

class AppUpdateException implements Exception {
  const AppUpdateException(this.message);
  final String message;

  @override
  String toString() => message;
}

class Semver implements Comparable<Semver> {
  const Semver(this.major, this.minor, this.patch);

  final int major;
  final int minor;
  final int patch;

  static Semver? tryParse(String raw) {
    var value = raw.trim();
    if (value.startsWith('v') || value.startsWith('V')) {
      value = value.substring(1);
    }
    final plus = value.indexOf('+');
    if (plus >= 0) {
      value = value.substring(0, plus);
    }
    final dash = value.indexOf('-');
    if (dash >= 0) {
      value = value.substring(0, dash);
    }
    final parts = value.split('.');
    if (parts.length < 2 || parts.length > 3) {
      return null;
    }
    final major = int.tryParse(parts[0]);
    final minor = int.tryParse(parts[1]);
    final patch = parts.length == 3 ? int.tryParse(parts[2]) : 0;
    if (major == null || minor == null || patch == null) {
      return null;
    }
    if (major < 0 || minor < 0 || patch < 0) {
      return null;
    }
    return Semver(major, minor, patch);
  }

  @override
  int compareTo(Semver other) {
    if (major != other.major) return major.compareTo(other.major);
    if (minor != other.minor) return minor.compareTo(other.minor);
    return patch.compareTo(other.patch);
  }

  bool operator <(Semver other) => compareTo(other) < 0;
  bool operator >(Semver other) => compareTo(other) > 0;
  bool operator <=(Semver other) => compareTo(other) <= 0;
  bool operator >=(Semver other) => compareTo(other) >= 0;

  @override
  String toString() => '$major.$minor.$patch';

  @override
  bool operator ==(Object other) =>
      other is Semver &&
      other.major == major &&
      other.minor == minor &&
      other.patch == patch;

  @override
  int get hashCode => Object.hash(major, minor, patch);
}

enum UpdateInstallKind { linuxAppImage, linuxDev, windowsPortable, windowsSetup, android }

class ReleaseAsset {
  const ReleaseAsset({
    required this.name,
    required this.url,
    this.digest = '',
    this.size = 0,
  });

  final String name;
  final String url;
  final String digest;
  final int size;

  factory ReleaseAsset.fromJson(Map<String, dynamic> json) {
    return ReleaseAsset(
      name: json['name'] as String? ?? '',
      url: json['browser_download_url'] as String? ?? '',
      digest: json['digest'] as String? ?? '',
      size: _jsonInt(json['size']),
    );
  }
}

class GithubRelease {
  const GithubRelease({
    required this.tagName,
    required this.assets,
    this.etag = '',
  });

  final String tagName;
  final List<ReleaseAsset> assets;
  final String etag;

  String get version => Semver.tryParse(tagName)?.toString() ?? tagName;

  factory GithubRelease.fromJson(Map<String, dynamic> json, {String etag = ''}) {
    final rawAssets = json['assets'];
    final assets = <ReleaseAsset>[];
    if (rawAssets is List) {
      for (final item in rawAssets) {
        if (item is Map<String, dynamic>) {
          assets.add(ReleaseAsset.fromJson(item));
        } else if (item is Map) {
          assets.add(ReleaseAsset.fromJson(Map<String, dynamic>.from(item)));
        }
      }
    }
    return GithubRelease(
      tagName: json['tag_name'] as String? ?? '',
      assets: List.unmodifiable(assets),
      etag: etag,
    );
  }

  ReleaseAsset? assetNamed(String name) {
    for (final asset in assets) {
      if (asset.name == name) return asset;
    }
    return null;
  }
}

class UpdateCheckResult {
  const UpdateCheckResult({
    required this.currentVersion,
    required this.latestVersion,
    required this.newerAvailable,
    this.release,
    this.missingAssets = const [],
  });

  final String currentVersion;
  final String latestVersion;
  final bool newerAvailable;
  final GithubRelease? release;
  final List<String> missingAssets;
}

String linuxAppImageName(String ver) => 'NagaNetwork-$ver-x86_64.AppImage';
String linuxControlName() => 'naga-control-linux-x86_64';
String windowsZipName(String ver) => 'NagaNetwork-$ver-windows-x64.zip';
String windowsSetupName(String ver) => 'NagaNetwork-$ver-windows-x64-setup.exe';
String androidApkName(String ver) => 'NagaNetwork-$ver-arm64-v8a.apk';

List<String> requiredAssetNames(UpdateInstallKind kind, String ver) {
  switch (kind) {
    case UpdateInstallKind.linuxAppImage:
      return [linuxAppImageName(ver), linuxControlName()];
    case UpdateInstallKind.linuxDev:
      return [linuxAppImageName(ver), linuxControlName()];
    case UpdateInstallKind.windowsPortable:
      return [windowsZipName(ver)];
    case UpdateInstallKind.windowsSetup:
      return [windowsSetupName(ver)];
    case UpdateInstallKind.android:
      return [androidApkName(ver)];
  }
}

String? parseSha256Digest(String raw) {
  var value = raw.trim().toLowerCase();
  if (value.isEmpty) return null;
  if (value.startsWith('sha256:')) {
    value = value.substring(7);
  }
  if (value.length != 64) return null;
  if (!RegExp(r'^[0-9a-f]{64}$').hasMatch(value)) return null;
  return value;
}

UpdateInstallKind detectInstallKind({
  Map<String, String>? environment,
  String? resolvedExecutable,
  bool? isLinux,
  bool? isWindows,
  bool? isAndroid,
}) {
  final linux = isLinux ?? Platform.isLinux;
  final windows = isWindows ?? Platform.isWindows;
  final android = isAndroid ?? Platform.isAndroid;
  final env = environment ?? Platform.environment;
  if (android) return UpdateInstallKind.android;
  if (windows) {
    final exe = resolvedExecutable ?? Platform.resolvedExecutable;
    final dir = File(exe).parent.path;
    final unins = File(p.join(dir, 'unins000.exe'));
    final lower = dir.toLowerCase();
    if (unins.existsSync() ||
        lower.contains('${p.separator}programs${p.separator}naga')) {
      return UpdateInstallKind.windowsSetup;
    }
    return UpdateInstallKind.windowsPortable;
  }
  if (linux) {
    final appImage = (env['APPIMAGE'] ?? '').trim();
    if (appImage.isNotEmpty) return UpdateInstallKind.linuxAppImage;
    return UpdateInstallKind.linuxDev;
  }
  return UpdateInstallKind.linuxDev;
}

Directory _systemTempDir() => Directory.systemTemp;

const autoAppUpdateInterval = Duration(hours: 12);

class AppUpdatePrefs {
  const AppUpdatePrefs({
    this.autoInstall = true,
    this.lastCheckUtc,
    this.lastLatestVersion = '',
  });

  final bool autoInstall;
  final DateTime? lastCheckUtc;
  final String lastLatestVersion;

  bool dueForCheck(DateTime now) {
    final previous = lastCheckUtc;
    if (previous == null) return true;
    return !now.difference(previous).isNegative &&
        now.difference(previous) >= autoAppUpdateInterval;
  }

  AppUpdatePrefs copyWith({
    bool? autoInstall,
    DateTime? lastCheckUtc,
    String? lastLatestVersion,
  }) {
    return AppUpdatePrefs(
      autoInstall: autoInstall ?? this.autoInstall,
      lastCheckUtc: lastCheckUtc ?? this.lastCheckUtc,
      lastLatestVersion: lastLatestVersion ?? this.lastLatestVersion,
    );
  }

  Map<String, Object?> toJson() {
    return {
      'auto_install': autoInstall,
      'last_check_utc': lastCheckUtc?.toUtc().toIso8601String(),
      'last_latest_version': lastLatestVersion,
    };
  }

  factory AppUpdatePrefs.fromJson(Map<String, dynamic> json) {
    return AppUpdatePrefs(
      autoInstall: json['auto_install'] as bool? ?? true,
      lastCheckUtc: DateTime.tryParse(json['last_check_utc'] as String? ?? ''),
      lastLatestVersion: json['last_latest_version'] as String? ?? '',
    );
  }
}

bool canAutoApplyUpdate({
  required UpdateCheckResult result,
  required UpdateInstallKind kind,
  required bool autoInstall,
}) {
  return autoInstall &&
      result.newerAvailable &&
      result.missingAssets.isEmpty &&
      kind != UpdateInstallKind.linuxDev;
}

bool runningUnderFlutterTest([Map<String, String>? environment]) {
  final env = environment ?? Platform.environment;
  return env['FLUTTER_TEST'] == 'true';
}

String? appUpdatePrefsPath({
  Map<String, String>? environment,
  bool? isWindows,
  bool? isAndroid,
}) {
  final env = environment ?? Platform.environment;
  final dataDir = env['NAGA_DATA_DIR']?.trim();
  if (dataDir != null && dataDir.isNotEmpty) {
    return p.join(dataDir, 'app-update.json');
  }
  if (isAndroid ?? Platform.isAndroid) {
    return null;
  }
  if (isWindows ?? Platform.isWindows) {
    final localAppData = env['LOCALAPPDATA']?.trim();
    if (localAppData == null || localAppData.isEmpty) return null;
    return p.join(localAppData, 'naga-network', 'app-update.json');
  }
  final dataHome = env['XDG_DATA_HOME']?.trim();
  if (dataHome != null && dataHome.isNotEmpty) {
    return p.join(dataHome, 'naga-network', 'app-update.json');
  }
  final home = env['HOME']?.trim();
  if (home == null || home.isEmpty) return null;
  return p.join(home, '.local', 'share', 'naga-network', 'app-update.json');
}

AppUpdatePrefs loadAppUpdatePrefs({
  Map<String, String>? environment,
  bool? isWindows,
  bool? isAndroid,
  String? path,
}) {
  final filePath =
      path ??
      appUpdatePrefsPath(
        environment: environment,
        isWindows: isWindows,
        isAndroid: isAndroid,
      );
  if (filePath == null) return const AppUpdatePrefs();
  try {
    final raw = File(filePath).readAsStringSync();
    final decoded = jsonDecode(raw);
    if (decoded is Map<String, dynamic>) {
      return AppUpdatePrefs.fromJson(decoded);
    }
    if (decoded is Map) {
      return AppUpdatePrefs.fromJson(Map<String, dynamic>.from(decoded));
    }
  } catch (_) {}
  return const AppUpdatePrefs();
}

Future<String?> resolveAppUpdatePrefsPath({
  Map<String, String>? environment,
  bool? isWindows,
  bool? isAndroid,
  MethodChannel? channel,
}) async {
  final path = appUpdatePrefsPath(
    environment: environment,
    isWindows: isWindows,
    isAndroid: isAndroid,
  );
  if (path != null) return path;
  if (!(isAndroid ?? Platform.isAndroid)) return null;
  try {
    final dir = await (channel ?? const MethodChannel(updateChannelName))
        .invokeMethod<String>('dataDir');
    if (dir == null || dir.trim().isEmpty) return null;
    return p.join(dir, 'app-update.json');
  } catch (_) {
    return null;
  }
}

void saveAppUpdatePrefs(
  AppUpdatePrefs prefs, {
  Map<String, String>? environment,
  bool? isWindows,
  bool? isAndroid,
  String? path,
}) {
  final filePath =
      path ??
      appUpdatePrefsPath(
        environment: environment,
        isWindows: isWindows,
        isAndroid: isAndroid,
      );
  if (filePath == null) return;
  final file = File(filePath);
  file.parent.createSync(recursive: true);
  file.writeAsStringSync('${jsonEncode(prefs.toJson())}\n');
}

bool controlPlaneBehind(String uiVersion, String controlVersion) {
  final ui = Semver.tryParse(uiVersion);
  final control = Semver.tryParse(controlVersion);
  if (ui == null || control == null) return false;
  return control < ui;
}

class AppUpdater {
  AppUpdater({
    http.Client? client,
    MethodChannel? channel,
    this.repo = githubReleasesRepo,
    this.currentVersion = embeddedAppVersion,
    UpdateInstallKind? kind,
    this.environment,
    this.resolvedExecutable,
    this.pid,
    Directory Function()? tempDirectory,
    this.now,
  }) : _client = client ?? http.Client(),
       _ownsClient = client == null,
       _channel = channel ?? const MethodChannel(updateChannelName),
       _tempDirectory = tempDirectory ?? _systemTempDir,
       kind = kind ?? detectInstallKind();

  final http.Client _client;
  final bool _ownsClient;
  final MethodChannel _channel;
  final String repo;
  final String currentVersion;
  final UpdateInstallKind kind;
  final Map<String, String>? environment;
  final String? resolvedExecutable;
  final int? pid;
  final Directory Function() _tempDirectory;
  final DateTime Function()? now;

  String? _etag;
  GithubRelease? _cachedRelease;
  GithubRelease? lastRelease;

  void close() {
    if (_ownsClient) _client.close();
  }

  String get userAgent => 'Naga-Network-App/$currentVersion';

  Uri get latestUri =>
      Uri.https('api.github.com', '/repos/$repo/releases/latest');

  Future<UpdateCheckResult> checkLatest() async {
    try {
      final headers = <String, String>{
        'User-Agent': userAgent,
        'Accept': 'application/vnd.github+json',
      };
      if (_etag != null && _etag!.isNotEmpty) {
        headers['If-None-Match'] = _etag!;
      }
      final response = await _client.get(latestUri, headers: headers);
      if (response.statusCode == 304 && _cachedRelease != null) {
        return _compare(_cachedRelease!);
      }
      if (response.statusCode == 403 || response.statusCode == 429) {
        throw const AppUpdateException(
          'GitHub временно ограничил запросы. Попробуйте позже.',
        );
      }
      if (response.statusCode == 404) {
        throw const AppUpdateException('Релиз на GitHub не найден.');
      }
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw const AppUpdateException(
          'Не удалось проверить обновление. Попробуйте позже.',
        );
      }
      final decoded = jsonDecode(response.body);
      if (decoded is! Map) {
        throw const AppUpdateException('GitHub вернул непонятный ответ.');
      }
      final etag = response.headers['etag'] ?? '';
      final release = GithubRelease.fromJson(
        Map<String, dynamic>.from(decoded),
        etag: etag,
      );
      _etag = etag;
      _cachedRelease = release;
      lastRelease = release;
      return _compare(release);
    } on AppUpdateException {
      rethrow;
    } on SocketException {
      throw const AppUpdateException('Нет сети. Проверьте соединение.');
    } on http.ClientException {
      throw const AppUpdateException('Нет сети. Проверьте соединение.');
    } catch (_) {
      throw const AppUpdateException(
        'Не удалось проверить обновление. Попробуйте позже.',
      );
    }
  }

  UpdateCheckResult _compare(GithubRelease release) {
    lastRelease = release;
    final latest = Semver.tryParse(release.tagName);
    final current = Semver.tryParse(currentVersion);
    if (latest == null || current == null) {
      throw const AppUpdateException('Не удалось сравнить номера версий.');
    }
    final newer = current < latest;
    final missing = <String>[];
    if (newer) {
      for (final name in requiredAssetNames(kind, latest.toString())) {
        if (release.assetNamed(name) == null) missing.add(name);
      }
    }
    return UpdateCheckResult(
      currentVersion: current.toString(),
      latestVersion: latest.toString(),
      newerAvailable: newer,
      release: release,
      missingAssets: missing,
    );
  }

  Future<List<File>> downloadRequired(
    GithubRelease release,
    String version, {
    void Function(int received, int total)? onProgress,
  }) async {
    if (kind == UpdateInstallKind.linuxDev) {
      throw const AppUpdateException(
        'Обновление из приложения доступно только для AppImage.',
      );
    }
    final names = requiredAssetNames(kind, version);
    if (names.isEmpty) {
      throw const AppUpdateException('Для этой платформы нет файла обновления.');
    }
    final assets = <ReleaseAsset>[];
    for (final name in names) {
      final asset = release.assetNamed(name);
      if (asset == null || asset.url.isEmpty) {
        throw AppUpdateException(
          'В релизе нет файла $name. UI без runtime не обновляем.',
        );
      }
      assets.add(asset);
    }
    final dir = Directory(
      p.join(
        _tempDirectory().path,
        'naga-update-${now?.call().millisecondsSinceEpoch ?? DateTime.now().millisecondsSinceEpoch}',
      ),
    );
    await dir.create(recursive: true);
    final files = <File>[];
    var receivedAll = 0;
    final totalAll = assets.fold<int>(0, (sum, asset) => sum + asset.size);
    for (final asset in assets) {
      final file = File(p.join(dir.path, asset.name));
      await _downloadAsset(
        asset,
        file,
        onProgress: (received, _) {
          onProgress?.call(receivedAll + received, totalAll);
        },
      );
      receivedAll += asset.size > 0 ? asset.size : await file.length();
      files.add(file);
    }
    return files;
  }

  Future<void> applyDownloaded(
    List<File> files, {
    required String version,
    required Future<bool> Function() disconnectIfNeeded,
    required Future<bool> Function(String expectedVersion) waitForControlVersion,
  }) async {
    if (kind == UpdateInstallKind.linuxDev) {
      throw const AppUpdateException(
        'Обновление из приложения доступно только для AppImage.',
      );
    }
    if (!await disconnectIfNeeded()) {
      throw const AppUpdateException('Сначала отключите VPN.');
    }
    switch (kind) {
      case UpdateInstallKind.linuxAppImage:
        await _applyLinux(files, version, waitForControlVersion);
      case UpdateInstallKind.windowsPortable:
        await _applyWindowsPortable(files);
      case UpdateInstallKind.windowsSetup:
        await _applyWindowsSetup(files);
      case UpdateInstallKind.android:
        await _applyAndroid(files);
      case UpdateInstallKind.linuxDev:
        throw const AppUpdateException(
          'Обновление из приложения доступно только для AppImage.',
        );
    }
  }

  Future<void> _applyLinux(
    List<File> files,
    String version,
    Future<bool> Function(String expectedVersion) waitForControlVersion,
  ) async {
    File? appImage;
    File? control;
    for (final file in files) {
      final name = p.basename(file.path);
      if (name == linuxAppImageName(version)) appImage = file;
      if (name == linuxControlName()) control = file;
    }
    if (appImage == null || control == null) {
      throw const AppUpdateException(
        'Не хватает AppImage или naga-control. UI без runtime не обновляем.',
      );
    }
    final env = environment ?? Platform.environment;
    final currentImage = (env['APPIMAGE'] ?? '').trim();
    if (currentImage.isEmpty) {
      throw const AppUpdateException(
        'Обновление из приложения доступно только для AppImage.',
      );
    }
    final destImage = p.join(
      File(currentImage).parent.path,
      linuxAppImageName(version),
    );
    await appImage.copy(destImage);
    await Process.run('chmod', ['+x', destImage]);
    final home = (env['HOME'] ?? '').trim();
    if (home.isEmpty) {
      throw const AppUpdateException('Не удалось определить домашний каталог.');
    }
    final binDir = Directory(p.join(home, '.local', 'bin'));
    await binDir.create(recursive: true);
    final destControl = p.join(binDir.path, 'naga-control');
    final staged = '$destControl.new';
    await control.copy(staged);
    await File(staged).rename(destControl);
    await Process.run('chmod', ['+x', destControl]);
    final user = (env['USER'] ?? env['LOGNAME'] ?? '').trim();
    if (user.isEmpty) {
      throw const AppUpdateException('Не удалось определить имя пользователя.');
    }
    final unit = 'naga-network-control@$user.service';
    try {
      await _channel.invokeMethod<void>('restartSystemd', {'unit': unit});
    } on MissingPluginException {
      final result = await Process.run('pkexec', ['systemctl', 'restart', unit]);
      if (result.exitCode != 0) {
        throw AppUpdateException(
          'Нужны права администратора. Выполните:\n'
          'pkexec systemctl restart $unit',
        );
      }
    } on PlatformException {
      throw AppUpdateException(
        'Нужны права администратора. Выполните:\n'
        'pkexec systemctl restart $unit',
      );
    }
    if (!await waitForControlVersion(version)) {
      throw AppUpdateException(
        'naga-control не поднялся с версией $version. Выполните:\n'
        'pkexec systemctl restart $unit',
      );
    }
    try {
      await _channel.invokeMethod<void>('launchAndQuit', {'path': destImage});
    } on MissingPluginException {
      await Process.start(
        destImage,
        const [],
        mode: ProcessStartMode.detached,
      );
      exit(0);
    }
  }

  Future<void> _applyWindowsPortable(List<File> files) async {
    final zip = files.firstWhere(
      (file) => p.basename(file.path).endsWith('.zip'),
      orElse: () => throw const AppUpdateException('В загрузке нет zip.'),
    );
    final exe = resolvedExecutable ?? Platform.resolvedExecutable;
    final installDir = File(exe).parent.path;
    final helper = p.join(installDir, 'naga-update.exe');
    if (!File(helper).existsSync()) {
      throw const AppUpdateException(
        'Рядом с приложением нет naga-update.exe.',
      );
    }
    try {
      await _channel.invokeMethod<void>('applyWindowsPortable', {
        'helperPath': helper,
        'zipPath': zip.path,
        'installDir': installDir,
        'launch': 'NagaNetwork.exe',
        'pid': pid ?? pidFromIo(),
      });
    } on MissingPluginException {
      throw const AppUpdateException(
        'Этот сборщик UI не умеет заменять файлы на Windows.',
      );
    }
  }

  Future<void> _applyWindowsSetup(List<File> files) async {
    final setup = files.firstWhere(
      (file) => p.basename(file.path).toLowerCase().endsWith('.exe'),
      orElse: () => throw const AppUpdateException('В загрузке нет setup.exe.'),
    );
    try {
      await _channel.invokeMethod<void>('applyWindowsSetup', {
        'setupPath': setup.path,
      });
    } on MissingPluginException {
      await Process.start(
        setup.path,
        const [],
        mode: ProcessStartMode.detached,
      );
    }
  }

  Future<void> _applyAndroid(List<File> files) async {
    final apk = files.firstWhere(
      (file) => p.basename(file.path).toLowerCase().endsWith('.apk'),
      orElse: () => throw const AppUpdateException('В загрузке нет APK.'),
    );
    try {
      await _channel.invokeMethod<void>('applyAndroid', {'apkPath': apk.path});
    } on MissingPluginException {
      throw const AppUpdateException(
        'Установка APK недоступна в этой сборке.',
      );
    } on PlatformException catch (error) {
      throw AppUpdateException(
        error.message ?? 'Не удалось запустить установку APK.',
      );
    }
  }

  Future<void> _downloadAsset(
    ReleaseAsset asset,
    File dest, {
    void Function(int received, int total)? onProgress,
  }) async {
    final request = http.Request('GET', Uri.parse(asset.url));
    request.headers['User-Agent'] = userAgent;
    request.headers['Accept'] = 'application/octet-stream';
    late http.StreamedResponse response;
    try {
      response = await _client.send(request);
    } on SocketException {
      throw const AppUpdateException('Нет сети. Проверьте соединение.');
    }
    if (response.statusCode == 403 || response.statusCode == 429) {
      throw const AppUpdateException(
        'GitHub временно ограничил запросы. Попробуйте позже.',
      );
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw AppUpdateException('Не удалось скачать ${asset.name}.');
    }
    final sink = dest.openWrite();
    var received = 0;
    final total = response.contentLength ?? asset.size;
    final digest = AccumulatorSink<Digest>();
    final hashSink = sha256.startChunkedConversion(digest);
    try {
      await for (final chunk in response.stream) {
        sink.add(chunk);
        hashSink.add(chunk);
        received += chunk.length;
        onProgress?.call(received, total);
      }
      await sink.close();
      hashSink.close();
    } catch (error) {
      await sink.close();
      hashSink.close();
      rethrow;
    }
    final expected = parseSha256Digest(asset.digest);
    if (expected != null) {
      final actual = digest.events.single.toString();
      if (actual != expected) {
        await dest.delete();
        throw const AppUpdateException(
          'Контрольная сумма файла не совпала. Обновление прервано.',
        );
      }
    }
  }
}

int pidFromIo() => pid;

class AccumulatorSink<T> implements Sink<T> {
  final List<T> events = [];
  @override
  void add(T data) => events.add(data);
  @override
  void close() {}
}

int _jsonInt(Object? value) {
  if (value is int) return value;
  if (value is String) return int.tryParse(value) ?? 0;
  return 0;
}
