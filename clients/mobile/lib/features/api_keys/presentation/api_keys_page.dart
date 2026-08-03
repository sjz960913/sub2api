import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../app/theme.dart';
import '../../../core/widgets/page_frame.dart';
import '../application/api_key_catalog.dart';
import '../domain/api_key_summary.dart';

class ApiKeysPage extends ConsumerWidget {
  const ApiKeysPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final catalog = ref.watch(apiKeyCatalogProvider);
    final keys = catalog.keys;
    return PageFrame(
      title: '秘钥',
      child: Column(
        children: [
          if (catalog.isLoading && keys.isEmpty)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 48),
              child: CircularProgressIndicator(),
            )
          else if (catalog.errorCode != null && keys.isEmpty)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 32),
              child: Column(
                children: [
                  const Text('无法加载秘钥，请检查网络后重试'),
                  const SizedBox(height: 12),
                  OutlinedButton.icon(
                    onPressed: () => ref.read(apiKeyCatalogProvider.notifier).load(force: true),
                    icon: const Icon(Icons.refresh_rounded),
                    label: const Text('重试'),
                  ),
                ],
              ),
            )
          else if (keys.isEmpty)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 48),
              child: Text('当前账号没有可用秘钥'),
            ),
          for (var index = 0; index < keys.length; index++) ...[
            _ApiKeyCard(
              key: ValueKey(keys[index].id),
              apiKey: keys[index],
              onGroupChanged: (group) async {
                final changed = await ref
                    .read(apiKeyCatalogProvider.notifier)
                    .updateGroup(keys[index].id, group);
                if (!changed && context.mounted) {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('分组切换失败，请稍后重试')),
                  );
                }
              },
              onSelect: () {
                ref.read(apiKeyCatalogProvider.notifier).selectForChat(keys[index].id);
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(content: Text('已切换至 ${keys[index].name}')),
                );
              },
            ),
            if (index != keys.length - 1) const SizedBox(height: 14),
          ],
        ],
      ),
    );
  }
}

class _ApiKeyCard extends StatelessWidget {
  const _ApiKeyCard({
    required this.apiKey,
    required this.onGroupChanged,
    required this.onSelect,
    super.key,
  });

  final ApiKeySummary apiKey;
  final ValueChanged<String> onGroupChanged;
  final VoidCallback onSelect;

  @override
  Widget build(BuildContext context) {
    return Card(
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(16),
        side: BorderSide(
          color: apiKey.isSelected ? AppColors.primary : AppColors.border,
          width: apiKey.isSelected ? 1.4 : 1,
        ),
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
                  decoration: BoxDecoration(
                    color: AppColors.iconTile,
                    borderRadius: BorderRadius.circular(14),
                  ),
                  child: Icon(
                    apiKey.kind == ApiKeyKind.image ? Icons.image_outlined : Icons.key_rounded,
                    color: AppColors.primary,
                  ),
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        apiKey.name,
                        style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 17),
                      ),
                      const SizedBox(height: 4),
                      Text(apiKey.maskedKey, style: const TextStyle(color: AppColors.muted)),
                    ],
                  ),
                ),
                Icon(
                  apiKey.isSelected
                      ? Icons.radio_button_checked_rounded
                      : Icons.radio_button_off_rounded,
                  color: apiKey.isSelected ? AppColors.primary : AppColors.muted,
                ),
              ],
            ),
            const SizedBox(height: 18),
            const Text('分组', style: TextStyle(color: AppColors.muted, fontSize: 12)),
            const SizedBox(height: 6),
            DropdownButtonFormField<String>(
              key: ValueKey('${apiKey.id}:${apiKey.group}'),
              initialValue: apiKey.group,
              hint: const Text('请选择 OpenAI 分组'),
              items: apiKey.availableGroups
                  .map((item) => DropdownMenuItem(value: item, child: Text(item)))
                  .toList(),
              onChanged: (value) {
                if (value != null) {
                  onGroupChanged(value);
                }
              },
            ),
            const SizedBox(height: 16),
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                const Text('用量', style: TextStyle(color: AppColors.muted)),
                Text(apiKey.monthlyUsage, style: const TextStyle(fontWeight: FontWeight.w700)),
              ],
            ),
            const SizedBox(height: 16),
            if (apiKey.group == null)
              const OutlinedButton(onPressed: null, child: Text('请先选择分组'))
            else if (apiKey.isSelected)
              FilledButton.icon(
                onPressed: null,
                style: FilledButton.styleFrom(
                  disabledBackgroundColor: AppColors.primary,
                  disabledForegroundColor: Colors.white,
                ),
                icon: const Icon(Icons.check_rounded),
                label: const Text('当前聊天使用'),
              )
            else
              OutlinedButton(onPressed: onSelect, child: const Text('设为当前')),
          ],
        ),
      ),
    );
  }
}
