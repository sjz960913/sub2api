import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

class LoginPage extends StatelessWidget {
  const LoginPage({super.key});

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
              Text('登录', style: Theme.of(context).textTheme.headlineMedium),
              const SizedBox(height: 8),
              const Text('登录凭证仅发送到当前站点。'),
              const SizedBox(height: 28),
              const TextField(keyboardType: TextInputType.emailAddress, decoration: InputDecoration(labelText: '邮箱')),
              const SizedBox(height: 14),
              const TextField(obscureText: true, decoration: InputDecoration(labelText: '密码')),
              const SizedBox(height: 18),
              FilledButton(onPressed: () => context.go('/app/chat'), child: const Text('登录')),
            ],
          ),
        ),
      ),
    );
  }
}
