import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sub2api_mobile/app/app.dart';
import 'package:sub2api_mobile/app/router.dart';

void main() {
  testWidgets('main shell exposes exactly the four product tabs', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: Sub2ApiApp()));
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

  testWidgets('ordinary user cannot deep-link into admin routes', (tester) async {
    final container = ProviderContainer();
    addTearDown(container.dispose);
    await tester.pumpWidget(UncontrolledProviderScope(container: container, child: const Sub2ApiApp()));
    await tester.pumpAndSettle();

    container.read(appRouterProvider).go('/admin/coming-soon');
    await tester.pumpAndSettle();

    expect(find.text('管理控制台'), findsNothing);
    expect(find.text('我的'), findsWidgets);
  });
}
