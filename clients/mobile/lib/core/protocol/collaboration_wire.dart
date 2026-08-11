// GENERATED FILE. Source digest: d2030c717713d2dc
// Run: node protocol/scripts/generate.mjs

class CollaborationHealth {
  final bool enabled;
  final int protocolVersion;
  final int heartbeatIntervalSeconds;
  final String websocketPath;

  const CollaborationHealth({
    required this.enabled,
    required this.protocolVersion,
    required this.heartbeatIntervalSeconds,
    required this.websocketPath,
  });

  factory CollaborationHealth.fromJson(Map<String, dynamic> json) =>
      CollaborationHealth(
        enabled: json['enabled'] as bool,
        protocolVersion: json['protocol_version'] as int,
        heartbeatIntervalSeconds: json['heartbeat_interval_seconds'] as int,
        websocketPath: json['websocket_path'] as String,
      );

  Map<String, dynamic> toJson() => {
    'enabled': enabled,
    'protocol_version': protocolVersion,
    'heartbeat_interval_seconds': heartbeatIntervalSeconds,
    'websocket_path': websocketPath,
  };
}

class DeviceCapabilities {
  final bool appServer;
  final bool threadRead;
  final bool threadWrite;
  final bool imageInput;

  const DeviceCapabilities({
    required this.appServer,
    required this.threadRead,
    required this.threadWrite,
    required this.imageInput,
  });

  factory DeviceCapabilities.fromJson(Map<String, dynamic> json) =>
      DeviceCapabilities(
        appServer: json['app_server'] as bool,
        threadRead: json['thread_read'] as bool,
        threadWrite: json['thread_write'] as bool,
        imageInput: json['image_input'] as bool,
      );

  Map<String, dynamic> toJson() => {
    'app_server': appServer,
    'thread_read': threadRead,
    'thread_write': threadWrite,
    'image_input': imageInput,
  };
}

class RegisterDeviceRequest {
  final String installationIdHash;
  final String name;
  final String platform;
  final String? platformVersion;
  final String companionVersion;
  final String? codexVersion;
  final int protocolVersion;
  final DeviceCapabilities capabilities;

  const RegisterDeviceRequest({
    required this.installationIdHash,
    required this.name,
    required this.platform,
    this.platformVersion,
    required this.companionVersion,
    this.codexVersion,
    required this.protocolVersion,
    required this.capabilities,
  });

  factory RegisterDeviceRequest.fromJson(Map<String, dynamic> json) =>
      RegisterDeviceRequest(
        installationIdHash: json['installation_id_hash'] as String,
        name: json['name'] as String,
        platform: json['platform'] as String,
        platformVersion: json['platform_version'] as String?,
        companionVersion: json['companion_version'] as String,
        codexVersion: json['codex_version'] as String?,
        protocolVersion: json['protocol_version'] as int,
        capabilities: DeviceCapabilities.fromJson(
          json['capabilities'] as Map<String, dynamic>,
        ),
      );

  Map<String, dynamic> toJson() => {
    'installation_id_hash': installationIdHash,
    'name': name,
    'platform': platform,
    'platform_version': platformVersion,
    'companion_version': companionVersion,
    'codex_version': codexVersion,
    'protocol_version': protocolVersion,
    'capabilities': capabilities.toJson(),
  };
}

class RegisterDeviceResult {
  final String deviceId;
  final int heartbeatIntervalSeconds;
  final int eventProtocolVersion;
  final String serverTime;

  const RegisterDeviceResult({
    required this.deviceId,
    required this.heartbeatIntervalSeconds,
    required this.eventProtocolVersion,
    required this.serverTime,
  });

  factory RegisterDeviceResult.fromJson(Map<String, dynamic> json) =>
      RegisterDeviceResult(
        deviceId: json['device_id'] as String,
        heartbeatIntervalSeconds: json['heartbeat_interval_seconds'] as int,
        eventProtocolVersion: json['event_protocol_version'] as int,
        serverTime: json['server_time'] as String,
      );

  Map<String, dynamic> toJson() => {
    'device_id': deviceId,
    'heartbeat_interval_seconds': heartbeatIntervalSeconds,
    'event_protocol_version': eventProtocolVersion,
    'server_time': serverTime,
  };
}

class Device {
  final String id;
  final String name;
  final String platform;
  final String? companionVersion;
  final String? codexVersion;
  final String status;
  final String? lastSeenAt;
  final DeviceCapabilities capabilities;

