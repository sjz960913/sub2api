#!/bin/bash
set -euo pipefail

SUB2API_DEPLOY_TEST_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
export SUB2API_DEPLOY_TEST_ROOT
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT

# Serve the tracked deployment templates without making network requests.
curl() {
    [[ $# -eq 4 && "$1" == -sSL && "$3" == -o ]]
    cp "$SUB2API_DEPLOY_TEST_ROOT/deploy/${2##*/}" "$4"
}
export -f curl

mkdir -p "$test_dir/random" "$test_dir/custom"
(
    cd "$test_dir/random"
    SUB2API_ADMIN_EMAIL= SUB2API_ADMIN_PASSWORD= SUB2API_POSTGRES_PASSWORD= \
        SUB2API_SERVER_PORT=8080 SUB2API_IMAGE=ghcr.io/sjz960913/sub2api-allinone:latest \
        bash "$SUB2API_DEPLOY_TEST_ROOT/deploy/docker-deploy.sh" > preparation.log
    grep -Eq '^ADMIN_EMAIL=admin-[0-9a-f]{12}@sub2api\.local$' .env
    grep -Eq '^ADMIN_PASSWORD=[0-9a-f]{64}$' .env
    grep -Eq '^POSTGRES_PASSWORD=[0-9a-f]{64}$' .env
    grep -Eq '^JWT_SECRET=[0-9a-f]{64}$' .env
    grep -Eq '^TOTP_ENCRYPTION_KEY=[0-9a-f]{64}$' .env
    cmp docker-compose.yml "$SUB2API_DEPLOY_TEST_ROOT/deploy/docker-compose.allinone.yml"
    test -d data && test -d postgres_data && test -d redis_data
    test "$(stat -c %a .env)" = 600
    before=$(sha256sum .env)
    printf 'n\n' | bash "$SUB2API_DEPLOY_TEST_ROOT/deploy/docker-deploy.sh" > cancel.log
    test "$(sha256sum .env)" = "$before"
)
(
    cd "$test_dir/custom"
    SUB2API_ADMIN_EMAIL=custom@example.com SUB2API_ADMIN_PASSWORD=test-admin-password \
        SUB2API_POSTGRES_PASSWORD=test-database-password SUB2API_SERVER_PORT=18080 \
        SUB2API_IMAGE=example/sub2api:custom \
        bash "$SUB2API_DEPLOY_TEST_ROOT/deploy/docker-deploy.sh" > preparation.log
    grep -Fxq 'ADMIN_EMAIL=custom@example.com' .env
    grep -Fxq 'ADMIN_PASSWORD=test-admin-password' .env
    grep -Fxq 'POSTGRES_PASSWORD=test-database-password' .env
    grep -Fxq 'SERVER_PORT=18080' .env
    grep -Fxq 'SUB2API_IMAGE=example/sub2api:custom' .env
    test "$(grep -c 'ADMIN_EMAIL:' preparation.log)" = 1
    grep -Fq 'custom@example.com' preparation.log
)

printf 'docker deploy all-in-one test passed\n'
