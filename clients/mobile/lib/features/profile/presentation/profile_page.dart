import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';

import '../../../app/theme.dart';
import '../../../core/auth/user_role.dart';
import '../../../core/widgets/app_icon_tile.dart';
import '../../../core/widgets/page_frame.dart';
import '../../auth/application/session_controller.dart';
import '../application/profile_controller.dart';
import '../data/profile_repository.dart';
import '../domain/user_announcement.dart';

const rechargeUri = 'https://pay.ldxp.cn/shop/codecodeai';

class ProfilePage extends ConsumerWidget {
  const ProfilePage({super.key});

  Future<void> _openRecharge(BuildContext context) async {
    final opened = await launchUrl(
      Uri.parse(rechargeUri),
      mode: LaunchMode.externalApplication,
    );
    if (!opened && context.mounted) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('无法打开充值页面')));
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isAdmin = ref.watch(currentUserRoleProvider) == UserRole.admin;
    final session = ref.watch(sessionControllerProvider);
    final profile = ref.watch(profileControllerProvider);
    final user = session.user;
    final displayName = user?.email ?? '未登录';
    final avatarSource = user?.username.isNotEmpty == true
        ? user!.username
        : displayName;
    final avatarText = avatarSource.substring(0, 1).toUpperCase();
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
                    CircleAvatar(
                      radius: 38,
                      backgroundColor: AppColors.iconTile,
                      child: Text(
                        avatarText,
                        style: const TextStyle(
                          fontSize: 30,
                          color: AppColors.primary,
                        ),
                      ),
                    ),
                    const SizedBox(height: 16),
                    Text(
                      displayName,
                      style: const TextStyle(
                        fontWeight: FontWeight.w700,
                        fontSize: 18,
                      ),
                    ),
                    const SizedBox(height: 8),
                    Container(
                      padding: const EdgeInsets.symmetric(
                        horizontal: 12,
                        vertical: 5,
                      ),
                      decoration: BoxDecoration(
                        color: AppColors.iconTile,
                        borderRadius: BorderRadius.circular(8),
                      ),
                      child: Text(
                        isAdmin ? '管理员' : '用户',
                        style: const TextStyle(color: AppColors.primary),
                      ),
                    ),
                  ],
                ),
              ),
            ),
          ),
          const SizedBox(height: 22),
          _ProfileSection(
            children: [
              _ProfileRow(
                icon: Icons.redeem_outlined,
                label: '兑换',
                onTap: () => _showRedeemDialog(context),
              ),
              _ProfileRow(
                icon: Icons.account_balance_wallet_outlined,
                label: '充值',
                onTap: () => _openRecharge(context),
              ),
              _ProfileRow(
                icon: Icons.notifications_none_rounded,
                label: '公告',
                badge: profile.unreadCount > 0
                    ? profile.unreadCount.toString()
                    : null,
                onTap: () => _showAnnouncements(context),
              ),
            ],
          ),
          const SizedBox(height: 14),
          if (isAdmin)
            Padding(
              padding: const EdgeInsets.only(bottom: 14),
              child: _ProfileSection(
                children: [
                  _ProfileRow(
                    icon: Icons.admin_panel_settings_outlined,
                    label: '管理控制台',
                    onTap: () => context.push('/admin/coming-soon'),
                  ),
                ],
              ),
            ),
          _ProfileSection(
            children: [
              _ProfileRow(
                icon: Icons.settings_outlined,
                label: '设置',
                onTap: () => _showSettings(context),
              ),
              _ProfileRow(
                icon: Icons.info_outline_rounded,
                label: '关于',
                onTap: () => showAboutDialog(
                  context: context,
                  applicationName: 'Sub2API',
                  applicationVersion: '0.1.0',
                ),
              ),
              _ProfileRow(
                icon: Icons.logout_rounded,
                label: '退出登录',
                onTap: session.isBusy
                    ? () {}
                    : () => _confirmLogout(context, ref),
              ),
            ],
          ),
        ],
      ),
    );
  }
}

