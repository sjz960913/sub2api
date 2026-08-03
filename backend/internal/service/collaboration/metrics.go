package collaboration

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	"github.com/google/uuid"
)

// MetricsSnapshot is a secret-free, process-local collaboration telemetry
// snapshot. Fixed fields avoid unbounded labels such as raw error or thread IDs.
type MetricsSnapshot struct {
	WSConnectionsPCTotal       uint64 `json:"ws_connections_pc_total"`
	WSConnectionsMobileTotal   uint64 `json:"ws_connections_mobile_total"`
	WSActivePC                 int64  `json:"ws_active_pc"`
	WSActiveMobile             int64  `json:"ws_active_mobile"`
	WSCloseNormalTotal         uint64 `json:"ws_close_normal_total"`
	WSCloseTokenExpiredTotal   uint64 `json:"ws_close_token_expired_total"`
	WSCloseDeviceRevokedTotal  uint64 `json:"ws_close_device_revoked_total"`
	WSCloseRelayOverflowTotal  uint64 `json:"ws_close_relay_overflow_total"`
	WSCloseWriteErrorTotal     uint64 `json:"ws_close_write_error_total"`
	OnlineDevices              int64  `json:"online_devices"`
	PresenceOnlineTotal        uint64 `json:"presence_online_total"`
	PresenceDegradedTotal      uint64 `json:"presence_degraded_total"`
	PresenceOfflineTotal       uint64 `json:"presence_offline_total"`
	SessionSyncRequestedTotal  uint64 `json:"session_sync_requested_total"`
	ThreadSyncRequestedTotal   uint64 `json:"thread_sync_requested_total"`
	SyncReplayTotal            uint64 `json:"sync_replay_total"`
	SyncCompletedTotal         uint64 `json:"sync_completed_total"`
	SyncFailedTotal            uint64 `json:"sync_failed_total"`
	SyncResultItemsTotal       uint64 `json:"sync_result_items_total"`
	SyncDurationCount          uint64 `json:"sync_duration_count"`
	SyncDurationTotalMillis    uint64 `json:"sync_duration_total_millis"`
	CommandAcceptedTotal       uint64 `json:"command_accepted_total"`
	CommandReplayTotal         uint64 `json:"command_replay_total"`
	CommandDispatchedTotal     uint64 `json:"command_dispatched_total"`
	CommandReceivedTotal       uint64 `json:"command_received_total"`
	CommandStartedTotal        uint64 `json:"command_started_total"`
	CommandItemTotal           uint64 `json:"command_item_total"`
	CommandCompletedTotal      uint64 `json:"command_completed_total"`
	CommandFailedTotal         uint64 `json:"command_failed_total"`
	CommandRateLimitedTotal    uint64 `json:"command_rate_limited_total"`
	CommandDurationCount       uint64 `json:"command_duration_count"`
	CommandDurationTotalMillis uint64 `json:"command_duration_total_millis"`
	ChargeCommittedTotal       uint64 `json:"charge_committed_total"`
	ChargeReplayTotal          uint64 `json:"charge_replay_total"`
	ChargeRejectedTotal        uint64 `json:"charge_rejected_total"`
	ChargeCommandMismatchTotal uint64 `json:"charge_command_mismatch_total"`
	RelayPublishFailureTotal   uint64 `json:"relay_publish_failure_total"`
}

// Metrics stores bounded counters exposed through the service snapshot.
type Metrics struct {
	wsConnectionsPC       atomic.Uint64
	wsConnectionsMobile   atomic.Uint64
	wsActivePC            atomic.Int64
	wsActiveMobile        atomic.Int64
	wsCloseNormal         atomic.Uint64
	wsCloseTokenExpired   atomic.Uint64
	wsCloseDeviceRevoked  atomic.Uint64
	wsCloseRelayOverflow  atomic.Uint64
	wsCloseWriteError     atomic.Uint64
	onlineDevices         atomic.Int64
	presenceOnline        atomic.Uint64
	presenceDegraded      atomic.Uint64
	presenceOffline       atomic.Uint64
	sessionSyncRequested  atomic.Uint64
	threadSyncRequested   atomic.Uint64
	syncReplay            atomic.Uint64
	syncCompleted         atomic.Uint64
	syncFailed            atomic.Uint64
	syncResultItems       atomic.Uint64
	syncDurationCount     atomic.Uint64
	syncDurationMillis    atomic.Uint64
	commandAccepted       atomic.Uint64
	commandReplay         atomic.Uint64
	commandDispatched     atomic.Uint64
	commandReceived       atomic.Uint64
	commandStarted        atomic.Uint64
	commandItem           atomic.Uint64
	commandCompleted      atomic.Uint64
	commandFailed         atomic.Uint64
	commandRateLimited    atomic.Uint64
	commandDurationCount  atomic.Uint64
	commandDurationMillis atomic.Uint64
	chargeCommitted       atomic.Uint64
	chargeReplay          atomic.Uint64
	chargeRejected        atomic.Uint64
	chargeCommandMismatch atomic.Uint64
	relayPublishFailure   atomic.Uint64

	presenceMu sync.Mutex
	presence   map[uuid.UUID]collabdomain.DeviceStatus
}

