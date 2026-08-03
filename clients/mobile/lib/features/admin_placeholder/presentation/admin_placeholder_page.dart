import 'package:flutter/material.dart';

class AdminPlaceholderPage extends StatelessWidget {
  const AdminPlaceholderPage({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('管理控制台')),
      body: const Center(child: Text('管理员功能将在后续版本开放')),
    );
  }
}
