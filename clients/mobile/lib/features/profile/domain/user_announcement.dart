class UserAnnouncement {
  const UserAnnouncement({
    required this.id,
    required this.title,
    required this.content,
    required this.notifyMode,
    required this.createdAt,
    required this.readAt,
  });

  final int id;
  final String title;
  final String content;
  final String notifyMode;
  final DateTime? createdAt;
  final DateTime? readAt;

  bool get isUnread => readAt == null;

  UserAnnouncement markRead(DateTime time) {
    return UserAnnouncement(
      id: id,
      title: title,
      content: content,
      notifyMode: notifyMode,
      createdAt: createdAt,
      readAt: time,
    );
  }
}
