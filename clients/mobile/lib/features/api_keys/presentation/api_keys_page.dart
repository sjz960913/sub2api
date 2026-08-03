import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/page_frame.dart';

final selectedChatKeyProvider = NotifierProvider<SelectedChatKey, String>(SelectedChatKey.new);

class SelectedChatKey extends Notifier<String> {
  @override
  String build() => 'mobile-chat';

  void select(String id) => state = id;
}

class ApiKeysPage extends ConsumerWidget {
  const ApiKeysPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final selected = ref.watch(selectedChatKeyProvider);
    return PageFrame(
      title: '秘钥',
      actions: [IconButton(onPressed: () {}, icon: const Icon(Icons.add_circle_outline_rounded))],
      child: Column(
        children: [
          _ApiKeyCard(
            name: 'Mobile Chat',
            maskedKey: 'sk-••••8H2Q',
            usage: r'$3.26',
            groups: const ['OpenAI 默认', 'OpenAI 图片'],
            selected: selected == 'mobile-chat',
            onSelect: () => ref.read(selectedChatKeyProvider.notifier).select('mobile-chat'),
          ),
          const SizedBox(height: 14),
          _ApiKeyCard(
            name: 'Image Lab',
            maskedKey: 'sk-••••1K9M',
            usage: r'$1.08',
            groups: const ['OpenAI 图片', 'OpenAI 默认'],
            selected: selected == 'image-lab',
            onSelect: () => ref.read(selectedChatKeyProvider.notifier).select('image-lab'),
          ),
        ],
      ),
    );
  }
}

class _ApiKeyCard extends StatefulWidget {
  const _ApiKeyCard({
    required this.name,
    required this.maskedKey,
    required this.usage,
    required this.groups,
    required this.selected,
    required this.onSelect,
  });

  final String name;
  final String maskedKey;
  final String usage;
  final List<String> groups;
  final bool selected;
  final VoidCallback onSelect;

  @override
  State<_ApiKeyCard> createState() => _ApiKeyCardState();
}

class _ApiKeyCardState extends State<_ApiKeyCard> {
  late String group = widget.groups.first;

  @override
  Widget build(BuildContext context) {
    return Card(
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(color: widget.selected ? AppColors.primary : AppColors.border),
      ),
      child: Padding(
        padding: const EdgeInsets.all(18),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Row(
              children: [
                Container(
                  width: 52,
                  height: 52,
                  decoration: BoxDecoration(color: AppColors.iconTile, borderRadius: BorderRadius.circular(14)),
                  child: const Icon(Icons.key_rounded, color: AppColors.primary),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(widget.name, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 17)),
                      const SizedBox(height: 4),
                      Text(widget.maskedKey, style: const TextStyle(color: AppColors.muted)),
                    ],
                  ),
                ),
                if (widget.selected) const Icon(Icons.radio_button_checked, color: AppColors.primary),
              ],
            ),
            const SizedBox(height: 18),
            const Text('分组', style: TextStyle(color: AppColors.muted, fontSize: 12)),
            const SizedBox(height: 6),
            DropdownButtonFormField<String>(
              initialValue: group,
              items: widget.groups.map((item) => DropdownMenuItem(value: item, child: Text(item))).toList(),
              onChanged: (value) => setState(() => group = value ?? group),
            ),
            const SizedBox(height: 16),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [const Text('本月用量', style: TextStyle(color: AppColors.muted)), Text(widget.usage, style: const TextStyle(fontWeight: FontWeight.w700))],
            ),
            const SizedBox(height: 16),
            if (widget.selected)
              const FilledButton(onPressed: null, child: Text('✓ 当前聊天使用'))
            else
              OutlinedButton(onPressed: widget.onSelect, child: const Text('设为当前')),
          ],
        ),
      ),
    );
  }
}
