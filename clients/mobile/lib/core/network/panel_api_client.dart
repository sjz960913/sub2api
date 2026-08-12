import 'dart:async';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../features/auth/domain/panel_session.dart';
import '../storage/secure_value_store.dart';

const _siteStorageKey = 'sub2api.mobile.site';
const _refreshTokenStorageKey = 'sub2api.mobile.refresh_token';
const _maxTokenLength = 8192;
const fixedPanelSiteUrl = 'https://codecodelove.top/';

final secureValueStoreProvider = Provider<SecureValueStore>(
  (_) => const PlatformSecureValueStore(),
);

final panelApiClientProvider = Provider<PanelApiClient>(
  (ref) => PanelApiClient(store: ref.watch(secureValueStoreProvider)),
);

class PanelApiException implements Exception {
  const PanelApiException(this.publicCode);

  final String publicCode;

  @override
  String toString() => publicCode;
}

class PanelApiClient {
  PanelApiClient({required SecureValueStore store, Dio? dio})
    : _store = store,
      _dio =
          dio ??
          Dio(
            BaseOptions(
              connectTimeout: const Duration(seconds: 8),
              sendTimeout: const Duration(seconds: 15),
              receiveTimeout: const Duration(seconds: 30),
              responseType: ResponseType.json,
              followRedirects: false,
              maxRedirects: 0,
              headers: const {'Content-Type': 'application/json'},
              validateStatus: (status) =>
                  status != null && status >= 200 && status < 600,
            ),
          );

  final SecureValueStore _store;
  final Dio _dio;
  String? _siteUrl = fixedPanelSiteUrl;
  String? _accessToken;
  String? _pendingTwoFactorToken;
  Future<void>? _refreshOperation;

  String? get siteUrl => _siteUrl;

  static String normalizeSiteUrl(String raw) {
    final value = raw.trim();
    final uri = Uri.tryParse(value);
    if (uri == null ||
        !uri.hasScheme ||
        uri.host.isEmpty ||
        uri.userInfo.isNotEmpty ||
        uri.hasFragment ||
        uri.hasQuery) {
      throw const PanelApiException('PANEL_INVALID_SITE');
    }
    final loopback =
        uri.host.toLowerCase() == 'localhost' ||
        uri.host == '127.0.0.1' ||
        uri.host == '::1';
    if (uri.scheme != 'https' && !(uri.scheme == 'http' && loopback)) {
      throw const PanelApiException('PANEL_INSECURE_SITE');
    }
    final normalizedPath = uri.path.endsWith('/') ? uri.path : '${uri.path}/';
    return uri.replace(path: normalizedPath).toString();
  }

  static Map<String, String> stringifyQueryParameters(
    Map<String, dynamic> query,
  ) {
    return query.map((key, value) => MapEntry(key, value.toString()));
  }

  Future<PanelRestoreResult> restore() async {
    _siteUrl = fixedPanelSiteUrl;
    await _deleteSecureSafely(_siteStorageKey);
    try {
      final refreshToken = await _readSecure(_refreshTokenStorageKey);
      if (refreshToken == null || refreshToken.isEmpty) {
        return PanelRestoreResult(siteUrl: _siteUrl);
      }
      await _refreshTokens(refreshToken);
      final profile = await request('GET', 'auth/me');
      return PanelRestoreResult(siteUrl: _siteUrl, user: _parseUser(profile));
    } catch (_) {
      _accessToken = null;
      _pendingTwoFactorToken = null;
      await _deleteSecureSafely(_refreshTokenStorageKey);
      return PanelRestoreResult(siteUrl: _siteUrl);
    }
  }

