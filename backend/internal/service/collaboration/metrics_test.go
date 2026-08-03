package collaboration

import (
	"testing"
	"time"

	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	"github.com/google/uuid"
)

func TestCollaborationMetricsSnapshotTracksBoundedOperationalState(t *testing.T) {
	metrics := newMetrics()
	service := &Service{metrics: metrics}
	deviceID := uuid.New()

	service.RecordWebSocketOpened("pc")
	service.RecordWebSocketOpened("mobile")
	service.RecordWebSocketClosed("mobile", "token_expired")
	metrics.observePresence(deviceID, collabdomain.DeviceStatusOnline)
	metrics.observePresence(deviceID, collabdomain.DeviceStatusOnline)
	metrics.observePresence(deviceID, collabdomain.DeviceStatusOffline)
	metrics.observeSyncRequested(collabdomain.SyncKindSessionList, false)
	metrics.observeSyncRequested(collabdomain.SyncKindThreadSnapshot, true)
	now := time.Now().UTC()
	metrics.observeSyncTerminal(SyncRequest{
		Status: collabdomain.SyncStatusCompleted, ResultCount: 3,
		CreatedAt: now, UpdatedAt: now.Add(250 * time.Millisecond),
	})
	metrics.observeChargeResult(CreateCommandResult{Command: Command{
		Status: collabdomain.CommandStatusAccepted, CreatedAt: now, UpdatedAt: now,
	}}, nil)
	metrics.observeCommandStatus(Command{
		Status:    collabdomain.CommandStatusCompleted,
		CreatedAt: now, UpdatedAt: now.Add(time.Second),
	})

	snapshot := service.SnapshotMetrics()
	if snapshot.WSConnectionsPCTotal != 1 || snapshot.WSConnectionsMobileTotal != 1 || snapshot.WSActivePC != 1 || snapshot.WSActiveMobile != 0 {
		t.Fatalf("WebSocket metrics = %#v", snapshot)
	}
	if snapshot.WSCloseTokenExpiredTotal != 1 || snapshot.PresenceOnlineTotal != 1 || snapshot.PresenceOfflineTotal != 1 || snapshot.OnlineDevices != 0 {
		t.Fatalf("connection/presence metrics = %#v", snapshot)
	}
	if snapshot.SessionSyncRequestedTotal != 1 || snapshot.ThreadSyncRequestedTotal != 1 || snapshot.SyncReplayTotal != 1 || snapshot.SyncCompletedTotal != 1 || snapshot.SyncResultItemsTotal != 3 || snapshot.SyncDurationTotalMillis != 250 {
		t.Fatalf("sync metrics = %#v", snapshot)
	}
	if snapshot.CommandAcceptedTotal != 1 || snapshot.CommandCompletedTotal != 1 || snapshot.ChargeCommittedTotal != 1 || snapshot.CommandDurationTotalMillis != 1000 {
		t.Fatalf("command/charge metrics = %#v", snapshot)
	}
}

func TestNilCollaborationServiceMetricsAreSafe(t *testing.T) {
	var service *Service
	service.RecordWebSocketOpened("pc")
	service.RecordWebSocketClosed("pc", "normal")
	if snapshot := service.SnapshotMetrics(); snapshot != (MetricsSnapshot{}) {
		t.Fatalf("nil service metrics = %#v", snapshot)
	}
}
