import 'package:flutter/material.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';

class ChatPage extends StatelessWidget {
  const ChatPage({super.key});

  static const _conversations = [
    ('修复支付回调异常', '排查通知失败的问题并修复。', '10:15'),
    ('优化登录流程', '简化登录步骤，提升体验。', '昨天'),
    ('生成报表接口', '新增导出 Excel 的接口。', '昨天'),
  ];

  @override
  Widget build(BuildContext context) {
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
                  const Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text('当前秘钥', style: TextStyle(color: AppColors.muted, fontSize: 12)),
                        SizedBox(height: 4),
                        Text('Mobile Chat', style: TextStyle(fontWeight: FontWeight.w700)),
                        Text('OpenAI · 默认分组', style: TextStyle(color: AppColors.muted)),
                      ],
                    ),
                  ),
                  TextButton(onPressed: () {}, child: const Text('切换')),
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
                  title: Text(item.$1, style: const TextStyle(fontWeight: FontWeight.w700)),
                  subtitle: Text(item.$2, maxLines: 1, overflow: TextOverflow.ellipsis),
                  trailing: Text(item.$3, style: const TextStyle(color: AppColors.muted, fontSize: 12)),
                ),
              ),
            ),
          ),
          const SizedBox(height: 8),
          Align(
            alignment: Alignment.centerRight,
            child: FloatingActionButton(
              elevation: 0,
              onPressed: () {},
              child: const Icon(Icons.add_comment_rounded),
            ),
          ),
        ],
      ),
    );
  }
}
