import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/protocol/collaboration_wire.dart';
import '../../auth/application/session_controller.dart';
import '../data/collaboration_repository.dart';

class CodexSessionSummary {
  const CodexSessionSummary({
    required this.id,
    required this.title,
    required this.preview,
    required this.updatedAt,
    required this.writeState,
  });

  final String id;
  final String title;
  final String preview;
  final String updatedAt;
  final String writeState;
}

class CollaborationDeviceSummary {
  const CollaborationDeviceSummary({
    required this.id,
    required this.name,
    required this.platform,
    required this.status,
  });

  final String id;
  final String name;
  final String platform;
  final String status;

  bool get isOnline => status == 'online';
}

class CollaborationOverviewState {
  const CollaborationOverviewState({
    this.hasQueried = false,
    this.sessions = const [],
    this.devices = const [],
    this.selectedDeviceId,
    this.isLoadingDevices = false,
    this.isQuerying = false,
    this.errorCode,
  });

  final bool hasQueried;
  final List<CodexSessionSummary> sessions;
  final List<CollaborationDeviceSummary> devices;
  final String? selectedDeviceId;
  final bool isLoadingDevices;
  final bool isQuerying;
  final String? errorCode;

  CollaborationDeviceSummary? get selectedDevice {
    for (final device in devices) {
      if (device.id == selectedDeviceId) {
        return device;
      }
    }
    return null;
  }

  CollaborationOverviewState copyWith({
    bool? hasQueried,
    List<CodexSessionSummary>? sessions,
    List<CollaborationDeviceSummary>? devices,
    String? selectedDeviceId,
    bool? isLoadingDevices,
    bool? isQuerying,
    String? errorCode,
    bool clearError = false,
  }) {
    return CollaborationOverviewState(
      hasQueried: hasQueried ?? this.hasQueried,
      sessions: sessions ?? this.sessions,
      devices: devices ?? this.devices,
      selectedDeviceId: selectedDeviceId ?? this.selectedDeviceId,
      isLoadingDevices: isLoadingDevices ?? this.isLoadingDevices,
      isQuerying: isQuerying ?? this.isQuerying,
      errorCode: clearError ? null : errorCode ?? this.errorCode,
    );
  }
}

final collaborationOverviewSeedProvider = Provider<CollaborationOverviewState?>((_) => null);

final collaborationOverviewProvider =
    NotifierProvider<CollaborationOverview, CollaborationOverviewState>(
      CollaborationOverview.new,
    );

class CollaborationOverview extends Notifier<CollaborationOverviewState> {
  @override
  CollaborationOverviewState build() {
    final authenticated = ref.watch(
      sessionControllerProvider.select((session) => session.isAuthenticated),
    );
    if (!authenticated) {
      return const CollaborationOverviewState();
    }
    final seed = ref.watch(collaborationOverviewSeedProvider);
    if (seed != null) {
      return seed;
    }
    Future<void>.microtask(loadDevices);
    return const CollaborationOverviewState(isLoadingDevices: true);
  }

  Future<void> loadDevices() async {
    state = state.copyWith(isLoadingDevices: true, clearError: true);
    try {
      final devices = await ref.read(collaborationRepositoryProvider).listDevices();
      final summaries = devices.map(_deviceSummary).toList(growable: false);
      final selected = summaries.where((device) => device.isOnline).firstOrNull ??
          summaries.firstOrNull;
      state = CollaborationOverviewState(
        devices: summaries,
        selectedDeviceId: selected?.id,
      );
    } on CollaborationRepositoryException catch (error) {
      state = state.copyWith(isLoadingDevices: false, errorCode: error.publicCode);
    }
  }

  void selectDevice(String deviceId) {
    if (!state.devices.any((device) => device.id == deviceId)) {
      return;
    }
    state = CollaborationOverviewState(
      devices: state.devices,
      selectedDeviceId: deviceId,
    );
  }

  Future<bool> querySessions({String? searchTerm}) async {
    final device = state.selectedDevice;
    if (device == null || !device.isOnline || state.isQuerying) {
      return false;
    }
    state = state.copyWith(isQuerying: true, clearError: true);
    try {
      final sync = await ref.read(collaborationRepositoryProvider).syncSessions(
        deviceId: device.id,
        searchTerm: searchTerm,
      );
      state = state.copyWith(
        hasQueried: true,
        sessions: sync.items.map(_sessionSummary).toList(growable: false),
        isQuerying: false,
      );
      return true;
    } on CollaborationRepositoryException catch (error) {
      state = state.copyWith(isQuerying: false, errorCode: error.publicCode);
      return false;
    }
  }

  static CollaborationDeviceSummary _deviceSummary(Device device) {
    return CollaborationDeviceSummary(
      id: device.id,
      name: device.name,
      platform: device.platform,
      status: device.status,
    );
  }

  static CodexSessionSummary _sessionSummary(CodexSession session) {
    final time = DateTime.tryParse(session.updatedAt)?.toLocal();
    final updatedAt = time == null
        ? session.updatedAt
        : '${time.month}月${time.day}日 '
              '${time.hour.toString().padLeft(2, '0')}:'
              '${time.minute.toString().padLeft(2, '0')}';
    return CodexSessionSummary(
      id: session.threadId,
      title: session.title,
      preview: session.preview ?? '',
      updatedAt: updatedAt,
      writeState: session.writeState,
    );
  }
}

extension<T> on Iterable<T> {
  T? get firstOrNull {
    final iterator = this.iterator;
    return iterator.moveNext() ? iterator.current : null;
  }
}
