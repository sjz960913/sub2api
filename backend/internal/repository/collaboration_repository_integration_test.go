//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collabservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestCollaborationCommandChargeConcurrentIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	user := createCollaborationTestUser(t, decimal.RequireFromString("10"), decimal.RequireFromString("1.5"))
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, user.ID)
	input := collaborationCommandInput(user.ID, device.ID, uuid.New())

	const workers = 50
	start := make(chan struct{})
	results := make(chan collabservice.CreateCommandResult, workers)
	errorsCh := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for range workers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			<-start
			result, err := repository.CreateCommandAndCharge(ctx, input)
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}()
	}
	close(start)
	waitGroup.Wait()
	close(results)
	close(errorsCh)

	for err := range errorsCh {
		t.Errorf("CreateCommandAndCharge() error = %v", err)
	}
	var (
		commandID uuid.UUID
		chargeID  uuid.UUID
		created   int
		seen      int
	)
	for result := range results {
		seen++
		if commandID == uuid.Nil {
			commandID = result.Command.ID
			chargeID = result.Charge.ID
		}
		if result.Command.ID != commandID || result.Charge.ID != chargeID {
			t.Errorf("idempotent call returned different IDs: command=%s charge=%s", result.Command.ID, result.Charge.ID)
		}
		if !result.Replayed {
			created++
		}
	}
	if seen != workers {
		t.Fatalf("successful results = %d, want %d", seen, workers)
	}
	if created != 1 {
		t.Fatalf("new command results = %d, want 1", created)
	}

	assertCollaborationCounts(t, user.ID, 1, 1)
	var balance, frozenBalance string
	if err := integrationDB.QueryRowContext(ctx, `
		SELECT balance::text, frozen_balance::text FROM users WHERE id = $1
	`, user.ID).Scan(&balance, &frozenBalance); err != nil {
		t.Fatalf("query user balances: %v", err)
	}
	if balance != "9.90000000" || frozenBalance != "1.50000000" {
		t.Fatalf("balances = %s/%s, want 9.90000000/1.50000000", balance, frozenBalance)
	}
}

func TestCollaborationCommandChargeInsufficientBalanceRollsBack(t *testing.T) {
	user := createCollaborationTestUser(t, decimal.RequireFromString("0.05"), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, user.ID)

	_, err := repository.CreateCommandAndCharge(context.Background(), collaborationCommandInput(user.ID, device.ID, uuid.New()))
	var balanceError *collabservice.InsufficientBalanceError
	if !errors.As(err, &balanceError) {
		t.Fatalf("CreateCommandAndCharge() error = %v, want InsufficientBalanceError", err)
	}
	assertCollaborationCounts(t, user.ID, 0, 0)

	var balance string
	if err := integrationDB.QueryRow(`SELECT balance::text FROM users WHERE id = $1`, user.ID).Scan(&balance); err != nil {
		t.Fatalf("query user balance: %v", err)
	}
	if balance != "0.05000000" {
		t.Fatalf("balance = %s, want 0.05000000", balance)
	}
}

func TestCollaborationCommandIdempotencyConflictDoesNotChargeAgain(t *testing.T) {
	user := createCollaborationTestUser(t, decimal.RequireFromString("1"), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, user.ID)
	idempotencyKey := uuid.New()
	input := collaborationCommandInput(user.ID, device.ID, idempotencyKey)

	first, err := repository.CreateCommandAndCharge(context.Background(), input)
	if err != nil {
		t.Fatalf("first CreateCommandAndCharge() error = %v", err)
	}
	differentDigest := sha256.Sum256([]byte("different"))
	input.PromptSHA256 = hex.EncodeToString(differentDigest[:])
	input.PromptBytes++
	_, err = repository.CreateCommandAndCharge(context.Background(), input)
	if !errors.Is(err, collabservice.ErrIdempotencyConflict) {
		t.Fatalf("second CreateCommandAndCharge() error = %v, want ErrIdempotencyConflict", err)
	}
	assertCollaborationCounts(t, user.ID, 1, 1)

	var balance string
	if err := integrationDB.QueryRow(`SELECT balance::text FROM users WHERE id = $1`, user.ID).Scan(&balance); err != nil {
		t.Fatalf("query user balance: %v", err)
	}
	if balance != "0.90000000" || first.Charge.BalanceAfter.StringFixed(8) != balance {
		t.Fatalf("balance = %s, want one direct charge", balance)
	}
}