  const Device({
    required this.id,
    required this.name,
    required this.platform,
    this.companionVersion,
    this.codexVersion,
    required this.status,
    this.lastSeenAt,
    required this.capabilities,
  });

  factory Device.fromJson(Map<String, dynamic> json) => Device(
    id: json['id'] as String,
    name: json['name'] as String,
    platform: json['platform'] as String,
    companionVersion: json['companion_version'] as String?,
    codexVersion: json['codex_version'] as String?,
    status: json['status'] as String,
    lastSeenAt: json['last_seen_at'] as String?,
    capabilities: DeviceCapabilities.fromJson(
      json['capabilities'] as Map<String, dynamic>,
    ),
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'platform': platform,
    'companion_version': companionVersion,
    'codex_version': codexVersion,
    'status': status,
    'last_seen_at': lastSeenAt,
    'capabilities': capabilities.toJson(),
  };
}

class RenameDeviceRequest {
  final String name;

  const RenameDeviceRequest({required this.name});

  factory RenameDeviceRequest.fromJson(Map<String, dynamic> json) =>
      RenameDeviceRequest(name: json['name'] as String);

  Map<String, dynamic> toJson() => {'name': name};
}

class SessionSyncRequest {
  final String? searchTerm;
  final String? cwd;
  final bool? archived;
  final String? cursor;
  final int? limit;

  const SessionSyncRequest({
    this.searchTerm,
    this.cwd,
    this.archived,
    this.cursor,
    this.limit,
  });

  factory SessionSyncRequest.fromJson(Map<String, dynamic> json) =>
      SessionSyncRequest(
        searchTerm: json['search_term'] as String?,
        cwd: json['cwd'] as String?,
        archived: json['archived'] as bool?,
        cursor: json['cursor'] as String?,
        limit: json['limit'] as int?,
      );

  Map<String, dynamic> toJson() => {
    'search_term': searchTerm,
    'cwd': cwd,
    'archived': archived,
    'cursor': cursor,
    'limit': limit,
  };
}

class ThreadSyncRequest {
  final String? afterItemId;
  final String? cursor;
  final int? limit;
  final String? includeToolOutput;

  const ThreadSyncRequest({
    this.afterItemId,
    this.cursor,
    this.limit,
    this.includeToolOutput,
  });

  factory ThreadSyncRequest.fromJson(Map<String, dynamic> json) =>
      ThreadSyncRequest(
        afterItemId: json['after_item_id'] as String?,
        cursor: json['cursor'] as String?,
        limit: json['limit'] as int?,
        includeToolOutput: json['include_tool_output'] as String?,
      );

  Map<String, dynamic> toJson() => {
    'after_item_id': afterItemId,
    'cursor': cursor,
    'limit': limit,
    'include_tool_output': includeToolOutput,
  };
}

class SyncAccepted {
  final String syncId;
  final String status;
  final String expiresAt;

  const SyncAccepted({
    required this.syncId,
    required this.status,
    required this.expiresAt,
  });

  factory SyncAccepted.fromJson(Map<String, dynamic> json) => SyncAccepted(
    syncId: json['sync_id'] as String,
    status: json['status'] as String,
    expiresAt: json['expires_at'] as String,
  );

  Map<String, dynamic> toJson() => {
    'sync_id': syncId,
    'status': status,
    'expires_at': expiresAt,
  };
}

class CodexSession {
  final String threadId;
  final String title;
  final String? preview;
  final String? cwdDisplay;
  final String? createdAt;
  final String updatedAt;
  final String? status;
  final bool archived;
  final String writeState;
  final String? writeStateReason;

  const CodexSession({
    required this.threadId,
    required this.title,
    this.preview,
    this.cwdDisplay,
    this.createdAt,
    required this.updatedAt,
    this.status,
    required this.archived,
    required this.writeState,
    this.writeStateReason,
  });

  factory CodexSession.fromJson(Map<String, dynamic> json) => CodexSession(
    threadId: json['thread_id'] as String,
    title: json['title'] as String,
    preview: json['preview'] as String?,
    cwdDisplay: json['cwd_display'] as String?,
    createdAt: json['created_at'] as String?,
    updatedAt: json['updated_at'] as String,
    status: json['status'] as String?,
    archived: json['archived'] as bool,
    writeState: json['write_state'] as String,
    writeStateReason: json['write_state_reason'] as String?,
  );

  Map<String, dynamic> toJson() => {
    'thread_id': threadId,
    'title': title,
    'preview': preview,
    'cwd_display': cwdDisplay,
    'created_at': createdAt,
    'updated_at': updatedAt,
    'status': status,
    'archived': archived,
    'write_state': writeState,
    'write_state_reason': writeStateReason,
  };
}

