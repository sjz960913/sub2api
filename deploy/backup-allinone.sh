#!/bin/bash
# =============================================================================
# Sub2API all-in-one backup script
# =============================================================================
# Run this script from the deployment directory that contains docker-compose.yml
# and .env. It creates a single archive with:
#   - deployment config files
#   - PostgreSQL logical dump
#   - Redis dump.rdb snapshot when available
#   - app/PostgreSQL/Redis volume tarballs exported from the running container
#
# Usage:
#   ./backup-allinone.sh
#   BACKUP_DIR=/path/to/backups ./backup-allinone.sh
#   CONTAINER_NAME=sub2api-allinone ./backup-allinone.sh
# =============================================================================

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

BACKUP_DIR="${BACKUP_DIR:-backups}"
CONTAINER_NAME="${CONTAINER_NAME:-sub2api-allinone}"
TIMESTAMP="${TIMESTAMP:-$(date +%Y%m%d-%H%M%S)}"
BACKUP_NAME="${BACKUP_NAME:-sub2api-full-${TIMESTAMP}}"
ARCHIVE_PATH="${BACKUP_DIR}/${BACKUP_NAME}.tar.gz"

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

command_exists() {
    command -v "$1" >/dev/null 2>&1
}

cleanup() {
    if [ -n "${TMP_DIR:-}" ] && [ -d "${TMP_DIR}" ]; then
        rm -rf "${TMP_DIR}"
    fi
}

trap cleanup EXIT

require_command() {
    local cmd="$1"
    if ! command_exists "${cmd}"; then
        print_error "${cmd} is not installed or not in PATH."
        exit 1
    fi
}

copy_if_exists() {
    local src="$1"
    local dst_dir="$2"
    if [ -e "${src}" ]; then
        cp -a "${src}" "${dst_dir}/"
    fi
}

container_is_running() {
    local status
    status="$(docker inspect -f '{{.State.Running}}' "${CONTAINER_NAME}" 2>/dev/null || true)"
    [ "${status}" = "true" ]
}

write_metadata() {
    local metadata_file="$1"

    {
        echo "backup_name=${BACKUP_NAME}"
        echo "created_at=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
        echo "host=$(hostname 2>/dev/null || echo unknown)"
        echo "deployment_dir=$(pwd)"
        echo "container=${CONTAINER_NAME}"
        docker inspect -f 'container_image={{.Config.Image}}' "${CONTAINER_NAME}" 2>/dev/null || true
        docker inspect -f 'container_id={{.Id}}' "${CONTAINER_NAME}" 2>/dev/null || true
        docker inspect -f 'container_status={{.State.Status}}' "${CONTAINER_NAME}" 2>/dev/null || true
    } > "${metadata_file}"
}

dump_postgres() {
    local output_file="$1"

    print_info "Exporting PostgreSQL logical backup..."
    docker exec "${CONTAINER_NAME}" sh -lc '
        set -e
        db_user="${DATABASE_USER:-${POSTGRES_USER:-sub2api}}"
        db_name="${DATABASE_DBNAME:-${POSTGRES_DB:-sub2api}}"
        db_port="${DATABASE_PORT:-5432}"
        if command -v gosu >/dev/null 2>&1; then
            gosu postgres pg_dump -p "${db_port}" -U "${db_user}" -d "${db_name}" -Fc
        else
            pg_dump -p "${db_port}" -U "${db_user}" -d "${db_name}" -Fc
        fi
    ' > "${output_file}"
    print_success "PostgreSQL dump saved."
}

dump_redis() {
    local output_file="$1"

    print_info "Saving Redis snapshot..."
    docker exec "${CONTAINER_NAME}" sh -lc '
        set -e
        redis_port="${REDIS_PORT:-6379}"
        if [ -n "${REDIS_PASSWORD:-}" ]; then
            redis-cli --no-auth-warning -h 127.0.0.1 -p "${redis_port}" -a "${REDIS_PASSWORD}" SAVE
        else
            redis-cli -h 127.0.0.1 -p "${redis_port}" SAVE
        fi
    ' >/dev/null

    if docker cp "${CONTAINER_NAME}:/var/lib/redis/dump.rdb" "${output_file}" >/dev/null 2>&1; then
        print_success "Redis dump saved."
    else
        print_warning "Redis dump.rdb was not found; Redis volume tarball will still be included."
    fi
}

