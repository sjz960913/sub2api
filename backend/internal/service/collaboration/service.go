package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	ErrDeviceOffline    = errors.New("collaboration device offline")
	ErrDeviceCapability = errors.New("collaboration device capability unavailable")
	ErrRelayUnavailable = errors.New("collaboration relay unavailable")
	ErrCommandRateLimit = errors.New("collaboration command rate limit reached")
)

var installationHashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var collaborationErrorCodePattern = regexp.MustCompile(`^[a-z0-9_]{1,128}$`)

type Service struct {
	repository      Repository
	protocolVersion int
	fee             decimal.Decimal
	currency        string
	commandTTL      time.Duration
	syncTTL         time.Duration
	maxPromptBytes  int
	maxEventBytes   int64
	balanceCache    BalanceCacheInvalidator
	authCache       AuthCacheInvalidator
	presence        PresenceStore
	commandLimiter  CommandRateLimiter
	eventBus        EventBus
	payloads        PayloadStore
	now             func() time.Time
}

func (s *Service) SetPresenceStore(presence PresenceStore) {
	s.presence = presence
}

func (s *Service) SetRealtime(eventBus EventBus, payloads PayloadStore) {
	s.eventBus = eventBus
	s.payloads = payloads
}

func (s *Service) SetCommandRateLimiter(limiter CommandRateLimiter) {
	s.commandLimiter = limiter
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
	syncTTL := time.Duration(cfg.SyncTTLSeconds) * time.Second
	if syncTTL <= 0 {
		syncTTL = 10 * time.Second
	}
	maxEventBytes := cfg.MaxEventBytes
	if maxEventBytes <= 0 {
		maxEventBytes = 1024 * 1024
	}
	return &Service{
		repository:      repository,
		protocolVersion: cfg.ProtocolVersion,
		fee:             fee,
		currency:        "USD",
		commandTTL:      time.Duration(cfg.CommandTTLSeconds) * time.Second,
		syncTTL:         syncTTL,
		maxPromptBytes:  cfg.MaxPromptBytes,
		maxEventBytes:   maxEventBytes,
		now:             time.Now,
	}, nil
}

type RequestSyncInput struct {
	DeviceID       uuid.UUID
	IdempotencyKey uuid.UUID
	Kind           collabdomain.SyncKind
	ThreadID       *string
	Cursor         *string
	Payload        map[string]any
}

