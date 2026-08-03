import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/panel_api_client.dart';
import '../domain/chat_models.dart';

final chatRepositoryProvider = Provider<ChatRepository>(
  (ref) => ChatRepository(ref.watch(panelApiClientProvider)),
);

class ChatRepositoryException implements Exception {
  const ChatRepositoryException(this.publicCode);

  final String publicCode;
}

class ChatRepository {
  const ChatRepository(this._client);

  final PanelApiClient _client;

  Future<ChatModelCatalog> listModels(String apiKey) async {
    try {
      final response = _asMap(
        await _client.gatewayRequest('GET', 'v1/models', apiKey: apiKey),
      );
      final rawModels = response['data'];
      if (rawModels is! List) {
        throw const ChatRepositoryException('CHAT_INVALID_RESPONSE');
      }
      final ids = rawModels
          .map((item) => item is Map ? item['id'] : null)
          .whereType<String>()
          .where((id) => id.isNotEmpty && id.length <= 200)
          .toSet()
          .toList(growable: false);
      final imageModels = ids.where((id) => id.startsWith('gpt-image-')).toList(growable: false);
      final chatModels = ids
          .where((id) => !id.startsWith('gpt-image-'))
          .where((id) => !id.contains('embedding'))
          .toList(growable: false);
      if (chatModels.isEmpty) {
        throw const ChatRepositoryException('CHAT_NO_MODELS');
      }
      return ChatModelCatalog(chatModels: chatModels, imageModels: imageModels);
    } on PanelApiException catch (error) {
      throw ChatRepositoryException(error.publicCode);
    }
  }

  Future<String> complete({
    required String apiKey,
    required String model,
    required List<ChatMessage> messages,
  }) async {
    final history = messages
        .where((message) => !message.hasImage && message.text.trim().isNotEmpty)
        .map(
          (message) => {
            'role': message.fromUser ? 'user' : 'assistant',
            'content': message.text,
          },
        )
        .toList(growable: false);
    try {
      final response = _asMap(
        await _client.gatewayRequest(
          'POST',
          'v1/chat/completions',
          apiKey: apiKey,
          data: {'model': model, 'messages': history, 'stream': false},
        ),
      );
      final choices = response['choices'];
      if (choices is! List || choices.isEmpty || choices.first is! Map) {
        throw const ChatRepositoryException('CHAT_INVALID_RESPONSE');
      }
      final message = (choices.first as Map)['message'];
      if (message is! Map) {
        throw const ChatRepositoryException('CHAT_INVALID_RESPONSE');
      }
      final content = _extractText(message['content']);
      if (content == null || content.trim().isEmpty) {
        throw const ChatRepositoryException('CHAT_EMPTY_RESPONSE');
      }
      return content.trim();
    } on PanelApiException catch (error) {
      throw ChatRepositoryException(error.publicCode);
    }
  }

  Future<GeneratedImage> generateImage({
    required String apiKey,
    required String model,
    required String prompt,
    required String size,
  }) async {
    try {
      final response = _asMap(
        await _client.gatewayRequest(
          'POST',
          'v1/images/generations',
          apiKey: apiKey,
          data: {
            'model': model,
            'prompt': prompt,
            'size': size,
            'n': 1,
            'response_format': 'b64_json',
          },
        ),
      );
      final data = response['data'];
      if (data is! List || data.isEmpty || data.first is! Map) {
        throw const ChatRepositoryException('IMAGE_INVALID_RESPONSE');
      }
      final image = data.first as Map;
      final base64 = image['b64_json'];
      final url = image['url'];
      if (base64 is String && base64.isNotEmpty) {
        return GeneratedImage(base64: base64);
      }
      if (url is String && Uri.tryParse(url)?.hasScheme == true) {
        return GeneratedImage(url: url);
      }
      throw const ChatRepositoryException('IMAGE_INVALID_RESPONSE');
    } on PanelApiException catch (error) {
      throw ChatRepositoryException(error.publicCode);
    }
  }

  static String? _extractText(dynamic content) {
    if (content is String) {
      return content;
    }
    if (content is List) {
      final parts = content
          .whereType<Map<dynamic, dynamic>>()
          .map((item) => item['text'])
          .whereType<String>()
          .toList(growable: false);
      return parts.isEmpty ? null : parts.join('\n');
    }
    return null;
  }

  static Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return value.map((key, item) => MapEntry(key.toString(), item));
    }
    throw const ChatRepositoryException('CHAT_INVALID_RESPONSE');
  }
}
