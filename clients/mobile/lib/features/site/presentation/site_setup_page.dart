import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../auth/application/session_controller.dart';

class SiteSetupPage extends ConsumerStatefulWidget {
  const SiteSetupPage({super.key});

  @override
  ConsumerState<SiteSetupPage> createState() => _SiteSetupPageState();
}

class _SiteSetupPageState extends ConsumerState<SiteSetupPage> {
  final siteController = TextEditingController();

  @override
  void dispose() {
    siteController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionControllerProvider);
    return Scaffold(
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 24),
              Text('连接 Sub2API', style: Theme.of(context).textTheme.headlineMedium),
              const SizedBox(height: 32),
              TextField(
                controller: siteController,
                enabled: !session.isBusy,
                keyboardType: TextInputType.url,
                textInputAction: TextInputAction.done,
                autocorrect: false,
                decoration: const InputDecoration(
                  labelText: '站点地址',
                  hintText: 'https://example.com',
                ),
                onSubmitted: session.isBusy ? null : (_) => _connect(),
              ),
              if (session.errorCode != null) ...[
                const SizedBox(height: 10),
                Text(
                  _siteErrorMessage(session.errorCode!),
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
              const SizedBox(height: 16),
              FilledButton(
                onPressed: session.isBusy ? null : _connect,
                child: session.isBusy
                    ? const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Text('检测并继续'),
              ),
              const SizedBox(height: 12),
              const Text(
                '远程站点必须使用 HTTPS；仅本机调试地址允许 HTTP。',
                textAlign: TextAlign.center,
                style: TextStyle(color: Colors.black54, fontSize: 12),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _connect() async {
    FocusScope.of(context).unfocus();
    await ref.read(sessionControllerProvider.notifier).configureSite(siteController.text);
  }
}

String _siteErrorMessage(String code) {
  return switch (code) {
    'PANEL_INVALID_SITE' => '请输入有效的站点地址',
    'PANEL_INSECURE_SITE' => '远程站点必须使用 HTTPS',
    'PANEL_NETWORK_ERROR' => '无法连接站点，请检查地址与网络',
    _ => '站点检测失败，请稍后重试',
  };
}