func TestCollaborationDeviceQueriesAreUserScoped(t *testing.T) {
	owner := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	other := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, owner.ID)
	if _, err := repository.GetDevice(context.Background(), other.ID, device.ID); !errors.Is(err, collabservice.ErrNotFound) {
		t.Fatalf("GetDevice() cross-user error = %v, want ErrNotFound", err)
	}
	if err := repository.UpdateDevicePresence(context.Background(), other.ID, device.ID, collabdomain.DeviceStatusOnline, time.Now()); !errors.Is(err, collabservice.ErrNotFound) {
		t.Fatalf("UpdateDevicePresence() cross-user error = %v, want ErrNotFound", err)
	}
	seenAt := time.Now().UTC().Truncate(time.Microsecond)
	if err := repository.UpdateDevicePresence(context.Background(), owner.ID, device.ID, collabdomain.DeviceStatusOnline, seenAt); err != nil {
		t.Fatalf("UpdateDevicePresence() owner error = %v", err)
	}
	updatedDevice, err := repository.GetDevice(context.Background(), owner.ID, device.ID)
	if err != nil {
		t.Fatalf("GetDevice() owner error = %v", err)
	}
	if updatedDevice.Status != collabdomain.DeviceStatusOnline || updatedDevice.LastSeenAt == nil || !updatedDevice.LastSeenAt.Equal(seenAt) {
		t.Fatalf("updated device = %#v", updatedDevice)
	}

	if _, err := repository.RenameDevice(context.Background(), other.ID, device.ID, "stolen"); !errors.Is(err, collabservice.ErrNotFound) {
		t.Fatalf("RenameDevice() cross-user error = %v, want ErrNotFound", err)
	}
	if _, err := repository.RevokeDevice(context.Background(), other.ID, device.ID); !errors.Is(err, collabservice.ErrNotFound) {
		t.Fatalf("RevokeDevice() cross-user error = %v, want ErrNotFound", err)
	}
	devices, err := repository.ListDevices(context.Background(), other.ID)
	if err != nil {
		t.Fatalf("ListDevices() error = %v", err)
	}
	if len(devices) != 0 {
		t.Fatalf("other user can see %d devices, want 0", len(devices))
	}

	if _, err := repository.RevokeDevice(context.Background(), owner.ID, device.ID); err != nil {
		t.Fatalf("RevokeDevice() owner error = %v", err)
	}
	_, err = repository.RegisterDevice(context.Background(), owner.ID, collabservice.RegisterDeviceInput{
		InstallationIDHash: device.InstallationIDHash,
		Name:               "Workstation",
		Platform:           "linux",
		CompanionVersion:   "0.1.0",
		ProtocolVersion:    1,
		Capabilities:       map[string]bool{"app_server": true},
	})
	if !errors.Is(err, collabservice.ErrDeviceRevoked) {
		t.Fatalf("RegisterDevice() revoked error = %v, want ErrDeviceRevoked", err)
	}
}