class SessionSync {
  final String syncId;
  final String status;
  final String deviceId;
  final int? snapshotVersion;
  final List<CodexSession> items;
  final String? nextCursor;
  final ErrorSummary? error;

  const SessionSync({
    required this.syncId,
    required this.status,
    required this.deviceId,
    this.snapshotVersion,
    required this.items,
    this.nextCursor,
    this.error,
  });

  factory SessionSync.fromJson(Map<String, dynamic> json) => SessionSync(
    syncId: json['sync_id'] as String,
    status: json['status'] as String,
    deviceId: json['device_id'] as String,
    snapshotVersion: json['snapshot_version'] as int?,
    items: (json['items'] as List<dynamic>)
        .map((item) => CodexSession.fromJson(item as Map<String, dynamic>))
        .toList(),
    nextCursor: json['next_cursor'] as String?,
    error: json['error'] == null
        ? null
        : ErrorSummary.fromJson(json['error'] as Map<String, dynamic>),
  );

  Map<String, dynamic> toJson() => {
    'sync_id': syncId,
    'status': status,
    'device_id': deviceId,
    'snapshot_version': snapshotVersion,
    'items': items.map((item) => item.toJson()).toList(),
    'next_cursor': nextCursor,
    'error': error?.toJson(),
  };
}

class ThreadSummary {
  final String threadId;
  final String title;
  final String status;
  final String writeState;

  const ThreadSummary({
    required this.threadId,
    required this.title,
    required this.status,
    required this.writeState,
  });

  factory ThreadSummary.fromJson(Map<String, dynamic> json) => ThreadSummary(
    threadId: json['thread_id'] as String,
    title: json['title'] as String,
    status: json['status'] as String,
    writeState: json['write_state'] as String,
  );

  Map<String, dynamic> toJson() => {
    'thread_id': threadId,
    'title': title,
    'status': status,
    'write_state': writeState,
  };
}

class ContentPart {
  final String type;
  final String text;

  const ContentPart({required this.type, required this.text});

  factory ContentPart.fromJson(Map<String, dynamic> json) =>
      ContentPart(type: json['type'] as String, text: json['text'] as String);

  Map<String, dynamic> toJson() => {'type': type, 'text': text};
}

class ThreadItem {
  final String itemId;
  final String? turnId;
  final int sequence;
  final String type;
  final String? role;
  final String? title;
  final String? summary;
  final List<ContentPart>? content;
  final String status;
  final String createdAt;

  const ThreadItem({
    required this.itemId,
    this.turnId,
    required this.sequence,
    required this.type,
    this.role,
    this.title,
    this.summary,
    this.content,
    required this.status,
    required this.createdAt,
  });

  factory ThreadItem.fromJson(Map<String, dynamic> json) => ThreadItem(
    itemId: json['item_id'] as String,
    turnId: json['turn_id'] as String?,
    sequence: json['sequence'] as int,
    type: json['type'] as String,
    role: json['role'] as String?,
    title: json['title'] as String?,
    summary: json['summary'] as String?,
    content: json['content'] == null
        ? null
        : (json['content'] as List<dynamic>)
              .map((item) => ContentPart.fromJson(item as Map<String, dynamic>))
              .toList(),
    status: json['status'] as String,
    createdAt: json['created_at'] as String,
  );

  Map<String, dynamic> toJson() => {
    'item_id': itemId,
    'turn_id': turnId,
    'sequence': sequence,
    'type': type,
    'role': role,
    'title': title,
    'summary': summary,
    'content': content?.map((item) => item.toJson()).toList(),
    'status': status,
    'created_at': createdAt,
  };
}

class ThreadSync {
  final String syncId;
  final String status;
  final ThreadSummary? thread;
  final List<ThreadItem> items;
  final String? nextCursor;
  final ErrorSummary? error;

  const ThreadSync({
    required this.syncId,
    required this.status,
    this.thread,
    required this.items,
    this.nextCursor,
    this.error,
  });

  factory ThreadSync.fromJson(Map<String, dynamic> json) => ThreadSync(
    syncId: json['sync_id'] as String,
    status: json['status'] as String,
    thread: json['thread'] == null
        ? null
        : ThreadSummary.fromJson(json['thread'] as Map<String, dynamic>),
    items: (json['items'] as List<dynamic>)
        .map((item) => ThreadItem.fromJson(item as Map<String, dynamic>))
        .toList(),
    nextCursor: json['next_cursor'] as String?,
    error: json['error'] == null
        ? null
        : ErrorSummary.fromJson(json['error'] as Map<String, dynamic>),
  );