Future<void> _showRedeemDialog(BuildContext context) {
  return showDialog<void>(
    context: context,
    builder: (context) => const _RedeemDialog(),
  );
}

Future<void> _showAnnouncements(BuildContext context) {
  return showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    isScrollControlled: true,
    builder: (context) => const FractionallySizedBox(
      heightFactor: 0.72,
      child: _AnnouncementSheet(),
    ),
  );
}

Future<void> _showSettings(BuildContext context) {
  return showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    builder: (context) => const Padding(
      padding: EdgeInsets.fromLTRB(20, 4, 20, 28),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text(
            '设置',
            style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
          ),
          SizedBox(height: 10),
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: AppIconTile(
              Icons.language_rounded,
              color: AppColors.muted,
            ),
            title: Text('语言'),
            trailing: Text('简体中文'),
          ),
          ListTile(
            contentPadding: EdgeInsets.zero,
            leading: AppIconTile(
              Icons.palette_outlined,
              color: AppColors.muted,
            ),
            title: Text('外观'),
            trailing: Text('跟随系统'),
          ),
        ],
      ),
    ),
  );
}

Future<void> _confirmLogout(BuildContext context, WidgetRef ref) async {
  final confirmed = await showDialog<bool>(
    context: context,
    builder: (context) => AlertDialog(
      title: const Text('退出登录'),
      content: const Text('确认退出当前账号？'),
      actions: [
        TextButton(
          onPressed: () => Navigator.pop(context, false),
          child: const Text('取消'),
        ),
        FilledButton(
          onPressed: () => Navigator.pop(context, true),
          child: const Text('退出'),
        ),
      ],
    ),
  );
  if (confirmed == true && context.mounted) {
    await ref.read(sessionControllerProvider.notifier).logout();
  }
}

class _RedeemDialog extends ConsumerStatefulWidget {
  const _RedeemDialog();

  @override
  ConsumerState<_RedeemDialog> createState() => _RedeemDialogState();
}

class _RedeemDialogState extends ConsumerState<_RedeemDialog> {
  final controller = TextEditingController();
  bool isBusy = false;
  String? error;

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      title: const Text('兑换码'),
      content: TextField(
        controller: controller,
        autofocus: true,
        enabled: !isBusy,
        onChanged: (_) => setState(() {}),
        decoration: InputDecoration(hintText: '请输入兑换码', errorText: error),
      ),
      actions: [
        TextButton(
          onPressed: isBusy ? null : () => Navigator.pop(context),
          child: const Text('取消'),
        ),
        FilledButton(
          onPressed: controller.text.trim().isEmpty || isBusy ? null : _redeem,
          child: isBusy
              ? const SizedBox.square(
                  dimension: 18,
                  child: CircularProgressIndicator(strokeWidth: 2),
                )
              : const Text('兑换'),
        ),
      ],
    );
  }

  Future<void> _redeem() async {
    setState(() {
      isBusy = true;
      error = null;
    });
    try {
      final result = await ref
          .read(profileControllerProvider.notifier)
          .redeem(controller.text);
      if (!mounted) {
        return;
      }
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(result.message)));
      Navigator.pop(context);
    } on ProfileRepositoryException {
      if (mounted) {
        setState(() {
          isBusy = false;
          error = '兑换失败，请检查兑换码后重试';
        });
      }
    }
  }
}

class _AnnouncementSheet extends ConsumerStatefulWidget {
  const _AnnouncementSheet();

  @override
  ConsumerState<_AnnouncementSheet> createState() => _AnnouncementSheetState();
}

