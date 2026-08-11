import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';
import '../../api_keys/application/api_key_catalog.dart';
import '../data/chat_history_repository.dart';
import '../domain/chat_models.dart';

class ChatPage extends ConsumerWidget {
  const ChatPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final currentKey = ref.watch(selectedChatKeyProvider);
    final history = ref.watch(chatHistoryListProvider);
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
          Row(
            children: [
              Expanded(child: Text('历史记录', style: Theme.of(context).textTheme.titleLarge)),
              TextButton.icon(
                onPressed: currentKey == null ? null : () => context.push('/app/chat/new'),
                icon: const Icon(Icons.add_rounded),
                label: const Text('新对话'),
              ),
            ],
          ),
          const SizedBox(height: 12),
          history.when(
            loading: () => const Padding(
              padding: EdgeInsets.symmetric(vertical: 32),
              child: Center(child: CircularProgressIndicator()),
            ),
            error: (_, _) => Card(
              child: ListTile(
                leading: const AppIconTile(Icons.history_toggle_off_rounded),
                title: const Text('无法读取本地历史'),
                subtitle: const Text('历史仅保存在当前设备，可点击重试'),
                trailing: IconButton(
                  onPressed: () => ref.invalidate(chatHistoryListProvider),
                  icon: const Icon(Icons.refresh_rounded),
                  tooltip: '重试',
                ),
              ),
            ),
            data: (items) => items.isEmpty
                ? _EmptyHistory(currentKeyAvailable: currentKey != null)
                : Column(
                    children: [
                      for (final item in items)
                        Padding(
                          padding: const EdgeInsets.only(bottom: 10),
                          child: _HistoryCard(
                            item: item,
                            onOpen: () => context.push('/app/chat/${item.id}'),
                            onDelete: () => _deleteConversation(context, ref, item),
                          ),
                        ),
                    ],
                  ),
          ),
        ],
      ),
    );
  }
}

class _EmptyHistory extends StatelessWidget {
  const _EmptyHistory({required this.currentKeyAvailable});

  final bool currentKeyAvailable;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
        leading: const AppIconTile(Icons.chat_bubble_outline_rounded),
        title: Text(
          currentKeyAvailable ? '暂无本地历史' : '请先选择秘钥',
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        subtitle: Text(
          currentKeyAvailable ? '新对话会安全地保存在当前设备' : '前往秘钥页设置当前聊天使用的 Key',
        ),
        trailing: const Icon(Icons.chevron_right_rounded),
        onTap: () => context.go(currentKeyAvailable ? '/app/chat/new' : '/app/keys'),
      ),
    );
  }
}

class _HistoryCard extends StatelessWidget {
  const _HistoryCard({required this.item, required this.onOpen, required this.onDelete});

  final ChatConversationSummary item;
  final VoidCallback onOpen;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        contentPadding: const EdgeInsets.fromLTRB(14, 8, 6, 8),
        leading: const AppIconTile(Icons.chat_bubble_outline_rounded),
        title: Text(
          item.title,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        subtitle: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SizedBox(height: 3),
            Text(item.preview, maxLines: 1, overflow: TextOverflow.ellipsis),
            const SizedBox(height: 5),
            Text(
              '${_formatLocalTime(item.updatedAt)} · ${item.messageCount} 条消息',
              style: const TextStyle(color: AppColors.muted, fontSize: 12),
            ),
          ],
        ),
        trailing: PopupMenuButton<String>(
          tooltip: '更多',
          onSelected: (value) {
            if (value == 'delete') {
              onDelete();
            }
          },
          itemBuilder: (_) => const [
            PopupMenuItem(value: 'delete', child: Text('删除本地记录')),
          ],
        ),
        onTap: onOpen,
      ),
    );
  }
}

Future<void> _deleteConversation(
  BuildContext context,
  WidgetRef ref,
  ChatConversationSummary item,
) async {
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('删除本地记录？'),
      content: Text('“${item.title}”仅会从当前设备删除。'),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('取消')),
        FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('删除')),
      ],
    ),
  );
  if (confirmed != true || !context.mounted) {
    return;
  }
  final scope = ref.read(chatHistoryScopeProvider);
  if (scope == null) {
    return;
  }
  try {
    await ref.read(chatHistoryRepositoryProvider).delete(scope, item.id);
    ref.invalidate(chatHistoryListProvider);
  } catch (_) {
    if (context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('删除失败，请稍后重试')),
      );
    }
  }
}

String _formatLocalTime(DateTime value) {
  final local = value.toLocal();
  final now = DateTime.now();
  String twoDigits(int number) => number.toString().padLeft(2, '0');
  if (local.year == now.year && local.month == now.month && local.day == now.day) {
    return '今天 ${twoDigits(local.hour)}:${twoDigits(local.minute)}';
  }
  return '${local.month}月${local.day}日 ${twoDigits(local.hour)}:${twoDigits(local.minute)}';
}
