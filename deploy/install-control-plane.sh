#!/bin/sh
set -eu

REPOSITORY="${JOBDOCK_RELEASE_REPOSITORY:-Alejandro-GZ/JobDock}"
RELEASES_URL="${JOBDOCK_RELEASES_URL:-https://github.com/$REPOSITORY/releases}"
CONFIG_DIR="${JOBDOCK_INSTALL_CONFIG_DIR:-/etc/jobdock}"
DATA_DIR="${JOBDOCK_INSTALL_DATA_DIR:-/var/lib/jobdock}"
RELEASES_DIR="${JOBDOCK_INSTALL_RELEASES_DIR:-/usr/local/lib/jobdock/releases}"
BIN_DIR="${JOBDOCK_INSTALL_BIN_DIR:-/usr/local/bin}"
HEALTH_TIMEOUT="${JOBDOCK_INSTALL_HEALTH_TIMEOUT:-180}"
DEFAULT_VERSION=""
VERSION="$DEFAULT_VERSION"
HTTP_PORT="8080"
PORT_SET=false
PUBLIC_URL=""
ADMIN_USERNAME="admin"
MODE=""
DOMAIN=""
ALLOW_INSECURE_HTTP=false

usage() {
  cat <<'EOF'
Install the JobDock control plane from one verified stable release.

Usage: install-control-plane.sh [options]

Options:
  --version VERSION       Install an explicit stable version (default: current stable)
  --mode MODE             Exposure mode: domain, proxy, or local
  --domain DOMAIN         Public DNS name for domain mode
  --port PORT             Publish the web console on this host port (default: 8080)
  --public-url URL        Required HTTPS URL for proxy mode
  --allow-insecure-http   Explicitly permit HTTP in local/LAN mode
  --admin-username NAME   Bootstrap administrator username (default: admin)
  --help                  Show this help

The supported system layout is /etc/jobdock for configuration,
/var/lib/jobdock for persistent state, and /usr/local/lib/jobdock/releases for
verified release assets. Reinstalling the same version preserves both config
and data.
EOF
}

fail() {
  printf 'JobDock installation failed: %s\n' "$*" >&2
  exit 1
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --version)
      [ "$#" -ge 2 ] || fail "--version requires a value"
      VERSION="$2"
      shift 2
      ;;
    --mode)
      [ "$#" -ge 2 ] || fail "--mode requires a value"
      MODE="$2"
      shift 2
      ;;
    --domain)
      [ "$#" -ge 2 ] || fail "--domain requires a value"
      DOMAIN="$2"
      shift 2
      ;;
    --port)
      [ "$#" -ge 2 ] || fail "--port requires a value"
      HTTP_PORT="$2"
      PORT_SET=true
      shift 2
      ;;
    --public-url)
      [ "$#" -ge 2 ] || fail "--public-url requires a value"
      PUBLIC_URL="$2"
      shift 2
      ;;
    --admin-username)
      [ "$#" -ge 2 ] || fail "--admin-username requires a value"
      ADMIN_USERNAME="$2"
      shift 2
      ;;
    --allow-insecure-http)
      ALLOW_INSECURE_HTTP=true
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *) fail "unknown option: $1" ;;
  esac
done

[ "$(uname -s 2>/dev/null || true)" = "Linux" ] || fail "Linux is required"
case "$(uname -m 2>/dev/null || true)" in
  x86_64|amd64) ;;
  *) fail "this release supports Linux amd64 only" ;;
esac

if [ "${JOBDOCK_INSTALL_ALLOW_NON_ROOT:-false}" != "true" ] && [ "$(id -u)" -ne 0 ]; then
  fail "run this installer as root (for example, curl ... | sudo sh)"
fi

case "$HTTP_PORT" in
  ''|*[!0-9]*) fail "--port must be an integer from 1 to 65535" ;;
esac
[ "$HTTP_PORT" -ge 1 ] && [ "$HTTP_PORT" -le 65535 ] || fail "--port must be an integer from 1 to 65535"
printf '%s\n' "$ADMIN_USERNAME" | grep -Eq '^[A-Za-z0-9_.@-]{3,64}$' || fail "--admin-username must contain 3 to 64 safe characters"

