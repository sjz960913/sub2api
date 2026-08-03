import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../application/session_controller.dart';

class LoginPage extends ConsumerStatefulWidget {
  const LoginPage({super.key});

  @override
  ConsumerState<LoginPage> createState() => _LoginPageState();
}

class _LoginPageState extends ConsumerState<LoginPage> {
  final emailController = TextEditingController();
  final passwordController = TextEditingController();
  final totpController = TextEditingController();

  @override
  void dispose() {
    emailController.dispose();
    passwordController.dispose();
    totpController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final session = ref.watch(sessionControllerProvider);
    final requiresTwoFactor = session.phase == SessionPhase.requiresTwoFactor;
    return Scaffold(
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              const SizedBox(height: 24),
              Row(
                children: [
                  Expanded(
                    child: Text('登录', style: Theme.of(context).textTheme.headlineMedium),
                  ),
                  TextButton(
                    onPressed: session.isBusy
                        ? null
                        : () => ref.read(sessionControllerProvider.notifier).changeSite(),
                    child: const Text('更换站点'),
                  ),
                ],
              ),
              const SizedBox(height: 8),
              Text(session.siteUrl ?? '登录凭证仅发送到当前站点。'),
              const SizedBox(height: 28),
              if (requiresTwoFactor) ...[
                Text(
                  session.emailMasked == null
                      ? '请输入身份验证器中的 6 位验证码'
                      : '请输入 ${session.emailMasked} 的 6 位验证码',
                ),
                const SizedBox(height: 14),
                TextField(
                  controller: totpController,
                  enabled: !session.isBusy,
                  keyboardType: TextInputType.number,
                  maxLength: 6,
                  textInputAction: TextInputAction.done,
                  decoration: const InputDecoration(labelText: '两步验证码', counterText: ''),
                  onSubmitted: session.isBusy ? null : (_) => _submitTwoFactor(),
                ),
              ] else ...[
                TextField(
                  controller: emailController,
                  enabled: !session.isBusy,
                  keyboardType: TextInputType.emailAddress,
                  textInputAction: TextInputAction.next,
                  autocorrect: false,
                  decoration: const InputDecoration(labelText: '邮箱'),
                ),
                const SizedBox(height: 14),
                TextField(
                  controller: passwordController,
                  enabled: !session.isBusy,
                  obscureText: true,
                  textInputAction: TextInputAction.done,
                  decoration: const InputDecoration(labelText: '密码'),
                  onSubmitted: session.isBusy ? null : (_) => _submitLogin(),
                ),
              ],
              if (session.errorCode != null) ...[
                const SizedBox(height: 10),
                Text(
                  _loginErrorMessage(session.errorCode!),
                  style: TextStyle(color: Theme.of(context).colorScheme.error),
                ),
              ],
              const SizedBox(height: 18),
              FilledButton(
                onPressed: session.isBusy
                    ? null
                    : requiresTwoFactor
                    ? _submitTwoFactor
                    : _submitLogin,
                child: session.isBusy
                    ? const SizedBox.square(
                        dimension: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : Text(requiresTwoFactor ? '验证并登录' : '登录'),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Future<void> _submitLogin() async {
    FocusScope.of(context).unfocus();
    await ref
        .read(sessionControllerProvider.notifier)
        .login(emailController.text, passwordController.text);
  }

  Future<void> _submitTwoFactor() async {
    FocusScope.of(context).unfocus();
    await ref
        .read(sessionControllerProvider.notifier)
        .completeTwoFactor(totpController.text);
  }
}

String _loginErrorMessage(String code) {
  return switch (code) {
    'PANEL_INVALID_EMAIL' => '请输入有效邮箱',
    'PANEL_INVALID_PASSWORD' => '请输入密码',
    'PANEL_INVALID_TWO_FACTOR_CODE' => '请输入 6 位验证码',
    'PANEL_UNAUTHORIZED' => '邮箱、密码或验证码不正确',
    'PANEL_FORBIDDEN' => '当前账号无法登录此站点',
    'PANEL_RATE_LIMITED' => '尝试次数过多，请稍后再试',
    'PANEL_NETWORK_ERROR' => '网络连接失败，请稍后重试',
    _ => '登录失败，请检查信息后重试',
  };
}
