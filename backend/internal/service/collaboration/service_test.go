package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

type repositoryStub struct {
	registeredInput *RegisterDeviceInput
	commandInput    *CreateCommandInput
}

func (r *repositoryStub) RegisterDevice(_ context.Context, _ int64, input RegisterDeviceInput) (Device, error) {
	r.registeredInput = &input
	return Device{Name: input.Name}, nil
}

func (r *repositoryStub) ListDevices(context.Context, int64) ([]Device, error) {
	return nil, nil
}

func (r *repositoryStub) RenameDevice(context.Context, int64, uuid.UUID, string) (Device, error) {
	return Device{}, nil
}

func (r *repositoryStub) RevokeDevice(context.Context, int64, uuid.UUID) (Device, error) {
	return Device{}, nil
}

func (r *repositoryStub) CreateCommandAndCharge(_ context.Context, input CreateCommandInput) (CreateCommandResult, error) {
	r.commandInput = &input
	return CreateCommandResult{}, nil
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

func TestNewServiceRejectsNonUSDChargeConfig(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	cfg.TaskFeeCurrency = "CNY"
	if _, err := NewService(&repositoryStub{}, cfg); err == nil {
		t.Fatal("NewService() accepted non-USD collaboration fee")
	}
}
