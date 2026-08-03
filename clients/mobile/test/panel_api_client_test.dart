import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/core/network/panel_api_client.dart';

void main() {
  test('normalizes secure site URLs and preserves a path prefix', () {
    expect(
      PanelApiClient.normalizeSiteUrl('https://panel.example.com/base'),
      'https://panel.example.com/base/',
    );
    expect(
      PanelApiClient.normalizeSiteUrl('http://127.0.0.1:8080'),
      'http://127.0.0.1:8080/',
    );
  });

  test('rejects insecure remote sites and URL credentials', () {
    expect(
      () => PanelApiClient.normalizeSiteUrl('http://panel.example.com'),
      throwsA(isA<PanelApiException>()),
    );
    expect(
      () => PanelApiClient.normalizeSiteUrl('https://user:pass@panel.example.com'),
      throwsA(isA<PanelApiException>()),
    );
  });
}
