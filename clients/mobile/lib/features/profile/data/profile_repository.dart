import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/panel_api_client.dart';
import '../domain/user_announcement.dart';

final profileRepositoryProvider = Provider<ProfileRepository>(
  (ref) => ProfileRepository(ref.watch(panelApiClientProvider)),
);

class ProfileRepositoryException implements Exception {
  const ProfileRepositoryException(this.publicCode);

  final String publicCode;
}

class RedeemResult {
  const RedeemResult({required this.message});

  final String message;
}

class ProfileRepository {
  const ProfileRepository(this._client);

  final PanelApiClient _client;

  Future<List<UserAnnouncement>> listAnnouncements() async {
    try {
      final data = await _client.request('GET', 'announcements');
      if (data is! List) {
        throw const ProfileRepositoryException('PROFILE_INVALID_RESPONSE');
      }
      return data.map(_parseAnnouncement).toList(growable: false);
    } on PanelApiException catch (error) {
      throw ProfileRepositoryException(error.publicCode);
    }
  }

  Future<void> markAnnouncementRead(int id) async {
    try {
      await _client.request('POST', 'announcements/$id/read');
    } on PanelApiException catch (error) {
      throw ProfileRepositoryException(error.publicCode);
    }
  }

  Future<RedeemResult> redeem(String code) async {
    final normalized = code.trim();
    if (normalized.isEmpty || normalized.length > 256) {
      throw const ProfileRepositoryException('REDEEM_INVALID_CODE');
    }
    try {
      final data = await _client.request('POST', 'redeem', data: {'code': normalized});
      final map = _asMap(data);
      final message = map['message'];
      return RedeemResult(message: message is String && message.isNotEmpty ? message : '兑换成功');
    } on PanelApiException catch (error) {
      throw ProfileRepositoryException(error.publicCode);
    }
  }

  static UserAnnouncement _parseAnnouncement(dynamic value) {
    final map = _asMap(value);
    final id = map['id'];
    final title = map['title'];
    final content = map['content'];
    if (id is! num || title is! String || content is! String) {
      throw const ProfileRepositoryException('PROFILE_INVALID_RESPONSE');
    }
    return UserAnnouncement(
      id: id.toInt(),
      title: title,
      content: content,
      notifyMode: map['notify_mode'] is String ? map['notify_mode'] as String : 'silent',
      createdAt: _parseDate(map['created_at']),
      readAt: _parseDate(map['read_at']),
    );
  }

  static DateTime? _parseDate(dynamic value) {
    return value is String ? DateTime.tryParse(value)?.toLocal() : null;
  }

  static Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return value.map((key, item) => MapEntry(key.toString(), item));
    }
    throw const ProfileRepositoryException('PROFILE_INVALID_RESPONSE');
  }
}
