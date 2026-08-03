package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	"github.com/google/uuid"
)

type repositoryStub struct {
	registeredInput    *RegisterDeviceInput
	commandInput       *CreateCommandInput
	syncInput          *CreateSyncInput
	syncTransitions    []SyncTransitionInput
	commandTransitions []CommandTransitionInput
	syncRequest        SyncRequest
	command            Command
	device             Device
	devices            []Device
	presenceStatus     collabdomain.DeviceStatus
	presenceSeenAt     time.Time
}

func (r *repositoryStub) RegisterDevice(_ context.Context, _ int64, input RegisterDeviceInput) (Device, error) {
	r.registeredInput = &input
	return Device{Name: input.Name}, nil
}

func (r *repositoryStub) ListDevices(context.Context, int64) ([]Device, error) {
	return r.devices, nil
}

func (r *repositoryStub) RenameDevice(context.Context, int64, uuid.UUID, string) (Device, error) {
	return Device{}, nil
}

func (r *repositoryStub) RevokeDevice(context.Context, int64, uuid.UUID) (Device, error) {
	return r.device, nil
}

func (r *repositoryStub) GetDevice(context.Context, int64, uuid.UUID) (Device, error) {
	return r.device, nil
}

func (r *repositoryStub) UpdateDevicePresence(_ context.Context, _ int64, _ uuid.UUID, status collabdomain.DeviceStatus, seenAt time.Time) error {
	r.presenceStatus = status
	r.presenceSeenAt = seenAt
	return nil
}

func (r *repositoryStub) CreateSync(_ context.Context, input CreateSyncInput) (CreateSyncResult, error) {
	r.syncInput = &input
	if r.syncRequest.ID == uuid.Nil {
		r.syncRequest = SyncRequest{
			ID: input.IdempotencyKey, UserID: input.UserID, DeviceID: input.DeviceID,
			IdempotencyKey: input.IdempotencyKey, RequestSHA256: input.RequestSHA256,
			Kind: input.Kind, ThreadID: input.ThreadID, Cursor: input.Cursor,
			Status: collabdomain.SyncStatusPending, ExpiresAt: input.ExpiresAt,
		}
	}
	return CreateSyncResult{Sync: r.syncRequest}, nil
}

func (r *repositoryStub) GetSync(context.Context, int64, uuid.UUID) (SyncRequest, error) {
	return r.syncRequest, nil
}

func (r *repositoryStub) TransitionSync(_ context.Context, input SyncTransitionInput) (SyncRequest, error) {
	r.syncTransitions = append(r.syncTransitions, input)
	r.syncRequest.Status = input.Status
	r.syncRequest.ErrorCode = input.ErrorCode
	r.syncRequest.SnapshotVersion = input.SnapshotVersion
	r.syncRequest.ResultCount = input.ResultCount
	return r.syncRequest, nil
}

type presenceStoreStub struct {
	items   map[uuid.UUID]DevicePresence
	touched *DevicePresence
	removed uuid.UUID
}

type realtimeEventBusStub struct {
	deviceEvents []EventEnvelope
	userEvents   []EventEnvelope
	publishErr   error
}

func (s *realtimeEventBusStub) PublishUser(
	_ context.Context,
	_ int64,
	eventType string,
	requestID *string,
	payload map[string]any,
) (EventEnvelope, error) {
	event := EventEnvelope{Version: 1, Type: eventType, RequestID: requestID, Payload: payload}
	s.userEvents = append(s.userEvents, event)
	return event, s.publishErr
}

func (s *realtimeEventBusStub) PublishDevice(
	_ context.Context,
	_ int64,
	_ uuid.UUID,
	eventType string,
	requestID *string,
	payload map[string]any,
) (EventEnvelope, error) {
	event := EventEnvelope{Version: 1, Type: eventType, RequestID: requestID, Payload: payload}
	s.deviceEvents = append(s.deviceEvents, event)
	return event, s.publishErr
}

func (s *realtimeEventBusStub) SubscribeUser(context.Context, int64) (EventSubscription, error) {
	return nil, nil
}

func (s *realtimeEventBusStub) SubscribeDevice(context.Context, uuid.UUID) (EventSubscription, error) {
	return nil, nil
}

