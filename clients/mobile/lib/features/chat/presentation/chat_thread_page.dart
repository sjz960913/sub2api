import 'dart:convert';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../api_keys/application/api_key_catalog.dart';
import '../../api_keys/domain/api_key_summary.dart';
import '../data/chat_repository.dart';
import '../domain/chat_models.dart';

class ChatThreadPage extends ConsumerStatefulWidget {
  const ChatThreadPage({required this.conversationId, super.key});

  final String conversationId;

  @override
  ConsumerState<ChatThreadPage> createState() => _ChatThreadPageState();
}

class _ChatThreadPageState extends ConsumerState<ChatThreadPage> {
  final composer = TextEditingController();
  final messages = <ChatMessage>[];
  List<String> models = const [];
  List<String> imageModels = const [];
  String? selectedModel;
  String? loadedKeyId;
  bool isLoadingModels = false;
  bool isSending = false;
  String? modelError;

  @override
  void dispose() {
    composer.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final currentKey = ref.watch(selectedChatKeyProvider);
    if (currentKey != null && loadedKeyId != currentKey.id) {
      final shouldClear = loadedKeyId != null;
      loadedKeyId = currentKey.id;
      Future<void>.microtask(() => _loadModels(currentKey, clearConversation: shouldClear));
    }
    return SafeArea(
      bottom: false,
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 12, 16, 10),
            child: Row(
              children: [
                IconButton(
                  onPressed: () => context.pop(),
                  icon: const Icon(Icons.arrow_back_rounded),
                ),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text('新对话', style: Theme.of(context).textTheme.titleLarge),
                      const SizedBox(height: 2),
                      Text(
                        currentKey == null
                            ? '请先选择秘钥'
                            : '${currentKey.name} · ${currentKey.maskedKey}',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(color: AppColors.muted, fontSize: 12),
                      ),
                    ],
                  ),
                ),
                TextButton(onPressed: () => context.go('/app/keys'), child: const Text('切换')),
              ],
            ),
          ),
          const Divider(height: 1),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 10, 16, 8),
            child: DropdownButtonFormField<String>(
              initialValue: selectedModel,
              key: ValueKey('${loadedKeyId ?? 'none'}:${selectedModel ?? 'none'}'),
              decoration: const InputDecoration(
                labelText: '模型',
                prefixIcon: Icon(Icons.auto_awesome_outlined),
              ),
              hint: Text(isLoadingModels ? '正在读取可用模型…' : '请选择模型'),
              items: models
                  .map((model) => DropdownMenuItem(value: model, child: Text(model)))
                  .toList(growable: false),
              onChanged: isLoadingModels || isSending
                  ? null
                  : (value) => setState(() => selectedModel = value),
            ),
          ),
          if (modelError != null)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 18),
              child: Row(
                children: [
                  const Expanded(
                    child: Text('无法读取模型列表', style: TextStyle(color: AppColors.muted)),
                  ),
                  TextButton(
                    onPressed: currentKey == null ? null : () => _loadModels(currentKey),
                    child: const Text('重试'),
                  ),
                ],
              ),
            ),
          Expanded(
            child: messages.isEmpty
                ? const _EmptyConversation()
                : ListView.separated(
                    padding: const EdgeInsets.fromLTRB(20, 20, 20, 20),
                    itemCount: messages.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 18),
                    itemBuilder: (context, index) => _MessageBubble(message: messages[index]),
                  ),
          ),
          if (isSending)
            const Padding(
              padding: EdgeInsets.only(bottom: 8),
              child: Text('正在生成…', style: TextStyle(color: AppColors.muted, fontSize: 12)),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 14),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                IconButton.filledTonal(
                  onPressed: currentKey != null &&
                          currentKey.kind == ApiKeyKind.image &&
                          !isSending
                      ? () => _generateImage(currentKey)
                      : null,
                  tooltip: '生成图片',
                  icon: const Icon(Icons.image_outlined),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: TextField(
                    controller: composer,
                    enabled: !isSending,
                    minLines: 1,
                    maxLines: 5,
                    onChanged: (_) => setState(() {}),
                    decoration: const InputDecoration(hintText: '输入消息…'),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.filled(
                  onPressed: currentKey != null &&
                          selectedModel != null &&
                          !isSending &&
                          composer.text.trim().isNotEmpty
                      ? () => _submitDraft(currentKey)
                      : null,
                  tooltip: '发送',
                  icon: const Icon(Icons.arrow_upward_rounded),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _loadModels(ApiKeySummary key, {bool clearConversation = false}) async {
    if (!mounted) {
      return;
    }
    setState(() {
      isLoadingModels = true;
      modelError = null;
      models = const [];
      imageModels = const [];
      selectedModel = null;
      if (clearConversation) {
        messages.clear();
      }
    });
    try {
      final catalog = await ref.read(chatRepositoryProvider).listModels(key.secretKey);
      if (!mounted || loadedKeyId != key.id) {
        return;
      }
      setState(() {
        models = catalog.chatModels;
        imageModels = catalog.imageModels;
        selectedModel = catalog.chatModels.first;
        isLoadingModels = false;
      });
    } on ChatRepositoryException catch (error) {
      if (mounted && loadedKeyId == key.id) {
        setState(() {
          isLoadingModels = false;
          modelError = error.publicCode;
        });
      }
    }
  }

  Future<void> _submitDraft(ApiKeySummary key) async {
    final text = composer.text.trim();
    final model = selectedModel;
    if (text.isEmpty || model == null) {
      return;
    }
    setState(() {
      messages.add(ChatMessage.text(fromUser: true, text: text));
      composer.clear();
      isSending = true;
    });
    try {
      final answer = await ref.read(chatRepositoryProvider).complete(
        apiKey: key.secretKey,
        model: model,
        messages: List.unmodifiable(messages),
      );
      if (mounted) {
        setState(() => messages.add(ChatMessage.text(fromUser: false, text: answer)));
      }
    } on ChatRepositoryException {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('消息发送失败，请稍后重试')),
        );
      }
    } finally {
      if (mounted) {
        setState(() => isSending = false);
      }
    }
  }

  Future<void> _generateImage(ApiKeySummary key) async {
    final draft = await _showImageComposer(context);
    if (draft == null || !mounted) {
      return;
    }
    final imageModel = imageModels.isNotEmpty ? imageModels.first : 'gpt-image-1';
    setState(() {
      messages.add(ChatMessage.text(fromUser: true, text: draft.prompt));
      isSending = true;
    });
    try {
      final image = await ref.read(chatRepositoryProvider).generateImage(
        apiKey: key.secretKey,
        model: imageModel,
        prompt: draft.prompt,
        size: draft.size,
      );
      if (mounted) {
        setState(
          () => messages.add(
            ChatMessage.image(imageBase64: image.base64, imageUrl: image.url),
          ),
        );
      }
    } on ChatRepositoryException {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(content: Text('图片生成失败，请稍后重试')),
        );
      }
    } finally {
      if (mounted) {
        setState(() => isSending = false);
      }
    }
  }
}