func TestCollaborationExpirePendingConvergesWithoutRefund(t *testing.T) {
	user := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, user.ID)
	now := time.Now().UTC()

	expiredInput := collaborationCommandInput(user.ID, device.ID, uuid.New())
	expiredInput.ExpiresAt = now.Add(-time.Minute)
	expiredCommand, err := repository.CreateCommandAndCharge(context.Background(), expiredInput)
	if err != nil {
		t.Fatalf("create expired command: %v", err)
	}
	startedInput := collaborationCommandInput(user.ID, device.ID, uuid.New())
	startedInput.ExpiresAt = now.Add(-time.Minute)
	startedCommand, err := repository.CreateCommandAndCharge(context.Background(), startedInput)
	if err != nil {
		t.Fatalf("create started command: %v", err)
	}
	if _, err := integrationDB.Exec(`
		UPDATE collaboration_commands SET status = 'started', started_at = $2 WHERE id = $1
	`, startedCommand.Command.ID, now.Add(-2*time.Minute)); err != nil {
		t.Fatalf("mark command started: %v", err)
	}

	pendingSyncID := uuid.New()
	completedSyncID := uuid.New()
	for _, item := range []struct {
		id     uuid.UUID
		status string
	}{
		{id: pendingSyncID, status: "pending"},
		{id: completedSyncID, status: "completed"},
	} {
		if _, err := integrationDB.Exec(`
			INSERT INTO collaboration_sync_requests (
				id, user_id, device_id, idempotency_key, request_sha256, kind, status,
				expires_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, 'session_list', $6, $7, $8, $8)
		`, item.id, user.ID, device.ID, uuid.New(), strings.Repeat("0", 64), item.status, now.Add(-time.Minute), now.Add(-2*time.Minute)); err != nil {
			t.Fatalf("insert %s sync request: %v", item.status, err)
		}
	}

	result, err := repository.ExpirePending(context.Background(), now)
	if err != nil {
		t.Fatalf("ExpirePending() error = %v", err)
	}
	if result.ExpiredCommands != 1 || result.ExpiredSyncs != 1 {
		t.Fatalf("ExpirePending() = %#v, want one command and one sync", result)
	}

	var expiredStatus, startedStatus, pendingStatus, completedStatus string
	if err := integrationDB.QueryRow(`SELECT status FROM collaboration_commands WHERE id = $1`, expiredCommand.Command.ID).Scan(&expiredStatus); err != nil {
		t.Fatalf("query expired command: %v", err)
	}
	if err := integrationDB.QueryRow(`SELECT status FROM collaboration_commands WHERE id = $1`, startedCommand.Command.ID).Scan(&startedStatus); err != nil {
		t.Fatalf("query started command: %v", err)
	}
	if err := integrationDB.QueryRow(`SELECT status FROM collaboration_sync_requests WHERE id = $1`, pendingSyncID).Scan(&pendingStatus); err != nil {
		t.Fatalf("query pending sync: %v", err)
	}
	if err := integrationDB.QueryRow(`SELECT status FROM collaboration_sync_requests WHERE id = $1`, completedSyncID).Scan(&completedStatus); err != nil {
		t.Fatalf("query completed sync: %v", err)
	}
	if expiredStatus != "expired" || startedStatus != "started" || pendingStatus != "expired" || completedStatus != "completed" {
		t.Fatalf("statuses = %s/%s/%s/%s", expiredStatus, startedStatus, pendingStatus, completedStatus)
	}

	var balance string
	if err := integrationDB.QueryRow(`SELECT balance::text FROM users WHERE id = $1`, user.ID).Scan(&balance); err != nil {
		t.Fatalf("query user balance: %v", err)
	}
	if balance != "0.80000000" {
		t.Fatalf("balance = %s, want 0.80000000 with no expiry refund", balance)
	}
}

func TestCollaborationSyncLifecycleIsIdempotentAndTenantScoped(t *testing.T) {
	owner := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	other := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, owner.ID)
	now := time.Now().UTC().Truncate(time.Microsecond)
	idempotencyKey := uuid.New()
	input := collabservice.CreateSyncInput{
		UserID:         owner.ID,
		DeviceID:       device.ID,
		IdempotencyKey: idempotencyKey,
		RequestSHA256:  strings.Repeat("a", 64),
		Kind:           collabdomain.SyncKindSessionList,
		ExpiresAt:      now.Add(time.Minute),
	}
	created, err := repository.CreateSync(context.Background(), input)
	if err != nil || created.Replayed {
		t.Fatalf("CreateSync() = %#v, %v", created, err)
	}
	replayed, err := repository.CreateSync(context.Background(), input)
	if err != nil || !replayed.Replayed || replayed.Sync.ID != created.Sync.ID {
		t.Fatalf("CreateSync(replay) = %#v, %v", replayed, err)
	}
	conflict := input
	conflict.RequestSHA256 = strings.Repeat("b", 64)
	if _, err := repository.CreateSync(context.Background(), conflict); !errors.Is(err, collabservice.ErrIdempotencyConflict) {
		t.Fatalf("CreateSync(conflict) error = %v", err)
	}
	if _, err := repository.GetSync(context.Background(), other.ID, created.Sync.ID); !errors.Is(err, collabservice.ErrNotFound) {
		t.Fatalf("GetSync(cross-user) error = %v", err)
	}
	running, err := repository.TransitionSync(context.Background(), collabservice.SyncTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, SyncID: created.Sync.ID,
		Status: collabdomain.SyncStatusRunning, OccurredAt: now,
	})
	if err != nil || running.Status != collabdomain.SyncStatusRunning {
		t.Fatalf("TransitionSync(running) = %#v, %v", running, err)
	}
	snapshotVersion := int64(4)
	completed, err := repository.TransitionSync(context.Background(), collabservice.SyncTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, SyncID: created.Sync.ID,
		Status: collabdomain.SyncStatusCompleted, SnapshotVersion: &snapshotVersion,
		ResultCount: 2, OccurredAt: now.Add(time.Second),
	})
	if err != nil || completed.Status != collabdomain.SyncStatusCompleted || completed.SnapshotVersion == nil || *completed.SnapshotVersion != 4 || completed.ResultCount != 2 {
		t.Fatalf("TransitionSync(completed) = %#v, %v", completed, err)
	}
	if replayed, err := repository.TransitionSync(context.Background(), collabservice.SyncTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, SyncID: created.Sync.ID,
		Status: collabdomain.SyncStatusCompleted, SnapshotVersion: &snapshotVersion,
		ResultCount: 2, OccurredAt: now.Add(2 * time.Second),
	}); err != nil || replayed.ID != completed.ID {
		t.Fatalf("TransitionSync(completed replay) = %#v, %v", replayed, err)
	}
	conflictingVersion := int64(5)
	if _, err := repository.TransitionSync(context.Background(), collabservice.SyncTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, SyncID: created.Sync.ID,
		Status: collabdomain.SyncStatusCompleted, SnapshotVersion: &conflictingVersion,
		ResultCount: 2, OccurredAt: now.Add(2 * time.Second),
	}); !errors.Is(err, collabservice.ErrInvalidTransition) {
		t.Fatalf("TransitionSync(conflicting replay) error = %v", err)
	}
	if _, err := repository.TransitionSync(context.Background(), collabservice.SyncTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, SyncID: created.Sync.ID,
		Status: collabdomain.SyncStatusFailed, OccurredAt: now.Add(2 * time.Second),
	}); !errors.Is(err, collabservice.ErrInvalidTransition) {
		t.Fatalf("TransitionSync(terminal) error = %v", err)
	}
}