type payloadStoreStub struct {
	commandPrompt string
	syncPayload   []byte
	deletedSync   uuid.UUID
}

type commandRateLimiterStub struct {
	allowed bool
	err     error
	keys    []uuid.UUID
}

func (s *commandRateLimiterStub) Allow(_ context.Context, _ int64, idempotencyKey uuid.UUID) (bool, error) {
	s.keys = append(s.keys, idempotencyKey)
	return s.allowed, s.err
}

func (s *payloadStoreStub) PutCommand(_ context.Context, _ int64, _ uuid.UUID, prompt string) error {
	s.commandPrompt = prompt
	return nil
}

func (s *payloadStoreStub) GetCommand(context.Context, int64, uuid.UUID) (string, error) {
	return s.commandPrompt, nil
}

func (s *payloadStoreStub) PutSync(_ context.Context, _ int64, _ uuid.UUID, _ collabdomain.SyncKind, payload []byte) error {
	s.syncPayload = payload
	return nil
}

func (s *payloadStoreStub) GetSync(context.Context, int64, uuid.UUID, collabdomain.SyncKind) ([]byte, error) {
	return s.syncPayload, nil
}

func (s *payloadStoreStub) DeleteSync(_ context.Context, _ int64, syncID uuid.UUID, _ collabdomain.SyncKind) error {
	s.deletedSync = syncID
	return nil
}

func (s *presenceStoreStub) Touch(_ context.Context, presence DevicePresence) error {
	s.touched = &presence
	return nil
}

func (s *presenceStoreStub) GetMany(context.Context, []uuid.UUID) (map[uuid.UUID]DevicePresence, error) {
	return s.items, nil
}

func (s *presenceStoreStub) Remove(_ context.Context, deviceID uuid.UUID) error {
	s.removed = deviceID
	return nil
}

func (r *repositoryStub) CreateCommandAndCharge(_ context.Context, input CreateCommandInput) (CreateCommandResult, error) {
	r.commandInput = &input
	if r.command.ID == uuid.Nil {
		r.command = Command{
			ID: input.IdempotencyKey, UserID: input.UserID, DeviceID: input.DeviceID,
			ThreadID: input.ThreadID, IdempotencyKey: input.IdempotencyKey,
			PromptSHA256: input.PromptSHA256, PromptBytes: input.PromptBytes,
			Status: collabdomain.CommandStatusAccepted, ExpiresAt: input.ExpiresAt,
		}
	}
	return CreateCommandResult{Command: r.command}, nil
}

func (r *repositoryStub) GetCommand(context.Context, int64, uuid.UUID) (Command, error) {
	return r.command, nil
}

func (r *repositoryStub) TransitionCommand(_ context.Context, input CommandTransitionInput) (Command, error) {
	r.commandTransitions = append(r.commandTransitions, input)
	r.command.Status = input.Status
	r.command.TurnID = input.TurnID
	r.command.ErrorCode = input.ErrorCode
	return r.command, nil
}

func (r *repositoryStub) ExpirePending(context.Context, time.Time) (SweepResult, error) {
	return SweepResult{}, nil
}

type chargeCacheStub struct {
	balanceUserIDs []int64
	authUserIDs    []int64
}

func (s *chargeCacheStub) InvalidateUserBalance(_ context.Context, userID int64) error {
	s.balanceUserIDs = append(s.balanceUserIDs, userID)
	return nil
}

func (s *chargeCacheStub) InvalidateAuthCacheByUserID(_ context.Context, userID int64) {
	s.authUserIDs = append(s.authUserIDs, userID)
}

func testConfig() config.CollaborationConfig {
	return config.CollaborationConfig{
		ProtocolVersion:   1,
		TaskFeeAmount:     "0.100000",
		TaskFeeCurrency:   "USD",
		CommandTTLSeconds: 300,
		MaxPromptBytes:    32 * 1024,
	}
}

