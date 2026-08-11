import 'dart:async';
import 'dart:convert';
import 'dart:math';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../api_keys/application/api_key_catalog.dart';
import '../../api_keys/domain/api_key_summary.dart';
import '../data/chat_history_repository.dart';
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
  late final String conversationId;
  late DateTime conversationCreatedAt;
  List<String> models = const [];
  List<String> imageModels = const [];
  String? selectedModel;
  String? persistedModel;
  String? loadedKeyId;
  String conversationTitle = '新对话';
  bool isLoadingHistory = true;
  bool isLoadingModels = false;
  bool isSending = false;
  String? modelError;
  StreamSubscription<String>? activeCompletion;
  Future<void> pendingHistoryWrite = Future<void>.value();

  @override
  void initState() {
    super.initState();
    conversationId = widget.conversationId == 'new'
        ? _newConversationId()
        : widget.conversationId;
    conversationCreatedAt = DateTime.now().toUtc();
    Future<void>.microtask(_loadLocalConversation);
  }

  @override
  void dispose() {
    unawaited(activeCompletion?.cancel());
    composer.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final currentKey = ref.watch(selectedChatKeyProvider);
    if (currentKey != null && loadedKeyId != currentKey.id) {
      loadedKeyId = currentKey.id;
      Future<void>.microtask(() => _loadModels(currentKey));
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
                      Text(
                        conversationTitle,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      const SizedBox(height: 2),
                      Text(
                        currentKey == null
                            ? '请先选择秘钥'
                            : '${currentKey.name} · ${currentKey.maskedKey}',
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: const TextStyle(
                          color: AppColors.muted,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                ),
                TextButton(
                  onPressed: () => context.go('/app/keys'),
                  child: const Text('切换'),
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 10, 16, 8),
            child: DropdownButtonFormField<String>(
              initialValue: selectedModel,
              key: ValueKey(
                '${loadedKeyId ?? 'none'}:${selectedModel ?? 'none'}',
              ),
              decoration: const InputDecoration(
                labelText: '模型',
                prefixIcon: Icon(Icons.auto_awesome_outlined),
              ),
              hint: Text(isLoadingModels ? '正在读取可用模型…' : '请选择模型'),
              items: models
                  .map(
                    (model) =>
                        DropdownMenuItem(value: model, child: Text(model)),
                  )
                  .toList(growable: false),
              onChanged: isLoadingModels || isSending
                  ? null
                  : (value) {
                      setState(() {
                        selectedModel = value;
                        persistedModel = value;
                      });
                      _queueHistoryWrite();
                    },
            ),
          ),
          if (modelError != null)
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 18),
              child: Row(
                children: [
                  const Expanded(
                    child: Text(
                      '无法读取模型列表',
                      style: TextStyle(color: AppColors.muted),
                    ),
                  ),
                  TextButton(
                    onPressed: currentKey == null
                        ? null
                        : () => _loadModels(currentKey),
                    child: const Text('重试'),
                  ),
                ],
              ),
            ),
          Expanded(
            child: isLoadingHistory
                ? const Center(child: CircularProgressIndicator())
                : messages.isEmpty
                ? const _EmptyConversation()
                : ListView.separated(
                    padding: const EdgeInsets.fromLTRB(20, 20, 20, 20),
                    itemCount: messages.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 18),
                    itemBuilder: (context, index) =>
                        _MessageBubble(message: messages[index]),
                  ),
          ),
          if (isSending)
            const Padding(
              padding: EdgeInsets.only(bottom: 8),
              child: Text(
                '正在生成…',
                style: TextStyle(color: AppColors.muted, fontSize: 12),
              ),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 14),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                IconButton.filledTonal(
                  onPressed:
                      currentKey != null &&
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
                if (isSending)
                  IconButton.filled(
                    onPressed: _stopCompletion,
                    tooltip: '停止生成',
                    icon: const Icon(Icons.stop_rounded),
                  )
                else
                  IconButton.filled(
                    onPressed:
                        currentKey != null &&
                            selectedModel != null &&
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

  Future<void> _loadLocalConversation() async {
    if (widget.conversationId == 'new') {
      if (mounted) {
        setState(() => isLoadingHistory = false);
      }
      return;
    }
    final scope = ref.read(chatHistoryScopeProvider);
    if (scope == null) {
      if (mounted) {
        setState(() => isLoadingHistory = false);
      }
      return;
    }
    try {
      final conversation = await ref
          .read(chatHistoryRepositoryProvider)
          .get(scope, conversationId);
      if (!mounted) {
        return;
      }
      if (conversation == null) {
        setState(() => isLoadingHistory = false);
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('本地历史不存在或已被删除')));
        return;
      }
      setState(() {
        conversationTitle = conversation.title;
        conversationCreatedAt = conversation.createdAt;
        persistedModel = conversation.model;
        if (conversation.model != null && models.contains(conversation.model)) {
          selectedModel = conversation.model;
        }
        messages
          ..clear()
          ..addAll(conversation.messages);
        isLoadingHistory = false;
      });
    } catch (_) {
      if (mounted) {
        setState(() => isLoadingHistory = false);
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('无法读取本地历史')));
      }
    }
  }

  Future<void> _loadModels(ApiKeySummary key) async {
    if (!mounted) {
      return;
    }
    setState(() {
      isLoadingModels = true;
      modelError = null;
      models = const [];
      imageModels = const [];
      selectedModel = null;
    });
    try {
      final catalog = await ref
          .read(chatRepositoryProvider)
          .listModels(key.secretKey);
      if (!mounted || loadedKeyId != key.id) {
        return;
      }
      setState(() {
        models = catalog.chatModels;
        imageModels = catalog.imageModels;
        selectedModel =
            persistedModel != null &&
                catalog.chatModels.contains(persistedModel)
            ? persistedModel
            : catalog.chatModels.first;
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

  void _submitDraft(ApiKeySummary key) {
    final text = composer.text.trim();
    final model = selectedModel;
    if (text.isEmpty || model == null) {
      return;
    }
    setState(() {
      if (messages.isEmpty) {
        conversationTitle = _conversationTitleFrom(text);
      }
      persistedModel = model;
      messages.add(ChatMessage.text(fromUser: true, text: text));
      messages.add(const ChatMessage.text(fromUser: false, text: ''));
      composer.clear();
      isSending = true;
    });
    _queueHistoryWrite();
    final assistantIndex = messages.length - 1;
    late StreamSubscription<String> subscription;
    subscription = ref
        .read(chatRepositoryProvider)
        .completeStream(
          apiKey: key.secretKey,
          model: model,
          messages: List.unmodifiable(messages.take(assistantIndex)),
        )
        .listen(
          (fragment) {
            if (!mounted || !identical(activeCompletion, subscription)) {
              return;
            }
            setState(() {
              final previous = messages[assistantIndex];
              messages[assistantIndex] = ChatMessage.text(
                fromUser: false,
                text: '${previous.text}$fragment',
              );
            });
          },
          onError: (Object _) =>
              _finishCompletion(subscription, assistantIndex, failed: true),
          onDone: () => _finishCompletion(subscription, assistantIndex),
          cancelOnError: true,
        );
    activeCompletion = subscription;
  }

  Future<void> _stopCompletion() async {
    final subscription = activeCompletion;
    if (subscription == null) {
      return;
    }
    activeCompletion = null;
    await subscription.cancel();
    if (!mounted) {
      return;
    }
    setState(() {
      isSending = false;
      if (messages.isNotEmpty &&
          !messages.last.fromUser &&
          messages.last.text.isEmpty) {
        messages.removeLast();
      }
    });
    _queueHistoryWrite();
  }

  void _finishCompletion(
    StreamSubscription<String> subscription,
    int assistantIndex, {
    bool failed = false,
  }) {
    if (!mounted || !identical(activeCompletion, subscription)) {
      return;
    }
    activeCompletion = null;
    final empty =
        assistantIndex >= messages.length ||
        messages[assistantIndex].text.isEmpty;
    setState(() {
      isSending = false;
      if (empty && assistantIndex < messages.length) {
        messages.removeAt(assistantIndex);
      }
    });
    _queueHistoryWrite();
    if (failed) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('消息发送失败，请稍后重试')));
    }
  }

  Future<void> _generateImage(ApiKeySummary key) async {
    final draft = await _showImageComposer(context);
    if (draft == null || !mounted) {
      return;
    }
    final imageModel = imageModels.isNotEmpty
        ? imageModels.first
        : 'gpt-image-1';
    setState(() {
      if (messages.isEmpty) {
        conversationTitle = _conversationTitleFrom(draft.prompt);
      }
      persistedModel = imageModel;
      messages.add(ChatMessage.text(fromUser: true, text: draft.prompt));
      isSending = true;
    });
    _queueHistoryWrite();
    try {
      final image = await ref
          .read(chatRepositoryProvider)
          .generateImage(
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
        _queueHistoryWrite();
      }
    } on ChatRepositoryException {
      if (mounted) {
        ScaffoldMessenger.of(
          context,
        ).showSnackBar(const SnackBar(content: Text('图片生成失败，请稍后重试')));
      }
    } finally {
      if (mounted) {
        setState(() => isSending = false);
      }
    }
  }

  void _queueHistoryWrite() {
    final scope = ref.read(chatHistoryScopeProvider);
    final model = persistedModel ?? selectedModel;
    final snapshot = List<ChatMessage>.unmodifiable(messages);
    if (scope == null ||
        snapshot.every(
          (message) => !message.hasImage && message.text.isEmpty,
        )) {
      return;
    }
    final conversation = ChatConversation(
      id: conversationId,
      title: conversationTitle,
      model: model,
      createdAt: conversationCreatedAt,
      updatedAt: DateTime.now().toUtc(),
      messages: snapshot,
    );
    final repository = ref.read(chatHistoryRepositoryProvider);
    pendingHistoryWrite = pendingHistoryWrite
        .then((_) => repository.save(scope, conversation))
        .then((_) {
          if (mounted) {
            ref.invalidate(chatHistoryListProvider);
          }
        })
        .catchError((Object _) {
          if (mounted) {
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(content: Text('聊天已保留在当前页面，但本地历史保存失败')),
            );
          }
        });
  }
}

