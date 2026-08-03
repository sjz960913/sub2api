package collaboration

import "testing"

func TestDeviceStatusTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		from DeviceStatus
		to   DeviceStatus
		want bool
	}{
		{name: "offline to online", from: DeviceStatusOffline, to: DeviceStatusOnline, want: true},
		{name: "online to degraded", from: DeviceStatusOnline, to: DeviceStatusDegraded, want: true},
		{name: "online to revoked", from: DeviceStatusOnline, to: DeviceStatusRevoked, want: true},
		{name: "revoked is terminal", from: DeviceStatusRevoked, to: DeviceStatusOnline, want: false},
		{name: "same status is not a transition", from: DeviceStatusOnline, to: DeviceStatusOnline, want: false},
		{name: "unknown status", from: DeviceStatus("unknown"), to: DeviceStatusOnline, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Fatalf("CanTransitionTo(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestSyncStatusTransitions(t *testing.T) {
	t.Parallel()

	allowed := [][2]SyncStatus{
		{SyncStatusPending, SyncStatusRunning},
		{SyncStatusPending, SyncStatusCompleted},
		{SyncStatusPending, SyncStatusFailed},
		{SyncStatusPending, SyncStatusExpired},
		{SyncStatusRunning, SyncStatusCompleted},
		{SyncStatusRunning, SyncStatusFailed},
		{SyncStatusRunning, SyncStatusExpired},
	}
	for _, transition := range allowed {
		if !transition[0].CanTransitionTo(transition[1]) {
			t.Errorf("expected sync transition %q -> %q to be allowed", transition[0], transition[1])
		}
	}

	if SyncStatusCompleted.CanTransitionTo(SyncStatusRunning) {
		t.Error("completed sync must be terminal")
	}
	if SyncStatusPending.CanTransitionTo(SyncStatusPending) {
		t.Error("same-state sync update must not be treated as a transition")
	}
}

func TestCommandStatusTransitions(t *testing.T) {
	t.Parallel()

	allowed := [][2]CommandStatus{
		{CommandStatusAccepted, CommandStatusDispatched},
		{CommandStatusAccepted, CommandStatusStarted},
		{CommandStatusAccepted, CommandStatusFailed},
		{CommandStatusAccepted, CommandStatusExpired},
		{CommandStatusDispatched, CommandStatusStarted},
		{CommandStatusDispatched, CommandStatusFailed},
		{CommandStatusDispatched, CommandStatusExpired},
		{CommandStatusStarted, CommandStatusCompleted},
		{CommandStatusStarted, CommandStatusFailed},
	}
	for _, transition := range allowed {
		if !transition[0].CanTransitionTo(transition[1]) {
			t.Errorf("expected command transition %q -> %q to be allowed", transition[0], transition[1])
		}
	}

	denied := [][2]CommandStatus{
		{CommandStatusAccepted, CommandStatusCompleted},
		{CommandStatusDispatched, CommandStatusCompleted},
		{CommandStatusStarted, CommandStatusDispatched},
		{CommandStatusCompleted, CommandStatusStarted},
		{CommandStatusFailed, CommandStatusAccepted},
		{CommandStatusExpired, CommandStatusAccepted},
	}
	for _, transition := range denied {
		if transition[0].CanTransitionTo(transition[1]) {
			t.Errorf("expected command transition %q -> %q to be denied", transition[0], transition[1])
		}
	}
}
