#!/bin/bash
# =============================================================================
# Sub2API Docker Deployment Preparation Script
# =============================================================================
# This script prepares deployment files for Sub2API:
#   - Downloads the all-in-one Docker Compose file and env template from this fork
#   - Generates secure secrets (JWT_SECRET, TOTP_ENCRYPTION_KEY, POSTGRES_PASSWORD, ADMIN_PASSWORD)
#   - Applies default deployment values (image, port, admin account)
#   - Creates necessary data directories
#
# After running this script, you can start services with:
#   docker-compose up -d
# =============================================================================

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Defaults for this fork. Override any of these when running the script, e.g.:
#   SUB2API_SERVER_PORT=18080 SUB2API_ADMIN_EMAIL=admin@example.com bash docker-deploy.sh
GITHUB_REPO="${GITHUB_REPO:-sjz960913/sub2api}"
GITHUB_BRANCH="${GITHUB_BRANCH:-main}"
GITHUB_RAW_URL="${GITHUB_RAW_URL:-https://raw.githubusercontent.com/${GITHUB_REPO}/${GITHUB_BRANCH}/deploy}"

DEFAULT_SUB2API_IMAGE="${SUB2API_IMAGE:-ghcr.io/sjz960913/sub2api-allinone:latest}"
DEFAULT_SERVER_PORT="${SUB2API_SERVER_PORT:-8080}"
DEFAULT_ADMIN_EMAIL="${SUB2API_ADMIN_EMAIL:-admin@sub2api.local}"
DEFAULT_ADMIN_PASSWORD="${SUB2API_ADMIN_PASSWORD:-}"
DEFAULT_POSTGRES_PASSWORD="${SUB2API_POSTGRES_PASSWORD:-}"

# Print colored message
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

# Generate random secret
generate_secret() {
    openssl rand -hex 32
}

# Replace an existing KEY=value line in .env, or append it when missing.
set_env_value() {
    local key="$1"
    local value="$2"
    local tmp_file=".env.tmp.$$"

    if grep -q "^${key}=" .env; then
        awk -v key="${key}" -v value="${value}" '
            $0 ~ "^" key "=" {
                print key "=" value
                next
            }
            { print }
        ' .env > "${tmp_file}"
        mv "${tmp_file}" .env
    else
        printf '%s=%s\n' "${key}" "${value}" >> .env
    fi
}