func TestSubmitCommandBuildsServerOwnedChargeInput(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	deviceID := uuid.New()
	idempotencyKey := uuid.New()
	prompt := "继续实现协同功能"

	_, err = service.SubmitCommand(context.Background(), 42, SubmitCommandInput{
		DeviceID:       deviceID,
		ThreadID:       "thread_123",
		IdempotencyKey: idempotencyKey,
		Prompt:         prompt,
	})
	if err != nil {
		t.Fatalf("SubmitCommand() error = %v", err)
	}
	if repository.commandInput == nil {
		t.Fatal("repository did not receive command")
	}
	digest := sha256.Sum256([]byte(prompt))
	if got, want := repository.commandInput.PromptSHA256, hex.EncodeToString(digest[:]); got != want {
		t.Fatalf("PromptSHA256 = %q, want %q", got, want)
	}
	if got, want := repository.commandInput.PromptBytes, len([]byte(prompt)); got != want {
		t.Fatalf("PromptBytes = %d, want %d", got, want)
	}
	if !repository.commandInput.Fee.Equal(service.fee) || repository.commandInput.Currency != "USD" {
		t.Fatalf("repository received unexpected fee snapshot: %s %s", repository.commandInput.Fee, repository.commandInput.Currency)
	}
	if got, want := repository.commandInput.ExpiresAt, now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("ExpiresAt = %s, want %s", got, want)
	}
}

func TestSubmitCommandInvalidatesBalanceSnapshotsOnlyForNewCharge(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	cache := &chargeCacheStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetChargeCacheInvalidators(cache, cache)
	input := SubmitCommandInput{
		DeviceID:       uuid.New(),
		ThreadID:       "thread_123",
		IdempotencyKey: uuid.New(),
		Prompt:         "continue",
	}

	if _, err := service.SubmitCommand(context.Background(), 42, input); err != nil {
		t.Fatalf("SubmitCommand() error = %v", err)
	}
	if len(cache.balanceUserIDs) != 1 || cache.balanceUserIDs[0] != 42 {
		t.Fatalf("balance invalidations = %v, want [42]", cache.balanceUserIDs)
	}
	if len(cache.authUserIDs) != 1 || cache.authUserIDs[0] != 42 {
		t.Fatalf("auth invalidations = %v, want [42]", cache.authUserIDs)
	}

	repositoryResult := CreateCommandResult{Replayed: true}
	repository.commandInput = nil
	replayRepository := &repositoryResultStub{result: repositoryResult}
	replayService, err := NewService(replayRepository, testConfig())
	if err != nil {
		t.Fatalf("NewService(replay) error = %v", err)
	}
	replayService.SetChargeCacheInvalidators(cache, cache)
	if _, err := replayService.SubmitCommand(context.Background(), 42, input); err != nil {
		t.Fatalf("SubmitCommand(replay) error = %v", err)
	}
	if len(cache.balanceUserIDs) != 1 || len(cache.authUserIDs) != 1 {
		t.Fatal("idempotent replay must not repeat cache invalidation")
	}
}

type repositoryResultStub struct {
	repositoryStub
	result CreateCommandResult
}

func (r *repositoryResultStub) CreateCommandAndCharge(_ context.Context, input CreateCommandInput) (CreateCommandResult, error) {
	r.commandInput = &input
	return r.result, nil
}

func TestSubmitCommandRejectsOversizedPromptBeforeRepository(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	cfg := testConfig()
	cfg.MaxPromptBytes = 4
	service, err := NewService(repository, cfg)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	_, err = service.SubmitCommand(context.Background(), 42, SubmitCommandInput{
		DeviceID:       uuid.New(),
		ThreadID:       "thread_123",
		IdempotencyKey: uuid.New(),
		Prompt:         "12345",
	})
	if err != ErrInvalidArgument {
		t.Fatalf("SubmitCommand() error = %v, want ErrInvalidArgument", err)
	}
	if repository.commandInput != nil {
		t.Fatal("repository must not receive an invalid prompt")
	}
}

