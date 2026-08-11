import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../features/auth/application/session_controller.dart';
import '../../features/auth/domain/panel_session.dart';

enum UserRole { user, admin }

final currentUserRoleProvider = Provider<UserRole>((ref) {
  final role = ref.watch(
    sessionControllerProvider.select((state) => state.user?.role),
  );
  return role == PanelRole.admin ? UserRole.admin : UserRole.user;
});
