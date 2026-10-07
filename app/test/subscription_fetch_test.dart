import 'package:flutter_test/flutter_test.dart';
import 'package:naga_network/subscription_fetch.dart';

void main() {
  test('subscription host check rejects local and private targets', () {
    expect(
      subscriptionHostAllowed(Uri.parse('https://example.com/a.json')),
      isTrue,
    );
    for (final raw in [
      'http://example.com/a.json',
      'https://localhost/a.json',
      'https://panel.example/a.json',
      'https://127.0.0.1/a.json',
      'https://10.1.2.3/a.json',
      'https://192.168.0.1/a.json',
      'https://100.64.0.1/a.json',
      'https://[::1]/a.json',
    ]) {
      expect(subscriptionHostAllowed(Uri.parse(raw)), isFalse, reason: raw);
    }
  });

  test('downloaded profile body keeps the source URL and headers', () {
    const download = SubscriptionDownload(
      body: '{"outbounds":[]}',
      url: 'https://example.com/a.json',
      profileTitle: 'base64:TmFnYQ==',
      subscriptionUserinfo: 'upload=0; download=0; total=0; expire=0',
      profileUpdateInterval: '1',
      providerName: 'Naga',
    );
    final body = download.toJson();
    expect(body['config'], '{"outbounds":[]}');
    expect(body['url'], 'https://example.com/a.json');
    expect(body['profile_title'], 'base64:TmFnYQ==');
    expect(body.containsKey('url'), isTrue);

    final refresh = download.toJson(includeUrl: false);
    expect(refresh.containsKey('url'), isFalse);
    expect(refresh['provider_name'], 'Naga');
  });
}