existing_environment="$CONFIG_DIR/jobdock.env"
stored_mode=""
if [ -f "$existing_environment" ]; then
  stored_mode=$(awk -F= '$1 == "JOBDOCK_EXPOSURE_MODE" {print $2; exit}' "$existing_environment")
  if [ -n "$stored_mode" ]; then
    if [ "$PORT_SET" = "false" ]; then
      HTTP_PORT=$(awk -F= '$1 == "JOBDOCK_HTTP_PORT" {print $2; exit}' "$existing_environment")
      [ -n "$HTTP_PORT" ] || HTTP_PORT=8080
    fi
    [ -n "$PUBLIC_URL" ] || PUBLIC_URL=$(awk -F= '$1 == "JOBDOCK_PUBLIC_URL" {sub(/^[^=]*=/, ""); print; exit}' "$existing_environment")
    [ -n "$DOMAIN" ] || DOMAIN=$(awk -F= '$1 == "JOBDOCK_DOMAIN" {print $2; exit}' "$existing_environment")
  fi
fi
if [ -n "$stored_mode" ]; then
  [ -z "$MODE" ] || [ "$MODE" = "$stored_mode" ] || fail "this installation uses $stored_mode mode; changing exposure mode requires the supported reconfiguration flow"
  MODE="$stored_mode"
fi
case "$MODE" in
  domain)
    [ -n "$DOMAIN" ] || fail "domain mode requires --domain"
    printf '%s\n' "$DOMAIN" | awk '
      length($0) > 253 || $0 ~ /[^A-Za-z0-9.-]/ || $0 ~ /\.\./ {exit 1}
      {count=split($0, labels, "."); if (count < 2 || labels[count] !~ /^[A-Za-z][A-Za-z0-9-]*$/) exit 1; for (i=1; i<=count; i++) if (labels[i] !~ /^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$/ || length(labels[i]) > 63) exit 1}
    ' || fail "--domain must be a valid fully qualified DNS name"
    expected_public_url="https://$DOMAIN"
    [ -z "$PUBLIC_URL" ] || [ "$PUBLIC_URL" = "$expected_public_url" ] || fail "domain mode public URL must be $expected_public_url"
    PUBLIC_URL="$expected_public_url"
    ;;
  proxy)
    case "$PUBLIC_URL" in https://*) ;; *) fail "proxy mode requires --public-url with an https:// URL" ;; esac
    ;;
  local)
    [ "$ALLOW_INSECURE_HTTP" = "true" ] || [ "$stored_mode" = "local" ] || fail "local mode requires explicit --allow-insecure-http"
    [ -n "$PUBLIC_URL" ] || PUBLIC_URL="http://localhost:$HTTP_PORT"
    case "$PUBLIC_URL" in http://*) ;; *) fail "local mode requires an http:// public URL" ;; esac
    ;;
  "") fail "--mode is required for a new installation (domain, proxy, or local)" ;;
  *) fail "--mode must be domain, proxy, or local" ;;
esac
case "$HTTP_PORT" in
  ''|*[!0-9]*) fail "stored HTTP port must be an integer from 1 to 65535" ;;
esac
[ "$HTTP_PORT" -ge 1 ] && [ "$HTTP_PORT" -le 65535 ] || fail "stored HTTP port must be an integer from 1 to 65535"

for command_name in curl sha256sum docker mktemp awk grep od tr base64 dd wc install id mkdir chmod chown sleep mv cat; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done
docker info >/dev/null 2>&1 || fail "Docker Engine is not available or the daemon is not running"
docker compose version >/dev/null 2>&1 || fail "the Docker Compose plugin is required"

