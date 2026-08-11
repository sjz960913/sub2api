import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as path;
import 'package:sqflite/sqflite.dart';

import '../../auth/application/session_controller.dart';
import '../domain/chat_models.dart';

const _databaseFileName = 'sub2api_chat_history.db';
const _schemaVersion = 1;
const _maxConversationsPerAccount = 100;

final chatHistoryRepositoryProvider = Provider<ChatHistoryRepository>((ref) {
  final repository = SqliteChatHistoryRepository();
  ref.onDispose(() => unawaited(repository.close()));
  return repository;
});

final chatHistoryScopeProvider = Provider<String?>((ref) {
  final session = ref.watch(sessionControllerProvider);
  final siteUrl = session.siteUrl;
  final user = session.user;
  if (!session.isAuthenticated || siteUrl == null || user == null) {
    return null;
  }
  final site = Uri.tryParse(siteUrl);
  if (site == null || !site.hasScheme || site.host.isEmpty) {
    return null;
  }
  final origin =
      '${site.scheme.toLowerCase()}://${site.host.toLowerCase()}'
      '${site.hasPort ? ':${site.port}' : ''}';
  return '$origin|user:${user.id}';
});

final chatHistoryListProvider =
    FutureProvider.autoDispose<List<ChatConversationSummary>>((ref) {
      final scope = ref.watch(chatHistoryScopeProvider);
      if (scope == null) {
        return const <ChatConversationSummary>[];
      }
      return ref.watch(chatHistoryRepositoryProvider).list(scope);
    });

abstract interface class ChatHistoryRepository {
  Future<List<ChatConversationSummary>> list(String scope);

  Future<ChatConversation?> get(String scope, String conversationId);

  Future<void> save(String scope, ChatConversation conversation);

  Future<void> delete(String scope, String conversationId);
}

class SqliteChatHistoryRepository implements ChatHistoryRepository {
  SqliteChatHistoryRepository({DatabaseFactory? factory, String? databasePath})
    : _databaseFactory = factory ?? databaseFactory,
      _databasePath = databasePath;

  final DatabaseFactory _databaseFactory;
  final String? _databasePath;
  Database? _database;
  Future<Database>? _openingDatabase;

  Future<Database> get _db async {
    final existing = _database;
    if (existing != null) {
      return existing;
    }
    final opening = _openingDatabase;
    if (opening != null) {
      return opening;
    }
    final openOperation = _openDatabase();
    _openingDatabase = openOperation;
    try {
      final database = await openOperation;
      _database = database;
      return database;
    } finally {
      _openingDatabase = null;
    }
  }

  Future<Database> _openDatabase() async {
    final resolvedPath =
        _databasePath ?? path.join(await getDatabasesPath(), _databaseFileName);
    return _databaseFactory.openDatabase(
      resolvedPath,
      options: OpenDatabaseOptions(
        version: _schemaVersion,
        onConfigure: (db) => db.execute('PRAGMA foreign_keys = ON'),
        onCreate: (db, _) async {
          await db.execute('''
CREATE TABLE chat_conversations (
  scope TEXT NOT NULL,
  id TEXT NOT NULL,
  title TEXT NOT NULL,
  model TEXT,
  preview TEXT NOT NULL,
  message_count INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  PRIMARY KEY (scope, id)
)
''');
          await db.execute('''
CREATE TABLE chat_messages (
  scope TEXT NOT NULL,
  conversation_id TEXT NOT NULL,
  position INTEGER NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
  text TEXT NOT NULL,
  image_base64 TEXT,
  image_url TEXT,
  PRIMARY KEY (scope, conversation_id, position),
  FOREIGN KEY (scope, conversation_id)
    REFERENCES chat_conversations(scope, id)
    ON DELETE CASCADE
)
''');
          await db.execute(
            'CREATE INDEX chat_conversations_scope_updated_idx '
            'ON chat_conversations(scope, updated_at DESC)',
          );
        },
      ),
    );
  }

  @override
  Future<List<ChatConversationSummary>> list(String scope) async {
    final rows = await (await _db).query(
      'chat_conversations',
      columns: ['id', 'title', 'preview', 'message_count', 'updated_at'],
      where: 'scope = ?',
      whereArgs: [scope],
      orderBy: 'updated_at DESC',
    );
    return rows
        .map(
          (row) => ChatConversationSummary(
            id: row['id']! as String,
            title: row['title']! as String,
            preview: row['preview']! as String,
            messageCount: row['message_count']! as int,
            updatedAt: DateTime.fromMillisecondsSinceEpoch(
              row['updated_at']! as int,
              isUtc: true,
            ),
          ),
        )
        .toList(growable: false);
  }

