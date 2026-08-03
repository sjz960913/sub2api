//go:build integration

package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
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
				id, user_id, device_id, idempotency_key, kind, status,
				expires_at, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 'session_list', $5, $6, $7, $7)
		`, item.id, user.ID, device.ID, uuid.New(), item.status, now.Add(-time.Minute), now.Add(-2*time.Minute)); err != nil {
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
		Capabilities:       map[string]bool{"app_server": true, "thread_write": true},
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