# Check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Main installation function
main() {
    echo ""
    echo "=========================================="
    echo "  Sub2API Deployment Preparation"
    echo "=========================================="
    echo ""

    # Check if openssl is available
    if ! command_exists openssl; then
        print_error "openssl is not installed. Please install openssl first."
        exit 1
    fi

    # Check if deployment already exists
    if [ -f "docker-compose.yml" ] && [ -f ".env" ]; then
        print_warning "Deployment files already exist in current directory."
        read -p "Overwrite existing files? (y/N): " -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            print_info "Cancelled."
            exit 0
        fi
    fi

    # Download docker-compose.allinone.yml and save as docker-compose.yml
    print_info "Downloading docker-compose.yml..."
    if command_exists curl; then
        curl -sSL "${GITHUB_RAW_URL}/docker-compose.allinone.yml" -o docker-compose.yml
    elif command_exists wget; then
        wget -q "${GITHUB_RAW_URL}/docker-compose.allinone.yml" -O docker-compose.yml
    else
        print_error "Neither curl nor wget is installed. Please install one of them."
        exit 1
    fi
    print_success "Downloaded docker-compose.yml"

    # Download all-in-one .env.example
    print_info "Downloading .env.example..."
    if command_exists curl; then
        curl -sSL "${GITHUB_RAW_URL}/.env.allinone.example" -o .env.example
    else
        wget -q "${GITHUB_RAW_URL}/.env.allinone.example" -O .env.example
    fi
    print_success "Downloaded .env.example"

    # Download backup script
    print_info "Downloading backup-allinone.sh..."
    if command_exists curl; then
        curl -sSL "${GITHUB_RAW_URL}/backup-allinone.sh" -o backup-allinone.sh
    else
        wget -q "${GITHUB_RAW_URL}/backup-allinone.sh" -O backup-allinone.sh
    fi
    chmod +x backup-allinone.sh
    print_success "Downloaded backup-allinone.sh"

    # Generate .env file with auto-generated secrets
    print_info "Generating secure secrets..."
    echo ""

    # Generate secrets and apply deployment defaults
    JWT_SECRET=$(generate_secret)
    TOTP_ENCRYPTION_KEY=$(generate_secret)
    POSTGRES_PASSWORD="${DEFAULT_POSTGRES_PASSWORD}"
    ADMIN_PASSWORD="${DEFAULT_ADMIN_PASSWORD}"

    if [ -z "${POSTGRES_PASSWORD}" ]; then
        POSTGRES_PASSWORD=$(generate_secret)
    fi

    if [ -z "${ADMIN_PASSWORD}" ]; then
        ADMIN_PASSWORD=$(generate_secret)
    fi

    # Create .env from .env.example
    cp .env.example .env

    set_env_value "SUB2API_IMAGE" "${DEFAULT_SUB2API_IMAGE}"
    set_env_value "SERVER_PORT" "${DEFAULT_SERVER_PORT}"
    set_env_value "ADMIN_EMAIL" "${DEFAULT_ADMIN_EMAIL}"
    set_env_value "ADMIN_PASSWORD" "${ADMIN_PASSWORD}"
    set_env_value "JWT_SECRET" "${JWT_SECRET}"
    set_env_value "TOTP_ENCRYPTION_KEY" "${TOTP_ENCRYPTION_KEY}"
    set_env_value "POSTGRES_PASSWORD" "${POSTGRES_PASSWORD}"

    # Create data directories
    print_info "Creating data directories..."
    mkdir -p data postgres_data redis_data
    print_success "Created data directories"

    # Set secure permissions for .env file (readable/writable only by owner)
    chmod 600 .env
    echo ""

    # Display completion message
    echo "=========================================="
    echo "  Preparation Complete!"
    echo "=========================================="
    echo ""
    echo "Generated secure credentials:"
    echo "  SUB2API_IMAGE:         ${DEFAULT_SUB2API_IMAGE}"
    echo "  SERVER_PORT:           ${DEFAULT_SERVER_PORT}"
    echo "  ADMIN_EMAIL:           ${DEFAULT_ADMIN_EMAIL}"
    echo "  ADMIN_PASSWORD:        ${ADMIN_PASSWORD}"
    echo "  POSTGRES_PASSWORD:     ${POSTGRES_PASSWORD}"
    echo "  JWT_SECRET:            ${JWT_SECRET}"
    echo "  TOTP_ENCRYPTION_KEY:   ${TOTP_ENCRYPTION_KEY}"
    echo ""
    print_warning "These credentials have been saved to .env file."
    print_warning "Please keep them secure and do not share publicly!"
    echo ""
    echo "Directory structure:"
    echo "  docker-compose.yml        - Docker Compose configuration"
    echo "  .env                      - Environment variables (generated secrets)"
    echo "  .env.example              - Example template (for reference)"
    echo "  backup-allinone.sh        - One-click full backup script"
    echo "  data/                     - Application data (will be created on first run)"
    echo "  postgres_data/            - PostgreSQL data"
    echo "  redis_data/               - Redis data"
    echo ""
    echo "Next steps:"
    echo "  1. (Optional) Edit .env to customize configuration"
    echo "  2. Start services:"
    echo "     docker-compose up -d"
    echo ""
    echo "  3. View logs:"
    echo "     docker-compose logs -f sub2api"
    echo ""
    echo "  4. Access Web UI:"
    echo "     http://localhost:${DEFAULT_SERVER_PORT}"
    echo ""
    echo "  Backup all config and data:"
    echo "     ./backup-allinone.sh"
    echo ""
}

# Run main function
main "$@"
