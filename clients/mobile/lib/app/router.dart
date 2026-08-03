import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/auth/user_role.dart';
import '../features/admin_placeholder/presentation/admin_placeholder_page.dart';
import '../features/api_keys/presentation/api_keys_page.dart';
import '../features/auth/presentation/login_page.dart';
import '../features/chat/presentation/chat_page.dart';
import '../features/codex_sessions/presentation/collaboration_page.dart';
import '../features/profile/presentation/profile_page.dart';
import '../features/site/presentation/site_setup_page.dart';
import 'shell/app_shell.dart';

final _rootNavigatorKey = GlobalKey<NavigatorState>();

final appRouterProvider = Provider<GoRouter>((ref) {
  final role = ref.watch(currentUserRoleProvider);
  return GoRouter(
    navigatorKey: _rootNavigatorKey,
    initialLocation: '/app/chat',
    redirect: (context, state) {
      if (state.matchedLocation.startsWith('/admin') && role != UserRole.admin) {
        return '/app/profile';
      }
      return null;
    },
    routes: [
      GoRoute(path: '/site/setup', builder: (_, __) => const SiteSetupPage()),
      GoRoute(path: '/auth/login', builder: (_, __) => const LoginPage()),
      StatefulShellRoute.indexedStack(
        builder: (_, __, shell) => AppShell(navigationShell: shell),
        branches: [
          StatefulShellBranch(
            routes: [GoRoute(path: '/app/chat', builder: (_, __) => const ChatPage())],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/app/collab',
                builder: (_, __) => const CollaborationPage(),
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [GoRoute(path: '/app/keys', builder: (_, __) => const ApiKeysPage())],
          ),
          StatefulShellBranch(
            routes: [GoRoute(path: '/app/profile', builder: (_, __) => const ProfilePage())],
          ),
        ],
      ),
      GoRoute(
        path: '/admin/coming-soon',
        parentNavigatorKey: _rootNavigatorKey,
        builder: (_, __) => const AdminPlaceholderPage(),
      ),
    ],
  );
});
