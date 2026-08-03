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
	registeredInput *RegisterDeviceInput
	commandInput    *CreateCommandInput
	device          Device
	devices         []Device
	presenceStatus  collabdomain.DeviceStatus
	presenceSeenAt  time.Time
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

type presenceStoreStub struct {
	items   map[uuid.UUID]DevicePresence
	touched *DevicePresence
	removed uuid.UUID
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
	return CreateCommandResult{}, nil
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
