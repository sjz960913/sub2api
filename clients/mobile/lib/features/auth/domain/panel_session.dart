enum PanelRole { user, admin }

class PanelUser {
  const PanelUser({
    required this.id,
    required this.email,
    required this.username,
    required this.role,
  });

  final int id;
  final String email;
  final String username;
  final PanelRole role;
}

class PanelLoginResult {
  const PanelLoginResult.authenticated(this.user)
    : requiresTwoFactor = false,
      emailMasked = null;

  const PanelLoginResult.requiresTwoFactor(this.emailMasked)
    : requiresTwoFactor = true,
      user = null;

  final bool requiresTwoFactor;
  final PanelUser? user;
  final String? emailMasked;
}

class PanelRestoreResult {
  const PanelRestoreResult({required this.siteUrl, this.user});

  final String? siteUrl;
  final PanelUser? user;
}
