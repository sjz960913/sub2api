#!/usr/bin/env bash
# =============================================================================
# Sub2API HTTPS Reverse Proxy Setup Script
# =============================================================================
# Installs Nginx + Certbot, configures an HTTPS reverse proxy for Sub2API,
# enables HTTP -> HTTPS redirects, and keeps the Nginx header behavior required
# by Codex CLI sticky-session headers.
#
# Usage:
#   sudo DOMAIN=codecodelove.top UPSTREAM_PORT=18080 bash deploy/setup-https-nginx.sh
#
# Optional environment variables:
#   UPSTREAM_HOST=127.0.0.1
#   UPSTREAM_PORT=8080
#   EMAIL=admin@example.com
#   CLIENT_MAX_BODY_SIZE=256m
#   CERTBOT_STAGING=false
#   SKIP_CERTBOT=false
# =============================================================================

set -euo pipefail

DOMAIN="${DOMAIN:-}"
UPSTREAM_HOST="${UPSTREAM_HOST:-127.0.0.1}"
UPSTREAM_PORT="${UPSTREAM_PORT:-8080}"
EMAIL="${EMAIL:-}"
CLIENT_MAX_BODY_SIZE="${CLIENT_MAX_BODY_SIZE:-256m}"
CERTBOT_STAGING="${CERTBOT_STAGING:-false}"
SKIP_CERTBOT="${SKIP_CERTBOT:-false}"
SITE_NAME="${SITE_NAME:-sub2api}"

print_info() {
    printf '\033[0;34m[INFO]\033[0m %s\n' "$1"
}

print_success() {
    printf '\033[0;32m[SUCCESS]\033[0m %s\n' "$1"
}

print_warning() {
    printf '\033[1;33m[WARNING]\033[0m %s\n' "$1"
}

print_error() {
    printf '\033[0;31m[ERROR]\033[0m %s\n' "$1" >&2
}

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        print_error "Please run as root, for example: sudo DOMAIN=example.com bash $0"
        exit 1
    fi
}

validate_inputs() {
    if [ -z "${DOMAIN}" ]; then
        print_error "DOMAIN is required. Example: DOMAIN=codecodelove.top UPSTREAM_PORT=18080 bash $0"
        exit 1
    fi

    if ! printf '%s' "${UPSTREAM_PORT}" | grep -Eq '^[0-9]+$'; then
        print_error "UPSTREAM_PORT must be a number. Got: ${UPSTREAM_PORT}"
        exit 1
    fi
}

install_packages() {
    print_info "Installing Nginx and Certbot..."
    export DEBIAN_FRONTEND=noninteractive
    apt-get update
    apt-get install -y nginx certbot python3-certbot-nginx curl
    systemctl enable --now nginx
}

write_nginx_globals() {
    print_info "Writing Nginx global proxy settings..."
    cat > /etc/nginx/conf.d/sub2api_proxy_globals.conf <<'NGINX'
underscores_in_headers on;

map $http_upgrade $connection_upgrade {
    default upgrade;
    '' close;
}
NGINX
}

write_http_site() {
    local site_available="/etc/nginx/sites-available/${SITE_NAME}.conf"
    local site_enabled="/etc/nginx/sites-enabled/${SITE_NAME}.conf"

    print_info "Writing Nginx reverse proxy site for ${DOMAIN}..."
    mkdir -p /etc/nginx/sites-available /etc/nginx/sites-enabled

    cat > "${site_available}" <<NGINX
server {
    listen 80;
    listen [::]:80;
    server_name ${DOMAIN};

    client_max_body_size ${CLIENT_MAX_BODY_SIZE};

    location / {
        proxy_pass http://${UPSTREAM_HOST}:${UPSTREAM_PORT};
        proxy_http_version 1.1;

        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$connection_upgrade;

        proxy_buffering off;
        proxy_cache off;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
}
NGINX

    rm -f /etc/nginx/sites-enabled/default
    ln -sfn "${site_available}" "${site_enabled}"
    nginx -t
    systemctl reload nginx
}

check_upstream() {
    print_info "Checking upstream http://${UPSTREAM_HOST}:${UPSTREAM_PORT}/ ..."
    if curl -fsS -I --max-time 10 "http://${UPSTREAM_HOST}:${UPSTREAM_PORT}/" >/dev/null; then
        print_success "Upstream is reachable."
    else
        print_warning "Upstream did not return a successful HEAD response. Continuing because some apps reject HEAD or are still starting."
    fi
}

show_dns_warning() {
    local resolved_ips
    resolved_ips="$(getent ahostsv4 "${DOMAIN}" | awk '{print $1}' | sort -u | xargs || true)"

    if [ -z "${resolved_ips}" ]; then
        print_warning "DNS lookup for ${DOMAIN} returned no IPv4 address. Certbot HTTP-01 validation may fail until DNS is ready."
    else
        print_info "${DOMAIN} currently resolves to: ${resolved_ips}"
    fi
}

issue_certificate() {
    if [ "${SKIP_CERTBOT}" = "true" ]; then
        print_warning "SKIP_CERTBOT=true, leaving HTTP reverse proxy configured without HTTPS."
        return
    fi

    local certbot_args=(--nginx -d "${DOMAIN}" --non-interactive --agree-tos --redirect)

    if [ -n "${EMAIL}" ]; then
        certbot_args+=(--email "${EMAIL}")
    else
        certbot_args+=(--register-unsafely-without-email)
    fi

    if [ "${CERTBOT_STAGING}" = "true" ]; then
        certbot_args+=(--staging)
    fi

    print_info "Requesting and installing Let's Encrypt certificate..."
    certbot "${certbot_args[@]}"
    nginx -t
    systemctl reload nginx
}

verify_setup() {
    print_info "Verifying Nginx and Certbot status..."
    systemctl is-active --quiet nginx

    if [ "${SKIP_CERTBOT}" != "true" ]; then
        systemctl enable --now certbot.timer >/dev/null 2>&1 || true
        certbot certificates -d "${DOMAIN}" || true
        curl -fsS -I --max-time 20 "https://${DOMAIN}/" || true
        certbot renew --dry-run
    else
        curl -fsS -I --max-time 20 "http://${DOMAIN}/" || true
    fi
}

main() {
    require_root
    validate_inputs
    install_packages
    write_nginx_globals
    write_http_site
    check_upstream
    show_dns_warning
    issue_certificate
    verify_setup
    print_success "Done. Open: https://${DOMAIN}"
}

main "$@"