func newMetrics() *Metrics {
	return &Metrics{presence: make(map[uuid.UUID]collabdomain.DeviceStatus)}
}

func (s *Service) SnapshotMetrics() MetricsSnapshot {
	if s == nil || s.metrics == nil {
		return MetricsSnapshot{}
	}
	return s.metrics.snapshot()
}

func (s *Service) RecordWebSocketOpened(clientType string) {
	if s == nil || s.metrics == nil {
		return
	}
	if clientType == "pc" {
		s.metrics.wsConnectionsPC.Add(1)
		s.metrics.wsActivePC.Add(1)
		return
	}
	s.metrics.wsConnectionsMobile.Add(1)
	s.metrics.wsActiveMobile.Add(1)
}

func (s *Service) RecordWebSocketClosed(clientType, reason string) {
	if s == nil || s.metrics == nil {
		return
	}
	if clientType == "pc" {
		decrementNonnegative(&s.metrics.wsActivePC)
	} else {
		decrementNonnegative(&s.metrics.wsActiveMobile)
	}
	switch reason {
	case "token_expired":
		s.metrics.wsCloseTokenExpired.Add(1)
	case "device_revoked":
		s.metrics.wsCloseDeviceRevoked.Add(1)
	case "relay_overflow":
		s.metrics.wsCloseRelayOverflow.Add(1)
	case "write_error":
		s.metrics.wsCloseWriteError.Add(1)
	default:
		s.metrics.wsCloseNormal.Add(1)
	}
}

func (m *Metrics) observePresence(deviceID uuid.UUID, status collabdomain.DeviceStatus) {
	if m == nil || deviceID == uuid.Nil {
		return
	}
	m.presenceMu.Lock()
	defer m.presenceMu.Unlock()
	previous, exists := m.presence[deviceID]
	if exists && previous == status {
		return
	}
	if previous == collabdomain.DeviceStatusOnline {
		decrementNonnegative(&m.onlineDevices)
	}
	switch status {
	case collabdomain.DeviceStatusOnline:
		m.onlineDevices.Add(1)
		m.presenceOnline.Add(1)
	case collabdomain.DeviceStatusDegraded:
		m.presenceDegraded.Add(1)
	default:
		m.presenceOffline.Add(1)
	}
	if status == collabdomain.DeviceStatusOffline || status == collabdomain.DeviceStatusRevoked {
		delete(m.presence, deviceID)
		return
	}
	m.presence[deviceID] = status
}

func (m *Metrics) observeSyncRequested(kind collabdomain.SyncKind, replayed bool) {
	if m == nil {
		return
	}
	if kind == collabdomain.SyncKindSessionList {
		m.sessionSyncRequested.Add(1)
	} else {
		m.threadSyncRequested.Add(1)
	}
	if replayed {
		m.syncReplay.Add(1)
	}
}

func (m *Metrics) observeSyncTerminal(syncRequest SyncRequest) {
	if m == nil {
		return
	}
	switch syncRequest.Status {
	case collabdomain.SyncStatusCompleted:
		m.syncCompleted.Add(1)
		if syncRequest.ResultCount > 0 {
			m.syncResultItems.Add(uint64(syncRequest.ResultCount))
		}
	case collabdomain.SyncStatusFailed, collabdomain.SyncStatusExpired:
		m.syncFailed.Add(1)
	default:
		return
	}
	m.syncDurationCount.Add(1)
	m.syncDurationMillis.Add(durationMillis(syncRequest.UpdatedAt.Sub(syncRequest.CreatedAt)))
}

