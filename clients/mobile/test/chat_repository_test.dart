import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/features/chat/data/chat_repository.dart';

void main() {
  test('decodes split OpenAI chat completion SSE fragments', () async {
    const payload =
        'data: {"choices":[{"delta":{"content":"你"}}]}\n\n'
        'event: message\n'
        'data: {"choices":[{"delta":{"content":"好"}}]}\n\n'
        'data: [DONE]\n\n';
    final encoded = utf8.encode(payload);
    final chunks = Stream<List<int>>.fromIterable([
      encoded.sublist(0, 17),
      encoded.sublist(17, 61),
      encoded.sublist(61),
    ]);

    expect(await decodeChatCompletionSse(chunks).toList(), ['你', '好']);
  });

  test('rejects malformed OpenAI SSE data', () async {
    final chunks = Stream<List<int>>.value(utf8.encode('data: not-json\n\n'));

    await expectLater(
      decodeChatCompletionSse(chunks),
      emitsError(isA<ChatRepositoryException>()),
    );
  });
}
