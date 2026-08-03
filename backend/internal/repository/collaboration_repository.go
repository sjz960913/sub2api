package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	collabdomain "github.com/Wei-Shaw/sub2api/internal/domain/collaboration"
	collabservice "github.com/Wei-Shaw/sub2api/internal/service/collaboration"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type collaborationRepository struct {
	db  *sql.DB
	now func() time.Time
}

func NewCollaborationRepository(db *sql.DB) collabservice.Repository {
	return &collaborationRepository{
		db:  db,
		now: time.Now,
	}
}

func (r *collaborationRepository) RegisterDevice(
	ctx context.Context,
	userID int64,
	input collabservice.RegisterDeviceInput,
) (collabservice.Device, error) {
	capabilities, err := json.Marshal(input.Capabilities)
	if err != nil {
		return collabservice.Device{}, fmt.Errorf("marshal collaboration capabilities: %w", err)
	}

	now := r.now().UTC()
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO collaboration_devices (
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, registered_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'offline', $10, $11, $11, $11)
		ON CONFLICT (user_id, installation_id_hash) DO UPDATE SET
			name = EXCLUDED.name,
			platform = EXCLUDED.platform,
			platform_version = EXCLUDED.platform_version,
			companion_version = EXCLUDED.companion_version,
			codex_version = EXCLUDED.codex_version,
			protocol_version = EXCLUDED.protocol_version,
			capabilities = EXCLUDED.capabilities,
			updated_at = EXCLUDED.updated_at
		WHERE collaboration_devices.status <> 'revoked'
		RETURNING
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, last_seen_at, revoked_at, registered_at, created_at, updated_at
	`, uuid.New(), userID, input.InstallationIDHash, input.Name, input.Platform,
		input.PlatformVersion, input.CompanionVersion, input.CodexVersion,
		input.ProtocolVersion, capabilities, now)

	device, err := scanCollaborationDevice(row)
	if !errors.Is(err, sql.ErrNoRows) {
		return device, err
	}

	var status collabdomain.DeviceStatus
	err = r.db.QueryRowContext(ctx, `
		SELECT status
		FROM collaboration_devices
		WHERE user_id = $1 AND installation_id_hash = $2
	`, userID, input.InstallationIDHash).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.Device{}, collabservice.ErrNotFound
	}
	if err != nil {
		return collabservice.Device{}, err
	}
	if status == collabdomain.DeviceStatusRevoked {
		return collabservice.Device{}, collabservice.ErrDeviceRevoked
	}
	return collabservice.Device{}, collabservice.ErrInvariantViolation
}

func (r *collaborationRepository) ListDevices(ctx context.Context, userID int64) ([]collabservice.Device, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, last_seen_at, revoked_at, registered_at, created_at, updated_at
		FROM collaboration_devices
		WHERE user_id = $1 AND status <> 'revoked'
		ORDER BY updated_at DESC, id
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	devices := make([]collabservice.Device, 0)
	for rows.Next() {
		device, scanErr := scanCollaborationDevice(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return devices, nil
}

func (r *collaborationRepository) RenameDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	name string,
) (collabservice.Device, error) {
	row := r.db.QueryRowContext(ctx, `
		UPDATE collaboration_devices
		SET name = $3, updated_at = $4
		WHERE user_id = $1 AND id = $2 AND status <> 'revoked'
		RETURNING
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, last_seen_at, revoked_at, registered_at, created_at, updated_at
	`, userID, deviceID, name, r.now().UTC())
	device, err := scanCollaborationDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.Device{}, collabservice.ErrNotFound
	}
	return device, err
}

func (r *collaborationRepository) RevokeDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
) (collabservice.Device, error) {
	now := r.now().UTC()
	row := r.db.QueryRowContext(ctx, `
		UPDATE collaboration_devices
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, $3), updated_at = $3
		WHERE user_id = $1 AND id = $2
		RETURNING
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, last_seen_at, revoked_at, registered_at, created_at, updated_at
	`, userID, deviceID, now)
	device, err := scanCollaborationDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.Device{}, collabservice.ErrNotFound
	}
	return device, err
}

func (r *collaborationRepository) CreateCommandAndCharge(
	ctx context.Context,
	input collabservice.CreateCommandInput,
) (result collabservice.CreateCommandResult, err error) {
	if !input.Fee.IsPositive() || input.Currency != "USD" {
		return result, collabservice.ErrInvariantViolation
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	result, found, err := findExistingCollaborationCommand(ctx, tx, input.UserID, input.IdempotencyKey)
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}
	if found {
		return replayCollaborationCommand(input, result)
	}

	var balanceText string
	err = tx.QueryRowContext(ctx, `
		SELECT balance::text
		FROM users
		WHERE id = $1 AND deleted_at IS NULL AND status = 'active'
		FOR UPDATE
	`, input.UserID).Scan(&balanceText)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.CreateCommandResult{}, collabservice.ErrNotFound
	}
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}

	result, found, err = findExistingCollaborationCommand(ctx, tx, input.UserID, input.IdempotencyKey)
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}
	if found {
		return replayCollaborationCommand(input, result)
	}

	var deviceStatus collabdomain.DeviceStatus
	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM collaboration_devices
		WHERE user_id = $1 AND id = $2
		FOR UPDATE
	`, input.UserID, input.DeviceID).Scan(&deviceStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.CreateCommandResult{}, collabservice.ErrNotFound
	}
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}
	if deviceStatus == collabdomain.DeviceStatusRevoked {
		return collabservice.CreateCommandResult{}, collabservice.ErrDeviceRevoked
	}

	balanceBefore, err := decimal.NewFromString(balanceText)
	if err != nil {
		return collabservice.CreateCommandResult{}, fmt.Errorf("parse user balance: %w", collabservice.ErrInvariantViolation)
	}
	if balanceBefore.LessThan(input.Fee) {
		return collabservice.CreateCommandResult{}, &collabservice.InsufficientBalanceError{
			Available: balanceBefore,
			Required:  input.Fee,
		}
	}

	now := r.now().UTC()
	balanceAfter := balanceBefore.Sub(input.Fee)
	commandID := uuid.New()
	chargeID := uuid.New()
	feeText := input.Fee.StringFixed(8)
	beforeText := balanceBefore.StringFixed(8)
	afterText := balanceAfter.StringFixed(8)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collaboration_commands (
			id, user_id, device_id, thread_id, idempotency_key,
			prompt_sha256, prompt_bytes, status, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'accepted', $8, $9, $9)
	`, commandID, input.UserID, input.DeviceID, input.ThreadID,
		input.IdempotencyKey, input.PromptSHA256, input.PromptBytes,
		input.ExpiresAt.UTC(), now)
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}

	updateResult, err := tx.ExecContext(ctx, `
		UPDATE users
		SET balance = $2, updated_at = $3
		WHERE id = $1
	`, input.UserID, afterText, now)
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}
	updated, err := updateResult.RowsAffected()
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}
	if updated != 1 {
		return collabservice.CreateCommandResult{}, collabservice.ErrInvariantViolation
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO collaboration_charges (
			id, command_id, user_id, amount, currency, status,
			balance_before, balance_after, reason, charged_at
		) VALUES ($1, $2, $3, $4, $5, 'charged', $6, $7, $8, $9)
	`, chargeID, commandID, input.UserID, feeText, input.Currency,
		beforeText, afterText, input.Reason, now)
	if err != nil {
		return collabservice.CreateCommandResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return collabservice.CreateCommandResult{}, err
	}

	return collabservice.CreateCommandResult{
		Command: collabservice.Command{
			ID:             commandID,
			UserID:         input.UserID,
			DeviceID:       input.DeviceID,
			ThreadID:       input.ThreadID,
			IdempotencyKey: input.IdempotencyKey,
			PromptSHA256:   input.PromptSHA256,
			PromptBytes:    input.PromptBytes,
			Status:         collabdomain.CommandStatusAccepted,
			ExpiresAt:      input.ExpiresAt.UTC(),
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		Charge: collabservice.Charge{
			ID:            chargeID,
			CommandID:     commandID,
			UserID:        input.UserID,
			Amount:        input.Fee,
			Currency:      input.Currency,
			Status:        collabdomain.ChargeStatusCharged,
			BalanceBefore: balanceBefore,
			BalanceAfter:  balanceAfter,
			Reason:        input.Reason,
			ChargedAt:     now,
		},
	}, nil
}

