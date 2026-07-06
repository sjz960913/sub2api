#!/bin/bash
set -euo pipefail

DATA_DIR="${DATA_DIR:-/app/data}"
PGDATA="${PGDATA:-/var/lib/postgresql/data}"
REDIS_DATA_DIR="${REDIS_DATA_DIR:-/var/lib/redis}"
PERSIST_ENV="${DATA_DIR}/allinone.env"

mkdir -p "${DATA_DIR}" "${PGDATA}" "${REDIS_DATA_DIR}" /run/postgresql
chown -R sub2api:sub2api "${DATA_DIR}" /app/resources 2>/dev/null || true
chown -R postgres:postgres /var/lib/postgresql /run/postgresql
chown -R redis:redis "${REDIS_DATA_DIR}"
chmod 2775 /run/postgresql

generate_secret() {
    openssl rand -hex "${1:-32}"
}

write_persisted_env() {
    local key="$1"
    local value="$2"

    touch "${PERSIST_ENV}"
    chmod 600 "${PERSIST_ENV}"
    if grep -q "^${key}=" "${PERSIST_ENV}"; then
        awk -v key="${key}" -v value="${value}" '
            $0 ~ "^" key "=" {
                print key "=" value
                next
            }
            { print }
        ' "${PERSIST_ENV}" > "${PERSIST_ENV}.tmp"
        mv "${PERSIST_ENV}.tmp" "${PERSIST_ENV}"
    else
        printf '%s=%s\n' "${key}" "${value}" >> "${PERSIST_ENV}"
    fi
}

if [ -f "${PERSIST_ENV}" ]; then
    set -a
    # shellcheck disable=SC1090
    . "${PERSIST_ENV}"
    set +a
fi

if [ -z "${POSTGRES_PASSWORD:-}" ]; then
    POSTGRES_PASSWORD="$(generate_secret 32)"
    write_persisted_env "POSTGRES_PASSWORD" "${POSTGRES_PASSWORD}"
fi

if [ -z "${ADMIN_PASSWORD:-}" ]; then
    ADMIN_PASSWORD="$(generate_secret 16)"
    write_persisted_env "ADMIN_PASSWORD" "${ADMIN_PASSWORD}"
fi

if [ -z "${JWT_SECRET:-}" ]; then
    JWT_SECRET="$(generate_secret 32)"
    write_persisted_env "JWT_SECRET" "${JWT_SECRET}"
fi

if [ -z "${TOTP_ENCRYPTION_KEY:-}" ]; then
    TOTP_ENCRYPTION_KEY="$(generate_secret 32)"
    write_persisted_env "TOTP_ENCRYPTION_KEY" "${TOTP_ENCRYPTION_KEY}"
fi

export DATA_DIR PGDATA
export AUTO_SETUP="${AUTO_SETUP:-true}"
export SERVER_HOST="${SERVER_HOST:-0.0.0.0}"
export SERVER_PORT="${SERVER_PORT:-8080}"
export SERVER_MODE="${SERVER_MODE:-release}"
export RUN_MODE="${RUN_MODE:-standard}"
export DATABASE_HOST="${DATABASE_HOST:-127.0.0.1}"
export DATABASE_PORT="${DATABASE_PORT:-5432}"
export DATABASE_USER="${DATABASE_USER:-${POSTGRES_USER:-sub2api}}"
export DATABASE_PASSWORD="${POSTGRES_PASSWORD}"
export DATABASE_DBNAME="${DATABASE_DBNAME:-${POSTGRES_DB:-sub2api}}"
export DATABASE_SSLMODE="${DATABASE_SSLMODE:-disable}"
export REDIS_HOST="${REDIS_HOST:-127.0.0.1}"
export REDIS_PORT="${REDIS_PORT:-6379}"
export REDIS_PASSWORD="${REDIS_PASSWORD:-}"
export REDIS_DB="${REDIS_DB:-0}"
export REDIS_ENABLE_TLS="${REDIS_ENABLE_TLS:-false}"
export ADMIN_EMAIL="${ADMIN_EMAIL:-admin@sub2api.local}"
export ADMIN_PASSWORD
export JWT_SECRET
export JWT_EXPIRE_HOUR="${JWT_EXPIRE_HOUR:-24}"
export TOTP_ENCRYPTION_KEY
export TZ="${TZ:-Asia/Shanghai}"

