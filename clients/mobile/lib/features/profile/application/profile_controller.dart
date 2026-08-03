import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../auth/application/session_controller.dart';
import '../data/profile_repository.dart';
import '../domain/user_announcement.dart';

class ProfileState {
  const ProfileState({
    this.announcements = const [],
    this.isLoading = false,
    this.errorCode,
  });

  final List<UserAnnouncement> announcements;
  final bool isLoading;
  final String? errorCode;

  int get unreadCount => announcements.where((item) => item.isUnread).length;

  ProfileState copyWith({
    List<UserAnnouncement>? announcements,
    bool? isLoading,
    String? errorCode,
    bool clearError = false,
  }) {
    return ProfileState(
      announcements: announcements ?? this.announcements,
      isLoading: isLoading ?? this.isLoading,
      errorCode: clearError ? null : errorCode ?? this.errorCode,
    );
  }
}

final profileControllerProvider = NotifierProvider<ProfileController, ProfileState>(
  ProfileController.new,
);

class ProfileController extends Notifier<ProfileState> {
  @override
  ProfileState build() {
    final authenticated = ref.watch(
      sessionControllerProvider.select((session) => session.isAuthenticated),
    );
    if (authenticated) {
      Future<void>.microtask(loadAnnouncements);
    }
    return const ProfileState();
  }

  Future<void> loadAnnouncements({bool force = false}) async {
    if (state.isLoading || (!force && state.announcements.isNotEmpty)) {
      return;
    }
    state = state.copyWith(isLoading: true, clearError: true);
    try {
      final announcements = await ref.read(profileRepositoryProvider).listAnnouncements();
      state = ProfileState(announcements: announcements);
    } on ProfileRepositoryException catch (error) {
      state = state.copyWith(isLoading: false, errorCode: error.publicCode);
    }
  }

  Future<void> markRead(int id) async {
    final index = state.announcements.indexWhere((item) => item.id == id);
    if (index < 0 || !state.announcements[index].isUnread) {
      return;
    }
    final previous = state;
    state = state.copyWith(
      announcements: [
        for (final item in state.announcements)
          if (item.id == id) item.markRead(DateTime.now()) else item,
      ],
      clearError: true,
    );
    try {
      await ref.read(profileRepositoryProvider).markAnnouncementRead(id);
    } on ProfileRepositoryException catch (error) {
      state = previous.copyWith(errorCode: error.publicCode);
    }
  }

  Future<RedeemResult> redeem(String code) {
    return ref.read(profileRepositoryProvider).redeem(code);
  }
}
