import 'package:flutter/material.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';

class CollaborationPage extends StatelessWidget {
  const CollaborationPage({super.key});

  @override
  Widget build(BuildContext context) {
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
                        Text('Workstation', style: TextStyle(fontWeight: FontWeight.w700, fontSize: 17)),
                        SizedBox(height: 4),
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
                  TextButton(onPressed: () {}, child: const Text('查看设备')),
                ],
              ),
            ),
          ),
          const SizedBox(height: 18),
          FilledButton.icon(
            onPressed: () {},
            icon: const Icon(Icons.sync_rounded),
            label: const Text('同步会话'),
          ),
          const SizedBox(height: 18),
          const TextField(
            decoration: InputDecoration(prefixIcon: Icon(Icons.search_rounded), hintText: '搜索会话'),
          ),
          const SizedBox(height: 26),
          Text('最近 Codex 会话', style: Theme.of(context).textTheme.titleLarge),
          const SizedBox(height: 12),
          const _SessionCard(title: '修复支付回调', preview: '修复支付回调接口在部分订单状态下失败…', time: '今天 09:23'),
          const SizedBox(height: 10),
          const _SessionCard(title: '更新登录流程', preview: '优化登录流程，增加短信验证并处理异常…', time: '昨天 16:48'),
        ],
      ),
    );
  }
}

class _SessionCard extends StatelessWidget {
  const _SessionCard({required this.title, required this.preview, required this.time});

  final String title;
  final String preview;
  final String time;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: ListTile(
        contentPadding: const EdgeInsets.all(14),
        leading: const AppIconTile(Icons.article_outlined),
        title: Text(title, style: const TextStyle(fontWeight: FontWeight.w700)),
        subtitle: Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(preview, maxLines: 1, overflow: TextOverflow.ellipsis),
              const SizedBox(height: 6),
              Text(time, style: const TextStyle(fontSize: 12)),
            ],
          ),
        ),
        trailing: const Icon(Icons.chevron_right_rounded),
      ),
    );
  }
}
