enum ApiKeyKind { text, image }

class ApiKeySummary {
  const ApiKeySummary({
    required this.id,
    required this.name,
    required this.secretKey,
    required this.maskedKey,
    required this.group,
    required this.availableGroups,
    required this.groupIdsByName,
    required this.imageGroups,
    required this.monthlyUsage,
    required this.isSelected,
  });

  final String id;
  final String name;
  final String secretKey;
  final String maskedKey;
  final String? group;
  final List<String> availableGroups;
  final Map<String, String> groupIdsByName;
  final Set<String> imageGroups;
  final String monthlyUsage;
  final bool isSelected;

  ApiKeyKind get kind => imageGroups.contains(group) ? ApiKeyKind.image : ApiKeyKind.text;

  ApiKeySummary copyWith({String? group, bool? isSelected}) {
    return ApiKeySummary(
      id: id,
      name: name,
      secretKey: secretKey,
      maskedKey: maskedKey,
      group: group ?? this.group,
      availableGroups: availableGroups,
      groupIdsByName: groupIdsByName,
      imageGroups: imageGroups,
      monthlyUsage: monthlyUsage,
      isSelected: isSelected ?? this.isSelected,
    );
  }
}
