import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

class SiteSetupPage extends StatelessWidget {
  const SiteSetupPage({super.key});

  @override
  Widget build(BuildContext context) {
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
              const TextField(keyboardType: TextInputType.url, decoration: InputDecoration(labelText: '站点地址', hintText: 'https://example.com')),
              const SizedBox(height: 16),
              FilledButton(onPressed: () => context.go('/auth/login'), child: const Text('检测并继续')),
            ],
          ),
        ),
      ),
    );
  }
}
