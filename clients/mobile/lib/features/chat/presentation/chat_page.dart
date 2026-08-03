import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';
import '../../api_keys/application/api_key_catalog.dart';

class ChatPage extends ConsumerWidget {
  const ChatPage({super.key});

  static const _conversations = [
    ('payment-callback', '修复支付回调异常', '排查通知失败的问题并修复。', '10:15'),
    ('login-flow', '优化登录流程', '简化登录步骤，提升体验。', '昨天'),
    ('report-api', '生成报表接口', '新增导出 Excel 的接口。', '昨天'),
  ];

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
          Text('最近对话', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 12),
          ..._conversations.map(
            (item) => Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: Card(
                child: ListTile(
                  contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
                  leading: const AppIconTile(Icons.chat_bubble_outline_rounded),
                  title: Text(item.$2, style: const TextStyle(fontWeight: FontWeight.w700)),
                  subtitle: Text(item.$3, maxLines: 1, overflow: TextOverflow.ellipsis),
                  trailing: Text(
                    item.$4,
                    style: const TextStyle(color: AppColors.muted, fontSize: 12),
                  ),
                  onTap: () => context.push('/app/chat/${item.$1}'),
                ),
              ),
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