  Future<PanelLoginResult> login({
    required String email,
    required String password,
    String turnstileToken = '',
  }) async {
    if (email.trim().isEmpty || !email.contains('@')) {
      throw const PanelApiException('PANEL_INVALID_EMAIL');
    }
    if (password.isEmpty) {
      throw const PanelApiException('PANEL_INVALID_PASSWORD');
    }
    final data = await request(
      'POST',
      'auth/login',
      authenticated: false,
      data: {
        'email': email.trim().toLowerCase(),
        'password': password,
        'turnstile_token': turnstileToken,
      },
    );
    final map = _asMap(data);
    if (map['requires_2fa'] == true) {
      final token = map['temp_token'];
      if (token is! String || !_validToken(token)) {
        throw const PanelApiException('PANEL_INVALID_RESPONSE');
      }
      _pendingTwoFactorToken = token;
      return PanelLoginResult.requiresTwoFactor(
        map['user_email_masked'] as String?,
      );
    }
    return PanelLoginResult.authenticated(await _establishSession(map));
  }

  Future<PanelUser> completeTwoFactor(String code) async {
    final token = _pendingTwoFactorToken;
    if (token == null) {
      throw const PanelApiException('PANEL_NO_PENDING_TWO_FACTOR');
    }
    final normalizedCode = code.trim();
    if (normalizedCode.length != 6 || int.tryParse(normalizedCode) == null) {
      throw const PanelApiException('PANEL_INVALID_TWO_FACTOR_CODE');
    }
    final data = await request(
      'POST',
      'auth/login/2fa',
      authenticated: false,
      data: {'temp_token': token, 'totp_code': normalizedCode},
    );
    final user = await _establishSession(_asMap(data));
    _pendingTwoFactorToken = null;
    return user;
  }

  Future<void> logout() async {
    String? refreshToken;
    try {
      refreshToken = await _readSecure(_refreshTokenStorageKey);
    } on PanelApiException {
      refreshToken = null;
    }
    if (refreshToken != null && refreshToken.isNotEmpty && _siteUrl != null) {
      try {
        await request(
          'POST',
          'auth/logout',
          authenticated: false,
          data: {'refresh_token': refreshToken},
        );
      } catch (_) {
        // Local logout must succeed even when the site is unavailable.
      }
    }
    _accessToken = null;
    _pendingTwoFactorToken = null;
    await _deleteSecureSafely(_refreshTokenStorageKey);
  }

  Future<dynamic> request(
    String method,
    String path, {
    Object? data,
    Map<String, dynamic>? query,
    Map<String, String>? headers,
    bool authenticated = true,
    bool retryAfterRefresh = true,
  }) async {
    final site = _siteUrl;
    if (site == null) {
      throw const PanelApiException('PANEL_SITE_NOT_CONFIGURED');
    }
    if (authenticated && _accessToken == null) {
      throw const PanelApiException('PANEL_SESSION_NOT_FOUND');
    }
    var endpoint = Uri.parse(
      site,
    ).resolve('api/v1/${path.replaceFirst(RegExp(r'^/+'), '')}');
    if (query != null && query.isNotEmpty) {
      endpoint = endpoint.replace(
        queryParameters: stringifyQueryParameters(query),
      );
    }
    Response<dynamic> response;
    try {
      response = await _dio.requestUri<dynamic>(
        endpoint,
        data: data,
        options: Options(
          method: method,
          headers: {
            'Accept-Language': 'zh-CN',
            'X-Sub2API-Client-Type': 'mobile',
            if (authenticated) 'Authorization': 'Bearer $_accessToken',
            ...?headers,
          },
        ),
      );
    } on DioException {
      throw const PanelApiException('PANEL_NETWORK_ERROR');
    }
    if (response.statusCode == 401 && authenticated && retryAfterRefresh) {
      await refreshAccessToken();
      return request(
        method,
        path,
        data: data,
        query: query,
        headers: headers,
        authenticated: authenticated,
        retryAfterRefresh: false,
      );
    }
    if (response.statusCode == 401) {
      throw const PanelApiException('PANEL_UNAUTHORIZED');
    }
    if (response.statusCode == 403) {
      throw const PanelApiException('PANEL_FORBIDDEN');
    }
    if (response.statusCode == 429) {
      throw const PanelApiException('PANEL_RATE_LIMITED');
    }
    if ((response.statusCode ?? 500) >= 400) {
      throw PanelApiException(_responseReason(response.data));
    }
    final envelope = _asMap(response.data);
    if (envelope['code'] != 0 || !envelope.containsKey('data')) {
      throw const PanelApiException('PANEL_REQUEST_FAILED');
    }
    return envelope['data'];
  }

