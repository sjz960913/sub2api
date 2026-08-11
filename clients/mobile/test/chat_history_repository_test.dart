import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:sqflite_common_ffi/sqflite_ffi.dart';
import 'package:sub2api_mobile/features/chat/data/chat_history_repository.dart';
import 'package:sub2api_mobile/features/chat/domain/chat_models.dart';

void main() {
  sqfliteFfiInit();

  late Directory temporaryDirectory;
  late String databasePath;
  late SqliteChatHistoryRepository repository;

  setUp(() async {
    temporaryDirectory = await Directory.systemTemp.createTemp('sub2api-chat-history-');
    databasePath = '${temporaryDirectory.path}/history.db';
    repository = SqliteChatHistoryRepository(
      factory: databaseFactoryFfi,
      databasePath: databasePath,
    );
  });

  tearDown(() async {
    await repository.close();
    await temporaryDirectory.delete(recursive: true);
  });

  test('persists ordered messages and summaries locally', () async {
    final createdAt = DateTime.utc(2026, 8, 11, 10);
    await repository.save(
      'https://panel.example.com|user:7',
      ChatConversation(
        id: 'conversation-1',
        title: '检查 OpenAI 响应',
        model: 'gpt-5.2',
        createdAt: createdAt,
        updatedAt: createdAt.add(const Duration(minutes: 2)),
        messages: const [
          ChatMessage.text(fromUser: true, text: '检查这个响应'),
          ChatMessage.text(fromUser: false, text: '响应格式兼容。'),
        ],
      ),
    );

    final summaries = await repository.list('https://panel.example.com|user:7');
    expect(summaries, hasLength(1));
    expect(summaries.single.title, '检查 OpenAI 响应');
    expect(summaries.single.preview, '响应格式兼容。');
    expect(summaries.single.messageCount, 2);

    final conversation = await repository.get(
      'https://panel.example.com|user:7',
      'conversation-1',
    );
    expect(conversation?.model, 'gpt-5.2');
    expect(conversation?.messages.map((message) => message.text), [
      '检查这个响应',
      '响应格式兼容。',
    ]);
  });

  test('isolates history by site and account scope', () async {
    final now = DateTime.utc(2026, 8, 11);
    await repository.save(
      'https://panel-a.example.com|user:1',
      ChatConversation(
        id: 'same-id',
        title: '账号 A',
        model: 'gpt-5.2',
        createdAt: now,
        updatedAt: now,
        messages: const [ChatMessage.text(fromUser: true, text: 'A 的内容')],
      ),
    );
    await repository.save(
      'https://panel-a.example.com|user:2',
      ChatConversation(
        id: 'same-id',
        title: '账号 B',
        model: 'gpt-5.2',
        createdAt: now,
        updatedAt: now,
        messages: const [ChatMessage.text(fromUser: true, text: 'B 的内容')],
      ),
    );

    expect(
      (await repository.list('https://panel-a.example.com|user:1')).single.title,
      '账号 A',
    );
    expect(
      (await repository.list('https://panel-a.example.com|user:2')).single.title,
      '账号 B',
    );
    expect(await repository.list('https://panel-b.example.com|user:1'), isEmpty);
  });

  test('database schema has no API key or secret columns', () async {
    final now = DateTime.utc(2026, 8, 11);
    await repository.save(
      'https://panel.example.com|user:7',
      ChatConversation(
        id: 'conversation-1',
        title: '本地记录 sk-title-secret-123456',
        model: 'gpt-5.2',
        createdAt: now,
        updatedAt: now,
        messages: const [
          ChatMessage.text(
            fromUser: true,
            text: '不要保存 sk-super-secret-1234567890 或 api_key=another-secret-value',
          ),
        ],
      ),
    );
    final sanitized = await repository.get(
      'https://panel.example.com|user:7',
      'conversation-1',
    );
    expect(sanitized?.title, '本地记录 [已脱敏]');
    expect(sanitized?.messages.single.text, contains('[已脱敏]'));
    expect(sanitized?.messages.single.text, isNot(contains('super-secret')));
    expect(sanitized?.messages.single.text, isNot(contains('another-secret')));
    await repository.close();

    final database = await databaseFactoryFfi.openDatabase(databasePath);
    final rows = await database.rawQuery(
      "SELECT sql FROM sqlite_master WHERE type = 'table' AND name LIKE 'chat_%'",
    );
    final schema = rows.map((row) => row['sql']).join('\n').toLowerCase();
    expect(schema, isNot(contains('api_key')));
    expect(schema, isNot(contains('secret_key')));
    expect(schema, isNot(contains('key_id')));
    expect(schema, isNot(contains('key_name')));

    await database.close();
    final databaseBytes = await File(databasePath).readAsBytes();
    final rawDatabase = String.fromCharCodes(databaseBytes);
    expect(rawDatabase, isNot(contains('sk-super-secret-1234567890')));
    expect(rawDatabase, isNot(contains('another-secret-value')));
  });

  test('deleting a conversation removes it only from that scope', () async {
    final now = DateTime.utc(2026, 8, 11);
    const scope = 'https://panel.example.com|user:7';
    await repository.save(
      scope,
      ChatConversation(
        id: 'conversation-1',
        title: '待删除',
        model: null,
        createdAt: now,
        updatedAt: now,
        messages: const [ChatMessage.text(fromUser: true, text: '删除我')],
      ),
    );

    await repository.delete(scope, 'conversation-1');

    expect(await repository.get(scope, 'conversation-1'), isNull);
    expect(await repository.list(scope), isEmpty);
  });
}
