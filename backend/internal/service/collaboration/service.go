package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

var (
	ErrInvalidArgument  = errors.New("invalid collaboration argument")
	ErrProtocolMismatch = errors.New("collaboration protocol mismatch")
)

var installationHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

type Service struct {
	repository      Repository
	protocolVersion int
	fee             decimal.Decimal
	currency        string
	commandTTL      time.Duration
	maxPromptBytes  int
	balanceCache    BalanceCacheInvalidator
	authCache       AuthCacheInvalidator
	presence        PresenceStore
	now             func() time.Time
}

func (s *Service) SetPresenceStore(presence PresenceStore) {
	s.presence = presence
}

func NewService(repository Repository, cfg config.CollaborationConfig) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("collaboration repository is required: %w", ErrInvalidArgument)
	}
	fee, err := decimal.NewFromString(strings.TrimSpace(cfg.TaskFeeAmount))
	if err != nil || !fee.IsPositive() {
		return nil, fmt.Errorf("collaboration task fee must be a positive decimal: %w", ErrInvalidArgument)
	}
	if strings.TrimSpace(cfg.TaskFeeCurrency) != "USD" {
		return nil, fmt.Errorf("collaboration task fee currency must be USD: %w", ErrInvalidArgument)
	}
	if cfg.ProtocolVersion <= 0 || cfg.CommandTTLSeconds <= 0 || cfg.MaxPromptBytes <= 0 {
		return nil, fmt.Errorf("collaboration protocol, command TTL, and prompt limit must be positive: %w", ErrInvalidArgument)
	}
	return &Service{
		repository:      repository,
		protocolVersion: cfg.ProtocolVersion,
		fee:             fee,
		currency:        "USD",
		commandTTL:      time.Duration(cfg.CommandTTLSeconds) * time.Second,
		maxPromptBytes:  cfg.MaxPromptBytes,
		now:             time.Now,
	}, nil
}

func (s *Service) SetChargeCacheInvalidators(
	balanceCache BalanceCacheInvalidator,
	authCache AuthCacheInvalidator,
) {
	s.balanceCache = balanceCache
	s.authCache = authCache
}

func (s *Service) RegisterDevice(
	ctx context.Context,
	userID int64,
	input RegisterDeviceInput,
) (Device, error) {
	input.InstallationIDHash = strings.TrimSpace(input.InstallationIDHash)
	input.Name = strings.TrimSpace(input.Name)
	input.Platform = strings.ToLower(strings.TrimSpace(input.Platform))
	input.CompanionVersion = strings.TrimSpace(input.CompanionVersion)
	if input.PlatformVersion != nil {
		value := strings.TrimSpace(*input.PlatformVersion)
		input.PlatformVersion = &value
	}
	if input.CodexVersion != nil {
		value := strings.TrimSpace(*input.CodexVersion)
		input.CodexVersion = &value
	}
	if input.Capabilities == nil {
		input.Capabilities = map[string]bool{}
	}

	if userID <= 0 || !installationHashPattern.MatchString(input.InstallationIDHash) {
		return Device{}, ErrInvalidArgument
	}
	if input.Name == "" || utf8.RuneCountInString(input.Name) > 100 {
		return Device{}, ErrInvalidArgument
	}
	if input.Platform != "windows" && input.Platform != "macos" && input.Platform != "linux" {
		return Device{}, ErrInvalidArgument
	}
	if input.CompanionVersion == "" || len(input.CompanionVersion) > 64 {
		return Device{}, ErrInvalidArgument
	}
	if input.ProtocolVersion != s.protocolVersion {
		return Device{}, ErrProtocolMismatch
	}
	return s.repository.RegisterDevice(ctx, userID, input)
}

func (s *Service) ListDevices(ctx context.Context, userID int64) ([]Device, error) {
	if userID <= 0 {
		return nil, ErrInvalidArgument
	}
	devices, err := s.repository.ListDevices(ctx, userID)
	if err != nil || s.presence == nil || len(devices) == 0 {
		return devices, err
	}
	deviceIDs := make([]uuid.UUID, 0, len(devices))
	for _, device := range devices {
		deviceIDs = append(deviceIDs, device.ID)
	}
	presenceByDevice, presenceErr := s.presence.GetMany(ctx, deviceIDs)
	if presenceErr != nil {
		slog.Warn("read collaboration presence failed", "user_id", userID, "error", presenceErr)
		for index := range devices {
			devices[index].Status = collabdomain.DeviceStatusOffline
		}
		return devices, nil
	}
	for index := range devices {
		presence, online := presenceByDevice[devices[index].ID]
		if !online || presence.UserID != userID {
			devices[index].Status = collabdomain.DeviceStatusOffline
			continue
		}
		devices[index].Status = presence.Status
		lastSeenAt := presence.LastSeenAt
		devices[index].LastSeenAt = &lastSeenAt
	}
	return devices, nil
}

func (s *Service) RenameDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	name string,
) (Device, error) {
	name = strings.TrimSpace(name)
	if userID <= 0 || deviceID == uuid.Nil || name == "" || utf8.RuneCountInString(name) > 100 {
		return Device{}, ErrInvalidArgument
	}
	return s.repository.RenameDevice(ctx, userID, deviceID, name)
}

