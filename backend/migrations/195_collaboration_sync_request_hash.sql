ALTER TABLE collaboration_sync_requests
    ADD COLUMN IF NOT EXISTS request_sha256 CHAR(64);

UPDATE collaboration_sync_requests
SET request_sha256 = encode(
    sha256(
        convert_to(
            concat_ws(
                ':',
                device_id::text,
                kind,
                COALESCE(thread_id, ''),
                COALESCE(cursor, '')
            ),
            'UTF8'
        )
    ),
    'hex'
)
WHERE request_sha256 IS NULL;

ALTER TABLE collaboration_sync_requests
    ALTER COLUMN request_sha256 SET NOT NULL;

ALTER TABLE collaboration_sync_requests
    DROP CONSTRAINT IF EXISTS collaboration_sync_requests_request_hash_check;

ALTER TABLE collaboration_sync_requests
    ADD CONSTRAINT collaboration_sync_requests_request_hash_check
        CHECK (request_sha256 ~ '^[0-9a-f]{64}$');
