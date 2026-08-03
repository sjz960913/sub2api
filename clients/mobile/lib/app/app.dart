import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'router.dart';
import 'theme.dart';

class Sub2ApiApp extends ConsumerWidget {
  const Sub2ApiApp({super.key});

  @override
  Widget build(BuildContext _, WidgetRef ref) {
    return MaterialApp.router(
      title: 'Sub2API',
      debugShowCheckedModeBanner: false,
      theme: buildSub2ApiTheme(),
      routerConfig: ref.watch(appRouterProvider),
      supportedLocales: const [Locale('zh'), Locale('en')],
      localizationsDelegates: const [
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
    );
  }
}
