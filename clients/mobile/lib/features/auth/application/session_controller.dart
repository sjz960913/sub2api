import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/network/panel_api_client.dart';
import '../domain/panel_session.dart';

enum SessionPhase { booting, signedOut, requiresTwoFactor, authenticated }

class SessionState {
  const SessionState({
    required this.phase,
    this.siteUrl,
    this.user,
    this.emailMasked,
    this.isBusy = false,
    this.errorCode,
  });

  const SessionState.booting() : this(phase: SessionPhase.booting);

  final SessionPhase phase;
  final String? siteUrl;
  final PanelUser? user;
  final String? emailMasked;
  final bool isBusy;
  final String? errorCode;

  bool get isAuthenticated =>
      phase == SessionPhase.authenticated && user != null;

  SessionState copyWith({
    bool? isBusy,
    String? errorCode,
    bool clearError = false,
  }) {
    return SessionState(
      phase: phase,
      siteUrl: siteUrl,
      user: user,
      emailMasked: emailMasked,
      isBusy: isBusy ?? this.isBusy,
      errorCode: clearError ? null : errorCode ?? this.errorCode,
    );
  }
}

final initialSessionStateProvider = Provider<SessionState>(
  (_) => const SessionState.booting(),
);

final sessionControllerProvider =
    NotifierProvider<SessionController, SessionState>(SessionController.new);

class SessionController extends Notifier<SessionState> {
  PanelApiClient get _client => ref.read(panelApiClientProvider);

  @override
  SessionState build() {
    final initial = ref.watch(initialSessionStateProvider);
    if (initial.phase == SessionPhase.booting) {
      Future<void>.microtask(restore);
    }
    return initial;
  }

  Future<void> restore() async {
    final restored = await _client.restore();
    if (restored.user != null) {
      state = SessionState(
        phase: SessionPhase.authenticated,
        siteUrl: restored.siteUrl,
        user: restored.user,
      );
    } else {
      state = SessionState(
        phase: SessionPhase.signedOut,
        siteUrl: restored.siteUrl,
      );
    }
  }

  Future<bool> login(String email, String password) async {
    state = state.copyWith(isBusy: true, clearError: true);
    try {
      final result = await _client.login(email: email, password: password);
      if (result.requiresTwoFactor) {
        state = SessionState(
          phase: SessionPhase.requiresTwoFactor,
          siteUrl: _client.siteUrl,
          emailMasked: result.emailMasked,
        );
      } else {
        state = SessionState(
          phase: SessionPhase.authenticated,
          siteUrl: _client.siteUrl,
          user: result.user,
        );
      }
      return true;
    } on PanelApiException catch (error) {
      state = state.copyWith(isBusy: false, errorCode: error.publicCode);
      return false;
    }
  }

  Future<bool> completeTwoFactor(String code) async {
    state = state.copyWith(isBusy: true, clearError: true);
    try {
      final user = await _client.completeTwoFactor(code);
      state = SessionState(
        phase: SessionPhase.authenticated,
        siteUrl: _client.siteUrl,
        user: user,
      );
      return true;
    } on PanelApiException catch (error) {
      state = state.copyWith(isBusy: false, errorCode: error.publicCode);
      return false;
    }
  }

  Future<void> logout() async {
    state = state.copyWith(isBusy: true, clearError: true);
    await _client.logout();
    state = SessionState(
      phase: SessionPhase.signedOut,
      siteUrl: _client.siteUrl,
    );
  }
}