  static String _responseReason(dynamic data) {
    if (data is Map) {
      final reason = data['reason'];
      if (reason is String &&
          reason.isNotEmpty &&
          reason.length <= 96 &&
          RegExp(r'^[A-Z][A-Z0-9_]*$').hasMatch(reason)) {
        return reason;
      }
    }
    return 'PANEL_REQUEST_FAILED';
  }

  Future<dynamic> gatewayRequest(
    String method,
    String path, {
    required String apiKey,
    Object? data,
  }) async {
    final site = _siteUrl;
    if (site == null) {
      throw const PanelApiException('PANEL_SITE_NOT_CONFIGURED');
    }
    if (!_validToken(apiKey)) {
      throw const PanelApiException('GATEWAY_INVALID_API_KEY');
    }
    final siteUri = Uri.parse(site);
    final endpoint = siteUri.replace(
      path: '/${path.replaceFirst(RegExp(r'^/+'), '')}',
      query: null,
      fragment: null,
    );
    Response<dynamic> response;
    try {
      response = await _dio.requestUri<dynamic>(
        endpoint,
        data: data,
        options: Options(
          method: method,
          headers: {
            'Accept-Language': 'zh-CN',
            'Authorization': 'Bearer $apiKey',
            'X-Sub2API-Client-Type': 'mobile',
          },
        ),
      );
    } on DioException {
      throw const PanelApiException('GATEWAY_NETWORK_ERROR');
    }
    if (response.statusCode == 401) {
      throw const PanelApiException('GATEWAY_UNAUTHORIZED');
    }
    if (response.statusCode == 429) {
      throw const PanelApiException('GATEWAY_RATE_LIMITED');
    }
    if ((response.statusCode ?? 500) >= 400) {
      throw const PanelApiException('GATEWAY_REQUEST_FAILED');
    }
    return response.data;
  }

  Future<Stream<List<int>>> gatewayStream(
    String method,
    String path, {
    required String apiKey,
    Object? data,
  }) async {
    final site = _siteUrl;
    if (site == null) {
      throw const PanelApiException('PANEL_SITE_NOT_CONFIGURED');
    }
    if (!_validToken(apiKey)) {
      throw const PanelApiException('GATEWAY_INVALID_API_KEY');
    }
    final siteUri = Uri.parse(site);
    final endpoint = siteUri.replace(
      path: '/${path.replaceFirst(RegExp(r'^/+'), '')}',
      query: null,
      fragment: null,
    );
    Response<ResponseBody> response;
    try {
      response = await _dio.requestUri<ResponseBody>(
        endpoint,
        data: data,
        options: Options(
          method: method,
          responseType: ResponseType.stream,
          headers: {
            'Accept': 'text/event-stream',
            'Accept-Language': 'zh-CN',
            'Authorization': 'Bearer $apiKey',
            'X-Sub2API-Client-Type': 'mobile',
          },
        ),
      );
    } on DioException {
      throw const PanelApiException('GATEWAY_NETWORK_ERROR');
    }
    if (response.statusCode == 401) {
      throw const PanelApiException('GATEWAY_UNAUTHORIZED');
    }
    if (response.statusCode == 429) {
      throw const PanelApiException('GATEWAY_RATE_LIMITED');
    }
    if ((response.statusCode ?? 500) >= 400) {
      throw const PanelApiException('GATEWAY_REQUEST_FAILED');
    }
    final body = response.data;
    if (body == null) {
      throw const PanelApiException('GATEWAY_INVALID_RESPONSE');
    }
    return body.stream.map<List<int>>((chunk) => chunk);
  }

