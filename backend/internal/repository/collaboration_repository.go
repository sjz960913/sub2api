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

func (r *collaborationRepository) GetDevice(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
) (collabservice.Device, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, last_seen_at, revoked_at, registered_at, created_at, updated_at
		FROM collaboration_devices
		WHERE user_id = $1 AND id = $2
	`, userID, deviceID)
	device, err := scanCollaborationDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.Device{}, collabservice.ErrNotFound
	}
	return device, err
}

func (r *collaborationRepository) UpdateDevicePresence(
	ctx context.Context,
	userID int64,
	deviceID uuid.UUID,
	status collabdomain.DeviceStatus,
	seenAt time.Time,
) error {
	if status != collabdomain.DeviceStatusOffline && status != collabdomain.DeviceStatusOnline && status != collabdomain.DeviceStatusDegraded {
		return collabservice.ErrInvariantViolation
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE collaboration_devices
		SET status = $3, last_seen_at = $4, updated_at = $4
		WHERE user_id = $1 AND id = $2 AND status <> 'revoked'
	`, userID, deviceID, status, seenAt.UTC())
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return collabservice.ErrNotFound
	}
	return nil
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
) (device collabservice.Device, err error) {
	now := r.now().UTC()
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return collabservice.Device{}, err
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `
		UPDATE collaboration_devices
		SET status = 'revoked', revoked_at = COALESCE(revoked_at, $3), updated_at = $3
		WHERE user_id = $1 AND id = $2
		RETURNING
			id, user_id, installation_id_hash, name, platform, platform_version,
			companion_version, codex_version, protocol_version, status,
			capabilities, last_seen_at, revoked_at, registered_at, created_at, updated_at
	`, userID, deviceID, now)
	device, err = scanCollaborationDevice(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.Device{}, collabservice.ErrNotFound
	}
	if err != nil {
		return collabservice.Device{}, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE collaboration_commands
		SET status = 'failed', error_code = 'device_revoked',
			completed_at = COALESCE(completed_at, $3), updated_at = $3
		WHERE user_id = $1 AND device_id = $2
			AND status IN ('accepted', 'dispatched', 'started')
	`, userID, deviceID, now); err != nil {
		return collabservice.Device{}, err
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE collaboration_sync_requests
		SET status = 'failed', error_code = 'device_revoked',
			completed_at = COALESCE(completed_at, $3), updated_at = $3
		WHERE user_id = $1 AND device_id = $2
			AND status IN ('pending', 'running')
	`, userID, deviceID, now); err != nil {
		return collabservice.Device{}, err
	}
	if err = tx.Commit(); err != nil {
		return collabservice.Device{}, err
	}
	return device, nil
}

func (r *collaborationRepository) CreateSync(
	ctx context.Context,
	input collabservice.CreateSyncInput,
) (collabservice.CreateSyncResult, error) {
	now := r.now().UTC()
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO collaboration_sync_requests (
			id, user_id, device_id, idempotency_key, request_sha256,
			kind, thread_id, cursor, status, expires_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending', $9, $10, $10)
		ON CONFLICT (user_id, idempotency_key) DO NOTHING
		RETURNING
			id, user_id, device_id, idempotency_key, request_sha256,
			kind, thread_id, cursor, status, error_code, snapshot_version,
			result_count, expires_at, completed_at, created_at, updated_at
	`, uuid.New(), input.UserID, input.DeviceID, input.IdempotencyKey,
		input.RequestSHA256, input.Kind, input.ThreadID, input.Cursor,
		input.ExpiresAt.UTC(), now)
	syncRequest, err := scanCollaborationSync(row)
	if err == nil {
		return collabservice.CreateSyncResult{Sync: syncRequest}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return collabservice.CreateSyncResult{}, err
	}

	existing, err := r.getSyncByIdempotencyKey(ctx, input.UserID, input.IdempotencyKey)
	if err != nil {
		return collabservice.CreateSyncResult{}, err
	}
	if existing.DeviceID != input.DeviceID || existing.RequestSHA256 != input.RequestSHA256 ||
		existing.Kind != input.Kind || !equalOptionalString(existing.ThreadID, input.ThreadID) ||
		!equalOptionalString(existing.Cursor, input.Cursor) {
		return collabservice.CreateSyncResult{}, collabservice.ErrIdempotencyConflict
	}
	return collabservice.CreateSyncResult{Sync: existing, Replayed: true}, nil
}

func (r *collaborationRepository) GetSync(
	ctx context.Context,
	userID int64,
	syncID uuid.UUID,
) (collabservice.SyncRequest, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, user_id, device_id, idempotency_key, request_sha256,
			kind, thread_id, cursor, status, error_code, snapshot_version,
			result_count, expires_at, completed_at, created_at, updated_at
		FROM collaboration_sync_requests
		WHERE user_id = $1 AND id = $2
	`, userID, syncID)
	syncRequest, err := scanCollaborationSync(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.SyncRequest{}, collabservice.ErrNotFound
	}
	return syncRequest, err
}