String _newConversationId() {
  final timestamp = DateTime.now().microsecondsSinceEpoch.toRadixString(36);
  final random = Random.secure().nextInt(0x7fffffff).toRadixString(36);
  return '$timestamp-$random';
}

String _conversationTitleFrom(String text) {
  final compact = text.replaceAll(RegExp(r'\s+'), ' ').trim();
  if (compact.length <= 24) {
    return compact;
  }
  return '${compact.substring(0, 23)}…';
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
      alignment: message.fromUser
          ? Alignment.centerRight
          : Alignment.centerLeft,
      child: ConstrainedBox(
        constraints: BoxConstraints(
          maxWidth: MediaQuery.sizeOf(context).width * 0.78,
        ),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: message.fromUser ? AppColors.primary : AppColors.surface,
            border: message.fromUser
                ? null
                : Border.all(color: AppColors.border),
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
                            errorBuilder: (_, _, _) =>
                                const Icon(Icons.broken_image_outlined),
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
          const Text(
            '生成图片',
            style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
          ),
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
              DropdownMenuItem(
                value: '1024x1024',
                child: Text('正方形 · 1024×1024'),
              ),
              DropdownMenuItem(
                value: '1024x1536',
                child: Text('竖版 · 1024×1536'),
              ),
              DropdownMenuItem(
                value: '1536x1024',
                child: Text('横版 · 1536×1024'),
              ),
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
                    _ImageGenerationDraft(
                      prompt: prompt.text.trim(),
                      size: size,
                    ),
                  ),
            icon: const Icon(Icons.auto_awesome_outlined),
            label: const Text('开始生成'),
          ),
        ],
      ),
    );
  }
}
