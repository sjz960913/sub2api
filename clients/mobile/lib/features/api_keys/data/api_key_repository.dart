import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/panel_api_client.dart';
import '../domain/api_key_summary.dart';

final apiKeyRepositoryProvider = Provider<ApiKeyRepository>(
  (ref) => ApiKeyRepository(ref.watch(panelApiClientProvider)),
);

class ApiKeyRepositoryException implements Exception {
  const ApiKeyRepositoryException(this.publicCode);

  final String publicCode;
}

class ApiKeyRepository {
  const ApiKeyRepository(this._client);

  final PanelApiClient _client;

  Future<List<ApiKeySummary>> listOpenAIKeys() async {
    try {
      final keyResponse = await _client.request(
        'GET',
        'keys',
        query: {'page': 1, 'page_size': 100},
      );
      final page = _asMap(keyResponse);
      final rawKeys = _asList(
        page['items'],
      ).map(_asMap).toList(growable: false);

      List<Map<String, dynamic>> groups;
      try {
        final groupResponse = await _client.request('GET', 'groups/available');
        groups = _asList(groupResponse).map(_asMap).toList(growable: false);
      } catch (_) {
        // A temporary failure of the group metadata endpoint must not hide keys
        // that were already returned successfully.
        groups = _groupsFromKeys(rawKeys);
      }
      groups = groups
          .map(_asMap)
          .where(
            (group) =>
                group['platform'] == 'openai' && group['status'] == 'active',
          )
          .toList(growable: false);
      final names = <String>[];
      final idsByName = <String, String>{};
      final imageGroups = <String>{};
      for (final group in groups) {
        final id = group['id'];
        final name = group['name'];
        if (id is! num || name is! String || name.isEmpty) {
          continue;
        }
        names.add(name);
        idsByName[name] = id.toInt().toString();
        if (group['allow_image_generation'] == true) {
          imageGroups.add(name);
        }
      }
      return rawKeys
          .where((key) => key['status'] == 'active')
          .map(
            (key) => _parseKey(
              key,
              availableGroups: names,
              groupIdsByName: idsByName,
              imageGroups: imageGroups,
            ),
          )
          .where((key) => key != null)
          .cast<ApiKeySummary>()
          .toList(growable: false);
    } on PanelApiException catch (error) {
      throw ApiKeyRepositoryException(error.publicCode);
    } on ApiKeyRepositoryException {
      rethrow;
    } catch (_) {
      throw const ApiKeyRepositoryException('API_KEY_INVALID_RESPONSE');
    }
  }

  static List<Map<String, dynamic>> _groupsFromKeys(
    List<Map<String, dynamic>> keys,
  ) {
    final groups = <String, Map<String, dynamic>>{};
    for (final key in keys) {
      final rawGroup = key['group'];
      if (rawGroup is! Map) {
        continue;
      }
      final group = _asMap(rawGroup);
      final id = group['id'];
      final name = group['name'];
      if (id is num && name is String && name.isNotEmpty) {
        groups[id.toInt().toString()] = group;
      }
    }
    return groups.values.toList(growable: false);
  }

  Future<void> updateGroup(String keyId, String groupId) async {
    final parsedKeyId = int.tryParse(keyId);
    final parsedGroupId = int.tryParse(groupId);
    if (parsedKeyId == null || parsedGroupId == null) {
      throw const ApiKeyRepositoryException('API_KEY_INVALID_GROUP');
    }
    try {
      await _client.request(
        'PUT',
        'keys/$parsedKeyId',
        data: {'group_id': parsedGroupId},
      );
    } on PanelApiException catch (error) {
      throw ApiKeyRepositoryException(error.publicCode);
    }
  }

  static ApiKeySummary? _parseKey(
    Map<String, dynamic> key, {
    required List<String> availableGroups,
    required Map<String, String> groupIdsByName,
    required Set<String> imageGroups,
  }) {
    final id = key['id'];
    final name = key['name'];
    final secret = key['key'];
    if (id is! num || name is! String || secret is! String || secret.isEmpty) {
      return null;
    }
    final rawGroup = key['group'];
    final group = rawGroup is Map && rawGroup['platform'] == 'openai'
        ? rawGroup['name'] as String?
        : null;
    final quotaUsed = key['quota_used'];
    final usage = quotaUsed is num ? quotaUsed.toDouble() : 0.0;
    return ApiKeySummary(
      id: id.toInt().toString(),
      name: name,
      secretKey: secret,
      maskedKey: _maskKey(secret),
      group: availableGroups.contains(group) ? group : null,
      availableGroups: List.unmodifiable(availableGroups),
      groupIdsByName: Map.unmodifiable(groupIdsByName),
      imageGroups: Set.unmodifiable(imageGroups),
      monthlyUsage: '\$${usage.toStringAsFixed(2)}',
      isSelected: false,
    );
  }

  static String _maskKey(String value) {
    if (value.length <= 8) {
      return '••••${value.substring(value.length > 4 ? value.length - 4 : 0)}';
    }
    final prefixLength = value.startsWith('sk-') ? 3 : 2;
    return '${value.substring(0, prefixLength)}••••${value.substring(value.length - 4)}';
  }

  static Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return value.map((key, item) => MapEntry(key.toString(), item));
    }
    throw const ApiKeyRepositoryException('API_KEY_INVALID_RESPONSE');
  }

  static List<dynamic> _asList(dynamic value) {
    if (value is List<dynamic>) {
      return value;
    }
    throw const ApiKeyRepositoryException('API_KEY_INVALID_RESPONSE');
  }
}
