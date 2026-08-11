import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/app/app.dart';
import 'package:sub2api_mobile/app/router.dart';
import 'package:sub2api_mobile/features/api_keys/application/api_key_catalog.dart';
import 'package:sub2api_mobile/features/api_keys/domain/api_key_summary.dart';
import 'package:sub2api_mobile/features/auth/application/session_controller.dart';
import 'package:sub2api_mobile/features/auth/domain/panel_session.dart';
import 'package:sub2api_mobile/features/profile/application/profile_controller.dart';
import 'package:sub2api_mobile/features/profile/domain/user_announcement.dart';
import 'package:sub2api_mobile/features/profile/presentation/profile_page.dart';

const _authenticatedSession = SessionState(
  phase: SessionPhase.authenticated,
  siteUrl: 'https://panel.example.com/',
  user: PanelUser(
    id: 1,
    email: 'user@example.com',
    username: 'user',
    role: PanelRole.user,
  ),
);

const _previewKeys = [
  ApiKeySummary(
    id: 'mobile-chat',
    name: 'Mobile Chat',
    secretKey: 'sk-test-mobile-chat',
    maskedKey: 'sk-••••8H2Q',
    group: 'OpenAI 默认',
    availableGroups: ['OpenAI 默认', 'OpenAI 图片'],
    groupIdsByName: {'OpenAI 默认': '1', 'OpenAI 图片': '2'},
    imageGroups: {'OpenAI 图片'},
    monthlyUsage: r'$3.26',
    isSelected: true,
  ),
  ApiKeySummary(
    id: 'image-lab',
    name: 'Image Lab',
    secretKey: 'sk-test-image-lab',
    maskedKey: 'sk-••••1K9M',
    group: 'OpenAI 图片',
    availableGroups: ['OpenAI 默认', 'OpenAI 图片'],
    groupIdsByName: {'OpenAI 默认': '1', 'OpenAI 图片': '2'},
    imageGroups: {'OpenAI 图片'},
    monthlyUsage: r'$1.08',
    isSelected: false,
  ),
];

void main() {
  testWidgets('main shell exposes exactly the four product tabs', (
    tester,
  ) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          initialSessionStateProvider.overrideWithValue(_authenticatedSession),
          apiKeyCatalogSeedProvider.overrideWithValue(_previewKeys),
        ],
        child: const Sub2ApiApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byType(NavigationDestination), findsNWidgets(4));
    expect(find.text('聊天'), findsWidgets);
    expect(find.text('协同'), findsOneWidget);
    expect(find.text('秘钥'), findsOneWidget);
    expect(find.text('我的'), findsOneWidget);
    expect(find.textContaining('退款'), findsNothing);
    expect(find.textContaining('审批'), findsNothing);
    expect(find.textContaining('0.10'), findsNothing);
  });

  testWidgets('ordinary user cannot deep-link into admin routes', (
    tester,
  ) async {
    final container = ProviderContainer(
      overrides: [
        initialSessionStateProvider.overrideWithValue(_authenticatedSession),
        apiKeyCatalogSeedProvider.overrideWithValue(_previewKeys),
      ],
    );
    addTearDown(container.dispose);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const Sub2ApiApp(),
      ),
    );
    await tester.pumpAndSettle();

    container.read(appRouterProvider).go('/admin/coming-soon');
    await tester.pumpAndSettle();

    expect(find.text('管理控制台'), findsNothing);
    expect(find.text('我的'), findsWidgets);
  });

  testWidgets('selected key is shared by key catalog and chat', (tester) async {
    final container = ProviderContainer(
      overrides: [
        initialSessionStateProvider.overrideWithValue(_authenticatedSession),
        apiKeyCatalogSeedProvider.overrideWithValue(_previewKeys),
      ],
    );
    addTearDown(container.dispose);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const Sub2ApiApp(),
      ),
    );
    await tester.pumpAndSettle();

    container.read(apiKeyCatalogProvider.notifier).selectForChat('image-lab');
    await tester.pump();

    expect(container.read(selectedChatKeyProvider)?.name, 'Image Lab');
    expect(find.text('Image Lab'), findsOneWidget);
    expect(find.textContaining('OpenAI 图片'), findsOneWidget);
    expect(find.textContaining('sk-test'), findsNothing);
  });

  testWidgets('collaboration stays minimal and contains no fake sessions', (
    tester,
  ) async {
    final container = ProviderContainer(
      overrides: [
        initialSessionStateProvider.overrideWithValue(_authenticatedSession),
        apiKeyCatalogSeedProvider.overrideWithValue(_previewKeys),
      ],
    );
    addTearDown(container.dispose);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const Sub2ApiApp(),
      ),
    );
    await tester.pumpAndSettle();

    container.read(appRouterProvider).go('/app/collab');
    await tester.pumpAndSettle();

    expect(find.text('查询电脑会话'), findsOneWidget);
    expect(find.text('修复支付回调'), findsNothing);
    expect(find.textContaining('审批'), findsNothing);
    expect(find.textContaining('退款'), findsNothing);
  });

  testWidgets('profile exposes redeem recharge and announcement entries', (
    tester,
  ) async {
    final container = ProviderContainer(
      overrides: [
        initialSessionStateProvider.overrideWithValue(_authenticatedSession),
        apiKeyCatalogSeedProvider.overrideWithValue(_previewKeys),
      ],
    );
    addTearDown(container.dispose);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const Sub2ApiApp(),
      ),
    );
    await tester.pumpAndSettle();

    container.read(appRouterProvider).go('/app/profile');
    await tester.pumpAndSettle();

    expect(rechargeUri, 'https://pay.ldxp.cn/shop/codecodeai');
    expect(find.text('兑换'), findsOneWidget);
    expect(find.text('充值'), findsOneWidget);
    expect(find.text('公告'), findsOneWidget);
    expect(find.textContaining('0.10'), findsNothing);
  });

  testWidgets('unread popup announcement is shown once and can be dismissed', (
    tester,
  ) async {
    final container = ProviderContainer(
      overrides: [
        initialSessionStateProvider.overrideWithValue(_authenticatedSession),
        apiKeyCatalogSeedProvider.overrideWithValue(_previewKeys),
        profileStateSeedProvider.overrideWithValue(
          const ProfileState(
            announcements: [
              UserAnnouncement(
                id: 7,
                title: '系统公告',
                content: '今日服务已更新。',
                notifyMode: 'popup',
                createdAt: null,
                readAt: null,
              ),
            ],
          ),
        ),
      ],
    );
    addTearDown(container.dispose);
    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: const Sub2ApiApp(),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('系统公告'), findsOneWidget);
    expect(find.text('今日服务已更新。'), findsOneWidget);
    await tester.tap(find.text('知道了'));
    await tester.pumpAndSettle();

    expect(find.text('今日服务已更新。'), findsNothing);
  });
}
