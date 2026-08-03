// Package collaboration defines the authoritative collaboration state machines.
package collaboration

import "slices"

type DeviceStatus string

const (
	DeviceStatusOffline  DeviceStatus = "offline"
	DeviceStatusOnline   DeviceStatus = "online"
	DeviceStatusDegraded DeviceStatus = "degraded"
	DeviceStatusRevoked  DeviceStatus = "revoked"
)

func (s DeviceStatus) Valid() bool {
	return slices.Contains([]DeviceStatus{
		DeviceStatusOffline,
		DeviceStatusOnline,
		DeviceStatusDegraded,
		DeviceStatusRevoked,
	}, s)
}

func (s DeviceStatus) CanTransitionTo(next DeviceStatus) bool {
	if !s.Valid() || !next.Valid() || s == next {
		return false
	}
	if s == DeviceStatusRevoked {
		return false
	}
	return true
}

type SyncKind string

const (
	SyncKindSessionList    SyncKind = "session_list"
	SyncKindThreadSnapshot SyncKind = "thread_snapshot"
)

func (k SyncKind) Valid() bool {
	return k == SyncKindSessionList || k == SyncKindThreadSnapshot
}

type SyncStatus string

const (
	SyncStatusPending   SyncStatus = "pending"
	SyncStatusRunning   SyncStatus = "running"
	SyncStatusCompleted SyncStatus = "completed"
	SyncStatusFailed    SyncStatus = "failed"
	SyncStatusExpired   SyncStatus = "expired"
)

func (s SyncStatus) Valid() bool {
	return slices.Contains([]SyncStatus{
		SyncStatusPending,
		SyncStatusRunning,
		SyncStatusCompleted,
		SyncStatusFailed,
		SyncStatusExpired,
	}, s)
}

func (s SyncStatus) Terminal() bool {
	return s == SyncStatusCompleted || s == SyncStatusFailed || s == SyncStatusExpired
}

func (s SyncStatus) CanTransitionTo(next SyncStatus) bool {
	if !s.Valid() || !next.Valid() || s == next || s.Terminal() {
		return false
	}
	switch s {
	case SyncStatusPending:
		return next == SyncStatusRunning || next.Terminal()
	case SyncStatusRunning:
		return next.Terminal()
	default:
		return false
	}
}

type CommandStatus string

const (
	CommandStatusAccepted   CommandStatus = "accepted"
	CommandStatusDispatched CommandStatus = "dispatched"
	CommandStatusStarted    CommandStatus = "started"
	CommandStatusCompleted  CommandStatus = "completed"
	CommandStatusFailed     CommandStatus = "failed"
	CommandStatusExpired    CommandStatus = "expired"
)

func (s CommandStatus) Valid() bool {
	return slices.Contains([]CommandStatus{
		CommandStatusAccepted,
		CommandStatusDispatched,
		CommandStatusStarted,
		CommandStatusCompleted,
		CommandStatusFailed,
		CommandStatusExpired,
	}, s)
}

func (s CommandStatus) Terminal() bool {
	return s == CommandStatusCompleted || s == CommandStatusFailed || s == CommandStatusExpired
}

func (s CommandStatus) CanTransitionTo(next CommandStatus) bool {
	if !s.Valid() || !next.Valid() || s == next || s.Terminal() {
		return false
	}
	switch s {
	case CommandStatusAccepted:
		return next == CommandStatusDispatched || next == CommandStatusStarted || next == CommandStatusFailed || next == CommandStatusExpired
	case CommandStatusDispatched:
		return next == CommandStatusStarted || next == CommandStatusFailed || next == CommandStatusExpired
	case CommandStatusStarted:
		return next == CommandStatusCompleted || next == CommandStatusFailed
	default:
		return false
	}
}

const ChargeStatusCharged = "charged"
