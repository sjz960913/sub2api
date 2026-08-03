import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../api_keys/application/api_key_catalog.dart';

class ChatThreadPage extends ConsumerStatefulWidget {
  const ChatThreadPage({required this.conversationId, super.key});

  final String conversationId;

  @override
  ConsumerState<ChatThreadPage> createState() => _ChatThreadPageState();
}

class _ChatThreadPageState extends ConsumerState<ChatThreadPage> {
  final composer = TextEditingController();
  final messages = <_PreviewMessage>[
    const _PreviewMessage(
      fromUser: true,
      text: '帮我检查这段响应是否兼容 OpenAI 格式',
    ),
    const _PreviewMessage(
      fromUser: false,
      text: '可以。核心字段应包含 id、object、choices，并确保 usage 的结构正确。',
    ),
  ];

  @override
  void dispose() {
    composer.dispose();
    super.dispose();
  }

  void submitDraft() {
    final text = composer.text.trim();
    if (text.isEmpty) {
      return;
    }
    setState(() {
      messages.add(_PreviewMessage(fromUser: true, text: text));
      composer.clear();
    });
  }

  @override
  Widget build(BuildContext context) {
    final currentKey = ref.watch(selectedChatKeyProvider);
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
                        widget.conversationId == 'new' ? '新对话' : 'API 兼容性检查',
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
          Expanded(
            child: ListView.separated(
              padding: const EdgeInsets.fromLTRB(20, 24, 20, 20),
              itemCount: messages.length,
              separatorBuilder: (_, _) => const SizedBox(height: 18),
              itemBuilder: (context, index) => _MessageBubble(message: messages[index]),
            ),
          ),
          if (currentKey != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
                decoration: BoxDecoration(
                  color: AppColors.surface,
                  border: Border.all(color: AppColors.border),
                  borderRadius: BorderRadius.circular(999),
                ),
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.key_rounded, size: 16, color: AppColors.primary),
                    const SizedBox(width: 6),
                    Text(currentKey.name, style: const TextStyle(fontSize: 12)),
                  ],
                ),
              ),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 14),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                IconButton.filledTonal(
                  onPressed: () => _showImageComposer(context),
                  tooltip: '生成图片',
                  icon: const Icon(Icons.image_outlined),
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: TextField(
                    controller: composer,
                    minLines: 1,
                    maxLines: 5,
                    onChanged: (_) => setState(() {}),
                    decoration: const InputDecoration(hintText: '输入消息…'),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.filled(
                  onPressed: currentKey != null && composer.text.trim().isNotEmpty
                      ? submitDraft
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
}

class _MessageBubble extends StatelessWidget {
  const _MessageBubble({required this.message});

  final _PreviewMessage message;

  @override
  Widget build(BuildContext context) {
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
            child: Text(
              message.text,
              style: TextStyle(color: message.fromUser ? Colors.white : AppColors.ink, height: 1.5),
            ),
          ),
        ),
      ),
    );
  }
}

class _PreviewMessage {
  const _PreviewMessage({required this.fromUser, required this.text});

  final bool fromUser;
  final String text;
}

Future<void> _showImageComposer(BuildContext context) {
  return showModalBottomSheet<void>(
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
  String size = '正方形';

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
              DropdownMenuItem(value: '正方形', child: Text('正方形')),
              DropdownMenuItem(value: '竖版', child: Text('竖版')),
              DropdownMenuItem(value: '横版', child: Text('横版')),
            ],
            onChanged: (value) {
              if (value != null) {
                setState(() => size = value);
              }
            },
          ),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: prompt.text.trim().isEmpty ? null : () => Navigator.pop(context),
            icon: const Icon(Icons.auto_awesome_outlined),
            label: const Text('开始生成'),
          ),
        ],
      ),
    );
  }
}
