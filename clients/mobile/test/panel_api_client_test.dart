import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
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

  test('stringifies numeric query parameters before URI encoding', () {
    expect(
      PanelApiClient.stringifyQueryParameters({
        'page': 1,
        'page_size': 100,
        'status': 'active',
      }),
      {'page': '1', 'page_size': '100', 'status': 'active'},
    );
  });

  test('reads the account balance from the login user payload', () async {
    final result = await _LoginPanelClient().login(
      email: 'user@example.com',
      password: 'password',
    );

    expect(result.user?.balance, 18.75);
  });

  test('preserves a safe server reason for collaboration failures', () async {
    final dio = Dio(
      BaseOptions(
        responseType: ResponseType.json,
        validateStatus: (status) => status != null && status < 600,
      ),
    )..httpClientAdapter = _StaticResponseAdapter(
      statusCode: 503,
      body: {
        'code': 503,
        'message': 'Collaboration is disabled',
        'reason': 'COLLABORATION_DISABLED',
      },
    );
    final client = PanelApiClient(store: _MemoryStore(), dio: dio);

    await expectLater(
      client.request(
        'GET',
        'collaboration/devices',
        authenticated: false,
      ),
      throwsA(
        isA<PanelApiException>().having(
          (error) => error.publicCode,
          'publicCode',
          'COLLABORATION_DISABLED',
        ),
      ),
    );
  });
}

class _StaticResponseAdapter implements HttpClientAdapter {
  _StaticResponseAdapter({required this.statusCode, required this.body});

  final int statusCode;
  final Map<String, dynamic> body;

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    return ResponseBody.fromString(
      jsonEncode(body),
      statusCode,
      headers: {
        Headers.contentTypeHeader: [Headers.jsonContentType],
      },
    );
  }

  @override
  void close({bool force = false}) {}
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