  Future<void> refreshAccessToken() {
    final active = _refreshOperation;
    if (active != null) {
      return active;
    }
    final operation = _performStoredRefresh();
    _refreshOperation = operation;
    return operation.whenComplete(() {
      if (identical(_refreshOperation, operation)) {
        _refreshOperation = null;
      }
    });
  }

  Future<void> _performStoredRefresh() async {
    final refreshToken = await _readSecure(_refreshTokenStorageKey);
    if (refreshToken == null || refreshToken.isEmpty) {
      throw const PanelApiException('PANEL_SESSION_NOT_FOUND');
    }
    try {
      await _refreshTokens(refreshToken);
    } catch (_) {
      _accessToken = null;
      await _deleteSecureSafely(_refreshTokenStorageKey);
      rethrow;
    }
  }

  Future<void> _refreshTokens(String refreshToken) async {
    if (!_validToken(refreshToken)) {
      throw const PanelApiException('PANEL_INVALID_RESPONSE');
    }
    final data = await request(
      'POST',
      'auth/refresh',
      authenticated: false,
      data: {'refresh_token': refreshToken},
    );
    final map = _asMap(data);
    final accessToken = map['access_token'];
    final rotatedRefreshToken = map['refresh_token'];
    if (accessToken is! String ||
        rotatedRefreshToken is! String ||
        !_validToken(accessToken) ||
        !_validToken(rotatedRefreshToken)) {
      throw const PanelApiException('PANEL_INVALID_RESPONSE');
    }
    await _writeSecure(_refreshTokenStorageKey, rotatedRefreshToken);
    _accessToken = accessToken;
  }

  Future<PanelUser> _establishSession(Map<String, dynamic> map) async {
    final accessToken = map['access_token'];
    final refreshToken = map['refresh_token'];
    if (accessToken is! String ||
        refreshToken is! String ||
        !_validToken(accessToken) ||
        !_validToken(refreshToken)) {
      throw const PanelApiException('PANEL_INVALID_RESPONSE');
    }
    final user = _parseUser(map['user']);
    await _writeSecure(_refreshTokenStorageKey, refreshToken);
    _accessToken = accessToken;
    return user;
  }

  static PanelUser _parseUser(dynamic value) {
    final map = _asMap(value);
    final id = map['id'];
    final email = map['email'];
    final username = map['username'];
    final role = map['role'];
    final balance = _asDouble(map['balance']);
    if (id is! num || email is! String || role is! String) {
      throw const PanelApiException('PANEL_INVALID_RESPONSE');
    }
    return PanelUser(
      id: id.toInt(),
      email: email,
      username: username is String ? username : email.split('@').first,
      role: role == 'admin' ? PanelRole.admin : PanelRole.user,
      balance: balance,
    );
  }

  static double _asDouble(dynamic value) {
    if (value is num) {
      return value.toDouble();
    }
    if (value is String) {
      return double.tryParse(value) ?? 0;
    }
    return 0;
  }

  static Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return value.map((key, item) => MapEntry(key.toString(), item));
    }
    throw const PanelApiException('PANEL_INVALID_RESPONSE');
  }

  static bool _validToken(String value) {
    return value.isNotEmpty &&
        value.length <= _maxTokenLength &&
        !value.contains(RegExp(r'[\x00-\x1F\x7F]'));
  }

  Future<String?> _readSecure(String key) async {
    try {
      return await _store.read(key);
    } catch (_) {
      throw const PanelApiException('PANEL_SECURE_STORE_ERROR');
    }
  }

  Future<void> _writeSecure(String key, String value) async {
    try {
      await _store.write(key, value);
    } catch (_) {
      throw const PanelApiException('PANEL_SECURE_STORE_ERROR');
    }
  }

  Future<void> _deleteSecureSafely(String key) async {
    try {
      await _store.delete(key);
    } catch (_) {
      // Best effort cleanup; in-memory credentials are always cleared first.
    }
  }
}
