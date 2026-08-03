import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../app/theme.dart';
import '../../../core/auth/user_role.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';

const rechargeUri = 'https://pay.ldxp.cn/shop/codecodeai';

class ProfilePage extends ConsumerWidget {
  const ProfilePage({super.key});

  Future<void> _openRecharge(BuildContext context) async {
    final opened = await launchUrl(Uri.parse(rechargeUri), mode: LaunchMode.externalApplication);
    if (!opened && context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('无法打开充值页面')));
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isAdmin = ref.watch(currentUserRoleProvider) == UserRole.admin;
    return PageFrame(
      title: '我的',
      child: Column(
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 30),
              child: Center(
                child: Column(
                  children: [
                    const CircleAvatar(radius: 38, backgroundColor: AppColors.iconTile, child: Text('A', style: TextStyle(fontSize: 30, color: AppColors.primary))),
                    const SizedBox(height: 16),
                    Text(isAdmin ? 'admin@••••.com' : 'user@••••.com', style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 18)),
                    const SizedBox(height: 8),
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
                      decoration: BoxDecoration(color: AppColors.iconTile, borderRadius: BorderRadius.circular(8)),
                      child: Text(isAdmin ? '管理员' : '用户', style: const TextStyle(color: AppColors.primary)),
                    ),
                  ],
                ),
              ),
            ),
          ),
          const SizedBox(height: 22),
          _ProfileRow(icon: Icons.redeem_outlined, label: '兑换', onTap: () => _showRedeemDialog(context)),
          _ProfileRow(icon: Icons.account_balance_wallet_outlined, label: '充值', onTap: () => _openRecharge(context)),
          _ProfileRow(icon: Icons.notifications_none_rounded, label: '公告', badge: '3', onTap: () => _showAnnouncements(context)),
          if (isAdmin)
            _ProfileRow(icon: Icons.admin_panel_settings_outlined, label: '管理控制台', onTap: () => context.push('/admin/coming-soon')),
          _ProfileRow(icon: Icons.settings_outlined, label: '设置', onTap: () {}),
          _ProfileRow(icon: Icons.info_outline_rounded, label: '关于', onTap: () {}),
          _ProfileRow(icon: Icons.logout_rounded, label: '退出登录', onTap: () {}),
        ],
      ),
    );
  }
}

Future<void> _showRedeemDialog(BuildContext context) {
  return showDialog<void>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('兑换码'),
      content: const TextField(decoration: InputDecoration(hintText: '请输入兑换码')),
      actions: [
        TextButton(onPressed: () => Navigator.pop(context), child: const Text('取消')),
        FilledButton(onPressed: () => Navigator.pop(context), child: const Text('兑换')),
      ],
    ),
  );
}

Future<void> _showAnnouncements(BuildContext context) {
  return showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    builder: (context) => FractionallySizedBox(
      heightFactor: 0.72,
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            const Text('公告', style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800)),
            const SizedBox(height: 16),
            SegmentedButton<String>(
              segments: const [ButtonSegment(value: 'all', label: Text('全部')), ButtonSegment(value: 'unread', label: Text('未读'))],
              selected: const {'all'},
            ),
            const SizedBox(height: 16),
            const ListTile(title: Text('系统维护通知'), subtitle: Text('计划维护窗口与影响范围'), trailing: Icon(Icons.circle, size: 8, color: AppColors.primary)),
            const Divider(),
            const ListTile(title: Text('功能更新：支持文件上传'), subtitle: Text('聊天体验与稳定性更新'), trailing: Icon(Icons.circle, size: 8, color: AppColors.primary)),
          ],
        ),
      ),
    ),
  );
}

class _ProfileRow extends StatelessWidget {
  const _ProfileRow({required this.icon, required this.label, required this.onTap, this.badge});

  final IconData icon;
  final String label;
  final VoidCallback onTap;
  final String? badge;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: ListTile(
        contentPadding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        leading: AppIconTile(icon, color: AppColors.muted),
        title: Text(label, style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600)),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (badge != null)
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                decoration: const BoxDecoration(color: Colors.red, shape: BoxShape.circle),
                child: Text(badge!, style: const TextStyle(color: Colors.white, fontSize: 11)),
              ),
            const Icon(Icons.chevron_right_rounded),
          ],
        ),
        onTap: onTap,
      ),
    );
  }
}
