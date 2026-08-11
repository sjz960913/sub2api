import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../../core/protocol/collaboration_wire.dart';
import '../application/collaboration_overview.dart';
import '../data/collaboration_repository.dart';

class CollaborationThreadPage extends ConsumerStatefulWidget {
  const CollaborationThreadPage({required this.sessionId, super.key});

  final String sessionId;

  @override
  ConsumerState<CollaborationThreadPage> createState() =>
      _CollaborationThreadPageState();
}

class _CollaborationThreadPageState
    extends ConsumerState<CollaborationThreadPage> {
  final composer = TextEditingController();
  List<ThreadItem> items = const [];
  List<String> pendingTasks = const [];
  ThreadSummary? thread;
  bool isSyncing = false;
  bool isSending = false;
  String? errorCode;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) => _sync());
  }

  @override
  void dispose() {
    composer.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final overview = ref.watch(collaborationOverviewProvider);
    final session = overview.sessions
        .where((item) => item.id == widget.sessionId)
        .firstOrNull;
    final device = overview.selectedDevice;
    final title = thread?.title ?? session?.title ?? 'Codex 会话';
    final writeState = thread?.writeState ?? session?.writeState;
    final writable = writeState == null || writeState.startsWith('writable_');
    return SafeArea(
      bottom: false,
      child: Column(
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(12, 12, 12, 10),
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
                        title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: Theme.of(context).textTheme.titleLarge,
                      ),
                      const SizedBox(height: 3),
                      Row(
                        children: [
                          Icon(
                            Icons.circle,
                            size: 9,
                            color: device?.isOnline == true
                                ? AppColors.success
                                : AppColors.muted,
                          ),
                          const SizedBox(width: 5),
                          Text(
                            '${device?.name ?? '电脑'} · '
                            '${device?.isOnline == true ? '在线' : '离线'}',
                            style: const TextStyle(color: AppColors.muted),
                          ),
                        ],
                      ),
                    ],
                  ),
                ),
                IconButton(
                  onPressed: isSyncing || device?.isOnline != true
                      ? null
                      : _sync,
                  tooltip: '同步最新消息',
                  icon: isSyncing
                      ? const SizedBox.square(
                          dimension: 20,
                          child: CircularProgressIndicator(strokeWidth: 2),
                        )
                      : const Icon(Icons.sync_rounded),
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          if (errorCode != null)
            MaterialBanner(
              content: const Text('同步失败，请确认电脑工具在线后重试'),
              actions: [TextButton(onPressed: _sync, child: const Text('重试'))],
            ),
          Expanded(
            child: items.isEmpty && pendingTasks.isEmpty && isSyncing
                ? const Center(child: CircularProgressIndicator())
                : items.isEmpty && pendingTasks.isEmpty
                ? const Center(
                    child: Text(
                      '暂无可显示消息',
                      style: TextStyle(color: AppColors.muted),
                    ),
                  )
                : ListView(
                    padding: const EdgeInsets.fromLTRB(20, 24, 20, 20),
                    children: [
                      for (final item in items) ...[
                        _ThreadItemCard(item: item),
                        const SizedBox(height: 14),
                      ],
                      for (final task in pendingTasks) ...[
                        _PendingTaskBubble(text: task),
                        const SizedBox(height: 14),
                      ],
                    ],
                  ),
          ),
          if (isSending)
            const Padding(
              padding: EdgeInsets.only(bottom: 8),
              child: Text(
                'Codex 正在执行任务…',
                style: TextStyle(color: AppColors.muted),
              ),
            ),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 14),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.end,
              children: [
                Expanded(
                  child: TextField(
                    controller: composer,
                    enabled: writable && device?.isOnline == true && !isSending,
                    minLines: 1,
                    maxLines: 5,
                    onChanged: (_) => setState(() {}),
                    decoration: InputDecoration(
                      hintText: writable ? '继续发送任务…' : '此会话当前只读',
                    ),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.filled(
                  onPressed:
                      writable &&
                          device?.isOnline == true &&
                          !isSending &&
                          composer.text.trim().isNotEmpty
                      ? _submitTask
                      : null,
                  tooltip: '发送任务',
                  icon: const Icon(Icons.arrow_upward_rounded),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _sync() async {
    final device = ref.read(collaborationOverviewProvider).selectedDevice;
    if (device == null || !device.isOnline || isSyncing) {
      return;
    }
    setState(() {
      isSyncing = true;
      errorCode = null;
    });
    try {
      final result = await ref
          .read(collaborationRepositoryProvider)
          .syncThread(deviceId: device.id, threadId: widget.sessionId);
      if (mounted) {
        setState(() {
          thread = result.thread;
          items = result.items;
          isSyncing = false;
          pendingTasks = const [];
        });
      }
    } on CollaborationRepositoryException catch (error) {
      if (mounted) {
        setState(() {
          isSyncing = false;
          errorCode = error.publicCode;
        });
      }
    }
  }

  Future<void> _submitTask() async {
    final device = ref.read(collaborationOverviewProvider).selectedDevice;
    final text = composer.text.trim();
    if (device == null || !device.isOnline || text.isEmpty) {
      return;
    }
    setState(() {
      pendingTasks = [...pendingTasks, text];
      composer.clear();
      isSending = true;
      errorCode = null;
    });
    try {
      await ref
          .read(collaborationRepositoryProvider)
          .submitCommand(
            deviceId: device.id,
            threadId: widget.sessionId,
            text: text,
          );
      if (mounted) {
        setState(() => isSending = false);
        await _sync();
      }
    } on CollaborationRepositoryException catch (error) {
      if (mounted) {
        setState(() {
          isSending = false;
          errorCode = error.publicCode;
        });
      }
    }
  }
}

class _PendingTaskBubble extends StatelessWidget {
  const _PendingTaskBubble({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: Alignment.centerRight,
      child: Container(
        constraints: BoxConstraints(
          maxWidth: MediaQuery.sizeOf(context).width * 0.8,
        ),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
        decoration: BoxDecoration(
          color: AppColors.primary,
          borderRadius: BorderRadius.circular(
            18,
          ).copyWith(bottomRight: const Radius.circular(5)),
        ),
        child: Text(
          text,
          style: const TextStyle(color: Colors.white, height: 1.5),
        ),
      ),
    );
  }
}

class _ThreadItemCard extends StatelessWidget {
  const _ThreadItemCard({required this.item});

  final ThreadItem item;

  @override
  Widget build(BuildContext context) {
    final text = item.content
        ?.map((part) => part.text)
        .where((part) => part.isNotEmpty)
        .join('\n');
    if (item.role == 'user' && text != null && text.isNotEmpty) {
      return _PendingTaskBubble(text: text);
    }
    return Align(
      alignment: Alignment.centerLeft,
      child: Card(
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                item.role == 'assistant'
                    ? 'Codex'
                    : item.title ?? _itemTypeLabel(item.type),
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              if (text != null && text.isNotEmpty) ...[
                const SizedBox(height: 8),
                SelectableText(text, style: const TextStyle(height: 1.5)),
              ],
              if (item.summary != null && item.summary!.isNotEmpty) ...[
                const SizedBox(height: 10),
                Text(
                  item.summary!,
                  style: const TextStyle(color: AppColors.muted),
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

String _itemTypeLabel(String type) {
  return switch (type) {
    'reasoning_summary' => '思考摘要',
    'command_execution' => '命令执行',
    'file_change' => '文件修改',
    'plan' => '计划',
    'error' => '错误',
    _ => 'Codex 事件',
  };
}

extension<T> on Iterable<T> {
  T? get firstOrNull {
    final iterator = this.iterator;
    return iterator.moveNext() ? iterator.current : null;
  }
}
