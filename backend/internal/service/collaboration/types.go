package collaboration

import (
	"context"
	"errors"
	"fmt"
	"time"

	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrNotFound            = errors.New("collaboration resource not found")
	ErrDeviceRevoked       = errors.New("collaboration device revoked")
	ErrIdempotencyConflict = errors.New("collaboration idempotency conflict")
	ErrInvariantViolation  = errors.New("collaboration persistence invariant violated")
)

type InsufficientBalanceError struct {
	Available decimal.Decimal
	Required  decimal.Decimal
}

func (e *InsufficientBalanceError) Error() string {
	return fmt.Sprintf("insufficient balance: available=%s required=%s", e.Available.String(), e.Required.String())
}

type Device struct {
	ID                 uuid.UUID
	UserID             int64
	InstallationIDHash string
	Name               string
	Platform           string
	PlatformVersion    *string
	CompanionVersion   string
	CodexVersion       *string
	ProtocolVersion    int
	Status             collabdomain.DeviceStatus
	Capabilities       map[string]bool
	LastSeenAt         *time.Time
	RevokedAt          *time.Time
	RegisteredAt       time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type DevicePresence struct {
	DeviceID          uuid.UUID
	UserID            int64
	Status            collabdomain.DeviceStatus
	AppServerStatus   string
	ActiveThreadCount int
	LastSeenAt        time.Time
}

type RegisterDeviceInput struct {
	InstallationIDHash string
	Name               string
	Platform           string
	PlatformVersion    *string
	CompanionVersion   string
	CodexVersion       *string
	ProtocolVersion    int
	Capabilities       map[string]bool
}

type Command struct {
	ID             uuid.UUID
	UserID         int64
	DeviceID       uuid.UUID
	ThreadID       string
	IdempotencyKey uuid.UUID
	PromptSHA256   string
	PromptBytes    int
	Status         collabdomain.CommandStatus
	TurnID         *string
	ErrorCode      *string
	ExpiresAt      time.Time
	DispatchedAt   *time.Time
	StartedAt      *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Charge struct {
	ID            uuid.UUID
	CommandID     uuid.UUID
	UserID        int64
	Amount        decimal.Decimal
	Currency      string
	Status        string
	BalanceBefore decimal.Decimal
	BalanceAfter  decimal.Decimal
	Reason        *string
	ChargedAt     time.Time
}

type CreateCommandInput struct {
	UserID         int64
	DeviceID       uuid.UUID
	ThreadID       string
	IdempotencyKey uuid.UUID
	PromptSHA256   string
	PromptBytes    int
	Fee            decimal.Decimal
	Currency       string
	ExpiresAt      time.Time
	Reason         *string
}

type CreateCommandResult struct {
	Command  Command
	Charge   Charge
	Replayed bool
}

type SweepResult struct {
	ExpiredCommands int64
	ExpiredSyncs    int64
}

type Repository interface {
	RegisterDevice(context.Context, int64, RegisterDeviceInput) (Device, error)
	ListDevices(context.Context, int64) ([]Device, error)
	RenameDevice(context.Context, int64, uuid.UUID, string) (Device, error)
	RevokeDevice(context.Context, int64, uuid.UUID) (Device, error)
	GetDevice(context.Context, int64, uuid.UUID) (Device, error)
	UpdateDevicePresence(context.Context, int64, uuid.UUID, collabdomain.DeviceStatus, time.Time) error
	CreateCommandAndCharge(context.Context, CreateCommandInput) (CreateCommandResult, error)
	ExpirePending(context.Context, time.Time) (SweepResult, error)
}

type PresenceStore interface {
	Touch(context.Context, DevicePresence) error
	GetMany(context.Context, []uuid.UUID) (map[uuid.UUID]DevicePresence, error)
	Remove(context.Context, uuid.UUID) error
}

// BalanceCacheInvalidator removes the cached balance snapshot after a committed
// collaboration charge. Implementations must be safe to call more than once.
type BalanceCacheInvalidator interface {
	InvalidateUserBalance(context.Context, int64) error
}

// AuthCacheInvalidator removes API-key authentication snapshots that embed the
// user's balance. It deliberately exposes only the user-scoped operation.
type AuthCacheInvalidator interface {
	InvalidateAuthCacheByUserID(context.Context, int64)
}