type collaborationScanner interface {
	Scan(...any) error
}

func scanCollaborationDevice(scanner collaborationScanner) (collabservice.Device, error) {
	var (
		device          collabservice.Device
		platformVersion sql.NullString
		codexVersion    sql.NullString
		capabilities    []byte
		lastSeenAt      sql.NullTime
		revokedAt       sql.NullTime
	)
	err := scanner.Scan(
		&device.ID, &device.UserID, &device.InstallationIDHash, &device.Name,
		&device.Platform, &platformVersion, &device.CompanionVersion, &codexVersion,
		&device.ProtocolVersion, &device.Status, &capabilities, &lastSeenAt,
		&revokedAt, &device.RegisteredAt, &device.CreatedAt, &device.UpdatedAt,
	)
	if err != nil {
		return collabservice.Device{}, err
	}
	if platformVersion.Valid {
		device.PlatformVersion = &platformVersion.String
	}
	if codexVersion.Valid {
		device.CodexVersion = &codexVersion.String
	}
	if lastSeenAt.Valid {
		device.LastSeenAt = &lastSeenAt.Time
	}
	if revokedAt.Valid {
		device.RevokedAt = &revokedAt.Time
	}
	if err := json.Unmarshal(capabilities, &device.Capabilities); err != nil {
		return collabservice.Device{}, fmt.Errorf("decode collaboration capabilities: %w", err)
	}
	return device, nil
}

