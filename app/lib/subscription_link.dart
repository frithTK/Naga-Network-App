import 'dart:convert';

String? launchSubscriptionURL(List<String> args) {
  for (var i = args.length - 1; i >= 0; i--) {
    final url = subscriptionFetchURL(args[i]);
    if (url != null) {
      return url;
    }
    if (_nodeLink.hasMatch(args[i].trim())) return args[i].trim();
  }
  return null;
}

String? subscriptionFetchURL(String raw) {
  final trimmed = raw.trim();
  if (trimmed.isEmpty) {
    return null;
  }
  final uri = Uri.tryParse(trimmed);
  if (uri == null || uri.scheme.isEmpty) {
    return null;
  }
  switch (uri.scheme.toLowerCase()) {
    case 'https':
      if (uri.host.isEmpty) {
        return null;
      }
      return trimmed;
    case 'nagavpn':
    case 'clash':
    case 'v2rayng':
    case 'sing-box':
      final action = switch (uri.scheme.toLowerCase()) {
        'v2rayng' => 'install-sub',
        'sing-box' => 'import-remote-profile',
        _ => 'install-config',
      };
      if (_hasAction(uri, action)) {
        return _httpsTarget(uri.queryParameters['url'] ?? '');
      }
      return null;
    case 'hiddify':
      if (uri.host.toLowerCase() != 'import') return null;
      final prefix = 'hiddify://import/';
      if (!trimmed.toLowerCase().startsWith(prefix)) return null;
      final value = trimmed.substring(prefix.length).split('#').first;
      try {
        return _httpsTarget(value) ?? _httpsTarget(Uri.decodeComponent(value));
      } on FormatException {
        return null;
      }
    default:
      return null;
  }
}

String? _httpsTarget(String raw) {
  final value = raw.trim();
  final uri = Uri.tryParse(value);
  return uri?.scheme.toLowerCase() == 'https' && uri!.host.isNotEmpty
      ? value
      : null;
}

bool _hasAction(Uri uri, String action) {
  final host = uri.host.toLowerCase();
  final path = uri.path.replaceAll(RegExp(r'^/+|/+$'), '').toLowerCase();
  return host == action && path.isEmpty ||
      host.isEmpty && path == action ||
      action == 'install-config' && host == action && path == action;
}

final _nodeLink = RegExp(
  r'^\s*(vless|vmess|ss|ssr|trojan|hysteria|hysteria2|hy2|tuic|olcrtc|awg|wg|wireguard)://',
  caseSensitive: false,
  multiLine: true,
);

String? inlineProfileConfig(String raw) {
  final value = raw.trim();
  if (value.isEmpty) return null;
  if (_nodeLink.hasMatch(value) ||
      value.startsWith('{') ||
      value.startsWith('[Interface]') ||
      RegExp(r'^proxies\s*:', multiLine: true).hasMatch(value)) {
    return value;
  }
  if (value.length > 16 * 1024 * 1024 ||
      !RegExp(r'^[A-Za-z0-9_+=/\s-]+$').hasMatch(value)) {
    return null;
  }
  try {
    final decoded = utf8.decode(
      base64.decode(value.replaceAll(RegExp(r'\s'), '')),
    );
    return _nodeLink.hasMatch(decoded) ? value : null;
  } on FormatException {
    return null;
  }
}
