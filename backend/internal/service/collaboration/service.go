package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/config"
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
	now             func() time.Time
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
	return s.repository.ListDevices(ctx, userID)
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
	return s.repository.RevokeDevice(ctx, userID, deviceID)
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
	return s.repository.CreateCommandAndCharge(ctx, CreateCommandInput{
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
}