func (s *Service) RevokeDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
) (Device, error) {
	if userID <= 0 || deviceID == uuid.Nil {
		return Device{}, ErrInvalidArgument
	}
	device, err := s.repository.RevokeDevice(ctx, userID, deviceID)
	if err == nil && s.presence != nil {
		if removeErr := s.presence.Remove(ctx, deviceID); removeErr != nil {
			slog.Warn("remove revoked collaboration presence failed", "device_id", deviceID, "error", removeErr)
		}
	}
	return device, err
}

func (s *Service) AuthenticateDevice(ctx context.Context, userID int64, deviceID uuid.UUID) (Device, error) {
	if userID <= 0 || deviceID == uuid.Nil {
		return Device{}, ErrInvalidArgument
	}
	device, err := s.repository.GetDevice(ctx, userID, deviceID)
	if err != nil {
		return Device{}, err
	}
	if device.Status == collabdomain.DeviceStatusRevoked {
		return Device{}, ErrDeviceRevoked
	}
	if device.ProtocolVersion != s.protocolVersion {
		return Device{}, ErrProtocolMismatch
	}
	return device, nil
}

func (s *Service) RecordHeartbeat(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	appServerStatus string,
	activeThreadCount int,
) (DevicePresence, error) {
	appServerStatus = strings.ToLower(strings.TrimSpace(appServerStatus))
	if userID <= 0 || deviceID == uuid.Nil || activeThreadCount < 0 || activeThreadCount > 10000 {
		return DevicePresence{}, ErrInvalidArgument
	}
	if appServerStatus != "ready" && appServerStatus != "starting" && appServerStatus != "unavailable" {
		return DevicePresence{}, ErrInvalidArgument
	}
	if _, err := s.AuthenticateDevice(ctx, userID, deviceID); err != nil {
		return DevicePresence{}, err
	}
	status := collabdomain.DeviceStatusDegraded
	if appServerStatus == "ready" {
		status = collabdomain.DeviceStatusOnline
	}
	now := s.now().UTC()
	if err := s.repository.UpdateDevicePresence(ctx, userID, deviceID, status, now); err != nil {
		return DevicePresence{}, err
	}
	presence := DevicePresence{
		DeviceID:          deviceID,
		UserID:            userID,
		Status:            status,
		AppServerStatus:   appServerStatus,
		ActiveThreadCount: activeThreadCount,
		LastSeenAt:        now,
	}
	if s.presence != nil {
		if err := s.presence.Touch(ctx, presence); err != nil {
			return DevicePresence{}, err
		}
	}
	return presence, nil
}

func (s *Service) RecordDisconnect(ctx context.Context, userID int64, deviceID uuid.UUID) {
	if userID <= 0 || deviceID == uuid.Nil {
		return
	}
	now := s.now().UTC()
	if s.presence != nil {
		if err := s.presence.Remove(ctx, deviceID); err != nil {
			slog.Warn("remove disconnected collaboration presence failed", "device_id", deviceID, "error", err)
		}
	}
	if err := s.repository.UpdateDevicePresence(ctx, userID, deviceID, collabdomain.DeviceStatusOffline, now); err != nil {
		slog.Warn("mark collaboration device offline failed", "device_id", deviceID, "error", err)
	}
}

type SubmitCommandInput struct {
	DeviceID       uuid.UUID
	ThreadID       string
	IdempotencyKey uuid.UUID
	Prompt         string
}

func (s *Service) SubmitCommand(
	ctx context.Context,
	userID int64,
	input SubmitCommandInput,
) (CreateCommandResult, error) {
	input.ThreadID = strings.TrimSpace(input.ThreadID)
	promptBytes := []byte(input.Prompt)
	if userID <= 0 || input.DeviceID == uuid.Nil || input.IdempotencyKey == uuid.Nil {
		return CreateCommandResult{}, ErrInvalidArgument
	}
	if input.ThreadID == "" || len(input.ThreadID) > 512 || len(promptBytes) == 0 || len(promptBytes) > s.maxPromptBytes {
		return CreateCommandResult{}, ErrInvalidArgument
	}

	digest := sha256.Sum256(promptBytes)
	result, err := s.repository.CreateCommandAndCharge(ctx, CreateCommandInput{
		UserID:         userID,
		DeviceID:       input.DeviceID,
		ThreadID:       input.ThreadID,
		IdempotencyKey: input.IdempotencyKey,
		PromptSHA256:   hex.EncodeToString(digest[:]),
		PromptBytes:    len(promptBytes),
		Fee:            s.fee,
		Currency:       s.currency,
		ExpiresAt:      s.now().UTC().Add(s.commandTTL),
	})
	if err != nil || result.Replayed {
		return result, err
	}

	// The database transaction is authoritative. Cache invalidation happens only
	// after commit and must not turn an accepted, charged command into a client
	// retry. A detached bounded context also survives request cancellation.
	cacheCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(cacheCtx, userID)
	}
	if s.balanceCache != nil {
		if invalidateErr := s.balanceCache.InvalidateUserBalance(cacheCtx, userID); invalidateErr != nil {
			slog.Warn("invalidate collaboration balance cache failed", "user_id", userID, "error", invalidateErr)
		}
	}
	return result, nil
}
