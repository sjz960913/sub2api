import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../core/auth/user_role.dart';
import '../features/admin_placeholder/presentation/admin_placeholder_page.dart';
import '../features/api_keys/presentation/api_keys_page.dart';
import '../features/auth/presentation/login_page.dart';
import '../features/chat/presentation/chat_page.dart';
import '../features/chat/presentation/chat_thread_page.dart';
import '../features/codex_sessions/presentation/collaboration_page.dart';
import '../features/codex_sessions/presentation/collaboration_thread_page.dart';
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
      GoRoute(path: '/site/setup', builder: (_, _) => const SiteSetupPage()),
      GoRoute(path: '/auth/login', builder: (_, _) => const LoginPage()),
      StatefulShellRoute.indexedStack(
        builder: (_, _, shell) => AppShell(navigationShell: shell),
        branches: [
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/app/chat',
                builder: (_, _) => const ChatPage(),
                routes: [
                  GoRoute(
                    path: 'new',
                    builder: (_, _) => const ChatThreadPage(conversationId: 'new'),
                  ),
                  GoRoute(
                    path: ':conversation_id',
                    builder: (_, state) => ChatThreadPage(
                      conversationId: state.pathParameters['conversation_id']!,
                    ),
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [
              GoRoute(
                path: '/app/collab',
                builder: (_, _) => const CollaborationPage(),
                routes: [
                  GoRoute(
                    path: 'thread/:session_id',
                    builder: (_, state) => CollaborationThreadPage(
                      sessionId: state.pathParameters['session_id']!,
                    ),
                  ),
                ],
              ),
            ],
          ),
          StatefulShellBranch(
            routes: [GoRoute(path: '/app/keys', builder: (_, _) => const ApiKeysPage())],
          ),
          StatefulShellBranch(
            routes: [GoRoute(path: '/app/profile', builder: (_, _) => const ProfilePage())],
          ),
        ],
      ),
      GoRoute(
        path: '/admin/coming-soon',
        parentNavigatorKey: _rootNavigatorKey,
        builder: (_, _) => const AdminPlaceholderPage(),
      ),
    ],
  );
});
