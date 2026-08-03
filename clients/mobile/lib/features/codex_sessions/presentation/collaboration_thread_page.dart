import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import '../../../app/theme.dart';

class CollaborationThreadPage extends StatefulWidget {
  const CollaborationThreadPage({required this.sessionId, super.key});

  final String sessionId;

  @override
  State<CollaborationThreadPage> createState() => _CollaborationThreadPageState();
}

class _CollaborationThreadPageState extends State<CollaborationThreadPage> {
  final composer = TextEditingController();
  final tasks = <String>['请检查支付回调在已取消订单下的处理，并补充测试。'];

  @override
  void dispose() {
    composer.dispose();
    super.dispose();
  }

  void submitTask() {
    final value = composer.text.trim();
    if (value.isEmpty) {
      return;
    }
    setState(() {
      tasks.add(value);
      composer.clear();
    });
  }

  @override
  Widget build(BuildContext context) {
    final title = switch (widget.sessionId) {
      'login-flow' => '更新登录流程',
      'api-docs' => '整理 API 文档',
      _ => '修复支付回调',
    };
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
                      Text(title, style: Theme.of(context).textTheme.titleLarge),
                      const SizedBox(height: 3),
                      const Row(
                        children: [
                          Icon(Icons.circle, size: 9, color: AppColors.success),
                          SizedBox(width: 5),
                          Text('Workstation · 在线', style: TextStyle(color: AppColors.muted)),
                        ],
                      ),
                    ],
                  ),
                ),
                IconButton(
                  onPressed: () {
                    ScaffoldMessenger.of(context).showSnackBar(
                      const SnackBar(content: Text('会话已同步')),
                    );
                  },
                  tooltip: '同步最新消息',
                  icon: const Icon(Icons.sync_rounded),
                ),
              ],
            ),
          ),
          const Divider(height: 1),
          Expanded(
            child: ListView(
              padding: const EdgeInsets.fromLTRB(20, 24, 20, 20),
              children: [
                for (final task in tasks) ...[
                  Align(
                    alignment: Alignment.centerRight,
                    child: Container(
                      constraints: BoxConstraints(
                        maxWidth: MediaQuery.sizeOf(context).width * 0.8,
                      ),
                      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
                      decoration: BoxDecoration(
                        color: AppColors.primary,
                        borderRadius: BorderRadius.circular(18).copyWith(
                          bottomRight: const Radius.circular(5),
                        ),
                      ),
                      child: Text(task, style: const TextStyle(color: Colors.white, height: 1.5)),
                    ),
                  ),
                  const SizedBox(height: 18),
                ],
                Card(
                  child: Padding(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        const Text('Codex', style: TextStyle(fontWeight: FontWeight.w700)),
                        const SizedBox(height: 8),
                        const Text('已定位到回调状态判断，并补充了取消订单场景的测试。', style: TextStyle(height: 1.5)),
                        const SizedBox(height: 12),
                        Container(
                          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                          decoration: BoxDecoration(
                            color: AppColors.background,
                            borderRadius: BorderRadius.circular(10),
                          ),
                          child: const Row(
                            children: [
                              Icon(Icons.check_circle_outline_rounded, size: 18),
                              SizedBox(width: 8),
                              Text('修改 2 个文件 · 已完成'),
                              Spacer(),
                              Icon(Icons.expand_more_rounded),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
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
                    minLines: 1,
                    maxLines: 5,
                    onChanged: (_) => setState(() {}),
                    decoration: const InputDecoration(hintText: '继续发送任务…'),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton.filled(
                  onPressed: composer.text.trim().isEmpty ? null : submitTask,
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
}
