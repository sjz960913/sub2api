import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/core/network/panel_api_client.dart';
import 'package:sub2api_mobile/core/storage/secure_value_store.dart';
import 'package:sub2api_mobile/features/api_keys/data/api_key_repository.dart';

void main() {
  test('keeps returned keys when group metadata cannot be loaded', () async {
    final repository = ApiKeyRepository(_FallbackPanelClient());

    final keys = await repository.listOpenAIKeys();

    expect(keys, hasLength(1));
    expect(keys.single.name, 'Mobile Chat');
    expect(keys.single.group, 'OpenAI 默认');
    expect(keys.single.availableGroups, ['OpenAI 默认']);
  });
}

class _FallbackPanelClient extends PanelApiClient {
  _FallbackPanelClient() : super(store: _MemoryStore());

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
    if (path == 'groups/available') {
      throw const PanelApiException('PANEL_RATE_LIMITED');
    }
    return {
      'items': [
        {
          'id': 7,
          'name': 'Mobile Chat',
          'key': 'sk-mobile-chat-test',
          'status': 'active',
          'quota_used': 1.25,
          'group': {
            'id': 3,
            'name': 'OpenAI 默认',
            'platform': 'openai',
            'status': 'active',
            'allow_image_generation': false,
          },
        },
      ],
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
