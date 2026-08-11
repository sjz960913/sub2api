import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../auth/application/session_controller.dart';
import '../data/api_key_repository.dart';
import '../domain/api_key_summary.dart';

class ApiKeyCatalogState {
  const ApiKeyCatalogState({
    this.keys = const [],
    this.isLoading = false,
    this.errorCode,
  });

  final List<ApiKeySummary> keys;
  final bool isLoading;
  final String? errorCode;

  ApiKeyCatalogState copyWith({
    List<ApiKeySummary>? keys,
    bool? isLoading,
    String? errorCode,
    bool clearError = false,
  }) {
    return ApiKeyCatalogState(
      keys: keys ?? this.keys,
      isLoading: isLoading ?? this.isLoading,
      errorCode: clearError ? null : errorCode ?? this.errorCode,
    );
  }
}

final apiKeyCatalogSeedProvider = Provider<List<ApiKeySummary>>(
  (_) => const [],
);

final apiKeyCatalogProvider =
    NotifierProvider<ApiKeyCatalog, ApiKeyCatalogState>(ApiKeyCatalog.new);

final selectedChatKeyProvider = Provider<ApiKeySummary?>((ref) {
  final catalog = ref.watch(apiKeyCatalogProvider).keys;
  for (final key in catalog) {
    if (key.isSelected) {
      return key;
    }
  }
  return null;
});

class ApiKeyCatalog extends Notifier<ApiKeyCatalogState> {
  @override
  ApiKeyCatalogState build() {
    final authenticated = ref.watch(
      sessionControllerProvider.select((session) => session.isAuthenticated),
    );
    if (!authenticated) {
      return const ApiKeyCatalogState();
    }
    final seed = ref.watch(apiKeyCatalogSeedProvider);
    if (seed.isEmpty) {
      Future<void>.microtask(load);
    }
    return ApiKeyCatalogState(keys: seed);
  }

  Future<void> load({bool force = false}) async {
    if (state.isLoading || (!force && state.keys.isNotEmpty)) {
      return;
    }
    state = state.copyWith(isLoading: true, clearError: true);
    final selectedId = state.keys
        .where((key) => key.isSelected)
        .firstOrNull
        ?.id;
    try {
      final keys = await ref.read(apiKeyRepositoryProvider).listOpenAIKeys();
      final preferredId =
          selectedId ?? keys.where((key) => key.group != null).firstOrNull?.id;
      state = ApiKeyCatalogState(
        keys: [
          for (final key in keys)
            key.copyWith(
              isSelected: key.group != null && key.id == preferredId,
            ),
        ],
      );
    } on ApiKeyRepositoryException catch (error) {
      state = state.copyWith(isLoading: false, errorCode: error.publicCode);
    }
  }

  void selectForChat(String id) {
    if (!state.keys.any((key) => key.id == id && key.group != null)) {
      return;
    }
    state = state.copyWith(
      keys: [
        for (final key in state.keys) key.copyWith(isSelected: key.id == id),
      ],
    );
  }

  Future<bool> updateGroup(String id, String group) async {
    final index = state.keys.indexWhere((key) => key.id == id);
    if (index < 0 || !state.keys[index].availableGroups.contains(group)) {
      return false;
    }
    final previous = state;
    state = state.copyWith(
      keys: [
        for (final key in state.keys)
          if (key.id == id) key.copyWith(group: group) else key,
      ],
      clearError: true,
    );
    try {
      final groupId = state.keys[index].groupIdsByName[group];
      if (groupId == null) {
        throw const ApiKeyRepositoryException('API_KEY_INVALID_GROUP');
      }
      await ref.read(apiKeyRepositoryProvider).updateGroup(id, groupId);
      if (!state.keys.any((key) => key.isSelected)) {
        selectForChat(id);
      }
      return true;
    } on ApiKeyRepositoryException catch (error) {
      state = previous.copyWith(errorCode: error.publicCode);
      return false;
    }
  }
}

extension<T> on Iterable<T> {
  T? get firstOrNull {
    final iterator = this.iterator;
    return iterator.moveNext() ? iterator.current : null;
  }
}
