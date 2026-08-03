import 'package:flutter_riverpod/flutter_riverpod.dart';

enum ApiKeyKind { text, image }

class ApiKeySummary {
  const ApiKeySummary({
    required this.id,
    required this.name,
    required this.maskedKey,
    required this.group,
    required this.availableGroups,
    required this.monthlyUsage,
    required this.kind,
    required this.isSelected,
  });

  final String id;
  final String name;
  final String maskedKey;
  final String group;
  final List<String> availableGroups;
  final String monthlyUsage;
  final ApiKeyKind kind;
  final bool isSelected;

  ApiKeySummary copyWith({String? group, bool? isSelected}) {
    return ApiKeySummary(
      id: id,
      name: name,
      maskedKey: maskedKey,
      group: group ?? this.group,
      availableGroups: availableGroups,
      monthlyUsage: monthlyUsage,
      kind: kind,
      isSelected: isSelected ?? this.isSelected,
    );
  }
}

final apiKeyCatalogProvider = NotifierProvider<ApiKeyCatalog, List<ApiKeySummary>>(
  ApiKeyCatalog.new,
);

final selectedChatKeyProvider = Provider<ApiKeySummary?>((ref) {
  final catalog = ref.watch(apiKeyCatalogProvider);
  for (final key in catalog) {
    if (key.isSelected) {
      return key;
    }
  }
  return null;
});

class ApiKeyCatalog extends Notifier<List<ApiKeySummary>> {
  @override
  List<ApiKeySummary> build() {
    // M4 replaces this preview catalog with the authenticated site repository.
    // UI state intentionally stores only masked values and identifiers.
    return const [
      ApiKeySummary(
        id: 'mobile-chat',
        name: 'Mobile Chat',
        maskedKey: 'sk-••••8H2Q',
        group: 'OpenAI 默认',
        availableGroups: ['OpenAI 默认', 'OpenAI 图片'],
        monthlyUsage: r'$3.26',
        kind: ApiKeyKind.text,
        isSelected: true,
      ),
      ApiKeySummary(
        id: 'image-lab',
        name: 'Image Lab',
        maskedKey: 'sk-••••1K9M',
        group: 'OpenAI 图片',
        availableGroups: ['OpenAI 图片', 'OpenAI 默认'],
        monthlyUsage: r'$1.08',
        kind: ApiKeyKind.image,
        isSelected: false,
      ),
    ];
  }

  void selectForChat(String id) {
    if (!state.any((key) => key.id == id)) {
      return;
    }
    state = [for (final key in state) key.copyWith(isSelected: key.id == id)];
  }

  void updateGroup(String id, String group) {
    state = [
      for (final key in state)
        if (key.id == id && key.availableGroups.contains(group))
          key.copyWith(group: group)
        else
          key,
    ];
  }
}
