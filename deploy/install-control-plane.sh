#!/bin/sh
set -eu

REPOSITORY="${JOBDOCK_RELEASE_REPOSITORY:-Alejandro-GZ/JobDock}"
RELEASES_URL="${JOBDOCK_RELEASES_URL:-https://github.com/$REPOSITORY/releases}"
CONFIG_DIR="${JOBDOCK_INSTALL_CONFIG_DIR:-/etc/jobdock}"
DATA_DIR="${JOBDOCK_INSTALL_DATA_DIR:-/var/lib/jobdock}"
RELEASES_DIR="${JOBDOCK_INSTALL_RELEASES_DIR:-/usr/local/lib/jobdock/releases}"
HEALTH_TIMEOUT="${JOBDOCK_INSTALL_HEALTH_TIMEOUT:-180}"
DEFAULT_VERSION=""
VERSION="$DEFAULT_VERSION"
HTTP_PORT="8080"
PUBLIC_URL=""
ADMIN_USERNAME="admin"

usage() {
  cat <<'EOF'
Install the JobDock control plane from one verified stable release.

Usage: install-control-plane.sh [options]

Options:
  --version VERSION       Install an explicit stable version (default: current stable)
  --port PORT             Publish the web console on this host port (default: 8080)
  --public-url URL        Public URL advertised by JobDock (default: http://localhost:PORT)
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
    --port)
      [ "$#" -ge 2 ] || fail "--port requires a value"
      HTTP_PORT="$2"
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

for command_name in curl sha256sum docker mktemp awk grep od tr install id mkdir chmod chown sleep; do
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

[ -n "$PUBLIC_URL" ] || PUBLIC_URL="http://localhost:$HTTP_PORT"
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
for asset in release-manifest.json docker-compose.yml install-agent.sh; do
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
if [ -f "$state_file" ]; then
  installed_version=$(awk -F= '$1 == "version" {print $2}' "$state_file")
  [ -z "$installed_version" ] || [ "$installed_version" = "$VERSION" ] || fail "JobDock $installed_version is installed; use the supported upgrade flow to install $VERSION"
fi

printf 'Installing verified assets into stable system paths...\n'
release_dir="$RELEASES_DIR/$VERSION"
mkdir -p "$CONFIG_DIR" "$release_dir" "$DATA_DIR/server" "$DATA_DIR/builder" "$DATA_DIR/buildkit"
chmod 0750 "$CONFIG_DIR" "$DATA_DIR" "$DATA_DIR/server" "$DATA_DIR/builder" "$DATA_DIR/buildkit"
chown 10001:10001 "$DATA_DIR/server"
chown 10002:10002 "$DATA_DIR/builder"
chown 1000:1000 "$DATA_DIR/buildkit"

install -m 0644 "$temporary/release-manifest.json" "$release_dir/release-manifest.json"
install -m 0644 "$temporary/docker-compose.yml" "$release_dir/docker-compose.yml"
install -m 0755 "$temporary/install-agent.sh" "$release_dir/install-agent.sh"
install -m 0644 "$temporary/SHA256SUMS" "$release_dir/SHA256SUMS"
install -m 0644 "$temporary/docker-compose.yml" "$CONFIG_DIR/docker-compose.yml"
install -m 0644 "$temporary/release-manifest.json" "$CONFIG_DIR/release-manifest.json"

environment_file="$CONFIG_DIR/jobdock.env"
new_credentials=false
if [ ! -f "$environment_file" ]; then
  admin_password=$(od -An -N24 -tx1 /dev/urandom | tr -d ' \n')
  builder_token=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n')
  umask 077
  {
    printf 'COMPOSE_PROJECT_NAME=jobdock\n'
    printf 'JOBDOCK_HTTP_PORT=%s\n' "$HTTP_PORT"
    printf 'JOBDOCK_PUBLIC_URL=%s\n' "$PUBLIC_URL"
    printf 'JOBDOCK_ALLOW_INSECURE_HTTP=%s\n' "$(case "$PUBLIC_URL" in http://*) printf true;; *) printf false;; esac)"
    printf 'JOBDOCK_BOOTSTRAP_ADMIN_USERNAME=%s\n' "$ADMIN_USERNAME"
    printf 'JOBDOCK_BOOTSTRAP_ADMIN_PASSWORD=%s\n' "$admin_password"
    printf 'JOBDOCK_BUILDER_TOKEN=%s\n' "$builder_token"
    printf 'JOBDOCK_DATA_DIR=%s/server\n' "$DATA_DIR"
    printf 'JOBDOCK_BUILDER_DATA_DIR=%s/builder\n' "$DATA_DIR"
    printf 'JOBDOCK_BUILDKIT_DATA_DIR=%s/buildkit\n' "$DATA_DIR"
  } > "$environment_file"
  chmod 0600 "$environment_file"
  new_credentials=true
fi

compose() {
  docker compose --project-name jobdock --env-file "$environment_file" -f "$CONFIG_DIR/docker-compose.yml" "$@"
}

printf 'Pulling verified images...\n'
compose pull || fail "image pull failed; verified configuration remains in $CONFIG_DIR"
printf 'Starting the JobDock control plane...\n'
compose up -d || fail "service startup failed; inspect with: docker compose --env-file $environment_file -f $CONFIG_DIR/docker-compose.yml ps"

elapsed=0
while ! curl --fail --silent --show-error "http://127.0.0.1:$HTTP_PORT/health/ready" >/dev/null 2>&1; do
  if [ "$elapsed" -ge "$HEALTH_TIMEOUT" ]; then
    compose ps >&2 || true
    fail "readiness did not succeed within ${HEALTH_TIMEOUT}s; inspect with: docker compose --env-file $environment_file -f $CONFIG_DIR/docker-compose.yml logs"
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
printf 'Configuration: %s\n' "$CONFIG_DIR"
printf 'Persistent data: %s\n' "$DATA_DIR"
if [ "$new_credentials" = "true" ]; then
  printf 'Bootstrap username: %s\n' "$ADMIN_USERNAME"
  printf 'Bootstrap password: %s\n' "$admin_password"
  printf 'Store this password now; it will not be printed on reinstall.\n'
fi