if [ -n "$VERSION" ]; then
  VERSION=${VERSION#v}
  printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$' || fail "--version must be a semantic version such as 1.2.3"
  TAG="v$VERSION"
else
  latest_url=$(curl --fail --location --silent --show-error --output /dev/null --write-out '%{url_effective}' "$RELEASES_URL/latest") || fail "could not resolve the current stable release"
  TAG=${latest_url##*/}
  VERSION=${TAG#v}
  printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || fail "the current GitHub release is not a stable semantic version"
fi

case "$PUBLIC_URL" in
  http://*|https://*) ;;
  *) fail "--public-url must start with http:// or https://" ;;
esac
printf '%s\n' "$PUBLIC_URL" | grep -Eq '^https?://[^[:space:]#]+$' || fail "--public-url contains unsupported characters"

release_url="$RELEASES_URL/download/$TAG"
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

download() {
  asset="$1"
  curl --fail --location --silent --show-error --output "$temporary/$asset" "$release_url/$asset" || fail "could not download $asset from $TAG"
}

printf 'Resolving JobDock %s...\n' "$VERSION"
download SHA256SUMS
for asset in release-manifest.json docker-compose.yml docker-compose.domain.yml docker-compose.proxy.yml docker-compose.local.yml Caddyfile install-agent.sh jobdock-doctor; do
  download "$asset"
  awk -v expected="$asset" '$2 == expected || $2 == "*" expected {print}' "$temporary/SHA256SUMS" > "$temporary/$asset.sha256"
  [ -s "$temporary/$asset.sha256" ] || fail "SHA256SUMS does not cover $asset"
  (cd "$temporary" && sha256sum --check "$asset.sha256" >/dev/null) || fail "checksum verification failed for $asset"
done

grep -Eq 'image: "[^\"]+@sha256:[0-9a-f]{64}"' "$temporary/docker-compose.yml" || fail "release Compose does not contain immutable JobDock image references"
if grep -Eq 'jobdock-(server|builder):latest|@@JOBDOCK_' "$temporary/docker-compose.yml"; then
  fail "release Compose contains an unpinned JobDock image"
fi
grep -Fq "\"tag\":\"$TAG\"" "$temporary/release-manifest.json" || \
  grep -Eq "\"tag\"[[:space:]]*:[[:space:]]*\"$TAG\"" "$temporary/release-manifest.json" || \
  fail "release manifest tag does not match $TAG"
for component in server builder; do
  reference=$(awk -v component="/jobdock-$component" '
    $0 ~ /"image"[[:space:]]*:/ && index($0, component) {matched=1}
    matched && $0 ~ /"reference"[[:space:]]*:/ {
      line=$0
      sub(/^.*"reference"[[:space:]]*:[[:space:]]*"/, "", line)
      sub(/".*$/, "", line)
      print line
      exit
    }
  ' "$temporary/release-manifest.json")
  [ -n "$reference" ] || fail "release manifest does not contain the $component image reference"
  grep -Fq "image: \"$reference\"" "$temporary/docker-compose.yml" || fail "release Compose does not match the verified $component reference"
done

state_file="$CONFIG_DIR/install-state"
existing_install=false
if [ -f "$state_file" ]; then
  existing_install=true
  installed_version=$(awk -F= '$1 == "version" {print $2}' "$state_file")
  [ -z "$installed_version" ] || [ "$installed_version" = "$VERSION" ] || fail "JobDock $installed_version is installed; use the supported upgrade flow to install $VERSION"
fi

printf 'Installing verified assets into stable system paths...\n'
release_dir="$RELEASES_DIR/$VERSION"
secrets_dir="$CONFIG_DIR/secrets"
mkdir -p "$CONFIG_DIR" "$secrets_dir" "$release_dir" "$BIN_DIR" "$DATA_DIR/server" "$DATA_DIR/builder" "$DATA_DIR/buildkit"
chmod 0750 "$CONFIG_DIR" "$DATA_DIR" "$DATA_DIR/server" "$DATA_DIR/builder" "$DATA_DIR/buildkit"
if [ "$MODE" = "domain" ]; then
  mkdir -p "$DATA_DIR/caddy/data" "$DATA_DIR/caddy/config"
  chmod 0750 "$DATA_DIR/caddy" "$DATA_DIR/caddy/data" "$DATA_DIR/caddy/config"
fi
chmod 0700 "$secrets_dir"
chown 10001:10001 "$DATA_DIR/server"
chown 10002:10002 "$DATA_DIR/builder"
chown 1000:1000 "$DATA_DIR/buildkit"

install -m 0644 "$temporary/release-manifest.json" "$release_dir/release-manifest.json"
install -m 0644 "$temporary/docker-compose.yml" "$release_dir/docker-compose.yml"
for deployment_asset in docker-compose.domain.yml docker-compose.proxy.yml docker-compose.local.yml Caddyfile; do
  install -m 0644 "$temporary/$deployment_asset" "$release_dir/$deployment_asset"
done
install -m 0755 "$temporary/install-agent.sh" "$release_dir/install-agent.sh"
install -m 0755 "$temporary/jobdock-doctor" "$release_dir/jobdock-doctor"
install -m 0755 "$temporary/jobdock-doctor" "$BIN_DIR/jobdock-doctor"
install -m 0644 "$temporary/SHA256SUMS" "$release_dir/SHA256SUMS"
install -m 0644 "$temporary/docker-compose.yml" "$CONFIG_DIR/docker-compose.yml"
install -m 0644 "$temporary/docker-compose.$MODE.yml" "$CONFIG_DIR/docker-compose.exposure.yml"
install -m 0644 "$temporary/Caddyfile" "$CONFIG_DIR/Caddyfile"
install -m 0644 "$temporary/release-manifest.json" "$CONFIG_DIR/release-manifest.json"
install -m 0644 "$temporary/release-manifest.json" "$DATA_DIR/server/release-manifest.json"

environment_file="$CONFIG_DIR/jobdock.env"
overrides_file="$CONFIG_DIR/overrides.env"
legacy_builder_token=""
if [ -f "$environment_file" ]; then
  legacy_builder_token=$(awk -F= '$1 == "JOBDOCK_BUILDER_TOKEN" {sub(/^[^=]*=/, ""); print; exit}' "$environment_file")
fi

write_secret() {
  destination="$1"
  owner="$2"
  value="$3"
  temporary_secret="$destination.tmp.$$"
  umask 077
  printf '%s\n' "$value" > "$temporary_secret"
  chmod 0400 "$temporary_secret"
  chown "$owner" "$temporary_secret"
  mv "$temporary_secret" "$destination"
}

server_builder_secret="$secrets_dir/server-builder-token"
builder_secret="$secrets_dir/builder-token"
if [ -f "$server_builder_secret" ]; then
  builder_token=$(cat "$server_builder_secret")
elif [ -f "$builder_secret" ]; then
  builder_token=$(cat "$builder_secret")
elif [ -n "$legacy_builder_token" ]; then
  builder_token="$legacy_builder_token"
else
  builder_token=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
fi
[ ${#builder_token} -ge 32 ] || fail "the existing builder credential is invalid"
[ ! -f "$builder_secret" ] || [ "$(cat "$builder_secret")" = "$builder_token" ] || fail "server and builder credential files do not match"
[ -f "$server_builder_secret" ] || write_secret "$server_builder_secret" 10001:10001 "$builder_token"
[ -f "$builder_secret" ] || write_secret "$builder_secret" 10002:10002 "$builder_token"
chmod 0400 "$server_builder_secret" "$builder_secret"
chown 10001:10001 "$server_builder_secret"
chown 10002:10002 "$builder_secret"

master_key_secret="$secrets_dir/master-key"
if [ ! -f "$master_key_secret" ]; then
  if [ -f "$DATA_DIR/server/master.key" ]; then
    master_key=$(cat "$DATA_DIR/server/master.key")
  else
    master_key=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64 | tr -d '\n')
  fi
  write_secret "$master_key_secret" 10001:10001 "$master_key"
fi
decoded_key_bytes=$(base64 -d < "$master_key_secret" 2>/dev/null | wc -c | tr -d ' ')
[ "$decoded_key_bytes" = "32" ] || fail "the master key must be base64 for exactly 32 bytes"
chmod 0400 "$master_key_secret"
chown 10001:10001 "$master_key_secret"

setup_secret="$secrets_dir/setup-token"
new_setup_token=false
if [ ! -f "$setup_secret" ]; then
  setup_token=$(od -An -N48 -tx1 /dev/urandom | tr -d ' \n')
  write_secret "$setup_secret" 10001:10001 "$setup_token"
  if [ "$existing_install" = "false" ]; then
    new_setup_token=true
  fi
fi
[ "$(wc -c < "$setup_secret" | tr -d ' ')" -ge 33 ] || fail "the setup token file is invalid"
chmod 0400 "$setup_secret"
chown 10001:10001 "$setup_secret"

if [ ! -f "$environment_file" ]; then
  umask 027
  {
    printf 'COMPOSE_PROJECT_NAME=jobdock\n'
    printf 'JOBDOCK_HTTP_PORT=%s\n' "$HTTP_PORT"
    printf 'JOBDOCK_EXPOSURE_MODE=%s\n' "$MODE"
    printf 'JOBDOCK_PUBLIC_URL=%s\n' "$PUBLIC_URL"
    printf 'JOBDOCK_ALLOW_INSECURE_HTTP=%s\n' "$(case "$MODE" in local) printf true;; *) printf false;; esac)"
    printf 'JOBDOCK_TRUST_PROXY_HEADERS=%s\n' "$(case "$MODE" in domain|proxy) printf true;; *) printf false;; esac)"
    printf 'JOBDOCK_BIND_ADDRESS=%s\n' "$(case "$MODE" in proxy) printf 127.0.0.1;; *) printf 0.0.0.0;; esac)"
    if [ "$MODE" = "domain" ]; then
      printf 'JOBDOCK_DOMAIN=%s\n' "$DOMAIN"
      printf 'JOBDOCK_CADDYFILE_PATH=%s/Caddyfile\n' "$CONFIG_DIR"
      printf 'JOBDOCK_CADDY_DATA_DIR=%s/caddy/data\n' "$DATA_DIR"
      printf 'JOBDOCK_CADDY_CONFIG_DIR=%s/caddy/config\n' "$DATA_DIR"
    fi
    printf 'JOBDOCK_BOOTSTRAP_ADMIN_USERNAME=%s\n' "$ADMIN_USERNAME"
    printf 'JOBDOCK_DATA_DIR=%s/server\n' "$DATA_DIR"
    printf 'JOBDOCK_RELEASE_MANIFEST_PATH=/var/lib/jobdock/release-manifest.json\n'
    printf 'JOBDOCK_BUILDER_DATA_DIR=%s/builder\n' "$DATA_DIR"
    printf 'JOBDOCK_BUILDKIT_DATA_DIR=%s/buildkit\n' "$DATA_DIR"
    printf 'JOBDOCK_SETUP_SECRET_PATH=%s\n' "$setup_secret"
    printf 'JOBDOCK_MASTER_KEY_SECRET_PATH=%s\n' "$master_key_secret"
    printf 'JOBDOCK_SERVER_BUILDER_TOKEN_SECRET_PATH=%s\n' "$server_builder_secret"
    printf 'JOBDOCK_BUILDER_TOKEN_SECRET_PATH=%s\n' "$builder_secret"
  } > "$environment_file"
else
  sanitized_environment="$environment_file.tmp.$$"
  awk '$0 !~ /^JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD=/ && $0 !~ /^JOBDOCK_BUILDER_TOKEN=/' "$environment_file" > "$sanitized_environment"
  mv "$sanitized_environment" "$environment_file"
fi
chmod 0640 "$environment_file"

ensure_environment() {
  key="$1"
  value="$2"
  grep -q "^$key=" "$environment_file" || printf '%s=%s\n' "$key" "$value" >> "$environment_file"
}
set_environment() {
  key="$1"
  value="$2"
  updated_environment="$environment_file.tmp.$$"
  awk -v key="$key" 'index($0, key "=") != 1' "$environment_file" > "$updated_environment"
  printf '%s=%s\n' "$key" "$value" >> "$updated_environment"
  mv "$updated_environment" "$environment_file"
}
set_environment JOBDOCK_EXPOSURE_MODE "$MODE"
set_environment JOBDOCK_HTTP_PORT "$HTTP_PORT"
set_environment JOBDOCK_PUBLIC_URL "$PUBLIC_URL"
set_environment JOBDOCK_ALLOW_INSECURE_HTTP "$(case "$MODE" in local) printf true;; *) printf false;; esac)"
set_environment JOBDOCK_TRUST_PROXY_HEADERS "$(case "$MODE" in domain|proxy) printf true;; *) printf false;; esac)"
set_environment JOBDOCK_BIND_ADDRESS "$(case "$MODE" in proxy) printf 127.0.0.1;; *) printf 0.0.0.0;; esac)"
if [ "$MODE" = "domain" ]; then
  set_environment JOBDOCK_DOMAIN "$DOMAIN"
  set_environment JOBDOCK_CADDYFILE_PATH "$CONFIG_DIR/Caddyfile"
  set_environment JOBDOCK_CADDY_DATA_DIR "$DATA_DIR/caddy/data"
  set_environment JOBDOCK_CADDY_CONFIG_DIR "$DATA_DIR/caddy/config"
fi
ensure_environment JOBDOCK_SETUP_SECRET_PATH "$setup_secret"
ensure_environment JOBDOCK_MASTER_KEY_SECRET_PATH "$master_key_secret"
ensure_environment JOBDOCK_SERVER_BUILDER_TOKEN_SECRET_PATH "$server_builder_secret"
ensure_environment JOBDOCK_BUILDER_TOKEN_SECRET_PATH "$builder_secret"
ensure_environment JOBDOCK_RELEASE_MANIFEST_PATH /var/lib/jobdock/release-manifest.json
chmod 0640 "$environment_file"

if [ ! -f "$overrides_file" ]; then
  umask 027
  {
    printf '# Stable advanced overrides for the generated JobDock deployment.\n'
    printf '# Add supported JOBDOCK_* values here; do not edit docker-compose.yml.\n'
  } > "$overrides_file"
fi
chmod 0640 "$overrides_file"

compose() {
  docker compose --project-name jobdock --env-file "$environment_file" --env-file "$overrides_file" -f "$CONFIG_DIR/docker-compose.yml" -f "$CONFIG_DIR/docker-compose.exposure.yml" "$@"
}

printf 'Validating effective configuration...\n'
compose config --quiet || fail "effective configuration is invalid; review $overrides_file"
printf 'Pulling verified images...\n'
compose pull || fail "image pull failed; verified configuration remains in $CONFIG_DIR"
if [ "$MODE" = "domain" ]; then
  compose run --rm --no-deps caddy caddy validate --config /etc/caddy/Caddyfile || fail "Caddy configuration is invalid; review $CONFIG_DIR/Caddyfile and $overrides_file"
fi
printf 'Starting the JobDock control plane...\n'
compose up -d || fail "service startup failed; inspect with: docker compose --env-file $environment_file --env-file $overrides_file -f $CONFIG_DIR/docker-compose.yml -f $CONFIG_DIR/docker-compose.exposure.yml ps"

elapsed=0
case "$MODE" in
  domain) readiness_url="$PUBLIC_URL/health/ready" ;;
  *) readiness_url="http://127.0.0.1:$HTTP_PORT/health/ready" ;;
esac
while ! curl --fail --silent --show-error "$readiness_url" >/dev/null 2>&1; do
  if [ "$elapsed" -ge "$HEALTH_TIMEOUT" ]; then
    compose ps >&2 || true
    if [ "$MODE" = "domain" ]; then
      compose logs --tail=100 caddy >&2 || true
      fail "HTTPS readiness failed for $DOMAIN within ${HEALTH_TIMEOUT}s; verify that DNS points to this host and inbound TCP 80/443 and UDP 443 are reachable; the latest Caddy diagnostics are printed above"
    fi
    fail "readiness did not succeed within ${HEALTH_TIMEOUT}s; inspect with: docker compose --env-file $environment_file --env-file $overrides_file -f $CONFIG_DIR/docker-compose.yml -f $CONFIG_DIR/docker-compose.exposure.yml logs"
  fi
  sleep 1
  elapsed=$((elapsed + 1))
done

manifest_checksum=$(sha256sum "$temporary/release-manifest.json" | awk '{print $1}')
{
  printf 'version=%s\n' "$VERSION"
  printf 'tag=%s\n' "$TAG"
  printf 'manifest_sha256=%s\n' "$manifest_checksum"
} > "$state_file"
chmod 0640 "$state_file"

printf '\nJobDock %s is healthy.\n' "$VERSION"
printf 'Web console: %s\n' "$PUBLIC_URL"
printf 'Exposure mode: %s\n' "$MODE"
printf 'Configuration: %s\n' "$CONFIG_DIR"
printf 'Persistent data: %s\n' "$DATA_DIR"
if [ "$new_setup_token" = "true" ]; then
  printf 'Suggested administrator username: %s\n' "$ADMIN_USERNAME"
  printf 'One-time setup token: %s\n' "$setup_token"
  printf 'Open the web console to create the permanent administrator. This token will not be printed again.\n'
fi