func TestCollaborationCommandTransitionsAreAtomicAndTenantScoped(t *testing.T) {
	owner := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	other := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, owner.ID)
	created, err := repository.CreateCommandAndCharge(context.Background(), collaborationCommandInput(owner.ID, device.ID, uuid.New()))
	if err != nil {
		t.Fatalf("CreateCommandAndCharge() error = %v", err)
	}
	if _, err := repository.GetCommand(context.Background(), other.ID, created.Command.ID); !errors.Is(err, collabservice.ErrNotFound) {
		t.Fatalf("GetCommand(cross-user) error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	dispatched, err := repository.TransitionCommand(context.Background(), collabservice.CommandTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, CommandID: created.Command.ID,
		Status: collabdomain.CommandStatusDispatched, OccurredAt: now,
	})
	if err != nil || dispatched.DispatchedAt == nil {
		t.Fatalf("TransitionCommand(dispatched) = %#v, %v", dispatched, err)
	}
	turnID := "turn_123"
	started, err := repository.TransitionCommand(context.Background(), collabservice.CommandTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, CommandID: created.Command.ID,
		Status: collabdomain.CommandStatusStarted, TurnID: &turnID, OccurredAt: now.Add(time.Second),
	})
	if err != nil || started.TurnID == nil || *started.TurnID != turnID {
		t.Fatalf("TransitionCommand(started) = %#v, %v", started, err)
	}
	conflictingTurnID := "turn_other"
	if _, err := repository.TransitionCommand(context.Background(), collabservice.CommandTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, CommandID: created.Command.ID,
		Status: collabdomain.CommandStatusStarted, TurnID: &conflictingTurnID, OccurredAt: now.Add(time.Second),
	}); !errors.Is(err, collabservice.ErrInvalidTransition) {
		t.Fatalf("TransitionCommand(conflicting replay) error = %v", err)
	}
	completed, err := repository.TransitionCommand(context.Background(), collabservice.CommandTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, CommandID: created.Command.ID,
		Status: collabdomain.CommandStatusCompleted, OccurredAt: now.Add(2 * time.Second),
	})
	if err != nil || completed.CompletedAt == nil {
		t.Fatalf("TransitionCommand(completed) = %#v, %v", completed, err)
	}
	if _, err := repository.TransitionCommand(context.Background(), collabservice.CommandTransitionInput{
		UserID: owner.ID, DeviceID: device.ID, CommandID: created.Command.ID,
		Status: collabdomain.CommandStatusFailed, OccurredAt: now.Add(3 * time.Second),
	}); !errors.Is(err, collabservice.ErrInvalidTransition) {
		t.Fatalf("TransitionCommand(terminal) error = %v", err)
	}
}

