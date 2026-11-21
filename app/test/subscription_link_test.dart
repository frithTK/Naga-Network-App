import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/subscription_link.dart';

void main() {
  test('keeps HTTPS subscription URLs', () {
    expect(
      subscriptionFetchURL(
        ' https://nagavpn.example/bundles/nagavpn/uuid.json ',
      ),
      'https://nagavpn.example/bundles/nagavpn/uuid.json',
    );
  });

  test('unwraps nagavpn install-config deep-link', () {
    expect(
      subscriptionFetchURL(
        'nagavpn://install-config?url=https%3A%2F%2Fnagavpn.example%2Fbundles%2Fnagavpn%2Fuuid.json',
      ),
      'https://nagavpn.example/bundles/nagavpn/uuid.json',
    );
  });

  test('does not turn an olcrtc uri into a fetch URL', () {
    expect(
      subscriptionFetchURL(
        'olcrtc://jitsi?datachannel@https://meet.example/room#0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef',
      ),
      isNull,
    );
  });

  test('unwraps supported client links without changing their HTTPS query', () {
    const target = 'https://example.invalid/a?token=test&format=json';
    final encoded = Uri.encodeComponent(target);
    for (final value in [
      'clash://install-config?url=$encoded',
      'v2rayng://install-sub?url=$encoded',
      'sing-box://import-remote-profile?url=$encoded',
      'hiddify://import/$encoded#profile',
      'hiddify://import/$target#profile',
    ]) {
      expect(subscriptionFetchURL(value), target, reason: value);
    }
  });

  test('keeps pasted proxy links as config and never fetches them as URLs', () {
    const list = 'vless://test@example.invalid:443\nhy2://test@example.invalid';
    expect(inlineProfileConfig(list), list);
    expect(subscriptionFetchURL(list), isNull);
  });

  test('rejects http and foreign schemes', () {
    expect(subscriptionFetchURL('http://example.invalid/a.json'), isNull);
    expect(
      subscriptionFetchURL(
        'karing://install-config?url=https://example.invalid/a.json',
      ),
      isNull,
    );
  });

  test('reads the last launch argument that is a subscription URL', () {
    expect(
      launchSubscriptionURL(const [
        '--enable-software-rendering',
        'nagavpn://install-config?url=https://nagavpn.example/bundles/nagavpn/uuid.json',
      ]),
      'https://nagavpn.example/bundles/nagavpn/uuid.json',
    );
  });
}
