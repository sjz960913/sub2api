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
    final device = state.selectedDevice;
    final deviceError = _collaborationErrorMessage(state.errorCode);
    final filteredSessions = state.sessions
        .where(
          (session) => '${session.title}${session.preview}'
              .toLowerCase()
              .contains(query),
        )
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
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          state.isLoadingDevices
                              ? '正在查询电脑…'
                              : device?.name ??
                                    (deviceError?.title ?? '没有已登录的电脑'),
                          style: const TextStyle(
                            fontWeight: FontWeight.w700,
                            fontSize: 17,
                          ),
                        ),
                        const SizedBox(height: 5),
                        Row(
                          children: [
                            Icon(
                              Icons.circle,
                              color: device?.isOnline == true
                                  ? AppColors.success
                                  : AppColors.muted,
                              size: 10,
                            ),
                            const SizedBox(width: 6),
                            Text(
                              device == null
                                  ? deviceError?.description ??
                                        '请先在电脑端登录同一账号'
                                  : '${device.platform} · ${device.isOnline ? '在线' : '离线'}',
                              style: const TextStyle(color: AppColors.muted),
                            ),
                          ],
                        ),
                      ],
                    ),
                  ),
                  TextButton(
                    onPressed: state.devices.isEmpty
                        ? state.isLoadingDevices
                              ? null
                              : () => ref
                                    .read(
                                      collaborationOverviewProvider.notifier,
                                    )
                                    .loadDevices()
                        : () async {
                            final selected = await _showDevice(
                              context,
                              state.devices,
                            );
                            if (selected != null) {
                              ref
                                  .read(collaborationOverviewProvider.notifier)
                                  .selectDevice(selected);
                            }
                          },
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Text(state.devices.isEmpty ? '重试' : '查看设备'),
                        const Icon(Icons.chevron_right_rounded, size: 18),
                      ],
                    ),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 18),
          FilledButton.icon(
            onPressed: device?.isOnline == true && !state.isQuerying
                ? () async {
                    final loaded = await ref
                        .read(collaborationOverviewProvider.notifier)
                        .querySessions();
                    if (context.mounted) {
                      ScaffoldMessenger.of(context).showSnackBar(
                        SnackBar(
                          content: Text(loaded ? '已获取电脑会话' : '查询失败，请稍后重试'),
                        ),
                      );
                    }
                  }
                : null,
            icon: state.isQuerying
                ? const SizedBox.square(
                    dimension: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : const Icon(Icons.sync_rounded),
            label: Text(state.isQuerying ? '正在查询…' : '查询电脑会话'),
          ),
          const SizedBox(height: 14),
          TextField(
            enabled: state.hasQueried,
            onChanged: (value) =>
                setState(() => query = value.trim().toLowerCase()),
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
                child: Text(
                  '没有匹配的会话',
                  style: TextStyle(color: AppColors.muted),
                ),
              ),
            )
          else
            for (var index = 0; index < filteredSessions.length; index++) ...[
              _SessionCard(session: filteredSessions[index]),
              if (index != filteredSessions.length - 1)
                const SizedBox(height: 10),
            ],
        ],
      ),
    );
  }
}

class _CollaborationErrorMessage {
  const _CollaborationErrorMessage(this.title, this.description);

  final String title;
  final String description;
}

_CollaborationErrorMessage? _collaborationErrorMessage(String? code) {
  return switch (code) {
    'COLLABORATION_DISABLED' => const _CollaborationErrorMessage(
      '协同服务尚未启用',
      '请在服务端开启协同功能后重试',
    ),
    'PANEL_NETWORK_ERROR' => const _CollaborationErrorMessage(
      '无法连接协同服务',
      '请检查网络后重试',
    ),
    'PANEL_SESSION_NOT_FOUND' || 'PANEL_UNAUTHORIZED' =>
      const _CollaborationErrorMessage('登录状态已失效', '请重新登录后重试'),
    null => null,
    _ => const _CollaborationErrorMessage('无法获取电脑', '请稍后重试'),
  };
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
            const Icon(
              Icons.desktop_windows_outlined,
              color: AppColors.primary,
              size: 38,
            ),
            const SizedBox(height: 12),
            Text(
              '点击查询获取电脑上的 Codex 会话',
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            const SizedBox(height: 6),
            const Text(
              '电脑工具需登录同一个站点并保持在线',
              style: TextStyle(color: AppColors.muted),
            ),
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
        title: Text(
          session.title,
          style: const TextStyle(fontWeight: FontWeight.w700),
        ),
        subtitle: Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                session.preview,
                maxLines: 1,
                overflow: TextOverflow.ellipsis,
              ),
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

Future<String?> _showDevice(
  BuildContext context,
  List<CollaborationDeviceSummary> devices,
) {
  return showModalBottomSheet<String>(
    context: context,
    showDragHandle: true,
    builder: (context) => Padding(
      padding: const EdgeInsets.fromLTRB(20, 4, 20, 28),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (final device in devices)
            ListTile(
              contentPadding: EdgeInsets.zero,
              leading: const AppIconTile(Icons.desktop_windows_rounded),
              title: Text(
                device.name,
                style: const TextStyle(fontWeight: FontWeight.w700),
              ),
              subtitle: Text(
                '${device.platform} · ${device.isOnline ? '在线' : '离线'}',
              ),
              trailing: Icon(
                Icons.circle,
                color: device.isOnline ? AppColors.success : AppColors.muted,
                size: 12,
              ),
              onTap: () => Navigator.pop(context, device.id),
            ),
        ],
      ),
    ),
  );
}