  @override
  Future<ChatConversation?> get(String scope, String conversationId) async {
    final database = await _db;
    final conversations = await database.query(
      'chat_conversations',
      where: 'scope = ? AND id = ?',
      whereArgs: [scope, conversationId],
      limit: 1,
    );
    if (conversations.isEmpty) {
      return null;
    }
    final row = conversations.single;
    final messageRows = await database.query(
      'chat_messages',
      where: 'scope = ? AND conversation_id = ?',
      whereArgs: [scope, conversationId],
      orderBy: 'position ASC',
    );
    return ChatConversation(
      id: row['id']! as String,
      title: row['title']! as String,
      model: row['model'] as String?,
      createdAt: DateTime.fromMillisecondsSinceEpoch(
        row['created_at']! as int,
        isUtc: true,
      ),
      updatedAt: DateTime.fromMillisecondsSinceEpoch(
        row['updated_at']! as int,
        isUtc: true,
      ),
      messages: messageRows
          .map(
            (message) =>
                message['image_base64'] != null || message['image_url'] != null
                ? ChatMessage.image(
                    imageBase64: message['image_base64'] as String?,
                    imageUrl: message['image_url'] as String?,
                  )
                : ChatMessage.text(
                    fromUser: message['role'] == 'user',
                    text: message['text']! as String,
                  ),
          )
          .toList(growable: false),
    );
  }

  @override
  Future<void> save(String scope, ChatConversation conversation) async {
    final persistedMessages = conversation.messages
        .where((message) => message.hasImage || message.text.trim().isNotEmpty)
        .map(_sanitizeMessage)
        .toList(growable: false);
    if (persistedMessages.isEmpty) {
      return;
    }
    final previewMessage = persistedMessages.last;
    final preview = previewMessage.hasImage
        ? '[图片]'
        : _truncate(
            previewMessage.text.replaceAll(RegExp(r'\s+'), ' ').trim(),
            80,
          );
    final database = await _db;
    await database.transaction((transaction) async {
      await transaction.insert('chat_conversations', {
        'scope': scope,
        'id': conversation.id,
        'title': _truncate(_redactSecrets(conversation.title.trim()), 80),
        'model': conversation.model == null
            ? null
            : _redactSecrets(conversation.model!),
        'preview': preview,
        'message_count': persistedMessages.length,
        'created_at': conversation.createdAt.millisecondsSinceEpoch,
        'updated_at': conversation.updatedAt.millisecondsSinceEpoch,
      }, conflictAlgorithm: ConflictAlgorithm.replace);
      await transaction.delete(
        'chat_messages',
        where: 'scope = ? AND conversation_id = ?',
        whereArgs: [scope, conversation.id],
      );
      for (var index = 0; index < persistedMessages.length; index += 1) {
        final message = persistedMessages[index];
        await transaction.insert('chat_messages', {
          'scope': scope,
          'conversation_id': conversation.id,
          'position': index,
          'role': message.fromUser ? 'user' : 'assistant',
          'text': message.text,
          'image_base64': message.imageBase64,
          'image_url': message.imageUrl,
        });
      }
      await transaction.rawDelete(
        '''
DELETE FROM chat_conversations
WHERE scope = ? AND id NOT IN (
  SELECT id FROM chat_conversations
  WHERE scope = ?
  ORDER BY updated_at DESC
  LIMIT ?
)
''',
        [scope, scope, _maxConversationsPerAccount],
      );
    });
  }

  @override
  Future<void> delete(String scope, String conversationId) async {
    await (await _db).delete(
      'chat_conversations',
      where: 'scope = ? AND id = ?',
      whereArgs: [scope, conversationId],
    );
  }

  Future<void> close() async {
    final database = _database;
    _database = null;
    await database?.close();
  }
}

ChatMessage _sanitizeMessage(ChatMessage message) {
  if (!message.hasImage) {
    return ChatMessage.text(
      fromUser: message.fromUser,
      text: _redactSecrets(message.text),
    );
  }
  return ChatMessage.image(
    imageBase64: message.imageBase64,
    imageUrl: _sanitizeImageUrl(message.imageUrl),
  );
}

String? _sanitizeImageUrl(String? value) {
  if (value == null) {
    return null;
  }
  final uri = Uri.tryParse(value);
  if (uri == null || !uri.hasScheme) {
    return null;
  }
  return uri.replace(query: null, fragment: null).toString();
}

String _redactSecrets(String value) {
  var redacted = value.replaceAll(RegExp(r'\bsk-[A-Za-z0-9_-]{8,}\b'), '[已脱敏]');
  redacted = redacted.replaceAll(
    RegExp(r'\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b'),
    '[已脱敏]',
  );
  redacted = redacted.replaceAllMapped(
    RegExp(
      r'\b(api[_-]?key|x-api-key|authorization)\b\s*[:=]\s*([^\s,;]+)',
      caseSensitive: false,
    ),
    (match) => '${match.group(1)}=[已脱敏]',
  );
  return redacted;
}

String _truncate(String value, int maxLength) {
  if (value.length <= maxLength) {
    return value;
  }
  return '${value.substring(0, maxLength - 1)}…';
}