func TestRequestSyncRequiresOnlineDeviceAndPublishesCanonicalRequest(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	repository := &repositoryStub{device: Device{
		ID: deviceID, UserID: 42, ProtocolVersion: 1,
		Capabilities: map[string]bool{"thread_read": true},
	}}
	presence := &presenceStoreStub{items: map[uuid.UUID]DevicePresence{
		deviceID: {
			DeviceID: deviceID, UserID: 42, Status: collabdomain.DeviceStatusOnline,
			AppServerStatus: "ready", LastSeenAt: time.Now(),
		},
	}}
	events := &realtimeEventBusStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)
	service.SetRealtime(events, &payloadStoreStub{})
	idempotencyKey := uuid.New()
	result, err := service.RequestSync(context.Background(), 42, RequestSyncInput{
		DeviceID:       deviceID,
		IdempotencyKey: idempotencyKey,
		Kind:           collabdomain.SyncKindSessionList,
		Payload:        map[string]any{"limit": 50, "archived": false},
	})
	if err != nil {
		t.Fatalf("RequestSync() error = %v", err)
	}
	if result.Sync.Status != collabdomain.SyncStatusRunning || repository.syncInput == nil || len(repository.syncInput.RequestSHA256) != 64 {
		t.Fatalf("sync result/input = %#v / %#v", result, repository.syncInput)
	}
	if len(events.deviceEvents) != 1 || events.deviceEvents[0].Type != "session_sync.requested" || events.deviceEvents[0].Payload["sync_id"] != idempotencyKey.String() {
		t.Fatalf("device events = %#v", events.deviceEvents)
	}

	offlineRepository := &repositoryStub{device: repository.device}
	offlineService, err := NewService(offlineRepository, testConfig())
	if err != nil {
		t.Fatalf("NewService(offline) error = %v", err)
	}
	offlineService.SetPresenceStore(&presenceStoreStub{items: map[uuid.UUID]DevicePresence{}})
	if _, err := offlineService.RequestSync(context.Background(), 42, RequestSyncInput{
		DeviceID: deviceID, IdempotencyKey: uuid.New(), Kind: collabdomain.SyncKindSessionList,
	}); err != ErrDeviceOffline || offlineRepository.syncInput != nil {
		t.Fatalf("offline RequestSync() error/input = %v / %#v", err, offlineRepository.syncInput)
	}
}

func TestDispatchCommandStoresPromptAndFailsClosedWhenRelayIsGone(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	repository := &repositoryStub{device: Device{
		ID: deviceID, UserID: 42, ProtocolVersion: 1,
		Capabilities: map[string]bool{"thread_write": true},
	}}
	presence := &presenceStoreStub{items: map[uuid.UUID]DevicePresence{
		deviceID: {
			DeviceID: deviceID, UserID: 42, Status: collabdomain.DeviceStatusOnline,
			AppServerStatus: "ready", LastSeenAt: time.Now(),
		},
	}}
	payloads := &payloadStoreStub{}
	events := &realtimeEventBusStub{publishErr: ErrRelayUnavailable}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)
	service.SetCommandRateLimiter(&commandRateLimiterStub{allowed: true})
	service.SetRealtime(events, payloads)
	result, err := service.DispatchCommand(context.Background(), 42, SubmitCommandInput{
		DeviceID: deviceID, ThreadID: "thread_123", IdempotencyKey: uuid.New(), Prompt: "continue",
	})
	if err != nil {
		t.Fatalf("DispatchCommand() error = %v", err)
	}
	if payloads.commandPrompt != "continue" || result.Command.Status != collabdomain.CommandStatusFailed {
		t.Fatalf("payload/result = %q / %#v", payloads.commandPrompt, result)
	}
	if len(repository.commandTransitions) != 2 || repository.commandTransitions[0].Status != collabdomain.CommandStatusDispatched || repository.commandTransitions[1].Status != collabdomain.CommandStatusFailed {
		t.Fatalf("command transitions = %#v", repository.commandTransitions)
	}
	if result.Command.ErrorCode == nil || *result.Command.ErrorCode != "relay_unavailable" {
		t.Fatalf("command error = %#v", result.Command.ErrorCode)
	}
}

