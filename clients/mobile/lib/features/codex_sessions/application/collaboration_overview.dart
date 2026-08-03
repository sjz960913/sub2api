import 'package:flutter_riverpod/flutter_riverpod.dart';

class CodexSessionSummary {
  const CodexSessionSummary({
    required this.id,
    required this.title,
    required this.preview,
    required this.updatedAt,
  });

  final String id;
  final String title;
  final String preview;
  final String updatedAt;
}

class CollaborationOverviewState {
  const CollaborationOverviewState({
    required this.hasQueried,
    required this.sessions,
  });

  final bool hasQueried;
  final List<CodexSessionSummary> sessions;

  CollaborationOverviewState copyWith({bool? hasQueried}) {
    return CollaborationOverviewState(
      hasQueried: hasQueried ?? this.hasQueried,
      sessions: sessions,
    );
  }
}

final collaborationOverviewProvider =
    NotifierProvider<CollaborationOverview, CollaborationOverviewState>(
      CollaborationOverview.new,
    );

class CollaborationOverview extends Notifier<CollaborationOverviewState> {
  @override
  CollaborationOverviewState build() {
    // M6 replaces this preview list with the authoritative sync response.
    return const CollaborationOverviewState(
      hasQueried: false,
      sessions: [
        CodexSessionSummary(
          id: 'payment-callback',
          title: '修复支付回调',
          preview: '修复支付回调接口在部分订单状态下失败…',
          updatedAt: '今天 09:23',
        ),
        CodexSessionSummary(
          id: 'login-flow',
          title: '更新登录流程',
          preview: '优化登录流程，增加短信验证并处理异常…',
          updatedAt: '昨天 16:48',
        ),
        CodexSessionSummary(
          id: 'api-docs',
          title: '整理 API 文档',
          preview: '梳理用户模块接口，补充请求参数和返回示例…',
          updatedAt: '昨天 10:15',
        ),
      ],
    );
  }

  void revealSessions() => state = state.copyWith(hasQueried: true);
}
