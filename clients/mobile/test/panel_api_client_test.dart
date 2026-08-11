import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/core/network/panel_api_client.dart';
import 'package:sub2api_mobile/core/storage/secure_value_store.dart';

void main() {
  test('always restores the fixed production site', () async {
    final restored = await PanelApiClient(store: _MemoryStore()).restore();

    expect(restored.siteUrl, fixedPanelSiteUrl);
    expect(restored.user, isNull);
  });

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
      () => PanelApiClient.normalizeSiteUrl(
        'https://user:pass@panel.example.com',
      ),
      throwsA(isA<PanelApiException>()),
    );
  });

  test('reads the account balance from the login user payload', () async {
    final result = await _LoginPanelClient().login(
      email: 'user@example.com',
      password: 'password',
    );

    expect(result.user?.balance, 18.75);
  });
}

class _LoginPanelClient extends PanelApiClient {
  _LoginPanelClient() : super(store: _MemoryStore());

  @override
  Future<dynamic> request(
    String method,
    String path, {
    Object? data,
    Map<String, dynamic>? query,
    Map<String, String>? headers,
    bool authenticated = true,
    bool retryAfterRefresh = true,
  }) async {
    return {
      'access_token': 'access-token-for-test',
      'refresh_token': 'refresh-token-for-test',
      'user': {
        'id': 1,
        'email': 'user@example.com',
        'username': 'user',
        'role': 'user',
        'balance': 18.75,
      },
    };
  }
}

class _MemoryStore implements SecureValueStore {
  @override
  Future<void> delete(String key) async {}

  @override
  Future<String?> read(String key) async => null;

  @override
  Future<void> write(String key, String value) async {}
}