archive_container_dir() {
    local container_dir="$1"
    local output_file="$2"
    local label="$3"

    print_info "Archiving ${label} from ${container_dir}..."
    if docker exec "${CONTAINER_NAME}" sh -lc "test -d '${container_dir}'" >/dev/null 2>&1; then
        if docker exec "${CONTAINER_NAME}" tar -C "${container_dir}" -czf - . > "${output_file}"; then
            print_success "${label} archive saved."
        else
            rm -f "${output_file}"
            print_warning "Failed to archive ${label}; continuing with logical dumps."
        fi
    else
        print_warning "${container_dir} does not exist in container; skipped ${label}."
    fi
}

main() {
    echo ""
    echo "=========================================="
    echo "  Sub2API All-in-One Backup"
    echo "=========================================="
    echo ""

    require_command docker
    require_command tar

    if ! container_is_running; then
        print_error "Container '${CONTAINER_NAME}' is not running."
        print_error "Start it first, or set CONTAINER_NAME to the running all-in-one container."
        exit 1
    fi

    mkdir -p "${BACKUP_DIR}"
    TMP_DIR="$(mktemp -d)"
    PAYLOAD_DIR="${TMP_DIR}/${BACKUP_NAME}"
    mkdir -p "${PAYLOAD_DIR}/config" "${PAYLOAD_DIR}/dumps" "${PAYLOAD_DIR}/volumes"

    print_info "Collecting deployment config files..."
    copy_if_exists ".env" "${PAYLOAD_DIR}/config"
    copy_if_exists ".env.example" "${PAYLOAD_DIR}/config"
    copy_if_exists ".env.allinone" "${PAYLOAD_DIR}/config"
    copy_if_exists ".env.allinone.example" "${PAYLOAD_DIR}/config"
    copy_if_exists "docker-compose.yml" "${PAYLOAD_DIR}/config"
    copy_if_exists "docker-compose.allinone.yml" "${PAYLOAD_DIR}/config"
    copy_if_exists "docker-deploy.sh" "${PAYLOAD_DIR}/config"
    copy_if_exists "backup-allinone.sh" "${PAYLOAD_DIR}/config"

    write_metadata "${PAYLOAD_DIR}/metadata.txt"
    dump_postgres "${PAYLOAD_DIR}/dumps/postgres.dump"
    dump_redis "${PAYLOAD_DIR}/dumps/redis-dump.rdb"

    archive_container_dir "/app/data" "${PAYLOAD_DIR}/volumes/data.tar.gz" "application data"
    archive_container_dir "/var/lib/postgresql/data" "${PAYLOAD_DIR}/volumes/postgres_data.tar.gz" "PostgreSQL data directory"
    archive_container_dir "/var/lib/redis" "${PAYLOAD_DIR}/volumes/redis_data.tar.gz" "Redis data directory"

    print_info "Creating final backup archive..."
    tar -C "${TMP_DIR}" -czf "${ARCHIVE_PATH}" "${BACKUP_NAME}"
    chmod 600 "${ARCHIVE_PATH}"

    echo ""
    echo "=========================================="
    echo "  Backup Complete"
    echo "=========================================="
    echo ""
    print_success "Backup archive: ${ARCHIVE_PATH}"
    echo ""
    echo "Archive contents:"
    echo "  config/                 Deployment .env and Compose files"
    echo "  dumps/postgres.dump     PostgreSQL logical backup (pg_restore -Fc)"
    echo "  dumps/redis-dump.rdb    Redis snapshot when available"
    echo "  volumes/data.tar.gz     /app/data volume contents"
    echo "  volumes/postgres_data.tar.gz"
    echo "  volumes/redis_data.tar.gz"
    echo "  metadata.txt"
    echo ""
    echo "Quick restore outline on a new server:"
    echo "  1. Extract this archive."
    echo "  2. Copy config/.env and config/docker-compose.yml into a new deployment directory."
    echo "  3. Extract volume tarballs into data/, postgres_data/, and redis_data/ when doing a physical restore."
    echo "  4. Or start a fresh container and restore dumps/postgres.dump with pg_restore."
    echo ""
}

main "$@"
