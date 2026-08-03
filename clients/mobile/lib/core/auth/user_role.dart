import 'package:flutter_riverpod/flutter_riverpod.dart';

enum UserRole { user, admin }

// M4 replaces this default with the role returned by /api/v1/auth/me. Keeping
// the secure default as user makes deep-link guards testable from M0 onward.
final currentUserRoleProvider = Provider<UserRole>((_) => UserRole.user);
