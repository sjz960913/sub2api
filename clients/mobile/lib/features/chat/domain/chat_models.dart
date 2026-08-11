class ChatModelCatalog {
  const ChatModelCatalog({required this.chatModels, required this.imageModels});

  final List<String> chatModels;
  final List<String> imageModels;
}

class ChatMessage {
  const ChatMessage.text({required this.fromUser, required this.text})
    : imageBase64 = null,
      imageUrl = null;

  const ChatMessage.image({this.imageBase64, this.imageUrl})
    : fromUser = false,
      text = '';

  final bool fromUser;
  final String text;
  final String? imageBase64;
  final String? imageUrl;

  bool get hasImage => imageBase64 != null || imageUrl != null;
}

class GeneratedImage {
  const GeneratedImage({this.base64, this.url});

  final String? base64;
  final String? url;
}

class ChatConversation {
  const ChatConversation({
    required this.id,
    required this.title,
    required this.createdAt,
    required this.updatedAt,
    required this.messages,
    this.model,
  });

  final String id;
  final String title;
  final String? model;
  final DateTime createdAt;
  final DateTime updatedAt;
  final List<ChatMessage> messages;
}

class ChatConversationSummary {
  const ChatConversationSummary({
    required this.id,
    required this.title,
    required this.preview,
    required this.updatedAt,
    required this.messageCount,
  });

  final String id;
  final String title;
  final String preview;
  final DateTime updatedAt;
  final int messageCount;
}