func (s *Service) RequestSync(
	ctx context.Context,
	userID int64,
	input RequestSyncInput,
) (CreateSyncResult, error) {
	if userID <= 0 || input.DeviceID == uuid.Nil || input.IdempotencyKey == uuid.Nil || !input.Kind.Valid() {
		return CreateSyncResult{}, ErrInvalidArgument
	}
	input.ThreadID = normalizedOptionalString(input.ThreadID)
	input.Cursor = normalizedOptionalString(input.Cursor)
	if input.Kind == collabdomain.SyncKindThreadSnapshot && (input.ThreadID == nil || len(*input.ThreadID) > 512) {
		return CreateSyncResult{}, ErrInvalidArgument
	}
	if input.Kind == collabdomain.SyncKindSessionList && input.ThreadID != nil {
		return CreateSyncResult{}, ErrInvalidArgument
	}
	if input.Cursor != nil && len(*input.Cursor) > 1024 {
		return CreateSyncResult{}, ErrInvalidArgument
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil || int64(len(payload)) > s.maxEventBytes {
		return CreateSyncResult{}, ErrInvalidArgument
	}
	if _, err := s.requireOnlineDevice(ctx, userID, input.DeviceID, "thread_read"); err != nil {
		return CreateSyncResult{}, err
	}
	digestInput, err := json.Marshal(struct {
		DeviceID uuid.UUID             `json:"device_id"`
		Kind     collabdomain.SyncKind `json:"kind"`
		ThreadID *string               `json:"thread_id"`
		Cursor   *string               `json:"cursor"`
		Payload  json.RawMessage       `json:"payload"`
	}{
		DeviceID: input.DeviceID,
		Kind:     input.Kind,
		ThreadID: input.ThreadID,
		Cursor:   input.Cursor,
		Payload:  payload,
	})
	if err != nil {
		return CreateSyncResult{}, ErrInvalidArgument
	}
	digest := sha256.Sum256(digestInput)
	result, err := s.repository.CreateSync(ctx, CreateSyncInput{
		UserID:         userID,
		DeviceID:       input.DeviceID,
		IdempotencyKey: input.IdempotencyKey,
		RequestSHA256:  hex.EncodeToString(digest[:]),
		Kind:           input.Kind,
		ThreadID:       input.ThreadID,
		Cursor:         input.Cursor,
		ExpiresAt:      s.now().UTC().Add(s.syncTTL),
	})
	if err != nil {
		return result, err
	}
	if result.Replayed && result.Sync.Status != collabdomain.SyncStatusPending {
		return result, nil
	}
	if s.eventBus == nil {
		return s.failSyncDispatch(ctx, result, ErrRelayUnavailable)
	}
	running, err := s.repository.TransitionSync(ctx, SyncTransitionInput{
		UserID:     userID,
		DeviceID:   input.DeviceID,
		SyncID:     result.Sync.ID,
		Status:     collabdomain.SyncStatusRunning,
		OccurredAt: s.now().UTC(),
	})
	if err != nil {
		return CreateSyncResult{}, err
	}
	result.Sync = running
	eventPayload := make(map[string]any, len(input.Payload)+4)
	for key, value := range input.Payload {
		eventPayload[key] = value
	}
	eventPayload["sync_id"] = result.Sync.ID.String()
	eventPayload["device_id"] = input.DeviceID.String()
	eventPayload["expires_at"] = result.Sync.ExpiresAt
	if input.ThreadID != nil {
		eventPayload["thread_id"] = *input.ThreadID
	}
	eventType := "session_sync.requested"
	if input.Kind == collabdomain.SyncKindThreadSnapshot {
		eventType = "thread_sync.requested"
	}
	requestID := result.Sync.ID.String()
	if _, err := s.eventBus.PublishDevice(ctx, userID, input.DeviceID, eventType, &requestID, eventPayload); err != nil {
		return s.failSyncDispatch(ctx, result, ErrRelayUnavailable)
	}
	return result, nil
}

func (s *Service) failSyncDispatch(
	ctx context.Context,
	result CreateSyncResult,
	dispatchErr error,
) (CreateSyncResult, error) {
	errorCode := "relay_unavailable"
	failed, err := s.repository.TransitionSync(ctx, SyncTransitionInput{
		UserID:     result.Sync.UserID,
		DeviceID:   result.Sync.DeviceID,
		SyncID:     result.Sync.ID,
		Status:     collabdomain.SyncStatusFailed,
		ErrorCode:  &errorCode,
		OccurredAt: s.now().UTC(),
	})
	if err == nil {
		result.Sync = failed
	}
	return result, dispatchErr
}

func (s *Service) GetSyncResult(
	ctx context.Context,
	userID int64,
	syncID uuid.UUID,
) (SyncRequest, json.RawMessage, error) {
	if userID <= 0 || syncID == uuid.Nil {
		return SyncRequest{}, nil, ErrInvalidArgument
	}
	syncRequest, err := s.repository.GetSync(ctx, userID, syncID)
	if err != nil || syncRequest.Status != collabdomain.SyncStatusCompleted {
		return syncRequest, nil, err
	}
	if s.payloads == nil {
		return syncRequest, nil, ErrPayloadNotFound
	}
	payload, err := s.payloads.GetSync(ctx, userID, syncID, syncRequest.Kind)
	return syncRequest, payload, err
}

func (s *Service) requireOnlineDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	capability string,
) (Device, error) {
	device, err := s.AuthenticateDevice(ctx, userID, deviceID)
	if err != nil {
		return Device{}, err
	}
	if !device.Capabilities[capability] {
		return Device{}, ErrDeviceCapability
	}
	if s.presence == nil {
		return Device{}, ErrDeviceOffline
	}
	presenceByDevice, err := s.presence.GetMany(ctx, []uuid.UUID{deviceID})
	if err != nil {
		return Device{}, ErrRelayUnavailable
	}
	presence, online := presenceByDevice[deviceID]
	if !online || presence.UserID != userID || presence.Status != collabdomain.DeviceStatusOnline || presence.AppServerStatus != "ready" {
		return Device{}, ErrDeviceOffline
	}
	return device, nil
}

func normalizedOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := strings.TrimSpace(*value)
	if normalized == "" {
		return nil
	}
	return &normalized
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
	input, promptBytes, err := s.validatedCommandInput(userID, input)
	if err != nil {
		return CreateCommandResult{}, err
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

type DispatchCommandResult struct {
	Command  Command
	Replayed bool
}

func (s *Service) DispatchCommand(
	ctx context.Context,
	userID int64,
	input SubmitCommandInput,
) (DispatchCommandResult, error) {
	input, _, err := s.validatedCommandInput(userID, input)
	if err != nil {
		return DispatchCommandResult{}, err
	}
	if _, err := s.requireOnlineDevice(ctx, userID, input.DeviceID, "thread_write"); err != nil {
		return DispatchCommandResult{}, err
	}
	if s.commandLimiter == nil {
		return DispatchCommandResult{}, ErrRelayUnavailable
	}
	allowed, err := s.commandLimiter.Allow(ctx, userID, input.IdempotencyKey)
	if err != nil {
		return DispatchCommandResult{}, ErrRelayUnavailable
	}
	if !allowed {
		return DispatchCommandResult{}, ErrCommandRateLimit
	}
	created, err := s.SubmitCommand(ctx, userID, input)
	if err != nil {
		return DispatchCommandResult{}, err
	}
	result := DispatchCommandResult{Command: created.Command, Replayed: created.Replayed}
	if created.Replayed && created.Command.Status != collabdomain.CommandStatusAccepted {
		return result, nil
	}
	if s.payloads == nil {
		failed, transitionErr := s.failCommandDispatch(ctx, created.Command, "payload_unavailable")
		result.Command = failed
		return result, transitionErr
	}
	if err := s.payloads.PutCommand(ctx, userID, created.Command.ID, input.Prompt); err != nil {
		failed, transitionErr := s.failCommandDispatch(ctx, created.Command, "payload_unavailable")
		result.Command = failed
		return result, transitionErr
	}
	dispatched, err := s.repository.TransitionCommand(ctx, CommandTransitionInput{
		UserID:     userID,
		DeviceID:   input.DeviceID,
		CommandID:  created.Command.ID,
		Status:     collabdomain.CommandStatusDispatched,
		OccurredAt: s.now().UTC(),
	})
	if err != nil {
		return result, err
	}
	result.Command = dispatched
	if s.eventBus == nil {
		failed, transitionErr := s.failCommandDispatch(ctx, dispatched, "relay_unavailable")
		result.Command = failed
		return result, transitionErr
	}
	requestID := dispatched.ID.String()
	_, publishErr := s.eventBus.PublishDevice(ctx, userID, input.DeviceID, "command.dispatched", &requestID, map[string]any{
		"command_id": dispatched.ID.String(),
		"thread_id":  dispatched.ThreadID,
		"input": []map[string]string{
			{"type": "text", "text": input.Prompt},
		},
		"expires_at": dispatched.ExpiresAt,
	})
	if publishErr != nil {
		failed, transitionErr := s.failCommandDispatch(ctx, dispatched, "relay_unavailable")
		result.Command = failed
		return result, transitionErr
	}
	return result, nil
}

func (s *Service) validatedCommandInput(
	userID int64,
	input SubmitCommandInput,
) (SubmitCommandInput, []byte, error) {
	input.ThreadID = strings.TrimSpace(input.ThreadID)
	promptBytes := []byte(input.Prompt)
	if userID <= 0 || input.DeviceID == uuid.Nil || input.IdempotencyKey == uuid.Nil ||
		input.ThreadID == "" || len(input.ThreadID) > 512 || len(promptBytes) == 0 || len(promptBytes) > s.maxPromptBytes {
		return SubmitCommandInput{}, nil, ErrInvalidArgument
	}
	return input, promptBytes, nil
}

func (s *Service) failCommandDispatch(
	ctx context.Context,
	command Command,
	errorCode string,
) (Command, error) {
	return s.repository.TransitionCommand(ctx, CommandTransitionInput{
		UserID:     command.UserID,
		DeviceID:   command.DeviceID,
		CommandID:  command.ID,
		Status:     collabdomain.CommandStatusFailed,
		ErrorCode:  &errorCode,
		OccurredAt: s.now().UTC(),
	})
}

func (s *Service) GetCommand(
	ctx context.Context,
	userID int64,
	commandID uuid.UUID,
) (Command, error) {
	if userID <= 0 || commandID == uuid.Nil {
		return Command{}, ErrInvalidArgument
	}
	return s.repository.GetCommand(ctx, userID, commandID)
}

func (s *Service) HandleDeviceEvent(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	event EventEnvelope,
) error {
	if userID <= 0 || deviceID == uuid.Nil {
		return ErrInvalidArgument
	}
	switch event.Type {
	case "session_sync.completed":
		return s.completeSync(ctx, userID, deviceID, event, collabdomain.SyncKindSessionList)
	case "thread_sync.completed":
		return s.completeSync(ctx, userID, deviceID, event, collabdomain.SyncKindThreadSnapshot)
	case "session_sync.failed":
		return s.failSync(ctx, userID, deviceID, event, collabdomain.SyncKindSessionList)
	case "thread_sync.failed":
		return s.failSync(ctx, userID, deviceID, event, collabdomain.SyncKindThreadSnapshot)
	case "command.received":
		command, err := s.commandForDeviceEvent(ctx, userID, deviceID, event)
		if err != nil {
			return err
		}
		if command.Status == collabdomain.CommandStatusAccepted {
			return ErrInvalidTransition
		}
		s.publishUserBestEffort(ctx, userID, event.Type, event.RequestID, map[string]any{
			"command_id": command.ID.String(),
			"status":     command.Status,
		})
		return nil
	case "command.started":
		return s.transitionCommandEvent(ctx, userID, deviceID, event, collabdomain.CommandStatusStarted)
	case "command.completed":
		return s.transitionCommandEvent(ctx, userID, deviceID, event, collabdomain.CommandStatusCompleted)
	case "command.failed":
		return s.transitionCommandEvent(ctx, userID, deviceID, event, collabdomain.CommandStatusFailed)
	case "command.item":
		if containsForbiddenCollaborationPayload(event.Payload) {
			return ErrInvalidArgument
		}
		command, err := s.commandForDeviceEvent(ctx, userID, deviceID, event)
		if err != nil {
			return err
		}
		if command.Status != collabdomain.CommandStatusStarted {
			return ErrInvalidTransition
		}
		threadID, threadOK := event.Payload["thread_id"].(string)
		turnID, turnOK := event.Payload["turn_id"].(string)
		item, itemOK := event.Payload["item"].(map[string]any)
		if !threadOK || threadID != command.ThreadID || !turnOK || command.TurnID == nil || turnID != *command.TurnID ||
			!itemOK || !validCollaborationItem(item) {
			return ErrInvalidArgument
		}
		s.publishUserBestEffort(ctx, userID, event.Type, event.RequestID, map[string]any{
			"command_id": command.ID.String(),
			"thread_id":  command.ThreadID,
			"turn_id":    *command.TurnID,
			"item":       item,
		})
		return nil
	default:
		return ErrInvalidArgument
	}
}

func (s *Service) completeSync(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	event EventEnvelope,
	kind collabdomain.SyncKind,
) error {
	syncID, err := payloadUUID(event.Payload, "sync_id")
	if err != nil {
		return err
	}
	syncRequest, err := s.repository.GetSync(ctx, userID, syncID)
	if err != nil {
		return err
	}
	if syncRequest.DeviceID != deviceID || syncRequest.Kind != kind {
		return ErrNotFound
	}
	if syncRequest.Status == collabdomain.SyncStatusCompleted {
		return nil
	}
	if !syncRequest.Status.CanTransitionTo(collabdomain.SyncStatusCompleted) {
		return ErrInvalidTransition
	}
	items, ok := event.Payload["items"].([]any)
	if !ok || (kind == collabdomain.SyncKindSessionList && len(items) > 100) ||
		(kind == collabdomain.SyncKindThreadSnapshot && len(items) > 200) || containsForbiddenCollaborationPayload(event.Payload) {
		return ErrInvalidArgument
	}
	snapshotVersion, ok := collaborationPayloadInt64(event.Payload["snapshot_version"])
	if !ok || snapshotVersion < 0 {
		return ErrInvalidArgument
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil || int64(len(payload)) > s.maxEventBytes || s.payloads == nil {
		return ErrInvalidArgument
	}
	if err := s.payloads.PutSync(ctx, userID, syncID, kind, payload); err != nil {
		return err
	}
	completed, err := s.repository.TransitionSync(ctx, SyncTransitionInput{
		UserID:          userID,
		DeviceID:        deviceID,
		SyncID:          syncID,
		Status:          collabdomain.SyncStatusCompleted,
		SnapshotVersion: &snapshotVersion,
		ResultCount:     len(items),
		OccurredAt:      s.now().UTC(),
	})
	if err != nil {
		_ = s.payloads.DeleteSync(ctx, userID, syncID, kind)
		return err
	}
	s.publishUserBestEffort(ctx, userID, event.Type, event.RequestID, map[string]any{
		"sync_id":          completed.ID.String(),
		"status":           completed.Status,
		"snapshot_version": completed.SnapshotVersion,
		"result_count":     completed.ResultCount,
	})
	return nil
}

func (s *Service) failSync(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	event EventEnvelope,
	kind collabdomain.SyncKind,
) error {
	syncID, err := payloadUUID(event.Payload, "sync_id")
	if err != nil {
		return err
	}
	syncRequest, err := s.repository.GetSync(ctx, userID, syncID)
	if err != nil {
		return err
	}
	if syncRequest.DeviceID != deviceID || syncRequest.Kind != kind {
		return ErrNotFound
	}
	errorCode, err := payloadErrorCode(event.Payload)
	if err != nil {
		return err
	}
	if syncRequest.Status == collabdomain.SyncStatusFailed {
		if syncRequest.ErrorCode == nil || *syncRequest.ErrorCode != errorCode {
			return ErrInvalidTransition
		}
		return nil
	}
	if !syncRequest.Status.CanTransitionTo(collabdomain.SyncStatusFailed) {
		return ErrInvalidTransition
	}
	failed, err := s.repository.TransitionSync(ctx, SyncTransitionInput{
		UserID:     userID,
		DeviceID:   deviceID,
		SyncID:     syncID,
		Status:     collabdomain.SyncStatusFailed,
		ErrorCode:  &errorCode,
		OccurredAt: s.now().UTC(),
	})
	if err != nil {
		return err
	}
	s.publishUserBestEffort(ctx, userID, event.Type, event.RequestID, map[string]any{
		"sync_id":    failed.ID.String(),
		"status":     failed.Status,
		"error_code": errorCode,
	})
	return nil
}

func (s *Service) transitionCommandEvent(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	event EventEnvelope,
	status collabdomain.CommandStatus,
) error {
	command, err := s.commandForDeviceEvent(ctx, userID, deviceID, event)
	if err != nil {
		return err
	}
	input := CommandTransitionInput{
		UserID:     userID,
		DeviceID:   deviceID,
		CommandID:  command.ID,
		Status:     status,
		OccurredAt: s.now().UTC(),
	}
	if status == collabdomain.CommandStatusStarted {
		turnID, ok := event.Payload["turn_id"].(string)
		turnID = strings.TrimSpace(turnID)
		if !ok || turnID == "" || len(turnID) > 512 {
			return ErrInvalidArgument
		}
		input.TurnID = &turnID
	}
	if status == collabdomain.CommandStatusFailed {
		errorCode, err := payloadErrorCode(event.Payload)
		if err != nil {
			return err
		}
		input.ErrorCode = &errorCode
	}
	if command.Status == status {
		if status == collabdomain.CommandStatusStarted && !sameOptionalString(command.TurnID, input.TurnID) {
			return ErrInvalidTransition
		}
		if status == collabdomain.CommandStatusFailed && !sameOptionalString(command.ErrorCode, input.ErrorCode) {
			return ErrInvalidTransition
		}
		s.publishCommandStatus(ctx, userID, event, command)
		return nil
	}
	if !command.Status.CanTransitionTo(status) {
		return ErrInvalidTransition
	}
	updated, err := s.repository.TransitionCommand(ctx, input)
	if err != nil {
		return err
	}
	s.publishCommandStatus(ctx, userID, event, updated)
	return nil
}

func (s *Service) publishCommandStatus(
	ctx context.Context,
	userID int64,
	event EventEnvelope,
	command Command,
) {
	payload := map[string]any{
		"command_id": command.ID.String(),
		"status":     command.Status,
		"turn_id":    command.TurnID,
	}
	if command.ErrorCode != nil {
		payload["error_code"] = *command.ErrorCode
	}
	s.publishUserBestEffort(ctx, userID, event.Type, event.RequestID, payload)
}

func (s *Service) commandForDeviceEvent(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	event EventEnvelope,
) (Command, error) {
	commandID, err := payloadUUID(event.Payload, "command_id")
	if err != nil {
		return Command{}, err
	}
	command, err := s.repository.GetCommand(ctx, userID, commandID)
	if err != nil {
		return Command{}, err
	}
	if command.DeviceID != deviceID {
		return Command{}, ErrNotFound
	}
	return command, nil
}

func (s *Service) publishUserBestEffort(
	ctx context.Context,
	userID int64,
	eventType string,
	requestID *string,
	payload map[string]any,
) {
	if s.eventBus != nil {
		_, _ = s.eventBus.PublishUser(ctx, userID, eventType, requestID, payload)
	}
}

func payloadUUID(payload map[string]any, key string) (uuid.UUID, error) {
	value, ok := payload[key].(string)
	if !ok {
		return uuid.Nil, ErrInvalidArgument
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, ErrInvalidArgument
	}
	return id, nil
}

func payloadErrorCode(payload map[string]any) (string, error) {
	errorCode, ok := payload["error_code"].(string)
	errorCode = strings.ToLower(strings.TrimSpace(errorCode))
	if !ok || !collaborationErrorCodePattern.MatchString(errorCode) {
		return "", ErrInvalidArgument
	}
	return errorCode, nil
}

func collaborationPayloadInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		converted := int64(typed)
		return converted, typed == float64(converted)
	case int:
		return int64(typed), true
	case int64:
		return typed, true
	default:
		return 0, false
	}
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validCollaborationItem(item map[string]any) bool {
	itemID, itemIDOK := item["item_id"].(string)
	itemType, itemTypeOK := item["type"].(string)
	status, statusOK := item["status"].(string)
	sequence, sequenceOK := collaborationPayloadInt64(item["sequence"])
	if !itemIDOK || strings.TrimSpace(itemID) == "" || len(itemID) > 512 ||
		!itemTypeOK || !statusOK || strings.TrimSpace(status) == "" || len(status) > 64 ||
		!sequenceOK || sequence < 0 {
		return false
	}
	switch itemType {
	case "user_message", "agent_message", "reasoning_summary", "command_execution", "file_change",
		"tool_call", "tool_result", "plan", "error", "unsupported":
		return true
	default:
		return false
	}
}

func containsForbiddenCollaborationPayload(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.TrimSpace(key))
			switch normalized {
			case "source_path", "rollout_path", "raw", "stderr", "authorization", "access_token", "refresh_token", "api_key", "charge", "amount", "currency":
				return true
			}
			if containsForbiddenCollaborationPayload(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if containsForbiddenCollaborationPayload(child) {
				return true
			}
		}
	}
	return false
}