func TestDispatchCommandRateLimitPreventsCharge(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	repository := &repositoryStub{device: Device{
		ID: deviceID, UserID: 42, ProtocolVersion: 1,
		Capabilities: map[string]bool{"thread_write": true},
	}}
	presence := &presenceStoreStub{items: map[uuid.UUID]DevicePresence{
		deviceID: {
			DeviceID: deviceID, UserID: 42, Status: collabdomain.DeviceStatusOnline,
			AppServerStatus: "ready", LastSeenAt: time.Now(),
		},
	}}
	limiter := &commandRateLimiterStub{allowed: false}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)
	service.SetCommandRateLimiter(limiter)
	_, err = service.DispatchCommand(context.Background(), 42, SubmitCommandInput{
		DeviceID: deviceID, ThreadID: "thread_123", IdempotencyKey: uuid.New(), Prompt: "continue",
	})
	if err != ErrCommandRateLimit {
		t.Fatalf("DispatchCommand() error = %v, want ErrCommandRateLimit", err)
	}
	if repository.commandInput != nil || len(limiter.keys) != 1 {
		t.Fatalf("rate-limited command reached repository: input=%#v keys=%v", repository.commandInput, limiter.keys)
	}
}

func TestHandleDeviceEventCompletesSyncWithoutAllowingSnapshotOverwrite(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	syncID := uuid.New()
	repository := &repositoryStub{syncRequest: SyncRequest{
		ID: syncID, UserID: 42, DeviceID: deviceID,
		Kind: collabdomain.SyncKindSessionList, Status: collabdomain.SyncStatusRunning,
	}}
	payloads := &payloadStoreStub{}
	events := &realtimeEventBusStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetRealtime(events, payloads)
	requestID := syncID.String()
	event := EventEnvelope{
		Version: 1, Type: "session_sync.completed", RequestID: &requestID,
		Payload: map[string]any{
			"sync_id": syncID.String(), "snapshot_version": int64(7),
			"items": []any{map[string]any{"thread_id": "thread_123", "title": "Fix tests"}},
		},
	}
	if err := service.HandleDeviceEvent(context.Background(), 42, deviceID, event); err != nil {
		t.Fatalf("HandleDeviceEvent() error = %v", err)
	}
	if repository.syncRequest.Status != collabdomain.SyncStatusCompleted || len(repository.syncTransitions) != 1 {
		t.Fatalf("sync state/transitions = %#v / %#v", repository.syncRequest, repository.syncTransitions)
	}
	if len(payloads.syncPayload) == 0 || len(events.userEvents) != 1 || events.userEvents[0].Type != "session_sync.completed" {
		t.Fatalf("payload/user events = %q / %#v", payloads.syncPayload, events.userEvents)
	}

	stored := string(payloads.syncPayload)
	event.Payload["items"] = []any{map[string]any{"thread_id": "thread_other"}}
	if err := service.HandleDeviceEvent(context.Background(), 42, deviceID, event); err != nil {
		t.Fatalf("HandleDeviceEvent(replay) error = %v", err)
	}
	if got := string(payloads.syncPayload); got != stored || len(repository.syncTransitions) != 1 {
		t.Fatalf("completed snapshot was overwritten: payload=%q transitions=%d", got, len(repository.syncTransitions))
	}

	repository.syncRequest.Status = collabdomain.SyncStatusRunning
	if err := service.HandleDeviceEvent(context.Background(), 42, uuid.New(), event); err != ErrNotFound {
		t.Fatalf("HandleDeviceEvent(other device) error = %v, want ErrNotFound", err)
	}
}