func (m *Metrics) observeCommandStatus(command Command) {
	if m == nil {
		return
	}
	switch command.Status {
	case collabdomain.CommandStatusAccepted:
		m.commandAccepted.Add(1)
	case collabdomain.CommandStatusDispatched:
		m.commandDispatched.Add(1)
	case collabdomain.CommandStatusStarted:
		m.commandStarted.Add(1)
	case collabdomain.CommandStatusCompleted:
		m.commandCompleted.Add(1)
	case collabdomain.CommandStatusFailed, collabdomain.CommandStatusExpired:
		m.commandFailed.Add(1)
	default:
		return
	}
	if command.Status.Terminal() {
		m.commandDurationCount.Add(1)
		m.commandDurationMillis.Add(durationMillis(command.UpdatedAt.Sub(command.CreatedAt)))
	}
}

func (m *Metrics) observeChargeResult(result CreateCommandResult, err error) {
	if m == nil {
		return
	}
	if err != nil {
		var insufficient *InsufficientBalanceError
		if errors.As(err, &insufficient) {
			m.chargeRejected.Add(1)
		}
		if errors.Is(err, ErrInvariantViolation) {
			m.chargeCommandMismatch.Add(1)
		}
		return
	}
	if result.Replayed {
		m.commandReplay.Add(1)
		m.chargeReplay.Add(1)
		return
	}
	m.chargeCommitted.Add(1)
	m.observeCommandStatus(result.Command)
}

func (m *Metrics) snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		WSConnectionsPCTotal:       m.wsConnectionsPC.Load(),
		WSConnectionsMobileTotal:   m.wsConnectionsMobile.Load(),
		WSActivePC:                 m.wsActivePC.Load(),
		WSActiveMobile:             m.wsActiveMobile.Load(),
		WSCloseNormalTotal:         m.wsCloseNormal.Load(),
		WSCloseTokenExpiredTotal:   m.wsCloseTokenExpired.Load(),
		WSCloseDeviceRevokedTotal:  m.wsCloseDeviceRevoked.Load(),
		WSCloseRelayOverflowTotal:  m.wsCloseRelayOverflow.Load(),
		WSCloseWriteErrorTotal:     m.wsCloseWriteError.Load(),
		OnlineDevices:              m.onlineDevices.Load(),
		PresenceOnlineTotal:        m.presenceOnline.Load(),
		PresenceDegradedTotal:      m.presenceDegraded.Load(),
		PresenceOfflineTotal:       m.presenceOffline.Load(),
		SessionSyncRequestedTotal:  m.sessionSyncRequested.Load(),
		ThreadSyncRequestedTotal:   m.threadSyncRequested.Load(),
		SyncReplayTotal:            m.syncReplay.Load(),
		SyncCompletedTotal:         m.syncCompleted.Load(),
		SyncFailedTotal:            m.syncFailed.Load(),
		SyncResultItemsTotal:       m.syncResultItems.Load(),
		SyncDurationCount:          m.syncDurationCount.Load(),
		SyncDurationTotalMillis:    m.syncDurationMillis.Load(),
		CommandAcceptedTotal:       m.commandAccepted.Load(),
		CommandReplayTotal:         m.commandReplay.Load(),
		CommandDispatchedTotal:     m.commandDispatched.Load(),
		CommandReceivedTotal:       m.commandReceived.Load(),
		CommandStartedTotal:        m.commandStarted.Load(),
		CommandItemTotal:           m.commandItem.Load(),
		CommandCompletedTotal:      m.commandCompleted.Load(),
		CommandFailedTotal:         m.commandFailed.Load(),
		CommandRateLimitedTotal:    m.commandRateLimited.Load(),
		CommandDurationCount:       m.commandDurationCount.Load(),
		CommandDurationTotalMillis: m.commandDurationMillis.Load(),
		ChargeCommittedTotal:       m.chargeCommitted.Load(),
		ChargeReplayTotal:          m.chargeReplay.Load(),
		ChargeRejectedTotal:        m.chargeRejected.Load(),
		ChargeCommandMismatchTotal: m.chargeCommandMismatch.Load(),
		RelayPublishFailureTotal:   m.relayPublishFailure.Load(),
	}
}

func decrementNonnegative(counter *atomic.Int64) {
	for {
		current := counter.Load()
		if current <= 0 || counter.CompareAndSwap(current, current-1) {
			return
		}
	}
}

func durationMillis(duration time.Duration) uint64 {
	if duration <= 0 {
		return 0
	}
	return uint64(duration.Milliseconds())
}