func TestCollaborationDeviceRevocationFailsActiveWorkWithoutRefund(t *testing.T) {
	user := createCollaborationTestUser(t, decimal.NewFromInt(1), decimal.Zero)
	repository := NewCollaborationRepository(integrationDB)
	device := registerCollaborationTestDevice(t, repository, user.ID)
	command, err := repository.CreateCommandAndCharge(context.Background(), collaborationCommandInput(user.ID, device.ID, uuid.New()))
	if err != nil {
		t.Fatalf("CreateCommandAndCharge() error = %v", err)
	}
	syncRequest, err := repository.CreateSync(context.Background(), collabservice.CreateSyncInput{
		UserID: user.ID, DeviceID: device.ID, IdempotencyKey: uuid.New(),
		RequestSHA256: strings.Repeat("c", 64), Kind: collabdomain.SyncKindSessionList,
		ExpiresAt: time.Now().UTC().Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("CreateSync() error = %v", err)
	}
	revoked, err := repository.RevokeDevice(context.Background(), user.ID, device.ID)
	if err != nil || revoked.Status != collabdomain.DeviceStatusRevoked {
		t.Fatalf("RevokeDevice() = %#v, %v", revoked, err)
	}
	failedCommand, err := repository.GetCommand(context.Background(), user.ID, command.Command.ID)
	if err != nil || failedCommand.Status != collabdomain.CommandStatusFailed || failedCommand.ErrorCode == nil || *failedCommand.ErrorCode != "device_revoked" {
		t.Fatalf("revoked command = %#v, %v", failedCommand, err)
	}
	failedSync, err := repository.GetSync(context.Background(), user.ID, syncRequest.Sync.ID)
	if err != nil || failedSync.Status != collabdomain.SyncStatusFailed || failedSync.ErrorCode == nil || *failedSync.ErrorCode != "device_revoked" {
		t.Fatalf("revoked sync = %#v, %v", failedSync, err)
	}
	var balance string
	if err := integrationDB.QueryRow(`SELECT balance::text FROM users WHERE id = $1`, user.ID).Scan(&balance); err != nil {
		t.Fatalf("query user balance: %v", err)
	}
	if balance != "0.90000000" {
		t.Fatalf("balance = %s, want charged balance without refund", balance)
	}
}

func createCollaborationTestUser(t *testing.T, balance, frozenBalance decimal.Decimal) *dbent.User {
	t.Helper()
	ctx := context.Background()
	user, err := integrationEntClient.User.Create().
		SetEmail(fmt.Sprintf("collaboration-%s@example.test", uuid.NewString())).
		SetPasswordHash("test-password-hash").
		SetBalance(balance.InexactFloat64()).
		SetFrozenBalance(frozenBalance.InexactFloat64()).
		Save(ctx)
	if err != nil {
		t.Fatalf("create collaboration user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM collaboration_charges WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM collaboration_commands WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM collaboration_sync_requests WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM collaboration_devices WHERE user_id = $1`, user.ID)
		_, _ = integrationDB.ExecContext(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	})
	return user
}

func registerCollaborationTestDevice(t *testing.T, repository collabservice.Repository, userID int64) collabservice.Device {
	t.Helper()
	digest := sha256.Sum256([]byte(uuid.NewString()))
	device, err := repository.RegisterDevice(context.Background(), userID, collabservice.RegisterDeviceInput{
		InstallationIDHash: "sha256:" + hex.EncodeToString(digest[:]),
		Name:               "Workstation",
		Platform:           "linux",
		CompanionVersion:   "0.1.0",
		ProtocolVersion:    1,
		Capabilities:       map[string]bool{"app_server": true, "thread_read": true, "thread_write": true},
	})
	if err != nil {
		t.Fatalf("RegisterDevice() error = %v", err)
	}
	return device
}

func collaborationCommandInput(userID int64, deviceID, idempotencyKey uuid.UUID) collabservice.CreateCommandInput {
	prompt := []byte("continue the task")
	digest := sha256.Sum256(prompt)
	return collabservice.CreateCommandInput{
		UserID:         userID,
		DeviceID:       deviceID,
		ThreadID:       "thread_123",
		IdempotencyKey: idempotencyKey,
		PromptSHA256:   hex.EncodeToString(digest[:]),
		PromptBytes:    len(prompt),
		Fee:            decimal.RequireFromString("0.1"),
		Currency:       "USD",
		ExpiresAt:      time.Now().UTC().Add(5 * time.Minute),
	}
}

func assertCollaborationCounts(t *testing.T, userID int64, wantCommands, wantCharges int) {
	t.Helper()
	var commands, charges int
	if err := integrationDB.QueryRow(`SELECT COUNT(*) FROM collaboration_commands WHERE user_id = $1`, userID).Scan(&commands); err != nil {
		t.Fatalf("count collaboration commands: %v", err)
	}
	if err := integrationDB.QueryRow(`SELECT COUNT(*) FROM collaboration_charges WHERE user_id = $1`, userID).Scan(&charges); err != nil {
		t.Fatalf("count collaboration charges: %v", err)
	}
	if commands != wantCommands || charges != wantCharges {
		t.Fatalf("command/charge counts = %d/%d, want %d/%d", commands, charges, wantCommands, wantCharges)
	}
}