func (r *collaborationRepository) TransitionSync(
	ctx context.Context,
	input collabservice.SyncTransitionInput,
) (collabservice.SyncRequest, error) {
	switch input.Status {
	case collabdomain.SyncStatusRunning:
	case collabdomain.SyncStatusCompleted, collabdomain.SyncStatusFailed:
	default:
		return collabservice.SyncRequest{}, collabservice.ErrInvalidTransition
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE collaboration_sync_requests
		SET
			status = $4,
			error_code = CASE WHEN $4 = 'failed' THEN $5 ELSE NULL END,
			snapshot_version = CASE WHEN $4 = 'completed' THEN $6 ELSE snapshot_version END,
			result_count = CASE WHEN $4 = 'completed' THEN $7 ELSE result_count END,
			completed_at = CASE WHEN $4 IN ('completed', 'failed') THEN $8 ELSE completed_at END,
			updated_at = $8
		WHERE user_id = $1 AND device_id = $2 AND id = $3
			AND (($4 = 'running' AND status = 'pending')
				OR ($4 IN ('completed', 'failed') AND status IN ('pending', 'running')))
		RETURNING
			id, user_id, device_id, idempotency_key, request_sha256,
			kind, thread_id, cursor, status, error_code, snapshot_version,
			result_count, expires_at, completed_at, created_at, updated_at
	`, input.UserID, input.DeviceID, input.SyncID,
		input.Status, input.ErrorCode, input.SnapshotVersion, input.ResultCount,
		input.OccurredAt.UTC())
	syncRequest, err := scanCollaborationSync(row)
	if err == nil {
		return syncRequest, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return collabservice.SyncRequest{}, err
	}
	current, err := r.GetSync(ctx, input.UserID, input.SyncID)
	if err != nil {
		return collabservice.SyncRequest{}, err
	}
	if current.DeviceID != input.DeviceID {
		return collabservice.SyncRequest{}, collabservice.ErrNotFound
	}
	if current.Status == input.Status {
		if input.Status == collabdomain.SyncStatusCompleted &&
			(!equalOptionalInt64(current.SnapshotVersion, input.SnapshotVersion) || current.ResultCount != input.ResultCount) {
			return collabservice.SyncRequest{}, collabservice.ErrInvalidTransition
		}
		if input.Status == collabdomain.SyncStatusFailed && !equalOptionalString(current.ErrorCode, input.ErrorCode) {
			return collabservice.SyncRequest{}, collabservice.ErrInvalidTransition
		}
		return current, nil
	}
	return collabservice.SyncRequest{}, collabservice.ErrInvalidTransition
}

func (r *collaborationRepository) getSyncByIdempotencyKey(
	ctx context.Context,
	userID int64,
	idempotencyKey uuid.UUID,
) (collabservice.SyncRequest, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, user_id, device_id, idempotency_key, request_sha256,
			kind, thread_id, cursor, status, error_code, snapshot_version,
			result_count, expires_at, completed_at, created_at, updated_at
		FROM collaboration_sync_requests
		WHERE user_id = $1 AND idempotency_key = $2
	`, userID, idempotencyKey)
	syncRequest, err := scanCollaborationSync(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.SyncRequest{}, collabservice.ErrNotFound
	}
	return syncRequest, err
}