func TestHandleDeviceEventAdvancesCommandAndRejectsConflictingReplay(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	commandID := uuid.New()
	repository := &repositoryStub{command: Command{
		ID: commandID, UserID: 42, DeviceID: deviceID,
		Status: collabdomain.CommandStatusDispatched,
	}}
	events := &realtimeEventBusStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetRealtime(events, &payloadStoreStub{})
	started := EventEnvelope{Type: "command.started", Payload: map[string]any{
		"command_id": commandID.String(), "turn_id": "turn_123",
	}}
	if err := service.HandleDeviceEvent(context.Background(), 42, deviceID, started); err != nil {
		t.Fatalf("HandleDeviceEvent(started) error = %v", err)
	}
	if repository.command.Status != collabdomain.CommandStatusStarted || repository.command.TurnID == nil || *repository.command.TurnID != "turn_123" {
		t.Fatalf("started command = %#v", repository.command)
	}
	started.Payload["turn_id"] = "turn_conflict"
	if err := service.HandleDeviceEvent(context.Background(), 42, deviceID, started); err != ErrInvalidTransition {
		t.Fatalf("HandleDeviceEvent(conflicting replay) error = %v, want ErrInvalidTransition", err)
	}
	completed := EventEnvelope{Type: "command.completed", Payload: map[string]any{"command_id": commandID.String()}}
	if err := service.HandleDeviceEvent(context.Background(), 42, deviceID, completed); err != nil {
		t.Fatalf("HandleDeviceEvent(completed) error = %v", err)
	}
	if repository.command.Status != collabdomain.CommandStatusCompleted || len(repository.commandTransitions) != 2 || len(events.userEvents) != 2 {
		t.Fatalf("command transitions/user events = %#v / %#v", repository.commandTransitions, events.userEvents)
	}
	if err := service.HandleDeviceEvent(context.Background(), 42, deviceID, completed); err != nil {
		t.Fatalf("HandleDeviceEvent(completed replay) error = %v", err)
	}
	if len(repository.commandTransitions) != 2 || len(events.userEvents) != 3 {
		t.Fatalf("completed replay mutated state: transitions=%d events=%d", len(repository.commandTransitions), len(events.userEvents))
	}
}

func TestCancelCommandPublishesInterruptWithoutChangingChargeState(t *testing.T) {
	t.Parallel()
	deviceID := uuid.New()
	commandID := uuid.New()
	turnID := "turn_123"
	repository := &repositoryStub{command: Command{
		ID: commandID, UserID: 42, DeviceID: deviceID, ThreadID: "thread_123",
		Status: collabdomain.CommandStatusStarted, TurnID: &turnID,
	}}
	events := &realtimeEventBusStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetRealtime(events, nil)

	result, err := service.CancelCommand(context.Background(), 42, commandID)
	if err != nil {
		t.Fatalf("CancelCommand() error = %v", err)
	}
	if !result.Requested || result.Command.Status != collabdomain.CommandStatusStarted || len(events.deviceEvents) != 1 {
		t.Fatalf("cancel result/events = %#v / %#v", result, events.deviceEvents)
	}
	event := events.deviceEvents[0]
	if event.Type != "command.cancel_requested" || event.Payload["command_id"] != commandID.String() || event.Payload["turn_id"] != turnID {
		t.Fatalf("cancel event = %#v", event)
	}
	for _, forbidden := range []string{"fee", "charge", "amount", "currency", "refund", "balance"} {
		if _, exposed := event.Payload[forbidden]; exposed {
			t.Fatalf("cancel event exposed billing field %q: %#v", forbidden, event.Payload)
		}
	}
	if len(repository.commandTransitions) != 0 {
		t.Fatalf("cancellation changed command or charge state: %#v", repository.commandTransitions)
	}
}

func TestCancelCommandIsIdempotentForTerminalCommand(t *testing.T) {
	t.Parallel()
	commandID := uuid.New()
	repository := &repositoryStub{command: Command{
		ID: commandID, UserID: 42, DeviceID: uuid.New(), ThreadID: "thread_123",
		Status: collabdomain.CommandStatusCompleted,
	}}
	events := &realtimeEventBusStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetRealtime(events, nil)

	result, err := service.CancelCommand(context.Background(), 42, commandID)
	if err != nil || result.Requested || len(events.deviceEvents) != 0 {
		t.Fatalf("terminal cancel result/error/events = %#v / %v / %#v", result, err, events.deviceEvents)
	}
}

func TestRegisterDeviceNormalizesMetadataAndChecksProtocol(t *testing.T) {
	t.Parallel()

	repository := &repositoryStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	input := RegisterDeviceInput{
		InstallationIDHash: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Name:               "  Workstation  ",
		Platform:           " LINUX ",
		CompanionVersion:   " 0.1.0 ",
		ProtocolVersion:    1,
	}

	_, err = service.RegisterDevice(context.Background(), 42, input)
	if err != nil {
		t.Fatalf("RegisterDevice() error = %v", err)
	}
	if repository.registeredInput == nil {
		t.Fatal("repository did not receive device")
	}
	if repository.registeredInput.Name != "Workstation" || repository.registeredInput.Platform != "linux" {
		t.Fatalf("device metadata was not normalized: %#v", repository.registeredInput)
	}

	input.ProtocolVersion = 2
	_, err = service.RegisterDevice(context.Background(), 42, input)
	if err != ErrProtocolMismatch {
		t.Fatalf("RegisterDevice() error = %v, want ErrProtocolMismatch", err)
	}
}