  Map<String, dynamic> toJson() => {
    'sync_id': syncId,
    'status': status,
    'thread': thread?.toJson(),
    'items': items.map((item) => item.toJson()).toList(),
    'next_cursor': nextCursor,
    'error': error?.toJson(),
  };
}

class CreateCommandRequest {
  final String deviceId;
  final String threadId;
  final List<CommandInput> input;
  final ClientContext? clientContext;

  const CreateCommandRequest({
    required this.deviceId,
    required this.threadId,
    required this.input,
    this.clientContext,
  });

  factory CreateCommandRequest.fromJson(Map<String, dynamic> json) =>
      CreateCommandRequest(
        deviceId: json['device_id'] as String,
        threadId: json['thread_id'] as String,
        input: (json['input'] as List<dynamic>)
            .map((item) => CommandInput.fromJson(item as Map<String, dynamic>))
            .toList(),
        clientContext: json['client_context'] == null
            ? null
            : ClientContext.fromJson(
                json['client_context'] as Map<String, dynamic>,
              ),
      );

  Map<String, dynamic> toJson() => {
    'device_id': deviceId,
    'thread_id': threadId,
    'input': input.map((item) => item.toJson()).toList(),
    'client_context': clientContext?.toJson(),
  };
}

class CommandInput {
  final String type;
  final String text;

  const CommandInput({required this.type, required this.text});

  factory CommandInput.fromJson(Map<String, dynamic> json) =>
      CommandInput(type: json['type'] as String, text: json['text'] as String);

  Map<String, dynamic> toJson() => {'type': type, 'text': text};
}

class ClientContext {
  final String? locale;
  final String? source;

  const ClientContext({this.locale, this.source});

  factory ClientContext.fromJson(Map<String, dynamic> json) => ClientContext(
    locale: json['locale'] as String?,
    source: json['source'] as String?,
  );

  Map<String, dynamic> toJson() => {'locale': locale, 'source': source};
}

class Command {
  final String commandId;
  final String status;
  final String? turnId;
  final ErrorSummary? error;
  final String createdAt;
  final String? updatedAt;

  const Command({
    required this.commandId,
    required this.status,
    this.turnId,
    this.error,
    required this.createdAt,
    this.updatedAt,
  });

  factory Command.fromJson(Map<String, dynamic> json) => Command(
    commandId: json['command_id'] as String,
    status: json['status'] as String,
    turnId: json['turn_id'] as String?,
    error: json['error'] == null
        ? null
        : ErrorSummary.fromJson(json['error'] as Map<String, dynamic>),
    createdAt: json['created_at'] as String,
    updatedAt: json['updated_at'] as String?,
  );

  Map<String, dynamic> toJson() => {
    'command_id': commandId,
    'status': status,
    'turn_id': turnId,
    'error': error?.toJson(),
    'created_at': createdAt,
    'updated_at': updatedAt,
  };
}

class ErrorSummary {
  final String reason;
  final String message;

  const ErrorSummary({required this.reason, required this.message});

  factory ErrorSummary.fromJson(Map<String, dynamic> json) => ErrorSummary(
    reason: json['reason'] as String,
    message: json['message'] as String,
  );

  Map<String, dynamic> toJson() => {'reason': reason, 'message': message};
}

class EventEnvelope {
  final int v;
  final String type;
  final String eventId;
  final String? requestId;
  final int sequence;
  final String occurredAt;
  final Map<String, dynamic> payload;

  const EventEnvelope({
    required this.v,
    required this.type,
    required this.eventId,
    this.requestId,
    required this.sequence,
    required this.occurredAt,
    required this.payload,
  });

  factory EventEnvelope.fromJson(Map<String, dynamic> json) => EventEnvelope(
    v: json['v'] as int,
    type: json['type'] as String,
    eventId: json['event_id'] as String,
    requestId: json['request_id'] as String?,
    sequence: json['sequence'] as int,
    occurredAt: json['occurred_at'] as String,
    payload: json['payload'] as Map<String, dynamic>,
  );

  Map<String, dynamic> toJson() => {
    'v': v,
    'type': type,
    'event_id': eventId,
    'request_id': requestId,
    'sequence': sequence,
    'occurred_at': occurredAt,
    'payload': payload,
  };
}