func (r *collaborationRepository) GetCommand(
	ctx context.Context,
	userID int64,
	commandID uuid.UUID,
) (collabservice.Command, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT
			id, user_id, device_id, thread_id, idempotency_key,
			prompt_sha256, prompt_bytes, status, turn_id, error_code,
			expires_at, dispatched_at, started_at, completed_at, created_at, updated_at
		FROM collaboration_commands
		WHERE user_id = $1 AND id = $2
	`, userID, commandID)
	command, err := scanCollaborationCommand(row)
	if errors.Is(err, sql.ErrNoRows) {
		return collabservice.Command{}, collabservice.ErrNotFound
	}
	return command, err
}

func (r *collaborationRepository) TransitionCommand(
	ctx context.Context,
	input collabservice.CommandTransitionInput,
) (collabservice.Command, error) {
	switch input.Status {
	case collabdomain.CommandStatusDispatched:
	case collabdomain.CommandStatusStarted:
	case collabdomain.CommandStatusCompleted:
	case collabdomain.CommandStatusFailed:
	default:
		return collabservice.Command{}, collabservice.ErrInvalidTransition
	}
	row := r.db.QueryRowContext(ctx, `
		UPDATE collaboration_commands
		SET
			status = $4,
			turn_id = CASE WHEN $4 = 'started' THEN COALESCE($5, turn_id) ELSE turn_id END,
			error_code = CASE WHEN $4 = 'failed' THEN $6 ELSE NULL END,
			dispatched_at = CASE WHEN $4 = 'dispatched' THEN COALESCE(dispatched_at, $7) ELSE dispatched_at END,
			started_at = CASE WHEN $4 = 'started' THEN COALESCE(started_at, $7) ELSE started_at END,
			completed_at = CASE WHEN $4 IN ('completed', 'failed') THEN COALESCE(completed_at, $7) ELSE completed_at END,
			updated_at = $7
		WHERE user_id = $1 AND device_id = $2 AND id = $3
			AND (($4 = 'dispatched' AND status = 'accepted')
				OR ($4 = 'started' AND status IN ('accepted', 'dispatched'))
				OR ($4 = 'completed' AND status = 'started')
				OR ($4 = 'failed' AND status IN ('accepted', 'dispatched', 'started')))
		RETURNING
			id, user_id, device_id, thread_id, idempotency_key,
			prompt_sha256, prompt_bytes, status, turn_id, error_code,
			expires_at, dispatched_at, started_at, completed_at, created_at, updated_at
	`, input.UserID, input.DeviceID, input.CommandID,
		input.Status, input.TurnID, input.ErrorCode, input.OccurredAt.UTC())
	command, err := scanCollaborationCommand(row)
	if err == nil {
		return command, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return collabservice.Command{}, err
	}
	current, err := r.GetCommand(ctx, input.UserID, input.CommandID)
	if err != nil {
		return collabservice.Command{}, err
	}
	if current.DeviceID != input.DeviceID {
		return collabservice.Command{}, collabservice.ErrNotFound
	}
	if current.Status == input.Status {
		if input.Status == collabdomain.CommandStatusStarted && !equalOptionalString(current.TurnID, input.TurnID) {
			return collabservice.Command{}, collabservice.ErrInvalidTransition
		}
		if input.Status == collabdomain.CommandStatusFailed && !equalOptionalString(current.ErrorCode, input.ErrorCode) {
			return collabservice.Command{}, collabservice.ErrInvalidTransition
		}
		return current, nil
	}
	return collabservice.Command{}, collabservice.ErrInvalidTransition
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

func (r *collaborationRepository) ExpirePending(
	ctx context.Context,
	now time.Time,
) (result collabservice.SweepResult, err error) {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	commandResult, err := tx.ExecContext(ctx, `
		UPDATE collaboration_commands
		SET status = 'expired', error_code = COALESCE(error_code, 'expired'), updated_at = $1
		WHERE status IN ('accepted', 'dispatched') AND expires_at <= $1
	`, now.UTC())
	if err != nil {
		return result, err
	}
	result.ExpiredCommands, err = commandResult.RowsAffected()
	if err != nil {
		return collabservice.SweepResult{}, err
	}

	syncResult, err := tx.ExecContext(ctx, `
		UPDATE collaboration_sync_requests
		SET status = 'expired', error_code = COALESCE(error_code, 'expired'), updated_at = $1
		WHERE status IN ('pending', 'running') AND expires_at <= $1
	`, now.UTC())
	if err != nil {
		return collabservice.SweepResult{}, err
	}
	result.ExpiredSyncs, err = syncResult.RowsAffected()
	if err != nil {
		return collabservice.SweepResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return collabservice.SweepResult{}, err
	}
	return result, nil
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

func scanCollaborationSync(scanner collaborationScanner) (collabservice.SyncRequest, error) {
	var (
		syncRequest     collabservice.SyncRequest
		threadID        sql.NullString
		cursor          sql.NullString
		errorCode       sql.NullString
		snapshotVersion sql.NullInt64
		completedAt     sql.NullTime
	)
	err := scanner.Scan(
		&syncRequest.ID, &syncRequest.UserID, &syncRequest.DeviceID,
		&syncRequest.IdempotencyKey, &syncRequest.RequestSHA256,
		&syncRequest.Kind, &threadID, &cursor, &syncRequest.Status,
		&errorCode, &snapshotVersion, &syncRequest.ResultCount,
		&syncRequest.ExpiresAt, &completedAt, &syncRequest.CreatedAt,
		&syncRequest.UpdatedAt,
	)
	if err != nil {
		return collabservice.SyncRequest{}, err
	}
	if threadID.Valid {
		syncRequest.ThreadID = &threadID.String
	}
	if cursor.Valid {
		syncRequest.Cursor = &cursor.String
	}
	if errorCode.Valid {
		syncRequest.ErrorCode = &errorCode.String
	}
	if snapshotVersion.Valid {
		syncRequest.SnapshotVersion = &snapshotVersion.Int64
	}
	if completedAt.Valid {
		syncRequest.CompletedAt = &completedAt.Time
	}
	return syncRequest, nil
}

func scanCollaborationCommand(scanner collaborationScanner) (collabservice.Command, error) {
	var (
		command      collabservice.Command
		turnID       sql.NullString
		errorCode    sql.NullString
		dispatchedAt sql.NullTime
		startedAt    sql.NullTime
		completedAt  sql.NullTime
	)
	err := scanner.Scan(
		&command.ID, &command.UserID, &command.DeviceID, &command.ThreadID,
		&command.IdempotencyKey, &command.PromptSHA256, &command.PromptBytes,
		&command.Status, &turnID, &errorCode, &command.ExpiresAt,
		&dispatchedAt, &startedAt, &completedAt, &command.CreatedAt,
		&command.UpdatedAt,
	)
	if err != nil {
		return collabservice.Command{}, err
	}
	if turnID.Valid {
		command.TurnID = &turnID.String
	}
	if errorCode.Valid {
		command.ErrorCode = &errorCode.String
	}
	if dispatchedAt.Valid {
		command.DispatchedAt = &dispatchedAt.Time
	}
	if startedAt.Valid {
		command.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		command.CompletedAt = &completedAt.Time
	}
	return command, nil
}

func equalOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func equalOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
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