func TestRevokeDeviceRemovesPresenceAndRequestsRemoteShutdown(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	repository := &repositoryStub{device: Device{
		ID: deviceID, UserID: 42, Status: collabdomain.DeviceStatusRevoked,
	}}
	presence := &presenceStoreStub{}
	events := &realtimeEventBusStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)
	service.SetRealtime(events, nil)

	device, err := service.RevokeDevice(context.Background(), 42, deviceID)
	if err != nil {
		t.Fatalf("RevokeDevice() error = %v", err)
	}
	if device.ID != deviceID || presence.removed != deviceID {
		t.Fatalf("device/presence = %#v / %s", device, presence.removed)
	}
	if len(events.deviceEvents) != 1 || events.deviceEvents[0].Type != "server.shutdown" ||
		events.deviceEvents[0].Payload["reason"] != "device_revoked" {
		t.Fatalf("shutdown events = %#v", events.deviceEvents)
	}
}

func TestListDevicesProjectsRedisPresence(t *testing.T) {
	t.Parallel()

	onlineID := uuid.New()
	offlineID := uuid.New()
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	repository := &repositoryStub{devices: []Device{
		{ID: onlineID, UserID: 42, Status: collabdomain.DeviceStatusOffline},
		{ID: offlineID, UserID: 42, Status: collabdomain.DeviceStatusOnline},
	}}
	presence := &presenceStoreStub{items: map[uuid.UUID]DevicePresence{
		onlineID: {
			DeviceID:   onlineID,
			UserID:     42,
			Status:     collabdomain.DeviceStatusOnline,
			LastSeenAt: now,
		},
	}}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	service.SetPresenceStore(presence)

	devices, err := service.ListDevices(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListDevices() error = %v", err)
	}
	if devices[0].Status != collabdomain.DeviceStatusOnline || devices[0].LastSeenAt == nil || !devices[0].LastSeenAt.Equal(now) {
		t.Fatalf("online projection = %#v", devices[0])
	}
	if devices[1].Status != collabdomain.DeviceStatusOffline {
		t.Fatalf("missing presence status = %s, want offline", devices[1].Status)
	}
}

func TestRecordHeartbeatAuthenticatesAndRefreshesPresence(t *testing.T) {
	t.Parallel()

	deviceID := uuid.New()
	repository := &repositoryStub{device: Device{
		ID:              deviceID,
		UserID:          42,
		ProtocolVersion: 1,
		Status:          collabdomain.DeviceStatusOffline,
	}}
	presenceStore := &presenceStoreStub{}
	service, err := NewService(repository, testConfig())
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	now := time.Date(2026, time.August, 3, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.SetPresenceStore(presenceStore)

	presence, err := service.RecordHeartbeat(context.Background(), 42, deviceID, "ready", 2)
	if err != nil {
		t.Fatalf("RecordHeartbeat() error = %v", err)
	}
	if presence.Status != collabdomain.DeviceStatusOnline || repository.presenceStatus != collabdomain.DeviceStatusOnline {
		t.Fatalf("heartbeat status = %s/%s", presence.Status, repository.presenceStatus)
	}
	if presenceStore.touched == nil || *presenceStore.touched != presence {
		t.Fatalf("presence touch = %#v, want %#v", presenceStore.touched, presence)
	}

	if _, err := service.RecordHeartbeat(context.Background(), 42, deviceID, "unknown", 0); err != ErrInvalidArgument {
		t.Fatalf("invalid heartbeat error = %v, want ErrInvalidArgument", err)
	}
}

func TestNewServiceRejectsNonUSDChargeConfig(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.TaskFeeCurrency = "CNY"
	if _, err := NewService(&repositoryStub{}, cfg); err == nil {
		t.Fatal("NewService() accepted non-USD collaboration fee")
	}
}
