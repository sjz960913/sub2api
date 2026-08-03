import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/core/network/panel_api_client.dart';
import 'package:sub2api_mobile/core/storage/secure_value_store.dart';
import 'package:sub2api_mobile/features/codex_sessions/data/collaboration_repository.dart';

void main() {
  test('command network retry reuses one idempotency key', () async {
    final client = _RetryingPanelClient();
    final repository = CollaborationRepository(client);

    final command = await repository.submitCommand(
      deviceId: 'device-1',
      threadId: 'thread-1',
      text: '继续任务',
    );

    expect(command.status, 'completed');
    expect(client.commandAttempts, 2);
    expect(client.idempotencyKeys.toSet(), hasLength(1));
    expect(client.idempotencyKeys.first, isNotEmpty);
    expect(client.idempotencyKeys[1], client.idempotencyKeys.first);
  });
}

class _RetryingPanelClient extends PanelApiClient {
  _RetryingPanelClient() : super(store: _MemorySecureValueStore());

  int commandAttempts = 0;
  final List<String> idempotencyKeys = [];

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
    if (method != 'POST' || path != 'collaboration/commands') {
      throw StateError('Unexpected request: $method $path');
    }
    commandAttempts++;
    idempotencyKeys.add(headers?['Idempotency-Key'] ?? '');
    if (commandAttempts == 1) {
      throw const PanelApiException('PANEL_NETWORK_ERROR');
    }
    return <String, dynamic>{
      'command_id': '018f7f3e-86f6-7cc8-98ec-4f56dc1f2322',
      'status': 'completed',
      'created_at': '2026-08-03T00:00:00Z',
    };
  }
}

class _MemorySecureValueStore implements SecureValueStore {
  final Map<String, String> values = {};

  @override
  Future<void> delete(String key) async {
    values.remove(key);
  }

  @override
  Future<String?> read(String key) async => values[key];

  @override
  Future<void> write(String key, String value) async {
    values[key] = value;
  }
}