func findExistingCollaborationCommand(
	ctx context.Context,
	tx *sql.Tx,
	userID int64,
	idempotencyKey uuid.UUID,
) (collabservice.CreateCommandResult, bool, error) {
	var (
		result       collabservice.CreateCommandResult
		turnID       sql.NullString
		errorCode    sql.NullString
		dispatchedAt sql.NullTime
		startedAt    sql.NullTime
		completedAt  sql.NullTime
	)
	err := tx.QueryRowContext(ctx, `
		SELECT
			id, user_id, device_id, thread_id, idempotency_key,
			prompt_sha256, prompt_bytes, status, turn_id, error_code,
			expires_at, dispatched_at, started_at, completed_at, created_at, updated_at
		FROM collaboration_commands
		WHERE user_id = $1 AND idempotency_key = $2
	`, userID, idempotencyKey).Scan(
		&result.Command.ID, &result.Command.UserID, &result.Command.DeviceID,
		&result.Command.ThreadID, &result.Command.IdempotencyKey,
		&result.Command.PromptSHA256, &result.Command.PromptBytes,
		&result.Command.Status, &turnID, &errorCode, &result.Command.ExpiresAt,
		&dispatchedAt, &startedAt, &completedAt,
		&result.Command.CreatedAt, &result.Command.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.CreateCommandResult{}, false, nil
	}
	if err != nil {
		return collabservice.CreateCommandResult{}, false, err
	}
	if turnID.Valid {
		result.Command.TurnID = &turnID.String
	}
	if errorCode.Valid {
		result.Command.ErrorCode = &errorCode.String
	}
	if dispatchedAt.Valid {
		result.Command.DispatchedAt = &dispatchedAt.Time
	}
	if startedAt.Valid {
		result.Command.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		result.Command.CompletedAt = &completedAt.Time
	}

	var (
		amountText        string
		balanceBeforeText string
		balanceAfterText  string
		reason            sql.NullString
	)
	err = tx.QueryRowContext(ctx, `
		SELECT
			id, command_id, user_id, amount::text, currency, status,
			balance_before::text, balance_after::text, reason, charged_at
		FROM collaboration_charges
		WHERE user_id = $1 AND command_id = $2
	`, userID, result.Command.ID).Scan(
		&result.Charge.ID, &result.Charge.CommandID, &result.Charge.UserID,
		&amountText, &result.Charge.Currency, &result.Charge.Status,
		&balanceBeforeText, &balanceAfterText, &reason, &result.Charge.ChargedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.CreateCommandResult{}, false, collabservice.ErrInvariantViolation
	}
	if err != nil {
		return collabservice.CreateCommandResult{}, false, err
	}
	result.Charge.Amount, err = decimal.NewFromString(amountText)
	if err != nil {
		return collabservice.CreateCommandResult{}, false, collabservice.ErrInvariantViolation
	}
	result.Charge.BalanceBefore, err = decimal.NewFromString(balanceBeforeText)
	if err != nil {
		return collabservice.CreateCommandResult{}, false, collabservice.ErrInvariantViolation
	}
	result.Charge.BalanceAfter, err = decimal.NewFromString(balanceAfterText)
	if err != nil {
		return collabservice.CreateCommandResult{}, false, collabservice.ErrInvariantViolation
	}
	if reason.Valid {
		result.Charge.Reason = &reason.String
	}
	return result, true, nil
}

func replayCollaborationCommand(
	input collabservice.CreateCommandInput,
	result collabservice.CreateCommandResult,
) (collabservice.CreateCommandResult, error) {
	if result.Command.DeviceID != input.DeviceID ||
		result.Command.ThreadID != input.ThreadID ||
		result.Command.PromptSHA256 != input.PromptSHA256 ||
		result.Command.PromptBytes != input.PromptBytes {
		return collabservice.CreateCommandResult{}, collabservice.ErrIdempotencyConflict
	}
	result.Replayed = true
	return result, nil
}
