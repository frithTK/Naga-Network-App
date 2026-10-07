String? launchSubscriptionURL(List<String> args) {
  for (var i = args.length - 1; i >= 0; i--) {
    final url = subscriptionFetchURL(args[i]);
    if (url != null) {
      return url;
    }
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
      if (!_isInstallConfig(uri)) {
        return null;
      }
      final inner = uri.queryParameters['url']?.trim() ?? '';
      if (inner.isEmpty) {
        return null;
      }
      final nested = Uri.tryParse(inner);
      if (nested == null ||
          nested.scheme.toLowerCase() != 'https' ||
          nested.host.isEmpty) {
        return null;
      }
      return inner;
    default:
      return null;
  }
}

bool _isInstallConfig(Uri uri) {
  final host = uri.host.toLowerCase();
  final path = uri.path.replaceAll(RegExp(r'^/+|/+$'), '').toLowerCase();
  if (host == 'install-config' && (path.isEmpty || path == 'install-config')) {
    return true;
  }
  return host.isEmpty && path == 'install-config';
}