class _EmptyConversation extends StatelessWidget {
  const _EmptyConversation();

  @override
  Widget build(BuildContext context) {
    return const Center(
      child: Padding(
        padding: EdgeInsets.all(32),
        child: Text(
          '选择模型并输入消息开始对话',
          textAlign: TextAlign.center,
          style: TextStyle(color: AppColors.muted),
        ),
      ),
    );
  }
}

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({required this.message});

  final ChatMessage message;

  @override
  Widget build(BuildContext context) {
    final imageBytes = _decodeBase64(message.imageBase64);
    return Align(
      alignment: message.fromUser ? Alignment.centerRight : Alignment.centerLeft,
      child: ConstrainedBox(
        constraints: BoxConstraints(maxWidth: MediaQuery.sizeOf(context).width * 0.78),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: message.fromUser ? AppColors.primary : AppColors.surface,
            border: message.fromUser ? null : Border.all(color: AppColors.border),
            borderRadius: BorderRadius.circular(18).copyWith(
              bottomRight: message.fromUser ? const Radius.circular(5) : null,
              bottomLeft: message.fromUser ? null : const Radius.circular(5),
            ),
          ),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
            child: message.hasImage
                ? ClipRRect(
                    borderRadius: BorderRadius.circular(12),
                    child: imageBytes != null
                        ? Image.memory(imageBytes, fit: BoxFit.cover)
                        : message.imageUrl != null
                        ? Image.network(
                            message.imageUrl!,
                            fit: BoxFit.cover,
                            errorBuilder: (_, _, _) => const Icon(Icons.broken_image_outlined),
                          )
                        : const Icon(Icons.broken_image_outlined),
                  )
                : SelectableText(
                    message.text,
                    style: TextStyle(
                      color: message.fromUser ? Colors.white : AppColors.ink,
                      height: 1.5,
                    ),
                  ),
          ),
        ),
      ),
    );
  }
}

Uint8List? _decodeBase64(String? value) {
  if (value == null) {
    return null;
  }
  try {
    return base64Decode(value);
  } on FormatException {
    return null;
  }
}

class _ImageGenerationDraft {
  const _ImageGenerationDraft({required this.prompt, required this.size});

  final String prompt;
  final String size;
}

Future<_ImageGenerationDraft?> _showImageComposer(BuildContext context) {
  return showModalBottomSheet<_ImageGenerationDraft>(
    context: context,
    isScrollControlled: true,
    showDragHandle: true,
    builder: (context) => const _ImageComposerSheet(),
  );
}

class _ImageComposerSheet extends StatefulWidget {
  const _ImageComposerSheet();

  @override
  State<_ImageComposerSheet> createState() => _ImageComposerSheetState();
}

class _ImageComposerSheetState extends State<_ImageComposerSheet> {
  final prompt = TextEditingController();
  String size = '1024x1024';

  @override
  void dispose() {
    prompt.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: EdgeInsets.fromLTRB(
        20,
        4,
        20,
        MediaQuery.viewInsetsOf(context).bottom + 24,
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text('生成图片', style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800)),
          const SizedBox(height: 16),
          TextField(
            controller: prompt,
            minLines: 3,
            maxLines: 5,
            onChanged: (_) => setState(() {}),
            decoration: const InputDecoration(hintText: '描述你想生成的图片'),
          ),
          const SizedBox(height: 12),
          DropdownButtonFormField<String>(
            initialValue: size,
            decoration: const InputDecoration(labelText: '尺寸'),
            items: const [
              DropdownMenuItem(value: '1024x1024', child: Text('正方形 · 1024×1024')),
              DropdownMenuItem(value: '1024x1536', child: Text('竖版 · 1024×1536')),
              DropdownMenuItem(value: '1536x1024', child: Text('横版 · 1536×1024')),
            ],
            onChanged: (value) {
              if (value != null) {
                setState(() => size = value);
              }
            },
          ),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: prompt.text.trim().isEmpty
                ? null
                : () => Navigator.pop(
                    context,
                    _ImageGenerationDraft(prompt: prompt.text.trim(), size: size),
                  ),
            icon: const Icon(Icons.auto_awesome_outlined),
            label: const Text('开始生成'),
          ),
        ],
      ),
    );
  }
}
