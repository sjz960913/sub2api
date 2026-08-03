CREATE TABLE IF NOT EXISTS collaboration_devices (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    installation_id_hash VARCHAR(80) NOT NULL,
    name VARCHAR(100) NOT NULL,
    platform VARCHAR(16) NOT NULL,
    platform_version VARCHAR(128) NULL,
    companion_version VARCHAR(64) NOT NULL,
    codex_version VARCHAR(64) NULL,
    protocol_version INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'offline',
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_seen_at TIMESTAMPTZ NULL,
    revoked_at TIMESTAMPTZ NULL,
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT collaboration_devices_platform_check
        CHECK (platform IN ('windows', 'macos', 'linux')),
    CONSTRAINT collaboration_devices_status_check
        CHECK (status IN ('offline', 'online', 'degraded', 'revoked')),
    CONSTRAINT collaboration_devices_installation_hash_check
        CHECK (installation_id_hash ~ '^sha256:[0-9a-f]{64}$'),
    CONSTRAINT collaboration_devices_revoked_at_check
        CHECK ((status = 'revoked' AND revoked_at IS NOT NULL) OR status <> 'revoked'),
    CONSTRAINT collaboration_devices_user_installation_key
        UNIQUE (user_id, installation_id_hash)
);

CREATE INDEX IF NOT EXISTS collaboration_devices_user_status_idx
    ON collaboration_devices (user_id, status);
CREATE INDEX IF NOT EXISTS collaboration_devices_user_updated_at_idx
    ON collaboration_devices (user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS collaboration_devices_last_seen_at_idx
    ON collaboration_devices (last_seen_at);
CREATE INDEX IF NOT EXISTS collaboration_devices_revoked_at_idx
    ON collaboration_devices (revoked_at);

CREATE TABLE IF NOT EXISTS collaboration_sync_requests (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    device_id UUID NOT NULL REFERENCES collaboration_devices(id) ON DELETE RESTRICT,
    idempotency_key UUID NOT NULL,
    kind VARCHAR(32) NOT NULL,
    thread_id VARCHAR(512) NULL,
    cursor VARCHAR(1024) NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    error_code VARCHAR(128) NULL,
    snapshot_version BIGINT NULL,
    result_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT collaboration_sync_requests_kind_check
        CHECK (kind IN ('session_list', 'thread_snapshot')),
    CONSTRAINT collaboration_sync_requests_status_check
        CHECK (status IN ('pending', 'running', 'completed', 'failed', 'expired')),
    CONSTRAINT collaboration_sync_requests_thread_check
        CHECK ((kind = 'thread_snapshot' AND thread_id IS NOT NULL) OR kind = 'session_list'),
    CONSTRAINT collaboration_sync_requests_result_count_check
        CHECK (result_count >= 0),
    CONSTRAINT collaboration_sync_requests_user_idempotency_key
        UNIQUE (user_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS collaboration_sync_requests_user_device_created_idx
    ON collaboration_sync_requests (user_id, device_id, created_at DESC);
CREATE INDEX IF NOT EXISTS collaboration_sync_requests_user_id_idx
    ON collaboration_sync_requests (user_id, id);
CREATE INDEX IF NOT EXISTS collaboration_sync_requests_status_expires_idx
    ON collaboration_sync_requests (status, expires_at);

CREATE TABLE IF NOT EXISTS collaboration_commands (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    device_id UUID NOT NULL REFERENCES collaboration_devices(id) ON DELETE RESTRICT,
    thread_id VARCHAR(512) NOT NULL,
    idempotency_key UUID NOT NULL,
    prompt_sha256 CHAR(64) NOT NULL,
    prompt_bytes INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'accepted',
    turn_id VARCHAR(512) NULL,
    error_code VARCHAR(128) NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    dispatched_at TIMESTAMPTZ NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT collaboration_commands_prompt_hash_check
        CHECK (prompt_sha256 ~ '^[0-9a-f]{64}$'),
    CONSTRAINT collaboration_commands_prompt_bytes_check
        CHECK (prompt_bytes > 0),
    CONSTRAINT collaboration_commands_status_check
        CHECK (status IN ('accepted', 'dispatched', 'started', 'completed', 'failed', 'expired')),
    CONSTRAINT collaboration_commands_user_idempotency_key
        UNIQUE (user_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS collaboration_commands_user_device_created_idx
    ON collaboration_commands (user_id, device_id, created_at DESC);
CREATE INDEX IF NOT EXISTS collaboration_commands_user_id_idx
    ON collaboration_commands (user_id, id);
CREATE INDEX IF NOT EXISTS collaboration_commands_thread_status_idx
    ON collaboration_commands (user_id, device_id, thread_id, status);
CREATE INDEX IF NOT EXISTS collaboration_commands_status_expires_idx
    ON collaboration_commands (status, expires_at);

CREATE TABLE IF NOT EXISTS collaboration_charges (
    id UUID PRIMARY KEY,
    command_id UUID NOT NULL REFERENCES collaboration_commands(id) ON DELETE RESTRICT,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    amount NUMERIC(20,8) NOT NULL,
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    status VARCHAR(16) NOT NULL DEFAULT 'charged',
    balance_before NUMERIC(20,8) NOT NULL,
    balance_after NUMERIC(20,8) NOT NULL,
    reason VARCHAR(128) NULL,
    charged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT collaboration_charges_command_key UNIQUE (command_id),
    CONSTRAINT collaboration_charges_amount_check CHECK (amount > 0),
    CONSTRAINT collaboration_charges_currency_check CHECK (currency = 'USD'),
    CONSTRAINT collaboration_charges_status_check CHECK (status = 'charged'),
    CONSTRAINT collaboration_charges_balance_check
        CHECK (balance_before >= 0 AND balance_after >= 0 AND balance_before - amount = balance_after)
);

CREATE INDEX IF NOT EXISTS collaboration_charges_user_charged_at_idx
    ON collaboration_charges (user_id, charged_at DESC);
