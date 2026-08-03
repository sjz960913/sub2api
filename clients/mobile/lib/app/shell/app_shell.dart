import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/profile/application/profile_controller.dart';
import '../../features/profile/domain/user_announcement.dart';
import '../theme.dart';

class AppShell extends ConsumerStatefulWidget {
  const AppShell({required this.navigationShell, super.key});

  final StatefulNavigationShell navigationShell;

  @override
  ConsumerState<AppShell> createState() => _AppShellState();
}

class _AppShellState extends ConsumerState<AppShell> {
  bool _handledPopupAnnouncement = false;

  @override
  Widget build(BuildContext context) {
    ref.listen(profileControllerProvider, (previous, next) {
      if (_handledPopupAnnouncement) {
        return;
      }
      final popup = _firstUnreadPopup(next.announcements);
      if (popup == null) {
        return;
      }
      _handledPopupAnnouncement = true;
      WidgetsBinding.instance.addPostFrameCallback((_) async {
        if (!mounted) {
          return;
        }
        await showDialog<void>(
          context: context,
          builder: (context) => AlertDialog(
            title: Text(popup.title),
            content: SingleChildScrollView(child: SelectableText(popup.content)),
            actions: [
              FilledButton(onPressed: () => Navigator.pop(context), child: const Text('知道了')),
            ],
          ),
        );
        await ref.read(profileControllerProvider.notifier).markRead(popup.id);
      });
    }, fireImmediately: true);
    return Scaffold(
      body: widget.navigationShell,
      bottomNavigationBar: SafeArea(
        minimum: const EdgeInsets.fromLTRB(16, 0, 16, 12),
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: AppColors.surface,
            border: Border.all(color: AppColors.border),
            borderRadius: BorderRadius.circular(26),
            boxShadow: const [
              BoxShadow(color: Color(0x10000000), blurRadius: 18, offset: Offset(0, 6)),
            ],
          ),
          child: ClipRRect(
            borderRadius: BorderRadius.circular(26),
            child: NavigationBar(
              selectedIndex: widget.navigationShell.currentIndex,
              onDestinationSelected: (index) {
                widget.navigationShell.goBranch(
                  index,
                  initialLocation: index == widget.navigationShell.currentIndex,
                );
              },
              destinations: const [
                NavigationDestination(
                  icon: Icon(Icons.chat_bubble_outline_rounded),
                  selectedIcon: Icon(Icons.chat_bubble_rounded),
                  label: '聊天',
                ),
                NavigationDestination(
                  icon: Icon(Icons.group_outlined),
                  selectedIcon: Icon(Icons.group_rounded),
                  label: '协同',
                ),
                NavigationDestination(
                  icon: Icon(Icons.key_outlined),
                  selectedIcon: Icon(Icons.key_rounded),
                  label: '秘钥',
                ),
                NavigationDestination(
                  icon: Icon(Icons.person_outline_rounded),
                  selectedIcon: Icon(Icons.person_rounded),
                  label: '我的',
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

UserAnnouncement? _firstUnreadPopup(List<UserAnnouncement> announcements) {
  for (final announcement in announcements) {
    if (announcement.isUnread && announcement.notifyMode == 'popup') {
      return announcement;
    }
  }
  return null;
}