class _AnnouncementSheetState extends ConsumerState<_AnnouncementSheet> {
  String filter = 'all';

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(profileControllerProvider);
    final visible = filter == 'unread'
        ? state.announcements
              .where((announcement) => announcement.isUnread)
              .toList()
        : state.announcements;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 4, 20, 24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            '公告',
            style: TextStyle(fontSize: 24, fontWeight: FontWeight.w800),
          ),
          const SizedBox(height: 16),
          SegmentedButton<String>(
            segments: const [
              ButtonSegment(value: 'all', label: Text('全部')),
              ButtonSegment(value: 'unread', label: Text('未读')),
            ],
            selected: {filter},
            onSelectionChanged: (value) => setState(() => filter = value.first),
          ),
          const SizedBox(height: 16),
          Expanded(
            child: state.isLoading && state.announcements.isEmpty
                ? const Center(child: CircularProgressIndicator())
                : state.errorCode != null && state.announcements.isEmpty
                ? Center(
                    child: OutlinedButton.icon(
                      onPressed: () => ref
                          .read(profileControllerProvider.notifier)
                          .loadAnnouncements(force: true),
                      icon: const Icon(Icons.refresh_rounded),
                      label: const Text('重新加载'),
                    ),
                  )
                : visible.isEmpty
                ? const Center(
                    child: Text(
                      '暂无公告',
                      style: TextStyle(color: AppColors.muted),
                    ),
                  )
                : ListView.separated(
                    itemCount: visible.length,
                    separatorBuilder: (_, _) => const SizedBox(height: 8),
                    itemBuilder: (context, index) {
                      final announcement = visible[index];
                      return Card(
                        child: ListTile(
                          title: Text(
                            announcement.title,
                            style: const TextStyle(fontWeight: FontWeight.w700),
                          ),
                          subtitle: Padding(
                            padding: const EdgeInsets.only(top: 5),
                            child: Text(
                              '${announcement.content}\n${_formatAnnouncementTime(announcement)}',
                              maxLines: 3,
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                          isThreeLine: true,
                          trailing: announcement.isUnread
                              ? const Icon(
                                  Icons.circle,
                                  size: 8,
                                  color: AppColors.primary,
                                )
                              : null,
                          onTap: () => _openAnnouncement(announcement),
                        ),
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }

  Future<void> _openAnnouncement(UserAnnouncement announcement) async {
    await ref
        .read(profileControllerProvider.notifier)
        .markRead(announcement.id);
    if (!mounted) {
      return;
    }
    await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(announcement.title),
        content: SingleChildScrollView(
          child: SelectableText(announcement.content),
        ),
        actions: [
          FilledButton(
            onPressed: () => Navigator.pop(context),
            child: const Text('知道了'),
          ),
        ],
      ),
    );
  }
}

String _formatAnnouncementTime(UserAnnouncement announcement) {
  final time = announcement.createdAt;
  if (time == null) {
    return '';
  }
  String twoDigits(int value) => value.toString().padLeft(2, '0');
  return '${time.month} 月 ${time.day} 日 ${twoDigits(time.hour)}:${twoDigits(time.minute)}';
}

class _ProfileSection extends StatelessWidget {
  const _ProfileSection({required this.children});

  final List<_ProfileRow> children;

  @override
  Widget build(BuildContext context) {
    return Card(
      child: Column(
        children: [
          for (var index = 0; index < children.length; index++) ...[
            children[index],
            if (index != children.length - 1)
              const Divider(height: 1, indent: 70, endIndent: 12),
          ],
        ],
      ),
    );
  }
}

class _ProfileRow extends StatelessWidget {
  const _ProfileRow({
    required this.icon,
    required this.label,
    required this.onTap,
    this.badge,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;
  final String? badge;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 5),
      leading: AppIconTile(icon, color: AppColors.muted),
      title: Text(
        label,
        style: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
      ),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (badge != null)
            Container(
              constraints: const BoxConstraints(minWidth: 22, minHeight: 22),
              alignment: Alignment.center,
              decoration: const BoxDecoration(
                color: Colors.red,
                shape: BoxShape.circle,
              ),
              child: Text(
                badge!,
                style: const TextStyle(color: Colors.white, fontSize: 11),
              ),
            ),
          const SizedBox(width: 4),
          const Icon(Icons.chevron_right_rounded),
        ],
      ),
      onTap: onTap,
    );
  }
}