POSTGRES_BIN_DIR="$(find /usr/lib/postgresql -maxdepth 3 -type f -name postgres -printf '%h\n' | sort -V | tail -1)"
export PATH="${POSTGRES_BIN_DIR}:${PATH}"

start_postgres() {
    gosu postgres postgres -D "${PGDATA}" -c listen_addresses=127.0.0.1 &
    POSTGRES_PID=$!

    for _ in $(seq 1 60); do
        if gosu postgres pg_isready -h 127.0.0.1 -p "${DATABASE_PORT}" -U postgres >/dev/null 2>&1; then
            return 0
        fi
        sleep 1
    done

    echo "PostgreSQL did not become ready in time" >&2
    return 1
}

initialize_postgres() {
    if [ -s "${PGDATA}/PG_VERSION" ]; then
        return 0
    fi

    echo "Initializing PostgreSQL data directory..."
    pwfile="$(mktemp)"
    printf '%s\n' "${POSTGRES_PASSWORD}" > "${pwfile}"
    chown postgres:postgres "${pwfile}"
    chmod 600 "${pwfile}"

    gosu postgres initdb \
        -D "${PGDATA}" \
        --username=postgres \
        --pwfile="${pwfile}" \
        --auth-local=trust \
        --auth-host=scram-sha-256
    rm -f "${pwfile}"

    {
        echo "listen_addresses = '127.0.0.1'"
        echo "port = ${DATABASE_PORT}"
        echo "max_connections = ${POSTGRES_MAX_CONNECTIONS:-1024}"
    } >> "${PGDATA}/postgresql.conf"
}

ensure_database() {
    gosu postgres psql -v ON_ERROR_STOP=1 \
        --username postgres \
        --dbname postgres \
        -v app_user="${DATABASE_USER}" \
        -v app_password="${POSTGRES_PASSWORD}" \
        -v app_db="${DATABASE_DBNAME}" <<'SQL'
SELECT format('CREATE USER %I WITH PASSWORD %L', :'app_user', :'app_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = :'app_user')\gexec
SELECT format('CREATE DATABASE %I OWNER %I', :'app_db', :'app_user')
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = :'app_db')\gexec
GRANT ALL PRIVILEGES ON DATABASE :"app_db" TO :"app_user";
SQL
}

start_redis() {
    redis_args=(
        "--bind" "127.0.0.1"
        "--port" "${REDIS_PORT}"
        "--dir" "${REDIS_DATA_DIR}"
        "--save" "60" "1"
        "--appendonly" "yes"
        "--appendfsync" "everysec"
        "--daemonize" "yes"
    )

    if [ -n "${REDIS_PASSWORD}" ]; then
        redis_args+=("--requirepass" "${REDIS_PASSWORD}")
    fi

    gosu redis redis-server "${redis_args[@]}"

    for _ in $(seq 1 30); do
        if [ -n "${REDIS_PASSWORD}" ]; then
            if redis-cli -h 127.0.0.1 -p "${REDIS_PORT}" -a "${REDIS_PASSWORD}" ping >/dev/null 2>&1; then
                return 0
            fi
        else
            if redis-cli -h 127.0.0.1 -p "${REDIS_PORT}" ping >/dev/null 2>&1; then
                return 0
            fi
        fi
        sleep 1
    done

    echo "Redis did not become ready in time" >&2
    return 1
}

shutdown() {
    if [ -n "${APP_PID:-}" ] && kill -0 "${APP_PID}" >/dev/null 2>&1; then
        kill "${APP_PID}" >/dev/null 2>&1 || true
    fi
    if [ -n "${POSTGRES_PID:-}" ] && kill -0 "${POSTGRES_PID}" >/dev/null 2>&1; then
        gosu postgres pg_ctl -D "${PGDATA}" -m fast -w stop >/dev/null 2>&1 || true
    fi
    redis-cli -h 127.0.0.1 -p "${REDIS_PORT}" ${REDIS_PASSWORD:+-a "${REDIS_PASSWORD}"} shutdown >/dev/null 2>&1 || true
}

trap shutdown TERM INT

initialize_postgres
start_postgres
ensure_database
start_redis

echo "Sub2API all-in-one container is starting."
echo "Web UI: http://localhost:${SERVER_PORT}"
echo "Admin email: ${ADMIN_EMAIL}"
echo "Admin password: ${ADMIN_PASSWORD}"
echo "Persisted generated credentials: ${PERSIST_ENV}"

gosu sub2api /app/sub2api &
APP_PID=$!
wait "${APP_PID}"
