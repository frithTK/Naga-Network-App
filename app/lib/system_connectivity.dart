import 'dart:async';
import 'dart:io';

/// Tests the application's ordinary system network path without a proxy.
/// A successful response does not establish which interface carried it.
Future<bool> checkSystemConnectivity({Uri? endpoint}) async {
  final client = HttpClient()..findProxy = (_) => 'DIRECT';
  try {
    for (var attempt = 0; attempt < 2; attempt++) {
      try {
        final result = await (() async {
          final request = await client.getUrl(
            endpoint ?? Uri.parse('https://www.gstatic.com/generate_204'),
          );
          request.followRedirects = false;
          final response = await request.close();
          return response.statusCode == HttpStatus.noContent;
        })().timeout(const Duration(seconds: 8));
        if (result) return true;
      } on Exception {
        // A second attempt handles transient network failures.
      }
    }
    return false;
  } finally {
    client.close(force: true);
  }
}
