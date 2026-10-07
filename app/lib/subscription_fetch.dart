import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'subscription_link.dart';

class SubscriptionFetchException implements Exception {
  const SubscriptionFetchException(this.message);

  final String message;

  @override
  String toString() => message;
}

const _maxSubscriptionBytes = 16 * 1024 * 1024;

class SubscriptionDownload {
  const SubscriptionDownload({
    required this.body,
    required this.url,
    this.profileTitle = '',
    this.subscriptionUserinfo = '',
    this.profileUpdateInterval = '',
    this.providerName = '',
  });

  final String body;
  final String url;
  final String profileTitle;
  final String subscriptionUserinfo;
  final String profileUpdateInterval;
  final String providerName;

  Map<String, String> toJson({bool includeUrl = true}) {
    return {
      'config': body,
      if (includeUrl) 'url': url,
      if (profileTitle.isNotEmpty) 'profile_title': profileTitle,
      if (subscriptionUserinfo.isNotEmpty)
        'subscription_userinfo': subscriptionUserinfo,
      if (profileUpdateInterval.isNotEmpty)
        'profile_update_interval': profileUpdateInterval,
      if (providerName.isNotEmpty) 'provider_name': providerName,
    };
  }
}

/// Downloads a subscription with the UI process over Dart's HTTPS stack.
/// naga-control's Go client is a separate path and can be stalled on
/// networks where this one still connects.
Future<SubscriptionDownload> downloadSubscription(String rawUrl) async {
  final resolved = subscriptionFetchURL(rawUrl);
  if (resolved == null) {
    throw const SubscriptionFetchException(
      'Нужна HTTPS-ссылка профиля или nagavpn://install-config.',
    );
  }
  final initial = Uri.parse(resolved);
  if (!subscriptionHostAllowed(initial)) {
    throw const SubscriptionFetchException(
      'subscription URL host is not allowed',
    );
  }
  final client = HttpClient();
  client.connectionTimeout = const Duration(seconds: 20);
  client.userAgent = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36';
  try {
    var current = initial;
    for (var hop = 0; hop < 5; hop++) {
      if (!subscriptionHostAllowed(current)) {
        throw const SubscriptionFetchException(
          'subscription URL host is not allowed',
        );
      }
      final request = await client.getUrl(current);
      request.followRedirects = false;
      request.headers.set(
        HttpHeaders.acceptHeader,
        'application/json, text/plain, */*',
      );
      final response = await request.close().timeout(
        const Duration(seconds: 25),
      );
      if (_isRedirect(response.statusCode)) {
        final location = response.headers.value(HttpHeaders.locationHeader);
        await response.drain<void>();
        if (location == null || location.trim().isEmpty) {
          throw const SubscriptionFetchException(
            'subscription redirect is invalid',
          );
        }
        final next = current.resolve(location.trim());
        if (next.scheme.toLowerCase() != 'https') {
          throw const SubscriptionFetchException(
            'subscription redirect left HTTPS',
          );
        }
        current = next;
        continue;
      }
      if (response.statusCode < 200 || response.statusCode >= 300) {
        await response.drain<void>();
        throw SubscriptionFetchException(
          'subscription returned HTTP ${response.statusCode}',
        );
      }
      final builder = BytesBuilder(copy: false);
      await for (final chunk in response.timeout(const Duration(seconds: 25))) {
        builder.add(chunk);
        if (builder.length > _maxSubscriptionBytes) {
          throw const SubscriptionFetchException(
            'subscription exceeds 16777216 bytes',
          );
        }
      }
      return SubscriptionDownload(
        body: utf8.decode(builder.takeBytes()),
        url: resolved,
        profileTitle: response.headers.value('profile-title') ?? '',
        subscriptionUserinfo:
            response.headers.value('subscription-userinfo') ?? '',
        profileUpdateInterval:
            response.headers.value('profile-update-interval') ?? '',
        providerName: response.headers.value('isp-name') ?? '',
      );
    }
    throw const SubscriptionFetchException(
      'subscription exceeded redirect limit',
    );
  } on SubscriptionFetchException {
    rethrow;
  } on TimeoutException {
    throw const SubscriptionFetchException('subscription endpoint timed out');
  } on HandshakeException {
    throw const SubscriptionFetchException(
      'subscription tls certificate could not be verified',
    );
  } on TlsException {
    throw const SubscriptionFetchException(
      'subscription tls certificate could not be verified',
    );
  } on SocketException catch (error) {
    final text = error.message.toLowerCase();
    if (text.contains('timed out') || text.contains('timeout')) {
      throw const SubscriptionFetchException('subscription endpoint timed out');
    }
    if (text.contains('failed host lookup') || text.contains('no address')) {
      throw const SubscriptionFetchException('subscription dns lookup failed');
    }
    throw const SubscriptionFetchException('subscription endpoint blocked');
  } catch (error) {
    final text = error.toString().toLowerCase();
    if (text.contains('timed out') || text.contains('timeout')) {
      throw const SubscriptionFetchException('subscription endpoint timed out');
    }
    throw const SubscriptionFetchException('subscription endpoint unavailable');
  } finally {
    client.close(force: true);
  }
}

bool _isRedirect(int status) =>
    status == HttpStatus.movedPermanently ||
    status == HttpStatus.found ||
    status == HttpStatus.seeOther ||
    status == HttpStatus.temporaryRedirect ||
    status == HttpStatus.permanentRedirect;

bool subscriptionHostAllowed(Uri uri) {
  if (uri.scheme.toLowerCase() != 'https' || uri.host.isEmpty) {
    return false;
  }
  final host = uri.host.toLowerCase();
  if (host == 'localhost' || host.startsWith('localhost.')) {
    return false;
  }
  if (host.split('.').first == 'panel') {
    return false;
  }
  final ip = InternetAddress.tryParse(host);
  if (ip == null) {
    return true;
  }
  return !_blockedAddress(ip);
}

bool _blockedAddress(InternetAddress ip) {
  if (ip.isLoopback || ip.isLinkLocal || ip.isMulticast) {
    return true;
  }
  final raw = ip.rawAddress;
  if (raw.length == 4) {
    final a = raw[0];
    final b = raw[1];
    if (a == 0 || a == 10 || a == 127) return true;
    if (a == 100 && b >= 64 && b <= 127) return true;
    if (a == 169 && b == 254) return true;
    if (a == 172 && b >= 16 && b <= 31) return true;
    if (a == 192 && b == 168) return true;
    if (a >= 224) return true;
    return false;
  }
  if (raw.length == 16) {
    if (raw[0] == 0xFE && (raw[1] & 0xC0) == 0x80) return true;
    if ((raw[0] & 0xFE) == 0xFC) return true;
    // IPv4-mapped ::ffff:0:0/96
    const mapped = 10;
    if (raw.sublist(0, mapped).every((byte) => byte == 0) &&
        raw[10] == 0xFF &&
        raw[11] == 0xFF) {
      return _blockedAddress(InternetAddress.fromRawAddress(raw.sublist(12)));
    }
  }
  return false;
}
