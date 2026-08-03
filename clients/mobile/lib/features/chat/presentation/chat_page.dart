import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';
import '../../api_keys/application/api_key_catalog.dart';

class ChatPage extends ConsumerWidget {
  const ChatPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final currentKey = ref.watch(selectedChatKeyProvider);
    return PageFrame(
      title: '聊天',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(18),
              child: Row(
                children: [
                  const AppIconTile(Icons.key_rounded),
                  const SizedBox(width: 14),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text(
                          '当前秘钥',
                          style: TextStyle(color: AppColors.muted, fontSize: 12),
                        ),
                        const SizedBox(height: 4),
                        Text(
                          currentKey?.name ?? '尚未选择',
                          style: const TextStyle(fontWeight: FontWeight.w700),
                        ),
                        if (currentKey != null)
                          Text(
                            '${currentKey.group} · ${currentKey.maskedKey}',
                            style: const TextStyle(color: AppColors.muted),
                          ),
                      ],
                    ),
                  ),
                  TextButton(onPressed: () => context.go('/app/keys'), child: const Text('切换')),
                ],
              ),
            ),
          ),
          const SizedBox(height: 26),
          Text('开始对话', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 12),
          Card(
            child: ListTile(
              contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
              leading: const AppIconTile(Icons.chat_bubble_outline_rounded),
              title: Text(
                currentKey == null ? '请先选择秘钥' : '新对话',
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              subtitle: Text(
                currentKey == null ? '前往秘钥页设置当前聊天使用的 Key' : '选择模型后开始聊天或生成图片',
              ),
              trailing: const Icon(Icons.chevron_right_rounded),
              onTap: currentKey == null
                  ? () => context.go('/app/keys')
                  : () => context.push('/app/chat/new'),
            ),
          ),
          const SizedBox(height: 8),
          Align(
            alignment: Alignment.centerRight,
            child: FloatingActionButton(
              elevation: 0,
              onPressed: currentKey == null ? null : () => context.push('/app/chat/new'),
              child: const Icon(Icons.add_comment_rounded),
            ),
          ),
        ],
      ),
    );
  }
}
