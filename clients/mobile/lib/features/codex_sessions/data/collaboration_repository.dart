import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/idempotency_key.dart';
import '../../../core/network/panel_api_client.dart';
import '../../../core/protocol/collaboration_wire.dart';

final collaborationRepositoryProvider = Provider<CollaborationRepository>(
  (ref) => CollaborationRepository(ref.watch(panelApiClientProvider)),
);

class CollaborationRepositoryException implements Exception {
  const CollaborationRepositoryException(this.publicCode);

  final String publicCode;
}

class CollaborationRepository {
  const CollaborationRepository(this._client);

  final PanelApiClient _client;

  Future<List<Device>> listDevices() async {
    try {
      final data = _asMap(
        await _client.request('GET', 'collaboration/devices'),
      );
      final items = data['items'];
      if (items is! List) {
        throw const CollaborationRepositoryException('COLLAB_INVALID_RESPONSE');
      }
      return items
          .map((item) => Device.fromJson(_asMap(item)))
          .where((device) => device.capabilities.threadRead)
          .toList(growable: false);
    } on PanelApiException catch (error) {
      throw CollaborationRepositoryException(error.publicCode);
    }
  }

  Future<SessionSync> syncSessions({
    required String deviceId,
    String? searchTerm,
  }) async {
    try {
      final accepted = SyncAccepted.fromJson(
        _asMap(
          await _client.request(
            'POST',
            'collaboration/devices/${Uri.encodeComponent(deviceId)}/session-syncs',
            headers: {'Idempotency-Key': newIdempotencyKey()},
            data: SessionSyncRequest(
              searchTerm: searchTerm?.trim().isEmpty == true
                  ? null
                  : searchTerm?.trim(),
              archived: false,
              limit: 100,
            ).toJson(),
          ),
        ),
      );
      return _pollSessionSync(accepted.syncId);
    } on PanelApiException catch (error) {
      throw CollaborationRepositoryException(error.publicCode);
    }
  }

  Future<ThreadSync> syncThread({
    required String deviceId,
    required String threadId,
    String? afterItemId,
  }) async {
    try {
      final accepted = SyncAccepted.fromJson(
        _asMap(
          await _client.request(
            'POST',
            'collaboration/devices/${Uri.encodeComponent(deviceId)}/threads/'
                '${Uri.encodeComponent(threadId)}/syncs',
            headers: {'Idempotency-Key': newIdempotencyKey()},
            data: ThreadSyncRequest(
              afterItemId: afterItemId,
              limit: 200,
              includeToolOutput: 'summary',
            ).toJson(),
          ),
        ),
      );
      return _pollThreadSync(accepted.syncId);
    } on PanelApiException catch (error) {
      throw CollaborationRepositoryException(error.publicCode);
    }
  }

  Future<Command> submitCommand({
    required String deviceId,
    required String threadId,
    required String text,
  }) async {
    final prompt = text.trim();
    if (prompt.isEmpty || prompt.length > 32768) {
      throw const CollaborationRepositoryException('COLLAB_INVALID_COMMAND');
    }
    final idempotencyKey = newIdempotencyKey();
    try {
      dynamic response;
      for (var attempt = 0; attempt < 2; attempt++) {
        try {
          response = await _client.request(
            'POST',
            'collaboration/commands',
            headers: {'Idempotency-Key': idempotencyKey},
            data: CreateCommandRequest(
              deviceId: deviceId,
              threadId: threadId,
              input: [CommandInput(type: 'text', text: prompt)],
              clientContext: const ClientContext(
                locale: 'zh-CN',
                source: 'android',
              ),
            ).toJson(),
          );
          break;
        } on PanelApiException catch (error) {
          if (attempt == 0 && error.publicCode == 'PANEL_NETWORK_ERROR') {
            await Future<void>.delayed(const Duration(milliseconds: 250));
            continue;
          }
          rethrow;
        }
      }
      final command = Command.fromJson(_asMap(response));
      return _pollCommand(command);
    } on PanelApiException catch (error) {
      throw CollaborationRepositoryException(error.publicCode);
    }
  }

  Future<SessionSync> _pollSessionSync(String syncId) async {
    for (var attempt = 0; attempt < 40; attempt++) {
      final sync = SessionSync.fromJson(
        _asMap(
          await _client.request(
            'GET',
            'collaboration/session-syncs/${Uri.encodeComponent(syncId)}',
          ),
        ),
      );
      if (sync.status == 'completed') {
        return sync;
      }
      if (sync.status == 'failed' || sync.status == 'expired') {
        throw const CollaborationRepositoryException('COLLAB_SYNC_FAILED');
      }
      await Future<void>.delayed(const Duration(milliseconds: 500));
    }
    throw const CollaborationRepositoryException('COLLAB_SYNC_TIMEOUT');
  }

  Future<ThreadSync> _pollThreadSync(String syncId) async {
    for (var attempt = 0; attempt < 40; attempt++) {
      final sync = ThreadSync.fromJson(
        _asMap(
          await _client.request(
            'GET',
            'collaboration/thread-syncs/${Uri.encodeComponent(syncId)}',
          ),
        ),
      );
      if (sync.status == 'completed') {
        return sync;
      }
      if (sync.status == 'failed' || sync.status == 'expired') {
        throw const CollaborationRepositoryException('COLLAB_SYNC_FAILED');
      }
      await Future<void>.delayed(const Duration(milliseconds: 500));
    }
    throw const CollaborationRepositoryException('COLLAB_SYNC_TIMEOUT');
  }

  Future<Command> _pollCommand(Command command) async {
    var current = command;
    for (var attempt = 0; attempt < 120; attempt++) {
      if (current.status == 'completed') {
        return current;
      }
      if (current.status == 'failed' || current.status == 'expired') {
        throw const CollaborationRepositoryException('COLLAB_COMMAND_FAILED');
      }
      await Future<void>.delayed(const Duration(seconds: 1));
      current = Command.fromJson(
        _asMap(
          await _client.request(
            'GET',
            'collaboration/commands/${Uri.encodeComponent(current.commandId)}',
          ),
        ),
      );
    }
    throw const CollaborationRepositoryException('COLLAB_COMMAND_TIMEOUT');
  }

  static Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return value.map((key, item) => MapEntry(key.toString(), item));
    }
    throw const CollaborationRepositoryException('COLLAB_INVALID_RESPONSE');
  }
}
