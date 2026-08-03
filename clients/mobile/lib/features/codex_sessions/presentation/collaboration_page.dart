import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';
import '../application/collaboration_overview.dart';

class CollaborationPage extends ConsumerStatefulWidget {
  const CollaborationPage({super.key});

  @override
  ConsumerState<CollaborationPage> createState() => _CollaborationPageState();
}

class _CollaborationPageState extends ConsumerState<CollaborationPage> {
  String query = '';

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(collaborationOverviewProvider);
    final filteredSessions = state.sessions
        .where((session) => '${session.title}${session.preview}'.toLowerCase().contains(query))
        .toList();

    return PageFrame(
      title: '协同',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(18),
              child: Row(
                children: [
                  const AppIconTile(Icons.desktop_windows_rounded),
                  const SizedBox(width: 14),
                  const Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          'Workstation',
                          style: TextStyle(fontWeight: FontWeight.w700, fontSize: 17),
                        ),
                        SizedBox(height: 5),
                        Row(
                          children: [
                            Icon(Icons.circle, color: AppColors.success, size: 10),
                            SizedBox(width: 6),
                            Text('Linux · 在线', style: TextStyle(color: AppColors.muted)),
                          ],
                        ),
                      ],
                    ),
                  ),
                  TextButton(
                    onPressed: () => _showDevice(context),
                    child: const Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [Text('查看设备'), Icon(Icons.chevron_right_rounded, size: 18)],
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 18),
          FilledButton.icon(
            onPressed: () {
              ref.read(collaborationOverviewProvider.notifier).revealSessions();
              ScaffoldMessenger.of(context).showSnackBar(
                const SnackBar(content: Text('已获取电脑会话')),
              );
            },
            icon: const Icon(Icons.sync_rounded),
            label: const Text('查询电脑会话'),
          ),
          const SizedBox(height: 14),
          TextField(
            enabled: state.hasQueried,
            onChanged: (value) => setState(() => query = value.trim().toLowerCase()),
            decoration: const InputDecoration(
              prefixIcon: Icon(Icons.search_rounded),
              hintText: '搜索会话',
            ),
          ),
          const SizedBox(height: 26),
          Text('Codex 会话', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 12),
          if (!state.hasQueried)
            const _CollaborationEmptyState()
          else if (filteredSessions.isEmpty)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 28),
              child: Center(
                child: Text('没有匹配的会话', style: TextStyle(color: AppColors.muted)),
              ),
            )
          else
            for (var index = 0; index < filteredSessions.length; index++) ...[
              _SessionCard(session: filteredSessions[index]),
              if (index != filteredSessions.length - 1) const SizedBox(height: 10),
            ],
        ],
      ),
    );
  }
}

class _CollaborationEmptyState extends StatelessWidget {
  const _CollaborationEmptyState();

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 30),
        child: Column(
          children: [
            const Icon(Icons.desktop_windows_outlined, color: AppColors.primary, size: 38),
            const SizedBox(height: 12),
            Text('点击查询获取电脑上的 Codex 会话', style: Theme.of(context).textTheme.bodyLarge),
            const SizedBox(height: 6),
            const Text('电脑工具需登录同一个站点并保持在线', style: TextStyle(color: AppColors.muted)),
          ],
        ),
      ),
    );
  }
}

class _SessionCard extends StatelessWidget {
  const _SessionCard({required this.session});

  final CodexSessionSummary session;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        contentPadding: const EdgeInsets.all(14),
        leading: const AppIconTile(Icons.article_outlined),
        title: Text(session.title, style: const TextStyle(fontWeight: FontWeight.w700)),
        subtitle: Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(session.preview, maxLines: 1, overflow: TextOverflow.ellipsis),
              const SizedBox(height: 6),
              Text(session.updatedAt, style: const TextStyle(fontSize: 12)),
            ],
          ),
        ),
        trailing: const Icon(Icons.chevron_right_rounded),
        onTap: () => context.push('/app/collab/thread/${session.id}'),
      ),
    );
  }
}

Future<void> _showDevice(BuildContext context) {
  return showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    builder: (context) => const Padding(
      padding: EdgeInsets.fromLTRB(20, 4, 20, 28),
      child: ListTile(
        contentPadding: EdgeInsets.zero,
        leading: AppIconTile(Icons.desktop_windows_rounded),
        title: Text('Workstation', style: TextStyle(fontWeight: FontWeight.w700)),
        subtitle: Text('Linux · Codex 已就绪'),
        trailing: Icon(Icons.circle, color: AppColors.success, size: 12),
      ),
    ),
  );
}
