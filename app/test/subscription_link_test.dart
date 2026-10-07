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
